-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- LuaMML's tables of the classic math encodings (luamml-legacy-mappings: OML, OMS, OMX), with the
-- slots it names wrongly set to the character the glyph paints, as the font's own glyph name and
-- the PDF's ToUnicode map spell it. The module table is shared: every require sees the correction.
local mappings = require('luamml-legacy-mappings')
-- OML 0x2E and 0x2F are \triangleright and \triangleleft (cmmi glyphs "triangleright",
-- "triangleleft": U+25B7, U+25C1), which LuaMML gives the logical xor and nand (U+22BB, U+22BC).
mappings.oml[0x2E] = 0x25B7
mappings.oml[0x2F] = 0x25C1
-- OML 0x3F is \star (glyph "star", U+22C6), which LuaMML gives not-equivalent-to (U+226D).
mappings.oml[0x3F] = 0x22C6
-- OMS 0x0F is \bullet, glyph "bullet" (U+2022) in cmsy, lmsy and txsy alike, which LuaMML
-- gives the bullet operator (U+2219).
mappings.oms[0x0F] = 0x2022
return mappings
