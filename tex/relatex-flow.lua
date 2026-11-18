-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Logical spaces are captured before line breaking can remove their glue.
local M = {}
local owner_attr = luatexbase.new_attribute('relatex.flow.owner')
local slot_attr = luatexbase.new_attribute('relatex.flow.slot')
local reference_attr = luatexbase.new_attribute('relatex.flow.reference')
local fragment_attr = luatexbase.new_attribute('relatex.flow.fragment')
local symbol_attr = luatexbase.new_attribute('relatex.flow.symbol')
-- 1 on a glyph that shares the slot of the glyph before it: the post-break half of a ligature a line
-- break split (broken_slots). It is shipped text, but its slot is already read.
local echo_attr = luatexbase.new_attribute('relatex.flow.echo')
-- A run-in heading's title (\@svsechd), set at the start of the paragraph it opens.
local runin_attr = luatexbase.new_attribute('relatex.flow.runin')
-- A display's tag: the material amsmath's \maketag@@@ (or the kernel's \@eqnnum) sets, never an
-- \eqref's number, which is formatted the same way in the text (approved 2026-11-10).
local tag_attr = luatexbase.new_attribute('relatex.flow.tag')
local glyph, rule, glue, hlist, vlist, disc = node.id('glyph'), node.id('rule'), node.id('glue'), node.id('hlist'), node.id('vlist'), node.id('disc')
local penalty = node.id('penalty')
local kern = node.id('kern')
local math_node = node.id('math')
local margin_kern = node.id('margin_kern')
local emit, math, character, owner, reference = nil, nil, nil, 0, 0
local layout_discretionary
local notes, graphics
local reference_stack = {}
local symbol_stack, symbol = {}, 0
local pending_diagnostics = {}
local owner_contexts = {}
local math_max = _G.math.max

function M.start(writer, bridge, decoder, discretionary_reader, note_reader, graphic_reader)
  emit, math, character = writer, bridge, decoder
  layout_discretionary = discretionary_reader
  notes, graphics = note_reader, graphic_reader
  for _, code in ipairs(pending_diagnostics) do emit('diagnostic', {code = code}) end
  pending_diagnostics = {}
end

-- The run-in title's glyphs carry runin_attr: the reader keeps them apart from the body (decision 088, amendment).
function M.runin_title(on)
  tex.setattribute(runin_attr, on and 1 or -2147483647)
end

-- \eqref formats its number through \tagform@ too: inside it, a tag macro marks nothing.
local tag_suppressed, tag_saved = 0, {}
function M.equation_tag(on)
  if on then
    tag_saved[#tag_saved + 1] = tex.getattribute(tag_attr) or -2147483647
    if tag_suppressed == 0 then tex.setattribute(tag_attr, 1) end
  else
    tex.setattribute(tag_attr, table.remove(tag_saved) or -2147483647)
  end
end
function M.equation_reference(on)
  tag_suppressed = math_max(0, tag_suppressed + (on and 1 or -1))
end

function M.reference_begin(target, selector)
  if not emit then return end
  reference = reference + 1
  reference_stack[#reference_stack + 1] = tex.getattribute(reference_attr) or -2147483647
  tex.setattribute(reference_attr, reference)
  emit('reference', {reference = reference, target = target, selector = selector})
end

-- cleveref: the labels one \cref call names, in the order its hyperlinks open.
local cref_queue = {}
function M.cref_labels(first, second)
  cref_queue = {first, second}
end
function M.cref_begin()
  M.reference_begin(table.remove(cref_queue, 1) or '', '\\@firstoffive')
end

-- hyperref's \autoref: the label it names, kept for the span its patched number opens.
local autoref_target = ''
function M.autoref_label(label)
  autoref_target = label
end
function M.autoref_begin()
  M.reference_begin(autoref_target, '\\@firstoffive')
end

function M.reference_end()
  if not emit then return end
  assert(#reference_stack > 0, 'Unbalanced reference context')
  tex.setattribute(reference_attr, table.remove(reference_stack))
end

-- relatex-proof.sty reports a \qedsymbol it does not pin; once it is wrapped as a QED mark (decision 176) that
-- definition is supported.
local qed_symbol_wrapped = false
function M.qed_symbol_wrapped() qed_symbol_wrapped = true end

function M.unsupported_qed(code)
  code = code or 'unsupported_qed_definition'
  if code == 'unsupported_qed_definition' and qed_symbol_wrapped then return end
  if emit then emit('diagnostic', {code = code})
  else pending_diagnostics[#pending_diagnostics + 1] = code end
end

function M.symbol_begin(supported)
  if not emit then return end
  symbol_stack[#symbol_stack + 1] = tex.getattribute(symbol_attr) or -2147483647
  if not supported or math.current_formula() then
    M.unsupported_qed(supported and 'unsupported_qed_context' or 'unsupported_qed_definition')
    tex.setattribute(symbol_attr, -2147483647)
    return
  end
  symbol = symbol + 1
  tex.setattribute(symbol_attr, symbol)
  emit('symbol_candidate', {symbol = symbol, name = 'qed_open_square'})
end

function M.symbol_end()
  if not emit then return end
  assert(#symbol_stack > 0, 'Unbalanced symbol context')
  tex.setattribute(symbol_attr, table.remove(symbol_stack))
end

function M.is_symbol_box(n)
  return n.id == hlist and node.has_attribute(n, symbol_attr) ~= nil
end

-- QED marks by command (decision 176): a command that typesets a proof's end mark (\qedsymbol, the command it
-- aliases, \IEEEQED, ntheorem's \proofSymbol, a mark macro a proof's end code calls) is set in text mode inside
-- one box carrying the symbol attribute, so the mark reads as one symbol whatever it draws: glyphs, an image
-- (the author's duck) or rules. Called in math mode it is left alone. A formula inside the box is the mark's
-- drawing, not a formula (relatex-external-math). The box is a QED only when it holds one small item: at most
-- three glyphs none of which is a letter or digit, an image or rules, no larger than three ems; otherwise it is
-- ordinary content again, with a diagnostic.
local qed_frames, qed_depth, qed_ids = {}, 0, {}
local math_modes = {}
for value, name in pairs(tex.getmodevalues and tex.getmodevalues() or {}) do
  if name == 'math' then math_modes[value], math_modes[-value] = true, true end
end

function M.in_qed()
  return qed_depth > 0
end

function M.qed_before()
  local wrap = emit ~= nil and qed_depth == 0 and not math_modes[tex.nest.top.mode]
  qed_frames[#qed_frames + 1] = wrap
  if wrap then tex.sprint('\\leavevmode\\directlua{ltd.flow.qed_start()}\\hbox\\bgroup') end
end

function M.qed_start()
  symbol_stack[#symbol_stack + 1] = tex.getattribute(symbol_attr) or -2147483647
  symbol = symbol + 1
  qed_depth = qed_depth + 1
  tex.setattribute(symbol_attr, symbol)
end

function M.qed_after()
  local wrapped = table.remove(qed_frames)
  assert(wrapped ~= nil, 'Unbalanced QED mark')
  if wrapped then tex.sprint('\\egroup\\directlua{ltd.flow.qed_finish()}') end
end

local function unmark_symbol(n)
  node.unset_attribute(n, symbol_attr)
  if (n.id == hlist or n.id == vlist) and n.head then
    for inner in node.traverse(n.head) do unmark_symbol(inner) end
  end
end

local qed_spacing = {}
for _, name in ipairs({'glue', 'kern', 'penalty', 'whatsit', 'math', 'dir', 'local_par', 'boundary', 'margin_kern'}) do
  local id = node.id(name)
  if id then qed_spacing[id] = true end
end

-- What the box holds: glyph count, letters or digits among them, images, rules and anything else.
local function qed_content(head, found)
  for n in node.traverse(head) do
    if n.id == glyph then
      found.glyphs = found.glyphs + 1
      local text = character(n)
      if type(text) ~= 'string' or text:find('[%w]') then found.letters = true end
    elseif n.id == rule and graphics and graphics.identity(n) then
      found.images = found.images + 1
    elseif n.id == rule then
      found.rules = found.rules + 1
    elseif (n.id == hlist or n.id == vlist) and n.head then
      if M.graphic_of(n) then found.other = true else qed_content(n.head, found) end
    elseif not qed_spacing[n.id] then
      found.other = true
    end
  end
  return found
end

-- The box is the QED symbol id when it holds one small item; otherwise it is ordinary content again.
local function qed_accept(box, id)
  local found = qed_content(box.head, {glyphs = 0, images = 0, rules = 0})
  local f = font.getfont(font.current())
  local quad = f and f.parameters and f.parameters.quad or tex.sp('10pt')
  local small = box.width <= 3 * quad and box.height + box.depth <= 3 * quad
  local one = (found.glyphs > 0 and found.glyphs <= 3 and not found.letters and found.images == 0)
    or (found.glyphs == 0 and found.images == 1) or (found.glyphs == 0 and found.images == 0 and found.rules > 0)
  if small and one and not found.other then
    qed_ids[id] = true
    emit('symbol_candidate', {symbol = id, name = 'qed'})
  else
    unmark_symbol(box)
    emit('diagnostic', {code = 'unsupported_qed_definition'})
  end
end

function M.qed_finish()
  qed_depth = qed_depth - 1
  local id = tex.getattribute(symbol_attr)
  tex.setattribute(symbol_attr, table.remove(symbol_stack))
  local box = tex.nest.top.tail
  if not (box and box.id == hlist and node.has_attribute(box, symbol_attr) == id) then
    emit('diagnostic', {code = 'unsupported_qed_definition'})
    return
  end
  qed_accept(box, id)
end

-- QED marks by position (decision 176): what a proof environment's end code typesets, between its env/X/end
-- and env/X/after hooks. A one-symbol inline formula there (\hfill$\blacksquare$, \qquad$\Box$) is the mark,
-- set in one symbol box like a mark command's.
local ending = 0
function M.proof_ending(on)
  ending = math_max(0, ending + (on and 1 or -1))
end

function M.ending_mark(head)
  if not emit or ending == 0 or qed_depth > 0 or not head or head.next then return false end
  -- A page shipped while the end code runs (a page break inside it) sets its running head in the output
  -- routine: that head's formula is not the proof's mark (papers5/1258: the "k" of a running title).
  if status.output_active then return false end
  if head.id ~= node.id('noad') or head.sub or head.sup then return false end
  local nucleus = head.nucleus
  return nucleus ~= nil and nucleus.id == node.id('math_char')
end

function M.wrap_ending_mark(head)
  symbol = symbol + 1
  local box = node.hpack(head)
  node.set_attribute(box, symbol_attr, symbol)
  qed_accept(box, symbol)
  return box
end

-- The QED box's own content as the symbol's: its glyphs' text, or its one image (decision 176). A glyph of a
-- font with one fixed layout (msam, lasy) is the character amssymb or latexsym names in that slot: msam's
-- \lozenge is named "diamond", which the text decoder reads as the suit ♦ (papers/0201: ◊).
local function qed_character(n)
  local f = font.getfont(n.font)
  local layout = f and require('relatex-math-remap').font_layout(f.name)
  local code = layout and layout[n.char]
  if code then return utf8.char(code) end
  return character(n)
end

function M.qed_item(n, item)
  local text, image = {}, nil
  local function walk(head)
    for inner in node.traverse(head) do
      if inner.id == glyph then text[#text + 1] = qed_character(inner) or ''
      elseif inner.id == rule and graphics and graphics.identity(inner) then image = graphics.identity(inner)
      elseif (inner.id == hlist or inner.id == vlist) and inner.head then walk(inner.head) end
    end
  end
  walk(n.head)
  if #text > 0 then item.text = table.concat(text) end
  if image then item.image = image end
end

-- A graphic without an inspectable source (a TikZ picture) is one item, like a symbol box:
-- its labels are part of the figure, not prose. The attribute exists once graphics load.
function M.graphic_of(n)
  if n.id ~= hlist and n.id ~= vlist then return nil end
  local attr = luatexbase.attributes['relatex.graphic.unsupported']
  return attr and node.has_attribute(n, attr) or nil
end

-- A picture contained in the current material. Material typeset inside a picture (a diagram's
-- cell formula) carries that picture's attribute itself; only a different picture is contained.
function M.contained_graphic(n)
  local id = M.graphic_of(n)
  if not id then return nil end
  return id ~= tex.getattribute(luatexbase.attributes['relatex.graphic.unsupported']) and id or nil
end

local function count_rules(head)
  local count = 0
  for n in node.traverse(head) do
    if n.id == rule then count = count + 1 end
    if n.id == hlist or n.id == vlist then count = count + count_rules(n.head) end
  end
  return count
end

local function literal_hyphen_pair(n)
  local before, replacement = n.pre, n.replace
  return before and replacement and before.id == glyph and replacement.id == glyph
    and not before.next and not replacement.next and before.char == 45
    and replacement.char == 45 and before.font == replacement.font
end

local function literal_hyphen(n)
  local before, replacement = n.pre, n.replace
  if literal_hyphen_pair(n) then
    for _, attribute in ipairs{owner_attr, slot_attr, reference_attr, fragment_attr} do
      node.set_attribute(before, attribute, node.has_attribute(replacement, attribute) or -2147483647)
    end
  end
end

-- The characters a glyph spells: a ligature glyph (ff, ffi) names the glyphs it was built from.
local function spelled(g)
  local count = 0
  for c in node.traverse_id(glyph, g.components) do count = count + spelled(c) end
  return count > 0 and count or 1
end

-- A line broken at a discretionary ships its pre/post lists instead of the replace list, so
-- text held only by the replacement (an ff/ffi ligature or an en dash spanning the break)
-- lost its slot. Give the replacement's slots, in order, to the pre-break glyphs, then to the
-- post-break glyphs; a trailing pre glyph beyond the slots is the inserted hyphen and stays
-- layout-only. A ligature glyph spells several characters ("ef-fects": one ff glyph in the
-- replacement, f- before the break and f after it): its slot serves one glyph per character,
-- so the post-break piece is owned too (papers6/0689 read "efects"); the inserted hyphen then
-- stays layout-only whatever the slots left. An unbroken line still ships the replacement itself.
local function broken_slots(n)
  local slots, echoes, ligature = {}, {}, false
  for g in node.traverse_id(glyph, n.replace) do
    local id, slot = node.has_attribute(g, owner_attr), node.has_attribute(g, slot_attr)
    if id == owner and slot then
      for copy = 1, spelled(g) do
        slots[#slots + 1] = g
        echoes[#slots] = copy > 1
      end
      ligature = ligature or spelled(g) > 1
    end
  end
  if #slots == 0 or literal_hyphen_pair(n) then return end
  local index = 1
  for _, list in ipairs({n.pre, n.post}) do
    for g in node.traverse_id(glyph, list) do
      if index > #slots then return end
      local inserted_hyphen = ligature and (g.char == 45 or g.char == 8208) and list == n.pre
      if node.has_attribute(g, owner_attr) == nil and not inserted_hyphen then
        for _, attribute in ipairs{owner_attr, slot_attr, reference_attr, fragment_attr} do
          node.set_attribute(g, attribute, node.has_attribute(slots[index], attribute) or -2147483647)
        end
        if echoes[index] then node.set_attribute(g, echo_attr, 1) end
        index = index + 1
      end
    end
  end
end

-- A line broken at a discretionary ends in the hyphen the break inserted: TeX moves the
-- pre-break list into the line before the emptied discretionary node, so the last glyph
-- (past glue, penalties, kerns and empty discretionaries; or inside a disc's own pre list)
-- is a layout-only glyph owned by no slot (paper 68: "interfer-" / "ence"). microtype's protrusion
-- puts a margin kern after it (book 0003: "triangula-" / "tion" in a table-cell minipage).
local function hyphenated_line(line)
  local last
  for n in node.traverse(line.head) do
    local empty_disc = n.id == disc and n.pre == nil and n.post == nil and n.replace == nil
    if n.id ~= glue and n.id ~= penalty and n.id ~= kern and n.id ~= margin_kern and not empty_disc then last = n end
  end
  if last and last.id == disc then last = last.pre and node.tail(last.pre) end
  return last ~= nil and last.id == glyph and layout_discretionary ~= nil and layout_discretionary(last) and
    node.has_attribute(last, owner_attr) == nil or false
end

-- A paragraph of another entity keeps its glyphs, except those a native display row has just
-- re-identified as its own formula (a display set inside a tcolorbox is typeset into the box's
-- paragraph first; the shipped row is composed later): the row's math flow reads them. That
-- includes the row's equation number, read as text by the paragraph (a display in a minipage).
local function recomposed_math(n, prior, state)
  if state.group ~= 'math' or not prior.items then return false end
  local item = prior.items[node.has_attribute(n, slot_attr)]
  local now = math.identity(n)
  return item and now and (item.type ~= 'math' or item.fragment ~= now.fragment)
end

-- The float each caption context belongs to (0 for a caption outside any float).
local caption_floats = {}
function M.caption_entity(entity, float)
  caption_floats[entity] = float
end

-- A caption set as a box (a one-line caption is an \hbox to \hsize) in a float that wrapfig
-- inserts into the next paragraph is that float's caption, not the paragraph's text (cohort 73,
-- third cohort 726). The captions of the paragraph's own float (side-by-side minipages) stay with it.
local function foreign_caption(prior, state)
  local float = caption_floats[prior.entity]
  return state.group == 'paragraph' and float ~= nil and float ~= state.entity
end

local function foreign_paragraph(n, state)
  local prior = owner_contexts[node.has_attribute(n, owner_attr)]
  if not prior or prior.entity == state.entity then return false end
  return (prior.group == 'paragraph' or foreign_caption(prior, state)) and not recomposed_math(n, prior, state)
end

local function paragraph_boundary(n, state)
  if state.group ~= 'paragraph' then return end
  local previous = node.has_attribute(n, owner_attr)
  local context = owner_contexts[previous]
  if not context or context.group ~= 'paragraph' then return end
  if state.paragraph and state.paragraph ~= previous and #state.items > 0 then
    state.space = ' '
    emit('diagnostic', {code='flow_paragraph_boundary', source='observed-paragraph-owners',
      owner=owner, previous_owner=state.paragraph, next_owner=previous})
  end
  state.paragraph = previous
end

local function prior_space(n, item)
  local context = owner_contexts[node.has_attribute(n, owner_attr)]
  if not context or context.group ~= 'paragraph' or not context.items then return end
  local original = context.items[node.has_attribute(n, slot_attr)]
  if original and original.type == item.type and original.text == item.text
      and original.table_cell == item.table_cell and original.space == ' ' then return ' ' end
end

-- A rule with a size running to its box (\vrule in a line, an \hrule across a vbox) is
-- read with the text; its size is known at shipout. Formula and table rules are not,
-- nor the sides of a frame around another paragraph: they belong to no line of it.
local running_size = -1073741824
local function running_rule(n)
  if n.id ~= rule or (n.width <= 0 and n.width ~= running_size) then return false end
  local runs_vertically = n.height == running_size or n.depth == running_size
  if n.width ~= running_size and not runs_vertically then return false end
  -- An \hrule of no height and depth paints nothing.
  if not runs_vertically and n.height + n.depth <= 0 then return false end
  for _, name in ipairs({'relatex.math.formula', 'relatex.table'}) do
    local attribute = luatexbase.attributes[name]
    if attribute and node.has_attribute(n, attribute) then return false end
  end
  return true
end

local function holds_foreign_paragraph(head, state)
  for n in node.traverse(head) do
    if n.id == glyph and foreign_paragraph(n, state) then return true end
    if (n.id == hlist or n.id == vlist) and holds_foreign_paragraph(n.head, state) then return true end
  end
  return false
end

local function frames_paragraph(head, state)
  for n in node.traverse_id(vlist, head) do
    if holds_foreign_paragraph(n.head, state) then return true end
  end
  return false
end

-- \accent sets its accent between two accent kerns that cancel its width (or in a raised box
-- between them): no advance of its own. Its combining mark follows the letter it is set on.
local accentkern
local function accent_set(n)
  if accentkern == nil then
    accentkern = false
    for id, name in pairs(node.subtypes('kern')) do if name == 'accentkern' then accentkern = id end end
  end
  local function accent_kern(k) return k and k.id == node.id('kern') and k.subtype == accentkern and k.kern end
  local after = accent_kern(n.next)
  if not after then return false end
  local net = (accent_kern(n.prev) or 0) + n.width + after
  return net >= -1 and net <= 1
end

local function accented_text(n, state)
  local text = character(n)
  local mark = (state.accent_box or accent_set(n)) and require('relatex-text-symbols').accent(n, text)
  if mark then
    state.accent = mark
    return ''
  end
  local accent = state.accent
  state.accent = nil
  if not accent or not text then return text end
  -- The dotless letters carry an accent in place of their dot: Unicode spells í as i + U+0301.
  return (text == 'ı' and 'i' or text == 'ȷ' and 'j' or text) .. accent
end

-- Packing a box converts glue without stretch into a kern: \enspace, \quad and an explicit
-- \hskip in a heading's number or a caption's label arrive here as user kerns. A kern as wide
-- as the font's interword space is a word gap; a thinner one (\thinspace, \;) or a font kern
-- is not.
-- A glue narrower than this fraction of the font's size is no word space.
local HAIRLINE_EM = 0.1
local function hairline(state)
  local parameters = state.font and font.getfont(state.font)
  return parameters and (parameters.size or 0) * HAIRLINE_EM or 0
end

-- The font a word space between two glyphs of one font was set in, when it is another one
-- (\texttt{repeat} \texttt{repeat}: the roman space between two typewriter words, papers4/0919;
-- decided 2026-11-09): the glyphs' font does not set the glue's natural width (its interword space,
-- or that plus its extra space after a sentence) and one declared text font does. Nil otherwise.
local function sets_space(id, width)
  local f = id and font.getfont(id)
  local parameters = f and f.parameters
  local space = parameters and parameters.space
  -- An OpenType font's parameters are scaled reals (218234.88sp); its glue rounds them.
  local function near(value) return width - value < 1 and value - width < 1 end
  return space ~= nil and space > 0 and (near(space) or near(space + (parameters.extra_space or 0)))
end
local function foreign_space_style(space, glyph_font)
  if not space or space.font ~= glyph_font or not M.text_styles or not M.text_styles.space_font then return end
  if sets_space(glyph_font, space.width) or not M.text_styles.identity_of(glyph_font) then return end
  return M.text_styles.space_font(function(id) return sets_space(id, space.width) end)
end

local userkern
local function word_gap(n, state)
  if n.id ~= kern or not state.font then return false end
  if userkern == nil then
    userkern = false
    for id, name in pairs(node.subtypes('kern')) do if name == 'userkern' then userkern = id end end
  end
  if not userkern or n.subtype ~= userkern or n.kern <= 0 then return false end
  local parameters = font.getfont(state.font)
  local space = parameters and parameters.parameters.space or 0
  return space > 0 and n.kern >= space
end

-- A glue of no natural width takes the width its box sets it to: the \hfil after a table of
-- contents entry's number (\numberline) and the \hss before its page number are packed to as
-- wide a gap as the surrounding text shows. Where that set width (node.effective_glue in the
-- parent box) reaches the font's interword space, the glue is a word space wherever its
-- natural width is zero. A formula's own boxes (a matrix's cells) space math, not reading.
-- TeX rounds a box's glue ratio, so a \hfil packed to the interword space comes out a few
-- thousandths of a point short of it (paper 229: 294126sp for a 294912sp space, 0.3%): a set
-- width within SET_TOLERANCE of the space is that space.
-- The interword glue itself shrinks to its space less its shrink, so a set width that reaches
-- that narrowest word space is one too (a two-digit chapter number fills more of \numberline's
-- box: amsldoc's "10 Error messages" leaves 0.35em).
local SET_TOLERANCE = 0.01
local function set_word_gap(n, state, box)
  -- Only a horizontal box sets a gap between words: a vertical box's \vss (the raised A of the
  -- \LaTeX logo) is set to a height.
  if not box or box.id ~= hlist or state.group == 'math' or not state.font then return false end
  local parameters = font.getfont(state.font)
  local space = parameters and parameters.parameters.space or 0
  local narrowest = space - (parameters and parameters.parameters.space_shrink or 0)
  return space > 0 and node.effective_glue(n, box) >= narrowest * (1 - SET_TOLERANCE)
end

-- soul's \hl, \ul and \st set each interword space as rule leaders over the font's own interword
-- glue (the highlight or line painted across the gap): that gap is a word space ("Asymmetric
-- encryption", slides/0018). A rule fill (\hrulefill: no width) or a fixed drawn line (no stretch
-- or shrink, or one far from the font's space) is no word.
local function leader_word_space(n, state)
  if n.id ~= glue or not n.leader or n.leader.id ~= rule or not state.font then return false end
  if n.stretch_order ~= 0 or n.shrink_order ~= 0 or (n.stretch == 0 and n.shrink == 0) then return false end
  local parameters = font.getfont(state.font)
  local space = parameters and parameters.parameters.space or 0
  return space > 0 and n.width >= space / 2 and n.width <= space * 2
end

-- A backspace as wide as a word gap (\hskip -1em after a heading number's \quad, cvpr) takes
-- the gap back: the words on both sides touch, so no logical space separates them.
local function word_backspace(n, state)
  -- A text-only picture's kerns place its text; they take back no word space.
  if not state.font or state.space ~= ' ' or state.positioned then return false end
  local width
  if n.id == kern and userkern and n.subtype == userkern then width = n.kern end
  if n.id == glue and (n.subtype == 0 or n.subtype == 13) then width = n.width end
  if not width or width >= 0 then return false end
  local parameters = font.getfont(state.font)
  local space = parameters and parameters.parameters.space or 0
  return space > 0 and -width >= space
end

-- The rows of an overlay (\ooalign: \baselineskip 0pt and \lineskiplimit -\maxdimen, the circle
-- of \textcircled over its digit) share one baseline: the interline glue after a row takes back
-- its depth and the next row's height, so the next row starts no lower. Such a row ends no word.
local function overlaid_row(n)
  local advance = n.depth or 0
  local following = n.next
  while following and following.id ~= hlist and following.id ~= vlist do
    if following.id == glue or following.id == kern then advance = advance + (following.width or following.kern or 0) end
    following = following.next
  end
  -- Without measured heights (a box the unit harness builds) nothing overlays.
  if not following or n.depth == nil or following.height == nil then return false end
  return advance + following.height <= 0
end

-- The width a row sets before its first glyph, rule or formula: the left margin and the tab fields
-- before it (tabbing's \+ and \=). Boxes holding text are entered, empty boxes count their width.
local function leading_width(list)
  local width = 0
  for n in node.traverse(list) do
    if n.id == glyph or n.id == rule or n.id == math_node then return width, true end
    if n.id == glue or n.id == kern then
      width = width + (n.width or n.kern or 0)
    elseif (n.id == hlist or n.id == vlist) and n.head then
      local inner, found = leading_width(n.head)
      if found then return width + inner, true end
      width = width + (n.width or 0)
    end
  end
  return width, false
end

-- hlist subtype of a paragraph's broken line: those wrap. A row of a box (tabbing's rows, a box's rows set one
-- under the other) has another subtype and is a line of its own.
local line_subtype = 1

-- The first glyph of a list that an owner has read, with its owner and slot.
local function first_read(list)
  for n in node.traverse(list) do
    if n.id == glyph or n.id == rule then
      local who, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
      if who and slot then return who, slot end
    elseif (n.id == hlist or n.id == vlist) and n.head then
      local who, slot = first_read(n.head)
      if who then return who, slot end
    end
  end
  return nil
end

-- Whether a row of the vertical list comes before the row.
local function row_before(n)
  local m = n.prev
  while m do
    if m.id == hlist then return true end
    m = m.prev
  end
  return false
end

-- Whether another line (an hlist) follows the line in its vertical list.
local function following_line(head, n)
  local seen = false
  for m in node.traverse(head) do
    if seen and m.id == hlist then return true end
    if m == n then seen = true end
  end
  return false
end

-- A line broken at a literal hyphen ships the discretionary's pre-break '-', which carries the slot of
-- the replacement '-' it stands for (literal_hyphen); the hyphen a break inserts has none. A capture that
-- reads those broken lines again (overwrite: beamer's title page boxes the \date text twice) reads that
-- hyphen as its text, or it keeps the first capture's owner and parts the paragraph around a lone hyphen
-- (slides 0019, papers4 1538). Only the hyphen: a ligature's pre-break piece (broken_slots) stands for
-- the whole ligature ("ff" broken as f- f), and reading it alone would drop a letter (papers5/1480).
local function carried_text(n, overwrite)
  local slot = overwrite and n.char == 45 and node.has_attribute(n, slot_attr)
  return slot ~= nil and slot ~= false and slot > 0
end

-- relatex.decoration: 1 on a text underline's glyphs, 2 on its rule (M.text_underline; before LaTeX 2025-11,
-- relatex-external-math's math-mode underline), 3 on struck-out glyphs (soul's \st; relatex-external.sty).
-- soul's \ul marks its words 1; soul draws its lines as leaders, read as word spaces (leader_word_space).
local text_decorations = {[1] = 'underline', [3] = 'strike'}

local function decoration(n)
  local attribute = luatexbase.attributes['relatex.decoration']
  return attribute and node.has_attribute(n, attribute) or nil
end

local function decoration_rule(n)
  return n.id == rule and decoration(n) == 2
end

-- Text-mode \underline{X} (the kernel's \underline@text since LaTeX 2025-11: X in an \hbox over an
-- \hrule, no formula; relatex-external.sty wraps it): its rule is set with relatex.decoration = 2,
-- X with 1, so X's glyphs read as underlined text and the rule as their decoration.
function M.text_underline(value)
  local attribute = luatexbase.attributes['relatex.decoration'] or luatexbase.new_attribute('relatex.decoration')
  tex.setattribute(attribute, value)
end

-- After the underline box is appended: an underline over no text (\underline{\hspace{3cm}}, a blank
-- to fill in, or only a formula) draws a rule, not a decoration, so its rule keeps no mark.
function M.text_underline_end()
  local box = tex.nest.top.tail
  local attribute = luatexbase.attributes['relatex.decoration']
  if not (box and attribute and (box.id == hlist or box.id == vlist)) then return end
  local function text_glyph(list)
    for n in node.traverse(list) do
      if n.id == glyph and not (math and math.identity(n)) then return true end
      if (n.id == hlist or n.id == vlist) and n.head and text_glyph(n.head) then return true end
    end
    return false
  end
  if text_glyph(box.head) then return end
  local function unmark(list)
    for n in node.traverse(list) do
      if n.id == rule and node.has_attribute(n, attribute) == 2 then node.unset_attribute(n, attribute) end
      if (n.id == hlist or n.id == vlist) and n.head then unmark(n.head) end
    end
  end
  unmark(box.head)
end

-- Whether a discretionary is a formula's text: the glyph after it (the "cycle" of \text{$2\ell$-cycle}) is the
-- formula's; after an inline formula in a line of text ($\ell$-dependent) it is plain text.
local function math_text(n)
  if not (math and math.identity) then return false end
  for after in node.traverse_id(glyph, n.next) do return math.identity(after) ~= nil end
  return false
end

local form_rules = {}
for key, name in pairs(node.subtypes('rule')) do
  if name == 'box' or name == 'image' then form_rules[key] = true end
end

local function item_marker(n)
  local marker = luatexbase.attributes['relatex.item_marker']
  return marker ~= nil and node.has_attribute(n, marker) ~= nil
end

-- A formula an enclosing box reads again (a one-line caption is an \hbox around the formulas already
-- captured) keeps the items of its own flow, which know no space before it: the word space ahead of
-- its first glyph is recorded as a diagnostic for that item (the reading adds it) and consumed here,
-- so it neither goes missing before the formula nor lands on the text after it ("of $d$ and $\alpha$.").
-- The diagnostic is recorded even for an empty space: it tells the reading the formula sits inside that
-- box, which a formula ending the box ("inside $S_j$") shows nowhere else.
local recorded_math_space = {}
local function space_before_captured_math(n, state)
  local owner_id, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
  local context = owner_contexts[owner_id]
  local first = context and context.items and slot and context.items[slot]
  if not (first and first.type == 'math' and first.position == 1) then return end
  local key = owner_id .. ':' .. slot
  if not recorded_math_space[key] or state.space ~= '' then
    recorded_math_space[key] = true
    emit('diagnostic', {code = 'math_space', owner = owner_id, slot = slot, space = state.space})
  end
  state.space, state.space_glue = '', nil
end

local function collect(head, state, overwrite, box)
  for n in node.traverse(head) do
    if n.id == glyph then state.font = n.font end
    if foreign_paragraph(n, state) then
      -- Repacking an independent paragraph must preserve its existing owner.
    elseif M.graphic_of(n) then
      local item = {type = 'graphic', graphic = M.graphic_of(n), space = state.space,
        width = n.width, height = n.height, depth = n.depth}
      -- A picture in a rich table cell belongs to that cell (third cohort 593, 776).
      if M.rich_tables then
        item.table_cell = M.rich_tables.identity(n)
        item.table = M.rich_tables.table_identity(n)
      end
      -- A picture inside \href (\orcidlink's logo) names its link: an ORCID logo becomes the link.
      if M.external_links then M.external_links.identify(n, item) end
      state.space, state.gaps = '', 0
      state.items[#state.items + 1] = item
      node.set_attribute(n, owner_attr, owner)
      node.set_attribute(n, slot_attr, #state.items)
    elseif M.is_symbol_box(n) then
      local item = {type = 'symbol', symbol = node.has_attribute(n, symbol_attr), space = state.space,
        width = n.width, height = n.height, depth = n.depth, rules = count_rules(n.head)}
      if qed_ids[item.symbol] then M.qed_item(n, item) end
      if notes then notes.identify(n, item) end
      state.space, state.gaps = '', 0
      state.items[#state.items + 1] = item
      node.set_attribute(n, owner_attr, owner)
      node.set_attribute(n, slot_attr, #state.items)
    elseif decoration_rule(n) then
      -- The rule of a text underline is its decoration, not text (relatex-external-math, text_underline).
    elseif n.id == rule and form_rules[n.subtype] and not (graphics and graphics.identity(n)) and not item_marker(n) then
      -- A form or image no graphic claims paints no rule: a decoration picture's shading (the rounded theme's
      -- table of contents ball, relatex-beamer; slides/0029) is a reused XObject, not a filled rectangle. A beamer
      -- item's ball is such a form too, and stays the item's marker (decision 127; slides/0029, 0031).
    elseif n.id == glyph or (n.id == rule and n.width ~= 0 and n.height + n.depth > 0) or (running_rule(n) and not frames_paragraph(head, state)) then
      local item = (graphics and graphics.identity(n)) or math.identity(n)
      local changed_math = item and item.fragment ~= node.has_attribute(n, fragment_attr)
      if (not layout_discretionary(n) or carried_text(n, overwrite))
          and (overwrite or changed_math or not node.has_attribute(n, owner_attr)) then
        paragraph_boundary(n, state)
        if item and item.type == 'image' then
          -- Source use and native resource identity were bound before packing.
        elseif item then
          item.type = 'math'
          node.set_attribute(n, fragment_attr, item.fragment)
        elseif n.id == glyph then
          local text = accented_text(n, state)
          item = {type = 'text', text = text or '[unmapped]'}
          -- A known symbol font's glyph with no character keeps its name (marvosym's Tumbler),
          -- which a screen reader speaks in place of the character.
          if not text then
            local _, evidence = character(n)
            if type(evidence) == 'table' and type(evidence.name) == 'string' then item.glyph_name = evidence.name end
          end
          if M.text_styles then
            item.style = M.text_styles.identity(n)
            -- A direct TeX font has no NFSS style; name it so a run in another face (a
            -- small-caps word inside a heading) keeps its own style (paper 0593).
            if not item.style and n.font and n.font > 0 then item.font = n.font end
          end
          item.decoration = text_decorations[decoration(n)]
        else
          item = {type = 'unsupported', text = '[rule]', width = n.width, height = n.height, depth = n.depth}
          -- A beamer item marker drawn as this rule (relatex-beamer, decision 127).
          local marker = luatexbase.attributes['relatex.item_marker']
          local shape = marker and node.has_attribute(n, marker)
          if shape then item.item_marker = shape == 1 and 'square' or 'disc' end
        end
        if node.has_attribute(n, runin_attr) then item.runin_title = true end
        -- Images and graphics carry no tag mark (the schema has none; an image in a tag, books/0003).
        if node.has_attribute(n, tag_attr) == 1 and item.type ~= 'image' and item.type ~= 'graphic' then
          item.tag = true
          -- A display's number is text in the font \maketag@@@ selects (\normalfont at the size
          -- around): its glyph names that text style, the style of the number's span (reading).
          if item.type == 'math' and n.id == glyph and M.text_styles and M.text_styles.identity_of(n.font) then
            item.tag_style = n.font
          end
        end
        local ref = node.has_attribute(n, reference_attr)
        if ref then
          item.reference = ref
          -- A reference printed inside a formula (\eqref in its \text{}) has no flow text:
          -- its glyphs carry the printed number so the reader can record the use.
          if item.type == 'math' then
            local printed = math.printed_character(n) or character(n)
            if printed then item.text = printed end
          end
        end
        if M.citations then M.citations.identify(n, item) end
        if M.external_links then M.external_links.identify(n, item) end
        if notes then notes.identify(n, item) end
        if M.rich_tables then
          item.table_cell = M.rich_tables.identity(n)
          item.table = M.rich_tables.table_identity(n)
        end
        -- A containing paragraph may repack the already broken lines of a
        -- p/m/b cell in the same entity. Its discarded break glue is still
        -- known by the original scalar slot; preserve that logical space.
        item.space = prior_space(n, item) or state.space
        if item.type == 'text' and item.space == ' ' and n.id == glyph then
          item.space_style = foreign_space_style(state.space_glue, n.font)
        end
        state.space, state.space_glue, state.gaps = '', nil, 0
        state.items[#state.items + 1] = item
        node.set_attribute(n, owner_attr, owner)
        node.set_attribute(n, slot_attr, #state.items)
      elseif math and math.identity(n) then
        space_before_captured_math(n, state)
      end
    -- A box leader set once (acmart's affiliation comma, \cleaders\copy\@ACM@commabox) is that box's
    -- text; a fill that repeats its box (\dotfill) stays a gap (relatex-leaders).
    elseif n.id == glue and n.leader and require('relatex-leaders').single_copy(n, box) then
      collect(n.leader, state, overwrite, box)
    -- Glue wider than the hairline is a word space: a plain interword one, and zero-width
    -- fill (\numberline's \hfil, the \hss before a page number) where its box sets it as wide
    -- as the font's interword space. Zero-width glue below that is a break opportunity, not a
    -- word space (babel puts it between CJK characters with the spaceskip subtype); neither
    -- is a hairline (an author mark's gap).
    elseif n.id == glue and (n.subtype == 13 or n.subtype == 0)
        and (n.width > hairline(state) or set_word_gap(n, state, box)) then
      state.space, state.gaps = ' ', (state.gaps or 0) + 1
      state.space_glue = {width = n.width, font = state.font}
    elseif leader_word_space(n, state) then
      state.space = ' '
      state.space_glue = {width = n.width, font = state.font}
    -- \newline and \\ end a line with \hfil\break: stretch but no width, so the words on both
    -- sides would read as one.
    elseif n.id == glue and n.next and n.next.id == penalty and n.next.penalty <= -10000 then
      state.space = ' '
    -- A forced break with no glue before it (amsldoc's "often to the\break complex") still ends
    -- the paragraph's line: the words on the two lines are two words.
    elseif n.id == penalty and n.penalty <= -10000 and state.group == 'paragraph' and not box then
      state.space = ' '
    elseif word_gap(n, state) then
      state.space, state.gaps = ' ', (state.gaps or 0) + 1
    -- A backspace takes back one word gap: where two gaps stand before it (acmsmall's theorem head: the
    -- label's \enspace, then the \labelsep the item adds, which the head's \hskip-\labelsep cancels) the
    -- other still separates the head from its body ("LEMMA 3.2. Let").
    elseif word_backspace(n, state) then
      state.gaps = (state.gaps or 1) - 1
      if state.gaps <= 0 then state.space = '' end
    elseif n.id == hlist or n.id == vlist then
      -- A raised accent is the one glyph of a box set between its accent kerns.
      state.accent_box = n.id == hlist and n.head ~= nil and n.head.next == nil and n.head.id == glyph and accent_set(n)
      local vertical_line = state.in_vlist and n.id == hlist
      local outer, positioned = state.in_vlist, state.positioned
      state.in_vlist = n.id == vlist
      state.positioned = positioned or (graphics and graphics.limits and graphics.limits.positioned(n))
      collect(n.head, state, overwrite, n)
      state.in_vlist, state.positioned = outer, positioned
      -- A row of a box after another row starts a line: the item that opens it is recorded with the width the
      -- row sets before its text (the tab indent), so the reading ends the line before it.
      if vertical_line and n.subtype ~= line_subtype and row_before(n) then
        local who, slot = first_read(n.head)
        if who then emit('flow_row', {owner = who, slot = slot, indent = (leading_width(n.head))}) end
      end
      state.accent_box = false
      -- A line of a vertical list (an \IEEEauthorblockA row, a broken line of a box) ends a word
      -- when another line follows it: a forced break (\\) leaves no glue to say so, unless the
      -- line ends in a hyphen: a literal one, or the layout-only hyphen a discretionary break inserted
      -- (its word goes on). A box of one line (the raised A of the \LaTeX logo) breaks nothing, nor
      -- does a row the next row overlays (\ooalign).
      if vertical_line and following_line(head, n) and not overlaid_row(n) then
        local last = state.items[#state.items]
        if last and not (last.text and last.text:sub(-1) == '-') and not hyphenated_line(n) then state.space = ' ' end
      end
    elseif n.id == disc then
      -- A hyphen in a formula's text (\text{$2\ell$-cycle}) is a discretionary beside the formula's glyphs; its
      -- glyph has no fragment (relatex-math marks no discretionary) and keeps the owner the formula's capture
      -- gave it: read again (a tcolorbox repacks its lines) it made a paragraph "-" between the rows of an
      -- align* (papers2/0236).
      collect(n.replace, state, overwrite and not math_text(n), box)
      literal_hyphen(n)
      broken_slots(n)
    end
  end
end

-- RELATEX_LEAN (see relatex.lua): the text flow is read for the document, not for the SyncTeX.
local LEAN = os.getenv('RELATEX_LEAN') ~= nil

function M.capture(head, group, entity, role)
  if LEAN or not emit then return end
  owner = owner + 1
  owner_contexts[owner] = {group = group or 'box', entity = entity}
  if M.text_styles then M.text_styles.new_owner() end
  local state = {items = {}, space = '', entity = entity, group = group or 'box'}
  collect(head, state, group == 'paragraph' or group == 'math')
  owner_contexts[owner].items = state.items
  if #state.items > 0 then
    emit('flow_candidate', {owner = owner, group = group or 'box', entity = entity, role = role, items = state.items})
  end
end

-- A text underline list (its glyphs marked 1 before conversion, relatex-external-math): its rules are
-- the decoration (2). This runs before relatex.external.math.end, so the marks are set here, before
-- this list or its paragraph is captured.
local function mark_underline_rules(head)
  local attribute = luatexbase.attributes['relatex.decoration']
  if not attribute then return end
  -- Only a list whose every glyph is underlined text: a formula holding a nested underline keeps its rules.
  local marked, unmarked = 0, 0
  local function walk(list, mark)
    for n in node.traverse(list) do
      if n.id == glyph then
        if node.has_attribute(n, attribute) == 1 then marked = marked + 1 else unmarked = unmarked + 1 end
      end
      if mark and n.id == rule then node.set_attribute(n, attribute, 2) end
      if (n.id == hlist or n.id == vlist) and n.head then walk(n.head, mark) end
    end
  end
  walk(head, false)
  if marked > 0 and unmarked == 0 then walk(head, true) end
end

luatexbase.add_to_callback('post_mlist_to_hlist_filter', function(head)
  if not emit then return head end
  local entity, role = math.context()
  mark_underline_rules(head)
  M.capture(head, 'math', entity, role)
  return head
end, 'relatex.flow.math')
-- This callback is a reverselist: declared before means executed after.
luatexbase.declare_callback_rule('post_mlist_to_hlist_filter',
  'relatex.flow.math', 'before', 'relatex.math.fragments')

-- The page color stack (\pdfextension colorstack 0), followed in paint order across
-- pages, gives a rule its fill color. Black is the default and is not recorded.
local colorstack = node.subtype('pdf_colorstack')
local whatsit = node.id('whatsit')
local colors = {'0 g 0 G'}
local math_min, math_max = _G.math.min, _G.math.max
local function round(value) return _G.math.floor(value + 0.5) end

-- Follows one node of the walk on stack: a page color stack whatsit sets, pushes or pops.
function M.color_step(stack, n)
  if n.id ~= whatsit or n.subtype ~= colorstack or n.stack ~= 0 then return end
  if n.command == 0 then stack[#stack] = n.data
  elseif n.command == 1 then stack[#stack + 1] = n.data
  elseif n.command == 2 and #stack > 1 then stack[#stack] = nil end
end

-- The page's scale (\scalebox, \resizebox: graphicx saves the graphics state, sets a matrix and
-- restores it), followed in paint order: a rule painted inside paints at that scale. A matrix that
-- rotates or skews leaves the scale as it was.
local pdf_save, pdf_restore = node.subtype('pdf_save'), node.subtype('pdf_restore')
local pdf_setmatrix = node.subtype('pdf_setmatrix')
local scales = {{1, 1}}
function M.scale_step(stack, n)
  if n.id ~= whatsit then return end
  if n.subtype == pdf_save then stack[#stack + 1] = {stack[#stack][1], stack[#stack][2]}
  elseif n.subtype == pdf_restore and #stack > 1 then stack[#stack] = nil
  elseif n.subtype == pdf_setmatrix then
    local a, b, c, d = (n.data or ''):match('^%s*(%S+)%s+(%S+)%s+(%S+)%s+(%S+)%s*$')
    a, b, c, d = tonumber(a), tonumber(b), tonumber(c), tonumber(d)
    if a and d and b == 0 and c == 0 then stack[#stack] = {stack[#stack][1] * a, stack[#stack][2] * d} end
  end
end

function M.paint(n)
  M.color_step(colors, n)
  M.scale_step(scales, n)
end

-- A copy of the page color stack as the shipout walk left it: before a page's walk, the stack
-- at its start (the table capture follows its own copy through the page).
function M.color_stack()
  local copy = {}
  for index, data in ipairs(colors) do copy[index] = data end
  return copy
end

-- The fill color a color stack entry sets, as #rrggbb (black included), or nil when it sets
-- no gray, RGB or CMYK fill.
function M.color_hex(data)
  local words, values = {}, {}
  for word in (data or ''):gmatch('%S+') do words[#words + 1] = word end
  for i, word in ipairs(words) do
    local count = ({g = 1, rg = 3, k = 4})[word]
    if count and i > count then
      for j = 1, count do values[j] = tonumber(words[i - count + j - 1]) or 0 end
      values.n = count
    end
  end
  local rgb
  if values.n == 1 then rgb = {values[1], values[1], values[1]}
  elseif values.n == 3 then rgb = {values[1], values[2], values[3]}
  elseif values.n == 4 then
    rgb = {}
    for j = 1, 3 do rgb[j] = (1 - values[j]) * (1 - values[4]) end
  else return nil end
  -- (math is the formula module here.)
  local function byte(value) return (math_min(math_max(value, 0), 1) * 255 + 0.5) // 1 end
  return string.format('#%02x%02x%02x', byte(rgb[1]), byte(rgb[2]), byte(rgb[3]))
end

local function fill_color(data)
  local hex = M.color_hex(data)
  if hex ~= '#000000' then return hex end
end

-- The boxes enclosing the node being walked at shipout, outermost first.
local boxes = {}
function M.enter(box) boxes[#boxes + 1] = box end
function M.leave() boxes[#boxes] = nil end
function M.reset_boxes()
  boxes = {}
  scales = {{1, 1}}
end

local function offset_in(container, child)
  if container.id == vlist then return node.has_field(child, 'shift') and child.shift or 0 end
  return child == container.head and 0 or node.rangedimensions(container, container.head, child)
end

-- A rule inside a box narrower than itself (\rlap, \llap: a \colorbox or listing
-- background) is painted under what follows; its margins place it as in that box.
local function overlay_margins(n)
  if n.width <= 0 then return nil end
  for i = #boxes, 1, -1 do
    local outer = boxes[i]
    if outer.id == hlist and outer.width < n.width then
      local x, child = 0, n
      for j = #boxes, i, -1 do
        x = x + offset_in(boxes[j], child)
        child = boxes[j]
      end
      return x, outer.width - x - n.width
    end
  end
end

-- A collected glyph painted in a fill other than black records that color at shipout, by its
-- flow slot (the text counterpart of rule_paint). Black is the default and is not recorded.
function M.audit_text_color(n, page)
  if n.id ~= glyph then return end
  local id, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
  if not id or not slot then return end
  local color = fill_color(colors[#colors])
  if color then emit('diagnostic', {code = 'text_paint', page = page, owner = id, slot = slot, color = color}) end
end

-- Document language (decision 080, plan A1). Every babel or polyglossia language switch sets this
-- attribute to the language's number (\languagename, numbered in order of first use); the
-- document's language is the one current at the end of \begin{document}. A collected glyph set
-- in another language records that language's name at shipout by its flow slot; the product maps
-- names to BCP 47 tags, so this side keeps no language table.
local language_attr = luatexbase.new_attribute('relatex.language')
local language_ids, language_names, main_language = {}, {}, nil

function M.language_sync(name)
  if name == '' then return end
  local id = language_ids[name]
  if not id then
    id = #language_names + 1
    language_ids[name], language_names[id] = id, name
  end
  tex.setattribute(language_attr, id)
end

-- source: babel, polyglossia or none (neither package; the class options may still name one).
-- metadata: \DocumentMetadata's lang (the tagging project's document/lang property), or ''.
function M.language_main(name, source, options, metadata)
  if not emit then return end
  if source ~= 'none' then
    M.language_sync(name)
    main_language = language_ids[name]
  end
  emit('diagnostic', {code = 'document_language', source = source,
    language = source ~= 'none' and name or nil, class_options = options,
    metadata_language = metadata ~= '' and metadata or nil})
end

function M.audit_text_language(n, page)
  if n.id ~= glyph or not main_language then return end
  local language = node.has_attribute(n, language_attr)
  if not language or language == main_language then return end
  local id, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
  if not id or not slot then return end
  emit('diagnostic', {code = 'text_language', page = page, owner = id, slot = slot, language = language_names[language]})
end

-- A collected rule records its running size (its box's), its fill color and its overlay
-- margins at shipout.
-- Uncollected running vertical rules inherit their visible height/depth from their box;
-- they have no ordinary positive-size flow slot: never silently certify them.
function M.audit_running_rule(n, parent, page)
  local running = -1073741824
  local id, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
  if n.id == rule and parent and id and slot then
    local color = fill_color(colors[#colors])
    local left, right = overlay_margins(n)
    local x, y = scales[#scales][1], scales[#scales][2]
    if running_rule(n) or color or left or x ~= 1 or y ~= 1 then
      local function size(value, box, factor) return round((value == running and box or value) * factor) end
      local function scaled(value) return value and round(value * x) end
      emit('diagnostic', {code = 'rule_paint', page = page, owner = id, slot = slot, color = color,
        margin_left = scaled(left), margin_right = scaled(right), width = size(n.width, parent.width, x),
        height = size(n.height, parent.height, y), depth = size(n.depth, parent.depth, y)})
    end
    if running_rule(n) then return end
  end
  if n.id ~= rule or not parent or n.width <= 0
      or (n.height ~= running and n.depth ~= running) then return end
  local formula_attribute = luatexbase.attributes['relatex.math.formula']
  if formula_attribute and node.has_attribute(n, formula_attribute) then return end
  local height = n.height == running and parent.height or n.height
  local depth = n.depth == running and parent.depth or n.depth
  if height + depth <= 0 then return end
  emit('diagnostic', {code = 'unclassified_running_rule', page = page,
    width = n.width, height = n.height, depth = n.depth,
    box_height = parent.height, box_depth = parent.depth, box_kind = node.type(parent.id),
    table = luatexbase.attributes['relatex.table'] and node.has_attribute(n, luatexbase.attributes['relatex.table'])})
end

function M.retained(n, runs)
  local visible_rule = n.id == rule and n.width ~= 0 and n.height + n.depth > 0 or running_rule(n)
  if n.id ~= glyph and not visible_rule and not M.is_symbol_box(n) and not M.graphic_of(n) then return false end
  local id = node.has_attribute(n, owner_attr)
  if not id then return false end
  local slot = node.has_attribute(n, slot_attr)
  -- The second half of a split ligature: its slot is read with the first half.
  if node.has_attribute(n, echo_attr) then return true end
  local last = runs[#runs]
  if last and last.owner == id and last.last + 1 == slot then
    last.last = slot
  else
    runs[#runs + 1] = {owner = id, first = slot, last = slot}
  end
  return true
end

-- True on the post-break half of a split ligature: it repeats its first half's slot, so a consumer that
-- counts slots (citation and bibliography retention) must not count it twice.
function M.is_echo(n)
  return node.has_attribute(n, echo_attr) ~= nil
end

function M.scalar_identity(n)
  local id, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
  if id and slot and owner_contexts[id] and slot > 0 then return id, slot end
end

function M.finish()
  assert(#reference_stack == 0, 'Unclosed reference context')
  assert(#symbol_stack == 0, 'Unclosed symbol context')
  emit = nil
end

return M
