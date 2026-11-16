-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Bounded NFSS profiles bind alphabet semantics to the fonts actually selected.
-- Only the external tree changes; engine characters and font assignments do not.
local M, fonts = {}, {}
local math_char = node.id('math_char')
local bases = {
  bold={0x1D400,0x1D41A,0x1D7CE}, italic={0x1D434,0x1D44E},
  ['bold-italic']={0x1D468,0x1D482}, script={0x1D49C,0x1D4B6},
  ['bold-script']={0x1D4D0,0x1D4EA}, fraktur={0x1D504,0x1D51E},
  ['bold-fraktur']={0x1D56C,0x1D586}, ['sans-serif']={0x1D5A0,0x1D5BA,0x1D7E2},
  ['bold-sans-serif']={0x1D5D4,0x1D5EE,0x1D7EC},
  ['sans-serif-italic']={0x1D608,0x1D622},
  ['sans-serif-bold-italic']={0x1D63C,0x1D656}, monospace={0x1D670,0x1D68A,0x1D7F6},
  ['double-struck']={0x1D538,0x1D552,0x1D7D8}
}
local holes = {
  italic={[104]=0x210E},
  script={[66]=0x212C,[69]=0x2130,[70]=0x2131,[72]=0x210B,[73]=0x2110,
    [76]=0x2112,[77]=0x2133,[82]=0x211B,[101]=0x212F,[103]=0x210A,[111]=0x2134},
  fraktur={[67]=0x212D,[72]=0x210C,[73]=0x2111,[82]=0x211C,[90]=0x2128},
  ['double-struck']={[67]=0x2102,[72]=0x210D,[78]=0x2115,[80]=0x2119,[81]=0x211A,
    [82]=0x211D,[90]=0x2124}
}
local greek = 'ΑΒΓΔΕΖΗΘΙΚΛΜΝΞΟΠΡϴΣΤΥΦΧΨΩ∇αβγδεζηθικλμνξοπρςστυφχψω∂ϵϑϰϕϱϖ'
local greek_bases = {bold=0x1D6A8, italic=0x1D6E2, ['bold-italic']=0x1D71C,
  ['bold-sans-serif']=0x1D756, ['sans-serif-bold-italic']=0x1D790}
local reverse, greek_index = {}, {}

local function latin(code, variant)
  local spec = bases[variant]
  if not spec then return end
  if holes[variant] and holes[variant][code] then return holes[variant][code] end
  if code >= 65 and code <= 90 then return spec[1] + code - 65 end
  if code >= 97 and code <= 122 then return spec[2] + code - 97 end
  if spec[3] and code >= 48 and code <= 57 then return spec[3] + code - 48 end
end

for variant in pairs(bases) do
  for code = 48, 122 do
    local value = latin(code, variant)
    if value then reverse[value] = code end
  end
end
do
  local index = 0
  for _, code in utf8.codes(greek) do
    greek_index[code] = index
    for _, base in pairs(greek_bases) do reverse[base + index] = code end
    index = index + 1
  end
end

function M.variant(declaration)
  local encoding, family, series, shape = declaration:gsub('^\\', ''):match('^([^/]+)/([^/]+)/([^/]+)/([^/]+)$')
  if (series ~= 'm' and series ~= 'b' and series ~= 'bx') or (shape ~= 'n' and shape ~= 'it' and shape ~= 'sl') then return end
  local bold, italic = series == 'b' or series == 'bx', shape == 'it' or shape == 'sl'
  -- beamer's math letters (its pureletters font) are the sans text font, Latin Modern Sans in TU:
  -- its italic is the deck's math italic, as cmmi is a paper's (decision 083).
  if (encoding == 'OT1' and family == 'cmr') or (encoding == 'TU' and family == 'lmss') then
    return bold and (italic and 'bold-italic' or 'bold') or (italic and 'italic' or 'normal')
  end
  if encoding == 'OML' and family == 'cmm' then return bold and 'bold-italic' or 'italic' end
  -- txfonts' math italic letters (txmi) are the Latin letters of Computer Modern's OML.
  if encoding == 'U' and family == 'txmi' then return bold and 'bold-italic' or 'italic' end
  if encoding == 'OMS' and family == 'cmsy' then return bold and 'bold-script' or 'script' end
  if encoding == 'OT1' and family == 'cmss' then
    return bold and (italic and 'sans-serif-bold-italic' or 'bold-sans-serif')
      or (italic and 'sans-serif-italic' or 'sans-serif')
  end
  if encoding == 'OT1' and family == 'cmtt' then return 'monospace' end
  if encoding == 'U' and family == 'euf' then return bold and 'bold-fraktur' or 'fraktur' end
  -- bbm's and dsfont's roman alphabets keep their characters at ASCII slots; their sans and
  -- typewriter siblings (bbmss, bbmtt, dsss) and bbold's other encoding stay as they are.
  if encoding == 'U' and (family == 'bbm' or family == 'dsrom') then return 'double-struck' end
end

function M.bind(id, declaration)
  if type(id) ~= 'number' or id <= 0 then return end
  local variant = M.variant(declaration)
  if not variant then return end
  if fonts[id] ~= nil and fonts[id] ~= variant then fonts[id] = false else fonts[id] = variant end
end

function M.encode(text, variant)
  if variant == 'normal' then return text end
  local result = {}
  for _, code in utf8.codes(text) do
    local base = reverse[code] or code
    local value = latin(base, variant)
    if not value and greek_index[base] and greek_bases[variant] then
      value = greek_bases[variant] + greek_index[base]
    end
    result[#result + 1] = utf8.char(value or code)
  end
  return table.concat(result)
end

local function token_variant(tree)
  local variant
  for _, native in ipairs(tree[':nodes'] or {}) do
    if native.id == math_char then
      local size = math.min((tree[':style'] or 2) // 2, 2)
      local found = fonts[tex.getfontoffamily(native.fam, size)]
      if found == false then return nil, 'conflicting observed native alphabet font' end
      if variant and found and variant ~= found then return nil, 'mixed native alphabet fonts in one token' end
      variant = found or variant
    end
  end
  return variant
end

function M.apply(tree, failures)
  if type(tree) ~= 'table' then return end
  if tree[':alphabet_applied'] then return end
  if tree[0] == 'mi' or tree[0] == 'mn' then
    local variant, reason = token_variant(tree)
    if reason then failures[reason] = true end
    if variant then
      for index, value in ipairs(tree) do
        if type(value) == 'string' then tree[index] = M.encode(value, variant) end
      end
      if variant ~= 'normal' then tree.mathvariant = nil end
    end
  end
  for _, child in ipairs(tree) do M.apply(child, failures) end
  tree[':alphabet_applied'] = true
end

return M
