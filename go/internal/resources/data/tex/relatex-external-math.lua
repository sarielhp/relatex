-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Observe ordinary math conversion; no tagging writer or TeX math-list rewrite.
local M = {}
local converter = require('luamml-convert')
local write_xml = require('luamml-xmlwriter')
local mappings = require('relatex-legacy-mappings')
local symbols = require('relatex-external-symbols')
local arrays = require('relatex-external-arrays')
local boxes = require('relatex-external-boxes')
local alphabets = require('relatex-external-alphabets')
local arrows = require('relatex-math-arrows')
local remap = require('relatex-math-remap')
local observation = require('relatex-external-convert')
local primes = require('relatex-prime-ligatures')
local families, frames, total = {}, {}, 0
-- LuaMML's \text converter (luamml-lr) reads an inner formula's MathML from its start-math
-- node; without it, the inner formula's end deletes the text collected before it.
local properties = node.get_properties_table()
local supplemental, applied, declarations = {}, {}, {}
local core
local styles = {display=0, crampeddisplay=1, text=2, crampedtext=3,
  script=4, crampedscript=5, scriptscript=6, crampedscriptscript=7}
local math_mode
for id, name in pairs(tex.getmodevalues()) do if name == 'math' then math_mode = id end end
assert(math_mode, 'Math mode unavailable')

-- REVTeX centers a tabular as $\vcenter{...}$ where the kernel uses \vcenter@text and no
-- math, and Scientific WorkPlace sets $\begin{tabular}...\end{tabular}\ $: a formula
-- holding only a box with a rich table, and spacing, is layout and its cells stay text.
local boxed_subtypes
-- Whether the formula is one box, accepted by accept(noad, box), and spacing (and, with marks,
-- whatsits: listings leaves a whatsit beside its box).
local function lone_box(head, accept, marks)
  if not boxed_subtypes then
    boxed_subtypes = {}
    for id, name in pairs(node.subtypes('noad')) do
      if name == 'vcenter' or name == 'ord' then boxed_subtypes[id] = true end
    end
  end
  local found = false
  for n in node.traverse(head) do
    local nucleus = n.id == node.id('noad') and boxed_subtypes[n.subtype] and not n.sub and not n.sup and n.nucleus
    if nucleus and nucleus.id == node.id('sub_box') and not found then
      found = nucleus.head ~= nil and accept(n, nucleus.head)
      if not found then return false end
    elseif n.id ~= node.id('glue') and n.id ~= node.id('kern') and n.id ~= node.id('penalty')
        and not (marks and n.id == node.id('whatsit')) then
      return false
    end
  end
  return found
end
-- A cell of the box's own (not the cell the formula itself stands in: \vdots in a tabular cell is
-- a box of text set in that cell) makes the box a table.
local function holds_cells(head, cell, outer)
  for n in node.traverse(head) do
    local value = node.has_attribute(n, cell)
    if value and value ~= outer then return true end
    if (n.id == node.id('hlist') or n.id == node.id('vlist')) and holds_cells(n.head, cell, outer) then return true end
  end
  return false
end
local function boxed_table(head)
  local cell = luatexbase.attributes and luatexbase.attributes['relatex.table.cell']
  if not cell then return false end
  local outer = tex.getattribute(cell)
  if outer == -2147483647 then outer = nil end
  return lone_box(head, function(_, box) return holds_cells(box, cell, outer) end)
end

