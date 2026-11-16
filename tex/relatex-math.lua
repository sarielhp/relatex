-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Version-pinned latex-lab formula IDs, with copied scalar attributes only.
local M = {}
local formula_attr = luatexbase.new_attribute('relatex.math.formula')
local fragment_attr = luatexbase.new_attribute('relatex.math.fragment')
local position_attr = luatexbase.new_attribute('relatex.math.position')
local glyph, rule, hlist, vlist = node.id('glyph'), node.id('rule'), node.id('hlist'), node.id('vlist')
local emit, active, stack, fragment = nil, false, {}, 0
local current_context, formula_contexts = nil, {}
local page_attributes = {}
local choices_enabled, choices_installed = false, false
local compositions = {}
local fragment_metrics = {}

-- The printed character of a math glyph whose font declares a legacy encoding (OML: slot 0x3A is
-- a period), or nil. Letters stay with the text decoder: their math-alphabet forms are not what
-- a reference prints.
M.font_encodings = {}
function M.printed_character(n)
  local encoding = M.font_encodings[n.font]
  local map = encoding and require('relatex-legacy-mappings')[encoding:lower()]
  local f = map and font.getfont(n.font)
  local layout = f and require('relatex-math-remap').font_layout(f.name)
  local code = layout and layout[n.char] or map and map[n.char]
  if code and code ~= n.char and code >= 32 and code < 0x1D400 then return utf8.char(code) end
end

function M.compose_formula(id, children)
  assert(active and formula_contexts[id], 'Math composition requires a live parent')
  local found = {}
  for _, child in ipairs(children) do
    assert(child < id and formula_contexts[child], 'Math composition requires an earlier child')
    found[child] = true
  end
  compositions[id] = found
  emit('diagnostic', {code='external_math_composition', source='native-alignment-cells', formula=id, children=children})
end

-- The formulas a formula was composed of (its adopted children), or nil.
function M.composed(id)
  return compositions[id]
end

function M.enable_choices()
  choices_enabled = true
  if active and not choices_installed then
    require('relatex-choices').install(emit, M.current_formula, M.identity)
    choices_installed = true
  end
end

function M.page_begin()
  page_attributes[#page_attributes + 1] = {
    tex.getattribute(formula_attr), tex.getattribute(fragment_attr), tex.getattribute(position_attr),
  }
  tex.setattribute(formula_attr, -2147483647)
  tex.setattribute(fragment_attr, -2147483647)
  tex.setattribute(position_attr, -2147483647)
end

function M.page_end()
  local saved = assert(table.remove(page_attributes), 'Unbalanced page math attributes')
  tex.setattribute(formula_attr, saved[1] or -2147483647)
  tex.setattribute(fragment_attr, saved[2] or -2147483647)
  tex.setattribute(position_attr, saved[3] or -2147483647)
end

function M.start(writer, context_reader)
  emit, active = writer, true
  current_context = context_reader
  if choices_enabled then M.enable_choices() end
end

