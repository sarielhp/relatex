-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Ordinary executed mark/body hooks; no native footnote sockets or PDF changes.
local M = {}
local marks, bodies, pending, active = 0, 0, {}, false
local title = 0
-- hyperref's \@footnotetext wraps its copy of the kernel's, \H@@footnotetext, and both are
-- hooked: a copy's call inside an open body is that body.
local depth = 0
-- Marks executed in \maketitle, in order: \thanks sets its marks, then its bodies, in the
-- same order, though a mark and its body may spell the symbol differently
-- (\textasteriskcentered against \char"2217).
local title_marks = {}
local notes
function M.install(reader)
  notes = reader
  notes.source = 'ordinary-footnote-hooks'
end
-- A mark passed as stored (hyperref's copies) and one expanded by \directlua differ only in
-- spaces after control words: one key for both.
local function key(number) return (number:gsub('%s+', ' ')) end
function M.mark_begin(number)
  number = key(number)
  marks = marks + 1
  pending[number] = pending[number] or {}
  pending[number][#pending[number] + 1] = marks
  if title > 0 then title_marks[#title_marks + 1] = {number = number, mark = marks} end
  notes.mark_begin(marks, number, '')
end
function M.mark_end() notes.mark_end() end
function M.title_begin() title = title + 1 end
function M.title_end() title = math.max(0, title - 1) end
function M.body_begin(number)
  number = key(number)
  local original = number
  assert(not active, 'Nested ordinary footnote bodies unsupported')
  -- Class title notes (\thanks in amsart, IEEEtran, aa) set a body with no executed mark.
  -- Record it unmarked for the reader to classify; never abort the ordinary build.
  if not pending[number] and title > 0 then number = M.title_mark_number() or number end
  local references = pending[number] or {}
  bodies, active = bodies + 1, true
  notes.body_enter(original)
  notes.body_bind(bodies, references, title > 0)
  pending[number] = nil
end
-- The number of the oldest title mark no body has taken yet.
function M.title_mark_number()
  while #title_marks > 0 do
    local entry = table.remove(title_marks, 1)
    if pending[entry.number] then return entry.number end
  end
end
function M.body_end()
  assert(active, 'Unbalanced ordinary footnote body')
  if depth > 0 then depth = depth - 1 return end
  notes.body_end()
  active = false
end
function M.copy_body_begin(number)
  if active then depth = depth + 1 else M.body_begin(number) end
end
function M.number_begin() if active then notes.number_begin() end end
function M.number_end() if active then notes.number_end() end end
return M
