-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Observe ordinary LaTeX/AMS numeric citation labels without changing output.
local M = require('relatex-external-citations')
M.profile('latex-command-hooks')
local labels = {}
local entry_key

-- A key may name several items (\bibitem{a} twice, or empty keys): LaTeX keeps the label written
-- last, and so does the observer. Both auxiliary reads end on the same label.
function M.native_label(key, number)
  labels[key] = number
end

-- Read the label argument that follows unexpanded and tell TeX whether it may be expanded: a
-- control sequence undefined here (a style that defines it only inside \cite) would stop the run.
-- An author-year label (one that names \citeauthoryear or \citename) is typeset instead.
local author_year = {citeauthoryear = true, citename = true}
function M.label_readable()
  local readable, typeset = 'true', 'false'
  for _, t in ipairs(token.scan_toks()) do
    if t.cmdname == 'undefined_cs' then readable = 'false' end
    if t.csname and author_year[t.csname] then typeset = 'true' end
  end
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'),
    '\\ltd@label@readable' .. readable .. '\\ltd@label@typeset' .. typeset)
end

local function glyph_text(head, out)
  for n in node.traverse(head) do
    if n.id == node.id('glyph') then
      if n.components then glyph_text(n.components, out) else out[#out + 1] = utf8.char(n.char) end
    elseif n.id == node.id('glue') then
      out[#out + 1] = ' '
    elseif n.id == node.id('hlist') or n.id == node.id('vlist') then
      glyph_text(n.head, out)
    end
  end
end

-- The text a box prints: its glyphs, glue as one space, a space before a comma dropped.
function M.box_text(register)
  local out = {}
  glyph_text(tex.box[register].head, out)
  local text = table.concat(out):gsub('%s+', ' '):gsub(' ,', ','):gsub('^ ', ''):gsub(' $', '')
  return text
end

function M.native_link_begin(key)
  if not labels[key] then return M.unidentified_link(key, 'no executed bibliography label') end
  M.link_begin(key, labels[key])
end

-- The cite package names a number by its label macro b@KEY (plus hyperref's extra suffix, empty
-- by default).
function M.native_link_begin_tag(tag)
  M.native_link_begin((tag:gsub('^b@', '')))
end

function M.native_entry(key)
  entry_key = key
  M.entry(key)
end

function M.native_fallback_anchor(number)
  if entry_key then M.fallback_anchor(entry_key, number) end
end

-- The label an item's own \bibitem[..] gives; an item whose label printed nothing takes it.
local item_label
function M.native_item_label(label)
  item_label = label
end

function M.native_recorded_anchor()
  if entry_key and item_label then M.fallback_anchor(entry_key, item_label) end
end

function M.native_anchor(number)
  -- The bibliography measures its widest label before the first actual item.
  if entry_key then M.anchor(entry_key, number) end
end

return M
