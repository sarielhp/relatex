-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Where TeX paints the copies of a box leader: tex.web's hlist_out fits as many copies of the box as
-- the glue's width holds, aligned to the enclosing box (\leaders), centered (\cleaders) or spread
-- (\xleaders).
local M = {}

local kinds = {}
for key, name in pairs(node.subtypes('glue')) do kinds[key] = name end

-- TeX adds a few scaled points of slack to the glue before it counts the copies.
local SLACK = 10

-- The left edges of the copies of n's leader box in a glue width wide that starts at x, in the
-- coordinates of the box holding n.
function M.positions(n, x, width)
  local size, kind = n.leader.width, kinds[n.subtype]
  if size <= 0 then return {} end
  local edge, gap, at = x + width + SLACK, 0, x
  if kind == 'leaders' then
    at = size * (x // size)
    if at < x then at = at + size end
  else
    local count, rest = (width + SLACK) // size, (width + SLACK) % size
    if kind == 'cleaders' then at = x + rest // 2
    else gap = rest // (count + 1); at = x + (rest - (count - 1) * gap) // 2 end
  end
  local result = {}
  while at + size <= edge do
    result[#result + 1] = at
    at = at + size + gap
  end
  return result
end

-- A box leader of fixed width that holds exactly one copy of its box is that box set once (acmart's
-- affiliation comma, \cleaders\copy\@ACM@commabox\hskip\wd\@ACM@commabox): its text is read like any
-- other box's. A fill (\dotfill, a table of contents' dots) stretches and repeats, and stays a gap
-- even when its line leaves room for one copy. box is the list holding n, when it is packed; an
-- aligned leader's copies depend on where it sits, so it needs one. Returns the copy's left edge.
function M.single_copy(n, box)
  local leader = n.leader
  if not leader or (leader.id ~= node.id('hlist') and leader.id ~= node.id('vlist')) then return nil end
  if n.stretch ~= 0 or n.shrink ~= 0 then return nil end
  local x = 0
  if kinds[n.subtype] == 'leaders' then
    if not box or box.id ~= node.id('hlist') then return nil end
    x = node.rangedimensions(box, box.head, n)
  end
  local copies = M.positions(n, x, n.width)
  if #copies == 1 then return copies[1] - x end
end

return M
