-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Decision 072: what TeX itself made of every formula of the ordinary build.
-- An observer on pre_mlist_to_hlist_filter: it reads the math list TeX built from the author's
-- source (macros expanded, packages applied) and changes no node, so the control build stays
-- pixel-identical to the reference. The acceptance checks read these records instead of
-- re-parsing the author's TeX.
--
-- One JSON line per math list: file and line TeX was reading when the list closed, the style
-- ('display' or 'text'), the environment (\@currenvir) and the noad tree. Lists TeX builds
-- twice (amsmath's measuring passes) appear twice; the reader of the stream drops the repeats.
local M = {}
local source_name = require('relatex-source-name')

local out
local count = 0
local DEPTH = 40

local SKIP = {next = true, prev = true, id = true, attr = true, user_id = true, subtype = true}

local function quote(text)
  return '"' .. text:gsub('[%c"\\]', function(c)
    if c == '"' then return '\\"' elseif c == '\\' then return '\\\\' end
    return string.format('\\u%04x', c:byte())
  end) .. '"'
end

local function encode(value)
  local kind = type(value)
  if kind == 'string' then return quote(value) end
  if kind == 'number' then return string.format('%d', value) end
  if kind == 'boolean' then return tostring(value) end
  if kind ~= 'table' then return 'null' end
  if #value > 0 or next(value) == nil then
    local parts = {}
    for i = 1, #value do parts[i] = encode(value[i]) end
    return '[' .. table.concat(parts, ',') .. ']'
  end
  local keys = {}
  for key in pairs(value) do keys[#keys + 1] = key end
  table.sort(keys)
  local parts = {}
  for _, key in ipairs(keys) do parts[#parts + 1] = quote(key) .. ':' .. encode(value[key]) end
  return '{' .. table.concat(parts, ',') .. '}'
end

local chain

-- The text of a box: its glyphs, with a space where it has glue; a formula's \text{...} and
-- \mbox{...} arrive as boxes.
local function box_text(head, depth)
  local text = {}
  for item in node.traverse(head) do
    local id = item.id
    if id == node.id('glyph') then
      text[#text + 1] = utf8.char(item.char)
    elseif id == node.id('glue') then
      text[#text + 1] = ' '
    elseif (id == node.id('hlist') or id == node.id('vlist')) and depth < DEPTH then
      text[#text + 1] = box_text(item.list, depth + 1)
    end
  end
  return table.concat(text)
end

-- Every node of a formula carries the formula's number, so the shipped page says which formulas
-- reached it. An attribute changes no typesetting.
local attribute
local current = 0

-- An xy-pic picture (\xy...\endxy, \xymatrix) draws with font glyphs and rules, not literals:
-- every node made between its ends carries this attribute, so the shipped page knows what it
-- holds is drawn, as a pgf picture's literals say for theirs. Pictures nest; the depth counts.
local picture
local picture_depth = 0
-- The literals of a tcolorbox's frame and background (its TikZ picture sets them around the
-- text it typeset beforehand) carry this attribute: they draw the box, not the text in it.
local frame
-- A PGF picture that paints nothing (no stroke, fill or object: a TikZ node holding only a frame
-- title or a formula) only positions what it holds: its box carries this attribute, and the
-- literals under it draw nothing. Painting marks every picture open at the time.
local unpainted
local pgf_pictures = {}

-- A node made inside a pgf/TikZ or xy-pic picture is picture content: its literals are the
-- picture's ink. The mark tells a drawing literal from a link's styling literal, which a
-- hyperlink may hold too (ocgx2's ocgcolorlinks).
local ink
local ink_depth = 0

local function picture_ink(open)
  ink_depth = math.max(ink_depth + (open and 1 or -1), 0)
  if ink then tex.setattribute(ink, ink_depth > 0 and 1 or -0x7FFFFFFF) end
end

function M.pgf_begin()
  pgf_pictures[#pgf_pictures + 1] = {painted = false}
  picture_ink(true)
end
function M.pgf_paint()
  for _, open in ipairs(pgf_pictures) do open.painted = true end
end
function M.pgf_end()
  picture_ink(false)
  local open = table.remove(pgf_pictures)
  if not open or open.painted or not unpainted then return end
  local tail = tex.nest[tex.nest.ptr].tail
  if tail and (tail.id == node.id("hlist") or tail.id == node.id("vlist")) then node.set_attribute(tail, unpainted, 1) end
end
-- \diagbox's picture draws only the diagonal between its labels, typeset before it: its line is the
-- cell's decoration, like a tcolorbox frame, not a drawing holding the labels.
function M.diagbox(open)
  if frame then tex.setattribute(frame, open and 1 or -0x7FFFFFFF) end
end

function M.picture(open)
  picture_depth = picture_depth + (open and 1 or -1)
  if picture_depth < 0 then picture_depth = 0 end
  picture_ink(open)
  if picture then tex.setattribute(picture, picture_depth > 0 and 1 or -0x7FFFFFFF) end
end

-- A LaTeX picture environment's labels: a list typeset while the environment is open belongs to the
-- picture. A formula that merely holds a picture (an independence symbol drawn with \put and \line)
-- is no picture's label: only the lists opened inside count.
local latex_picture_depth = 0

function M.latex_picture(open)
  latex_picture_depth = latex_picture_depth + (open and 1 or -1)
  if latex_picture_depth < 0 then latex_picture_depth = 0 end
end

-- Math set in \intertext or \shortintertext is the text between the rows of an alignment, not a
-- row of it: the document holds it as an inline formula next to the rows, never as a display row
-- (papers5/1370). The mark changes no node.
local intertext_math = false

function M.intertext(open)
  intertext_math = open and true or false
end

-- ocgx2's ocgcolorlinks wraps a hyperlink's text in literals that clip to the link's own glyphs
-- and fill through them in a screen-only layer (link styling): a hyperlink literal is not ink.
-- The literals made while such a link is open carry this attribute, so a formula the link encloses
-- is not drawn inside a picture.
local link
local link_depth = 0

function M.link_colour(open)
  link_depth = link_depth + (open and 1 or -1)
  if link_depth < 0 then link_depth = 0 end
  if link then tex.setattribute(link, link_depth > 0 and 1 or -0x7FFFFFFF) end
end

-- A footnote marker is typeset as a formula (LaTeX's \@makefnmark sets its mark in math), but it
-- is the note's mark: the document holds it as the mark of the note, not as a formula of its own.
-- The lists TeX builds while the mark is set are no formulas.
local note_mark = false

function M.note_mark(open)
  note_mark = open and true or false
end

-- QED marks (decision 176): the document holds a proof's end mark as one symbol, not a formula. A list TeX
-- builds while a QED command typesets its mark in text mode (\qedsymbol, IEEEtran's, \squareforqed) is the
-- mark's drawing; so is a one-symbol inline formula (no script) set by a proof environment's end code, between
-- its end and after hooks (\hfill$\Box$). A proof environment is `proof` or one whose name says proof (not a
-- derivation tree, prooftree), as the document knows them; this recorder finds them on its own.
local qed_frames, qed_depth, proof_ending = {}, 0, 0
local PROOF_WORDS = {"proof", "beweis", "preuve", "dimostrazione", "demostracion", "demostraci\195\179n"}
local QED_COMMANDS = {"IEEEQED", "IEEEQEDclosed", "IEEEQEDopen", "IEEEQEDhere", "squareforqed", "qedsymbol"}

function M.qed(open)
  if open then
    local counted = qed_depth == 0 and math.abs(tex.nest.top.mode) ~= 267 -- not in math (MATH_MODES)
    qed_frames[#qed_frames + 1] = counted
    if counted then qed_depth = qed_depth + 1 end
  elseif table.remove(qed_frames) then
    qed_depth = qed_depth - 1
  end
end

function M.proof_ending(open)
  proof_ending = math.max(0, proof_ending + (open and 1 or -1))
end

local function proof_environment(name)
  if type(name) ~= "string" or name:find("@", 1, true) then return false end
  local lower = name:lower()
  if lower:find("prooftree", 1, true) then return false end
  for _, word in ipairs(PROOF_WORDS) do
    if lower:find(word, 1, true) then return token.is_defined(name) and token.is_defined("end" .. name) end
  end
  return false
end

function M.qed_hooks()
  local hooks = {}
  for _, name in ipairs(tex.hashtokens()) do
    if proof_environment(name) then
      hooks[#hooks + 1] = "\\AddToHook{env/" .. name .. "/end}{\\directlua{require(\"relatex-oracle\").proof_ending(true)}}" ..
        "\\AddToHook{env/" .. name .. "/after}{\\directlua{require(\"relatex-oracle\").proof_ending(false)}}"
    end
  end
  for _, name in ipairs(QED_COMMANDS) do
    local ok, meaning = pcall(token.get_meaning, name)
    if token.is_defined(name) and ok and type(meaning) == "string" and meaning:find("->", 1, true) then
      hooks[#hooks + 1] = "\\AddToHook{cmd/" .. name .. "/before}{\\directlua{require(\"relatex-oracle\").qed(true)}}" ..
        "\\AddToHook{cmd/" .. name .. "/after}{\\directlua{require(\"relatex-oracle\").qed(false)}}"
    end
  end
  table.sort(hooks)
  tex.sprint(luatexbase.registernumber("catcodetable@atletter"), table.concat(hooks))
end

-- Whether a list is a QED mark: built inside a mark command, or the one-symbol formula of a proof's end code.
local function qed_mark(head, style)
  if qed_depth > 0 then return true end
  if proof_ending == 0 or style == "display" or style == "crampeddisplay" or not head or head.next then return false end
  if head.id ~= node.id("noad") or head.sub or head.sup then return false end
  return head.nucleus ~= nil and head.nucleus.id == node.id("math_char")
end

-- A display's tag is typeset by amsmath's \maketag@@@ (every form: \tag, \tag*, an automatic
-- number). Its math is a list of its own, but the document keeps it as a formula only when the
-- tag stands outside an alignment row: a single display's \eqno (equation, \[ \]). The marker
-- lets the reader tell that list from the display it sits beside.
local in_tag = false
-- \eqref also formats its number through \tagform@ (hence \maketag@@@), but it is a reference in
-- the text, not a display's tag: its lists are no tag math.
local suppressed_tag = 0

function M.tag(open)
  in_tag = open and true or false
end

function M.tag_suppress(open)
  suppressed_tag = suppressed_tag + (open and 1 or -1)
  if suppressed_tag < 0 then suppressed_tag = 0 end
end

-- LaTeX can hook only a macro: a deck that lets \@makefnmark to a primitive (\let\@makefnmark
-- \noindent, which sets no mark at all) would stop the build at \begin{document}. Such a command
-- typesets no mark to observe, so its hooks are skipped and the log says so.
function M.note_mark_hooks()
  -- A prefix wrapper, not a generic hook: LaTeX refuses cmd hooks on some classes' \@makefntext
  -- (revtex4: "Generic hooks cannot be added"), which stopped the build (papers/0087, 0210).
  if token.create('@makefntext').cmdname:match('call$') then
    tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
      '\\let\\ltd@oracle@makefntext\\@makefntext' ..
      '\\def\\@makefntext{\\directlua{require("relatex-oracle").note_text()}\\ltd@oracle@makefntext}')
  end
  local command = token.create('@makefnmark')
  if not command.cmdname:match('call$') then
    texio.write_nl('log', 'relatex-oracle: \\@makefnmark is not a macro; its mark hooks are skipped')
    return
  end
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\AddToHook{cmd/@makefnmark/before}{\\directlua{require("relatex-oracle").note_mark(true)}}' ..
    '\\AddToHook{cmd/@makefnmark/after}{\\directlua{require("relatex-oracle").note_mark(false)}}')
end

-- \maketag@@@ is wrapped at \begin{document}, once amsmath has defined it and the @-letter catcode
-- table is available. The wrapper only brackets the original with the marker, changing no node.
function M.tag_hooks()
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\ifdefined\\maketag@@@\\let\\ltd@oracle@maketag@@@\\maketag@@@' ..
    '\\def\\maketag@@@#1{\\directlua{require("relatex-oracle").tag(true)}' ..
    '\\ltd@oracle@maketag@@@{#1}\\directlua{require("relatex-oracle").tag(false)}}\\fi' ..
    '\\ifdefined\\eqref\\let\\ltd@oracle@eqref\\eqref' ..
    '\\protected\\def\\eqref#1{\\directlua{require("relatex-oracle").tag_suppress(true)}' ..
    '\\ltd@oracle@eqref{#1}\\directlua{require("relatex-oracle").tag_suppress(false)}}\\fi')
end

local function record(item, depth)
  local name = node.type(item.id)
  if attribute then node.set_attribute(item, attribute, current) end
  local entry = {type = name, subtype = item.subtype}
  if name == 'hlist' or name == 'vlist' then
    entry.text = box_text(item.list, depth + 1)
    return entry
  end
  if name == 'glyph' then
    entry.char, entry.font = item.char, item.font
    return entry
  end
  -- A \write's token list is read at shipout: reading it here as a field releases it, and the
  -- memory then holds other tokens (an \index in a formula wrote '\pth{...}' instead; the
  -- Geometric Approximation Algorithms book stopped the control build).
  if name == 'whatsit' and node.whatsits()[item.subtype] == 'write' then return entry end
  for _, field in ipairs(node.fields(item.id, item.subtype)) do
    if not SKIP[field] then
      local value = item[field]
      if type(value) == 'userdata' then
        entry[field] = chain(value, depth + 1)
      elseif type(value) == 'number' or type(value) == 'string' or type(value) == 'boolean' then
        entry[field] = value
      end
    end
  end
  return entry
end

function chain(head, depth)
  local items = {}
  if depth > DEPTH then return items end
  for item in node.traverse(head) do items[#items + 1] = record(item, depth) end
  return items
end

-- What TeX knows about where the list sits: the mode of the list being built when the callback
-- runs (an equation number is a math list inside the display), how many math lists enclose
-- it, and whether amsmath is only measuring (its trial passes build a list that is set again).
local MATH_MODES = {[-267] = true, [267] = true}
local function context()
  local enclosing = 0
  for level = 0, tex.nest.ptr do
    if MATH_MODES[tex.nest[level].mode] then enclosing = enclosing + 1 end
  end
  local measuring, truth = token.create("ifmeasuring@"), token.create("iftrue")
  return {mode = tex.nest.top.mode, enclosing = enclosing,
    trial = measuring.command == truth.command and measuring.mode == truth.mode}
end

-- An included graphic is a rule node of the image subtype, or of the box subtype when clipped (a form).
local GRAPHIC = {}
for key, name in pairs(node.subtypes("rule")) do if name == "image" or name == "box" then GRAPHIC[key] = true end end

-- Whether the typeset list holds ink: a glyph or a visible rule, looking into boxes. An included
-- graphic is no ink of a formula: a list holding only one (adjustbox's valign=c sets the image in
-- $\vcenter{\hbox{...}}$) is the figure, which the document holds as an image, not a formula.
local function has_ink(list, depth)
  for item in node.traverse(list) do
    local id = item.id
    if id == node.id("glyph") then return true end
    if id == node.id("rule") and not GRAPHIC[item.subtype] and item.width ~= 0 and item.height + item.depth > 0 then
      return true
    end
    if (id == node.id("hlist") or id == node.id("vlist")) and depth < DEPTH and has_ink(item.list, depth + 1) then
      return true
    end
  end
  return false
end

-- Where each list was opened (\everymath and \everydisplay call open): an amsmath align or gather
-- builds its rows as lists inside a display that never reaches the callback, so a row knows the
-- opening it sits in, and an opening that no list closes stands for the whole display.
local opens = {}
local open_count = 0

-- A formula set while \@outputpage builds the running head and foot is page furniture, not the document's.
local in_page = false
function M.output_page(state) in_page = state end

-- A formula set in acmart's copyright permission insert (the license, ISBN and DOI URL the class
-- prints at the foot of the first page) is publisher material too: the collector keeps that insert
-- as a front-matter decoration ('acmart-copyright', relatex-acm-artifacts.sty), never as the
-- document's math.
local in_permission = false
-- url.sty sets a URL (\url, \path, \doi) in math mode only for its mathcodes: that math list is text, so
-- it is furniture too (decision 194), recognised by \Url@String, defined only while a URL is typeset.
function M.permission(state) in_permission = state end

local encloses, orphan_top

-- A footnote's text is set in an insertion (\insert\footins), also when the \footnote stands in a
-- display (papers2/0235: a note in an equation*): its formulas are the note's, enclosed by no list
-- opened before it. The note lasts while the level \@makefntext began on is open (the insertion's
-- vertical list); `floor` is the last opening made before it.
local notes = {}
function M.note_text()
  local level = tex.nest[tex.nest.ptr]
  notes[#notes + 1] = {ptr = tex.nest.ptr, mode = level.mode, modeline = level.modeline, floor = open_count}
end

-- The last opening made before the innermost live note (0 outside notes).
local function note_floor()
  while #notes > 0 do
    local note = notes[#notes]
    local level = tex.nest[note.ptr]
    if note.ptr <= tex.nest.ptr and level and level.mode == note.mode and level.modeline == note.modeline then
      return note.floor
    end
    table.remove(notes)
  end
  return 0
end

function M.open(kind)
  -- An opening left by a display that never reached the callback no longer encloses anything once
  -- a new list opens outside it (the next align at the same level): it is closed here, or every
  -- later display would read as its rows (amsldoc: a multline's opening swallowed the displays after it).
  while #opens > 0 and not encloses(opens[#opens]) do orphan_top() end
  open_count = open_count + 1
  opens[#opens + 1] = {id = open_count, ptr = tex.nest.ptr, modeline = tex.nest[tex.nest.ptr].modeline, kind = kind,
    line = tex.inputlineno,
    file = source_name(status.filename), environment = token.get_macro("@currenvir"),
    tag = (in_tag and suppressed_tag == 0) or nil}
end

-- The callback runs after the list's nest level is gone: the opening that belongs to it is one
-- level deeper than the current one. Openings deeper than that never reached the callback.
orphan_top = function()
  local orphan = table.remove(opens)
  local enclosing = opens[#opens]
  out:write(encode({orphan = orphan.id, kind = orphan.kind, line = orphan.line, file = orphan.file,
    environment = orphan.environment, parent_open = enclosing and enclosing.id}), "\n")
end

-- Whether an opening still encloses the list being closed: its nest level is still a math level
-- that began on the same line (a display that never reached the callback is left behind, and a
-- later formula may sit at the level it had).
encloses = function(opening)
  local level = tex.nest[opening.ptr]
  return opening.ptr <= tex.nest.ptr and level and (level.mode == 267 or level.mode == -267) and
    level.modeline == opening.modeline
end

local function opening()
  local depth = tex.nest.ptr + 1
  while #opens > 0 and opens[#opens].ptr > depth do orphan_top() end
  local own
  if #opens > 0 and opens[#opens].ptr == depth then own = table.remove(opens) end
  while #opens > 0 and not encloses(opens[#opens]) do orphan_top() end
  local enclosing = opens[#opens]
  if enclosing and enclosing.id <= note_floor() then enclosing = nil end
  return own and own.id, enclosing and enclosing.id, own and own.kind
end

-- A record is written when its list has been typeset (the post callback), so it can say whether
-- the result has ink; lists nest, so the open ones wait on a stack. Records come out in the
-- order the lists closed; "n" is the order they opened.
function M.enable(path)
  if not path then return end
  out = io.open(path, "wb")
  attribute = luatexbase.new_attribute("relatex_oracle_formula")
  picture = luatexbase.new_attribute("relatex_oracle_picture")
  frame = luatexbase.new_attribute("relatex_oracle_frame")
  unpainted = luatexbase.new_attribute("relatex_oracle_unpainted")
  link = luatexbase.new_attribute("relatex_oracle_link")
  ink = luatexbase.new_attribute("relatex_oracle_ink")
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\AddToHook{cmd/diagbox@pict/before}{\\directlua{require("relatex-oracle").diagbox(true)}}' ..
    '\\AddToHook{cmd/diagbox@pict/after}{\\directlua{require("relatex-oracle").diagbox(false)}}' ..
    '\\def\\ltd@oracle@pgf{\\AddToHook{cmd/pgfpicture/before}{\\directlua{require("relatex-oracle").pgf_begin()}}' ..
    '\\AddToHook{cmd/endpgfpicture/after}{\\directlua{require("relatex-oracle").pgf_end()}}' ..
    '\\@for\\ltd@oracle@use:=stroke,fill,fillstroke,useobject\\do{' ..
    '\\AddToHook{cmd/pgfsys@\\ltd@oracle@use/before}{\\directlua{require("relatex-oracle").pgf_paint()}}}}' ..
    '\\@ifpackageloaded{pgf}{\\ltd@oracle@pgf}{\\AddToHook{package/pgf/after}{\\ltd@oracle@pgf}}')
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'), '\\AddToHook{begindocument/before}{\\everymath\\expandafter{\\the\\everymath\\directlua{require("relatex-oracle").open("text")}}' ..
    '\\everydisplay\\expandafter{\\the\\everydisplay\\directlua{require("relatex-oracle").open("display")}}' ..
    '\\AddToHook{cmd/@outputpage/before}{\\directlua{require("relatex-oracle").output_page(true)}}' ..
    '\\AddToHook{cmd/@outputpage/after}{\\directlua{require("relatex-oracle").output_page(false)}}' ..
    -- amsmath (and mathtools on top of it) defines \intertext anew inside every alignment, from
    -- its \intertext@: wrap both after it ran, so the lists an intertext argument typesets are
    -- marked. \shortintertext exists only with mathtools. The mark sits inside the argument: the
    -- `\\` these macros issue before their \noalign sets the row above, not the intertext text.
    '\\def\\ltd@oracle@intertext@wrap{' ..
    '\\ifdefined\\intertext\\let\\ltd@oracle@intertext\\intertext' ..
    '\\def\\intertext##1{\\ltd@oracle@intertext{\\directlua{require("relatex-oracle").intertext(true)}##1' ..
    '\\directlua{require("relatex-oracle").intertext(false)}}}\\fi' ..
    '\\ifdefined\\shortintertext\\let\\ltd@oracle@shortintertext\\shortintertext' ..
    '\\def\\shortintertext##1{\\ltd@oracle@shortintertext{\\directlua{require("relatex-oracle").intertext(true)}##1' ..
    '\\directlua{require("relatex-oracle").intertext(false)}}}\\fi}' ..
    '\\AddToHook{cmd/intertext@/after}{\\ltd@oracle@intertext@wrap}' ..
    '\\directlua{require("relatex-oracle").note_mark_hooks()}' ..
    '\\directlua{require("relatex-oracle").qed_hooks()}' ..
    '\\directlua{require("relatex-oracle").tag_hooks()}' ..
    -- A class that redefines \@makefnmark inside \maketitle (arxiv.sty) drops that hook; the
    -- \footnotemark of a \thanks still sets its mark through \@footnotemark.
    -- hyperref redefines it at \begin{document}, after command hooks are patched in, and its
    -- \footnotemark calls the saved kernel command directly: wrap both once every package has
    -- set them. The mark a class sets in a footnote's own text ($^{\@thefnmark}$ in arxiv.sty's
    -- \@makefntext) stays a formula, as the document holds it.
    '\\AddToHook{begindocument/end}{\\let\\ltdOracleFootnoteMark\\@footnotemark' ..
    '\\def\\@footnotemark{\\directlua{require("relatex-oracle").note_mark(true)}\\ltdOracleFootnoteMark' ..
    '\\directlua{require("relatex-oracle").note_mark(false)}}' ..
    '\\ifdefined\\H@@footnotemark\\let\\ltdOracleHyperFootnoteMark\\H@@footnotemark' ..
    '\\def\\H@@footnotemark{\\directlua{require("relatex-oracle").note_mark(true)}\\ltdOracleHyperFootnoteMark' ..
    '\\directlua{require("relatex-oracle").note_mark(false)}}\\fi}}')
  -- ocgx2 defines its colour-link begin and end macros at \begin{document}: wrap those definitions
  -- once they are final, so a hyperlink literal is marked as link styling and not as a drawing.
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\newcommand\\ltd@oracle@linkcolour[2]{' ..
    '\\ifcsname #1\\endcsname\\ifcsname #2\\endcsname' ..
    '\\expandafter\\let\\csname ltd@oracle@#1\\expandafter\\endcsname\\csname #1\\endcsname' ..
    '\\expandafter\\let\\csname ltd@oracle@#2\\expandafter\\endcsname\\csname #2\\endcsname' ..
    '\\expandafter\\protected\\expandafter\\def\\csname #1\\endcsname' ..
    '{\\directlua{require("relatex-oracle").link_colour(true)}\\csname ltd@oracle@#1\\endcsname}' ..
    '\\expandafter\\protected\\expandafter\\def\\csname #2\\endcsname' ..
    '{\\csname ltd@oracle@#2\\endcsname\\directlua{require("relatex-oracle").link_colour(false)}}' ..
    '\\fi\\fi}' ..
    '\\AddToHook{begindocument/end}{' ..
    '\\ltd@oracle@linkcolour{ocgxii_colourlink_begin:}{ocgxii_colourlink_end:}' ..
    '\\ltd@oracle@linkcolour{ocgxii_colourlink_nobreak_begin:}{ocgxii_colourlink_nobreak_end:}}')
  -- acmart sets its permission insert through manyfoot's \Footnotetextcopyrightpermission{mark}{text},
  -- which builds the insert (and typesets its formulas) while it runs: wrap it once the class has
  -- defined it.
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\def\\ltd@oracle@permission{\\ifdefined\\Footnotetextcopyrightpermission' ..
    '\\let\\ltd@oracle@permission@text\\Footnotetextcopyrightpermission' ..
    '\\long\\def\\Footnotetextcopyrightpermission##1##2{\\directlua{require("relatex-oracle").permission(true)}' ..
    '\\ltd@oracle@permission@text{##1}{##2}\\directlua{require("relatex-oracle").permission(false)}}\\fi}' ..
    '\\AddToHook{begindocument/end}{\\ltd@oracle@permission}')
  -- xy is often read with \input xy, not as a package; command hooks apply at \begin{document}.
  -- Only xy-pic's own \xy (its \xyoption is defined) opens a picture, not a paper's macro of that name.
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'), '\\AddToHook{cmd/xy/before}{\\ifdefined\\xyoption\\directlua{require("relatex-oracle").picture(true)}\\fi}' ..
    '\\AddToHook{cmd/endxy/after}{\\ifdefined\\xyoption\\directlua{require("relatex-oracle").picture(false)}\\fi}')
  -- A LaTeX picture environment (picture, eepic, Inkscape's .pdf_tex overlays) is one drawn figure:
  -- the formulas it sets as labels (\put{$w_i$}) are the picture's text, not formulas of the document.
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'), '\\AddToHook{env/picture/begin}{\\directlua{require("relatex-oracle").latex_picture(true)}}' ..
    '\\AddToHook{env/picture/end}{\\directlua{require("relatex-oracle").latex_picture(false)}}')
  -- Every skin's drawing (standard, enhanced, widget) is the box's frame: it draws around the text,
  -- not the formulas in it.
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\AddToHook{env/tcb@drawing/begin}' ..
    '{\\directlua{tex.setattribute(luatexbase.attributes["relatex_oracle_frame"],1)}}')
  -- Beamer's spacing cover (\pause, \uncover, \only, overlay environments) moves the origin
  -- away and back with two pdf_literal whatsits; mark them as frame literals so the frame's
  -- text is not treated as a drawn picture.
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\AddToHook{begindocument/before}{\\ifdefined\\beamer@@spacingcover' ..
    '\\def\\beamer@@spacingcover{\\beamer@smuggle{\\begingroup' ..
    '\\directlua{tex.setattribute(luatexbase.attributes["relatex_oracle_frame"],1)}' ..
    '\\pgfsys@begininvisible\\pgfsys@endinvisible\\endgroup}}\\fi}')
  -- A hyperlink literal is link styling, not ink (ocgx2's ocgcolorlinks). A literal inside a
  -- picture is that picture's ink even when a hyperlink encloses it.
  local function link_styling(item)
    return link and node.has_attribute(item, link) and not (ink and node.has_attribute(item, ink))
  end
  -- Whether a list's boxes hold a drawing literal (pgf/TikZ): a pdf_literal whatsit that is neither
  -- a tcolorbox frame nor under an unpainted PGF picture (whose literals only position their content).
  -- A display that is only such a picture is the picture, not a formula of the document (slides 0029);
  -- a display that boxes only text (\mbox{...}) holds no literal and stays a formula.
  local function draws_picture(list, quiet, depth)
    local item = list
    while item do
      local id = item.id
      if id == node.id("whatsit") then
        if node.whatsits()[item.subtype] == "pdf_literal" and not quiet and
          not node.has_attribute(item, frame) and not link_styling(item) then return true end
      else
        local positions = quiet
        if id == node.id("hlist") or id == node.id("vlist") then
          positions = quiet or (unpainted and node.has_attribute(item, unpainted) ~= nil)
        end
        if depth < DEPTH then
          for _, field in ipairs(node.fields(id, item.subtype)) do
            if not SKIP[field] then
              local value = item[field]
              if type(value) == 'userdata' and draws_picture(value, positions, depth + 1) then return true end
            end
          end
        end
      end
      item = item.next
    end
    return false
  end
  local open = {}
  -- The runs of adjacent primes TeX set, by run length, and the pieces the font joined them into
  -- (decision 149): {"2": {"2": 3}} when every two primes printed as one glyph. Nothing when the
  -- prime's font joins no primes.
  local primes = require("relatex-prime-ligatures")
  local prime_runs = {}
  luatexbase.add_to_callback("pre_mlist_to_hlist_filter", function(head, style)
    count = count + 1
    for _, run in ipairs(primes.runs(head, style)) do
      local length, pieces = tostring(run[1]), table.concat(run[2], "+")
      prime_runs[length] = prime_runs[length] or {}
      prime_runs[length][pieces] = (prime_runs[length][pieces] or 0) + 1
    end
    current = count
    local own, enclosing, kind = opening()
    open[#open + 1] = {n = count, file = source_name(status.filename), line = tex.inputlineno, style = style,
      environment = token.get_macro("@currenvir"), context = context(), noads = chain(head, 0),
      own_open = own, parent_open = enclosing, open_kind = kind, note_mark = note_mark,
      qed_mark = qed_mark(head, style) or nil,
      intertext = intertext_math or nil,
      tag = (in_tag and suppressed_tag == 0) or nil,
      furniture = (in_page or in_permission or token.is_defined("Url@String")) or nil,
      in_picture = latex_picture_depth > 0 or nil,
      picture = style == "display" and draws_picture(head, false, 0) or nil}
    return true
  end, "relatex-oracle")
  luatexbase.add_to_callback("post_mlist_to_hlist_filter", function(head)
    local record = table.remove(open)
    if record then
      record.ink = has_ink(head, 0)
      out:write(encode(record), "\n")
    end
    return head
  end, "relatex-oracle")
  -- At shipout: a formula is shipped when a glyph or rule of the page carries its number, and
  -- drawn when that sits inside a box holding a literal (pgf and TikZ draw by literals; the
  -- position of what they hold is not TeX's). Its text is what its glyphs decode to.
  local glyphs = require("relatex-glyphs")
  local shipped, drawn, texts = {}, {}, {}
  -- Every glyph of the pages in shipout order, as runs of the formula number it belongs to (0 for
  -- none): [number, count, number, count, ...], one list per page. The product numbers the same glyphs by its own
  -- formulas, so the two sets of numbers can be matched glyph by glyph.
  local sequence, run_number, run_count = {}, nil, 0
  local per_page = {}
  local rows = {}
  -- The first formula number found in a box, looking into boxes inside it.
  local function first_number(list, depth)
    for item in node.traverse(list) do
      local number = node.has_attribute(item, attribute)
      if number then return number end
      if (item.id == node.id("hlist") or item.id == node.id("vlist")) and depth < DEPTH then
        local found = first_number(item.list, depth + 1)
        if found then return found end
      end
    end
  end

  -- The cells of an alignment row are boxes side by side in the row's box: the numbers of the
  -- formulas typeset in them belong to one row.
  local function note_row(box)
    local numbers, seen = {}, {}
    if not box.list then return end
    for child in node.traverse(box.list) do
      if child.id == node.id("hlist") then
        local number = node.has_attribute(child, attribute) or (child.list and first_number(child.list, 0))
        if number and not seen[number] then seen[number] = true; numbers[#numbers + 1] = number end
      end
    end
    if #numbers > 1 then table.sort(numbers); rows[#rows + 1] = numbers end
  end

  local function scan(list, inside, quiet)
    for item in node.traverse(list) do
      local id = item.id
      local number = node.has_attribute(item, attribute)
      if id == node.id("glyph") then
        if (number or 0) == run_number then
          run_count = run_count + 1
        else
          if run_number then sequence[#sequence + 1] = run_number; sequence[#sequence + 1] = run_count end
          run_number, run_count = number or 0, 1
        end
      end
      -- A node made inside an xy-pic picture is drawn, whatever box holds it: the picture's own
      -- box, a formula that is only that picture, is drawn too.
      if number and picture and node.has_attribute(item, picture) then drawn[number] = true end
      if number and (id == node.id("glyph") or id == node.id("rule")) then
        shipped[number] = true
        if inside then drawn[number] = true end
        if id == node.id("glyph") then
          texts[number] = (texts[number] or "") .. (glyphs.character(item, 0) or "\u{FFFD}")
        end
      end
      -- A glyph that protrudes into the margin (microtype) hangs on a margin kern node.
      if id == node.id("margin_kern") and item.glyph then scan(item.glyph, inside, quiet) end
      -- A formula that is only a box (an aligned block) ships through it.
      -- Inside a picture (a \vdots in a TikZ matrix node) such a formula is drawn like its glyphs.
      if number and (id == node.id("hlist") or id == node.id("vlist")) then
        shipped[number] = true
        if inside then drawn[number] = true end
      end
      if id == node.id("hlist") then note_row(item) end
      if id == node.id("hlist") or id == node.id("vlist") then
        local literal = false
        -- Under an unpainted PGF picture literals only position: they draw nothing.
        local positions = quiet or (unpainted and node.has_attribute(item, unpainted) ~= nil)
        for child in node.traverse(item.list) do
          if not positions and child.id == node.id("whatsit") and node.whatsits()[child.subtype] == "pdf_literal"
            and not node.has_attribute(child, frame) and not link_styling(child) then literal = true end
        end
        scan(item.list, inside or literal, positions)
      end
    end
  end
  luatexbase.add_to_callback("pre_shipout_filter", function(box)
    sequence, run_number, run_count = {}, nil, 0
    scan(box, false)
    if run_number then sequence[#sequence + 1] = run_number; sequence[#sequence + 1] = run_count end
    per_page[#per_page + 1] = sequence
    return true
  end, "relatex-oracle")
  luatexbase.add_to_callback("stop_run", function()
    -- The displays of the document's last formulas never saw a later list to close them.
    while #opens > 0 do orphan_top() end
    local function numbers(set)
      local list = {}
      for number in pairs(set) do list[#list + 1] = number end
      table.sort(list)
      return list
    end
    local typeset = {}
    for _, number in ipairs(numbers(texts)) do typeset[#typeset + 1] = {number, texts[number]} end
    out:write(encode({pages = {shipped = numbers(shipped), drawn = numbers(drawn), text = typeset, glyphs = per_page, rows = rows,
      prime_runs = next(prime_runs) and prime_runs or nil}}), "\n")
    out:close()
  end, "relatex-oracle")
end

return M
