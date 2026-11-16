-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Research observer only: no node mutation, no callback replacement.
require('lualibs')
local M = {}
local page = 0
local function snapshot(head)
  local result = {}
  for n in node.traverse(head) do
    local kind = node.type(n.id)
    local item = nil
    if kind == 'hlist' or kind == 'vlist' then
      item = {kind = kind, width = n.width, height = n.height, depth = n.depth, shift = n.shift,
        dir = n.dir, glue_order = n.glue_order, glue_sign = n.glue_sign,
        glue_set = string.format('%.17g', n.glue_set), children = snapshot(n.head)}
    elseif kind == 'glyph' then
      -- Some glyph fonts expose no Lua font table; record the id without a name.
      local f = font.getfont(n.font) or {}
      item = {kind = kind, char = n.char, font = f.fullname or f.psname or f.name, font_id = n.font, size = f.size,
        width = n.width, height = n.height, depth = n.depth, xoffset = n.xoffset, yoffset = n.yoffset,
        expansion = n.expansion_factor}
    elseif kind == 'rule' then
      item = {kind = kind, width = n.width, height = n.height, depth = n.depth}
    elseif kind == 'glue' and (n.width ~= 0 or n.stretch ~= 0 or n.shrink ~= 0) then
      item = {kind = kind, subtype = n.subtype, width = n.width, stretch = n.stretch, shrink = n.shrink,
        stretch_order = n.stretch_order, shrink_order = n.shrink_order}
      if n.leader then item.leader = snapshot(n.leader) end
    elseif kind == 'kern' and n.kern ~= 0 then
      item = {kind = kind, kern = n.kern, expansion = n.expansion_factor}
    elseif kind == 'disc' then
      item = {kind = kind, replace = snapshot(n.replace)}
    elseif kind == 'dir' then
      item = {kind = kind, dir = n.dir}
    elseif kind ~= 'whatsit' and kind ~= 'penalty' and kind ~= 'local_par'
      and kind ~= 'par' and kind ~= 'math' and kind ~= 'glue' and kind ~= 'kern' then
      item = {kind = 'unhandled', node_type = kind}
    end
    if item then result[#result + 1] = item end
  end
  return result
end

function M.capture(box, path)
  page = page + 1
  local file = assert(io.open(path or 'layout.jsonl', page == 1 and 'wb' or 'ab'))
  file:write(utilities.json.tostring({page = page, nodes = snapshot(box)}), '\n')
  file:close()
end

return M
