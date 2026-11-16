-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Native graphicx resources/operations, observed without editing node lists.
local M = {}
M.limits = require('relatex-graphic-limits')
local source_name = require('relatex-source-name')
local paths = require('relatex-path')
local attribute = luatexbase.new_attribute('relatex.graphic.use')
local subtypes = {}
for id, name in pairs(node.subtypes(node.id('rule'))) do subtypes[name] = id end
assert(subtypes.image, 'Image rule subtype unavailable')
local writer, uses = nil, 0
local pending, sources, bindings, read_stack, use_stack = {}, {}, {}, {}, {}
-- The name each resource was requested by (a case-folded find reads another spelling).
local requested = {}
-- The raster image resources (PNG, JPEG), by index (decision 162).
local rasters = {}
-- The path each resource was read from, relative to TeX's working directory (the main file's).
local read_paths = {}
local clip_stack = {}
local function emit(code, fields)
  fields.code, fields.source = code, 'graphics-def-luatex-media'
  if writer then writer('diagnostic', fields) else pending[#pending + 1] = fields end
end
local function unsupported(reason)
  emit('unsupported_graphic_source', {reason = reason})
end
local function resource(cache)
  return tonumber((token.get_macro(cache) or ''):match('^\\useimageresource%s+(%d+)$'))
end
-- A file of a TeX tree (a package's own image, such as doclicense's badge) is found by kpathsea
-- under an absolute name: it is copied into the working directory (relatex-texmf/, named by its
-- place in the tree) so that the export reads it like any figure of the source.
local tree_variables = {'TEXMFDIST', 'TEXMFHOME', 'TEXMFLOCAL', 'TEXMFMAIN'}
local function tree_relative(absolute)
  if absolute:find('%.%.') or absolute:find('\\') or absolute:find('%z') then return nil end
  for _, name in ipairs(tree_variables) do
    local root = (kpse.expand_var('$' .. name) or ''):gsub('/+$', '')
    if paths.is_absolute(root) and paths.comparable(absolute:sub(1, #root + 1)) == paths.comparable(root) .. '/' then
      return absolute:sub(#root + 2)
    end
  end
  return nil
end
local function copy_tree_file(read)
  local relative = tree_relative(read)
  local input = relative and io.open(read, 'rb')
  if not input then return nil end
  local bytes = input:read(67108865); input:close()
  if not bytes or #bytes > 67108864 then return nil end
  local path = 'relatex-texmf'
  for part in relative:gmatch('[^/]+') do
    lfs.mkdir(path)
    path = path .. '/' .. part
  end
  local output = io.open(path, 'wb')
  if not output then return nil end
  output:write(bytes); output:close()
  return path
end
-- The image read from filename (relative to TeX's working directory), named as the source names it.
local function fingerprint(filename)
  local read = filename:gsub('^%./', '')
  local path = source_name(read)
  if paths.is_absolute(path) or path:find('\\') or path:find('%z') then return nil end
  for part in path:gmatch('[^/]+') do if part == '..' or part == '.' then return nil end end
  local file = io.open(read, 'rb')
  -- kpathsea's case-folding search reads LG_fwhm_in2.pdf for \includegraphics{LG_Fwhm_in2.pdf}:
  -- the file read is the one found, when only the letter case differs.
  if not file and kpse and kpse.find_file then
    local found = (kpse.find_file(read, 'graphic/figure') or ''):gsub('^%./', '')
    if found ~= read and found:lower() == read:lower() then
      read, path = found, source_name(found)
      file = io.open(read, 'rb')
    end
  end
if not file and kpse and kpse.find_file then
  local copy = copy_tree_file(kpse.find_file(read, 'graphic/figure') or '')
  if copy then read, path = copy, source_name(copy); file = io.open(read, 'rb') end
end
if not file then return nil end
  local bytes = file:read(67108865); file:close()
  if not bytes or #bytes > 67108864 then return nil end
  local mime
  if bytes:sub(1,8) == '\137PNG\13\10\26\10' then mime = 'image/png'
  elseif bytes:sub(1,2) == '\255\216' then mime = 'image/jpeg'
  elseif bytes:sub(1,5) == '%PDF-' then mime = 'application/pdf' end
  if not mime then return nil end
  local digest = sha2.digest256(bytes):gsub('.', function(c) return string.format('%02x', c:byte()) end)
  return {filename = path, sha256 = digest, bytes = #bytes, mime_type = mime}, read
end
local function image_in(box)
  local found, count = nil, 0
  local function visit(head)
    for n in node.traverse(head) do
      if n.id == node.id('rule') and n.subtype == subtypes.image then
        found, count = {index = n.index, natural_width = n.width, natural_height = n.height, natural_depth = n.depth}, count + 1
      elseif n.id == node.id('hlist') or n.id == node.id('vlist') then visit(n.head) end
    end
  end
  if box then visit(box.head) end
  return count == 1 and found or nil
end
local function uses_in(box)
  local found = {}
  local function visit(head)
    for n in node.traverse(head) do
      local use = node.has_attribute(n, attribute)
      if n.id == node.id('rule') and bindings[use] then found[use] = true end
      if n.id == node.id('hlist') or n.id == node.id('vlist') then visit(n.head) end
    end
  end
  if box then visit(box.head) end
  return found
end
M.fingerprint = fingerprint
function M.start(emit_record)
  writer = emit_record
  M.limits.start(emit_record)
  for _, fields in ipairs(pending) do writer('diagnostic', fields) end
  pending = {}
end
function M.before_read(previous)
  read_stack[#read_stack + 1] = previous
end
function M.after_read(filename, cache, last, box, page, pagebox)
  local previous = assert(table.remove(read_stack), 'Unbalanced graphic resource read')
  local id = resource(cache)
  local image, read = fingerprint(filename)
  if not id or not image then unsupported('unbound or unsupported image resource'); return end
  local prior = sources[id]
  if prior then
    if prior.sha256 ~= image.sha256 or prior.filename ~= image.filename then unsupported('cached source changed') end
    return
  end
  if previous == last or id ~= last then unsupported('unobserved cached resource'); return end
  local natural = image_in(box)
  if not natural then unsupported('ambiguous native image resource'); return end
  image.resource, image.index = id, natural.index
  image.natural_width, image.natural_height, image.natural_depth = natural.natural_width, natural.natural_height, natural.natural_depth
  image.page, image.pagebox = tonumber(page) or 1, pagebox
  sources[id] = image
  if image.mime_type ~= 'application/pdf' then rasters[natural.index] = true end
  requested[id] = source_name((filename:gsub('^%./', '')))
  read_paths[id] = read
  emit('graphic_source', image)
end
function M.begin_use(filename, cache, width, height, clip, viewport)
  uses = uses + 1
  use_stack[#use_stack + 1] = tex.getattribute(attribute) or -2147483647
  tex.setattribute(attribute, uses)
  local id = resource(cache)
  local source = sources[id]
  local name = source_name((filename:gsub('^%./', '')))
  if not source or (source.filename ~= name and requested[id] ~= name) or width <= 0 or height <= 0 then
    unsupported('unbound image placement'); return
  end
  local use = {use = uses, resource = id, index = source.index, width = width, height = height,
    clip = clip, viewport = viewport, transforms = {}}
  bindings[uses] = use
  emit('graphic_use', {use = uses, resource = id, width = width, height = height,
    clip = clip, viewport = viewport})
  -- Where the source includes it (decision 115): the file TeX is reading and its line, the call
  -- of the author's macro when one wraps \includegraphics; a name outside the source is left out.
  local file = source_name(((status.filename or ''):gsub('^%./', '')))
  if type(file) == 'string' and file ~= '' and not paths.is_absolute(file) and not file:find('%.%./') and tex.inputlineno > 0 then
    emit('graphic_origin', {use = uses, file = file, line = tex.inputlineno})
  end
end
-- What the author says about the graphic just bound (decision 096): its alt, actualtext and
-- artifact keys, and the figure environment that holds it (0 outside one), for \Description.
function M.alternative(alt, given, actual, artifact, figure)
  if not bindings[uses] then return end
  local fields = {use = uses}
  if given then fields.alt = alt end
  if actual ~= '' then fields.actualtext = actual end
  if artifact then fields.artifact = true end
  if figure > 0 then fields.figure = figure end
  if given or actual ~= '' or artifact or figure > 0 then emit('graphic_alternative', fields) end
end
-- The tagging project's alt, actualtext or artifact key given to the PGF picture being drawn
-- (decision 111; pgf accepts them and ignores them without tagging), with the figure environment
-- that holds it. One record per key; the product merges a picture's records.
function M.picture_alternative(key, value, figure)
  local graphic = M.limits.current()
  if not graphic then return end
  local fields = {graphic = graphic}
  if key == 'artifact' then fields.artifact = true else fields[key] = value end
  if figure > 0 then fields.figure = figure end
  emit('picture_alternative', fields)
end
-- The figure environment that holds the PGF picture being drawn, for its \relatexalt (decision 125,
-- amendment 2): a picture_alternative record with no key, merged with the picture's others.
function M.picture_figure(figure)
  local graphic = M.limits.current()
  if not graphic or figure <= 0 then return end
  emit('picture_alternative', {graphic = graphic, figure = figure})
end
-- A chess diagram (package chessboard, decision 163) is a drawing: one picture context around
-- \chessboard, bound to its figure like a PGF picture. What the source gives about it is its
-- position: the first rank the board prints reads the board's squares, once, into a
-- picture_alternative record with drawing "chess", the board's files and ranks and the piece
-- placement in FEN (the same squares chessboard's own storefen reads). The board's glyphs are
-- judged when it closes: a board without a glyph of a chess diagram font is no drawing.
local boards = {}
function M.chess_begin(figure)
  M.limits.begin('picture')
  boards[#boards + 1] = {graphic = M.limits.current()}
  M.picture_figure(figure)
end
local function chess_macro(name)
  return token.get_macro('board@val@' .. name)
end
local function chess_number(name)
  return tonumber(chess_macro(name) or '')
end
-- A square's piece: a FEN letter, false for an empty square, nil when chessboard holds something
-- else there (a piece of another language, an unset square).
local function chess_square(file, rank)
  local piece = chess_macro('f' .. file .. 'r' .. rank .. '@piecechar')
  if piece == 'empty' then return false end
  if piece and piece:match('^[KQRBNPkqrbnp]$') then return piece end
end
-- The piece placement of the board's store area (FEN's first field), from its top rank down.
function M.chess_placement(first_file, last_file, top_rank, bottom_rank)
  local ranks = {}
  for rank = top_rank, bottom_rank, -1 do
    local row, empty = {}, 0
    for file = first_file, last_file do
      local piece = chess_square(file, rank)
      if piece == nil then return nil end
      if piece then
        if empty > 0 then row[#row + 1] = tostring(empty) end
        row[#row + 1], empty = piece, 0
      else
        empty = empty + 1
      end
    end
    if empty > 0 then row[#row + 1] = tostring(empty) end
    ranks[#ranks + 1] = table.concat(row)
  end
  return table.concat(ranks, '/')
end
function M.chess_position()
  local board = boards[#boards]
  if not board or not board.graphic or board.read then return end
  board.read = true
  local first, last = chess_number('startstorefilenum'), chess_number('stopstorefilenum')
  local top, bottom = chess_number('startstoreranknum'), chess_number('stopstoreranknum')
  if not (first and last and top and bottom) or first > last or bottom > top then return end
  local fields = {graphic = board.graphic, drawing = 'chess', files = last - first + 1, ranks = top - bottom + 1}
  fields.fen = M.chess_placement(first, last, top, bottom)
  emit('picture_alternative', fields)
end
function M.chess_end()
  local board = assert(table.remove(boards), 'Unbalanced chess diagram')
  M.limits.finish_picture()
  if board.graphic then M.limits.chess_board(board.graphic) end
end
-- acmart's \Description[short]{long} of figure environment figure.
function M.description(figure, short, long)
  if figure <= 0 or (short == '' and long == '') then return end
  local fields = {figure = figure, description = long}
  if short ~= '' then fields.short = short end
  emit('figure_description', fields)
end
-- relatex's \relatexalt{alt}{short}[details] of figure environment figure (decision 125,
-- amendment 2): nothing without an alternative or outside a figure.
function M.relatex_alt(figure, alt, short, details)
  if figure <= 0 or alt == '' then return end
  local fields = {figure = figure, alt = alt}
  if short ~= '' then fields.short = short end
  if details ~= '' then fields.details = details end
  emit('figure_alt', fields)
end
function M.end_use()
  local previous = assert(table.remove(use_stack), 'Unbalanced graphic use')
  tex.setattribute(attribute, previous)
end
function M.begin_clip()
  -- The driver is still inside its open clipping hbox here. Its box register
  -- is not assigned until the first primitive in GPT@DoClipEnd closes it.
  local use = tex.getattribute(attribute)
  clip_stack[#clip_stack + 1] = bindings[use] and use or false
end
function M.clip_form(index)
  local use = table.remove(clip_stack)
  assert(use ~= nil, 'Unbalanced graphic clipping operation')
  local binding = bindings[use]
  if not binding then unsupported('unbound clipped image form'); return end
  -- saveboxresource consumes @tempboxa; useboxresource appends this exact native
  -- form reference to the current list. Observe it instead of an emptied box.
  local form = tex.nest.top.tail
  if not form or form.id ~= node.id('rule') or form.subtype ~= subtypes.box or form.index ~= index then
    unsupported('clipped form has no matching native reference'); return
  end
  node.set_attribute(form, attribute, use)
  binding.form = {index = index, width = form.width, height = form.height, depth = form.depth}
end
function M.transform(kind, box, values)
  -- graphicx's internal scale is already represented by requested dimensions.
  if bindings[tex.getattribute(attribute)] and kind == 'scale' then return end
  local found = uses_in(box)
  local use, count
  count = 0
  for id in pairs(found) do use, count = id, count + 1 end
  if count == 0 then return end
  if count ~= 1 then emit('unsupported_graphic_transform', {transform = kind, reason = 'multiple graphic group'}); return end
  values.kind = kind
  values.input_width, values.input_height, values.input_depth = box.width, box.height, box.depth
  local transforms = bindings[use].transforms
  transforms[#transforms + 1] = values
end
-- The use whose image (or clipped form) the rule node n paints, with its binding; nil for any
-- other node. A bound image is the graphic wherever it sits: one typeset inside a formula
-- (adjustbox's valign=c sets it in $\vcenter{\hbox{...}}$) is still the image, never the
-- formula's ink (a painted rule).
local function bound(n)
  if n.id ~= node.id('rule') then return nil end
  local use = node.has_attribute(n, attribute)
  local binding = bindings[use]
  if not binding then return nil end
  local form = binding.form
  local matches_image = n.subtype == subtypes.image and n.index == binding.index
  local matches_form = form and n.subtype == subtypes.box and n.index == form.index
  if not matches_image and not matches_form then return nil end
  return use, binding, matches_form
end
-- Whether rule n places a raster image (a PNG or JPEG file, decision 162): such an image paints no
-- text, so no glyph of the page is its own. The images read from such files, by the image index
-- their rules carry (not the resource number \useimageresource names): graphicx's reads and PGF's
-- \pgfdeclareimage (a beamer \logo). Read only.
function M.raster(n)
  return n.id == node.id('rule') and n.subtype == subtypes.image and rasters[n.index] == true
end
-- PGF saved an image from filename (\pgfsys@defineimage); box `box` holds a rule placing it, set only
-- to be read and discarded. When the file is a PNG or JPEG, the image index the rule carries is a raster.
function M.pgf_image(filename, box)
  local path = type(filename) == 'string' and filename ~= '' and filename or nil
  local file = path and (io.open(path, 'rb') or (kpse and kpse.find_file and io.open(kpse.find_file(path, 'graphic/figure') or '', 'rb')))
  if not file then return end
  local head = file:read(8) or ''
  file:close()
  if head ~= '\137PNG\13\10\26\10' and head:sub(1, 2) ~= '\255\216' then return end
  local holder = tex.box[box]
  local found = holder and holder.head
  while found and not (found.id == node.id('rule') and found.subtype == subtypes.image) do found = found.next end
  if found then rasters[found.index] = true end
end
function M.is_image(n)
  return bound(n) ~= nil
end
function M.identity(n)
  local use, binding, matches_form = bound(n)
  if not use then return nil end
  local result = {type = 'image', use = use, resource = binding.resource, index = n.index,
    width = n.width, height = n.height, depth = n.depth}
  if matches_form then result.form = true end
  return result
end
function M.finish()
  M.limits.finish()
  assert(#read_stack == 0 and #use_stack == 0 and #clip_stack == 0, 'Unclosed graphic context')
  for id, source in pairs(sources) do
    local current = fingerprint(read_paths[id])
    if not current or current.sha256 ~= source.sha256 then unsupported('source changed before finish') end
  end
  for use = 1, uses do
    local binding = bindings[use]
    if binding then
      local fields = {use = use, transforms = binding.transforms}
      if binding.form then fields.form = binding.form end
      emit('graphic_placement', fields)
    end
  end
  writer = nil
end
return M
