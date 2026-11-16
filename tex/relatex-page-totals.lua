-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
local M = {}
local writer, shipouts
local expected = '7979159e363fabfc2f9e9680313683de0207157c2d5f46145f18c03eaa6c78c3'
function M.start(emit, counter) writer, shipouts = emit, counter end
function M.supported()
 local path = kpse.find_file('totpages.sty')
 local file = path and io.open(path,'rb')
 if not file then return false end
 local bytes = file:read(262145); file:close()
 if not bytes or #bytes > 262144 then return false end
 local hash = sha2.digest256(bytes):gsub('.',function(c) return string.format('%02x',c:byte()) end)
 return hash == expected
end
function M.unsupported()
 writer('diagnostic',{code = 'unsupported_document_page_total', source = 'totpages-2.00-hyperref'})
end
function M.observe(total, cached, anchor)
 writer('diagnostic',{code = 'document_page_total', source = 'totpages-2.00-hyperref',
  package_sha256 = expected, label = 'TotPages', total = total, cached = cached,
  shipouts = shipouts(), anchor = anchor})
end
function M.finish() writer, shipouts = nil, nil end
return M
