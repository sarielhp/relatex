-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Observe explicit LaTeX list lifetimes without replacing list commands.
local M = {}
local core, emit
local frames = {}
local roles = {itemize='unordered_list', enumerate='ordered_list', description='unordered_list'}
local romans = {'i', 'ii', 'iii', 'iv', 'v', 'vi', 'vii', 'viii', 'ix', 'x'}

-- A list environment under another name (decision 046, amendment 1): enumitem's
-- \newlist{compactenumA}{enumerate}{5} or paralist's compactenum is a list of its base kind.
-- An inline base (enumerate*) runs its items into the paragraph: such a name has no role.
-- counter names the family whose counters number its items: 'enum' for LaTeX's enumi..enumiv
-- (enumerate and paralist's compactenum), the list's own name for enumitem's compactenumAi.
local aliases = {}

function M.install(collector) core = collector end
function M.start(writer) emit = writer end

function M.alias(name, base, counter)
  aliases[name] = {base=base, counter=counter or name}
end

local function base_of(name)
  if roles[name] then return name end
  local alias = aliases[name]
  return alias and roles[alias.base] and alias.base or nil
end

local function counter_family(name)
  if name == 'enumerate' then return 'enum' end
  return aliases[name] and aliases[name].counter or name
end

function M.begin(kind, depth)
  local base = base_of(kind)
  frames[#frames + 1] = {kind=kind, base=base, depth=depth + 1, item=false}
  if base then core.enter(roles[base]) end
end

-- The counter of an ordered item: the family's name and the item's depth among the open lists
-- of the same family (enumii in an enumerate inside an enumerate).
local function item_counter(frame)
  local family, level = counter_family(frame.kind), 0
  for _, open in ipairs(frames) do
    if open.base == 'enumerate' and counter_family(open.kind) == family then level = level + 1 end
  end
  return romans[level] and family .. romans[level]
end

function M.item(kind, depth)
  local frame = frames[#frames]
  if not frame then return end
  -- center, quote, flushleft and theorems are trivlists: their internal \item is no list item;
  -- nor is an item of an inline list.
  if not base_of(kind) or not frame.base then return end
  if kind ~= frame.kind or depth ~= frame.depth then
    emit('diagnostic', {code='unsupported_list_context', source='latex-list-hooks',
      environment=kind, depth=depth, expected_environment=frame.kind, expected_depth=frame.depth})
    return
  end
  if frame.item then core.leave('list_item') end
  core.enter('list_item')
  frame.item = true
  if frame.base == 'enumerate' then
    local counter = item_counter(frame)
    if counter then core.name_context(counter) end
  end
end

function M.finish(kind)
  local frame = assert(table.remove(frames), 'Unbalanced list observation')
  assert(frame.kind == kind, 'Mismatched list observation')
  if frame.item then core.leave('list_item') end
  if frame.base then core.leave(roles[frame.base]) end
end

return M
