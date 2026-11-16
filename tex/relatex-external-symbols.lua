-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Supplemental mappings require the observed encoding AND live declarations.
-- Do not alias extended encodings wholesale to OMS/OML: their slots differ.
local M = {}
local greek = {
  {'Gammaup',0,0x393,7},{'Deltaup',1,0x394,7},{'Thetaup',2,0x398,7},
  {'Lambdaup',3,0x39B,7},{'Xiup',4,0x39E,7},{'Piup',5,0x3A0,7},
  {'Sigmaup',6,0x3A3,7},{'Upsilonup',7,0x3A5,7},{'Phiup',8,0x3A6,7},
  {'Psiup',9,0x3A8,7},{'Omegaup',10,0x3A9,7},
  {'alphaup',11,0x3B1,0},{'betaup',12,0x3B2,0},{'gammaup',13,0x3B3,0},
  {'deltaup',14,0x3B4,0},{'epsilonup',15,0x3F5,0},{'zetaup',16,0x3B6,0},
  {'etaup',17,0x3B7,0},{'thetaup',18,0x3B8,0},{'iotaup',19,0x3B9,0},
  {'kappaup',20,0x3BA,0},{'lambdaup',21,0x3BB,0},{'muup',22,0x3BC,0},
  {'nuup',23,0x3BD,0},{'xiup',24,0x3BE,0},{'piup',25,0x3C0,0},
  {'rhoup',26,0x3C1,0},{'sigmaup',27,0x3C3,0},{'tauup',28,0x3C4,0},
  {'upsilonup',29,0x3C5,0},{'phiup',30,0x3D5,0},{'chiup',31,0x3C7,0},
  {'psiup',32,0x3C8,0},{'omegaup',33,0x3C9,0},{'varepsilonup',34,0x3B5,0},
  {'varthetaup',35,0x3D1,0},{'varpiup',36,0x3D6,0},{'varrhoup',37,0x3F1,0},
  {'varsigmaup',38,0x3C2,0},{'varphiup',39,0x3C6,0}
}
local symbols = {
  {'leq',20,0x2264,3},{'geq',21,0x2265,3},{'in',50,0x2208,3},
  {'infty',49,0x221E,0},{'subseteq',18,0x2286,3},{'supseteq',19,0x2287,3},
  {'subset',26,0x2282,3},{'supset',27,0x2283,3},
  {'times',2,0xD7,2},{'cdot',1,0x22C5,2},{'pm',6,0xB1,2},
  {'bullet',15,0x2022,2},{'ast',3,0x2217,2}
}
local delimiters = {{40,185,40,4},{41,186,41,5},{91,187,91,4},{93,188,93,5},{47,157,47,4}}
local braces = {{'lbrace',102,123,4},{'rbrace',103,125,5}}
local blackboard = {[67]=0x2102,[72]=0x210D,[78]=0x2115,[80]=0x2119,[81]=0x211A,[82]=0x211D,[90]=0x2124}
local operator_greek = {
  {'Gamma',0,0x393,7},{'Delta',1,0x394,7},{'Theta',2,0x398,7},
  {'Lambda',3,0x39B,7},{'Xi',4,0x39E,7},{'Pi',5,0x3A0,7},
  {'Sigma',6,0x3A3,7},{'Upsilon',7,0x3A5,7},{'Phi',8,0x3A6,7},
  {'Psi',9,0x3A8,7},{'Omega',10,0x3A9,7}
}
-- FML (fourier's math letters, FML/fncmi) keeps OML's Greek slots; the font paints \phi as the loopy
-- U+03C6 and \varphi as the straight U+03D5 (its ToUnicode; papers5/1370).
local fml_greek = {
  {'alpha',0x0B,0x3B1,0},{'beta',0x0C,0x3B2,0},{'gamma',0x0D,0x3B3,0},
  {'delta',0x0E,0x3B4,0},{'epsilon',0x0F,0x3F5,0},{'zeta',0x10,0x3B6,0},
  {'eta',0x11,0x3B7,0},{'theta',0x12,0x3B8,0},{'iota',0x13,0x3B9,0},
  {'kappa',0x14,0x3BA,0},{'lambda',0x15,0x3BB,0},{'mu',0x16,0x3BC,0},
  {'nu',0x17,0x3BD,0},{'xi',0x18,0x3BE,0},{'pi',0x19,0x3C0,0},
  {'rho',0x1A,0x3C1,0},{'sigma',0x1B,0x3C3,0},{'tau',0x1C,0x3C4,0},
  {'upsilon',0x1D,0x3C5,0},{'phi',0x1E,0x3C6,0},{'chi',0x1F,0x3C7,0},
  {'psi',0x20,0x3C8,0},{'omega',0x21,0x3C9,0},
  {'varepsilon',0x22,0x3B5,0},{'vartheta',0x23,0x3D1,0},{'varpi',0x24,0x3D6,0},
  {'varrho',0x25,0x3F1,0},{'varsigma',0x26,0x3C2,0},{'varphi',0x27,0x3D5,0}
}
-- upgreek's Euler family (U/eur): capitals at 0-10, small letters from 11; Euler paints \upepsilon as
-- U+03F5 and \upphi as the straight U+03D5, the reverse of FML (pdftotext of eurm10; papers4/0865).
local euler_greek = {
  {'Upgamma',0x00,0x393,0},{'Updelta',0x01,0x394,0},{'Uptheta',0x02,0x398,0},
  {'Uplambda',0x03,0x39B,0},{'Upxi',0x04,0x39E,0},{'Uppi',0x05,0x3A0,0},
  {'Upsigma',0x06,0x3A3,0},{'Upupsilon',0x07,0x3A5,0},{'Upphi',0x08,0x3A6,0},
  {'Uppsi',0x09,0x3A8,0},{'Upomega',0x0A,0x3A9,0},
  {'upalpha',0x0B,0x3B1,0},{'upbeta',0x0C,0x3B2,0},{'upgamma',0x0D,0x3B3,0},
  {'updelta',0x0E,0x3B4,0},{'upepsilon',0x0F,0x3F5,0},{'upzeta',0x10,0x3B6,0},
  {'upeta',0x11,0x3B7,0},{'uptheta',0x12,0x3B8,0},{'upiota',0x13,0x3B9,0},
  {'upkappa',0x14,0x3BA,0},{'uplambda',0x15,0x3BB,0},{'upmu',0x16,0x3BC,0},
  {'upnu',0x17,0x3BD,0},{'upxi',0x18,0x3BE,0},{'uppi',0x19,0x3C0,0},
  {'uprho',0x1A,0x3C1,0},{'upsigma',0x1B,0x3C3,0},{'uptau',0x1C,0x3C4,0},
  {'upupsilon',0x1D,0x3C5,0},{'upphi',0x1E,0x3D5,0},{'upchi',0x1F,0x3C7,0},
  {'uppsi',0x20,0x3C8,0},{'upomega',0x21,0x3C9,0},
  {'upvarepsilon',0x22,0x3B5,0},{'upvartheta',0x23,0x3D1,0},{'upvarpi',0x24,0x3D6,0},
  {'upvarphi',0x27,0x3C6,0}
}
-- \dot is set from slot 95 as well, which a typewriter or Latin Modern text font draws as the
-- underscore (url, \texttt in math): the family keeps that character, and the accent is read
-- from the tree (external-math: a mover accent over '_').
local operator_accents = {
  {'bar',22,0xAF},{'tilde',126,0x2DC},{'ddot',127,0xA8},
  {'breve',21,0x2D8},{'check',20,0x2C7},{'hat',94,0x5E}
}

local function family_declared(command, family)
  local value = token.create(command)
  return value.cmdname == 'char_given' and value.mode == family
end

local function accent_declared(name, family, slot)
  local macro = token.get_macro(name .. ' ') or token.get_macro(name) or ''
  local encoded = macro:match('^\\mathaccentV%s*{' .. name .. '}%s*(%x%x%x)%s*$')
  if encoded then return tonumber(encoded, 16) == family * 256 + slot end
  encoded = macro:match('^\\mathaccent%s*"(%x+)%s*\\relax%s*$')
  return encoded and tonumber(encoded, 16) == 7 * 4096 + family * 256 + slot
end

local function declared(mapping, family, entries)
  for _, spec in ipairs(entries) do
    local command = token.create(spec[1])
    if command.cmdname == 'math_given'
        and command.mode == spec[4] * 4096 + family * 256 + spec[2] then
      mapping[spec[2]] = spec[3]
    end
  end
end

local function declared_delimiters(mapping, family)
  for _, spec in ipairs(delimiters) do
    local code = tex.getmathcode(spec[1])
    local delimiter = tex.getdelcode(spec[1])
    if code[1] == spec[4] and code[2] == family and code[3] == spec[2]
        and delimiter[1] == family and delimiter[2] == spec[2] then
      mapping[spec[2]] = spec[3]
    end
  end
  for _, spec in ipairs(braces) do
    local macro = token.get_macro(spec[1]) or ''
    local hex = macro:match('^\\delimiter%s*"(%x+)%s*$')
    local code = hex and tonumber(hex, 16)
    if code and code >> 24 == spec[4] and (code >> 20 & 15) == family
        and (code >> 12 & 255) == spec[2] then mapping[spec[2]] = spec[3] end
  end
end

function M.mapping(family, declaration)
  local mapping = {}
  if declaration:match('^LMS/ntxsy/') then
    declared(mapping, family, symbols)
    declared(mapping, family, {{'prime',48,0x2032,0}})
    declared_delimiters(mapping, family)
    local plus = tex.getmathcode(43)
    if plus[1] == 2 and plus[2] == family and plus[3] == 207 then mapping[207] = 43 end
    local minus = tex.getmathcode(45)
    if minus[1] == 2 and minus[2] == family and minus[3] == 0 then mapping[0] = 0x2212 end
  elseif declaration:match('^U/MnSymbolC/m/n$') then
    -- MnSymbol sets the math hyphen-minus as its own minus sign (slot '020 of MnSyC).
    declared(mapping, family, {{'minus',16,0x2212,2}})
    local minus = tex.getmathcode(45)
    if minus[1] == 2 and minus[2] == family and minus[3] == 16 then mapping[16] = 0x2212 end
  elseif declaration:match('^OMS/[%w]+/') and family_declared('symsymbols', family) then
    -- OMS fixes its slots in every family (cmsy, lmsy, the Times virtual fonts of mathptmx and
    -- txfonts): the math asterisk and bullet print U+2217 and U+2022, not the ASCII asterisk and
    -- U+2219 of the legacy table (an elsarticle \cortext mark, a $\bullet$ item label).
    declared(mapping, family, {{'bullet',15,0x2022,2},{'ast',3,0x2217,2}})
  elseif declaration:match('^U/ntxmia/') then
    declared(mapping, family, greek)
  elseif declaration:match('^OT1/cmr/bx/n$') then
    -- The bold version (and the families bm allocates) set the same OT1 encoding, whose slots 0-10 are the
    -- Greek capitals; no live \Gamma declaration names those families to check against.
    for _, spec in ipairs(operator_greek) do mapping[spec[2]] = spec[3] end
  elseif (declaration:match('^OT1/[%w]+/m/n$') or declaration:match('^OT1/[%w]+/bx?/n$'))
      and (family_declared('symoperators', family) or family_declared('symlegacymaths', family)) then
    -- OT1 fixes slots 0-10 as the Greek capitals in every family (cmr, pslatex's ptmcm, mathptmx's
    -- ztmcm), never the control characters of their codes, whether \Omega is a math character or
    -- a macro choosing the slot (flexisym's \math_sym_Var:Nn \mg@Greek {0A}). fontspec sets the
    -- operators in the Unicode text font and moves the Greek capitals and accents to an OT1 cmr
    -- family of their own, legacymaths (fontspec-luatex.sty: \DeclareSymbolFont{legacymaths}{OT1}{cmr}).
    -- The bold math version sets another series of the same font (txfonts: OT1/txr/bx/n, papers5/1257).
    for _, spec in ipairs(operator_greek) do mapping[spec[2]] = spec[3] end
    for _, spec in ipairs(operator_accents) do
      if accent_declared(spec[1], family, spec[2]) then mapping[spec[2]] = spec[3] end
    end
  elseif declaration:match('^FML/[%w]+/') then
    declared(mapping, family, fml_greek)
  elseif declaration:match('^U/eur/b?m?/n$') then
    declared(mapping, family, euler_greek)
  elseif declaration:match('^U/msb/m/n$') and family_declared('symAMSb', family) then
    for code = 65, 90 do mapping[code] = blackboard[code] or 0x1D538 + code - 65 end
    declared(mapping, family, {{'rtimes',111,0x22CA,2},{'nsubseteq',42,0x2288,3},{'hbar',126,0x210F,0},
      {'thicksim',115,0x223C,3},{'thickapprox',116,0x2248,3},{'approxeq',117,0x224A,3}})
  elseif declaration:match('^U/msa/m/n$') and family_declared('symAMSa', family) then
    declared(mapping, family, {{'square',3,0x25A1,0},{'blacksquare',4,0x25A0,0}})
    -- amsfonts' text-usable marks are \mathhexbox{AMSa}<slot> (no named math symbol to check): the
    -- live AMSa family paints msam's own glyphs there, the check mark, not the letter X of code "58.
    for _, spec in ipairs({{85,0xA5},{88,0x2713},{114,0xAE},{122,0x2720}}) do mapping[spec[1]] = spec[2] end
  elseif declaration:match('^U/ProdInt/m/n$') and family_declared('symProdint', family) then
    -- prodint's product integrals (text, display, big) have no Unicode character of their own; they
    -- are n-ary products, read as U+220F, not the letters P, R, T of their slots.
    declared(mapping, family, {{'prodi',80,0x220F,1},{'Prodi',82,0x220F,1},{'PRODI',84,0x220F,1}})
  end
  -- A ` typed in math is set from its \mathcode slot of a text family; an OT1 font paints the left
  -- quotation mark there (glyph "quoteleft"), not the grave accent the slot is in ASCII.
  -- A typed " is the right double quotation mark of a roman OT1 font ("quotedblright").
  local typed = {[96] = 0x2018, [34] = 0x201D}
  local roman = not declaration:match('^OT1/[^/]*tt/')
  for char, code in pairs(declaration:match('^OT1/') and tex.getmathcode and typed or {}) do
    local mathcode = tex.getmathcode(char)
    if type(mathcode) == 'table' and mathcode[2] == family and mathcode[3] == char and (roman or char == 96) then
      mapping[char] = code
    end
  end
  if next(mapping) then return mapping end
end

return M
