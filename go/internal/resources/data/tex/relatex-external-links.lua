-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Observe executed hyperref URI links without changing PDF annotations or ink.
local M = {}
local attribute = luatexbase.new_attribute('relatex.external.link')
local writer, serial, stack = nil, 0, {}
function M.start(emit) writer = emit end
function M.begin_link(href)
  if not writer then return end
  serial = serial + 1
  stack[#stack + 1] = tex.getattribute(attribute) or -2147483647
  tex.setattribute(attribute, serial)
  writer('diagnostic', {code='external_link', source='hyperref-execution', link=serial, href=href})
end
function M.end_link()
  if not writer then return end
  local previous = assert(table.remove(stack), 'Unbalanced external link')
  tex.setattribute(attribute, previous)
end
function M.identify(n, item)
  item.external_link = node.has_attribute(n, attribute)
end
function M.finish()
  assert(#stack == 0, 'Unclosed external link')
  writer = nil
end
return M
