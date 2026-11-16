-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Scalar legacy slots from LaTeX's fontmath.ltx; no node references are stored.
local M = {}
local operators = {
  [0] = 0x0393, [1] = 0x0394, [2] = 0x0398, [3] = 0x039B,
  [4] = 0x039E, [5] = 0x03A0, [6] = 0x03A3, [7] = 0x03A5,
  [8] = 0x03A6, [9] = 0x03A8, [10] = 0x03A9,
  [18] = 0x0060, [19] = 0x00B4, [20] = 0x02C7, [21] = 0x02D8,
  [22] = 0x00AF, [23] = 0x02DA,
}
local blackboard = {}
for offset = 0, 25 do blackboard[65 + offset] = 0x1D538 + offset end
blackboard[67], blackboard[72], blackboard[78] = 0x2102, 0x210D, 0x2115
blackboard[80], blackboard[81], blackboard[82], blackboard[90] = 0x2119, 0x211A, 0x211D, 0x2124

function M.legacy_operators(family)
  assert(type(family) == 'number' and family >= 0, 'Invalid operator family')
  require('luamml-convert').register_family(family, operators)
end

function M.blackboard_letters(family)
  assert(type(family) == 'number' and family >= 0, 'Invalid blackboard family')
  require('luamml-convert').register_family(family, blackboard)
end

return M