function M.begin_formula(id, display, trial, native_context)
  if not active then return end
  stack[#stack + 1] = tex.getattribute(formula_attr) or -2147483647
  tex.setattribute(formula_attr, id)
  local entity, role = current_context()
  if native_context then entity, role = native_context.entity, native_context.role end
  formula_contexts[id] = {entity = entity, role = role}
  local record = {formula = id, display = display, entity = entity}
  if trial then record.trial, record.trial_provenance = true, 'amsmath-ifmeasuring' end
  emit('math_candidate', record)
end

-- Mark later ink of a formula already recorded (the next page's lines of a broken multline): its
-- identity is set again, and no second candidate is recorded.
function M.resume_formula(id)
  if not active then return end
  assert(formula_contexts[id], 'A resumed formula must be recorded')
  stack[#stack + 1] = tex.getattribute(formula_attr) or -2147483647
  tex.setattribute(formula_attr, id)
end

function M.formula_context(id)
  local context = formula_contexts[id]
  return context and context.entity, context and context.role
end

function M.context()
  local saved = formula_contexts[M.current_formula()]
  if saved then return saved.entity, saved.role end
  if os.getenv('RELATEX_STRATEGY') == 'external' and current_context then return current_context() end
  return 0, 'paragraph'
end

function M.current_formula()
  local id = tex.getattribute(formula_attr)
  if id and id > 0 then return id end
end

function M.identity(n)
  local fragment_id = node.has_attribute(n, fragment_attr)
  if fragment_id then
    return {formula = node.has_attribute(n, formula_attr), fragment = fragment_id,
      position = node.has_attribute(n, position_attr)}
  end
end

function M.fragment_metrics(id)
  return fragment_metrics[id]
end

function M.end_formula()
  if not active then return end
  assert(#stack > 0, 'Unbalanced math context')
  tex.setattribute(formula_attr, table.remove(stack))
end

function M.is_math(n)
  return node.has_attribute(n, formula_attr) ~= nil
end

-- An included graphic is a rule node of the image subtype, or of the box subtype when it is
-- clipped (a form): never the formula's ink, though it sits inside one (adjustbox's valign=c).
local GRAPHIC = {}
for id, name in pairs(node.subtypes('rule')) do if name == 'image' or name == 'box' then GRAPHIC[id] = true end end
M.GRAPHIC = GRAPHIC

function M.is_ink(n)
  -- A running rule dimension is a negative sentinel, not absent ink.
  return n.id == glyph or (n.id == rule and not GRAPHIC[n.subtype] and n.width ~= 0 and n.height + n.depth > 0)
end

local function contained_graphic(n)
  local flow = (package.loaded['relatex'] or {}).flow
  return flow and flow.contained_graphic(n)
end

local function symbol_box(n)
  local flow = (package.loaded['relatex'] or {}).flow
  return flow and flow.is_symbol_box(n)
end

local function mark(head, id, positions, only_attributed)
  for n in node.traverse(head) do
    if M.is_ink(n) then
      local prior = node.has_attribute(n, formula_attr)
      if compositions[id] and compositions[id][prior] then
        node.unset_attribute(n, fragment_attr)
        node.unset_attribute(n, position_attr)
        node.set_attribute(n, formula_attr, id)
      end
      if not node.has_attribute(n, fragment_attr) and (not only_attributed or node.has_attribute(n, formula_attr) == id) then
        positions.count = positions.count + 1
        node.set_attribute(n, formula_attr, id)
        node.set_attribute(n, fragment_attr, fragment)
        node.set_attribute(n, position_attr, positions.count)
      end
    elseif (n.id == hlist or n.id == vlist) and not contained_graphic(n) and not symbol_box(n) then
      -- A picture's glyphs (xy-pic arrows) are its drawing, not the formula's text; a symbol
      -- box's rules (\qedhere's square in a display) read as that symbol.
      mark(n.head, id, positions, only_attributed)
    end
  end
end

local function capture(head, only_attributed)
  if not active then return head end
  local id = tex.getattribute(formula_attr)
  if not id or id < 0 then return head end
  fragment = fragment + 1
  local positions = {count = 0}
  mark(head, id, positions, only_attributed)
  if positions.count > 0 then
    local width, height, depth = node.dimensions(head)
    fragment_metrics[fragment] = {formula=id, nodes=positions.count, width=width, height=height, depth=depth}
    local measuring, truth = token.create('ifmeasuring@'), token.create('iftrue')
    local trial = measuring.command == truth.command and measuring.mode == truth.mode
    emit('math_fragment', {formula = id, fragment = fragment, nodes = positions.count, trial = trial})
  end
  return head
end

function M.capture_box(head)
  return capture(head, true)
end

function M.capture_native_row(head)
  return capture(head, false)
end

luatexbase.add_to_callback('post_mlist_to_hlist_filter', function(head)
  return capture(head, false)
end, 'relatex.math.fragments')

function M.retained(n, runs)
  if not M.is_ink(n) then return false end
  local id = node.has_attribute(n, fragment_attr)
  if not id then return false end
  local position = node.has_attribute(n, position_attr)
  local last = runs[#runs]
  if last and last.fragment == id and last.last + 1 == position then
    last.last = position
  else
    runs[#runs + 1] = {formula = node.has_attribute(n, formula_attr), fragment = id, first = position, last = position}
  end
  return true
end

function M.finish()
  assert(#stack == 0, 'Unclosed math context')
  active = false
end

return M
