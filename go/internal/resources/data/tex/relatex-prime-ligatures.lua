-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Decision 149: a math font may join primes into one glyph. mathabx's matha font has the ligature
-- program "31 "31 =: "32, "32 "31 =: "33 and "33 "31 =: "34 (\prime, \second, \third, \fourth), so
-- $S''$ prints one double prime. TeX applies a font's ligature program in math as in text
-- (make_ord): an ordinary atom with no scripts and a math character, followed by an atom whose
-- math character is of the same family, takes the ligature the font sets for the pair; the joined
-- atom takes the second atom's scripts and tries again with the atom after it.
--
-- This module is that rule restricted to \prime atoms, read from the font itself (its ligature
-- table at the size the atoms are set), and shared by both sides: the product writes the joined
-- primes as one character (relatex-external-math.lua) and the ordinary build's math oracle records
-- the runs the font joined (relatex-oracle.lua), which the independent math check applies to the
-- primes it reads from the source. A font without the ligature keeps every prime.
--
-- n primes joined are the n-fold prime, the character whose compatibility decomposition is those n
-- primes (U+2033 is U+2032 U+2032).
local M = {}

M.MULTIPLE = {[1] = 0x2032, [2] = 0x2033, [3] = 0x2034, [4] = 0x2057}

local noad_id, math_char_id, sub_mlist_id, style_id, choice_id
-- The atoms TeX's make_ord lets follow an ordinary atom into a ligature (ord to punct).
local follower = {}
local ord
local function kinds()
  if noad_id then return end
  noad_id, math_char_id, sub_mlist_id = node.id('noad'), node.id('math_char'), node.id('sub_mlist')
  style_id, choice_id = node.id('style'), node.id('choice')
  for subtype, name in pairs(node.subtypes('noad')) do
    if name == 'ord' then ord = subtype end
    if name == 'ord' or name:match('^op') or name == 'bin' or name == 'rel' or name == 'open'
        or name == 'close' or name == 'punct' then follower[subtype] = true end
  end
end

-- The family and slot of \prime when it is a math character (\mathchardef, as LaTeX and mathabx
-- declare it); nil when \prime is a macro or undefined.
function M.prime()
  local command = token.create('prime')
  if command.cmdname ~= 'math_given' then return nil end
  return (command.mode >> 8) & 0xF, command.mode & 0xFF
end

-- Styles by index: 0 display, 1 text, 2 script, 3 scriptscript; the font size of each.
local SIZE = {[0] = 0, 0, 1, 2}
local function style_index(name)
  name = tostring(name or 'text'):gsub('^cramped', '')
  if name == 'display' then return 0 elseif name == 'script' then return 2
  elseif name == 'scriptscript' then return 3 end
  return 1
end
local function script_style(style) return style < 2 and 2 or 3 end
local function fraction_style(style) return style == 0 and 1 or style == 1 and 2 or 3 end

-- The glyph the font sets for `left` followed by `right` (a plain =: ligature), or nil.
local faces = {}
local function face(id)
  if faces[id] == nil then
    local found = font.getfont(id) or (font.getcopy and font.getcopy(id))
    faces[id] = found or false
  end
  return faces[id] or nil
end
local function ligature(id, left, right)
  local found = id and id > 0 and face(id)
  local character = found and found.characters and found.characters[left]
  local entry = character and character.ligatures and character.ligatures[right]
  if entry and (entry.type or 0) == 0 then return entry.char end
end

-- The prime's family and slot when a font of its family (at any size) joins two primes.
local function joining_prime()
  local family, slot = M.prime()
  if not family then return nil end
  for size = 0, 2 do
    if ligature(tex.getfontoffamily(family, size), slot, slot) then return family, slot end
  end
end

local function prime_atom(n, family, slot)
  local nucleus = n.id == noad_id and n.nucleus
  return nucleus and nucleus.id == math_char_id and nucleus.fam == family and nucleus.char == slot
end

