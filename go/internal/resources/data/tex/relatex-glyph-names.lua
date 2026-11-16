-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- The glyph names of a TFM font's Type 1 file, as TeX Live ships them, and the character a name
-- stands for. A glyph's name is read through the font's line in pdftex.map: its encoding file
-- when the line re-encodes the font, else the AFM of its Type 1 file (marvosym.afm: slot 111 is
-- "Football"), else the AFM named after the TFM, else the Type 1 program's own built-in encoding
-- (tipa10.pfb has no AFM). A virtual font's slot that sets one glyph of a base font has that glyph's
-- name. A name stands for a character by the reviewed symbol names of relatex-symbol-glyphs.txt
-- (whose "none" names stand for none; a tfm:FONT/NAME entry holds for that font only), the Adobe
-- Glyph List (glyphlist.txt), TeX's extension of it (texglyphlist.txt: latticetop, circleS,
-- dotlessj), the kernel's tables for pdfTeX (glyphtounicode.tex, glyphtounicode-cmex.tex: mapsto,
-- summationtext, the extensible pieces), a uniXXXX or uXXXX[XX] name, or the Glyph List's rules for a suffix (aogonek.sc,
-- hyphen.alt; not .superior or .inferior, which change the character) and a ligature (f_i).
-- A private-use value is no reading (the Glyph List's dotlessj F6BE).
local M = {}

local fonts, agl, tex_list, pdftex_list, map, reviewed = {}, nil, nil, nil, nil, nil

local function read(path)
  local handle = path and io.open(path, 'rb')
  if not handle then return nil end
  local text = handle:read('a')
  handle:close()
  return text
end

-- A printable character: no control (C0, DEL, C1), surrogate, private-use or out-of-range code.
local function printable(code)
  return code and code >= 0x20 and not (code >= 0x7F and code < 0xA0)
    and not (code >= 0xD800 and code <= 0xDFFF) and not (code >= 0xE000 and code <= 0xF8FF)
    and code < 0xF0000 and code <= 0x10FFFF
end

local function encoding_names(path)
  local text = read(path)
  local body = text and text:gsub('%%[^\n]*', ''):match('%[(.-)%]')
  if not body then return nil end
  local names, slot = {}, 0
  for name in body:gmatch('/([^%s/%[%]]+)') do
    names[slot] = name
    slot = slot + 1
  end
  return names
end

local function afm_names(path)
  local text = read(path)
  if not text then return nil end
  local names = {}
  for slot, name in text:gmatch('\nC (%d+) ;[^\n]-N ([^%s;]+)') do names[tonumber(slot)] = name end
  return names
end

-- The built-in encoding of a Type 1 program (pfb or pfa): its clear text's "dup SLOT /NAME put".
local function program_names(path)
  local text = read(path)
  if not text then return nil end
  local clear = text:sub(1, (text:find('eexec', 1, true) or #text))
  if clear:find('/Encoding StandardEncoding', 1, true) then return nil end
  local names, found = {}, false
  for slot, name in clear:gmatch('dup (%d+) */([^%s/]+) put') do
    names[tonumber(slot)] = name
    found = true
  end
  return found and names or nil
end

-- The font's pdftex.map line (LuaTeX's default map), read once and only for a font that needs it.
local function map_line(tfm)
  if map == nil then map = read(kpse.find_file('pdftex.map', 'map')) or false end
  if not map then return nil end
  return ('\n' .. map):match('\n' .. tfm:gsub('%p', '%%%0') .. '%s[^\n]*')
end

local function tfm_names(tfm)
  local line = map_line(tfm)
  if line then
    local encoding = line:match('<%[?([^%s<%[]+%.enc)')
    if encoding then return encoding_names(kpse.find_file(encoding, 'enc files')) or {} end
    local file = line:match('<<?([^%s<]+)%.pf[ab]')
    local names = file and afm_names(kpse.find_file(file .. '.afm', 'afm'))
    if names then return names end
    names = afm_names(kpse.find_file(tfm .. '.afm', 'afm'))
    if names then return names end
    local program = line:match('<<?([^%s<]+%.pf[ab])')
    return program and program_names(kpse.find_file(program, 'type1 fonts')) or {}
  end
  -- A virtual font (LuaTeX loads only its metrics; the backend reads the .vf): its packets.
  local vf = kpse.find_file(tfm, 'vf') and font.read_vf and font.read_vf(tfm, 655360)
  if vf and vf.characters then return {virtual = vf} end
  return afm_names(kpse.find_file(tfm .. '.afm', 'afm')) or {}
end

-- The one base glyph a virtual font's character sets: the base font's TFM name and slot, or nil.
local function base_glyph(vf, slot)
  local info = vf.characters[slot]
  local commands = info and info.commands
  if not commands or not vf.fonts then return nil end
  local current, found = 1, nil
  for _, command in ipairs(commands) do
    local kind = command[1]
    if kind == 'font' then
      current = command[2]
    elseif kind == 'char' or kind == 'slot' then
      if found then return nil end
      found = kind == 'slot' and {command[2], command[3]} or {current, command[2]}
    elseif kind == 'rule' or kind == 'image' or kind == 'node' or kind == 'special' then
      return nil
    end
  end
  local base = found and vf.fonts[found[1]]
  return base and base.name and {base.name, found[2]}
end

local function names_in(tfm)
  local names = fonts[tfm]
  if not names then
    names = tfm_names(tfm)
    fonts[tfm] = names
  end
  return names
end

local function name_in(tfm, slot, depth)
  local names = names_in(tfm)
  if not names.virtual then return names[slot], tfm end
  local base = depth < 4 and base_glyph(names.virtual, slot)
  if not base then return nil end
  local name, base_tfm = name_in(base[1], base[2], depth + 1)
  return name, base_tfm, true
end

-- The name of the glyph at slot of font id, or nil; the TFM name of the font that holds the glyph
-- (a virtual font's base font) and, third, true when it was reached through a virtual font.
function M.name(id, slot)
  local f = font.getfont(id)
  local tfm = f and f.name
  if type(tfm) ~= 'string' or tfm == '' then return nil end
  return name_in(tfm, slot, 0)
end

-- A list of names and code points: glyphlist.txt's NAME;HEX HEX lines, or pdfTeX's
-- \pdfglyphtounicode{NAME}{HEX HEX} (the kernel's glyphtounicode.tex, glyphtounicode-cmex.tex). A
-- variation selector (the cmex sizes: parenleftbig is '(' with U+FE01) is left out; a name with an
-- unprintable or private-use value is no entry.
local function load_list(file, format, pattern, list)
  list = list or {}
  local text = read(kpse.find_file(file, format)) or ''
  for name, codes in text:gmatch(pattern) do
    local chars = {}
    for hex in codes:gmatch('%x+') do
      local code = tonumber(hex, 16)
      if not (code >= 0xFE00 and code <= 0xFE0F) then
        if not printable(code) then chars = nil break end
        chars[#chars + 1] = utf8.char(code)
      end
    end
    if chars and #chars > 0 and list[name] == nil then list[name] = table.concat(chars) end
  end
  return list
end

local GLYPHLIST = '\n([%w_.]+);([%x ]+)'
local PDFTEX = '\\pdfglyphtounicode{([^}]+)}{([%x ]+)}'

local function load_lists()
  agl = load_list('glyphlist.txt', 'map', GLYPHLIST)
  tex_list = load_list('texglyphlist.txt', 'map', GLYPHLIST)
  pdftex_list = load_list('glyphtounicode-cmex.tex', 'tex', PDFTEX)
  load_list('glyphtounicode.tex', 'tex', PDFTEX, pdftex_list)
end

-- relatex-symbol-glyphs.txt beside this module (the collector's directory, on TEXINPUTS): name ->
-- character, or false for a name that paints none.
local function load_reviewed()
  reviewed = {}
  local text = read(kpse.find_file('relatex-symbol-glyphs.txt'))
  assert(text, 'relatex-symbol-glyphs.txt is missing')
  for name, value in text:gmatch('\n([^#%s;][^%s;]*);(%w+)') do
    local code = value ~= 'none' and tonumber(value, 16)
    reviewed[name] = printable(code) and utf8.char(code) or false
  end
end

local plain
-- A suffix that changes the character (a superior figure is no plain one).
local SEMANTIC = {superior = true, sups = true, inferior = true, sinf = true, numerator = true, numr = true,
  denominator = true, dnom = true}

-- A figure's superior and inferior forms (one.superior, two.inferior) are its super- and subscript.
local DIGITS = {zero = 0, one = 1, two = 2, three = 3, four = 4, five = 5, six = 6, seven = 7, eight = 8, nine = 9}
local SUPERIOR = {[0] = 0x2070, 0xB9, 0xB2, 0xB3, 0x2074, 0x2075, 0x2076, 0x2077, 0x2078, 0x2079}

local function composed(name)
  local base, suffix = name:match('^([^.]+)%.([^.]+)')
  local digit = base and DIGITS[base]
  if digit and (suffix == 'superior' or suffix == 'sups') then return utf8.char(SUPERIOR[digit]) end
  if digit and (suffix == 'inferior' or suffix == 'sinf') then return utf8.char(0x2080 + digit) end
  if suffix and SEMANTIC[suffix] then return nil end
  base = base or name
  if not base:find('_', 1, true) then return base ~= name and plain(base) or nil end
  local parts = {}
  for part in base:gmatch('[^_]+') do
    local text = plain(part)
    if not text then return nil end
    parts[#parts + 1] = text
  end
  return table.concat(parts)
end

plain = function(name)
  if reviewed[name] ~= nil then return reviewed[name] or nil end
  if agl[name] then return agl[name] end
  if tex_list[name] then return tex_list[name] end
  if pdftex_list[name] then return pdftex_list[name] end
  local hex = name:match('^uni(%x%x%x%x)$') or name:match('^u(%x%x%x%x%x?%x?)$')
  local code = hex and tonumber(hex, 16)
  return printable(code) and utf8.char(code) or nil
end

-- The character a glyph name stands for (in font tfm, when given), or nil; false when the reviewed
-- names say it paints none.
function M.unicode(name, tfm)
  if type(name) ~= 'string' then return nil end
  if reviewed == nil then load_reviewed() end
  if agl == nil then load_lists() end
  local qualified = tfm and reviewed['tfm:' .. tfm .. '/' .. name]
  if qualified ~= nil then return qualified end
  if reviewed[name] ~= nil then return reviewed[name] end
  return plain(name) or composed(name)
end

-- The characters a tagged PDF's ToUnicode map gives glyph NAME of font TFM: the reviewed names
-- first; true where the kernel's glyphtounicode tables, which the backend spells the ordinary PDF
-- with, give a printable value (fi as "fi", not the Adobe Glyph List's U+FB01; cmex's parenleftbig
-- with its variation selector), so that entry stands; else the collector's reading (M.unicode), or
-- nil. A glyph the reviewed names call none keeps a published table's character (cwm: U+200C) or
-- has none.
function M.tounicode(name, tfm)
  if type(name) ~= 'string' then return nil end
  if reviewed == nil then load_reviewed() end
  if agl == nil then load_lists() end
  local value = tfm and reviewed['tfm:' .. tfm .. '/' .. name]
  if value == nil then value = reviewed[name] end
  if value then return value end
  if pdftex_list[name] then return true end
  if value == false then return agl[name] or tex_list[name] end
  return M.unicode(name, tfm) or nil
end

return M
