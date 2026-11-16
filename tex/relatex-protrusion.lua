-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Read scalar box/font data only; retain no nodes and preserve all PDF anchors.
local M = {}
local hlist, vlist, whatsit = node.id('hlist'), node.id('vlist'), node.id('whatsit')
local penalty, glyph, glue, kern = node.id('penalty'), node.id('glyph'), node.id('glue'), node.id('kern')
local destination = node.subtype('pdf_dest')

local function anchor(n, depth)
  if not n or depth > 4 then return false, false end
  if n.id == whatsit then return n.subtype == destination, n.subtype == destination end
  if n.id == penalty then return n.penalty == 10000, false end
  if (n.id ~= hlist and n.id ~= vlist) or n.width ~= 0 or n.height ~= 0 or n.depth ~= 0 then return false, false end
  local found = false
  for child in node.traverse(n.head) do
    local valid, nested = anchor(child, depth + 1)
    if not valid then return false, false end
    found = found or nested
  end
  return true, found
end

function M.heading(box)
  if not box or not box.head then return end
  local prefix = box.head
  if prefix.id == penalty and prefix.penalty == 10000 then prefix = prefix.next end
  local valid, destination_found = anchor(prefix, 0)
  if not valid or not destination_found then return end
  local current = prefix.next
  while current and ((current.id == glue and current.width == 0 and current.stretch == 0 and current.shrink == 0)
      or (current.id == kern and current.kern == 0)) do current = current.next end
  if not current or current.id ~= glyph or current.font ~= font.current()
      or current.xoffset ~= 0 or current.yoffset ~= 0 or current.char > 0x10FFFF then return end
  -- Use microtype's ordinary algorithm for this already-selected glyph. Emit
  -- its kern OUTSIDE the original label box, preserving the hanging indent.
  tex.sprint('\\csname MT@get@prot\\endcsname{\\char' .. current.char .. '\\relax}{left}')
end

return M
