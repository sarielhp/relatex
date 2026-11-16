-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Modern array execution hooks provide cells/spans; native nodes prove placement.
local M = {}
-- font.getfont has no table for many classic TFM fonts (mathbbol, cancel, picture-mode line
-- fonts, Omega encodings); getcopy reads TeX's own record. Never assert: a missing face only
-- leaves that glyph's face unknown.
local faces = {}
local function face_of(id)
  local face = faces[id]
  if face == nil then
    face = font.getfont(id) or (font.getcopy and font.getcopy(id)) or false
    faces[id] = face
  end
  return face or nil
end
local decoder = require('relatex-glyphs')
local cell_attr = luatexbase.new_attribute('relatex.table.cell')
local table_attr, contexts, emit
local cells, serial, stack = {}, 0, {}
local last_cells, sections_seen = {}, {}
-- The cell boxes of the rows matched on the page being captured, each to its rich cell.
local cell_boxes = {}
local core_stack = {}
local page_stack = {}
local effective_glue
-- 2pt: a painted rectangle thicker than this in both directions is an area fill, not a rule.
local fill_thickness = 131072
local hlist, vlist, glyph, rule, disc = node.id('hlist'), node.id('vlist'), node.id('glyph'), node.id('rule'), node.id('disc')
local alignment, cell
for key, name in pairs(node.subtypes(hlist)) do
  if name == 'alignment' then alignment = key elseif name == 'cell' then cell = key end
end
function M.configure(attribute, catalog) table_attr, contexts = attribute, catalog end
function M.start(writer) emit = writer end
function M.begin_cell(row, column, kind, colspan, padding)
  local id = tex.getattribute(table_attr)
  local context = contexts[id]
  if not context or not context.modern then return end
  serial = serial + 1
  local declaration = {id = serial, table = id, row = row, column = column, kind = kind,
    colspan = colspan, rowspan = 1, vertical_align = 'top', padding_sp = padding}
  cells[serial] = declaration
  stack[#stack + 1] = tex.getattribute(cell_attr) or -2147483647
  tex.setattribute(cell_attr, serial)
  -- Tag an existing strut in an otherwise empty cell. This changes no ink/metrics.
  local first = tex.nest.top.head and tex.nest.top.head.next
  if first then node.set_attribute(first, cell_attr, serial) end
  -- A paragraph cell's hook runs inside its still empty box; the cell's own list holds what the
  -- column set before it (the row strut, a | rule, the \tabcolsep glue). An empty paragraph cell
  -- starts its paragraph only at \@finalstrut, after the end hook, so nothing in the box carries
  -- the identity: the first node of the cell's list does, a strut or not (paper 1182: an empty
  -- cell of a later column under a \multirow, holding only its | border).
  -- A lead that carries the enclosing table's cell (the inherited attribute of a table nested in
  -- a cell: lplfitch's \fitchprf, whose empty p{0pt} column holds only its | bar) is this cell's.
  if (kind == 'p' or kind == 'm' or kind == 'b') and not first then
    local parent = tex.nest[tex.nest.ptr - 1]
    local lead = parent and parent.head and parent.head.next
    local marked = lead and cells[node.has_attribute(lead, cell_attr)]
    if lead and (not marked or marked.table ~= id) then node.set_attribute(lead, cell_attr, serial) end
  end
end
function M.end_cell()
  local current = cells[tex.getattribute(cell_attr)]
  if not current or current.table ~= tex.getattribute(table_attr) then return end
  emit('diagnostic', {code = 'rich_table_cell', source = 'native-array-cells', cell = current})
  last_cells[current.table] = current.id
  local previous = assert(table.remove(stack), 'Unbalanced rich cell')
  tex.setattribute(cell_attr, previous)
end
function M.multirow(rows, alignment)
  local current = cells[tex.getattribute(cell_attr)]
  if not current then return end
  current.rowspan, current.vertical_align = rows, ({t='top',c='middle',b='bottom'})[alignment] or alignment
end
function M.identity(n)
  local id = node.has_attribute(n, cell_attr)
  return cells[id] and id or nil
