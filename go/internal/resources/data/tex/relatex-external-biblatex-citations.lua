-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Native biblatex citation evidence, without rewriting the PDF.
local M = require('relatex-external-citations')
M.profile('biblatex-command-hooks')

-- \blx@tempb is an etoolbox list: one <key>| item per cited key.
local function keys(text)
  local result = {}
  for key in text:gmatch('([^|]*)|') do result[#result + 1] = key end
  return table.concat(result, ',')
end

function M.biblatex_begin(raw)
  if raw ~= '' then M.begin(raw) end
end

function M.biblatex_sorted(raw)
  M.sorted(keys(raw))
end

function M.biblatex_key(key, number)
  M.link_begin(key, number)
end

function M.biblatex_link_end()
  M.link_end()
end

function M.biblatex_finish()
  M.finish_call()
end

function M.biblatex_entry(key, number)
  M.entry(key)
  M.anchor(key, number)
end

return M
