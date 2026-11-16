-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Native latex-lab note/mark identities; scalar snapshots only.
local M = {}
local mark_attr = luatexbase.new_attribute('relatex.note.mark')
local number_attr = luatexbase.new_attribute('relatex.note.number')
local emit, collector, body
local mark_stack, number_stack = {}, {}

function M.start(writer, context)
  emit, collector = writer, context
end

function M.mark_begin(id, number, label)
  if not emit then return end
  assert(id > 0, 'Missing native footnote mark identity')
  mark_stack[#mark_stack + 1] = tex.getattribute(mark_attr) or -2147483647
  tex.setattribute(mark_attr, id)
  emit('diagnostic', {code = 'note_mark', source = M.source or 'latex-lab-footnote-sockets', mark = id,
    number = number, label = label})
end

function M.mark_end()
  if not emit then return end
  local saved = assert(table.remove(mark_stack), 'Unbalanced footnote mark')
  tex.setattribute(mark_attr, saved)
end

function M.body_enter(number)
  if not emit then return end
  assert(not body, 'Nested footnote bodies are unsupported')
  local parent = collector.current_parent()
  collector.enter('footnote')
  body = {number = number, entity = collector.current_context(), parent = parent}
end

function M.body_bind(id, marks, title)
  if not emit then return end
  assert(body and not body.note and id > 0, 'Unbalanced native footnote body')
  body.note = id
  emit('diagnostic', {code = 'note_body', source = M.source or 'latex-lab-footnote-sockets', note = id,
    entity = body.entity, parent = body.parent, number = body.number, marks = marks, title = title or nil})
end

function M.body_end()
  if not emit then return end
  assert(body and body.note, 'Unclosed native footnote body')
  collector.leave('footnote')
  body = nil
end

function M.number_begin()
  if not emit then return end
  assert(body and body.note, 'Footnote label outside a body')
  number_stack[#number_stack + 1] = tex.getattribute(number_attr) or -2147483647
  tex.setattribute(number_attr, body.note)
end

function M.number_end()
  if not emit then return end
  local saved = assert(table.remove(number_stack), 'Unbalanced footnote label')
  tex.setattribute(number_attr, saved)
end

function M.identify(n, item)
  local mark, number = node.has_attribute(n, mark_attr), node.has_attribute(n, number_attr)
  if mark then item.note_mark = mark end
  if number then item.note_number = number end
end

function M.finish()
  assert(not body and #mark_stack == 0 and #number_stack == 0, 'Unclosed footnote capture')
  emit = nil
end

return M
