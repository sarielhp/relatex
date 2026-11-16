-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Scalar MathML from executed alignment cells, attached only as Lua properties.
-- The engine's lists, dimensions and PDF output are never rewritten.
local M, trees = {}, {}
local properties = node.get_properties_table()
local hlist, vlist = node.id('hlist'), node.id('vlist')
local sub_box, sub_mlist = node.id('sub_box'), node.id('sub_mlist')
local noad, accent, radical, fraction = node.id('noad'), node.id('accent'), node.id('radical'), node.id('fraction')
local choice, style_node, glue, kern = node.id('choice'), node.id('style'), node.id('glue'), node.id('kern')
local branches = {'display', 'text', 'script', 'scriptscript'}
local alignment, cell
for key, name in pairs(node.subtypes(hlist)) do
  if name == 'alignment' then alignment = key end
  if name == 'cell' then cell = key end
end

local function scalar_copy(value)
  if type(value) ~= 'table' then return value end
  local result = {}
  for key, child in pairs(value) do
    if type(key) == 'number' or (type(key) == 'string' and not key:match('^:') and not key:match('^tex:')) then
      if type(child) ~= 'userdata' and type(child) ~= 'function' then result[key] = scalar_copy(child) end
    end
  end
  return result
end

M.scalar_tree = scalar_copy

function M.save(id, tree)
  trees[id] = scalar_copy(tree)
end

function M.tree(id)
  return trees[id] and scalar_copy(trees[id]) or nil
end

-- Returns true when the list holds a glyph outside every formula (text beside a formula).
local function collect_ids(head, bridge, ids)
  local text = false
  for n in node.traverse(head) do
    local identity = bridge.identity(n)
    if identity and identity.formula then ids[identity.formula] = true
    elseif n.id == node.id('glyph') then text = true end
    if (n.id == hlist or n.id == vlist) and collect_ids(n.head, bridge, ids) then text = true end
  end
  return text
end

local function cell_tree(box, bridge, adopted)
  local ids, count, id = {}, 0
  -- A cell with text beside its formula ($x$ \, . in a tabular) is no scalar formula cell:
  -- the vertical box reader keeps both.
  if collect_ids(box.head, bridge, ids) then return nil end
  -- A formula inside the cell's \text is one the cell's own formula was composed of.
  local nested = {}
  for found in pairs(ids) do
    for child in pairs(bridge.composed and bridge.composed(found) or {}) do nested[child] = true end
  end
  for child in pairs(nested) do ids[child] = nil end
  for found in pairs(ids) do count = count + 1; id = found end
  if count ~= 1 or not trees[id] then return nil end
  local tree = scalar_copy(trees[id])
  tree[0], tree.display, tree.xmlns = 'mrow', nil, nil
  adopted[id] = true
  return {[0]='mtd', tree}
end

local function cell_alignment(box)
  local left, right, ink = false, false, false
  local wrapper, wrappers = nil, 0
  for n in node.traverse(box.head) do
    if n.id == glue and n.stretch > 0 and n.stretch_order > 0 then
      if ink then right = true else left = true end
    elseif n.id == node.id('glyph') or n.id == hlist or n.id == vlist then
      ink = true
      if n.id == hlist then wrapper, wrappers = n, wrappers + 1 end
    end
  end
  if not left and not right and wrappers == 1 then return cell_alignment(wrapper) end
  if left and not right then return 'right' end
  if right and not left then return 'left' end
  return 'center'
end

local function alignment_tree(box, bridge)
  if not box or box.id ~= vlist then return end
  local result, adopted, columns, alignments = {[0]='mtable'}, {}, nil, nil
  local y, previous = box.height, nil
  for row in node.traverse(box.head) do
    if row.id == hlist or row.id == vlist then y = y - row.height
    elseif row.id == kern then y = y - row.kern
    elseif row.id == glue then y = y - node.effective_glue(row, box) end
    if row.id == hlist and row.subtype == alignment then
      -- An ooalign relation/decoration uses coincident baselines, not table
      -- rows. Let the native box adapter retain those physical placements.
      if previous and y >= previous - 1 then return end
      previous = y
      local xml, row_alignment = {[0]='mtr'}, {}
      for column in node.traverse(row.head) do
        if column.id == hlist and column.subtype == cell then
          local value = cell_tree(column, bridge, adopted)
          if not value then return end
          xml[#xml + 1] = value
          row_alignment[#row_alignment + 1] = cell_alignment(column)
        end
      end
      if #xml == 0 or (columns and columns ~= #xml) then return end
      columns = #xml
      local alignment = table.concat(row_alignment, ' ')
      if alignments and alignments ~= alignment then
        for index, value in ipairs(xml) do value.columnalign = row_alignment[index] end
      else alignments = alignment end
      result[#result + 1] = xml
    end
    if row.id == hlist or row.id == vlist then y = y - row.depth end
  end
  if #result > 0 then result.columnalign = alignments; return result, adopted end
end

local walk
local function kernel(value, bridge, adopted, style)
  if not value then return end
  if value.id == sub_mlist then walk(value.head, bridge, adopted, style); return end
  if value.id ~= sub_box then return end
  local tree, children = alignment_tree(value.head, bridge)
  if not tree then return end
  local props = properties[value] or {}
  if props.mathml_table ~= nil then return end
  props.mathml_table = tree
  properties[value] = props
  for id in pairs(children) do adopted[id] = true end
end

walk = function(head, bridge, adopted, style)
  if not head then return end
  for n in node.traverse(head) do
    if n.id == style_node then style = n.subtype
    elseif n.id == choice then walk(n[branches[style // 2 + 1]], bridge, adopted, style)
    elseif n.id == noad or n.id == accent or n.id == radical then
      kernel(n.nucleus, bridge, adopted, style)
      kernel(n.sub, bridge, adopted, style // 4 * 2 + 5)
      kernel(n.sup, bridge, adopted, style // 4 * 2 + 4 + style % 2)
    elseif n.id == fraction then
      kernel(n.num, bridge, adopted, style + 2 - style // 6 * 2)
      kernel(n.denom, bridge, adopted, style // 2 * 2 + 3 - style // 6 * 2)
    end
  end
end

function M.prepare(head, bridge, style)
  local adopted, ids = {}, {}
  walk(head, bridge, adopted, style or 2)
  for id in pairs(adopted) do ids[#ids + 1] = id end
  table.sort(ids)
  return ids
end

return M
