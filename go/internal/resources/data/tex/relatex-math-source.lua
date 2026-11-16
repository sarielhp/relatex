-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Where each formula's source is (decision 064): the file and line LuaTeX reads when the
-- formula opens and when it closes, and the environment around it. Always recorded, since
-- source synchronization places formulas by it (decision 066); the macro meanings the
-- author's math needs are recorded only when the conversion asks for it
-- (RELATEX_AUTHOR_MATH=1).
local M = {}
local source_name = require('relatex-source-name')

local stack = {}
local groups = {}
local group_total = 0
local core

-- Display environments whose rows are typeset after their body has been read (amsmath's
-- collected bodies, eqnarray): their formulas belong to the environment's source group.
M.GROUPED = {'align', 'align*', 'gather', 'gather*', 'multline', 'multline*', 'flalign', 'flalign*',
  'alignat', 'alignat*', 'eqnarray', 'eqnarray*'}

function M.enabled()
  return true
end

local function author_math()
  return os.getenv('RELATEX_AUTHOR_MATH') == '1'
end

-- \everymath and \everydisplay call this in the new math list.
function M.open()
  stack[#stack + 1] = {ptr = tex.nest.ptr, file = source_name(status.filename), line = tex.inputlineno,
    environment = token.get_macro('@currenvir')}
end

-- The math callback runs after the list's nest level is gone. The opening that belongs to it
-- is one level deeper than the current one; openings left deeper than that (a list that
-- never reached the callback) are dropped. A formula of an open group carries the group.
function M.close()
  local depth = tex.nest.ptr + 1
  while #stack > 0 and stack[#stack].ptr > depth do stack[#stack] = nil end
  local entry = stack[#stack]
  local group = groups[#groups]
  if entry and entry.ptr == depth then
    stack[#stack] = nil
    if not group then
      return {file = entry.file, line = entry.line, end_line = tex.inputlineno, environment = entry.environment}
    end
  end
  if not group then return nil end
  return {file = group.file, line = group.line, end_line = tex.inputlineno, environment = group.environment,
    group = group.id}
end

function M.group_begin(environment)
  group_total = group_total + 1
  groups[#groups + 1] = {id = group_total, file = source_name(status.filename), line = tex.inputlineno, environment = environment}
end

function M.group_end()
  local group = table.remove(groups)
  if group then
    core.math_source_group({group = group.id, file = group.file, line = group.line,
      end_line = tex.inputlineno, environment = group.environment})
  end
end

-- What each candidate macro finally means (names found in the paper's files by the reader,
-- one per line in RELATEX_MATH_MACROS). A robust command's meaning is \protect\NAME␣␣;
-- its body is the meaning of "NAME ", reported too. A mathtools paired delimiter
-- (\DeclarePairedDelimiter, ...X, ...XPP) is such a command whose body picks its starred form,
-- MT_delim_NAME_star:, whose meaning holds the delimiters and the body: reported as "NAME* "
-- (decision 114).
local function paired(name, inner)
  if inner and inner:find('->\\@ifstar {\\@nameuse {MT_delim_', 1, true) == 1 then
    core.math_macro({name = name .. '* ', meaning = token.get_meaning('MT_delim_' .. name .. '_star:')})
  end
end

function M.macros()
  local path = os.getenv('RELATEX_MATH_MACROS')
  local file = path and io.open(path, 'r')
  if not file then return end
  for name in file:lines() do
    if name ~= '' then
      local meaning = token.get_meaning(name)
      core.math_macro({name = name, meaning = meaning})
      if meaning and meaning == '->\\protect \\' .. name .. '  ' then
        local inner = token.get_meaning(name .. ' ')
        core.math_macro({name = name .. ' ', meaning = inner})
        paired(name, inner)
      end
    end
  end
  file:close()
end

-- Appended after the class and packages have set their own \everymath and \everydisplay.
function M.hook(collector)
  core = collector
  if not M.enabled() then return end
  local hooks = {'\\everymath\\expandafter{\\the\\everymath\\directlua{ltd.math_source.open()}}',
    '\\everydisplay\\expandafter{\\the\\everydisplay\\directlua{ltd.math_source.open()}}'}
  if author_math() then hooks[#hooks + 1] = '\\AddToHook{enddocument}{\\directlua{ltd.math_source.macros()}}' end
  for _, name in ipairs(M.GROUPED) do
    hooks[#hooks + 1] = string.format('\\AddToHook{env/%s/begin}{\\directlua{ltd.math_source.group_begin(%q)}}', name, name)
    hooks[#hooks + 1] = string.format('\\AddToHook{env/%s/after}{\\directlua{ltd.math_source.group_end()}}', name)
  end
  -- Read with LaTeX's own catcodes: a package may have made _ active by now (syntax.sty).
  tex.sprint(luatexbase.registernumber('catcodetable@atletter'), table.concat(hooks))
end

return M
