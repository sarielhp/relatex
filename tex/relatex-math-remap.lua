-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Legacy math fonts beyond OML/OMS/OMX (MnSymbol, stmaryrd, ...) carry no Unicode: LuaMML
-- would write their raw slots. Each slot of such a family that a live math declaration
-- (\mathchardef, \delimiter) names gets that command's Unicode value from unicode-math's
-- table; a slot two commands give different values stays unmapped.
local M = {}

-- Classic names unicode-math defines as aliases rather than in its table.
local ALIASES = {
  {'circ', 0x2218}, {'leq', 0x2264}, {'le', 0x2264}, {'geq', 0x2265}, {'ge', 0x2265}, {'neq', 0x2260},
  {'ne', 0x2260}, {'to', 0x2192}, {'gets', 0x2190}, {'owns', 0x220B}, {'lnot', 0xAC}, {'land', 0x2227},
  {'lor', 0x2228}, {'diamond', 0x22C4}, {'mid', 0x2223}, {'parallel', 0x2225}, {'perp', 0x27C2},
  {'emptyset', 0x2205}, {'lbrace', 0x7B}, {'rbrace', 0x7D}, {'vert', 0x7C}, {'Vert', 0x2016},
  {'doteq', 0x2250}, {'lhd', 0x22B2}, {'rhd', 0x22B3}, {'unlhd', 0x22B4}, {'unrhd', 0x22B5},
  {'setminus', 0x2216}, {'bullet', 0x2219}, {'colon', 0x3A}, {'cdotp', 0x22C5},
  -- \not is the symbols font's negation slash (slot "36), which unicode-math names \notaccent: in a foreign
  -- symbols font (fouriernc's fncsy) the raw slot read as the digit 6, so \ne printed "6=" (papers5/1370).
  {'not', 0x338},
  -- The capital Greek letters (txfonts and pxfonts set them in a font of their own).
  {'Gamma', 0x393}, {'Delta', 0x394}, {'Theta', 0x398}, {'Lambda', 0x39B}, {'Xi', 0x39E},
  {'Pi', 0x3A0}, {'Sigma', 0x3A3}, {'Upsilon', 0x3A5}, {'Phi', 0x3A6}, {'Psi', 0x3A8},
  {'Omega', 0x3A9},
  -- newtxmath sets its integrals in its own large-symbol fonts (LMX/ntxexx, U/ntxexa); unicode-math has no
  -- name for the sloped (sl) and upright (up) forms, which are the ordinary n-ary integrals.
  {'intslop', 0x222B}, {'iintslop', 0x222C}, {'iiintslop', 0x222D}, {'ointslop', 0x222E},
  {'oiintslop', 0x222F}, {'oiiintslop', 0x2230}, {'varointclockwiseslop', 0x2232},
  {'ointctrclockwiseslop', 0x2233}, {'sumintslop', 0x2A0B}, {'iiiintslop', 0x2A0C},
  {'fintslop', 0x2A0F}, {'sqintslop', 0x2A16},
  {'intupop', 0x222B}, {'iintupop', 0x222C}, {'iiintupop', 0x222D}, {'ointupop', 0x222E},
  {'oiintupop', 0x222F}, {'oiiintupop', 0x2230}, {'varointclockwiseupop', 0x2232},
  {'ointctrclockwiseupop', 0x2233}, {'sumintupop', 0x2A0B}, {'iiiintupop', 0x2A0C},
  {'fintupop', 0x2A0F}, {'sqintupop', 0x2A16},
  -- esint sets its integrals as \DeclareMathSymbol slots of its own font (esint10, family largesymbolsA):
  -- \iint and \int are macros around these ...op symbols, the glyphs the n-ary integrals unicode-math names.
  {'intop', 0x222B}, {'iintop', 0x222C}, {'iiintop', 0x222D}, {'iiiintop', 0x2A0C}, {'ointop', 0x222E},
  {'oiintop', 0x222F}, {'varoiintop', 0x222F}, {'ointctrclockwiseop', 0x2233}, {'ointclockwiseop', 0x2232},
  {'varointclockwiseop', 0x2232}, {'varointctrclockwiseop', 0x2233}, {'fintop', 0x2A0F},
  -- newtxmath's and txfonts' n-ary operators are \...op symbols that \sum, \prod, ... wrap in macros.
  {'sumop', 0x2211}, {'prodop', 0x220F}, {'coprodop', 0x2210}, {'bigcupop', 0x22C3}, {'bigcapop', 0x22C2},
  {'bigsqcupop', 0x2A06}, {'bigsqcapop', 0x2A05}, {'bigodotop', 0x2A00}, {'bigoplusop', 0x2A01},
  {'bigotimesop', 0x2A02}, {'biguplusop', 0x2A04}, {'bigcupdotop', 0x2A03}, {'bigwedgeop', 0x22C0},
  {'bigveeop', 0x22C1}, {'bigtimesop', 0x2A09},
  -- stmaryrd's slanted triangle relations (stmary10 "50.."53) and txfonts' \coloneqq (txsy slot 66, which mathtools
  -- builds from two characters instead): unicode-math names neither, so the slot would read as a letter.
  {'trianglelefteqslant', 0x22B4}, {'trianglerighteqslant', 0x22B5}, {'ntrianglelefteqslant', 0x22EC},
  {'ntrianglerighteqslant', 0x22ED}, {'coloneqq', 0x2254},
  -- amssymb's msam/msbm names that unicode-math's table lacks. The circle arrows are msam's glyphs
  -- "clockwise", "anticlockwise", gapped circles the PDF spells U+27F3, U+27F2 (texglyphlist).
  {'thicksim', 0x223C}, {'thickapprox', 0x2248}, {'backepsilon', 0x3F6}, {'blacklozenge', 0x29EB},
  {'blacksquare', 0x25A0}, {'centerdot', 0x22C5}, {'circlearrowleft', 0x27F2}, {'circlearrowright', 0x27F3},
  {'circledS', 0x24C8}, {'digamma', 0x3DD}, {'doteqdot', 0x2251}, {'eth', 0xF0}, {'lozenge', 0x25CA},
  {'nshortmid', 0x2224}, {'nshortparallel', 0x2226}, {'ntriangleleft', 0x22EA}, {'ntriangleright', 0x22EB},
  {'nsucceq', 0x22E1}, {'npreceq', 0x22E0}, {'shortmid', 0x2223}, {'shortparallel', 0x2225},
  {'smallfrown', 0x2322}, {'smallsetminus', 0x2216}, {'smallsmile', 0x2323}, {'square', 0x25A1},
  {'varkappa', 0x3F0}, {'varpropto', 0x221D},
  -- stmaryrd's white square brackets (stmary10 "4A, "4B), which unicode-math names \lBrack, \rBrack.
  {'llbracket', 0x27E6}, {'rrbracket', 0x27E7},
  -- The lower-case Greek letters: unicode-math names the italic and upright forms (\mitalpha, \mupalpha), not
  -- the classic \alpha; stix declares them as OML-layout slots of its math italic font. \epsilon and \phi are
  -- the lunate epsilon and the straight phi of TeX, \varepsilon and \varphi the round ones.
  {'alpha', 0x3B1}, {'beta', 0x3B2}, {'gamma', 0x3B3}, {'delta', 0x3B4}, {'epsilon', 0x3F5}, {'zeta', 0x3B6},
  {'eta', 0x3B7}, {'theta', 0x3B8}, {'iota', 0x3B9}, {'kappa', 0x3BA}, {'lambda', 0x3BB}, {'mu', 0x3BC},
  {'nu', 0x3BD}, {'xi', 0x3BE}, {'pi', 0x3C0}, {'rho', 0x3C1}, {'sigma', 0x3C3}, {'tau', 0x3C4},
  {'upsilon', 0x3C5}, {'phi', 0x3D5}, {'chi', 0x3C7}, {'psi', 0x3C8}, {'omega', 0x3C9},
  {'varepsilon', 0x3B5}, {'vartheta', 0x3D1}, {'varpi', 0x3D6}, {'varrho', 0x3F1}, {'varsigma', 0x3C2},
  {'varphi', 0x3C6},
  -- MnSymbol's dots: \cdots is a \mathchardef alias for its own slot, \hdots (the centered ellipsis, \cdots on the same slot) and \udots
  -- (the ascending dots) have no entry in unicode-math's table, nor has the internal \@vdots under \vdots.
  {'cdots', 0x22EF}, {'hdots', 0x22EF}, {'udots', 0x22F0}, {'@vdots', 0x22EE},
  -- upgreek's upright small Greek letters (Euler roman or Symbol slots) have no unicode-math name.
  {'upalpha', 0x3B1}, {'upbeta', 0x3B2}, {'upgamma', 0x3B3}, {'updelta', 0x3B4}, {'upepsilon', 0x3B5},
  {'upzeta', 0x3B6}, {'upeta', 0x3B7}, {'uptheta', 0x3B8}, {'upiota', 0x3B9}, {'upkappa', 0x3BA},
  {'uplambda', 0x3BB}, {'upmu', 0x3BC}, {'upnu', 0x3BD}, {'upxi', 0x3BE}, {'upomicron', 0x3BF},
  {'uppi', 0x3C0}, {'uprho', 0x3C1}, {'upvarsigma', 0x3C2}, {'upsigma', 0x3C3}, {'uptau', 0x3C4},
  {'upupsilon', 0x3C5}, {'upphi', 0x3C6}, {'upchi', 0x3C7}, {'uppsi', 0x3C8}, {'upomega', 0x3C9}
}

-- The classic lower-case Greek letters set in a slanted font (a math italic family: stix's LS1 letters,
-- newtxmath's OML nxlmi) print italic, as LuaMML's own OML table reads cmmi: the mathematical italic
-- letters (U+1D6FC...), not the upright U+03B1... that MathML writes with mathvariant="normal" (papers/0007's
-- \epsilon is LibertineMathMI's italic glyph). An upright family (newtxmath's lettersA under \alphaup,
-- upgreek's fonts, slant 0) keeps the upright letters of ALIASES.
local ITALIC_GREEK = {
  alpha = 0x1D6FC, beta = 0x1D6FD, gamma = 0x1D6FE, delta = 0x1D6FF, epsilon = 0x1D716, zeta = 0x1D701,
  eta = 0x1D702, theta = 0x1D703, iota = 0x1D704, kappa = 0x1D705, lambda = 0x1D706, mu = 0x1D707,
  nu = 0x1D708, xi = 0x1D709, pi = 0x1D70B, rho = 0x1D70C, sigma = 0x1D70E, tau = 0x1D70F,
  upsilon = 0x1D710, phi = 0x1D719, chi = 0x1D712, psi = 0x1D713, omega = 0x1D714,
  varepsilon = 0x1D700, vartheta = 0x1D717, varpi = 0x1D71B, varrho = 0x1D71A, varsigma = 0x1D70D,
  varphi = 0x1D711
}

-- True when the family's text font is slanted (its TFM slant parameter is positive).
local function slanted(family)
  local id = tex.getfontoffamily(family, 0)
  local data = id and id > 0 and font and font.getfont and font.getfont(id)
  return data and data.parameters and (data.parameters.slant or 0) > 0 or false
end

-- Operators MnSymbol offers in display and text sizes besides the big... ones.
local SIZED = {sum = true, prod = true, coprod = true, int = true}
-- amsmath keeps these operators' math characters as \sum@, \bigoplus@, ... and makes \sum a macro around them
-- (amsmath.sty: \let\bigoplus@\bigoplus): the named macro has no slot, so a foreign large-symbols font (fouriernc's
-- fourier-mex) read \bigoplus as its raw slot "4C, the letter L (papers5/1370).
local AMS_WRAPPED = {coprod = true, bigvee = true, bigwedge = true, biguplus = true, bigcap = true, bigcup = true,
  prod = true, sum = true, bigotimes = true, bigoplus = true, bigodot = true, bigsqcup = true}

local cached
local function commands()
  if cached then return cached end
  local path = kpse.find_file('unicode-math-table.tex')
  local file = path and io.open(path, 'r')
  if not file then return {} end
  local found = {}
  for _, alias in ipairs(ALIASES) do found[#found + 1] = alias end
  for line in file:lines() do
    local code, name = line:match('^\\UnicodeMathSymbol{"(%x+)}{\\(%a+)%s*}')
    -- \cdotp is \cdot's glyph set as punctuation (MnSymbol declares both on one slot): the glyph
    -- is the dot operator, so the table's middle dot would only make the slot conflict.
    if code and name ~= 'cdotp' then
      found[#found + 1] = {name, tonumber(code, 16)}
      -- MnSymbol sets its operators in two sizes of its own font: \dbigoplus (display) and \tbigoplus (text)
      -- are \bigoplus, \dsum and \tsum are \sum.
      if AMS_WRAPPED[name] then found[#found + 1] = {name .. '@', tonumber(code, 16)} end
      if name:match('^big') or SIZED[name] then
        found[#found + 1] = {'d' .. name, tonumber(code, 16)}
        found[#found + 1] = {'t' .. name, tonumber(code, 16)}
      end
    end
  end
  file:close()
  cached = found
  return found
end

local function slots(name)
  local command = token.create(name)
  if command.cmdname == 'math_given' then
    local value = command.mode
    return {{value // 256 % 16, value % 256}}
  end
  if command.cmdname ~= 'call' then return {} end
  local hex = (token.get_macro(name) or ''):match('^\\delimiter%s*"(%x+)%s*$')
  if not hex then return {} end
  local value = tonumber(hex, 16)
  return {{value >> 20 & 15, value >> 12 & 255}, {value >> 8 & 15, value & 255}}
end

-- flexisym (loaded by breqn and by pslatex's math) turns LaTeX's * and \ast into the same macro,
-- "\@sym \ast \math_sym_Bin:Nn \mg@bin {03}": the character is active, so a typed * has no
-- mathcode slot, and \ast is a macro, so it has no math character either. The macro names the
-- family (\mg@bin expands to its number) and the slot of the math asterisk; nil when \ast is not
-- such a macro (a plain or stix \mathchardef, or no flexisym).
local function flexisym_asterisk()
  local command = token.create('ast')
  if not tostring(command.cmdname):match('_?call$') then return nil end
  local font, hex = (token.get_macro('ast') or ''):match('^\\@sym%s*\\ast%s*\\math_sym_%a+:Nn%s*\\mg@(%a+)%s*{(%x+)}')
  local family = font and token.get_macro('mg@' .. font)
  if not (family and family:match('^%x+$')) then return nil end
  return tonumber(family, 16), tonumber(hex, 16)
end

-- LuaMML reads Computer and Latin Modern math fonts (OML, OMS, OMX) itself. A family set in
-- another legacy font (MnSymbol replaces even the symbol families) needs its own slots.
local function foreign(family)
  local id = tex.getfontoffamily(family, 0)
  local name = id and id > 0 and tex.fontname(id)
  return name and not name:match('^cm') and not name:match('^lm') and not name:match('^eu[fs]') or false
end

-- Characters typed in math ('|' in \left| ... \right|) reach their slots through \mathcode
-- and \delcode; as delimiters '<' and '>' are angle brackets.
local function character_slots(add)
  for char = 33, 126 do
    local code = tex.getmathcode(char)
    -- Class 7 (letters, digits) takes the current family: its glyphs are the alphabet's own.
    if type(code) == 'table' and code[1] ~= 8 and code[1] ~= 7 and code[2] and code[3] then add(code[2], code[3], char) end
    local delimiter = tex.getdelcode(char)
    if type(delimiter) == 'table' and delimiter[1] and delimiter[1] >= 0 then
      local value = char == 60 and 0x27E8 or char == 62 and 0x27E9 or char
      -- \delcode 0 is the null delimiter (\left.), not the glyph in slot 0 of family 0.
      if delimiter[1] ~= 0 or delimiter[2] ~= 0 then add(delimiter[1], delimiter[2], value) end
      if delimiter[3] ~= 0 or delimiter[4] ~= 0 then add(delimiter[3], delimiter[4], value) end
    end
  end
end

-- The AMS symbol fonts (msam, msbm) have one fixed layout, which amssymb.sty names slot by slot.
-- A class may name a slot again for another command (mn2e declares \leqslant on msam "36 and lets
-- \le be it, so \le and \leqslant vote for two characters): the font's own glyph decides.
local ams_cached
local function ams_layout()
  if ams_cached then return ams_cached end
  ams_cached = {AMSa = {}, AMSb = {}}
  local path = kpse.find_file('amssymb.sty')
  local file = path and io.open(path, 'r')
  if not file then return ams_cached end
  local codes = {}
  for _, entry in ipairs(commands()) do
    if codes[entry[1]] == nil then codes[entry[1]] = entry[2] end
  end
  for line in file:lines() do
    local name, font, hex = line:match('^%s*\\DeclareMath%a+{\\(%a+)}%s*{\\%a+}{(AMS[ab])}{"(%x+)}')
    local code = name and codes[name]
    if code then
      local layout, slot = ams_cached[font], tonumber(hex, 16)
      if layout[slot] == nil then layout[slot] = code elseif layout[slot] ~= code then layout[slot] = false end
    end
  end
  file:close()
  return ams_cached
end

-- Glyphs a font paints at a slot whatever command names it: newtxmath keeps the kernel's \star
-- (letters "3F, unicode-math's star operator), but its math italic fonts (ntxmi, ntxbmi) paint a
-- black star there, glyph "uni2605". Its \checkmark, \circledR and \maltese are \mathhexbox slots of
-- the AMSm font (ntxsym "D8, "F2, "FA: glyphs "check", "registered", "maltese"), no named symbol.
local NEWTX_MARKS = {[0xD8] = 0x2713, [0xF2] = 0xAE, [0xFA] = 0x2720}
local NEWTX_STAR = {[0x3F] = 0x2605}
-- latexsym's lasy fonts: the slots latexsym.sty declares (\Box "32, \Diamond "33, \lhd "01 ...). Read by their
-- ASCII codes they were digits and punctuation: \hfill $\Box$ ending a proof read "2" (papers2/0232).
local LASY = {[0x01] = 0x22B2, [0x02] = 0x22B4, [0x03] = 0x22B3, [0x04] = 0x22B5, [0x30] = 0x2127, [0x31] = 0x2A1D,
  [0x32] = 0x25A1, [0x33] = 0x25C7, [0x3B] = 0x2933, [0x3C] = 0x228F, [0x3D] = 0x2290}
local FONT_SLOTS = {ntxmi = NEWTX_STAR, ntxbmi = NEWTX_STAR, ntxsym = NEWTX_MARKS, ntxbsym = NEWTX_MARKS, lasy = LASY,
  lasyb = LASY}

-- The fixed layout of a font by its TFM name (design sizes alike: ntxmi7 is ntxmi), or nil.
function M.font_layout(name)
  -- tex.fontname spells a font loaded at another size "msam10 at 9.0pt".
  name = (name or ''):match('^%S*')
  return name:match('^msam%d+$') and ams_layout().AMSa or name:match('^msbm%d+$') and ams_layout().AMSb
    or FONT_SLOTS[name:gsub('%d+$', '')]
end

local function font_layouts(maps)
  for family = 0, 15 do
    local id = tex.getfontoffamily(family, 0)
    local layout = id and id > 0 and M.font_layout(tex.fontname(id))
    for slot, code in pairs(layout or {}) do
      if code then
        maps[family] = maps[family] or {}
        maps[family][slot] = code
      end
    end
  end
end

-- Symbol fonts whose glyph names are reviewed by the painted glyph (relatex-symbol-glyphs.txt): txfonts'
-- and pxfonts' symbolsC and largesymbolsA (txsyc, pxbsyc, txexa ...), stmaryrd and wasysym. Their
-- commands unicode-math has no name for (\strictif, \lJoin, \Wr, \lambdabar, \medcirc) gave no vote, so
-- the slot read as its ASCII code ("J", "m", "o", ^^07); in text these glyphs already read by name.
local NAMED_FONTS = {'^[tp]xb?syc$', '^[tp]xb?exa$', '^stmary%d+$', '^wasy%a*%d+$'}

local function named_font(name)
  name = (name or ''):match('^%S*')
  for _, pattern in ipairs(NAMED_FONTS) do
    if name:match(pattern) then return true end
  end
  return false
end

-- A slot of such a font that no command or typed character settles (no vote, or votes that differ)
-- reads as the one character its glyph's reviewed name stands for. A name that stands for none, or
-- for several characters, leaves the slot as it was.
local function named_layouts(maps)
  local ok, glyph_names = pcall(require, 'relatex-glyph-names')
  if not ok then return end
  for family = 0, 15 do
    local id = tex.getfontoffamily(family, 0)
    if id and id > 0 and named_font(tex.fontname(id)) then
      local map = maps[family] or {}
      for slot = 0, 255 do
        if not map[slot] then
          local name, tfm = glyph_names.name(id, slot)
          local text = name and glyph_names.unicode(name, tfm)
          if text and utf8.len(text) == 1 then map[slot] = utf8.codepoint(text) end
        end
      end
      if next(map) then maps[family] = map end
    end
  end
end

-- Every slot this module has given each family (LuaMML keeps a slot's value until it is set again, so a
-- slot a later install leaves out still reads as before), and the fonts (text, script, scriptscript)
-- the family held at the last install.
local registered, registrar = {}, nil

-- Slots a conversion must read as the math asterisk even though a later binding withdraws the
-- value this module gave them. relatex-external-math re-binds every declared family from its
-- encoding on each formula (M.family), and an OMS symbols family the kernel does not call
-- \symsymbols gets no \ast entry there: the slot falls back to its raw ASCII code (papers3/0757
-- with flexisym, $C^*$ read "*").
local asterisks = {}

local function family_fonts(family)
  return {tex.getfontoffamily(family, 0) or 0, tex.getfontoffamily(family, 1) or 0,
    tex.getfontoffamily(family, 2) or 0}
end

local function same_fonts(left, right)
  return left[1] == right[1] and left[2] == right[2] and left[3] == right[3]
end

-- url.sty sets a URL (\url, \path, \doi) in math as `$\fam\z@ \textfont\z@\font ...$`: family 0
-- then holds the URL's text font (cm-super's typewriter SFTT0800 in papers2/0414), while its
-- characters keep their own slots in family 0. LuaMML's remap is keyed by family, so under stix
-- the operators font's slots (\pm "2C, \mp "2D, ( "2E, ) "2F) turned 10.1007/s00453-019 into
-- 10(1007)s00453∓019. A family whose fonts are not those its map was built for reads its slots
-- plainly while convert runs, and gets its map back afterwards.
function M.with_family_fonts(convert)
  local detached, plain_families = {}, {}
  for family, entry in pairs(registered) do
    if not same_fonts(family_fonts(family), entry.fonts) then
      local plain = {}
      for char in pairs(entry.mapping) do plain[char] = char end
      registrar.register_family(family, plain)
      detached[#detached + 1] = family
      plain_families[family] = true
    end
  end
  -- The family binding above runs before this call (relatex-external-math: M.family in the
  -- callback): set the math asterisk again, still before the conversion, unless the family
  -- holds another font and reads its slots plainly.
  for _, entry in ipairs(asterisks) do
    if not plain_families[entry.family] then
      registrar.register_family(entry.family, {[entry.slot] = 0x2217})
    end
  end
  local result = table.pack(pcall(convert))
  for _, family in ipairs(detached) do registrar.register_family(family, registered[family].mapping) end
  if not result[1] then error(result[2], 0) end
  return table.unpack(result, 2, result.n)
end

function M.install()
  local ok, convert = pcall(require, 'luamml-convert')
  if not ok then return end
  registrar = convert
  for family, entry in pairs(registered) do entry.fonts = family_fonts(family) end
  local maps = {}
  local function add(family, char, code)
    if not foreign(family) then return end
    local map = maps[family] or {}
    maps[family] = map
    if map[char] == nil then map[char] = code
    elseif map[char] ~= code then map[char] = false end
  end
  for _, entry in ipairs(commands()) do
    for _, slot in ipairs(slots(entry[1])) do
      local italic = ITALIC_GREEK[entry[1]]
      add(slot[1], slot[2], italic and slanted(slot[1]) and italic or entry[2])
    end
  end
  -- A typed character prints its slot's glyph: it settles commands sharing that glyph (\vert, \mid).
  local typed = {}
  character_slots(function(family, char, code)
    if not foreign(family) then return end
    local key = family * 256 + char
    if typed[key] == nil then typed[key] = code elseif typed[key] ~= code then typed[key] = false end
  end)
  -- LaTeX's * in math is the math asterisk: plain and LaTeX give it \ast's slot
  -- (\mathcode`*="2203), and a family may declare `*` and \ast on one slot (stix.sty:239-240).
  -- Its own vote (U+002A) must not turn the command's U+2217 back into "*" (papers2/0414).
  local ast = {}
  for _, slot in ipairs(slots('ast')) do ast[slot[1] * 256 + slot[2]] = true end
  -- flexisym (papers3/0757, pslatex) declares the symbols as macros: `*` is active and \ast reads
  -- "\@sym \ast \math_sym_Bin:Nn \mg@bin {03}", so neither a command nor a typed character votes
  -- for the slot and LuaMML writes the font's own (ASCII) asterisk. The declaration names the
  -- family (\mg@bin -> its number) and slot of the math asterisk.
  local flexisym_family, flexisym_slot = flexisym_asterisk()
  asterisks = {}
  if flexisym_family and foreign(flexisym_family) then
    ast[flexisym_family * 256 + flexisym_slot] = true
    asterisks[1] = {family = flexisym_family, slot = flexisym_slot}
  end
  for key, code in pairs(typed) do
    local family = key // 256
    local mapped = maps[family] and maps[family][key % 256]
    maps[family] = maps[family] or {}
    if code == 0x2A and (mapped == 0x2217 or (mapped == nil and ast[key])) then
      maps[family][key % 256] = 0x2217
    else
      maps[family][key % 256] = code
    end
  end
  if flexisym_family and foreign(flexisym_family) then
    maps[flexisym_family] = maps[flexisym_family] or {}
    maps[flexisym_family][flexisym_slot] = 0x2217
  end
  font_layouts(maps)
  named_layouts(maps)
  for family, map in pairs(maps) do
    local mapping = {}
    for char, code in pairs(map) do
      if code then mapping[char] = code end
    end
    convert.register_family(family, mapping)
    local entry = registered[family] or {mapping = {}}
    for char, code in pairs(mapping) do entry.mapping[char] = code end
    entry.fonts = family_fonts(family)
    registered[family] = entry
  end
end

-- LaTeX loads math fonts at the start of the first formula and again whenever a size change
-- (\footnotesize, \scriptsize, \textsuperscript) selects other sizes (\check@mathfonts): each
-- time the families are registered anew, before any formula is converted.
local installed
-- url.sty sets a URL's characters (`.`, `/`, `-`, ...) as family-0 slots of their own codes in a group and
-- only then opens the math (\Url@MathSetup): a first URL loads the math fonts with those mathcodes in
-- force, and their typed votes ("2E is '.', "2C is ',') overrode stix's operators (( "2E, \pm "2C), so
-- every later formula read $(x-y)$ as `. x * y /` (the size kept its signature, so nothing reinstalled).
-- No family is registered inside a URL (it reads family 0 plainly anyway); the first formula after it does.
local function inside_url()
  return token.is_defined('Url@String')
end

function M.install_once()
  if inside_url() then return end
  -- The families are registered anew only when a size change selects other fonts (the scan of the
  -- table costs milliseconds and a long paper changes size a thousand times).
  local signature = {}
  for family = 0, 15 do
    for size = 0, 2 do signature[#signature + 1] = tex.getfontoffamily(family, size) or 0 end
  end
  signature = table.concat(signature, ",")
  if signature == installed then return end
  installed = signature
  M.install()
end
return M
