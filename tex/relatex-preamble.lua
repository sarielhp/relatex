-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Footnote text set in the preamble (PoS's \note{...} before \begin{document}) is typeset
-- before the collector starts: its glyphs ship on the first page with no flow identity.
-- Until then, each paragraph TeX breaks is numbered node by node and a private copy
-- kept. When the collector starts, a copy whose paragraph sits in a footnote insert still
-- waiting for the page is read in a footnote body as the paragraph would have been, and the
-- identities are passed to the shipped nodes by their numbers. The copies are owned here and
-- never reach a TeX list; nothing changes in what TeX typesets.
local M = {}
local slot_attr = luatexbase.new_attribute('relatex.preamble.slot')
local hlist, vlist, disc = node.id('hlist'), node.id('vlist'), node.id('disc')
local recording, slots, copies = os.getenv('RELATEX_LEAN') == nil, 0, {}

local function number(head)
  for n in node.traverse(head) do
    slots = slots + 1
    node.set_attribute(n, slot_attr, slots)
    if n.id == hlist or n.id == vlist then
      number(n.head)
    elseif n.id == disc then
      number(n.pre)
      number(n.post)
      number(n.replace)
    end
  end
end

luatexbase.add_to_callback('pre_linebreak_filter', function(head)
  if recording then
    number(head)
    copies[#copies + 1] = node.copy_list(head)
  end
  return head
end, 'relatex.preamble')

-- Every node of a list by its preamble number.
local function index(head, nodes)
  for n in node.traverse(head) do
    local slot = node.has_attribute(n, slot_attr)
    if slot then nodes[slot] = n end
    if n.id == hlist or n.id == vlist then
      index(n.head, nodes)
    elseif n.id == disc then
      index(n.pre, nodes)
      index(n.post, nodes)
      index(n.replace, nodes)
    end
  end
  return nodes
end

-- The relatex attributes of the copy, set on the shipped node of the same number.
local function transfer(copy, shipped)
  local names = {}
  for name, attribute in pairs(luatexbase.attributes) do
    if name:find('^relatex%.') and attribute ~= slot_attr then names[#names + 1] = attribute end
  end
  for slot, n in pairs(copy) do
    local target = shipped[slot]
    if target then
      for _, attribute in ipairs(names) do
        local value = node.has_attribute(n, attribute)
        if value then node.set_attribute(target, attribute, value) end
      end
    end
  end
end

-- The footnote inserts waiting on the contribution and page lists, in order.
local function footnote_inserts(footins)
  local found = {}
  for _, head in ipairs({tex.lists.page_head, tex.lists.contrib_head}) do
    for n in node.traverse_id(node.id('ins'), head) do
      -- LuaTeX keeps an insert's class in its subtype.
      if n.subtype == footins then found[#found + 1] = n end
    end
  end
  return found
end

-- Called once the collector has started: each preamble footnote paragraph is read in a body of
-- its own, as a title note (its mark is printed in the author line, not by a footnote call).
-- The last one takes the current \@thefnmark; earlier ones carry no number.
function M.finish(footins, mark, collector, notes)
  if not recording then return end
  recording = false
  local inserts = footnote_inserts(footins)
  for position, insert in ipairs(inserts) do
    local shipped = index(insert.list, {})
    for _, copy in ipairs(copies) do
      local nodes = index(copy, {})
      local first = next(nodes)
      if first and shipped[first] then
        notes.title_begin()
        notes.body_begin(position == #inserts and mark or '')
        collector.capture_detached(copy)
        notes.body_end()
        notes.title_end()
        transfer(nodes, shipped)
      end
    end
  end
  for _, copy in ipairs(copies) do node.flush_list(copy) end
  copies = {}
end

-- enumitem's \newlist and \renewlist run in the preamble, before the collector loads: each
-- declaration (name, type) is kept here in order, and the collector observes the named lists
-- once it starts (decision 046, amendment 1). \ltd@declare@list is redefined by the collector,
-- so a declaration after it starts goes to it directly.
M.lists = {}

function M.declare_list(name, base)
  M.lists[#M.lists + 1] = {name=name, base=base}
end

tex.print(luatexbase.registernumber('catcodetable@atletter'), {
  [[\def\ltd@declare@list#1#2{\directlua{ltd_preamble.declare_list('\luaescapestring{#1}','\luaescapestring{#2}')}}]],
  [[\def\ltd@observe@enumitem{\let\ltd@enit@newlist\enit@newlist]],
  [[  \def\enit@newlist##1##2##3##4{\ltd@declare@list{##2}{##3}\ltd@enit@newlist{##1}{##2}{##3}{##4}}}]],
  [[\AddToHook{package/enumitem/after}[relatex-preamble-lists]{\ltd@observe@enumitem}]],
})

return M
