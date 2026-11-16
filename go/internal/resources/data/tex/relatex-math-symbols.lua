-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Bounded legacy symbols: live mathchar declarations and engine font bindings.
-- Font names and numeric glyph slots alone never establish Unicode semantics.
local M = {}
local square_attribute
local square_stack, square_bindings = {}, {}
local declarations = {
  [15] = {{'bullet', 'symsymbols', 2, 0x2022}},
  [1] = {{'cdot', 'symsymbols', 2, 0x22C5}},
  [3] = {{'ast', 'symsymbols', 2, 0x2217}, {'square', 'symAMSa', 0, 0x25A1}},
  [121] = {{'dagger', 'symsymbols', 2, 0x2020}},
  [122] = {{'ddagger', 'symsymbols', 2, 0x2021}},
  -- newtx (newtxmath, newtxsf) set \square from its AMSm font ntxsym, slot 131; its \textsquare
  -- (the \openbox of amsthm and Beamer proofs) selects that very font as text.
  [131] = {{'square', 'symAMSm', 0, 0x25A1}}
}

local function declaration(entry, slot)
  local command, family = token.create(entry[1]), token.create(entry[2])
  if command.cmdname ~= 'math_given' or family.cmdname ~= 'char_given'
      or type(family.mode) ~= 'number' or family.mode % 1 ~= 0 or family.mode < 0 or family.mode > 15 then return end
  if command.mode ~= entry[3] * 4096 + family.mode * 256 + slot then return end
  return family.mode, command.mode
end

-- QED can be boxed at one font size and decoded after its group has ended.
-- Keep only scalar bindings observed at that execution, attached to its nodes.
-- This also preserves provenance on copied boxes without retaining node pointers.
function M.text_square_begin()
  if not square_attribute then square_attribute = luatexbase.new_attribute('relatex.math.text_square') end
  square_stack[#square_stack + 1] = tex.getattribute(square_attribute) or -2147483647
  local family, mathcode = declaration({'square', 'symAMSa', 0, 0x25A1}, 3)
  local binding = {family = family, mathcode = mathcode, fonts = {}, text_fonts = {}, conflicts = {}}
  if family then
    for style = 0, 2 do
      local id = tex.getfontoffamily(family, style)
      if id and id > 0 then binding.fonts[id] = style end
    end
  end
  local other_family = declaration(declarations[3][1], 3)
  if other_family then
    for style = 0, 2 do
      local id = tex.getfontoffamily(other_family, style)
      if binding.fonts[id] ~= nil then binding.conflicts[id] = true end
    end
  end
  square_bindings[#square_bindings + 1] = binding
  tex.setattribute(square_attribute, #square_bindings)
end

-- Called only inside the exactly guarded textsquare body, after its actual
-- usefont{U}{msa}{m}{n}. Do not initialize math fonts just to observe a text
-- glyph: LaTeX globally changes script sizes during that initialization.
function M.text_square_font()
  local id = square_attribute and tex.getattribute(square_attribute)
  local binding = id and square_bindings[id]
  if not binding or not binding.family then return end
  for name, value in pairs({['f@encoding']='U', ['f@family']='msa', ['f@series']='m', ['f@shape']='n'}) do
    if token.get_macro(name) ~= value then return end
  end
  local selected = font.current()
  local f = font.getfont(selected)
  if selected <= 0 or not f or not f.characters or not f.characters[3] then return end
  local other_family = declaration(declarations[3][1], 3)
  if other_family then
    for style = 0, 2 do
      if tex.getfontoffamily(other_family, style) == selected then binding.conflicts[selected] = true end
    end
  end
  binding.text_fonts[selected] = true
end

function M.text_square_end()
  assert(#square_stack > 0, 'Unbalanced text-square context')
  tex.setattribute(square_attribute, table.remove(square_stack))
end

local function bound_square(n, id)
  local binding = square_bindings[id]
  if binding and binding.conflicts[n.font] then return nil, 'conflict' end
  if binding and n.char == 3 and binding.text_fonts[n.font] then
    return utf8.char(0x25A1), {source = 'guarded-ams-textsquare', command = 'textsquare',
      family_command = 'symAMSa', family = binding.family, mathcode = binding.mathcode,
      encoding = 'U', font_family = 'msa', series = 'm', shape = 'n',
      font = n.font, char = n.char, text = utf8.char(0x25A1), binding = 'qed-execution'}
  end
  local style = binding and binding.fonts[n.font]
  if n.char ~= 3 or style == nil then return end
  return utf8.char(0x25A1), {source = 'luatex-mathchar-family', command = 'square', family_command = 'symAMSa',
    family = binding.family, mathcode = binding.mathcode, style = style, font = n.font, char = n.char,
    text = utf8.char(0x25A1), binding = 'qed-execution'}
end

-- Punctuation typed in math (the period of an \eqno number) is set from the math italic slot its
-- live \mathcode names: slot 0x3A for ".", which a text reading would take for ":". The slot
-- counts only when that mathcode points at this very font through its family.
local mathcode_punctuation = {[0x3A] = 0x2E, [0x3B] = 0x2C, [0x3C] = 0x3C, [0x3D] = 0x2F, [0x3E] = 0x3E}

local function mathcode_symbol(n)
  local typed = mathcode_punctuation[n.char]
  if not typed or not tex.getmathcode then return end
  local code = tex.getmathcode(typed)
  local family = type(code) == 'table' and code[2]
  if not family or code[3] ~= n.char then return end
  for style = 0, 2 do
    if tex.getfontoffamily(family, style) == n.font then
      local text = utf8.char(typed)
      return text, {source = 'luatex-mathcode-family', command = text, family = family, mathcode = code[1] * 4096 + family * 256 + n.char,
        style = style, font = n.font, char = n.char, text = text}
    end
  end
end

function M.decode(n)
  if not token or not tex or not tex.getfontoffamily then return end
  local f = font and font.getfont(n.font)
  if n.font <= 0 or not f or not f.characters or not f.characters[n.char] then return end
  local binding = square_attribute and node.has_attribute(n, square_attribute)
  if binding then return bound_square(n, binding) end
  local found, evidence
  for _, entry in ipairs(declarations[n.char] or {}) do
    local family, mathcode = declaration(entry, n.char)
    if family then
      for style = 0, 2 do
        if tex.getfontoffamily(family, style) == n.font then
          local text = utf8.char(entry[4])
          if found and found ~= text then return nil, 'conflict' end
          found = text
          evidence = {source = 'luatex-mathchar-family', command = entry[1], family_command = entry[2],
            family = family, mathcode = mathcode, style = style, font = n.font, char = n.char, text = text}
        end
      end
    end
  end
  if found == nil then return mathcode_symbol(n) end
  return found, evidence
end

return M
