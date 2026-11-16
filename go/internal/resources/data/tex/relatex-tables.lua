-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
require('lualibs')
local M, writer = {}, nil
local attr = luatexbase.new_attribute('relatex.table')
local saved, contexts, id = {}, {}, 0
-- The TeX group level each open table context began in: a class that redefines \endtabular at run
-- time (aastex's deluxetable) drops the end hook, and the context closes when its group ends.
local levels = {}
local page_stack = {}
local hlist, vlist, rule, glyph, disc = node.id('hlist'), node.id('vlist'), node.id('rule'), node.id('glyph'), node.id('disc')
local subtypes = node.subtypes(hlist)
local alignment, cell
for key, name in pairs(subtypes) do
 if name == 'alignment' then alignment = key end
 if name == 'cell' then cell = key end
end
function M.enable_modern()
 M.modern = require('relatex-rich-tables')
 M.modern.configure(attr, contexts)
 require('relatex').flow.rich_tables = M.modern
 if writer then M.modern.start(writer) end
end
function M.start(emit)
 writer = emit
 if M.modern then M.modern.start(emit) end
end
function M.begin_table(kind)
 assert(writer, 'Table capture requires active bridge')
 local parent = tex.getattribute(attr) or 0
 -- A rich table inside a rich table's cell is projected first, then read as that cell's
 -- content; any other nesting stays text.
 local parent_cell = M.modern and parent > 0 and M.modern.current_cell(parent)
 if parent > 0 and not parent_cell then writer('diagnostic', {code = 'unsupported_table_layout', reason = 'nested table'}) end
 saved[#saved + 1] = tex.getattribute(attr) or -2147483647
 levels[#saved] = tex.currentgrouplevel
 id = id + 1; contexts[id] = {kind = kind or 'tabular'}; tex.setattribute(attr, id)
 -- The tagging project's header rows in force for this table (\tagpdfsetup{table/header-rows=…},
 -- local to the group that sets it; decision 111): the author's declaration of its header rows.
 local declared = M.modern and token.get_macro('l__tbl_header_rows_clist')
 if declared and declared:match('%d') then
  writer('diagnostic', {code = 'rich_table_declared_header', source = 'native-array-cells', table = id, rows = declared})
  contexts[id].declared = true
 end
 if parent_cell then
  writer('diagnostic', {code = 'rich_table_nesting', source = 'native-array-cells', table = id, parent = parent, cell = parent_cell})
 end
end
function M.array(preamble)
 if M.modern and M.modern.in_cell() then return end
 local current = contexts[tex.getattribute(attr)]
 if current then
  current.preamble = preamble
  local classic = preamble:gsub('|[lcr]', '') == '|'
  current.classic = classic
  current.modern = M.modern ~= nil
  if M.modern then M.modern.core_preamble_begin() end
 end
end
-- The end hook closes the innermost context only if it began in this group or a deeper one: a
-- class that wraps \@tabular in a group of its own (revtex4-1's tabular*) ends that group first,
-- and the group end has closed the context already (third cohort 599). A context from an outer
-- group belongs to an enclosing table.
function M.end_table()
 if #saved == 0 or levels[#saved] < tex.currentgrouplevel then return end
 M.close_table()
end
function M.close_table()
 levels[#saved] = nil
 local prior = assert(table.remove(saved)); tex.setattribute(attr, prior)
end
-- After the group a tabular began in: the contexts of groups that have ended are closed.
function M.end_group()
 while #saved > 0 and levels[#saved] > tex.currentgrouplevel do M.close_table() end
end
local function content(head, runs, style)
 local owner_attr = luatexbase.attributes['relatex.flow.owner']
 local slot_attr = luatexbase.attributes['relatex.flow.slot']
 for n in node.traverse(head) do
  if n.id == glyph then
   -- Classic TFM fonts may have no Lua table (font.getfont); TeX's own record has the size.
   local face = font.getfont(n.font) or (font.getcopy and font.getcopy(n.font))
   local size = face and face.size or 0
   if not style.size then style.size = size elseif style.size ~= size then style.mixed = true end
   local owner, slot = owner_attr and node.has_attribute(n,owner_attr), slot_attr and node.has_attribute(n,slot_attr)
   local prior = runs[#runs]
   if prior and prior.owner == owner and prior.last and slot and prior.last + 1 == slot then prior.last = slot
   else runs[#runs + 1] = {owner = owner, first = slot, last = slot} end
  elseif n.id == hlist or n.id == vlist then content(n.head,runs,style)
  elseif n.id == disc then content(n.replace,runs,style) end
 end
end
local function row(n)
 local result = {width = n.width, height = n.height, depth = n.depth, cells = {}}
 for child in node.traverse(n.head) do
  if child.id == hlist and child.subtype == cell then
   local entry = {width = child.width, height = child.height, depth = child.depth, runs = {}, borders = {}}
   local style = {}
   content(child.head,entry.runs,style)
   entry.font_size = not style.mixed and style.size or 0
   for border in node.traverse(child.head) do
    if border.id == rule and border.width > 0 then
     local x = (border == child.head and 0 or node.rangedimensions(child,child.head,border))
     entry.borders[#entry.borders + 1] = {x = x, width = border.width,
       height = border.height == -1073741824 and child.height or border.height,
       depth = border.depth == -1073741824 and child.depth or border.depth}
    end
   end
   result.cells[#result.cells + 1] = entry
  end
 end
 return result
end
local function table_box(n, table_id)
 local result = {id = table_id, preamble = contexts[table_id].preamble, width = n.width, rows = {}, horizontal_borders = {}}
 for child in node.traverse(n.head) do
  if child.id == hlist and child.subtype == alignment then result.rows[#result.rows + 1] = row(child)
  elseif child.id == rule and child.width > 0 and child.height + child.depth > 0 then
   result.horizontal_borders[#result.horizontal_borders + 1] = {after_row = #result.rows, width = child.width, height = child.height, depth = child.depth}
  end
 end
 return result
end
-- A modern table whose cells no hook tagged (arydshln replaces array's cell internals) keeps
-- its classic alignment evidence.
local function classic_evidence(id)
 return not (contexts[id].modern and M.modern and M.modern.has_cells(id))
end
local function scan(head, tables)
 for n in node.traverse(head) do
  if n.id == vlist and node.has_attribute(n,attr) and classic_evidence(node.has_attribute(n,attr)) then
   local found = table_box(n,node.has_attribute(n,attr))
   if #found.rows > 0 then tables[#tables + 1] = found else scan(n.head,tables) end
  elseif n.id == hlist or n.id == vlist then scan(n.head,tables) end
 end
end
local function presence(head, groups, parent)
 for n in node.traverse(head) do
  local id = node.has_attribute(n,attr)
  if id and classic_evidence(id) and (n.id == glyph or n.id == rule) then
   local count = groups[id] or {glyphs = 0, rules = 0}; groups[id] = count
   if n.id == glyph then count.glyphs = count.glyphs + 1
   elseif n.width > 0 then
    local height = n.height == -1073741824 and parent.height or n.height
    local depth = n.depth == -1073741824 and parent.depth or n.depth
    if height + depth > 0 then count.rules = count.rules + 1 end
   end
  end
  if n.id == hlist or n.id == vlist then presence(n.head,groups,n)
  elseif n.id == disc then presence(n.replace,groups,parent) end
 end
end
function M.capture(box, page)
 if M.modern then M.modern.capture(box, page) end
 local tables, groups = {}, {}; scan(box.head,tables); presence(box.head,groups,box)
 for _, snapshot in ipairs(tables) do
  writer('diagnostic', {code = 'table_snapshot', source = 'native-classic-alignment', page = page, snapshot = snapshot})
 end
 for id, count in pairs(groups) do
  writer('diagnostic', {code = 'table_presence', source = 'native-classic-alignment', page = page,
    table = id, glyphs = count.glyphs, rules = count.rules})
 end
end
function M.finish()
 assert(#saved == 0 and #page_stack == 0, 'Unclosed table context')
 if M.modern then M.modern.finish() end
 writer = nil
end
function M.page_begin()
 page_stack[#page_stack + 1] = tex.getattribute(attr) or -2147483647
 tex.setattribute(attr, -2147483647)
 if M.modern then M.modern.page_begin() end
end
function M.page_end()
 if M.modern then M.modern.page_end() end
 local previous = assert(table.remove(page_stack), 'Unbalanced table page context')
 tex.setattribute(attr, previous)
end
return M