end
-- Whether any cell hook tagged a cell of table `id`: a package that replaces array's cell
-- internals (arydshln) runs none, and such a table keeps its classic alignment evidence.
function M.has_cells(id)
  return last_cells[id] ~= nil
end
function M.table_identity(n)
  local id = node.has_attribute(n, table_attr)
  return contexts[id] and contexts[id].modern and id or nil
end
-- The rich cell of table `parent` being typeset, if any.
function M.current_cell(parent)
  local current = cells[tex.getattribute(cell_attr)]
  if #stack > 0 and current and current.table == parent and contexts[parent] and contexts[parent].modern then return current.id end
end
function M.in_cell()
  local current = cells[tex.getattribute(cell_attr)]
  return #stack > 0 and current and current.table == tex.getattribute(table_attr)
end
function M.core_preamble_begin()
  local context = contexts[tex.getattribute(table_attr)]
  if context and not M.in_cell() then context.core_collecting, context.core_columns = true, 0 end
end
function M.core_column()
  local context = contexts[tex.getattribute(table_attr)]
  if context and context.core_collecting then context.core_columns = context.core_columns + 1 end
  -- A \multicolumn builds its own one-column preamble: the column its pending span belongs to.
  if context and context.core_span then context.core_span_witnessed = true end
end
function M.core_preamble_end()
  local context = contexts[tex.getattribute(table_attr)]
  if context then context.core_collecting = false end
end
-- A bare \multispan omits the template, so no cell of ours begins under it and its span must not reach
-- the next cell that does (a class head row of a deluxetable: papers5/1210 declared its first body cell
-- full-width). Only a span a \multicolumn's own preamble column witnessed is a cell's.
function M.core_span(value)
  local context = contexts[tex.getattribute(table_attr)]
  if context and not M.in_cell() then context.core_span, context.core_span_witnessed = value, false end
