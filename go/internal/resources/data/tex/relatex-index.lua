-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- The linked index (decision 084): where each \index entry was issued, and which paragraphs
-- typeset each entry of the index. Nothing here adds a node or changes the page.
--
-- An entry's write node (\protected@write of \indexentry, or memoir's line in the .aux) is made
-- while the issue attribute holds the entry's serial; when the page ships, the node is found
-- with the page and the flow glyph it follows on its line (else the next one), so the export can
-- set an anchor in that paragraph. Inside theindex each \item, \subitem and \subsubitem marks the
-- glyphs of its entry, and the entry's paragraphs are reported by flow owner with the .ind the
-- environment reads.
local M = {}
local issue_attr = luatexbase.new_attribute('relatex.index.issue')
local item_attr = luatexbase.new_attribute('relatex.index.item')
local glyph, hlist, vlist, disc = node.id('glyph'), node.id('hlist'), node.id('vlist'), node.id('disc')
local whatsit = node.id('whatsit')
local write_subtype = node.subtype('write')
local emit
local keys, serial, items, shipped = {}, 0, 0, 0
local item_owners = {}

local function attribute(name)
  return luatexbase.attributes and luatexbase.attributes[name]
end

function M.start(writer) emit = writer end

-- An \index call's key as its write expands it.
local pending_key = ''
-- The deferred write expands \protect to nothing (a robust command prints as its name).
function M.key(key) pending_key = key:gsub("\\protect ", "") end

-- An \index call: the key just read and the index it goes to.
function M.issue(name)
  local key = pending_key
  serial = serial + 1
  keys[serial] = {key = key, name = name}
  tex.setattribute(issue_attr, serial)
end

-- theindex has begun: the .ind it reads is the file being input.
function M.begin_index()
  if not emit then return end
  local file = status.filename
  local handle = file and io.open(file, 'rb')
  local text = handle and handle:read('a')
  if handle then handle:close() end
  if not text then
    emit('diagnostic', {code = 'index_source', file = file or '', missing = true})
    return
  end
  local size = 900000
  local parts = math.max(1, math.ceil(#text / size))
  for part = 1, parts do
    emit('diagnostic', {code = 'index_source', file = file, part = part, parts = parts,
      text = text:sub((part - 1) * size + 1, part * size)})
  end
end

-- An entry of the typeset index: \item (1), \subitem (2), \subsubitem (3).
function M.item(level)
  if not emit then return end
  items = items + 1
  tex.setattribute(item_attr, items)
  emit('diagnostic', {code = 'index_item', item = items, level = level})
end

-- Anchors bind to the glyphs a line holds itself (top): a glyph inside a box in the line belongs to
-- the box's own flow owner, which no paragraph of the reading names.
local function scan(head, state, line, top, owner_attr, slot_attr, decoration_attr)
  for n in node.traverse(head) do
    if n.id == glyph then
      local owner = owner_attr and node.has_attribute(n, owner_attr)
      if owner and not (decoration_attr and node.has_attribute(n, decoration_attr)) then
        if top then
          local where = {owner = owner, slot = node.has_attribute(n, slot_attr)}
          for _, anchor in ipairs(state.pending) do anchor.owner, anchor.slot, anchor.side = where.owner, where.slot, 'before' end
          state.pending = {}
          if line then line.last = where end
          state.last = where
        end
        local item = node.has_attribute(n, item_attr)
        if item then
          local key = item .. ':' .. owner
          if not item_owners[key] then
            item_owners[key] = true
            emit('diagnostic', {code = 'index_item_owner', item = item, owner = owner})
          end
        end
      end
    elseif n.id == whatsit and n.subtype == write_subtype then
      local issued = node.has_attribute(n, issue_attr)
      if issued and keys[issued] then
        shipped = shipped + 1
        local anchor = {anchor = shipped, serial = issued, key = keys[issued].key, name = keys[issued].name}
        state.anchors[#state.anchors + 1] = anchor
        if line and line.last then
          anchor.owner, anchor.slot, anchor.side = line.last.owner, line.last.slot, 'after'
        else
          state.pending[#state.pending + 1] = anchor
        end
      end
    elseif n.id == hlist and n.head then
      scan(n.head, state, line or {}, line == nil, owner_attr, slot_attr, decoration_attr)
    elseif n.id == vlist and n.head then
      scan(n.head, state, nil, true, owner_attr, slot_attr, decoration_attr)
    elseif n.id == disc and n.replace then
      scan(n.replace, state, line, top, owner_attr, slot_attr, decoration_attr)
    end
  end
end

-- A page ships: its index anchors, each bound to the flow glyph it follows (or precedes).
function M.shipout(head, page, printed)
  if not emit or serial == 0 and items == 0 then return end
  local state = {pending = {}, anchors = {}}
  scan(head, state, nil, true, attribute('relatex.flow.owner'), attribute('relatex.flow.slot'), attribute('relatex.page_decoration'))
  for _, anchor in ipairs(state.pending) do
    if state.last then anchor.owner, anchor.slot, anchor.side = state.last.owner, state.last.slot, 'after' end
  end
  for _, anchor in ipairs(state.anchors) do
    anchor.page, anchor.printed, anchor.code = page, type(printed) == 'string' and printed or nil, 'index_anchor'
    emit('diagnostic', anchor)
  end
end

return M