-- listings sets a listing in a table cell (inner horizontal mode) as $\vcenter{...}$ (\vtop,
-- \vbox by boxpos): a formula holding only that box, and spacing, is layout and the listing's
-- lines stay text, as a boxed table's cells do. The box carries the attribute its listing set
-- (M.listing_box, from listings' BoxUnsafe hook).
local listing_attr
function M.listing_box()
  listing_attr = listing_attr or luatexbase.new_attribute('relatex.listing.box')
  tex.setattribute(listing_attr, 1)
end
local function listing_box(head)
  if not listing_attr then return false end
  return lone_box(head, function(noad, box)
    return node.has_attribute(noad, listing_attr) == 1 or node.has_attribute(box, listing_attr) == 1
  end, true)
end

-- amsthm-style end-of-proof marks drawn with rules (\(\qed\) with \FullBox = \hbox{\vrule ...}, an
-- openbox of \vrule and \hrule): a formula holding only spacing and boxes made of rules (no glyph, no
-- inner formula) is the drawn box, a decoration like the rule of an underline, not a formula.
local function rule_only_box(list)
  local rules = 0
  for n in node.traverse(list) do
    if n.id == node.id('rule') then
      rules = rules + 1
    elseif n.id == node.id('hlist') or n.id == node.id('vlist') then
      local inner = n.head or n.list
      local count = inner and rule_only_box(inner)
      if not count then return false end
      rules = rules + count
    elseif n.id ~= node.id('glue') and n.id ~= node.id('kern') and n.id ~= node.id('penalty') then
      return false
    end
  end
  return rules
end
local function rule_box_formula(head)
  local rules = 0
  for n in node.traverse(head) do
    local id = n.id
    if id == node.id('noad') then
      local nucleus = n.nucleus
      if n.sub or n.sup or not nucleus or nucleus.id ~= node.id('sub_box') or not nucleus.head then return false end
      local count = rule_only_box(nucleus.head)
      if not count then return false end
      rules = rules + count
    elseif id ~= node.id('glue') and id ~= node.id('kern') and id ~= node.id('penalty') then
      return false
    end
  end
  return rules > 0
end

local function layout_only(tree)
  if type(tree) ~= 'table' then return tree == '' end
  local allowed = {math = true, mrow = true, mstyle = true, mtext = true}
  if not allowed[tree[0]] then return false end
  for _, child in ipairs(tree) do if not layout_only(child) then return false end end
  return true
end

-- A script with no content (url's hyphen, set with an empty subscript) says nothing: the base
-- stands alone.
local function without_empty_script(node)
  if type(node) ~= 'table' or (node[0] ~= 'msub' and node[0] ~= 'msup') or #node ~= 2 then return node end
  local script = node[2]
  if type(script) == 'table' and script[0] == 'mrow' and #script == 0 and type(node[1]) == 'table' then return node[1] end
  return node
end

-- An accent set from a text font's accent slots (OT1: grave 18, acute 19, check 20, breve 21,
-- macron 22, ring 23, double acute 125, diaeresis 127) reaches MathML as the raw control
-- character when no family mapping names it: it is the spacing accent.
local ACCENT_SLOTS = {
  [18] = '`', [19] = '\u{B4}', [20] = '\u{2C7}', [21] = '\u{2D8}', [22] = '\u{AF}', [23] = '\u{2DA}',
  [127] = '\u{A8}'
}
local function accent_text(text)
  return (text:gsub('[\18-\23\127]', function(char) return ACCENT_SLOTS[char:byte()] end))
end

local function unicode_negations(tree)
  if type(tree) ~= 'table' then return end
  -- TeX sets a binary operator that opens a list (the minus of e^-) as an ordinary symbol; its
  -- character is still an operator, ASCII or the decoded minus sign.
  if tree[0] == 'mi' and type(tree[1]) == 'string'
      and (tree[1]:match('^[%(%)%[%]{}:;,.=+%-]$') or tree[1] == '\u{2212}') then
    tree[0] = 'mo'
    tree.mathvariant = nil
  end
  -- \dot over a base: slot 95 of the operators family is the accent in a mover and the underscore
  -- anywhere else, so only over a base does the underscore character stand for the dot accent.
  if tree[0] == 'mover' then
    local mark = tree[2]
    if type(mark) == 'table' and mark[0] == 'mo' and mark[1] == '_' then mark[1] = utf8.char(0x2D9) end
    -- A wide \widetilde (wider than 2em) is slot 93 of the AMS symbol font, which spells the
    -- accent as the closing bracket the slot would be in ASCII: over a base it is the tilde.
    if type(mark) == 'table' and mark[0] == 'mo' and mark[1] == ']' then mark[1] = utf8.char(0x2DC) end
    -- A wide \widehat is its slot 91 (glyph "hatwider"), the opening bracket in ASCII: the hat,
    -- spelled as the narrower \widehat reads.
    if type(mark) == 'table' and mark[0] == 'mo' and mark[1] == '[' then mark[1] = '^' end
  end
  -- LuaMML marks the italic h (U+210E, the hole of the Mathematical Italic block) as normal,
  -- since only ASCII letters are italic by default; the character already is the italic h.
  if tree[0] == 'mi' and tree[1] == utf8.char(0x210E) and tree.mathvariant == 'normal' then
    tree.mathvariant = nil
  end
  for index, child in ipairs(tree) do
    unicode_negations(child)
    child = without_empty_script(child)
    if type(child) == 'string' and (tree[0] == 'mo' or tree[0] == 'mi') then child = accent_text(child) end
    tree[index] = child
  end
  local negated = {['=']='≠', ['∈']='∉'}
  local index = 1
  while index < #tree do
    local mark, operator = tree[index], tree[index+1]
    if type(mark) == 'table' and mark[0] == 'mo' and mark[1] == utf8.char(0x338)
        and type(operator) == 'table' and operator[0] == 'mo' and negated[operator[1]] then
      mark[1] = negated[operator[1]]
      mark.rspace = operator.rspace
      table.remove(tree, index+1)
    end
    -- Two left quotation marks typed in math (``) are adjacent ordinary characters of one text
    -- font, which TeX ligatures as in text: the font paints the double quotation mark.
    local quote = utf8.char(0x2018)
    if type(mark) == 'table' and mark[0] == 'mi' and mark[1] == quote and #mark == 1
        and type(operator) == 'table' and operator[0] == 'mi' and operator[1] == quote and #operator == 1 then
      mark[1] = utf8.char(0x201C)
      table.remove(tree, index+1)
    end
    index = index + 1
  end
end

-- MathML text holds only characters: an mtext with formulas inside it (math inside \text)
-- becomes an mrow of mtext runs and those formulas, in order.
-- LuaMML leaves a vertical box in math (\vbox, \parbox, \shortstack, a tabular) as an
-- <mglyph> placeholder for a tagging pass that does not run here. Read it as a table:
-- one row per line. A line reads as LuaMML's own text conversion (luamml-lr) does, with
-- an inline formula's saved MathML, except that a nested box reads through instead of as
-- U+FFFD (a tabularray cell, a split's \vcenter), a rule is no text, and glyphs come from
-- the collector's decoder (LuaMML's assumes every font has a Lua table; TFM fonts may not).
-- A glyph of an inner formula whose start node holds no saved MathML (a cell typeset
-- again after measuring) reads as that formula's converted tree, once.
local function formula_run(n, seen)
  local identity = core and core.math_bridge.identity(n)
  if not identity then return nil end
  if seen[identity.formula] then return false end
  local tree = arrays.tree(identity.formula)
  if not tree then return nil end
  seen[identity.formula] = true
  local row = {[0] = 'mrow'}
  for index, child in ipairs(tree) do row[index] = child end
  return row
end
local function line_runs(head, out, seen)
  local n = head
  while n do
    local props = properties[n]
    if n.id == node.id('glyph') then
      local run = formula_run(n, seen)
      if run == nil then run = require('relatex-glyphs').character(n) or '\u{FFFD}' end
      if run then out[#out + 1] = run end
    elseif n.id == node.id('math') and n.subtype == 0 then
      local mml = props and (props.saved_mathml_table or props.saved_mathml_core)
      if mml then
        out[#out + 1] = mml
        n = node.end_of_math(n)
        if not n then break end
      end
    elseif n.id == node.id('glue') and n.width > 1000 then
      -- As LuaMML does: MathML collapses ordinary spaces in text.
      out[#out + 1] = '\u{00A0}'
    elseif (n.id == node.id('hlist') or n.id == node.id('vlist')) and n.head then
      line_runs(n.head, out, seen)
    elseif n.id == node.id('disc') and n.replace then
      line_runs(n.replace, out, seen)
    end
    n = n.next
  end
  return out
end
local function line_text(head) return line_runs(head, {[0] = 'mtext'}, {}) end
local function vertical_lines(list, rows)
  for n in node.traverse(list) do
    if n.id == node.id('hlist') and n.head then
      local line = line_text(n.head)
      if #line > 0 then rows[#rows + 1] = {[0] = 'mtr', {[0] = 'mtd', line}} end
    elseif n.id == node.id('vlist') and n.head then
      vertical_lines(n.head, rows)
    end
  end
  return rows
end
local function expand_vertical_boxes(tree)
  if type(tree) ~= 'table' then return tree end
  for index, child in ipairs(tree) do
    if type(child) == 'table' and child[0] == 'mglyph' and child['tex:box'] then
      local box = child['tex:box']
      local rows = box.id == node.id('vlist') and vertical_lines(box.head, {}) or {}
      tree[index] = #rows > 0 and {[0] = 'mtable', table.unpack(rows)} or {[0] = 'mtext', '\u{FFFD}'}
    else
      tree[index] = expand_vertical_boxes(child)
    end
  end
  -- An mi holding only the expanded box is the box.
  if tree[0] == 'mi' and #tree == 1 and type(tree[1]) == 'table' and (tree[1][0] == 'mtable' or tree[1][0] == 'mtext') then
    return tree[1]
  end
  return tree
end

local function split_text(tree)
  if type(tree) ~= 'table' then return tree end
  for index, child in ipairs(tree) do tree[index] = split_text(child) end
  if tree[0] ~= 'mtext' then return tree end
  local nested = false
  for _, child in ipairs(tree) do if type(child) == 'table' then nested = true end end
  if not nested then return tree end
  local row, run = {[0] = 'mrow'}, nil
  for _, child in ipairs(tree) do
    if type(child) == 'table' then
      run = nil
      row[#row + 1] = child
    else
      if not run then
        run = {[0] = 'mtext'}
        for key, value in pairs(tree) do if type(key) == 'string' then run[key] = value end end
        row[#row + 1] = run
      end
      run[#run + 1] = child
    end
  end
  return row
end

function M.family(id, declaration)
  declaration = declaration:gsub('^\\', '')
  declarations[id] = declaration
  local encoding = declaration:gsub('^\\', ''):match('^([A-Za-z0-9]+)%s*/')
  assert(encoding, 'Unsupported NFSS family declaration: ' .. declaration)
  -- newtxmath's LMX (ntxexx) extends OMX above slot 127 and keeps its lower half: the n-ary
  -- operators, delimiters and wide accents (glyphs "hatwide", "tildewide" at "62, "65) of cmex.
  local legacy = mappings[encoding:lower()] or encoding == 'LMX' and mappings.omx
  supplemental[id] = (not legacy or encoding == 'OMS') and declaration or nil
  local extra = symbols.mapping(id, declaration)
  local mapping = legacy or extra
  if legacy and extra then
    mapping = {}
    for from, to in pairs(legacy) do mapping[from] = to end
    for from, to in pairs(extra) do mapping[from] = to end
  end
  if applied[id] then
    local reset = {}
    for code in pairs(applied[id]) do reset[code] = code end
    converter.register_family(id, reset)
  end
  if mapping then converter.register_family(id, mapping) end
  applied[id] = mapping
  families[id] = not mapping
end

function M.bind_fonts(declaration, text, script, scriptscript)
  local encoding = declaration:gsub('^\\', ''):match('^([A-Za-z0-9]+)%s*/')
  for _, id in ipairs({text, script, scriptscript}) do
    alphabets.bind(id, declaration)
    if encoding and type(id) == 'number' and id > 0 then core.math_bridge.font_encodings[id] = encoding end
  end
end

function M.seed_declared_families()
  -- AMS classes can initialize NFSS before this observer is loaded. Read the
  -- existing declaration table; do not initialize fonts or typeset anything.
  local groups = token.get_macro('group@list') or ''
  for command, declaration in groups:gmatch('\\group@elt%s*\\([%w@]+)%s*\\([^%s\\]+)') do
    local family = token.create(command)
    if family.cmdname == 'char_given' then M.family(family.mode, declaration) end
  end
end

function M.native_row(tree, children, head, entity)
  total = total + 1
  core.math_bridge.begin_formula(total, true, false, {entity = entity, role = 'paragraph'})
  core.math_bridge.compose_formula(total, children)
  arrows.compose(tree)
  core.external_math(write_xml(tree, false), total, entity)
  arrays.save(total, tree)
  -- A display of several lines (multline) is one formula whose ink lies under each line.
  for _, line in ipairs(type(head) == 'table' and head or {head}) do
    core.math_bridge.capture_native_row(line)
    core.flow.capture(line, 'math', entity, 'paragraph')
  end
  core.math_bridge.end_formula()
  return total
end

-- The later lines of a formula whose lines ship on several pages (a multline broken by a page): their
-- ink is the formula's, recorded once with all its lines on its first page.
function M.continue_row(formula, heads, entity)
  core.math_bridge.resume_formula(formula)
  for _, line in ipairs(heads) do
    core.math_bridge.capture_native_row(line)
    core.flow.capture(line, 'math', entity, 'paragraph')
  end
  core.math_bridge.end_formula()
end

-- Text-mode \underline{X} is the kernel's $\@@underline{\hbox{X}}$: an underline over text and nothing
-- else (MathML munder over mpadded/mtext, under mo "_"). It is text with an underline, not a formula:
-- its glyphs are marked with relatex.decoration = 1 and read as text, its rule (2) is the decoration.
-- An underline holding a formula ($P$ inside) keeps its formula reading.
local function decoration_attribute()
  return luatexbase.attributes['relatex.decoration'] or luatexbase.new_attribute('relatex.decoration')
end

local function text_underline(tree)
  if type(tree) ~= 'table' or tree[0] ~= 'math' or #tree ~= 1 then return false end
  local under = tree[1]
  if type(under) ~= 'table' or under[0] ~= 'munder' or #under ~= 2 then return false end
  local mark = under[2]
  if type(mark) ~= 'table' or mark[0] ~= 'mo' or mark[1] ~= '_' then return false end
  local function text_only(node)
    if type(node) ~= 'table' then return type(node) == 'string' end
    local allowed = {mrow = true, mstyle = true, mtext = true, mpadded = true}
    if not allowed[node[0]] then return false end
    for _, child in ipairs(node) do if not text_only(child) then return false end end
    return true
  end
  return text_only(under[1])
end

local function mark_decoration(list, value, kinds)
  local attribute = decoration_attribute()
  for n in node.traverse(list) do
    if kinds[n.id] then node.set_attribute(n, attribute, value) end
    local inner = n.head or n.list
    if (n.id == node.id('hlist') or n.id == node.id('vlist')) and inner then mark_decoration(inner, value, kinds) end
    local nucleus = n.nucleus
    if nucleus and nucleus.id == node.id('sub_box') and (nucleus.head or nucleus.list) then
      mark_decoration(nucleus.head or nucleus.list, value, kinds)
    end
  end
end

function M.install(collector)
  core = collector
  core.math_bridge.enable_choices()
  M.seed_declared_families()
  if tex.getfontoffamily then
    for id, declaration in pairs(declarations) do
      M.bind_fonts(declaration, tex.getfontoffamily(id, 0), tex.getfontoffamily(id, 1), tex.getfontoffamily(id, 2))
    end
  end
  luatexbase.add_to_callback('pre_mlist_to_hlist_filter', function(head, style)
    -- An eqno is converted inside its surrounding display and is not a formula.
    local equation_number = tex.nest.top.mode == math_mode
    local source = core.math_source and core.math_source.enabled() and core.math_source.close() or nil
    local startmath = tex.nest.top.tail
    -- A formula inside a QED mark box is the mark's drawing (relatex-flow, decision 176).
    local qed_mark = core.flow and core.flow.in_qed()
    if equation_number or qed_mark or boxed_table(head) or listing_box(head) then frames[#frames + 1] = false; return true end
    -- A one-symbol formula in a proof's end code is its QED mark (relatex-flow, decision 176).
    if (styles[style] or 0) >= 2 and core.flow and core.flow.ending_mark(head) then frames[#frames + 1] = 'qed'; return true end
    if (styles[style] or 0) >= 2 and rule_box_formula(head) then frames[#frames + 1] = false; return true end
    -- A first URL can initialize NFSS while temporarily replacing mathcodes
    -- and escapechar. Bind at conversion time to the current declarations,
    -- rather than retaining a partial mapping from that temporary context.
    for id, declaration in pairs(supplemental) do M.family(id, declaration) end
    local numeric_style = assert(styles[style], 'Unsupported native mathematical style')
    local children = arrays.prepare(head, core.math_bridge, numeric_style)
    local boxed, failures = boxes.prepare(head, core.math_bridge, arrays, numeric_style)
    local adopted = {}
    for _, id in ipairs(children) do adopted[id] = true end
    for _, id in ipairs(boxed) do adopted[id] = true end
    children = {}
    for id in pairs(adopted) do children[#children + 1] = id end
    table.sort(children)
    local xml = observation.observe(head, function(copied)
      if observation.wrap_scripts then observation.wrap_scripts(copied) end
      -- Primes the font joins into one glyph are one character (decision 149).
      primes.join(copied, style)
      local tree = converter.make_root(remap.with_family_fonts(function()
        return converter.process(copied, numeric_style, families)
      end), numeric_style)
      -- Never stop the build over a box the expansion cannot read: keep LuaMML's result.
      local expanded_ok, expanded = pcall(expand_vertical_boxes, tree)
      if expanded_ok then tree = expanded end
      local alphabet_failures = {}
      alphabets.apply(tree, alphabet_failures)
      for reason in pairs(alphabet_failures) do failures[#failures + 1] = reason end
      boxes.fraction_rules(tree)
      -- Arrows set from pieces are read while the pieces still know their family and slot.
      arrows.compose(tree)
      return arrays.scalar_tree(tree)
    end)
    -- Canonicalize the JSON-side tree only. Separate combining-overlay and
    -- relation nodes otherwise put a combining character before its base.
    unicode_negations(xml)
    xml = split_text(xml)
    if text_underline(xml) then
      mark_decoration(head, 1, {[node.id('glyph')] = true})
      frames[#frames + 1] = 'underline'
      core.external_math_layout(write_xml(xml, false), style)
      return true
    end
    if layout_only(xml) then
      frames[#frames + 1] = false
      core.external_math_layout(write_xml(xml, false), style)
      return true
    end
    total = total + 1
    frames[#frames + 1] = {id=total, tree=xml}
    -- An inline formula may sit inside an outer formula's \text{}: save its MathML on the
    -- start-math node, as LuaMML does, so the outer conversion keeps the surrounding text.
    if numeric_style >= 2 and startmath then
      local props = properties[startmath] or {}
      -- Its content, not a second <math> root inside the outer one.
      local inner = xml
      if type(xml) == 'table' and xml[0] == 'math' then
        inner = {[0] = 'mrow'}
        for index, child in ipairs(xml) do inner[index] = child end
      end
      props.saved_mathml_table = inner
      properties[startmath] = props
    end
    local measuring, truth = token.create('ifmeasuring@'), token.create('iftrue')
    local trial = measuring.command == truth.command and measuring.mode == truth.mode
    core.math_bridge.begin_formula(total, numeric_style < 2, trial)
    if #children > 0 then core.math_bridge.compose_formula(total, children) end
    core.external_math(write_xml(xml, false), total, nil, source)
    for _, reason in ipairs(failures) do core.external_math_diagnostic('unsupported_native_math_box', {formula=total, reason=reason}) end
    return true
  end, 'relatex.external.math')
  luatexbase.add_to_callback('post_mlist_to_hlist_filter', function(head)
    local entered = table.remove(frames)
    assert(entered ~= nil, 'Unbalanced external math callbacks')
    if entered == 'underline' then
      mark_decoration(head, 2, {[node.id('rule')] = true})
      entered = false
    end
    if entered == 'qed' then return core.flow.wrap_ending_mark(head) end
    if entered then
      arrays.save(entered.id, entered.tree)
      local function has_ink(list)
        for n in node.traverse(list) do
          -- An included graphic (adjustbox's valign=c image) is the figure, not the formula's ink.
          if core.math_bridge.is_ink(n) then return true end
          if (n.id == node.id('hlist') or n.id == node.id('vlist')) and not boxes.graphic_box(n) and has_ink(n.head) then
            return true
          end
        end
        return false
      end
      if not has_ink(head) then
        core.external_math_diagnostic('external_math_without_ink', {formula=entered.id, glyphs=0, rules=0})
      end
      core.math_bridge.end_formula()
    end
    return head
  end, 'relatex.external.math.end')
  -- post_mlist is a reverselist: before means executed after fragment capture.
  luatexbase.declare_callback_rule('post_mlist_to_hlist_filter',
    'relatex.external.math.end', 'before', 'relatex.flow.math')
  luatexbase.declare_callback_rule('post_mlist_to_hlist_filter',
    'relatex.external.math.end', 'before', 'relatex.math.fragments')
end

return M
