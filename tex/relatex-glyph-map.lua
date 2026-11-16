-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Decision 066: every shipped glyph with its page position, Unicode text and source line.
-- An observer at shipout/before: it reads the finished page and changes no node.
--
-- One JSON line per page (with the CPU seconds its capture took), columnar per glyph (all lengths in sp, page top-left origin, y down):
--   text, font, char (slot), dx and dy (baseline origin after graphicx transforms, as
--   differences from the previous glyph), ex (font expansion, millionths), tag, before, after
--   (source lines, below), atag (the after line's file tag), formula (the relatex.math.formula attribute, 0 outside math),
--   owner and slot (the relatex.flow.owner and relatex.flow.slot attributes, the flow item the glyph was
--   typeset from, 0 when none: decision 105's tagged PDF binds glyphs to the reading by them),
--   picture (the graphic context the glyph was set in: the relatex.graphic.unsupported attribute's
--   number of the tikzpicture, pgfpicture, LaTeX picture, xy-pic or QR code the export keeps as a
--   figure, 0 for the document's text, decision 162), flags;
--   metrics["font:char"] = {width, height, depth} before expansion;
--   matrices[i] = {a, b, c, d} for glyph i drawn under a graphicx transform;
--   images, the boxes of included graphics and pictures; pictures, those TeX typeset itself; rasters,
--   those of included PNG or JPEG images (decision 162).
-- flags: 1 a word boundary follows, 2 hyphen added by a discretionary break, 4 last glyph of
-- a typeset line, 8 inside a literal-drawn box (pgf/TikZ) or covered Beamer material: position not
-- known from TeX.
--
-- Glyphs carry no SyncTeX data, but interword spaces and user glue record the line on which
-- TeX made them. A glyph gets two of those lines, which bracket the line it was read on:
-- before, of the last such glue before it (else of its box), and after, of the first one
-- after it in the same typeset line (0 when none). The space ending a source line records
-- the next line, so after can exceed the word's own line by one. Font kerns and line-break
-- glue carry the paragraph's last line and are ignored.
local d = node.direct
local glyphs = require('relatex-glyphs')
local leaders = require('relatex-leaders')
local M = {}

local GLYPH, HLIST, VLIST, RULE, GLUE, KERN, WHATSIT, DISC, MATH =
  node.id('glyph'), node.id('hlist'), node.id('vlist'), node.id('rule'), node.id('glue'),
  node.id('kern'), node.id('whatsit'), node.id('disc'), node.id('math')
local MARGIN_KERN = node.id('margin_kern')
local names = node.whatsits()
-- userskip, spaceskip, xspaceskip: glue made from the author's input
local SOURCE_GLUE = {[0] = true, [13] = true, [14] = true}
local BOUNDARY, HYPHEN, LINE_END, UNKNOWN = 1, 2, 4, 8
-- An included graphic is a rule node of the image subtype, or of the box subtype when it is clipped (a form).
local GRAPHIC = {}
for id, name in pairs(node.subtypes('rule')) do if name == 'image' or name == 'box' then GRAPHIC[id] = true end end

local page_number = 0
local formula_attr, graphic_attr, owner_attr, slot_attr, state_attr, link_attr, cover_id
-- Covered Beamer material's depth at the walk's place, in shipping order over the whole page (see literal_box).
local covered = 0
local columns, fonts, metrics, matrices, pending, font_seen, images, pictures, rasters
-- The glyphs whose node the engine made a ligature of (it says so by their components).
local ligature = {}
-- Unicode text per font and character slot; ligatures are decoded each time.
local texts = {}
-- Per page: metrics[font][char] = {width, height, depth}, written as "font:char" keys.
-- The last source glue seen, kept across pages: a page that opens mid-paragraph continues the
-- line the previous page ended on.
local previous_line = {0, 0}
local matrix, stack, page_height
-- No transform is active (the common case): positions need no matrix product.
local plain = true

local function reset()
  columns = {text = {}, font = {}, char = {}, x = {}, y = {}, ex = {}, tag = {}, before = {}, after = {}, atag = {},
             formula = {}, owner = {}, slot = {}, picture = {}, flags = {}}
  fonts, metrics, matrices, pending, font_seen, images, pictures, rasters = {}, {}, {}, {}, {}, {}, {}, {}
  ligature = {}
  matrix, stack, plain = {1, 0, 0, 1, 0, 0}, {}, true
  covered = 0
end

local function compose(m, n) -- m after n, PDF orientation (y up)
  return {m[1]*n[1] + m[3]*n[2], m[2]*n[1] + m[4]*n[2], m[1]*n[3] + m[3]*n[4], m[2]*n[3] + m[4]*n[4],
          m[1]*n[5] + m[3]*n[6] + m[5], m[2]*n[5] + m[4]*n[6] + m[6]}
end

local function identity()
  return matrix[1] == 1 and matrix[2] == 0 and matrix[3] == 0 and matrix[4] == 1
end

-- The glyphs since the last source glue take this glue's line as their after line.
local function settle(tag, line)
  for _, index in ipairs(pending) do columns.after[index], columns.atag[index] = line, tag end
  pending = {}
end

local function mark(flag)
  local last = #columns.flags
  if last > 0 and columns.flags[last] & flag == 0 then columns.flags[last] = columns.flags[last] + flag end
end

-- Font expansion (microtype) widens glyphs and font kerns by their expansion factor, in
-- millionths; the backend places them at the expanded advance.
local function expanded(n, amount)
  local factor = d.getexpansion(n) or 0
  return factor == 0 and amount or math.floor(amount * (1 + factor / 1000000) + 0.5)
end

-- A virtual font's character is a small program; one that moves before setting its first real
-- character is drawn displaced from TeX's pen position (mathptmx's math italic f and p).
-- font.read_vf reads the .vf the backend uses; TFM fonts only, once per font.
local virtual = {}
local function virtual_offset(font_id, char)
  local chars = virtual[font_id]
  if chars == nil then
    local f = font.getfont(font_id) or {}
    local ok, vf = false, nil
    if f.name and (f.format == nil or f.format == 'unknown') and not f.filename then
      ok, vf = pcall(font.read_vf, f.name, f.size)
    end
    chars = ok and vf and vf.characters or false
    virtual[font_id] = chars
  end
  local commands = chars and chars[char] and chars[char].commands
  if not commands then return 0, 0 end
  local dx, dy = 0, 0
  for _, command in ipairs(commands) do
    local name = command[1]
    if name == 'char' or name == 'slot' or name == 'rule' or name == 'image' then break end
    if name == 'right' then dx = dx + command[2] elseif name == 'down' then dy = dy + command[2] end
  end
  return dx, dy
end

local function emit(n, h, v, flags)
  -- OpenType math and node-mode font features displace a glyph from its pen position; the
  -- backend draws it at the displaced point (y offset upward).
  local xoffset, yoffset = d.getoffsets(n)
  local font_id, char = d.getfont(n), d.getchar(n)
  local vx, vy = virtual_offset(font_id, char)
  local x, y = h + (xoffset or 0) + vx, page_height - v + (yoffset or 0) - vy
  local px, py = x, y
  if not plain then
    px = matrix[1]*x + matrix[3]*y + matrix[5]
    py = matrix[2]*x + matrix[4]*y + matrix[6]
  end
  local index = #columns.text + 1
  if not font_seen[font_id] then
    font_seen[font_id] = true
    local f = font.getfont(font_id) or {}
    fonts[tostring(font_id)] = {name = f.fullname or f.psname or f.name or '', size = f.size or 0}
  end
  local font_texts = texts[font_id]
  if not font_texts then font_texts = {}; texts[font_id] = font_texts end
  local text = font_texts[char]
  if not text then
    text = glyphs.character(d.tonode(n)) or ''
    if d.getsubtype(n) & 2 == 0 then font_texts[char] = text end
  end
  local font_metrics = metrics[font_id]
  if not font_metrics then font_metrics = {}; metrics[font_id] = font_metrics end
  if not font_metrics[char] then font_metrics[char] = {d.getwidth(n), d.getheight(n), d.getdepth(n)} end
  columns.text[index], columns.font[index], columns.char[index] = text, font_id, char
  columns.x[index], columns.y[index] = math.floor(px + 0.5), math.floor(page_height - py + 0.5)
  columns.ex[index] = d.getexpansion(n) or 0
  columns.tag[index], columns.before[index], columns.after[index] = previous_line[1], previous_line[2], 0
  columns.atag[index] = 0
  columns.formula[index] = formula_attr and d.has_attribute(n, formula_attr) or 0
  local owner = owner_attr and d.has_attribute(n, owner_attr) or 0
  if owner > 0 then
    columns.owner[index], columns.slot[index] = owner, math.max(d.has_attribute(n, slot_attr) or 0, 0)
  else
    columns.owner[index], columns.slot[index] = 0, 0
  end
  -- Read, never set: the attribute the graphic limits gave the glyph when it was made (and took
  -- away again from a picture judged no figure), so the column changes no page (decision 162).
  local picture = graphic_attr and d.has_attribute(n, graphic_attr) or 0
  columns.picture[index] = picture > 0 and picture or 0
  columns.flags[index] = flags
  if not plain and not identity() then matrices[tostring(index)] = {matrix[1], matrix[2], matrix[3], matrix[4]} end
  if d.getfield(n, 'components') then ligature[index] = true end
  pending[#pending + 1] = index
end

-- Whether a list holds a literal that leaves its text's position unknown: any literal but those TeX
-- marks as moving no text (relatex.graphic.state, tex/relatex-beamer.sty). LuaTeX 1.24 reads a
-- token-backed literal's data back as its field name, so a literal's operators cannot be read; the
-- mark is set where TeX knows the literal's role (a11y-827, decision 171): 1 on the literals of pgf's
-- colour macros (colour operators only), 4 on the two 2000 bp translations that bracket Beamer's
-- covered material (\pause, \uncover). Without them one colour change in a frame's list left every
-- glyph of the frame unknown (slides 0027: 10250 of 16646). The glyphs between the translations
-- stay unknown: the cover markers beside them (relatex.lua cover_mark) are counted over the page in
-- shipping order, as relatex.lua counts them, since \pause opens a cover in one line and the frame's
-- end closes it in another list. A literal in a picture (the graphic attribute) or a tcolorbox
-- frame's (linkcolour) is never trusted.
local function trusted_literal(n)
  local state = state_attr and d.has_attribute(n, state_attr)
  return (state == 1 or state == 4) and not (graphic_attr and d.has_attribute(n, graphic_attr))
    and not (link_attr and d.has_attribute(n, link_attr))
end

local function literal_box(list)
  for n, id, sub in d.traverse(list) do
    if id == WHATSIT and names[sub] == 'pdf_literal' and not trusted_literal(n) then return true end
  end
  return false
end

-- A cover marker's shift (+1 opens covered material, -1 closes it), else 0.
local function cover_shift(n, sub)
  if not cover_id or names[sub] ~= 'user_defined' or d.getfield(n, 'user_id') ~= cover_id then return 0 end
  return d.getfield(n, 'value') or 0
end

local function covers(n, sub)
  local shift = cover_shift(n, sub)
  if shift ~= 0 then covered = math.max(covered + shift, 0) end
end

local function whatsit(n, sub, h, v)
  local name = names[sub]
  if name == 'pdf_save' then stack[#stack + 1] = matrix
  elseif name == 'pdf_restore' then
    matrix = table.remove(stack) or {1, 0, 0, 1, 0, 0}
    plain = identity() and matrix[5] == 0 and matrix[6] == 0
  elseif name == 'pdf_setmatrix' then
    local a, b, c, e = tostring(d.getdata(n)):match('^%s*(%S+)%s+(%S+)%s+(%S+)%s+(%S+)%s*$')
    a, b, c, e = tonumber(a), tonumber(b), tonumber(c), tonumber(e)
    if a and b and c and e then
      -- graphicx applies the matrix around the current point.
      local x, y = h, page_height - v
      matrix = compose(matrix, {a, b, c, e, x - a*x - c*y, y - b*x - e*y})
      plain = identity() and matrix[5] == 0 and matrix[6] == 0
    end
  end
end

-- An included graphic's box on the page (sp, from the top): the text inside it belongs to the
-- graphic, not to the document's reading.
local function image(n, h, v, picture)
  local width, height, depth = d.getwidth(n), d.getheight(n), d.getdepth(n)
  local corners = {{h, page_height - v - depth}, {h + width, page_height - v - depth},
                   {h, page_height - v + height}, {h + width, page_height - v + height}}
  local x0, y0, x1, y1
  for _, corner in ipairs(corners) do
    local x, y = corner[1], corner[2]
    if not plain then
      x, y = matrix[1]*corner[1] + matrix[3]*corner[2] + matrix[5], matrix[2]*corner[1] + matrix[4]*corner[2] + matrix[6]
    end
    x0, x1 = math.min(x0 or x, x), math.max(x1 or x, x)
    y0, y1 = math.min(y0 or y, y), math.max(y1 or y, y)
  end
  local box = string.format('[%d,%d,%d,%d]', math.floor(x0 + 0.5), math.floor(page_height - y1 + 0.5),
    math.floor(x1 + 0.5), math.floor(page_height - y0 + 0.5))
  images[#images + 1] = box
  -- A picture (typeset by TeX itself) is also listed apart: the glyphs inside it are the figure's own.
  if picture then pictures[#pictures + 1] = box
  -- A raster image (an included PNG or JPEG) paints no text: it is listed apart too (decision 162).
  elseif ltd and ltd.graphics and ltd.graphics.raster and ltd.graphics.raster(d.tonode(n)) then
    rasters[#rasters + 1] = box
  end
end

-- Whether a list, or a box in it, holds a glyph: a box without one sets no text.
local function holds_glyph(list)
  for n, id in d.traverse(list) do
    if id == GLYPH then return true end
    if (id == HLIST or id == VLIST) and holds_glyph(d.getlist(n)) then return true end
    if id == DISC and holds_glyph(d.getfield(n, 'replace')) then return true end
  end
  return false
end

-- A box set inside an unsupported graphic (a LaTeX picture such as \diagbox's, a TikZ picture)
-- is that graphic's: the export keeps the figure, so the text in it is not the document's. A box
-- without a glyph (PGF's shading canvas, a 100 bp square centred on the picture's origin, which
-- spreads over the caption below a shaded frame) sets no text and claims none.
local function graphic_box(box, h, baseline)
  local context = graphic_attr and d.has_attribute(box, graphic_attr)
  if context and context > 0 and holds_glyph(d.getlist(box)) then image(box, h, baseline, true) end
end

local hlist, vlist

local function source_glue(n)
  local tag, line = d.get_synctex_fields(n)
  if tag and tag > 0 and line and line > 0 and SOURCE_GLUE[d.getsubtype(n)] then return tag, line end
end

-- The size of the font of the glyph last read: a gap much narrower than a space is no word boundary.
local last_em = 0
local MIN_SPACE_EM = 0.1

local function walk_list(list, h, v, unknown, box)
  for n, id, sub in d.traverse(list) do
    if id == GLYPH then
      emit(n, h, v, (unknown or covered > 0) and UNKNOWN or 0)
      last_em = (font.getfont(d.getfont(n)) or {}).size or last_em
      h = h + expanded(n, d.getwidth(n))
    elseif id == HLIST then
      -- A box inside a line (a listing's character box, \mbox, \raisebox) is part of the running
      -- text: it neither ends a word nor a line.
      hlist(n, h, v + d.getshift(n), unknown)
      h = h + d.getwidth(n)
    elseif id == VLIST then
      mark(BOUNDARY)
      vlist(n, h, v + d.getshift(n) - d.getheight(n), unknown)
      h = h + d.getwidth(n)
    elseif id == GLUE then
      local tag, line = source_glue(n)
      if tag then settle(tag, line); previous_line = {tag, line} end
      local effective = d.effective_glue(n, box)
      local hairline = effective ~= 0 and effective < MIN_SPACE_EM * last_em
      if (d.getwidth(n) ~= 0 or tag) and not hairline then mark(BOUNDARY) end
      -- A box leader set once (acmart's affiliation comma) paints its box's glyphs there (relatex-leaders).
      local leader = d.getleader(n)
      local at = leader and leaders.single_copy(d.tonode(n), box and d.tonode(box))
      if at then
        local shifted = v + d.getshift(leader)
        if d.getid(leader) == HLIST then hlist(leader, h + at, shifted, unknown)
        else vlist(leader, h + at, shifted - d.getheight(leader), unknown) end
      end
      h = h + effective
    elseif id == KERN then h = h + expanded(n, d.getkern(n))
    -- microtype's protrusion: the margin kerns that hang a line's first and last glyph out.
    elseif id == MARGIN_KERN then h = h + d.getwidth(n)
    elseif id == RULE then
      mark(BOUNDARY)
      if GRAPHIC[sub] then image(n, h, v) end
      h = h + d.getwidth(n)
    elseif id == MATH then h = h + (d.getkern(n) or 0)
    elseif id == WHATSIT then
      covers(n, sub)
      whatsit(n, sub, h, v)
    elseif id == DISC then
      -- An unbroken discretionary shows its replacement; at a break LuaTeX has already
      -- moved the pre- and post-break material into the lines.
      h = walk_list(d.getfield(n, 'replace'), h, v, unknown, box)
    end
  end
  return h
end

function hlist(box, h, v, unknown, line_box)
  graphic_box(box, h, v)
  unknown = unknown or literal_box(d.getlist(box))
  local before = #columns.text
  walk_list(d.getlist(box), h, v, unknown, box)
  if #columns.text > before then
    -- Glyphs after the last source glue of the box take the box's line, made when the box was
    -- finished, as their after line; glyphs that never saw one before them (the page's first)
    -- take it as their before line too.
    local tag, line = d.get_synctex_fields(box)
    if tag and tag > 0 and line and line > 0 then
      for _, index in ipairs(pending) do
        if columns.before[index] == 0 then columns.tag[index], columns.before[index] = tag, line end
        if line >= columns.before[index] then columns.after[index], columns.atag[index] = line, tag end
      end
    end
    pending = {}
    -- Only a typeset line's own box ends its last word and the line.
    if line_box then
      mark(LINE_END)
      mark(BOUNDARY)
    end
  end
end

function vlist(box, h, top, unknown)
  graphic_box(box, h, top + d.getheight(box))
  unknown = unknown or literal_box(d.getlist(box))
  local v = top
  for n, id, sub in d.traverse(d.getlist(box)) do
    if id == HLIST or id == VLIST then
      v = v + d.getheight(n)
      if id == HLIST then hlist(n, h + d.getshift(n), v, unknown, true)
      else vlist(n, h + d.getshift(n), v - d.getheight(n), unknown) end
      v = v + d.getdepth(n)
    elseif id == GLUE then v = v + d.effective_glue(n, box)
    elseif id == KERN then v = v + d.getkern(n)
    elseif id == RULE then
      -- A rule in a vertical list hangs from the current position: its baseline is its height below it.
      if GRAPHIC[sub] then image(n, h, v + d.getheight(n)) end
      v = v + d.getheight(n) + d.getdepth(n)
    elseif id == WHATSIT then
      covers(n, sub)
      whatsit(n, sub, h, v)
    end
  end
end

-- A ligature's node can be a copy the flow and math captures never annotated: a discretionary
-- break's replacement keeps the source word's characters but a fresh node (the glyph after a
-- hyphen break), and a box captured with its own attributes yields another copy than the text
-- the formula capture saw. Its neighbours in the same word are the same reading, so let them
-- answer for it; a word boundary follows a glyph it is set on. Words never mix owners, so a
-- neighbour's owner is the ligature's.
local function ligature_owner(count)
  for index = 1, count do
    if ligature[index] and columns.owner[index] == 0 and columns.formula[index] == 0 then
      -- Right first (the word continues), then left (it began before a break). Only a glyph of
      -- the text answers: a neighbouring formula's glyph says nothing about this word.
      local donor
      for other = index + 1, count do
        if columns.flags[other - 1] & BOUNDARY ~= 0 then break end
        if columns.owner[other] > 0 and columns.formula[other] == 0 then donor = other; break end
      end
      if not donor then
        for other = index - 1, 1, -1 do
          if columns.flags[other] & BOUNDARY ~= 0 then break end
          if columns.owner[other] > 0 and columns.formula[other] == 0 then donor = other; break end
        end
      end
      -- A donor is text (formula 0), so the ligature's formula stays 0.
      if donor then columns.owner[index], columns.slot[index] = columns.owner[donor], columns.slot[donor] end
    end
  end
  -- The other copy: inside \text the ligature is set in a box the formula capture read as its
  -- unligatured characters, so it kept neither formula nor its neighbours'. It lies between two
  -- glyphs of one formula in its word: it is that formula's.
  for index = 1, count do
    if ligature[index] and columns.formula[index] == 0 then
      local left = index > 1 and columns.flags[index - 1] & BOUNDARY == 0 and index - 1 or nil
      local right = index < count and columns.flags[index] & BOUNDARY == 0 and index + 1 or nil
      if left and right and columns.formula[left] > 0 and columns.formula[left] == columns.formula[right]
          and columns.owner[left] == columns.owner[right] then
        columns.formula[index] = columns.formula[left]
      end
    end
  end
end

-- The page record without its closing brace. Numeric columns are written directly; x and y
-- as differences from the previous glyph, which repeat and compress far better than positions.
local ESCAPES = {['"'] = '\\"', ['\\'] = '\\\\', ['\n'] = '\\n', ['\r'] = '\\r', ['\t'] = '\\t'}
local function json_string(text)
  if not text:find('[%c"\\]') then return '"' .. text .. '"' end
  local escaped = text:gsub('[%c"\\]', function(char) return ESCAPES[char] or string.format('\\u%04x', char:byte()) end)
  return '"' .. escaped .. '"'
end

local function record()
  ligature_owner(#columns.text)
  local function ints(values) return '[' .. table.concat(values, ',') .. ']' end
  local function deltas(values)
    local result, previous = {}, 0
    for index, value in ipairs(values) do result[index] = value - previous; previous = value end
    return ints(result)
  end
  local json = utilities.json.tostring
  local function strings(values)
    local result = {}
    for index, value in ipairs(values) do result[index] = json_string(value) end
    return '[' .. table.concat(result, ',') .. ']'
  end
  local sizes = {}
  for font_id, chars in pairs(metrics) do
    for char, size in pairs(chars) do
      sizes[#sizes + 1] = string.format('"%d:%d":[%d,%d,%d]', font_id, char, size[1], size[2], size[3])
    end
  end
  local parts = {string.format('{"page":%d,"width":%d,"height":%d', page_number, tex.pagewidth, page_height),
    '"fonts":' .. json(fonts), '"metrics":{' .. table.concat(sizes, ',') .. '}', '"matrices":' .. json(matrices),
    '"glyphs":{"text":' .. strings(columns.text), '"font":' .. ints(columns.font), '"char":' .. ints(columns.char),
    '"dx":' .. deltas(columns.x), '"dy":' .. deltas(columns.y), '"ex":' .. ints(columns.ex),
    '"tag":' .. ints(columns.tag), '"before":' .. ints(columns.before), '"after":' .. ints(columns.after), '"atag":' .. ints(columns.atag),
    '"formula":' .. ints(columns.formula), '"owner":' .. ints(columns.owner), '"slot":' .. ints(columns.slot),
    '"picture":' .. ints(columns.picture),
    '"flags":' .. ints(columns.flags) .. '}',
    '"images":[' .. table.concat(images, ',') .. ']',
    '"pictures":[' .. table.concat(pictures, ',') .. ']',
    '"rasters":[' .. table.concat(rasters, ',') .. ']'}
  return table.concat(parts, ',')
end

-- Only the final pass's glyph map is kept, and a conversion's final pass is never its first,
-- unless the build starts from converged files (RELATEX_SEEDED): the first pass of an unseeded
-- build is not captured.
function M.capture(box, path)
  if os.getenv('RELATEX_PASS') == '1' and os.getenv('RELATEX_SEEDED') ~= '1' then return end
  local started = os.clock()
  page_number = page_number + 1
  formula_attr = formula_attr or (luatexbase.attributes and luatexbase.attributes['relatex.math.formula'])
  graphic_attr = graphic_attr or (luatexbase.attributes and luatexbase.attributes['relatex.graphic.unsupported'])
  owner_attr = owner_attr or (luatexbase.attributes and luatexbase.attributes['relatex.flow.owner'])
  slot_attr = slot_attr or (luatexbase.attributes and luatexbase.attributes['relatex.flow.slot'])
  state_attr = state_attr or (luatexbase.attributes and luatexbase.attributes['relatex.graphic.state'])
  link_attr = link_attr or (luatexbase.attributes and luatexbase.attributes['relatex.graphic.linkcolour'])
  cover_id = cover_id or (package.loaded['relatex'] or {}).cover_id
  reset()
  page_height = tex.pageheight
  local b = d.todirect(box)
  -- The shipped box sits at the PDF origin (1in by default) plus \hoffset and \voffset.
  local h0, v0 = pdf.getorigin()
  h0, v0 = h0 + tex.hoffset, v0 + tex.voffset
  if d.getid(b) == VLIST then vlist(b, h0, v0) else hlist(b, h0, v0 + d.getheight(b)) end
  local file = assert(io.open(path, page_number == 1 and 'wb' or 'ab'))
  file:write(record(), string.format(',"cpu":%.6f}', os.clock() - started), '\n')
  file:close()
end

return M
