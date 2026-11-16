-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Observe executed box metrics and child formula identity without rewriting
-- TeX lists. All cached trees contain scalar MathML, never native node pointers.
local M = {}
local glyphs = require('relatex-glyphs')
local leaders = require('relatex-leaders')
local properties = node.get_properties_table()
local hlist, vlist, glyph, rule = node.id('hlist'), node.id('vlist'), node.id('glyph'), node.id('rule')
local kern, glue, math_node, penalty = node.id('kern'), node.id('glue'), node.id('math'), node.id('penalty')
local whatsit = node.id('whatsit')
local sub_box, sub_mlist, disc = node.id('sub_box'), node.id('sub_mlist'), node.id('disc')
local noad, accent, radical, fraction = node.id('noad'), node.id('accent'), node.id('radical'), node.id('fraction')
local choice, style_node = node.id('choice'), node.id('style')
local branches = {'display', 'text', 'script', 'scriptscript'}
local function length(value) return string.format('%.6fpt', value / 65781.76) end
local function dimension(value, fallback)
  return value == -1073741824 and fallback or value
end
local function stripped(tree)
  if tree[0] == 'math' then tree[0], tree.xmlns, tree.display = 'mrow', nil, nil end
  return tree
end
local function placed(tree, x, y)
  return {[0]='mpadded', width='0pt', height='0pt', depth='0pt',
    lspace=length(x), voffset=length(y), tree}
end

-- A formula tree belongs at the baseline of its complete converted list, not
-- at the first radical glyph or script found inside that list. Require exact
-- fragment coverage and native dimensions before adopting the whole formula.
local function formula_coverage(head, bridge, state)
  for n in node.traverse(head) do
    if n.id == hlist or n.id == vlist then formula_coverage(n.head, bridge, state)
    elseif bridge.is_ink(n) then
      local item = bridge.identity(n)
      if not item or (state.fragment and item.fragment ~= state.fragment) or state.positions[item.position] then
        state.invalid = true
      else
        state.fragment = item.fragment
        state.positions[item.position] = true
        state.count = state.count + 1
      end
    end
  end
end

local leader_copies, tree_text

-- A shaft leader too short for one copy (\xleftrightarrow[]{}: the two heads overlap) paints nothing, so
-- the arrow the formula composed from its heads (relatex-math-arrows: ←−→) loses its shaft (papers4/1066).
local function unspell(tree, shaft)
  for index, child in ipairs(tree) do
    if type(child) == 'table' then unspell(child, shaft)
    elseif tree[0] == 'mo' and child ~= shaft and child:find(shaft, 1, true) then
      tree[index] = child:gsub(shaft, '', 1)
    end
  end
end