end
function M.core_begin(kind, padding)
  local context = contexts[tex.getattribute(table_attr)]
  local active = context and not M.in_cell() and (context.core_columns or 0) > 0
  core_stack[#core_stack + 1] = active or false
  if not active then return end
  local row, column = context.core_row or 1, context.core_column or 1
  local span = context.core_span_witnessed and context.core_span or 1
  context.core_span, context.core_span_witnessed = nil, false
  M.begin_cell(row, column, kind, span, padding)
  column = column + span
  if column > context.core_columns then row, column = row + 1, 1 end
  context.core_row, context.core_column = row, column
end
-- A math array set inside a cell ($\begin{array}{l} +a \ -b \end{array}$) opens no table of its own, so
-- its \ reaches the enclosing table's row end: while one is open the row does not advance (papers4/1066).
function M.nested_array_begin()
  local context = contexts[tex.getattribute(table_attr)]
  if context and M.in_cell() then context.nested_arrays = (context.nested_arrays or 0) + 1 end
end
function M.nested_array_end()
  local context = contexts[tex.getattribute(table_attr)]
  if context and (context.nested_arrays or 0) > 0 then context.nested_arrays = context.nested_arrays - 1 end
end
-- \ ends a row: a row with fewer cells than the preamble starts the next row at column 1.
function M.core_row_end()
  local context = contexts[tex.getattribute(table_attr)]
  if not context or (context.core_columns or 0) == 0 or (context.core_column or 1) == 1 then return end
  if (context.nested_arrays or 0) > 0 then return end
  context.core_row, context.core_column = (context.core_row or 1) + 1, 1
end
-- AASTeX 6 (\tablehead, \zztablehead): the class opens the head with a spacer row of its own
-- (\multicolumn1c{\getDs…}\, which prints nothing) and closes it with a rule; the rows between are the
-- head the author wrote. They are the table's declared header rows (decision 111, amendment 5), unless
-- the tagging project declared some. A longtable keeps its own first head.
local function class_head_context()
  local id = tex.getattribute(table_attr)
  local context = contexts[id]
  if context and context.modern and context.kind ~= "longtable" then return id, context end
end
local function last_row(id) return last_cells[id] and cells[last_cells[id]].row or 0 end
function M.class_head_begin()
  local id, context = class_head_context()
  if id then context.class_head_spacer = last_row(id) + 1 end
end
function M.class_head_end()
  local id, context = class_head_context()
  local spacer = id and context.class_head_spacer
  if not spacer or context.declared then return end
  context.class_head_spacer = nil
  local rows = {}
  for row = spacer + 1, last_row(id) do rows[#rows + 1] = row end
  if #rows == 0 then return end
  context.declared = true
  emit("diagnostic", {code = "rich_table_declared_header", source = "native-array-cells", table = id,
    rows = table.concat(rows, ","), origin = "aastex-tablehead"})
end
function M.core_end()
  if assert(#core_stack > 0, 'Unbalanced kernel table cell') and table.remove(core_stack) then M.end_cell() end
end
local function layout_hyphen(n)
  local attribute = luatexbase.attributes['relatex.discretionary']
  return attribute and node.has_attribute(n, attribute) ~= nil
end
local graphic_item
local function content(head, result, parent, own)
  local owner_attr = luatexbase.attributes['relatex.flow.owner']
  local slot_attr = luatexbase.attributes['relatex.flow.slot']
  for n in node.traverse(head) do
    -- A nested table's nodes are its own cells', not this cell's.
    local foreign = own and (node.has_attribute(n, table_attr) or own) ~= own
    if foreign then
    elseif n.id == glyph or n.id == rule then
      local height = n.height == -1073741824 and parent.height or n.height
      local depth = n.depth == -1073741824 and parent.depth or n.depth
      local width = n.width == -1073741824 and parent.width or n.width
      local visible = n.id == glyph or (width > 0 and height + depth > 0)
      if visible then
        if n.id == glyph then result.glyphs = result.glyphs + 1 else result.rules = result.rules + 1 end
        if n.id == glyph then
          local face = face_of(n.font)
          local size = face and face.size or 0
          if result.font_size and result.font_size ~= size then result.mixed_font_size = true end
          result.font_size = result.font_size or size
        end
        local owner, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
        if owner and slot then
          local previous = result.runs[#result.runs]
          if previous and previous.owner == owner and previous.last + 1 == slot then previous.last = slot
          elseif not previous or previous.owner ~= owner or previous.last ~= slot then
            result.runs[#result.runs + 1] = {owner = owner, first = slot, last = slot}
          end
        elseif n.id == rule then result.unowned_rules = result.unowned_rules + 1
        -- The hyphen of a word broken across a p-cell's lines is layout, not text.
        elseif layout_hyphen(n) then result.layout_glyphs = (result.layout_glyphs or 0) + 1
        else result.unowned_glyphs = result.unowned_glyphs + 1
        end
      end
    elseif n.id == node.id('glue') and n.leader and n.leader.id == rule and parent.id == hlist then
      local leader = n.leader
      local height = leader.height == -1073741824 and parent.height or leader.height
      local depth = leader.depth == -1073741824 and parent.depth or leader.depth
      local width = effective_glue(n, parent)
      if width > 0 and height + depth > 0 then
        result.rules, result.unowned_rules = result.rules + 1, result.unowned_rules + 1
        -- colortbl's \rowcolor and \cellcolor fill a cell with a leader as wide as the cell and
        -- as tall as the row: an area, thicker than any table rule in both directions.
        if width > fill_thickness and height + depth > fill_thickness then result.fills = (result.fills or 0) + 1 end
      end
    end
    if (n.id == hlist or n.id == vlist) and not (not foreign and graphic_item(n, result, own)) then
      content(n.head, result, n, own)
    elseif n.id == disc then content(n.replace, result, parent, own) end
  end
end
-- A picture without an inspectable source (a TikZ picture) is one flow item, its box: its
-- labels are the figure's drawing, not the cell's text (third cohort 593, 776). Its ink stays
-- in the cell's paint counts and is covered by the picture's one slot.
graphic_item = function(n, result, own)
  local graphic_attr = luatexbase.attributes['relatex.graphic.unsupported']
  local owner_attr = luatexbase.attributes['relatex.flow.owner']
  local slot_attr = luatexbase.attributes['relatex.flow.slot']
  if not (graphic_attr and node.has_attribute(n, graphic_attr)) then return false end
  local owner, slot = node.has_attribute(n, owner_attr), node.has_attribute(n, slot_attr)
  if not (owner and slot) then return false end
  local ink = {runs = {}, glyphs = 0, rules = 0, unowned_rules = 0, unowned_glyphs = 0}
  content(n.head, ink, n, own)
  result.glyphs, result.rules = result.glyphs + ink.glyphs, result.rules + ink.rules
  result.graphic_ink = (result.graphic_ink or 0) + ink.glyphs + ink.rules
  result.graphics = (result.graphics or 0) + 1
  result.runs[#result.runs + 1] = {owner = owner, first = slot, last = slot}
  return true
end
local function identities(head, result, glyph_only, own)
  for n in node.traverse(head) do
    local id = node.has_attribute(n, cell_attr)
    if cells[id] and (not glyph_only or n.id == glyph) and (not own or cells[id].table == own) then result[id] = true end
    if n.id == hlist or n.id == vlist then identities(n.head, result, glyph_only, own)
    elseif n.id == disc then identities(n.replace, result, glyph_only, own) end
  end
end
function M.sections(boxes)
  for role, box in pairs(boxes) do
    if box then
      local found, grouped = {}, {}
      identities(box.head, found)
      for id in pairs(found) do
        local table_id = cells[id].table
        grouped[table_id] = grouped[table_id] or {}
        grouped[table_id][#grouped[table_id] + 1] = id
      end
      for table_id, ids in pairs(grouped) do
        table.sort(ids)
        local key = table_id .. ':' .. role .. ':' .. table.concat(ids, ',')
        if not sections_seen[key] then
          sections_seen[key] = true
          emit('diagnostic', {code = 'rich_table_section', source = 'native-array-cells', table = table_id, role = role, cells = ids})
        end
      end
    end
  end
end
function M.caption()
  local id = tex.getattribute(table_attr)
  if not contexts[id] or contexts[id].kind ~= 'longtable' or not last_cells[id] then return end
  emit('diagnostic', {code = 'rich_table_caption', source = 'native-array-cells', table = id,
    cell = last_cells[id], anchor = token.get_macro('@currentHref') or ''})
end
-- One identified physical cell box of a row: its entry and declaration, or nil and why the row is
-- no table row.
local function cell_entry(result, n, selected, left, column)
  local declaration = cells[selected]
  if declaration.column ~= column then return nil, 'declared column differs' end
  local context = contexts[declaration.table]
  if not context.modern then return nil, 'not a rich table' end
  if result.table and result.table ~= declaration.table then return nil, 'cells of two tables' end
  result.table, result.logical_row = declaration.table, declaration.row
  result.preamble = context.preamble
  local entry = {cell = selected, left = left, width = n.width, height = n.height, depth = n.depth,
    runs = {}, borders = {}, glyphs = 0, rules = 0, unowned_rules = 0, unowned_glyphs = 0}
  content(n.head, entry, n, declaration.table)
  entry.font_size = not entry.mixed_font_size and entry.font_size or 0
  entry.mixed_font_size = nil
  for border in node.traverse(n.head) do
    if border.id == rule and border.width > 0 then
      entry.borders[#entry.borders + 1] = {x = border == n.head and 0 or node.rangedimensions(n,n.head,border),
        width = border.width, height = border.height == -1073741824 and n.height or border.height,
        depth = border.depth == -1073741824 and n.depth or border.depth}
    end
  end
  result.cells[#result.cells + 1] = entry
  cell_boxes[node.direct.todirect(n)] = selected
  return entry, declaration
end
-- The one declared cell at (table, row, column) that no box of the row has claimed, or nil when
-- there is none or more than one.
local function declared_cell(table_id, row, column, result)
  local claimed, found = {}, nil
  for _, entry in ipairs(result.cells) do claimed[entry.cell] = true end
  for id, declaration in pairs(cells) do
    if declaration.table == table_id and declaration.row == row and declaration.column == column
        and not claimed[id] then
      if found then return nil end
      found = id
    end
  end
  return found
end
-- The empty bordered cells of a row, identified by position once its other cells named the row.
local function resolve_pending(result, pending)
  for _, box in ipairs(pending) do
    local selected = result.table and declared_cell(result.table, result.logical_row, box.column, result)
    if not selected then return nil, 'cell without identity (0 glyphs, ' .. box.rules .. ' rules)' end
    local entry, declaration = cell_entry(result, box.n, selected, box.left, box.column)
    if not entry then return nil, declaration end
    if declaration.colspan ~= 1 then return nil, 'unidentified spanning cell' end
  end
  if #pending > 0 then table.sort(result.cells, function(a, b) return a.left < b.left end) end
  return true
end
local function row_snapshot(row, page)
  local result = {page = page, width = row.width, height = row.height, depth = row.depth, cells = {}}
  local own = node.has_attribute(row, table_attr)
  local left, column, remaining, spanning, pending = 0, 0, 0, nil, {}
  for n in node.traverse(row.head) do
    if n.id == hlist and n.subtype == cell then
      column = column + 1
      if remaining > 0 then
        -- TeX retains positive-width, empty physical column boxes underneath a
        -- multicolumn span. Account for their width in its one logical anchor;
        -- never skip an occupied or nonempty node list.
        if n.head or n.height ~= 0 or n.depth ~= 0 then return nil, 'occupied spanned column' end
        spanning.width, remaining = spanning.width + n.width, remaining - 1
      else
      local ids, selected, count = {}, nil, 0
      -- Spanned cells can retain inherited IDs on non-ink alignment glue.
      -- Actual glyph ownership takes precedence; empty cells use the existing
      -- first-node marker assigned by begin_cell, then the full fallback scan.
      identities(n.head, ids, true, own)
      -- The marker is a cell of this row's own table: in a nested table's cell, colortbl sets its
      -- colour node first, while the enclosing table's cell is current (book 0003: a picture cell
      -- read as the outer cell's line, its rows silently dropped).
      if not next(ids) then
        local marker = n.head and node.has_attribute(n.head, cell_attr)
        if cells[marker] and (not own or cells[marker].table == own) then ids[marker] = true
        else identities(n.head, ids, false, own) end
      end
      for id in pairs(ids) do selected, count = id, count + 1 end
      -- An empty cell (under a \multirow from above) has nothing to carry its identity;
      -- it paints nothing but perhaps its colour fill, and the reader fills it as an empty cell.
      local blank = {runs = {}, glyphs = 0, rules = 0, unowned_rules = 0, unowned_glyphs = 0}
      if count == 0 then content(n.head, blank, n, own) end
      if count == 0 and blank.glyphs == 0 and blank.rules == (blank.fills or 0) then
        if blank.rules > 0 then result.blank_fills = (result.blank_fills or 0) + blank.rules end
      elseif count == 0 and blank.glyphs == 0 then
        -- An empty cell painting only its | border (under a \multirow, with colortbl, whose colour
        -- nodes take the place the identity marker needs): its declaration is found by position
        -- once the row's other cells name the row (papers5/1224, papers5/1480, papers3/0797).
        pending[#pending + 1] = {n = n, left = left, column = column, rules = blank.rules}
      elseif count ~= 1 then return nil, count == 0 and ('cell without identity (' .. blank.glyphs .. ' glyphs, ' .. blank.rules .. ' rules)') or 'cell with several identities'
      else
      local entry, declaration = cell_entry(result, n, selected, left, column)
      if not entry then return nil, declaration end
      spanning, remaining = entry, declaration.colspan - 1
      end
      end
      left = left + n.width
    elseif n.id == node.id('glue') then
      local width = n.width
      if n.stretch_order == row.glue_order and row.glue_sign == 1 then width = width + n.stretch * row.glue_set end
      if n.shrink_order == row.glue_order and row.glue_sign == 2 then width = width - n.shrink * row.glue_set end
      if remaining > 0 then spanning.width = spanning.width + width end
      left = left + width
    elseif n.id == node.id('kern') then
      if remaining > 0 then spanning.width = spanning.width + n.kern end
      left = left + n.kern
    end
  end
  if remaining ~= 0 then return nil, 'span past the row end' end
  local resolved, reason = resolve_pending(result, pending)
  if not resolved then return nil, reason end
  return #result.cells > 0 and result or nil
end
-- enclosing is the table of the row being scanned inside, if any.
local function scan(head, page, matched, enclosing)
  for n in node.traverse(head) do
    if n.id == hlist and n.subtype == alignment then
      local snapshot, reason = row_snapshot(n, page)
      if snapshot and snapshot.table == enclosing then
        -- An alignment inside a cell that is no table of its own (\shortstack's \halign) is
        -- that cell's content: its lines carry the cell's identity but are no rows (third
        -- cohort 843 read "Case" and "Study" twice). Its lines are no unmatched rows either.
        scan(n.head, page, matched, enclosing)
      elseif snapshot then
        matched[node.direct.todirect(n)] = snapshot
        -- A table nested in a cell has rows of its own.
        scan(n.head, page, matched, snapshot.table)
      else
        local table_id = node.has_attribute(n, table_attr)
        if contexts[table_id] and contexts[table_id].modern and table_id ~= enclosing then
          local entries = {}
          for child in node.traverse(n.head) do
            if child.id == hlist or child.id == vlist then
              local all, ink, ids, glyph_ids = {}, {}, {}, {}
              identities(child.head, all); identities(child.head, ink, true)
              for id in pairs(all) do ids[#ids + 1] = id end
              for id in pairs(ink) do glyph_ids[#glyph_ids + 1] = id end
              table.sort(ids); table.sort(glyph_ids)
              entries[#entries + 1] = {kind = node.type(child.id), subtype = child.subtype,
                width = child.width, height = child.height, depth = child.depth, cells = ids, glyph_cells = glyph_ids}
            end
          end
          emit('diagnostic', {code = 'rich_table_unmatched_row', source = 'native-array-cells',
            table = table_id, page = page, cells = entries, reason = reason})
        end
      end
    elseif n.id == hlist or n.id == vlist then scan(n.head, page, matched, enclosing) end
  end
end
local function extent(n, parent)
  if n.id ~= glyph and n.id ~= rule and n.id ~= hlist and n.id ~= vlist then return 0, 0 end
  local height = n.height == -1073741824 and parent.height or n.height
  local depth = n.depth == -1073741824 and parent.depth or n.depth
  return height, depth
end
effective_glue = function(n, parent)
  local width = n.width
  if n.stretch_order == parent.glue_order and parent.glue_sign == 1 then width = width + n.stretch * parent.glue_set end
  if n.shrink_order == parent.glue_order and parent.glue_sign == 2 then width = width - n.shrink * parent.glue_set end
  return width
end
-- A dashed rule (arydshln's \hdashline) is a box leader holding glue and dash rules: the table
-- reads it as the solid rule it dashes, whose height is the dashes'.
local function dashed_rule(leader)
  if leader.id ~= hlist then return false end
  local dashes = 0
  for item in node.traverse(leader.head) do
    if item.id == rule and item.subtype == 0 then dashes = dashes + 1
    elseif item.id ~= node.id('glue') then return false end
  end
  return dashes > 0
end
-- A leader box that paints glyphs and no rule, at any depth.
local function leader_ink(head, found)
  for item in node.traverse(head) do
    if item.id == rule then found.rules = true
    elseif item.id == glyph then found.glyphs = true
    elseif item.id == hlist or item.id == vlist then leader_ink(item.head, found) end
  end
  return found
end
local function glyph_leader(box)
  local found = leader_ink(box.head, {})
  return found.glyphs and not found.rules
end
local function leader_paint(n, parent, x, y, state, in_row, table_id, cell_id)
  local leader = n.leader
  -- An extensible arrow (\xrightarrow, \overrightarrow) repeats a box of its bar glyph with
  -- \cleaders, \dotfill a box of a dot: repeated glyphs that neither a cell's content nor
  -- the table's paint counts. The arrow is the formula's own drawing (MathML).
  if leader.id == hlist and glyph_leader(leader) then return end
  if parent.id ~= hlist or not (dashed_rule(leader) or (leader.id == rule and leader.subtype == 0)) then
    emit('diagnostic', {code = 'unsupported_table_layout', source = 'native-array-cells',
      table = table_id, reason = 'unsupported-native-leader'})
    return
  end
  local width = effective_glue(n, parent)
  local height, depth = extent(leader, parent)
  if width <= 0 or height + depth <= 0 then return end
  in_row = in_row == table_id
  local tally = state.counts[table_id] or {glyphs = 0, rules = 0, row_glyphs = 0, row_rules = 0}
  state.counts[table_id] = tally
  tally.rules = tally.rules + 1
  if in_row then tally.row_rules = tally.row_rules + 1 end
  -- colortbl's fill of a cell of this table (\cellcolor, \rowcolor, \columncolor, \rowcolors):
  -- a \vrule leader with running height and depth (as tall as the row) as wide as the cell, in the
  -- colour current where it is painted. soul's \hl highlights a word with a leader of a fixed height
  -- and depth: it paints behind that word, not the cell (slides 0018).
  local running = -1073741824
  local fill, color
  if in_row and cell_id and cells[cell_id].table == table_id and width > fill_thickness and height + depth > fill_thickness
      and leader.id == rule and leader.height == running and leader.depth == running then
    fill = cell_id
    color = state.colors and require('relatex').flow.color_hex(state.colors[#state.colors]) or nil
  end
  emit('diagnostic', {code = 'rich_table_rule', source = 'native-array-cells', table = table_id, page = state.page,
    rect_sp = {x, y + parent.height - height, width, height + depth},
    native_leader = true, glue_subtype = n.subtype,
    width = leader.width, height = leader.height, depth = leader.depth,
    box_height = parent.height, box_depth = parent.depth, box_kind = node.type(parent.id), in_row = not not in_row, math = false,
    cell = color and fill or nil, color = color})
end
local rule_names = node.subtypes('rule')
local function paint(head, parent, origin_x, origin_y, state, in_row, cell_id)
  local cursor, vertical = 0, parent.id == vlist
  for n in node.traverse(head) do
    local x, y = origin_x + (vertical and 0 or cursor), origin_y + (vertical and cursor or 0)
    local id = node.has_attribute(n, table_attr) or in_row
    -- A node is in a row of its own table only: a nested table's \hline lies in a row of the
    -- table around it but between its own rows (fourth cohort 1086).
    local own_row = in_row and in_row == id
    local modern = contexts[id] and contexts[id].modern
    if state.colors then require('relatex').flow.color_step(state.colors, n) end
    local height, depth = extent(n, parent)
    local width = n.id == rule and n.width == -1073741824 and parent.width or n.width
    if modern and n.id == node.id('glue') and n.leader then leader_paint(n, parent, x, y, state, in_row, id, cell_id) end
    if modern and (n.id == glyph or (n.id == rule and width > 0 and height + depth > 0)) then
      local tally = state.counts[id] or {glyphs = 0, rules = 0, row_glyphs = 0, row_rules = 0}
      state.counts[id] = tally
      local key = n.id == glyph and 'glyphs' or 'rules'
      tally[key] = tally[key] + 1
      if own_row then tally['row_' .. key] = tally['row_' .. key] + 1 end
      local formula = luatexbase.attributes['relatex.math.formula']
      if n.id == glyph then
        local face = face_of(n.font) or {}
        local glyphs = state.glyphs[id] or {}; state.glyphs[id] = glyphs
        local info = face.characters and face.characters[n.char] or {}
        local text = decoder.character(n)
        local slot = face.format == 'unknown' and not info.index and n.char >= 0 and n.char < 256 and n.char or nil
        glyphs[#glyphs + 1] = {text = text or '[unmapped]', char = n.char, glyph_name = info.name, slot = slot,
          baseline_sp = {x + n.xoffset, y + parent.height - n.yoffset},
          width_sp = n.width * (1 + (n.expansion_factor or 0) / 1000000),
          font = face.psname or face.fullname or face.name, size_sp = face.size,
          cell = node.has_attribute(n, cell_attr)}
        if not text then
          emit('diagnostic', {code = 'rich_table_glyph_identity', source = 'native-array-cells', table = id,
            font = face.name, filename = face.filename, format = face.format, char = n.char,
            glyph_name = info.name, glyph_index = info.index})
        end
      end
      -- An included graphic is a rule node of its own subtype: it is an image, not rule paint.
      if n.id == rule and not (rule_names[n.subtype] == 'image' or rule_names[n.subtype] == 'box') then
        local top = vertical and y or y + parent.height - height
        local owner = node.has_attribute(n, luatexbase.attributes['relatex.flow.owner'])
        local slot = node.has_attribute(n, luatexbase.attributes['relatex.flow.slot'])
        emit('diagnostic', {code = 'rich_table_rule', source = 'native-array-cells', table = id, page = state.page,
          rect_sp = {x, top, width, height + depth}, width = n.width, height = n.height, depth = n.depth,
          box_height = parent.height, box_depth = parent.depth, box_kind = node.type(parent.id), in_row = not not own_row,
          owner = owner, slot = slot, math = node.has_attribute(n, formula) ~= nil})
      end
    end
    if n.id == hlist or n.id == vlist then
      local child_x = x + (vertical and n.shift or 0)
      local child_y = y + (vertical and 0 or parent.height - n.height + n.shift)
      local row = state.matched[node.direct.todirect(n)]
      if row then row.origin_sp = {child_x, child_y} end
      paint(n.head, n, child_x, child_y, state, row and row.table or in_row, cell_boxes[node.direct.todirect(n)] or cell_id)
    elseif n.id == disc then
      -- A retained discretionary replacement has real horizontal advance (for
      -- example the hyphens in authored case labels). Its glyphs and its width
      -- must both participate in the parent list's coordinates.
      cursor = cursor + paint(n.replace, parent, x, y, state, in_row, cell_id)
    end
    if n.id == node.id('glue') then cursor = cursor + effective_glue(n, parent)
    elseif n.id == node.id('kern') then cursor = cursor + n.kern
    -- microtype's protrusion: a margin kern hangs a line's first glyph out ('(' or 'w' starting a
    -- line in a p cell, papers2 0309), so the glyphs after it lie where the PDF paints them.
    elseif n.id == node.id('margin_kern') then cursor = cursor + n.width
    elseif n.id == hlist or n.id == vlist or n.id == glyph or n.id == rule then
      if n.id == glyph then width = width * (1 + (n.expansion_factor or 0) / 1000000) end
      cursor = cursor + (vertical and height + depth or width)
    end
  end
  return cursor
end
function M.capture(box, page)
  -- The page's colour stack as it starts (the shipout walk follows it after this capture).
  local flow = require('relatex').flow
  local state = {page = page, counts = {}, matched = {}, glyphs = {}, colors = flow and flow.color_stack()}
  cell_boxes = {}
  scan(box.head, page, state.matched)
  paint(box.head, box, 0, 0, state, false)
  local rows = {}
  for _, row in pairs(state.matched) do rows[#rows + 1] = row end
  table.sort(rows, function(a,b) return a.origin_sp[2] < b.origin_sp[2] end)
  for _, row in ipairs(rows) do emit('diagnostic', {code = 'rich_table_row', source = 'native-array-cells', snapshot = row}) end
  for id, counts in pairs(state.counts) do
    local glyphs = state.glyphs[id] or {}
    for first = 1, #glyphs, 128 do
      local chunk = {}
      for index = first, math.min(first + 127, #glyphs) do chunk[#chunk + 1] = glyphs[index] end
      emit('diagnostic', {code = 'rich_table_glyphs', source = 'native-array-cells', table = id, page = page, glyphs = chunk})
    end
    emit('diagnostic', {code = 'rich_table_presence', source = 'native-array-cells', table = id, page = page, counts = counts})
  end
end
function M.finish()
  assert(#stack == 0 and #core_stack == 0 and #page_stack == 0, 'Unclosed native rich cell')
  emit = nil
end
function M.page_begin()
  page_stack[#page_stack + 1] = tex.getattribute(cell_attr) or -2147483647
  tex.setattribute(cell_attr, -2147483647)
end
function M.page_end()
  local previous = assert(table.remove(page_stack), 'Unbalanced rich table page context')
  tex.setattribute(cell_attr, previous)
end
return M
