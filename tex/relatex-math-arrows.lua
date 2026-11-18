-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Arrows TeX sets from pieces are one operator of the document: plain's \hookrightarrow is
-- \lhook\joinrel\rightarrow (the hook, slot 0x2C of the letters font, a negative space, the
-- arrow) and amssymb's \dashrightarrow is \dabar@\dabar@ and a head (slots 0x39, 0x39 and 0x4B
-- of the AMS symbol font). LuaMML reads the pieces by their raw slots (a comma, "99" and "K");
-- the sequence becomes the arrow's Unicode character, as the PDF text reads it.
local M = {}

local function char_of(node)
  local nodes = type(node) == 'table' and node[':nodes']
  local first = nodes and nodes[1]
  return first and first.char
end

-- The text of an element: its string children joined (LuaMML keeps a number's digits apart).
local function text_of(node)
  local parts = {}
  for _, child in ipairs(node) do
    if type(child) == 'string' then parts[#parts + 1] = child end
  end
  return table.concat(parts)
end

-- A piece: an element of one of the kinds, in a font family other than the operators (family 0
-- carries no tex:family), spelled as text when given, and typeset from the slot when known.
local function piece(node, kinds, text, slot)
  if type(node) ~= 'table' or not kinds[node[0]] or node['tex:family'] == nil then return false end
  if text and text_of(node) ~= text then return false end
  local actual = char_of(node)
  if slot and text == nil and actual == nil then return false end
  return actual == nil or actual == slot
end

local function space(node) return type(node) == 'table' and node[0] == 'mspace' end
local function arrow(node, text) return type(node) == 'table' and node[0] == 'mo' and node[1] == text end

local MO = {mo = true}
local MI = {mi = true, mo = true}
local MN = {mn = true, mi = true, mo = true}

local function backspace(node)
  return space(node) and tostring(node.width or ''):match('^%-') ~= nil
end

local function forwardspace(node)
  return space(node) and node.width ~= nil and tostring(node.width):match('^%-') == nil
end

-- The single mo of a cell of the congruence stack, through the row, cell and style wrappers
-- LuaMML puts between the table and the glyph.
local WRAPPERS = {mrow = true, mstyle = true, mtd = true, mtr = true}
local function sole_mo(node)
  local found
  for _, child in ipairs(node) do
    if type(child) == 'string' then
      if child:match('%S') then return nil end
    elseif type(child) == 'table' then
      local inner
      if child[0] == 'mo' then
        inner = child
      elseif WRAPPERS[child[0]] then
        inner = sole_mo(child)
      else
        return nil
      end
      if inner == nil or found ~= nil then return nil end
      found = inner
    else
      return nil
    end
  end
  return found
end

-- \cong is the table of the similar sign above the equals; a real matrix or array stays a table.
-- The table has no relation class of its own: the exact two single-glyph cells are the signature.
local function congruent(node)
  if type(node) ~= 'table' or node[0] ~= 'mtable' or #node ~= 2 then return false end
  local texts = {}
  for row = 1, 2 do
    local mtr = node[row]
    if type(mtr) ~= 'table' or mtr[0] ~= 'mtr' or #mtr ~= 1 then return false end
    local mtd = mtr[1]
    if type(mtd) ~= 'table' or mtd[0] ~= 'mtd' or #mtd ~= 1 then return false end
    local mo = sole_mo(mtd[1])
    if mo == nil then return false end
    texts[row] = text_of(mo)
  end
  return texts[1] == '∼' and texts[2] == '='
end

-- esvect's \vv{x} is a two-row table like \overrightarrow: the arrow (a shaft end, a backed-up space and a head of the
-- vect font) over the base (so is amsmath's \overrightarrow). It is the vector accent: the base under a U+2192 accent, as \vec reads.
local function vector_arrow(node, state)
  if type(node) ~= 'table' then return true end
  if node[0] == 'mo' then
    local text = text_of(node)
    if text == '→' then
      state.heads = state.heads + 1
      return true
    end
    return text == '−'
  end
  if not WRAPPERS[node[0]] and node[0] ~= 'mpadded' and node[0] ~= 'mspace' then return false end
  for _, child in ipairs(node) do
    if type(child) == 'table' and not vector_arrow(child, state) then return false end
  end
  return true
end

local function vector_accent(node)
  if type(node) ~= 'table' or node[0] ~= 'mtable' or #node ~= 2 then return nil end
  local top, bottom = node[1], node[2]
  if type(top) ~= 'table' or top[0] ~= 'mtr' or #top ~= 1 or type(bottom) ~= 'table' or bottom[0] ~= 'mtr' or #bottom ~= 1 then
    return nil
  end
  local state = {heads = 0}
  if not vector_arrow(top[1], state) or state.heads ~= 1 then return nil end
  local base = bottom[1]
  local content = {[0] = 'mrow'}
  for _, child in ipairs(type(base) == 'table' and base or {}) do content[#content + 1] = child end
  return {[0] = 'mover', accent = 'true', content,
    {[0] = 'mo', '→', stretchy = 'true', [':nodes'] = {}}}
end

local PATTERNS = {
  -- amsmath's \leftrightarrowfill@ (\overleftrightarrow): two heads, each backed up 7mu, and a leader of
  -- \relbar between them that LuaMML does not read; the PDF paints the shaft, one minus as the
  -- document spells an extensible arrow's shaft.
  {4, function(c, i) return arrow(c[i], '←') and backspace(c[i + 1]) and backspace(c[i + 2]) and arrow(c[i + 3], '→') end,
    '←−→'},
  {3, function(c, i) return piece(c[i], MO, nil, 0x2C) and space(c[i + 1]) and arrow(c[i + 2], '→') end, '↪'},
  {3, function(c, i) return arrow(c[i], '←') and space(c[i + 1]) and piece(c[i + 2], MO, nil, 0x2D) end, '↩'},
  {2, function(c, i) return piece(c[i], MN, '99', 0x39) and piece(c[i + 1], MI, 'K', 0x4B) end, '⇢'},
  {2, function(c, i) return piece(c[i], MI, 'L', 0x4C) and piece(c[i + 1], MN, '99', 0x39) end, '⇠'},
  -- \overbrace is \downbracefill: the brace tips of the extension font (slots 0x7A, 0x7D, 0x7C,
  -- 0x7B, read as ⎰ } | ⎱) around two leaders rules; \underbrace is \upbracefill, the tips
  -- the other way up (0x7C, 0x7B, 0x7A, 0x7D: | ⎱ ⎰ }).
  {4, function(c, i)
    return piece(c[i], MI, '⎰', 0x7A) and piece(c[i + 1], MI, '}', 0x7D) and piece(c[i + 2], MI, '|', 0x7C) and
      piece(c[i + 3], MI, '⎱', 0x7B)
  end, '⏞'},
  {4, function(c, i)
    return piece(c[i], MI, '|', 0x7C) and piece(c[i + 1], MI, '⎱', 0x7B) and piece(c[i + 2], MI, '⎰', 0x7A) and
      piece(c[i + 3], MI, '}', 0x7D)
  end, '⏟'},
  -- \longmapsto: the mapsto glyph, a rule and a backed-up arrow head (the rule may be gone); \mapsto is glyph and head.
  {4, function(c, i)
    return arrow(c[i], '↦') and type(c[i + 1]) == 'table' and c[i + 1][0] ~= 'mo' and
      c[i + 1][0] ~= 'mspace' and backspace(c[i + 2]) and arrow(c[i + 3], '→')
  end, '⟼'},
  {3, function(c, i) return arrow(c[i], '↦') and backspace(c[i + 1]) and arrow(c[i + 2], '→') end, '⟼'},
  {2, function(c, i) return arrow(c[i], '↦') and arrow(c[i + 1], '→') end, '↦'},
  -- \Longrightarrow is \Relbar\joinrel\Rightarrow; \Longleftrightarrow joins two heads and \Longleftarrow a head and \Relbar.
  {4, function(c, i) return arrow(c[i], '=') and backspace(c[i + 1]) and backspace(c[i + 2]) and arrow(c[i + 3], '⇒') end, '⟹'},
  {3, function(c, i) return arrow(c[i], '=') and backspace(c[i + 1]) and arrow(c[i + 2], '⇒') end, '⟹'},
  {4, function(c, i) return arrow(c[i], '⇐') and backspace(c[i + 1]) and backspace(c[i + 2]) and arrow(c[i + 3], '⇒') end, '⟺'},
  {3, function(c, i) return arrow(c[i], '⇐') and backspace(c[i + 1]) and arrow(c[i + 2], '⇒') end, '⟺'},
  {4, function(c, i) return arrow(c[i], '⇐') and backspace(c[i + 1]) and backspace(c[i + 2]) and arrow(c[i + 3], '=') end, '⟸'},
  {3, function(c, i) return arrow(c[i], '⇐') and backspace(c[i + 1]) and arrow(c[i + 2], '=') end, '⟸'},
  {5, function(c, i) return arrow(c[i], '⊥') and backspace(c[i + 1]) and backspace(c[i + 2]) and backspace(c[i + 3]) and arrow(c[i + 4], '⊥') end, '⫫'},
  {4, function(c, i) return arrow(c[i], '⊥') and backspace(c[i + 1]) and backspace(c[i + 2]) and arrow(c[i + 3], '⊥') end, '⫫'},
  {3, function(c, i) return arrow(c[i], '⊥') and backspace(c[i + 1]) and arrow(c[i + 2], '⊥') end, '⫫'},
}

local function composed(tree, index, length, text)
  local braces = {['⏞'] = true, ['⏟'] = true}
  local result = {[0] = 'mo', text, stretchy = braces[text] and 'true' or 'false', [':nodes'] = {}}
  for offset = 0, length - 1 do
    local node = tree[index + offset]
    if node[0] == 'mo' then
      result.lspace = result.lspace or node.lspace
      if node.rspace ~= nil then result.rspace = node.rspace end
    end
    result['tex:family'] = result['tex:family'] or node['tex:family']
    result[':style'] = result[':style'] or node[':style']
    for _, item in ipairs(node[':nodes'] or {}) do result[':nodes'][#result[':nodes'] + 1] = item end
  end
  return result
end

-- Replace a congruence table by its character; the positive mspaces around it become its lspace and rspace.
local function compose_congruence(tree, index)
  local left = forwardspace(tree[index - 1]) and tree[index - 1].width
  local right = forwardspace(tree[index + 1]) and tree[index + 1].width
  local result = composed(tree, index, 1, '≅')
  result.lspace = left or result.lspace
  result.rspace = right or result.rspace
  if left then table.remove(tree, index - 1); index = index - 1 end
  if right then table.remove(tree, index + 1) end
  tree[index] = result
  return index
end

-- Replace each piece sequence among the children of every element by its arrow, in place.
function M.compose(tree)
  if type(tree) ~= 'table' then return tree end
  local index = 1
  while index <= #tree do
    local matched = false
    if congruent(tree[index]) then
      index = compose_congruence(tree, index)
      matched = true
    end
local vector = not matched and vector_accent(tree[index])
if vector then
  tree[index] = vector
  matched = true
end
    if not matched then
      for _, pattern in ipairs(PATTERNS) do
        local length, test, text = pattern[1], pattern[2], pattern[3]
        if index + length - 1 <= #tree and test(tree, index) then
          local arrow_node = composed(tree, index, length, text)
          for _ = 2, length do table.remove(tree, index + 1) end
          tree[index] = arrow_node
          matched = true
          break
        end
      end
    end
    if not matched then M.compose(tree[index]) end
    index = index + 1
  end
  return tree
end

return M
