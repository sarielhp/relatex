-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- LuaMML normalizes binary noad subtypes while converting. It must never run
-- against the engine's live list: even an otherwise read-only observer would
-- change ordinary math spacing. Keep property annotations on an isolated copy.
local M = {}
local fields = {'head', 'nucleus', 'sub', 'sup', 'num', 'denom', 'degree',
  'display', 'text', 'script', 'scriptscript', 'accent', 'bot_accent', 'left', 'right'}

local function copy_value(value, seen)
  if type(value) ~= 'table' then return value end
  if seen[value] then return seen[value] end
  local result = {}
  seen[value] = result
  for key, child in pairs(value) do result[key] = copy_value(child, seen) end
  return result
end

local function properties(original, copied, seen)
  while original do
    assert(copied and original.id == copied.id, 'Math observer copy differs from native list')
    local value = node.getproperty(original)
    if value then node.setproperty(copied, copy_value(value, seen)) end
    for _, field in ipairs(fields) do
      local child = original[field]
      if child and node.is_node(child) then properties(child, copied[field], seen) end
    end
    original, copied = original.next, copied.next
  end
  assert(not copied, 'Math observer copy has extra nodes')
end

-- TeX attaches the scripts of \overline{K}_a to the over noad itself, which LuaMML does not read:
-- they would vanish from the MathML. On the copy only, the over (or under) noad becomes an
-- ordinary atom that carries the scripts around a copy of itself without them; the node keeps its
-- place in the list, so the copy still frees as one list.
local noad, sub_mlist, over, under, ord
local function noad_kinds()
  if noad then return end
  noad, sub_mlist = node.id('noad'), node.id('sub_mlist')
  for subtype, name in pairs(node.subtypes('noad')) do
    if name == 'over' then over = subtype elseif name == 'under' then under = subtype elseif name == 'ord' then ord = subtype end
  end
end
local wrap_scripts
local function wrap_children(n)
  for _, field in ipairs(fields) do
    local child = n[field]
    if child and node.is_node(child) then
      if child.id == sub_mlist then wrap_scripts(child.list) else wrap_children(child) end
    end
  end
end
function wrap_scripts(list)
  for n in node.traverse(list) do
    if n.id == noad then
      wrap_children(n)
      if (n.subtype == over or n.subtype == under) and (n.sub or n.sup) then
        local inner = node.new(noad, n.subtype)
        inner.nucleus = n.nucleus
        local box = node.new(sub_mlist)
        box.list = inner
        n.nucleus, n.subtype = box, ord
      end
    else
      wrap_children(n)
    end
  end
end
function M.wrap_scripts(list)
  noad_kinds()
  wrap_scripts(list)
end

-- LuaMML reads a text glyph's font with font.getfont, which has no table for a font TeX loaded
-- itself (the picture fonts line10 and lcircle10 of a \vector in a display): while it converts,
-- such a font reads as TeX's own record (font.getcopy), as the table readers do.
local faces = {}
local function face(getfont, id)
  local found = getfont(id)
  if found or not font.getcopy then return found end
  if faces[id] == nil then faces[id] = font.getcopy(id) or false end
  return faces[id] or nil
end

function M.observe(head, operation)
  local copied = node.copy_list(head)
  local getfont = font.getfont
  font.getfont = function(id) return face(getfont, id) end
  local ok, result = xpcall(function()
    properties(head, copied, {})
    return operation(copied)
  end, debug.traceback)
  font.getfont = getfont
  node.flush_list(copied)
  if not ok then error(result) end
  return result
end

return M
