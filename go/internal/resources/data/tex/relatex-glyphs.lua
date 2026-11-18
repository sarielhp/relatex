-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Decode authoritative font Unicode values or the engine's Latin ligature input.
-- Node references are inspected only during the call; none are retained.
local M = {}
local math_symbols = require('relatex-math-symbols')
local text_symbols = require('relatex-text-symbols')
local glyph = node.id('glyph')

local function scalar(code)
  return type(code) == 'number' and code % 1 == 0 and code >= 0 and code <= 0x10FFFF
    and not (code >= 0xD800 and code <= 0xDFFF)
end

function M.unicode(value)
  if type(value) == 'number' then return scalar(value) and utf8.char(value) or nil end
  local chars = {}
  if type(value) == 'table' then
    local count = 0
    for index, code in pairs(value) do
      if type(index) ~= 'number' or index % 1 ~= 0 or index < 1 or index > #value or not scalar(code) then return nil end
      count = count + 1
    end
    if count == 0 or count ~= #value then return nil end
    for _, code in ipairs(value) do chars[#chars + 1] = utf8.char(code) end
    return table.concat(chars)
  end
  if type(value) ~= 'string' or #value == 0 or #value % 4 ~= 0 or value:find('[^%x]') then return nil end
  local position = 1
  while position <= #value do
    local code = tonumber(value:sub(position, position + 3), 16)
    position = position + 4
    if code >= 0xD800 and code <= 0xDBFF then
      local low = tonumber(value:sub(position, position + 3), 16)
      if not low or low < 0xDC00 or low > 0xDFFF then return nil end
      code = 0x10000 + (code - 0xD800) * 0x400 + low - 0xDC00
      position = position + 4
    elseif code >= 0xDC00 and code <= 0xDFFF then
      return nil
    end
    chars[#chars + 1] = utf8.char(code)
  end
  return table.concat(chars)
end

-- The TeX font ligatures of punctuation (the lig table of every text font, in any encoding): a run
-- of hyphens is an en or em dash, two grave accents or two apostrophes a double quotation mark.
-- A component keys here only as its own input character, so a single '-' or "'" never maps.
local PUNCTUATION_LIGATURES = {
  ['--'] = utf8.char(0x2013), ['---'] = utf8.char(0x2014), [utf8.char(0x2013) .. '-'] = utf8.char(0x2014),
  ['``'] = utf8.char(0x201C), ["''"] = utf8.char(0x201D),
  -- An OT1 font's typed quotes read as the curly ones (relatex-text-symbols' quote slots).
  [utf8.char(0x2018, 0x2018)] = utf8.char(0x201C), [utf8.char(0x2019, 0x2019)] = utf8.char(0x201D)
}
local LIGATURE_COMPONENTS = {['-'] = true, ['`'] = true, ["'"] = true, [utf8.char(0x2013)] = true,
  [utf8.char(0x2018)] = true, [utf8.char(0x2019)] = true}

local function ligature(n, depth)
  if depth >= 8 or (n.subtype & 2) == 0 or not n.components then return nil end
  local chars, count = {}, 0
  for component in node.traverse(n.components) do
    count = count + 1
    if count > 16 or component.id ~= glyph or component.font ~= n.font then return nil end
    local text = M.character(component, depth + 1)
    -- Letter ligatures preserve their input spelling. Other punctuation substitutions
    -- and encodings need their own Unicode evidence, not this fallback.
    if not text or text == '' or (text:find('[^A-Za-z]') and not LIGATURE_COMPONENTS[text]) then return nil end
    chars[#chars + 1] = text
  end
  if count < 2 then return nil end
  local joined = table.concat(chars)
  if joined:find('[^A-Za-z]') then return PUNCTUATION_LIGATURES[joined] end
  return joined
end

function M.character(n, depth)
  -- Only a glyph has a character: math in a link (hyperref's \hyperref around \overline inside $...$) can hand
  -- over a node without a font or a char, which no decoder can read.
  if not n.font or not n.char then return nil end
  local f = font.getfont(n.font)
  local info = f and f.characters and f.characters[n.char]
  if info and info.tounicode ~= nil then return M.unicode(info.tounicode), 'font' end
  local symbol, evidence = math_symbols.decode(n)
  if evidence == 'conflict' then return nil, 'math-declaration-conflict' end
  if symbol then return symbol, 'math-symbol', evidence end
  local encoded, declaration, unmapped = text_symbols.decode(n)
  if declaration == 'conflict' then return nil, 'text-declaration-conflict' end
  -- A known symbol font's unlisted glyph has no text: its slot is no character (marvosym's
  -- \Football is no "o", its \NoWash no "Ý").
  if declaration == 'unmapped-symbol' then return nil, 'symbol-font', unmapped end
  if encoded then return encoded, 'text-symbol', declaration end
  local text = ligature(n, depth or 0)
  if text then return text, 'components' end
  -- A glyph its font's Type 1 names (decision 104); a ligature glyph set alone (\char12) included.
  local named, unread = text_symbols.named(n)
  if named then return named, 'glyph-name' end
  if unread then
    return nil, 'symbol-font', {source='type1-glyph-name', font=n.font, char=n.char,
      name=type(unread) == 'string' and unread or nil}
  end
  if ((n.char >= 32 and n.char < 127) or n.char >= 160) and n.char < 0xE000 then return utf8.char(n.char), 'slot' end
  return nil
end

return M
