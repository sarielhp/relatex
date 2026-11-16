-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- File names as TeX gives them, on every platform (decision 129). An absolute name starts at a
-- root: "/" on Unix; on Windows also a drive ("c:/texlive/...", "C:\...") or a UNC share
-- ("//server/share", "\\server\share"). kpathsea writes Windows names with "/", and its drive
-- letter's case varies between a variable's value and a found file's name.
local M = {}

function M.is_absolute(name)
  return name:find('^[/\\]') ~= nil or name:find('^%a:[/\\]') ~= nil
end

-- comparable is a name with its drive letter lower-cased, for comparing two absolute names.
function M.comparable(name)
  return (name:gsub('^(%a):', function(drive) return drive:lower() .. ':' end))
end

return M