-- The pieces TeX makes of a run of prime atoms (only the last may have scripts): from the first
-- atom, each following prime joins while the font has the ligature; the next atom starts a new
-- piece if it is an ordinary atom without scripts. Returns the lengths, in order.
local function pieces(run, font_id, slot)
  local result = {}
  local index = 1
  while index <= #run do
    local first = run[index]
    local length = 1
    if first.subtype == ord and not first.sub and not first.sup then
      local glyph = slot
      while index + length <= #run and M.MULTIPLE[length + 1] do
        local joined = ligature(font_id, glyph, slot)
        if not joined then break end
        glyph, length = joined, length + 1
        local taken = run[index + length - 1]
        if taken.sub or taken.sup then break end
      end
    end
    result[#result + 1] = length
    index = index + length
  end
  return result
end

local walk

-- Every list a noad holds, with the style it is set in.
local FIELDS = {'nucleus', 'sub', 'sup', 'num', 'denom', 'degree'}
local function children(n, style, prime, visit)
  local function into(value, child_style)
    if not value or not node.is_node(value) then return end
    if value.id == sub_mlist_id then
      if value.list then walk(value.list, child_style, prime, visit) end
    elseif value.id == noad_id or value.id == style_id or value.id == choice_id then
      walk(value, child_style, prime, visit)
    end
  end
  if n.id == choice_id then
    into(n.display, 0); into(n.text, 1); into(n.script, 2); into(n.scriptscript, 3)
    return
  end
  local styles = {nucleus = style, sub = script_style(style), sup = script_style(style),
    num = fraction_style(style), denom = fraction_style(style), degree = 3}
  for _, field in ipairs(FIELDS) do
    if node.has_field(n, field) then into(n[field], styles[field]) end
  end
end

-- Walks a math list; for each run of two or more adjacent prime atoms calls visit(run, lengths)
-- after reading the lists inside the run's atoms.
function walk(head, style, prime, visit)
  local n = head
  while n do
    local after = n.next
    if n.id == style_id then
      style = style_index(n.style)
    elseif prime_atom(n, prime.family, prime.slot) and n.subtype == ord and not n.sub and not n.sup then
      local run = {n}
      local next_atom = n.next
      while next_atom and prime_atom(next_atom, prime.family, prime.slot) and follower[next_atom.subtype] do
        run[#run + 1] = next_atom
        if next_atom.sub or next_atom.sup then break end
        next_atom = next_atom.next
      end
      for _, atom in ipairs(run) do children(atom, style, prime, visit) end
      after = run[#run].next
      if #run > 1 then
        visit(run, pieces(run, tex.getfontoffamily(prime.family, SIZE[style]), prime.slot))
      end
    else
      children(n, style, prime, visit)
    end
    n = after
  end
end

local function each_run(head, style, visit)
  local family, slot = joining_prime()
  if not family then return end
  kinds()
  walk(head, style_index(style), {family = family, slot = slot}, visit)
end

-- The runs of primes in a math list TeX is about to typeset (style as the callback names it), when
-- the prime's font joins primes at all: a list of {run length, piece lengths}. Changes no node.
function M.runs(head, style)
  local found = {}
  each_run(head, style, function(run, lengths) found[#found + 1] = {#run, lengths} end)
  return found
end

-- On a copy of the list, before LuaMML converts it: each joined piece becomes its first atom,
-- carrying the last atom's scripts, whose character LuaMML writes as the n-fold prime.
function M.join(head, style)
  local properties = node.get_properties_table()
  each_run(head, style, function(run, lengths)
    local index = 1
    for _, length in ipairs(lengths) do
      local first = run[index]
      if length > 1 then
        local last = run[index + length - 1]
        local rest = last.next
        first.sub, first.sup = last.sub, last.sup
        last.sub, last.sup = nil, nil
        for offset = 1, length - 1 do
          local atom = run[index + offset]
          atom.next, atom.prev = nil, nil
          node.flush_node(atom)
        end
        first.next = rest
        if rest then rest.prev = first end
        local text = utf8.char(M.MULTIPLE[length])
        local props = properties[first.nucleus] or {}
        props.mathml_filter = function(result)
          result[0], result[1], result.mathvariant = 'mo', text, nil
          return result, result
        end
        properties[first.nucleus] = props
      end
      index = index + length
    end
  end)
end

return M
