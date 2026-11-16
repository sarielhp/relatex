-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Unsupported original graphics remain visible as diagnostics, never page crops.
-- Inspect shipped nodes only: discarded scratch pictures are not document ink.
local M = {}
local source_name = require('relatex-source-name')
local attr = luatexbase.new_attribute('relatex.graphic.unsupported')
-- Set around ocgx2's link colouring (ocgcolorlinks): literals that clip to the link's own
-- glyphs and fill through them in a screen-only layer. Link styling, not a figure.
local link_attr = luatexbase.new_attribute('relatex.graphic.linkcolour')
-- Set around the tiny space ocgx2 sets at each end of a colour link: no text.
local space_attr = luatexbase.new_attribute('relatex.graphic.linkspace')
-- Set on a picture that paints nothing but text (M.text_only): its kerns only position the text.
local positioned_attr = luatexbase.new_attribute('relatex.graphic.positioned')
-- relatex-tables' context attribute, read to tell a table's rules from a picture's (decision 144).
-- relatex-tables loads after this module, so it is looked up when the first picture begins.
local table_attr
local function table_attribute()
  if table_attr == nil then table_attr = luatexbase.attributes and luatexbase.attributes['relatex.table'] or false end
  return table_attr or nil
end
local glyph, rule, whatsit = node.id('glyph'), node.id('rule'), node.id('whatsit')
local hlist, vlist = node.id('hlist'), node.id('vlist')
local literal = node.subtype('pdf_literal')
local contexts, stack, seen, pending, unknown = {}, {}, {}, {}, {}
local total, writer = 0, nil
local paint = {S=true, s=true, f=true, F=true, ['f*']=true, B=true, ['B*']=true,
  b=true, ['b*']=true, W=true, ['W*']=true, sh=true, BI=true, Do=true, BT=true, Tj=true, TJ=true}
