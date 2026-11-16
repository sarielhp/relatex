-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- A beamer frame is a top-level section (decision 083). beamer typesets a frame's body before
-- its title box, so \frametitle opens the section and the title box, set later, carries the
-- section's entity as a run-in heading's deferred title does.
local M = {}
local pending

function M.frame_title(ltd)
  pending = nil
  ltd.heading('section-@startsection', false, false)
  local id, kind = ltd.current_context()
  -- A frame title inside a block (a frame set in a theorem's context) stays its text.
  if kind ~= 'section' then return end
  ltd.section_end()
  pending = id
end

-- Decision 128, d: a title set while a picture is open (a theme's TikZ node) is drawn, not a
-- heading; the frame says so with a diagnostic.
local emit
local pictures, titling, drawn = 0, false, false

function M.start(writer) emit = writer end

function M.picture(step) pictures = math.max(pictures + step, 0) end

function M.title_set()
  if titling and pictures > 0 then drawn = true end
end

function M.title_begin(ltd)
  titling, drawn = true, false
  if pending then ltd.runin_begin(pending) end
end

function M.title_end(ltd)
  titling = false
  if drawn and emit then
    emit('diagnostic', {code = 'frame_title_in_picture', source = 'beamer-frametitle-template'})
  end
  drawn = false
  if not pending then return end
  ltd.runin_end()
  pending = nil
end

-- Blocks (decision 083, amendment 1). A block opens a theorem context with no counter whose
-- semantics name its beamer family (block, alerted, example) and whether it has a title; its end
-- template closes it. The block a theorem or proof sets through its own begin template belongs
-- to that structure.
local blocks, owned = {}, false

function M.template_begin(ltd, family, environment, titled)
  if family == 'owner' then owned = true; return end
  if family == 'end' then return end
  local entered = not owned
  owned = false
  blocks[#blocks + 1] = entered
  if not entered then return end
  ltd.enter('theorem')
  ltd.block_context(environment, family, titled)
end

function M.template_end(ltd, family)
  if family == 'owner' then owned = false; return end
  if family ~= 'end' then return end
  if table.remove(blocks) then ltd.leave('theorem') end
end

-- Item markers (decision 127). A marker template that sets no glyph and paints one rule draws
-- its marker: a plain rule is a square (the square template, the rectangles inner theme), a box or
-- image rule (the ball's shading, a form XObject: the rounded inner theme of Madrid and Frankfurt) a
-- disc. The rule carries the shape for the flow reader; any other drawing
-- stays as it is. A rule's height or depth may run to its box (the square's \vrule has no depth).
local marker_attr = luatexbase.new_attribute('relatex.item_marker')
local rule_id, glyph_id = node.id('rule'), node.id('glyph')
local hlist_id, vlist_id = node.id('hlist'), node.id('vlist')
local NORMAL_RULE = 0

local function marker_ink(head, found)
  for n in node.traverse(head) do
    if n.id == glyph_id then found.glyph = true
    elseif n.id == rule_id and n.width > 0 then found.rules[#found.rules + 1] = n
    elseif (n.id == hlist_id or n.id == vlist_id) and n.head then marker_ink(n.head, found) end
  end
  return found
end

function M.item_marker(register)
  local box = tex.box[register]
  if not box or not box.head then return end
  local found = marker_ink(box.head, {rules = {}})
  if found.glyph or #found.rules ~= 1 then return end
  local rule = found.rules[1]
  node.set_attribute(rule, marker_attr, rule.subtype == NORMAL_RULE and 1 or 2)
end

return M
