-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Native Natbib execution and retained logical slots; no PDF/node rewriting.
local M = {}
local citation_attr = luatexbase.new_attribute('relatex.citation')
local link_attr = luatexbase.new_attribute('relatex.citation.link')
local entry_attr = luatexbase.new_attribute('relatex.bibliography.entry')
local emit, flow
local calls, links, entries, bibliographies = 0, 0, 0, 0
local call_stack, link_stack, bibliography_stack = {}, {}, {}
local unset = -2147483647
local profile = 'natbib-command-hooks'

function M.profile(source)
  assert(not emit and (source == 'natbib-command-hooks' or source == 'latex-command-hooks'
    or source == 'biblatex-command-hooks'), 'Invalid citation observer profile')
  profile = source
end

local function record(code, fields)
  fields.code, fields.source = code, profile
  emit('diagnostic', fields)
end

function M.start(writer, observer) emit, flow = writer, observer end

function M.begin(keys)
  calls = calls + 1
  call_stack[#call_stack + 1] = {id=calls, old=tex.getattribute(citation_attr)}
  tex.setattribute(citation_attr, calls)
  record('citation_call', {citation=calls, requested=keys})
end

-- A citation command that calls the citex body directly (natbib's \citenum). Inside an observed
-- call the body belongs to that call: it is not a second one.
function M.begin_direct(keys)
  local call = call_stack[#call_stack]
  if call then
    call_stack[#call_stack + 1] = {id=call.id, nested=true}
    return
  end
  M.begin(keys)
end

function M.sorted(keys)
  local call = call_stack[#call_stack]
  if call then record('citation_sorted', {citation=call.id, keys=keys}) end
end

function M.link_begin(key, number)
  local call = call_stack[#call_stack]
  if not call then
    link_stack[#link_stack + 1] = false
    record('unsupported_citation_hook_context', {key=key})
    return
  end
  links = links + 1
  link_stack[#link_stack + 1] = {id=links, old=tex.getattribute(link_attr)}
  tex.setattribute(link_attr, links)
  record('citation_link', {citation=call.id, citation_link=links, key=key, number=number or token.get_macro('NAT@num')})
end

-- A link whose target cannot be identified: its text prints unlinked, recorded by key.
function M.unidentified_link(key, reason)
  link_stack[#link_stack + 1] = false
  record('unsupported_citation_link', {key=key, reason=reason})
end

function M.link_end()
  local link = table.remove(link_stack)
  assert(link ~= nil, 'Unbalanced citation hyperlink')
  if link then
    tex.setattribute(link_attr, link.old or unset)
    record('citation_link_end', {citation_link=link.id})
  end
end

function M.finish_call()
  local call = assert(table.remove(call_stack), 'Unbalanced citation call')
  if call.nested then return end
  tex.setattribute(citation_attr, call.old or unset)
  record('citation_end', {citation=call.id})
end

function M.bibliography_begin()
  bibliographies = bibliographies + 1
  bibliography_stack[#bibliography_stack + 1] = {id=bibliographies, old=tex.getattribute(entry_attr)}
  tex.setattribute(entry_attr, unset)
  record('bibliography_begin', {bibliography=bibliographies})
end

-- Open an entry scope before the item's label is typeset (biblatex prints the label
-- before the item driver runs).
function M.reserve()
  local bibliography = bibliography_stack[#bibliography_stack]
  if not bibliography then
    record('unsupported_bibliography_container', {key=''})
    return
  end
  if bibliography.entry then record('bibliography_entry_end', {entry=bibliography.entry}) end
  entries = entries + 1
  bibliography.entry = entries
  bibliography.reserved = true
  tex.setattribute(entry_attr, entries)
end

-- A reserved scope whose item printed nothing is dropped.
function M.reserve_end()
  local bibliography = bibliography_stack[#bibliography_stack]
  if bibliography and bibliography.reserved then
    bibliography.reserved = nil
    tex.setattribute(entry_attr, unset)
  end
end

function M.entry(key)
  local bibliography = bibliography_stack[#bibliography_stack]
  if not bibliography then
    -- A class bibliography container (such as a references environment) is not observed:
    -- report it for the reader to reject, and never stop the ordinary build.
    record('unsupported_bibliography_container', {key=key})
    return
  end
  if bibliography.entry and not bibliography.reserved then
    record('bibliography_entry_end', {entry=bibliography.entry})
  end
  if not bibliography.reserved then
    entries = entries + 1
    bibliography.entry = entries
    tex.setattribute(entry_attr, entries)
  end
  bibliography.reserved = nil
  record('bibliography_entry', {entry=bibliography.entry, bibliography=bibliography.id, key=key})
end

function M.anchor(key, number)
  local bibliography = bibliography_stack[#bibliography_stack]
  -- An item outside an observed bibliography was already reported by M.entry.
  if not bibliography then return end
  assert(bibliography.entry, 'Bibliography anchor without item')
  bibliography.anchored = bibliography.entry
  record('bibliography_anchor', {entry=bibliography.entry, key=key, number=number})
end

-- A class may print item labels without \@biblabel (jpconf: \list{[\arabic{enumi}]}).
-- After \@bibitem the list counter holds the number LaTeX writes through \bibcite.
function M.fallback_anchor(key, number)
  local bibliography = bibliography_stack[#bibliography_stack]
  if bibliography and bibliography.entry and bibliography.anchored ~= bibliography.entry then
    M.anchor(key, number)
  end
end

function M.bibliography_end()
  local bibliography = assert(table.remove(bibliography_stack), 'Unbalanced bibliography')
  if bibliography.entry then record('bibliography_entry_end', {entry=bibliography.entry}) end
  tex.setattribute(entry_attr, bibliography.old or unset)
  record('bibliography_end', {bibliography=bibliography.id})
end

function M.identify(n, item)
  item.citation = node.has_attribute(n, citation_attr)
  item.citation_link = node.has_attribute(n, link_attr)
  item.bibliography_entry = node.has_attribute(n, entry_attr)
end

local function append(runs, owner, slot, id, link)
  local last = runs[#runs]
  if last and last.owner == owner and last.id == id and last.link == link and last.last + 1 == slot then
    last.last = slot
  else
    runs[#runs + 1] = {owner=owner, first=slot, last=slot, id=id, link=link}
  end
end

function M.retained(n, citation_runs, bibliography_runs)
  local citation = node.has_attribute(n, citation_attr)
  local entry = node.has_attribute(n, entry_attr)
  if not citation and not entry then return end
  -- The second half of a split ligature repeats the first half's slot: that slot is already recorded.
  if flow.is_echo(n) then return end
  local owner, slot = flow.scalar_identity(n)
  if not owner then
    record('unsupported_citation_retention', {citation=citation, entry=entry, char=n.char})
    return
  end
  if citation then append(citation_runs, owner, slot, citation, node.has_attribute(n, link_attr)) end
  if entry then append(bibliography_runs, owner, slot, entry) end
end

function M.shipout(page, citation_runs, bibliography_runs)
  for _, run in ipairs(citation_runs) do
    record('citation_retained', {citation=run.id, citation_link=run.link, owner=run.owner,
      first=run.first, last=run.last, page=page})
  end
  for _, run in ipairs(bibliography_runs) do
    record('bibliography_retained', {entry=run.id, owner=run.owner, first=run.first, last=run.last, page=page})
  end
end

function M.finish()
  assert(#call_stack == 0 and #link_stack == 0 and #bibliography_stack == 0, 'Unclosed citation observation')
end

return M
