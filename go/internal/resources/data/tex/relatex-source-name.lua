-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- The name of a file TeX read, as the source names it. TeX runs in the main file's directory, as
-- `cd sub && lualatex main.tex` would: for a main file in a subdirectory (RELATEX_MAIN_DIRECTORY,
-- unset for a main file at the source root) a relative name is relative to that directory, and
-- becomes the name from the source root ("../parts/a.tex" read from slides/ is "parts/a.tex").
-- Absolute names, and names that leave the source, stay as TeX read them.
local paths = require('relatex-path')
local directory = os.getenv('RELATEX_MAIN_DIRECTORY')
return function(name)
  if type(name) ~= 'string' or not directory or directory == '' or paths.is_absolute(name) then return name end
  local parts = {}
  for part in (directory .. '/' .. name):gmatch('[^/]+') do
    if part == '..' then
      if #parts == 0 then return name end
      parts[#parts] = nil
    elseif part ~= '.' then
      parts[#parts + 1] = part
    end
  end
  return table.concat(parts, '/')
end