local function whole_formula(box, bridge, arrays, state)
  if box.id ~= hlist or not bridge.fragment_metrics then return end
  local coverage = {positions={}, count=0}
  formula_coverage(box.head, bridge, coverage)
  if coverage.invalid or not coverage.fragment then return end
  local metrics = bridge.fragment_metrics(coverage.fragment)
  if not metrics or coverage.count ~= metrics.nodes or state.seen[metrics.formula] then return end
  for index=1,metrics.nodes do if not coverage.positions[index] then return end end
  if math.abs(box.width - metrics.width) > 1 or math.abs(box.height - metrics.height) > 1
      or math.abs(box.depth - metrics.depth) > 1 then return end
  local tree = arrays.tree(metrics.formula)
  if not tree then return end
  state.seen[metrics.formula], state.adopted[metrics.formula] = true, true
  local result = {[0]='mpadded', width=length(box.width), height=length(box.height), depth=length(box.depth),
    placed(stripped(tree), 0, 0)}
  -- The formula's own reading leaves out its leaders (the shaft of \leftrightarrowfill@); a shaft the
  -- formula already spells (its composed arrow) is not spelled again.
  local spelled = table.concat(tree_text(tree, {}))
  for n in node.traverse(box.head) do
    if n.id == glue and n.leader then
      local copies, _, unpainted = leader_copies(n, box, node.rangedimensions(box, box.head, n), bridge, arrays,
        state, nil, nil, spelled)
      if unpainted then unspell(tree, unpainted) end
      for _, copy in ipairs(copies or {}) do result[#result + 1] = copy end
    end
  end
  return result
end

-- A picture without an inspectable source is read as a rendered figure, not as math. A
-- formula inside a picture (a diagram's cell) carries that picture's attribute itself; only a
-- different, contained picture is one.
local function graphic_box(n)
  local flow = (package.loaded['relatex'] or {}).flow
  return flow and flow.contained_graphic(n)
end
M.graphic_box = graphic_box

local box_tree
local function piece(n, bridge, arrays, state, box)
  local identity = bridge.identity(n)
  if identity then
    local tree = arrays.tree(identity.formula)
    if not tree then return nil, 'missing child formula tree' end
    if state.seen[identity.formula] then return false end
    state.seen[identity.formula], state.adopted[identity.formula] = true, true
    return stripped(tree)
  end
  if graphic_box(n) then return false end
  if n.id == hlist or n.id == vlist then return box_tree(n, bridge, arrays, state) end
  if n.id == glyph then
    local text = glyphs.character(n)
    if not text or text:find('[%z\1-\8\11\12\14-\31\127]') then return nil, 'unmapped native box glyph' end
    return {[0]='mtext', text}
  end
  if n.id == rule then
    -- An included graphic is the figure the page shows beside the formula, not a painted rule.
    if bridge.GRAPHIC and bridge.GRAPHIC[n.subtype] then return false end
    local height, depth = dimension(n.height, box.height), dimension(n.depth, box.depth)
    if n.width <= 0 or height + depth <= 0 then return false end
    return {[0]='mspace', width=length(n.width), height=length(height), depth=length(depth), mathbackground='#000000'}
  end
  -- A whatsit (a \label's bookkeeping) prints nothing; nor do the paragraph start, direction and boundary
  -- nodes of a paragraph box (\parbox{...} in a display, papers3/0853).
  if n.id == kern or n.id == glue or n.id == math_node or n.id == penalty or n.id == whatsit
      or n.id == node.id('local_par') or n.id == node.id('dir') or n.id == node.id('boundary') then return false end
  -- A discretionary left unbroken sets its replacement (the letters of a word the paragraph could hyphenate).
  if n.id == disc then
    local result = {[0]='mrow'}
    for item in node.traverse(n.replace) do
      local tree, reason = piece(item, bridge, arrays, state, box)
      if tree == nil then return nil, reason end
      if tree then result[#result + 1] = tree end
    end
    return #result > 0 and result
  end
  return nil, 'unsupported native node inside mathematical box: ' .. node.type(n.id)
end

-- A box leader in a row (amsmath's \hdotsfor: a math period repeated by \xleaders) paints its box
-- once per copy TeX fits in the glue, at the places tex.web's hlist_out gives them.
tree_text = function(tree, result)
  for _, child in ipairs(tree) do
    if type(child) == 'string' then result[#result + 1] = child
    elseif type(child) == 'table' then tree_text(child, result) end
  end
  return result
end

-- An arrow's shaft (amsmath's \rightarrowfill@ repeats \relbar by \cleaders) is spelled once, as an
-- extensible arrow is: by the fill's own piece when it has one (\relbar before the head), else by
-- one copy (\leftrightarrowfill@ has two heads and only the leader between them).
local SHAFTS = {['−'] = true, ['='] = true}

-- The characters of the glyphs from n on (its later siblings and what they hold).
local function glyph_text(n, result, bridge)
  while n do
    if n.id == glyph then result[#result + 1] = bridge.printed_character(n) or glyphs.character(n) or ''
    elseif n.id == hlist or n.id == vlist then glyph_text(n.head, result, bridge) end
    n = n.next
  end
  return result
end

-- spelled is the text of the formula holding the leader, when its tree is read whole; a shaft too short
-- for one copy returns that shaft third.
leader_copies = function(n, box, x, bridge, arrays, state, dx, dy, spelled)
  dx, dy = dx or 0, dy or 0
  local leader = n.leader
  if leader.id ~= hlist and leader.id ~= vlist or leader.width <= 0 then return {} end
  local own = {seen = setmetatable({}, {__index = state.seen}), adopted = {}}
  local tree, reason = piece(leader, bridge, arrays, own, box)
  if tree == nil then return nil, reason end
  if not tree then return {} end
  local shaft = table.concat(tree_text(tree, {}))
  if SHAFTS[shaft] then
    -- The box's own glyphs, before and after the leader (a piece read elsewhere still paints here).
    if node.effective_glue(n, box) + 10 < leader.width then return {}, nil, shaft end
    local around = table.concat(glyph_text(box.head, {}, bridge)) .. (spelled or '')
    if around:find(shaft, 1, true) then return {} end
    for id in pairs(own.adopted) do state.seen[id], state.adopted[id] = true, true end
    return {placed(tree, dx + x, dy - (leader.shift or 0))}
  end
  for id in pairs(own.adopted) do state.seen[id], state.adopted[id] = true, true end
  local copies = {}
  for _, at in ipairs(leaders.positions(n, x, node.effective_glue(n, box))) do
    copies[#copies + 1] = placed(arrays.scalar_tree(tree), dx + at, dy - (leader.shift or 0))
  end
  return copies
end

-- Where n sits in box: its left edge and its baseline above the box baseline. y is the running
-- top of a vertical list.
local function position(box, n, y)
  local x, baseline = 0, 0
  if box.id == hlist then
    x = node.rangedimensions(box, box.head, n)
    if n.id == hlist or n.id == vlist then baseline = -(n.shift or 0) end
  elseif n.id == hlist or n.id == vlist then
    y = y - n.height
    x, baseline = n.shift or 0, y
    y = y - n.depth
  elseif n.id == kern then y = y - n.kern
  elseif n.id == glue then y = y - n.width
  elseif n.id == rule then
    local height, depth = dimension(n.height, box.height), dimension(n.depth, box.depth)
    y = y - height; baseline = y; y = y - depth
  end
  return x, baseline, y
end

-- A box inside a box adds its pieces to the outer box at its own offset instead of nesting: its
-- dimensions sit inside a zero-size placement and change nothing, and nesting costs three MathML
-- levels a box (the HoTT book's proof trees, boxes of boxes, passed the depth limit of 64).
local function inlined(n, bridge)
  return (n.id == hlist or n.id == vlist) and not bridge.identity(n) and not graphic_box(n)
end

local box_pieces
local function inline_box(n, bridge, arrays, state, result, x, y)
  local whole = whole_formula(n, bridge, arrays, state)
  if whole then result[#result + 1] = placed(whole, x, y); return true end
  local before = #result
  local done, reason = box_pieces(n, bridge, arrays, state, result, x, y)
  if not done then return nil, reason end
  if #result == before then
    result[#result + 1] = placed({[0]='mspace', width=length(n.width), height=length(n.height), depth=length(n.depth)}, x, y)
  end
  return true
end

-- Text set in a box inside a formula (\text{if and only if}, \mbox{for all}) reads as words: the letters
-- of a line run join one mtext placed at its first letter, and the interword glue between them is a
-- space. Font kerns, penalties and the like continue a word; any other piece ends it.
local function word_gap(n)
  return (n.id == glue and not n.leader and n.width > 0) or (n.id == kern and n.subtype == 1 and n.kern > 0)
end

-- An unbroken hyphenation point (a discretionary with no replacement) prints nothing inside a word.
local function word_neutral(n)
  return n.id == kern or n.id == penalty or n.id == whatsit or n.id == glue and not n.leader
    or n.id == disc and not n.replace
end

-- The letters a discretionary sets unbroken (an ff ligature's), as one text piece.
local function letters(tree)
  if not tree or tree[0] ~= 'mrow' then return tree end
  local text = {}
  for _, child in ipairs(tree) do
    if child[0] ~= 'mtext' then return tree end
    text[#text + 1] = child[1]
  end
  return {[0]='mtext', table.concat(text)}
end

local function text_letter(tree, n, bridge)
  return (n.id == glyph and not bridge.identity(n) or n.id == disc) and tree and tree[0] == 'mtext'
end

-- The pieces of box, placed at (dx, dy) in the box being converted.
box_pieces = function(box, bridge, arrays, state, result, dx, dy)
  local y, first = box.height, #result + 1
  local word, gap
  for n in node.traverse(box.head) do
    local x, baseline
    x, baseline, y = position(box, n, y)
    local done, reason = true, nil
    if box.id == hlist and word and word_neutral(n) then
      gap = gap or word_gap(n)
    elseif n.id == glue and n.leader and box.id == hlist then
      local copies, unpainted
      local spelled = table.concat(tree_text({table.unpack(result, first)}, {}))
      copies, reason, unpainted = leader_copies(n, box, x, bridge, arrays, state, dx, dy, spelled)
      for index = first, unpainted and #result or 0 do unspell(result[index], unpainted) end
      done = copies ~= nil
      for _, copy in ipairs(copies or {}) do result[#result + 1] = copy end
      word = nil
    elseif inlined(n, bridge) then
      word = nil
      done, reason = inline_box(n, bridge, arrays, state, result, dx + x, dy + baseline)
    else
      local tree
      tree, reason = piece(n, bridge, arrays, state, box)
      done = tree ~= nil
      if n.id == disc then tree = letters(tree) end
      if word and box.id == hlist and text_letter(tree, n, bridge) then
        word[1] = word[1] .. (gap and ' ' or '') .. tree[1]
      elseif tree then
        result[#result + 1] = placed(tree, dx + x, dy + baseline)
        word = box.id == hlist and text_letter(tree, n, bridge) and tree or nil
      else
        word = nil
      end
      gap = false
    end
    if not done then return nil, reason end
  end
  return true
end

box_tree = function(box, bridge, arrays, state)
  local whole = whole_formula(box, bridge, arrays, state)
  if whole then return whole end
  local result = {[0]='mrow'}
  local done, reason = box_pieces(box, bridge, arrays, state, result, 0, 0)
  if not done then return nil, reason end
  if #result == 0 then
    return {[0]='mspace', width=length(box.width), height=length(box.height), depth=length(box.depth)}
  end
  return {[0]='mpadded', width=length(box.width), height=length(box.height), depth=length(box.depth), result}
end

local walk
local function kernel(value, bridge, arrays, adopted, failures, style)
  if not value then return end
  if value.id == sub_mlist then walk(value.head, bridge, arrays, adopted, failures, style); return end
  if value.id ~= sub_box or not value.head then return end
  local props = properties[value] or {}
  if props.mathml_table ~= nil or graphic_box(value.head) then return end
  local state = {seen={}, adopted={}}
  local tree, reason = box_tree(value.head, bridge, arrays, state)
  if not tree then failures[reason] = true; return end
  local shift = value.head.shift or 0
  if shift ~= 0 then
    local box = value.head
    tree = {[0]='mpadded', width=length(box.width), height=length(math.max(0, box.height - shift)),
      depth=length(math.max(0, box.depth + shift)), voffset=length(-shift), tree}
  end
  props.mathml_table = tree
  properties[value] = props
  for id in pairs(state.adopted) do adopted[id] = true end
end

walk = function(head, bridge, arrays, adopted, failures, style)
  if not head then return end
  for n in node.traverse(head) do
    if n.id == style_node then style = n.subtype
    elseif n.id == choice then walk(n[branches[style // 2 + 1]], bridge, arrays, adopted, failures, style)
    elseif n.id == noad or n.id == accent or n.id == radical then
      kernel(n.nucleus, bridge, arrays, adopted, failures, style)
      kernel(n.sub, bridge, arrays, adopted, failures, style // 4 * 2 + 5)
      kernel(n.sup, bridge, arrays, adopted, failures, style // 4 * 2 + 4 + style % 2)
      -- A root's index (\Uroot, unicode-math's \sqrt[n]) set in a box of its own formula
      -- (amsmath's \uproot: \hbox{$\scriptscriptstyle n$}) is part of the radical.
      if n.id == radical then kernel(n.degree, bridge, arrays, adopted, failures, 6 + style % 2) end
    elseif n.id == fraction then
      kernel(n.num, bridge, arrays, adopted, failures, style + 2 - style // 6 * 2)
      kernel(n.denom, bridge, arrays, adopted, failures, style // 2 * 2 + 3 - style // 6 * 2)
    end
  end
end

function M.prepare(head, bridge, arrays, style)
  local adopted, failures, ids, reasons = {}, {}, {}, {}
  walk(head, bridge, arrays, adopted, failures, style or 2)
  for id in pairs(adopted) do ids[#ids + 1] = id end
  for reason in pairs(failures) do reasons[#reasons + 1] = reason end
  table.sort(ids); table.sort(reasons)
  return ids, reasons
end

function M.fraction_rules(tree)
  if type(tree) ~= 'table' then return end
  if tree[0] == 'mfrac' then
    local nodes = tree[':nodes']
    local native = nodes and nodes[1]
    -- TeX's default-rule sentinel is not a physical dimension. All explicit
    -- dimensions fit below 2^30 scaled points, including a zero-width rule.
    if native and native.id == fraction and native.width >= 0 and native.width < 1073741824 then
      tree.linethickness = length(native.width)
    end
  end
  for _, child in ipairs(tree) do M.fraction_rules(child) end
end

return M
