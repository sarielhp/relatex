-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- timing profiler: wraps luatexbase callbacks and the functions of the ltd table (two levels)
local clock = os.clock
local totals, calls = {}, {}
local function wrap(name, fn)
  return function(...)
    local start = clock()
    local r1, r2, r3, r4 = fn(...)
    totals[name] = (totals[name] or 0) + (clock() - start)
    calls[name] = (calls[name] or 0) + 1
    return r1, r2, r3, r4
  end
end
local add = luatexbase.add_to_callback
luatexbase.add_to_callback = function(callback, fn, description)
  return add(callback, wrap("callback:" .. callback .. ":" .. tostring(description), fn), description)
end
function relatex_prof_wrap()
  local seen = {}
  local function walk(table_, prefix, depth)
    if seen[table_] then return end
    seen[table_] = true
    for key, value in pairs(table_) do
      if type(value) == "function" then
        rawset(table_, key, wrap(prefix .. tostring(key), value))
      elseif type(value) == "table" and depth < 2 and type(key) == "string" and key ~= "_G" then
        walk(value, prefix .. tostring(key) .. ".", depth + 1)
      end
    end
  end
  if ltd then walk(ltd, "ltd.", 0) end
  if ltd_external_glyph_map then walk(ltd_external_glyph_map, "glyphmap.", 0) end
  local g = require("relatex-glyphs"); walk(g, "glyphs.", 0)
end
add("stop_run", function()
  local list = {}
  for k, v in pairs(totals) do list[#list + 1] = {k, v, calls[k]} end
  table.sort(list, function(a, b) return a[2] > b[2] end)
  local out = io.open("relatex_prof.txt", "w")
  for i = 1, math.min(60, #list) do out:write(string.format("%8.3f %8d %s\n", list[i][2], list[i][3], list[i][1])) end
  out:close()
end, "relatex.prof")
local began
function relatex_prof_mark(label)
  local f = io.open("relatex_marks.txt", "a")
  f:write(string.format("%.2f %s\n", os.clock(), label))
  f:close()
end
