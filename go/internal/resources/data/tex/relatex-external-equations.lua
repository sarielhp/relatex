-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Bind executed AMS rows in the final native alignment before shipout.
-- Keep all original cell MathML. Only private attributes and JSON are written.
local M, stack, contexts = {}, {}, {}
local core
local arrays = require('relatex-external-arrays')
local decoder = require('relatex-glyphs')
local hlist, vlist, glyph = node.id('hlist'), node.id('vlist'), node.id('glyph')
local disc = node.id('disc')
local alignment, cell
for key, name in pairs(node.subtypes(hlist)) do
  if name == 'alignment' then alignment = key end
  if name == 'cell' then cell = key end
end
-- The formulas whose ink a list holds. A picture's own math (a TikZ node's label inside a cell,
-- \fug\Big(\tikz{... node {$j$}}\Big): papers/0191) is the picture's, never a formula of the row.
local graphic_box = require('relatex-external-boxes').graphic_box
local function identities(head, ids)
  for n in node.traverse(head) do
    local identity = core.math_bridge.identity(n)
    if identity then ids[identity.formula] = true end
    if (n.id == hlist or n.id == vlist) and not graphic_box(n) then identities(n.head, ids) end
  end
end
local function text(head, result)
  for n in node.traverse(head) do
    if n.id == glyph then
      -- A math glyph of a legacy-encoded font (the prime in \tag{\theequation$'$}) prints its own character.
      local value = core.math_bridge.printed_character(n) or decoder.character(n)
      if not value then return false end
      result[#result + 1] = value
    end
    if n.id == node.id('glue') and n.subtype == 13 then result[#result + 1] = ' ' end
    if (n.id == hlist or n.id == vlist) and not text(n.head, result) then return false end
    -- A literal hyphen of the number (\tag{I-RI1}) sits in an unbroken discretionary's replace list.
    if n.id == disc and n.replace and not text(n.replace, result) then return false end
  end
  return true
end
-- The parts of a tag in print order: runs of its text and the formulas it typesets
-- (\tag{by computation for $\rec{\Sn^1}$}), each formula once. A glyph of a formula is the formula's.
local function tag_parts(head, parts, seen)
  for n in node.traverse(head) do
    local identity = core.math_bridge.identity(n)
    if identity then
      if not seen[identity.formula] then
        seen[identity.formula] = true
        parts[#parts + 1] = {formula = identity.formula}
      end
    elseif n.id == glyph then
      local value = core.math_bridge.printed_character(n) or decoder.character(n)
      if not value then return false end
      parts[#parts + 1] = value
    elseif n.id == node.id('glue') and n.subtype == 13 then
      parts[#parts + 1] = ' '
    elseif (n.id == hlist or n.id == vlist) and not tag_parts(n.head, parts, seen) then
      return false
    elseif n.id == disc and n.replace and not tag_parts(n.replace, parts, seen) then
      return false
    end
  end
  return true
end
-- A text run beside a formula keeps its edge spaces as no-break spaces: MathML trims a token's
-- leading and trailing white space.
local function label_text(run)
  return (run:gsub('^ ', '\u{A0}'):gsub(' $', '\u{A0}'))
end
-- The label cell of a row: the tag as text, or, when the tag typesets math, its text runs and each
-- formula's MathML, marked as a formula of the tag (decided 2026-11-09: faithful to the PDF, and the
-- math count counts it as a formula).
local function label_cell(head, flat)
  local parts = {}
  if not tag_parts(head, parts, {}) then return nil end
  local cell, run = {[0] = 'mtd', intent = ':equation-label'}, {}
  local formulas = 0
  local function close()
    if #run > 0 then cell[#cell + 1] = {[0] = 'mtext', label_text(table.concat(run))} end
    run = {}
  end
  for _, part in ipairs(parts) do
    if type(part) == 'table' then
      local tree = arrays.tree(part.formula)
      if not tree then return nil end
      close()
      tree[0], tree.display, tree.xmlns, tree.class = 'mrow', nil, nil, 'tag-formula'
      cell[#cell + 1] = tree
      formulas = formulas + 1
    else
      run[#run + 1] = part
    end
  end
  close()
  if formulas == 0 then return {[0] = 'mtd', intent = ':equation-label', {[0] = 'mtext', flat}} end
  return cell
end
function M.install(collector) core = collector end
-- alignat and flalign set the same r l r l columns as align: the rows read and export alike.
local ALIGNED = {alignat = 'align', flalign = 'align'}
local function aligned(environment)
  local star = environment:match('%*$') or ''
  local base = environment:gsub('%*$', '')
  return (ALIGNED[base] or base) .. star
end
function M.begin(environment)
  -- A nested display binds no rows of its own: the outer display's row holds it as one cell.
  if core.equation_nested() then stack[#stack + 1] = {nested = true}; return end
  local entity = core.current_context()
  local context = {environment = environment, entity = entity, labels = {}, rows = 0}
  contexts[entity] = context
  stack[#stack + 1] = context
end
function M.defer(record, environment, row)
  local context = stack[#stack]
  if not context or context.environment ~= environment then return false end
  if environment:gsub('%*$', '') == 'eqnarray' then
    -- eqnarray has no row counter: the label belongs to the row that prints its number as the tag.
    context.tags = context.tags or {}
    -- Two labels printing one number (an unnumbered row's label shares the row before it) stay plain labels.
    if context.tags[record.number] then return false end
    context.tags[record.number] = record
    return true
  end
  assert(row > 0 and not context.labels[row], 'Ambiguous native alignment label')
  context.labels[row] = record
  return true
end
function M.finish()
  assert(table.remove(stack), 'Unbalanced external alignment')
end
local function row_context(row)
  local ids, context = {}, nil
  identities(row.head, ids)
  for id in pairs(ids) do
    local entity = core.math_bridge.formula_context(id)
    local found = contexts[entity]
    if not found or (context and context ~= found) then return nil end
    context = found
  end
  return context
end
local function row_tree(row, context)
  local cells, children, adopted = {}, {}, {}
  for column in node.traverse(row.head) do
    if column.id == hlist and column.subtype == cell then cells[#cells + 1] = column end
  end
  local family = aligned(context.environment):gsub('%*$', '')
  if (family == 'align' and #cells < 2) or (family == 'gather' and #cells ~= 2) or
      (family == 'eqnarray' and (#cells < 1 or #cells > 4)) then return end
  -- A row ended by \cr (not \\) is shorter than the four columns and has no number cell; so is an
  -- align row ended by \crcr (papers5/1373), whose column pairs leave an even number of cells.
  local short = (family == 'eqnarray' and #cells < 4) or (family == 'align' and #cells % 2 == 0)
  local content = short and #cells or #cells - 1
  local alignments = {align = {'right', 'left'}, eqnarray = {'right', 'center', 'left'}}
  local xml = {[0] = 'mtr'}
  for index = 1, content do
    local ids, count, found = {}, 0, nil
    identities(cells[index].head, ids)
    for id in pairs(ids) do count, found = count + 1, id end
    if count > 1 then return end
    local tree = count == 1 and arrays.tree(found) or {[0] = 'mrow'}
    if not tree then return end
    tree[0], tree.display, tree.xmlns = 'mrow', nil, nil
    xml[#xml + 1] = {[0] = 'mtd', columnalign = (alignments[family] or {'center'})[(index - 1) % #(alignments[family] or {'center'}) + 1], tree}
    if found then
      if adopted[found] then return end
      children[#children + 1], adopted[found] = found, true
    end
  end
  local tag_ids, tag = {}, {}
  if not short then
    identities(cells[#cells].head, tag_ids)
    if not text(cells[#cells].head, tag) then return end
    -- Math in the number (\tag{\theequation$'$}) is part of the row's label: the row adopts it.
    local inside = {}
    for id in pairs(tag_ids) do inside[#inside + 1] = id end
    table.sort(inside)
    for _, id in ipairs(inside) do
      if adopted[id] then return end
      children[#children + 1], adopted[id] = id, true
    end
  end
  if #children == 0 then return end
  local label = {[0] = 'mtd', intent = ':equation-label', {[0] = 'mtext', table.concat(tag)}}
  if not short then label = label_cell(cells[#cells].head, table.concat(tag)) end
  if not label then return end
  table.insert(xml, 1, label)
  return {[0] = 'math', xmlns = 'http://www.w3.org/1998/Math/MathML', display = 'block',
    {[0] = 'mtable', class = family, xml}}, children, table.concat(tag)
end
local function family_of(context)
  return (aligned(context.environment):gsub('%*$', ''))
end
-- The printed number of a multline sits after the math of its last line, in the same cell.
local function loose_text(head, result)
  for n in node.traverse(head) do
    if n.id == glyph and not core.math_bridge.identity(n) then
      local value = decoder.character(n)
      if not value then return false end
      result[#result + 1] = value
    elseif (n.id == hlist or n.id == vlist) and not loose_text(n.head, result) then
      return false
    elseif n.id == disc and n.replace and not loose_text(n.replace, result) then
      return false
    end
  end
  return true
end
-- One multline line: its one cell holds at most one formula and, on the last line, the number.
local function multline_line(row)
  local cells = {}
  for column in node.traverse(row.head) do
    if column.id == hlist and column.subtype == cell then cells[#cells + 1] = column end
  end
  if #cells ~= 1 then return end
  local ids, count, found = {}, 0, nil
  identities(cells[1].head, ids)
  for id in pairs(ids) do count, found = count + 1, id end
  if count > 1 then return end
  local tree = count == 1 and arrays.tree(found) or {[0] = 'mrow'}
  if not tree then return end
  tree[0], tree.display, tree.xmlns = 'mrow', nil, nil
  local tag = {}
  if not loose_text(cells[1].head, tag) then return end
  return {tree = tree, child = found, tag = table.concat(tag), head = row.head}
end
local function collect_multline(multlines, row, context)
  context.rows = context.rows + 1
  if not context.lines then
    context.lines = {}
    multlines[#multlines + 1] = context
  end
  context.lines[#context.lines + 1] = multline_line(row) or false
end
local function multline_tree(lines)
  local children, adopted, rows, tag = {}, {}, {}, ''
  for index, line in ipairs(lines) do
    if line.child then
      if adopted[line.child] then return end
      children[#children + 1], adopted[line.child] = line.child, true
    end
    if line.tag ~= '' then tag = line.tag end
    local align = index == 1 and 'left' or (index == #lines and 'right' or 'center')
    rows[#rows + 1] = {[0] = 'mtr',
      {[0] = 'mtd', intent = ':equation-label', {[0] = 'mtext', line.tag}},
      {[0] = 'mtd', columnalign = align, line.tree}}
  end
  if #children == 0 then return end
  return {[0] = 'math', xmlns = 'http://www.w3.org/1998/Math/MathML', display = 'block',
    {[0] = 'mtable', class = 'multline', table.unpack(rows)}}, children, tag
end
-- The lines of a multline that the page builder left for a later page (\allowdisplaybreaks): rows of
-- the same display still on the contribution list when this page ships.
local function following_lines(context)
  local lines = {}
  for _, name in ipairs({"page_head", "contrib_head"}) do
    local head = tex.lists[name]
    for n in node.traverse(head) do
      if n.id == hlist and n.subtype == alignment and row_context(n) == context then
        lines[#lines + 1] = multline_line(n) or false
      end
    end
  end
  return lines
end
local function finish_multline(context)
  local lines, heads = context.lines, {}
  context.lines = nil
  -- A multline broken across pages is one formula (papers4/0961): its later lines are rows of the
  -- formula begun on the first page, whose ink they hold.
  if context.formula or context.unsupported then
    for _, line in ipairs(lines) do heads[#heads + 1] = line and line.head or nil end
    if context.formula then ltd_external_math.continue_row(context.formula, heads, context.entity) end
    return
  end
  local whole = {table.unpack(lines)}
  local ahead = following_lines(context)
  for _, line in ipairs(ahead) do whole[#whole + 1] = line end
  for _, line in ipairs(whole) do
    if not line then heads = nil; break end
  end
  if heads then
    for _, line in ipairs(lines) do heads[#heads + 1] = line.head end
  end
  local tree, children, tag
  if heads then tree, children, tag = multline_tree(whole) end
  if not tree then
    core.external_equation_diagnostic({code = 'unsupported_external_alignment', environment = context.environment,
      entity = context.entity, row = context.rows})
    context.unsupported = #ahead > 0 or nil
    return
  end
  local formula = ltd_external_math.native_row(tree, children, heads, context.entity)
  if #ahead > 0 then context.formula = formula end
  core.external_equation_diagnostic({code = 'external_alignment_row', environment = family_of(context),
    entity = context.entity, row = context.rows + #ahead, formula = formula})
  -- One number labels the whole display, whichever line the \label stood on.
  local row = next(context.labels)
  local label = row and context.labels[row]
  if label then
    label.formula, label.math_environment = formula, 'equation'
    core.external_equation_label(label)
    context.labels[row] = nil
  end
end
function M.shipout(head)
  local multlines = {}
  for n in node.traverse(head) do
    local context = n.id == hlist and n.subtype == alignment and row_context(n)
    local tree, children, tag, collected
    if context and family_of(context) == "multline" then
      collect_multline(multlines, n, context)
      -- The line is read whole: an array set in it has rows of its own in the same equation context,
      -- which are no lines of the multline (papers4/1069: 7 rows for 3 lines dropped the multline).
      context, collected = nil, true
    elseif context then tree, children, tag = row_tree(n, context) end
    if context then
      context.rows = context.rows + 1
    end
    if tree then
      local formula = ltd_external_math.native_row(tree, children, n.head, context.entity)
      core.external_equation_diagnostic({code = 'external_alignment_row', environment = aligned(context.environment),
        entity = context.entity, row = context.rows, formula = formula})
      local label = context.labels[context.rows]
      local printed = tag and tag:match('^%s*%((.-)%)%s*$')
      if context.tags and printed and context.tags[printed] then label, context.tags[printed] = context.tags[printed], nil end
      if label then
        label.formula, label.math_environment, label.math_row = formula, aligned(context.environment), 1
        core.external_equation_label(label)
        context.labels[context.rows] = nil
      end
    elseif context then
      core.external_equation_diagnostic({code = 'unsupported_external_alignment', environment = context.environment,
        entity = context.entity, row = context.rows})
    elseif not collected and (n.id == hlist or n.id == vlist) then M.shipout(n.head) end
  end
  for _, context in ipairs(multlines) do finish_multline(context) end
end
function M.flush()
  assert(#stack == 0, 'Unclosed native alignment context')
  for _, context in pairs(contexts) do
    for _, label in pairs(context.labels) do core.external_equation_label(label) end
    for _, label in pairs(context.tags or {}) do core.external_equation_label(label) end
  end
end
return M