local function emit(feature, reason, fields)
  fields = fields or {}
  fields.code, fields.source, fields.feature = 'unsupported_graphic_source', 'native-graphic-limits', feature
  fields.reason = reason
  if writer then writer('diagnostic', fields) else pending[#pending + 1] = fields end
end
local function string_end(data, position)
  local depth = 1
  position = position + 1
  while position <= #data and depth > 0 do
    local char = data:sub(position, position)
    if char == '\\' then position = position + 1
    elseif char == '(' then depth = depth + 1
    elseif char == ')' then depth = depth - 1 end
    position = position + 1
  end
  return depth == 0 and position or nil
end
-- PDF tokens, not a substring heuristic: ignore comments, names and strings.
function M.literal_paint(data)
  if type(data) ~= 'string' or #data > 262144 then return 'unparsed' end
  local position = 1
  while position <= #data do
    local char = data:sub(position, position)
    if char == '%' then position = (data:find('[\r\n]', position) or #data) + 1
    elseif char == '(' then
      position = string_end(data, position)
      if not position then return 'unparsed' end
    elseif char == '<' and data:sub(position+1, position+1) ~= '<' then
      local ending = data:find('>', position+1, true)
      if not ending then return 'unparsed' end
      position = ending + 1
    elseif char == '/' then
      position = position + 1
      local name = data:match('^[^%z%s%(%)<>%[%]{}/%%]+', position)
      position = position + (name and #name or 0)
    else
      local token = data:match('^[^%z%s%(%)<>%[%]{}/%%]+', position)
      if token and paint[token] then return token end
      position = position + (token and #token or 1)
    end
  end
end
function M.start(emit_record)
  writer = emit_record
  for _, fields in ipairs(pending) do writer('diagnostic', fields) end
  pending = {}
end
-- A tcolorbox draws its frame and background with a TikZ picture around the box it typeset
-- beforehand: the picture is decoration, the content is ordinary text. The next picture begun
-- is such a frame; its literals pass like link styling and it is no graphic context.
local frame_next = false
function M.frame_picture() frame_next = true end
-- Page furniture (a beamer theme's background, headline and footline: a slide-sized fill, the
-- navigation dots, a progress bar) is decoration too: every picture begun inside it is.
local decoration = 0
function M.decoration(on)
  decoration = math.max(decoration + (on and 1 or -1), 0)
end
function M.begin(kind)
  assert(#stack < 64, 'Unsupported graphics context depth limit')
  if frame_next or decoration > 0 then
    frame_next = false
    stack[#stack + 1] = {frame=true, prior=tex.getattribute(link_attr) or -0x7FFFFFFF}
    tex.setattribute(link_attr, 1)
    return
  end
  total = total + 1
  contexts[total] = {filename=source_name(status.filename) or tex.jobname, line=tex.inputlineno, kind=kind or 'tikz',
    table=table_attribute() and tex.getattribute(table_attr)}
  stack[#stack + 1] = {id=total, prior=tex.getattribute(attr) or -2147483647}
  tex.setattribute(attr, total)
end
local pictures = {}
-- A LaTeX picture in math is a symbol of its formula (an independence sign) and stays math, unless
-- it is the whole display: an equation holding only a drawn diagram (\put labels and \vector
-- arrows, third cohort 0768) is a figure, as the math oracle reads it (a picture-only display is
-- no formula). A candidate opens at the start of a display list; the display's conversion keeps it
-- only when nothing but the picture has ink (M.display_picture).
local noad, sub_box, sub_mlist = node.id('noad'), node.id('sub_box'), node.id('sub_mlist')
local glue, kern, penalty = node.id('glue'), node.id('kern'), node.id('penalty')
local display_mode
-- The fake-node unit harness has no modes and no callbacks.
for id, name in pairs(tex.getmodevalues and tex.getmodevalues() or {}) do if name == 'math' then display_mode = id end end
local candidates = {}
-- A math list item without ink: spacing, a whatsit, or an empty group noad (amsmath's equation
-- opens with one). `except` is the candidate picture's own noad.
local function blank_item(n, except)
  if n == except or n.id == whatsit or n.id == glue or n.id == kern or n.id == penalty then return true end
  if n.id ~= noad or n.sub or n.sup then return false end
  local nucleus = n.nucleus
  return not nucleus or (nucleus.id == sub_mlist and not nucleus.head)
end
local function blank_math(head, except)
  for n in node.traverse(head) do
    if not blank_item(n, except) then return false end
  end
  return true
end
M.blank_math = blank_math
local function display_start()
  local top = tex.nest.top
  return top.mode == display_mode and blank_math(top.head.next)
end
function M.begin_picture(in_math)
  local whole = in_math and display_start()
  pictures[#pictures + 1] = not in_math or whole
  if in_math and not whole then return end
  local before = total
  M.begin('picture')
  if whole and total > before then candidates[total] = true end
end
-- The candidate picture's noad in a display list: its nucleus is the picture's box.
local function candidate_noad(head)
  for n in node.traverse_id(noad, head) do
    local nucleus = n.nucleus
    local id = nucleus and nucleus.id == sub_box and nucleus.head and node.has_attribute(nucleus.head, attr)
    if id and candidates[id] then return n, nucleus.head, id end
  end
end
local function forget(head, id)
  for n in node.traverse(head) do
    if node.has_attribute(n, attr) == id then node.unset_attribute(n, attr) end
    if (n.id == hlist or n.id == vlist) and n.head then forget(n.head, id) end
  end
end
-- Judges a display's candidate picture when the display is converted: a figure when the display
-- holds nothing else with ink, else math again (its nodes lose the context).
function M.display_picture(head)
  local item, box, id = candidate_noad(head)
  if not item then return end
  candidates[id] = nil
  if blank_math(head, item) then return end
  node.unset_attribute(box, attr)
  forget(box.head, id)
  contexts[id] = nil
end
if luatexbase.add_to_callback then
  luatexbase.add_to_callback('pre_mlist_to_hlist_filter', function(head)
    M.display_picture(head)
    return true
  end, 'relatex.graphic.display_picture')
  -- Before any reader of the display: the picture is a figure or math when they see it.
  for _, reader in ipairs({'relatex.external.math', 'relatex.math.choices'}) do
    luatexbase.declare_callback_rule('pre_mlist_to_hlist_filter', 'relatex.graphic.display_picture', 'before', reader)
  end
end
-- Whether a picture's own nodes draw: a glyph, rule or literal made while it was open. Boxes,
-- kerns and glue only place material.
local function draws(head, id)
  for n in node.traverse(head) do
    if node.has_attribute(n, attr) == id and (n.id == glyph or n.id == rule or (n.id == whatsit and n.subtype == literal)) then
      return true
    end
    if (n.id == hlist or n.id == vlist) and n.head and draws(n.head, id) then return true end
  end
  return false
end
-- A picture that draws nothing of its own only places boxes typeset before it (picins's \parpic
-- puts its box in one): no graphic, its material reads as text (the book 0003, an algorithm in a
-- \parpic). \endpicture has just appended the picture's \mbox to the current list.
local function placement_only(id)
  local tail = tex.nest[tex.nest.ptr].tail
  if not (tail and tail.id == hlist and node.has_attribute(tail, attr) == id and tail.head) then return end
  if draws(tail.head, id) then return end
  node.unset_attribute(tail, attr)
  forget(tail.head, id)
  contexts[id] = nil
end
function M.end_picture()
  local began = table.remove(pictures)
  assert(began ~= nil, 'Unbalanced picture context')
  if not began then return end
  local frame = stack[#stack]
  M.finish_picture()
  if frame and frame.id then placement_only(frame.id) end
end
function M.finish_picture()
  local frame = assert(table.remove(stack), 'Unbalanced unsupported graphics context')
  tex.setattribute(frame.frame and link_attr or attr, frame.prior)
  if frame.id then M.text_only(frame.id) end
end
-- The picture being drawn: its context number, nil outside a picture or in a frame's drawing.
function M.current()
  local frame = stack[#stack]
  return frame and frame.id
end
function M.clip()
  local frame = stack[#stack]
  if frame and frame.id then contexts[frame.id].clip = true end
end
function M.preprocess(kind)
  emit('implicit-' .. kind:lower(), kind .. ' preprocessing is unsupported; shell escape remains disabled',
    {filename=source_name(status.filename) or tex.jobname, line=tex.inputlineno})
  texio.write_nl('term and log', 'relatex: unsupported graphic: ' .. kind .. ' preprocessing')
end
-- The prepared ordinary build converted EPS once (epstopdf's -eps-converted-to.pdf);
-- that PDF is what LaTeX reads and the graphics evidence binds.
function M.extension(extension, base)
  local kind = ({['.eps']='EPS', ['.ps']='EPS', ['.svg']='SVG'})[extension:lower()]
  if kind == 'EPS' and base and lfs.isfile(base .. '-eps-converted-to.pdf') then return end
  if kind then M.preprocess(kind) end
end
function M.link_colour(on)
  tex.setattribute(link_attr, on and 1 or -0x7FFFFFFF)
end

function M.link_space(on)
  tex.setattribute(space_attr, on and 1 or -0x7FFFFFFF)
end
function M.link_space_glyph(n)
  return n.id == glyph and node.has_attribute(n, space_attr) ~= nil
end

function M.retained(n, page)
  if n.id ~= glyph and n.id ~= rule and n.id ~= whatsit and n.id ~= hlist and n.id ~= vlist then return end
  local id = node.has_attribute(n, attr)
  local context = contexts[id]
  if context then
    if seen[id] then return end
    seen[id] = true
    local fields = {context=id, page=page, filename=context.filename, line=context.line}
    emit(context.kind, 'Direct ' .. context.kind .. ' original-figure recovery is unsupported', fields)
    if context.clip then
      emit('pgf-clip', 'PGF clipping is unsupported; only captured rectangular graphicx viewports are supported',
        {context=id, page=page, filename=context.filename, line=context.line})
    end
    return
  end
  if n.id ~= whatsit or n.subtype ~= literal then return end
  if node.has_attribute(n, link_attr) then return end
  -- Some LuaTeX builds return the lookup key "data" for token-backed PDF
  -- literals (get_pdf_literal_direct_value has a duplicated type branch).
  -- That is not evidence of an empty/state-only literal. Reject uninspectable
  -- native content explicitly rather than silently dropping possible paint.
  local data = n.data
  local operator = M.literal_paint(data ~= 'data' and data or nil)
  local key = tostring(page) .. ':' .. tostring(operator)
  if not operator or unknown[key] then return end
  unknown[key] = true
  emit('native-pdf-path', 'Unbound native PDF literal has no inspectable original graphic source', {page=page, operator=operator})
end
-- Where a graphic without an inspectable source sits on its page: its outermost box,
-- measured when the PDF is written. A zero-size late_lua node is added to the shipped box
-- only, after layout, so nothing moves; it asks LuaTeX for its actual PDF position.
-- A PGF picture paints through pgf's system layer (stroke, fill, an object such as a shading or
-- an image); its literals are token lists, not inspectable at shipout. The open picture records it.
function M.paint()
  local frame = stack[#stack]
  if frame and frame.id and contexts[frame.id] then contexts[frame.id].painted = true end
end
-- Whether a picture's nodes hold text (a glyph) or paint that is a node (a rule, an image). A
-- picture begun inside it is its ink too: pgfplots typesets an axis as a nested pgfpicture at
-- \end{axis}, so every label, mark and line of a plot belongs to the inner context and the outer
-- tikzpicture's own nodes hold none of it (a plot in a figure lost whole: slides 0019).
-- The rules of a table typeset in a node (its | columns, \hline) are the table's, not the
-- picture's paint: a node holding a tabular is a table, and a picture whose only ink is text and
-- such tables is no drawing (decision 144, slides/0029). Only a table begun inside the picture:
-- a picture in a table's cell paints its own rules in that table's context.
local function table_rule(n, context)
  local owner = table_attr and node.has_attribute(n, table_attr)
  return owner ~= nil and owner > 0 and owner ~= context.table
end
local function ink(head, id, found)
  for n in node.traverse(head) do
    local own = node.has_attribute(n, attr)
    if own == id or (own and contexts[own]) then
      if n.id == glyph then found.text = true elseif n.id == rule and not table_rule(n, contexts[own]) then found.paint = true end
      if own ~= id and contexts[own].painted then found.paint = true end
    end
    if (n.id == hlist or n.id == vlist) and n.head then ink(n.head, id, found) end
  end
  return found
end
-- The picture is no graphic: its nodes lose the attribute and its literals pass as styling.
local function strip(head, id)
  for m in node.traverse(head) do
    if node.has_attribute(m, attr) == id then
      node.unset_attribute(m, attr)
      if m.id == whatsit and m.subtype == literal then node.set_attribute(m, link_attr, 1) end
    end
    if (m.id == hlist or m.id == vlist) and m.head then strip(m.head, id) end
  end
end
-- A PGF picture that paints nothing but text (a TikZ node holding a frame title or a formula, with
-- no drawing: slides 0022, 0029) is no graphic: its text reads as text, placed by the picture's
-- literals, which are only positioning. It is judged when it closes, before the paragraph or box
-- around it is read, so its words are that paragraph's.
function M.text_only(id)
  local context = contexts[id]
  if not context or context.kind ~= 'tikz' or context.painted or not tex.nest then return end
  local tail = tex.nest[tex.nest.ptr].tail
  if not (tail and (tail.id == hlist or tail.id == vlist) and node.has_attribute(tail, attr) == id and tail.head) then return end
  local found = ink(tail.head, id, {})
  if not found.text or found.paint then return end
  node.unset_attribute(tail, attr)
  strip(tail.head, id)
  node.set_attribute(tail, positioned_attr, 1)
  contexts[id] = nil
end
function M.positioned(n) return node.has_attribute(n, positioned_attr) ~= nil end
-- Chess diagram fonts (decision 163): chessfss's board fonts by their TeX name, lowercase prefix
-- (skak10, SkakNew-Diagram); the export keeps the same fonts' glyphs as outlines (decision 137).
local chess_font_prefixes = {'skak'}
local chess_fonts = {}
local function chess_font(id)
  if chess_fonts[id] == nil then
    local data = font.getfont(id)
    local name = string.lower(data and data.name or '')
    chess_fonts[id] = false
    for _, prefix in ipairs(chess_font_prefixes) do
      if name:sub(1, #prefix) == prefix then chess_fonts[id] = true end
    end
  end
  return chess_fonts[id]
end
local function chess_glyph(head)
  for n in node.traverse(head) do
    if n.id == glyph and chess_font(n.font) then return true end
    if (n.id == hlist or n.id == vlist) and n.head and chess_glyph(n.head) then return true end
  end
  return false
end
-- A chess diagram's context is a drawing only when its board shows a glyph of a chess diagram
-- font: the board is recognised by its source (\chessboard) and its font. Judged when \chessboard
-- closes, over the nodes of the current list the board made; a board without one (print=false,
-- another font) loses the context and reads as before.
function M.chess_board(id)
  if not contexts[id] or not tex.nest then return end
  local nest = tex.nest[tex.nest.ptr]
  local made = {}
  for n in node.traverse(nest.head) do
    if node.has_attribute(n, attr) == id then made[#made + 1] = n end
  end
  for _, n in ipairs(made) do
    if (n.id == glyph and chess_font(n.font)) or ((n.id == hlist or n.id == vlist) and n.head and chess_glyph(n.head)) then
      return
    end
  end
  for _, n in ipairs(made) do
    node.unset_attribute(n, attr)
    if n.id == whatsit and n.subtype == literal then node.set_attribute(n, link_attr, 1) end
    if (n.id == hlist or n.id == vlist) and n.head then strip(n.head, id) end
  end
  contexts[id] = nil
end
-- A PGF picture without text that paints nothing (a tikzmark, a node kept only for its name) is no
-- graphic: its crop would be an empty image. One that paints outside its boxes, which TeX leaves
-- without area (an overlay: remember picture, overlay), has nothing a crop of them can show, so
-- it is reported as not reproduced. Both were 1 bp images in the export (slides 0029: marks and
-- arrows between formulas). A picture's outermost boxes on the page are judged together.
local function inkless(boxes, id)
  local context = contexts[id]
  if context.kind ~= 'tikz' then return false end
  local found, area = {}, false
  for _, n in ipairs(boxes) do
    found = ink(n.head, id, found)
    area = area or (n.width ~= 0 and n.height + n.depth ~= 0)
  end
  local painted = found.paint or context.painted
  if found.text or (painted and area) then return false end
  for _, n in ipairs(boxes) do
    node.unset_attribute(n, attr)
    strip(n.head, id)
  end
  contexts[id] = nil
  if painted then
    emit('overlay', 'A picture drawn outside its own box (an overlay) has no region to crop',
      {filename=context.filename, line=context.line})
  end
  return true
end
-- The outermost boxes of graphics in a list, in order.
local function outermost(head, boxes)
  for n in node.traverse(head) do
    if n.id == hlist or n.id == vlist then
      local id = node.has_attribute(n, attr)
      if id and contexts[id] then
        boxes[#boxes + 1] = n
      elseif n.head then
        outermost(n.head, boxes)
      end
    end
  end
  return boxes
end
-- A LaTeX picture puts its labels and lines where \put says, often outside the box its size
-- declares (papers3/0768 sets a label at x=0 of a picture whose origin is at x=2.8): its region is
-- the box grown to the ink of its glyphs and rules, measured from the box's left baseline.
local running = -1073741824
local function grow(found, left, low, right, high)
  found[1], found[2] = math.min(found[1], left), math.min(found[2], low)
  found[3], found[4] = math.max(found[3], right), math.max(found[4], high)
end
local ink_extent
local function ink_item(n, x, y, found)
  if n.id == glyph then
    grow(found, x, y - n.depth, x + n.width, y + n.height)
  elseif n.id == rule and n.width ~= running and n.height ~= running and n.depth ~= running then
    grow(found, x, y - n.depth, x + n.width, y + n.height)
  elseif (n.id == hlist or n.id == vlist) and n.head then
    ink_extent(n, x, y, found)
  end
end
-- The ink of box `box` whose left baseline is at (x, y), y upwards, into found {left, low, right, high}.
function ink_extent(box, x, y, found)
  if box.id == hlist then
    for n in node.traverse(box.head) do
      ink_item(n, x, y - ((n.id == hlist or n.id == vlist) and n.shift or 0), found)
      if n.id == glue then x = x + node.effective_glue(n, box)
      elseif n.id == kern then x = x + n.kern
      elseif n.id == glyph or n.id == hlist or n.id == vlist then x = x + n.width
      elseif n.id == rule and n.width ~= running then x = x + n.width end
    end
  else
    local top = y + box.height
    for n in node.traverse(box.head) do
      if n.id == glue then top = top - node.effective_glue(n, box)
      elseif n.id == kern then top = top - n.kern
      elseif n.id == hlist or n.id == vlist or n.id == rule then
        local height = n.height ~= running and n.height or 0
        ink_item(n, x + ((n.id == hlist or n.id == vlist) and n.shift or 0), top - height, found)
        top = top - height - (n.depth ~= running and n.depth or 0)
      end
    end
  end
  return found
end
-- The region to record for an outermost picture box: its own size, grown to its ink for a picture.
local function region(n, id)
  if contexts[id].kind ~= 'picture' or n.id ~= hlist then return 0, n.width, n.height, n.depth end
  local found = ink_extent(n, 0, 0, {0, -n.depth, n.width, n.height})
  -- Stretched or shrunk glue moves ink by fractions of a scaled point (slides/0013): the region is
  -- recorded in whole sp.
  local function sp(x) return math.floor(x + 0.5) end
  return sp(found[1]), sp(found[3] - found[1]), sp(found[4]), sp(-found[2])
end
-- The ids are read before any picture is judged: dropping one unsets its boxes' attribute.
function M.mark(head, page)
  local boxes, ids, pictures, dropped = outermost(head, {}), {}, {}, {}
  for index, n in ipairs(boxes) do
    local id = node.has_attribute(n, attr)
    ids[index] = id
    pictures[id] = pictures[id] or {}
    table.insert(pictures[id], n)
  end
  for index, n in ipairs(boxes) do
    local id = ids[index]
    if dropped[id] == nil then dropped[id] = inkless(pictures[id], id) end
    if not dropped[id] then
      local probe = node.new('whatsit', 'late_lua')
      local left, width, height, depth = region(n, id)
      probe.data = string.format("ltd.graphics.limits.position(%d,%d,%d,%d,%d,'%s',%d)",
        id, page, width, height, depth, n.id == hlist and 'hlist' or 'vlist', left)
      n.head = node.insert_before(n.head, n.head, probe)
    end
  end
end
-- Inside \resizebox, \scalebox or \rotatebox the box is drawn through a PDF matrix that TeX's
-- positions do not include: pdf.getpos is the untransformed point, and the box's natural size is
-- not what the page shows (a pgf plot scaled to the line width, third cohort 0587, 0776). The
-- region on the page is the bounding box of the box's corners mapped by the current matrix
-- (x' = a x + c y + e, y' = b x + d y + f); it is recorded as an hlist without depth.
local function transformed(x, y, width, height, depth, kind)
  local a, b, c, d, e, f = pdf.getmatrix()
  if not a or (a == 1 and b == 0 and c == 0 and d == 1 and e == 0 and f == 0) then return end
  local top = kind == 'hlist' and y + height or y
  local bottom = top - height - depth
  local left, right, low, high = math.huge, -math.huge, math.huge, -math.huge
  for _, corner in ipairs({{x, bottom}, {x + width, bottom}, {x, top}, {x + width, top}}) do
    local px, py = a * corner[1] + c * corner[2] + e, b * corner[1] + d * corner[2] + f
    left, right, low, high = math.min(left, px), math.max(right, px), math.min(low, py), math.max(high, py)
  end
  left, right, low, high = math.floor(left), math.ceil(right), math.floor(low), math.ceil(high)
  return left, low, right - left, high - low
end
-- At the start of an hlist the position is its left baseline; of a vlist, its top left.
function M.position(id, page, width, height, depth, kind, left_offset)
  local x, y = pdf.getpos()
  x = x + (left_offset or 0)
  local left, low, across, tall = transformed(x, y, width, height, depth, kind)
  if left then
    x, y, width, height, depth, kind = left, low, across, tall, 0, 'hlist'
  end
  if writer then
    writer('diagnostic', {code = 'graphic_position', source = 'native-graphic-limits', context = id, page = page,
      x_sp = x, y_sp = y, width_sp = width, height_sp = height, depth_sp = depth, box = kind})
  end
end
function M.finish()
  assert(#stack == 0, 'Unclosed unsupported graphics context')
  writer = nil
end
return M
