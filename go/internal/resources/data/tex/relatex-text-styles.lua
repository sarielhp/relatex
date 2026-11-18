-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Bind selected NFSS declarations to actual font IDs; copied boxes keep IDs.
local M = {}
local bindings, writer, missing = {}, nil, {}
local function trim(text) return (text:gsub('^%s+', ''):gsub('%s+$', '')) end
local function declaration(id, style)
  writer('diagnostic', {code = 'external_text_font', source = 'nfss-selectfont', font = id, style = style})
  local f = font.getfont(id)
  local filename = f and f.filename
  if not filename or filename == '' then return end
  local parameters = {}
  for _, key in ipairs({'space', 'space_stretch', 'space_shrink', 'quad'}) do
    if f.parameters and type(f.parameters[key]) == 'number' then parameters[key] = f.parameters[key] end
  end
  writer('diagnostic', {code='external_text_font_source', source='luatex-font-table', font=id,
    font_name=style.font_name, filename=filename, format=f.format, psname=f.psname,
    size_sp=f.size, units_per_em=f.units_per_em, parameters=parameters})
end
function M.start(emit)
  writer = emit
  for id, style in pairs(bindings) do declaration(id, style) end
end
function M.observe(encoding, family, series, shape, roman, sans, mono)
  -- \fontseries{m<newline>} reaches NFSS as "m " (trailing space); the font is the same.
  encoding, series, shape = trim(encoding), trim(series), trim(shape)
  local id = font.current()
  local f = font.getfont(id)
  local name = f and (f.fullname or f.psname or f.name)
  if not name or not f.size or f.size <= 0 then return end
  local generic = family == mono and 'monospace' or family == sans and 'sans-serif'
    or family == roman and 'serif' or 'unknown'
  local style = {source = 'nfss-font-id', font_id = id, font_name = name, size_sp = f.size,
    encoding = encoding, family = family, series = series, shape = shape, generic = generic}
  if bindings[id] then
    -- Name each differing field with its first and later value, so a reader can tell NFSS
    -- substitution aliases (same rendering) from genuinely ambiguous styles.
    local fields = {}
    for key, value in pairs(style) do
      if bindings[id][key] ~= value then fields[key] = {bindings[id][key], value} end
    end
    if next(fields) and writer then
      writer('diagnostic', {code = 'conflicting_text_font_declaration', font = id, fields = fields})
    end
    return
  end
  bindings[id] = style
  if writer then declaration(id, style) end
end
-- A direct TeX font is reported again in each flow owner that prints it (a section heading set in
-- the \uno of an earlier abstract heading), so the reader can bind every such owner.
function M.new_owner() missing = {} end
function M.identity(n)
  if bindings[n.font] then return n.font end
  if not missing[n.font] then
    local f = font.getfont(n.font)
    writer('diagnostic', {code = 'unobserved_text_font', font = n.font, first_char = n.char,
      first_text = require('relatex-glyphs').character(n),
      font_name = f and (f.fullname or f.psname or f.name), size_sp = f and f.size})
    missing[n.font] = true
  end
end
-- Whether a font is a declared text font.
function M.identity_of(id) return bindings[id] ~= nil end
-- Whether a declared text font is monospaced (\tt, \texttt).
function M.is_monospaced(id) return bindings[id] ~= nil and bindings[id].generic == 'monospace' end
-- The declared text font whose interword space sets a glue (sets(id)): an upright medium serif
-- first (the \normalfont of the text around), then the lowest id; nil when none does.
function M.space_font(sets)
  local ids = {}
  for id in pairs(bindings) do if sets(id) then ids[#ids + 1] = id end end
  table.sort(ids)
  for _, id in ipairs(ids) do
    local style = bindings[id]
    if style.generic == 'serif' and style.series == 'm' and style.shape == 'n' then return id end
  end
  return ids[1]
end
return M
