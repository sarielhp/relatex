-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Math-choice provenance uses the final callback style, not first-pass
-- \mathstyle. Traversal follows the pinned LuaMML style rules. Only scalar
-- fragment identities survive this callback; source nodes are never modified.
local M = {}
local kind = {}
for _, name in ipairs{'hlist', 'vlist', 'sub_box', 'sub_mlist', 'noad', 'accent',
    'radical', 'fraction', 'choice', 'style'} do kind[name] = node.id(name) end
local styles = {display = 0, crampeddisplay = 1, text = 2, crampedtext = 3,
  script = 4, crampedscript = 5, scriptscript = 6, crampedscriptscript = 7}
local branches = {'display', 'text', 'script', 'scriptscript'}
local writer, identity, current_formula, choice_count
local walk

local function collect(head, formula, found)
  if not head then return end
  for n in node.traverse(head) do
    local item = identity(n)
    if item and item.formula == formula then found[item.fragment] = true end
    if n.id == kind.hlist or n.id == kind.vlist then
      collect(n.head, formula, found)
    elseif n.id == kind.sub_box or n.id == kind.sub_mlist then
      collect(n.head, formula, found)
    elseif n.id == kind.noad or n.id == kind.accent or n.id == kind.radical then
      collect(n.nucleus, formula, found)
      collect(n.sub, formula, found)
      collect(n.sup, formula, found)
    elseif n.id == kind.fraction then
      collect(n.num, formula, found)
      collect(n.denom, formula, found)
    elseif n.id == kind.choice then
      for _, name in ipairs(branches) do collect(n[name], formula, found) end
    end
  end
end

local function sorted_keys(values)
  local keys = {}
  for id in pairs(values) do keys[#keys + 1] = id end
  table.sort(keys)
  return keys
end

local function observe_choice(n, style, formula)
  local selected = branches[style // 2 + 1]
  local kept, discarded = {}, {}
  collect(n[selected], formula, kept)
  for _, name in ipairs(branches) do
    if name ~= selected then collect(n[name], formula, discarded) end
  end
  for id in pairs(kept) do discarded[id] = nil end
  if next(discarded) then
    choice_count = choice_count + 1
    writer('diagnostic', {code = 'math_choice_discard', source = 'luatex-mlist-style',
      choice = choice_count, formula = formula, chosen_style = style,
      discarded_fragments = sorted_keys(discarded), kept_fragments = sorted_keys(kept)})
  end
  walk(n[selected], 2 * (style // 2), formula)
end

local function kernel(n, style, formula)
  if n and n.id == kind.sub_mlist then walk(n.head, style, formula) end
end

walk = function(head, style, formula)
  if not head then return end
  for n in node.traverse(head) do
    if n.id == kind.style then
      style = n.subtype
    elseif n.id == kind.choice then
      observe_choice(n, style, formula)
    elseif n.id == kind.fraction then
      kernel(n.num, style + 2 - style // 6 * 2, formula)
      kernel(n.denom, style // 2 * 2 + 3 - style // 6 * 2, formula)
    elseif n.id == kind.noad or n.id == kind.accent or n.id == kind.radical then
      local nucleus_style = style
      if n.id ~= kind.noad or n.subtype == 11 then nucleus_style = style // 2 * 2 + 1 end
      kernel(n.nucleus, nucleus_style, formula)
      kernel(n.sub, style // 4 * 2 + 5, formula)
      kernel(n.sup, style // 4 * 2 + 4 + style % 2, formula)
    end
  end
end

function M.install(emit, formula_reader, identity_reader)
  assert(not writer, 'Math-choice observer already installed')
  writer, current_formula, identity, choice_count = emit, formula_reader, identity_reader, 0
  luatexbase.add_to_callback('pre_mlist_to_hlist_filter', function(head, style)
    local formula = current_formula()
    if formula then
      local number = assert(styles[style], 'Unsupported math-choice callback style')
      walk(head, number, formula)
    end
    return head
  end, 'relatex.math.choices')
  if os.getenv('RELATEX_STRATEGY') == 'external' then
    luatexbase.declare_callback_rule('pre_mlist_to_hlist_filter',
      'relatex.math.choices', 'after', 'relatex.external.math')
  else
    luatexbase.declare_callback_rule('pre_mlist_to_hlist_filter',
      'relatex.math.choices', 'before', 'luamml.to_mathml')
  end
end

return M
