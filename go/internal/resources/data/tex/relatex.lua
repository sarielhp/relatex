-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Scalar snapshots only: no node pointers survive a callback.
require('lualibs')
local glyphs = require('relatex-glyphs')
local M = {}
M.text_symbols = require('relatex-text-symbols')
if os.getenv('RELATEX_STRATEGY') == 'bridge' or os.getenv('RELATEX_STRATEGY') == 'external' then
  M.math_bridge = require('relatex-math')
  M.flow = require('relatex-flow')
end
local candidate_attr = luatexbase.new_attribute('relatex.candidate')
local position_attr = luatexbase.new_attribute('relatex.position')
local context_attr = luatexbase.new_attribute('relatex.context')
local discretionary_attr = luatexbase.new_attribute('relatex.discretionary')
local decoration_attr = luatexbase.new_attribute('relatex.page_decoration')
local decoration_style_attr = luatexbase.new_attribute('relatex.decoration_style')
local page_attributes = {}
local active, output, sequence, total, candidate, page = false, nil, 0, 0, 0, 0
local context, context_id = 'paragraph', 0
local context_total, section_id = 0, 0
-- Proof environments and their open frames (M.proof_begin).
local proof_names, proof_frames, proof_content = {proof = true}, {}, {}
local pending_runin
local section_levels = {}
-- A part and a chapter sit above the section (book, report, memoir, amsbook). In a class with
-- chapters the reported depth counts from the chapter: chapter 1, section 2 (a part shares the top).
local heading_levels = {part=-1, chapter=0, section=1, subsection=2, subsubsection=3, paragraph=4, subparagraph=5}
local level_offset = 0
local contexts = {[0] = 'paragraph'}
local parents, context_stack = {[0] = 0}, {}
local math_events = 0
local named_contexts = {}
-- Labels set right after \begin{subequations} name the first display of the group.
local subequations_open, subequation_labels = false, {}
-- A display set inside another (align* in gather) is a cell of the outer display, like aligned.
local equation_depth = 0
local theorem_counters = {}
local glyph_mappings = {}
local character
local glyph, glue, hlist, vlist, disc = node.id('glyph'), node.id('glue'), node.id('hlist'), node.id('vlist'), node.id('disc')

local function layout_discretionary(n)
  return node.has_attribute(n, discretionary_attr) ~= nil
end

local function emit(kind, payload)
  sequence = sequence + 1
  payload.seq, payload.kind = sequence, kind
  local line = utilities.json.tostring(payload)
  -- Bounds sized for long real papers (2 of 50 cohort papers exceeded 16 MB / 256 KB; a 104-page
  -- paper of displays and \mathchoice accents, third cohort 853, about 75 MB / 290 000 records;
  -- the 838-page Geometric Approximation Algorithms book, over 128 MiB).
  assert(#line <= 1048576, 'relatex record limit')
  total = total + #line + 1
  assert(total <= 268435456 and sequence <= 1600000, 'relatex stream limit')
  output:write(line, '\n')
  output:flush()
end

function M.start()
  if active then M.text_symbols.observe(); return end
  output = assert(io.open(assert(os.getenv('RELATEX_STREAM')), 'wb'))
  local header = {protocol = '0.6', collector = '0.6.1', attempt = os.getenv('RELATEX_ATTEMPT'),
    pass = tonumber(os.getenv('RELATEX_PASS')), input = os.getenv('RELATEX_INPUT'),
    engine = status.banner, adapter = os.getenv('RELATEX_STRATEGY')}
  if M.notes then header.extensions = {'notes-0.1'} end
  if M.graphics then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'graphics-0.2'
  end
  if M.tables then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'tables-0.1'
    if M.tables.modern then header.extensions[#header.extensions + 1] = 'tables-0.2' end
  end
  if M.page_totals then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'page-totals-0.1'
  end
  if os.getenv('RELATEX_STRATEGY') == 'external' and M.math_bridge then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'external-math-0.1'
  end
  if M.text_styles then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'text-style-0.1'
  end
  if M.external_lists then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'lists-0.1'
  end
  if M.citations then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'citations-0.1'
  end
  if M.external_links then
    header.extensions = header.extensions or {}
    header.extensions[#header.extensions + 1] = 'external-links-0.1'
  end
  emit('header', header)
  active = true
  if M.citations then M.citations.start(emit, M.flow) end
  if M.external_links then M.external_links.start(emit) end
  if M.index then M.index.start(emit) end
  if M.external_lists then M.external_lists.start(emit) end
  if M.text_styles then M.text_styles.start(emit) end
  M.text_symbols.observe()
  if M.math_bridge then M.math_bridge.start(emit, M.current_context) end
  if M.notes then M.notes.start(emit, M) end
  if M.graphics then M.graphics.start(emit) end
  if M.tables then M.tables.start(emit) end
  if M.page_totals then M.page_totals.start(emit, function() return page end) end
  if M.beamer then M.beamer.start(emit) end
  if M.flow then M.flow.start(emit, M.math_bridge, character, layout_discretionary, M.notes, M.graphics) end
end

function M.current_context()
  return context_id, context
end

function M.current_parent()
  return context == 'paragraph' and parents[context_id] or context_id
end

function M.page_begin(source)
  page_attributes[#page_attributes + 1] = tex.getattribute(decoration_attr) or -2147483647
  tex.setattribute(decoration_attr, source == 'acmart-copyright' and 2 or 1)
  if M.math_bridge then M.math_bridge.page_begin() end
  if M.tables then M.tables.page_begin() end
end

function M.page_end()
  if M.tables then M.tables.page_end() end
  if M.math_bridge then M.math_bridge.page_end() end
  local saved = assert(table.remove(page_attributes), 'Unbalanced page decoration context')
  tex.setattribute(decoration_attr, saved)
end

-- Names the current context as the owner of a LaTeX counter, so a \label under
-- that counter (an enumerate item's enumi) targets it.
function M.name_context(counter) named_contexts[counter] = context_id end

-- Forgets the owner of a counter whose \label runs before its caption is set (a listing's
-- label= key, which listings runs ahead of the caption in either position): the label then names
-- the caption that follows it, not the previous listing's.
function M.forget_context(counter) named_contexts[counter] = nil end

function M.context(name, parent)
  context_total = context_total + 1
  context, context_id = name, context_total
  contexts[context_id] = name
  parents[context_id] = parent or 0
  if name ~= 'paragraph' then named_contexts[name] = context_id end
  -- Logical section/environment lifetime follows explicit hooks, not a TeX
  -- styling group (thebibliography starts its heading inside such a group).
  tex.setattribute('global', context_attr, context_id)
  if active then emit('context', {entity = context_id, role = name, parent = parents[context_id]}) end
end

local function external_section(counter, runin)
  local level = heading_levels[counter] or 1
  local parent = 0
  for index = level - 1, -1, -1 do
    if section_levels[index] then parent = section_levels[index]; break end
  end
  for index = level, 5 do section_levels[index] = nil end
  if runin then
    M.context('paragraph', parent)
    pending_runin = context_id
    section_id = parent
  else
    M.context('section', parent)
    section_id = context_id
    section_levels[level] = context_id
  end
  named_contexts.section = section_levels[1]
  named_contexts[counter] = context_id
  -- The heading's sectioning command, for writers that rebuild the author's structure.
  if active then emit('diagnostic', {code='heading_context', source='latex-heading-hooks', entity=context_id,
    parent=parent, counter=counter, level=math.max(1, level + level_offset), runin=runin and true or false}) end
end

-- A class with \chapter: its headings are observed, and sections count one level deeper.
function M.chapter_levels() level_offset = 1 end

-- \frontmatter, \mainmatter and \backmatter close every open part and chapter: the back
-- matter's bibliography and index are not inside the last part.
function M.matter()
  if os.getenv('RELATEX_STRATEGY') ~= 'external' or #context_stack > 0 then return end
  for index = -1, 5 do section_levels[index] = nil end
  named_contexts.section, named_contexts.chapter, named_contexts.part = nil, nil, nil
  section_id = 0
  M.context('paragraph', 0)
end

-- Headings skipped by section_begin: their section_end is skipped too.
local skipped_headings = 0

-- A heading skipped inside the open block: the diagnostic names the heading's command, the block and
-- the source position (once per position, the heading's own wrappers may both observe it).
local reported_headings = {}
local function heading_inside_block(counter)
  if not active then return end
  local file = ((status.filename or tex.jobname or ''):gsub('^%./', ''))
  local key = file .. ':' .. tex.inputlineno .. ':' .. counter
  if reported_headings[key] then return end
  reported_headings[key] = true
  emit('diagnostic', {code = 'heading_inside_block', source = 'latex-heading-hooks', counter = counter,
    block = context_stack[#context_stack].entered, file = file, line = tex.inputlineno})
end

function M.section_begin(counter, runin)
  counter = counter or 'section'
  local previous = named_contexts.section
  -- A heading set inside an open block or list (a \subsection in an itemize is legal LaTeX) has no place
  -- in the bounded structure: its title stays text of the enclosing block, and says so.
  if #context_stack > 0 then
    skipped_headings = skipped_headings + 1
    heading_inside_block(counter)
    return
  end
  if os.getenv('RELATEX_STRATEGY') == 'external' then external_section(counter, runin); return end
  -- Subheadings own their title, while body/float scope remains the enclosing
  -- top-level section. Complete heading hierarchy is outside this bounded IR.
  if runin then
    M.context('paragraph', section_id)
    pending_runin = context_id
  else
    local top = counter == 'section' or counter == 'chapter' or counter == 'part'
    M.context('section', top and 0 or section_id)
    if top then section_id = context_id else named_contexts.section = previous end
  end
  named_contexts[counter] = context_id
end

local runin_saved = {}

function M.runin_begin(id)
  runin_saved[#runin_saved + 1] = tex.attribute[context_attr]
  tex.setattribute(context_attr, id)
  if M.flow then M.flow.runin_title(true) end
end

-- A display's tag (\maketag@@@, \@eqnnum) and an \eqref, whose number is set the same way.
function M.equation_tag(on)
  if M.flow then M.flow.equation_tag(on) end
end
function M.equation_reference(on)
  if M.flow then M.flow.equation_reference(on) end
end

function M.runin_end()
  tex.setattribute(context_attr, table.remove(runin_saved))
  if M.flow then M.flow.runin_title(false) end
end

function M.section_end()
  if skipped_headings > 0 then skipped_headings = skipped_headings - 1; return end
  M.context('paragraph', section_id)
end

local heading_counters = {
  ['section-@startsection'] = 'section', ['subsection-@startsection'] = 'subsection',
  ['subsubsection-@startsection'] = 'subsubsection', ['paragraph-@startsection'] = 'paragraph',
  ['subparagraph-@startsection'] = 'subparagraph', chapter = 'chapter', part = 'part'
}

function M.heading(name, ending, runin)
  local counter = heading_counters[name]
  if not counter then return end
  -- A heading inside a theorem or proof (a proof split over several sections, second cohort 268)
  -- has no place in the bounded structure: its title stays text of the block, and says so.
  if #context_stack > 0 then
    if not ending then heading_inside_block(counter) end
    return
  end
  if ending then
    if not runin then M.section_end() end
    -- A run-in title is typeset with the next paragraph, which may already be a theorem's:
    -- its glyphs carry the heading's own entity.
    if runin and pending_runin then tex.sprint(string.format('\\csname ltd@runinwrap\\endcsname{%d}', pending_runin)) end
  else
    M.section_begin(counter, runin)
  end
end

local float_stack, caption_stack, float_categories = {}, {}, {}

function M.float_begin(category)
  M.enter('paragraph')
  float_stack[#float_stack + 1] = context_id
  float_categories[#float_categories + 1] = category
  if active then emit('diagnostic', {code='float_context',source='latex-float-environment-hooks',
    entity=context_id,parent=parents[context_id],category=category}) end
end

function M.float_end()
  assert(table.remove(float_stack), 'Unbalanced float context')
  table.remove(float_categories)
  M.leave('paragraph')
end

-- Floats of other types close at the kernel's float end. A one-column \end@dblfloat runs
-- \end@float itself, so the second end finds its float already closed.
local other_floats = {}
function M.other_float_begin(category)
  M.float_begin(category)
  other_floats[#other_floats + 1] = context_id
end

-- A class float (\@float{table} from a class environment, aastex 5 deluxetable) is a float of its own
-- only when no float is open: LaTeX floats do not nest, so inside an open table the kernel float is that
-- table's own (sn-jnl's table wraps the original environment, papers3/0810).
function M.class_float_begin(category)
  if #float_stack == 0 then M.other_float_begin(category) end
end

function M.other_float_end()
  local open = other_floats[#other_floats]
  if open and open == float_stack[#float_stack] then
    table.remove(other_floats)
    M.float_end()
  end
end

function M.caption_begin(counter, numbered, source)
  source = source or 'caption-package-public-hooks'
  assert(source == 'caption-package-public-hooks' or
    (source == 'latex-native-caption-hooks-1' and os.getenv('RELATEX_STRATEGY') == 'external'),
    'Unsupported caption observer')
  -- Any caption is observed: in a float (whose type it need not share) or on its own
  -- (a listing's caption), where it reads as an ordinary paragraph labels can name. A caption the
  -- class formatter hook already opened (the caption package keeping IEEEtran's \@makecaption) is
  -- that same caption.
  if caption_stack[#caption_stack] == 'style' then
    caption_stack[#caption_stack + 1] = 'inside'
    return
  end
  local supported = type(counter) == 'string' and counter:match('^%a+$') ~= nil
  caption_stack[#caption_stack + 1] = supported
  if not supported then
    if active then emit('diagnostic', {code='unsupported_caption_counter',counter=counter}) end
    return
  end
  M.enter('paragraph')
  named_contexts[counter] = numbered and context_id or nil
  if active then emit('diagnostic', {code='caption_context',source=source,
    entity=context_id,parent=parents[context_id],category=counter,numbered=numbered,float=float_stack[#float_stack] or 0}) end
  if M.flow then M.flow.caption_entity(context_id, float_stack[#float_stack] or 0) end
end

function M.caption_end()
  local supported = table.remove(caption_stack)
  assert(supported ~= nil, 'Unbalanced caption context')
  if supported == true or supported == 'style' then M.leave('paragraph') end
end

-- A float-package style typesets its caption itself (\@fs@capt). The caption package may
-- already observe that caption; a style caption inside it is the same caption.
function M.style_caption_begin(counter)
  if #caption_stack > 0 then
    caption_stack[#caption_stack + 1] = 'inside'
    return
  end
  if os.getenv('RELATEX_STRATEGY') ~= 'external' then
    caption_stack[#caption_stack + 1] = false
    if active then emit('diagnostic', {code='unsupported_caption_counter',counter=counter}) end
    return
  end
  M.caption_begin(counter, true, 'latex-native-caption-hooks-1')
  if caption_stack[#caption_stack] == true then caption_stack[#caption_stack] = 'style' end
end

function M.style_caption_end()
  M.caption_end()
end

function M.enter(name)
  -- List begin hooks precede the list's implicit paragraph break. Finish the
  -- run-in scope now so its deferred packing cannot overwrite the list context.
  if pending_runin and context_id == pending_runin and (name == 'ordered_list' or name == 'unordered_list') then
    pending_runin = nil
    M.context('paragraph', section_id)
  end
  context_stack[#context_stack + 1] = {id = context_id, role = context, entered = name, named = named_contexts[name]}
  local parent = M.current_parent()
  local float = float_stack[#float_stack]
  local in_float = name ~= 'paragraph' and float ~= nil and context_id == float
  M.context(name, parent)
  -- A list or theorem set in a float's body (an algorithm's steps) is a member of that float;
  -- its structural parent stays the float's own.
  if in_float and active then
    emit('diagnostic', {code='float_member', source='latex-float-environment-hooks', entity=context_id,
      parent=parent, role=name, float=float})
  end
end

-- A theorem observer's entry for ENVIRONMENT; a proof environment (a proof declared with \newtheorem, as
-- ntheorem's \newtheorem*{proof} or an author's proofsketch) is left to the proof hooks, so it is one proof,
-- not a theorem inside a proof. theorem_leave closes what theorem_enter opened.
local theorem_frames = {}
function M.theorem_enter(environment)
  local skipped = proof_names[environment] == true
  theorem_frames[#theorem_frames + 1] = skipped
  if not skipped then M.enter('theorem') end
end

function M.theorem_leave()
  local skipped = table.remove(theorem_frames)
  assert(skipped ~= nil, 'Unbalanced theorem observer')
  if not skipped then M.leave('theorem') end
end

function M.theorem_counter(counter, numbered)
  if context ~= 'theorem' and theorem_frames[#theorem_frames] == true then return end
  assert(context == 'theorem', 'Native theorem outside its structural context')
  local previous = assert(context_stack[#context_stack], 'Missing theorem parent')
  if not numbered or counter ~= 'theorem' then named_contexts.theorem = previous.named end
  theorem_counters[context_id] = numbered and counter or false
  if numbered then
    assert(counter ~= '', 'Missing native theorem counter')
    named_contexts[counter] = context_id
  end
end

-- A theorem set outside an environment of its own (thm-restate at the document level) has no
-- context: its observation is skipped.
local theorem_skipped = false
function M.skip_theorem() theorem_skipped = true end

function M.theorem_context(environment, counter, title)
  if theorem_skipped then theorem_skipped = false; return end
  if proof_names[environment] then return end -- a proof declared with \newtheorem (M.theorem_enter)
  local numbered = counter ~= ''
  M.theorem_counter(counter, numbered)
  if numbered then named_contexts[environment] = context_id end
  local register = token.create('c@' .. counter)
  local shared = token.create('c@theorem')
  emit('diagnostic', {code='theorem_context',source='amsthm-arguments',entity=context_id,
    parent=parents[context_id],environment=environment,counter=counter,title=title,numbered=numbered,
    counter_register=register.cmdname == 'assign_int' and register.mode or nil,
    theorem_register=shared.cmdname == 'assign_int' and shared.mode or nil})
end

-- A beamer block (relatex-beamer): an unnumbered theorem-like structure of its environment, whose
-- first paragraph is its title (unless `untitled`: an empty title box sets no paragraph);
-- `beamer_block` names the template family (block, alerted, example).
function M.block_context(environment, family, titled)
  M.theorem_counter('', false)
  emit('diagnostic', {code='theorem_context',source='beamer-block-templates',entity=context_id,
    parent=parents[context_id],environment=environment,counter='',numbered=false,beamer_block=family,
    untitled=not titled or nil})
end

function M.subequations_begin() subequations_open, subequation_labels = true, {} end

function M.subequations_end()
  subequations_open = false
  for _, record in ipairs(subequation_labels) do emit('label', record) end
  subequation_labels = {}
end

function M.equation_nested() return equation_depth > 1 end

function M.equation_begin(environment)
  equation_depth = equation_depth + 1
  if equation_depth > 1 then return end
  M.enter('paragraph')
  named_contexts.equation = context_id
  -- A display set in a float (amsldoc's table of display examples) is a member of that float.
  emit('diagnostic', {code='equation_context', source='latex-equation-hooks',
    entity=context_id, parent=parents[context_id], environment=environment, float=float_stack[#float_stack]})
  if subequations_open then
    for _, record in ipairs(subequation_labels) do
      record.target_entity = context_id
      emit('label', record)
    end
    subequation_labels = {}
    subequations_open = false
  end
end

function M.equation_end()
  equation_depth = equation_depth - 1
  if equation_depth > 0 then return end
  M.leave('paragraph')
end

function M.leave(name)
  local previous = assert(table.remove(context_stack), 'Unbalanced structural context')
  assert(previous.entered == name, 'Mismatched structural context')
  context, context_id = previous.role, previous.id
  tex.setattribute('global', context_attr, context_id)
end

-- Proofs, whatever defines them (plans/proof-marks.md, phase 1). `proof` (amsthm, ntheorem, the classes,
-- beamer) and every environment whose name says proof (IEEEproof, proofExt, altproof, proofsketch, Beweis,
-- preuve, dimostrazione, demostración) open a proof context. An environment whose begin code itself opens
-- a proof (a wrapper: \newenvironment{proofExt}{\begin{proof}}{\end{proof}}) gives one proof, not one inside
-- another: an inner proof that starts before the outer one holds anything (no paragraph or box captured
-- for it, no context opened inside it, nothing set in the current list) is the same proof. A proof inside
-- a proof that already has content stays nested, as the author wrote it.
local proof_words = {'proof', 'beweis', 'preuve', 'dimostrazione', 'demostracion', 'demostraci\195\179n'}

local function proof_name(name)
  if name:find('@', 1, true) then return false end
  local lower = name:lower()
  -- A derivation tree (bussproofs and ebproof: prooftree) is a display of inference rules, not a proof; its
  -- macros build boxes the proof context must not enter (papers/0229: A <box> was supposed to be here).
  if lower:find('prooftree', 1, true) then return false end
  for _, word in ipairs(proof_words) do
    if lower:find(word, 1, true) then return true end
  end
  return false
end

-- At \begin{document}: the proof environments the document defines besides `proof` (hooked in relatex.sty).
function M.proof_environments()
  local found, hash = {}, tex.hashtokens()
  for _, name in ipairs(hash) do
    if type(name) == 'string' and name ~= 'proof' and proof_name(name)
        and token.is_defined(name) and token.is_defined('end' .. name) then
      found[#found + 1] = name
    end
  end
  table.sort(found)
  for _, name in ipairs(found) do
    proof_names[name] = true
    tex.sprint('\\AddToHook{env/' .. name .. '/begin}{\\directlua{ltd.proof_begin()}}'
      .. '\\AddToHook{env/' .. name .. '/end}{\\directlua{ltd.flow.proof_ending(true)}}'
      .. '\\AddToHook{env/' .. name .. '/after}{\\directlua{ltd.proof_end()}}')
  end
  M.qed_marks(hash)
end

-- QED marks by command and by position (decision 176). The mark commands are wrapped in one symbol box
-- (relatex-flow qed_before/qed_after): \qedsymbol unless it is one of the pinned definitions relatex-proof.sty
-- observes, the one command it aliases (\def\qedsymbol{\myqedsymbol}), IEEEtran's and others' QED commands,
-- ntheorem's per-environment symbol (\proofSymbol) of every proof environment, and the mark macros a proof
-- environment's end code calls (\newenvironment{proofExt}{...}{\hfill\myqedsymbol}), found through at most
-- three levels of macros. A mark macro draws something (a formula, an image, a rule or a known symbol
-- command) and lays nothing out (no paragraph, glue, penalty or list command); internal (@) commands are
-- neither wrapped nor followed.
local PINNED_QED = {['\\openbox '] = true, DONE = true, ['\\textsquare '] = true}
local QED_COMMANDS = {'IEEEQED', 'IEEEQEDclosed', 'IEEEQEDopen', 'IEEEQEDhere', 'squareforqed', 'qedsymbol'}
local LAYOUT = {}
for name in ([[par hfill hfil hss hfilneg hskip hspace quad qquad enspace vskip vspace unskip nobreak penalty
    end endtrivlist endlist item list trivlist popQED pushQED qed qedhere newline linebreak leavevmode
    noindent indent smallskip medskip bigskip endgroup begingroup egroup bgroup relax ignorespaces
    unhbox unhcopy unvbox kern vbox vtop parbox minipage centering raggedleft]]):gmatch('%S+') do
  LAYOUT[name] = true
end
local DRAWS = {}
for name in ([[includegraphics rule vrule hrule ensuremath blacksquare square Box openbox lozenge blacklozenge
    diamond Diamond blacktriangleleft blacktriangleright triangleleft triangleright textsquare ding
    smiley frownie checkmark bigstar]]):gmatch('%S+') do
  DRAWS[name] = true
end

local function macro_body(name)
  if not token.is_defined(name) then return nil end
  local ok, body = pcall(token.get_macro, name)
  return ok and type(body) == 'string' and body or nil
end

-- A command a mark box can wrap: a macro without parameters (its before and after hooks then enclose all it
-- typesets), not one of the drawing commands themselves (\rule, \includegraphics take arguments).
local function wrappable(name)
  if DRAWS[name] or not macro_body(name) then return false end
  local ok, meaning = pcall(token.get_meaning, name)
  return ok and type(meaning) == 'string' and meaning:sub(1, 2) == '->'
end

local function draws_without_layout(body)
  local draws = body:find('$', 1, true) ~= nil or body:find('\\(', 1, true) ~= nil
    or body:find('[\226][\150\136]') ~= nil -- a literal box or square sign (U+25xx, U+220E)
  for name in body:gmatch('\\([%a@]+)') do
    if LAYOUT[name] then return false end
    if DRAWS[name] then draws = true end
  end
  return draws
end

local function end_marks(body, depth, found)
  for name in body:gmatch('\\([%a@]+)') do
    if not name:find('@', 1, true) and not LAYOUT[name] and not found[name] then
      local inner = macro_body(name)
      if inner and draws_without_layout(inner) and wrappable(name) then
        found[name] = true
      elseif inner and depth < 3 then
        end_marks(inner, depth + 1, found)
      end
    end
  end
end

-- ntheorem's [thmmarks] places a proof's end mark through \mark<n><environment><n>, defined from the aux file
-- with the environment's symbol as its body.
local function ntheorem_marks(hash, proofs, found)
  for _, name in ipairs(hash) do
    if type(name) == 'string' and name:sub(1, 4) == 'mark' then
      for _, proof in ipairs(proofs) do
        local body = name:match('^mark[ivxlcdm]+' .. proof:gsub('%p', '%%%0') .. '([ivxlcdm]*)$')
        local mark = body and macro_body(name)
        if mark and draws_without_layout(mark) then found[name] = true end
      end
    end
  end
end

function M.qed_marks(hash)
  local found = {}
  local symbol = macro_body('qedsymbol')
  if symbol and not PINNED_QED[symbol] then
    local alias = symbol:match('^\\([%a]+) ?$')
    local aliased = alias and macro_body(alias)
    if draws_without_layout(aliased or symbol) then found.qedsymbol = true end
    if aliased and draws_without_layout(aliased) then found[alias] = true end
  end
  for _, name in ipairs(QED_COMMANDS) do
    local body = name ~= 'qedsymbol' and macro_body(name)
    if body and draws_without_layout(body) then found[name] = true end
  end
  local proofs = {'proof'}
  for name in pairs(proof_names) do proofs[#proofs + 1] = name end
  for _, name in ipairs(proofs) do
    local mark = macro_body(name .. 'Symbol')
    if mark and draws_without_layout(mark) then found[name .. 'Symbol'] = true end
    local ending = macro_body('end' .. name)
    if ending then end_marks(ending, 1, found) end
  end
  ntheorem_marks(hash, proofs, found)
  if PINNED_QED[symbol or ''] then found.qedsymbol = nil end
  local names = {}
  for name in pairs(found) do
    if wrappable(name) then names[#names + 1] = name else found[name] = nil end
  end
  if found.qedsymbol and M.flow then M.flow.qed_symbol_wrapped() end
  table.sort(names)
  for _, name in ipairs(names) do
    tex.sprint('\\AddToHook{cmd/' .. name .. '/before}{\\directlua{ltd.flow.qed_before()}}'
      .. '\\AddToHook{cmd/' .. name .. '/after}{\\directlua{ltd.flow.qed_after()}}')
  end
end

-- 1 when the environment is a proof (so the theorem observers leave it to the proof hooks), else 0.
function M.proof_environment(name) tex.sprint(proof_names[name] and '1' or '0') end

-- Something the proof OWNER set in an open paragraph (a paragraph the proof began inside may hold earlier text): a glyph, a rule, a formula or a box with content (not the empty
-- indentation box). A vertical list may still hold lines set before the proof began, so it does not count;
-- the proof's own finished paragraphs are counted by capture (proof_content).
local horizontal_modes = {}
for value, name in pairs(tex.getmodevalues()) do
  if name == 'horizontal' then horizontal_modes[value], horizontal_modes[-value] = true, true end
end
local function list_has_material(owner)
  local nest = tex.nest[tex.nest.ptr]
  if not nest or not horizontal_modes[nest.mode] or not nest.head then return false end
  for n in node.traverse(nest.head) do
    local set = n.id == glyph or n.id == node.id('rule') or n.id == node.id('math')
      or ((n.id == hlist or n.id == vlist) and n.head)
    if set and node.has_attribute(n, context_attr) == owner then return true end
  end
  return false
end

function M.proof_begin()
  local top = proof_frames[#proof_frames]
  if top and not top.alias and context == 'proof' and context_id == top.id and context_total == top.total
      and not proof_content[top.id] and not list_has_material(top.id) then
    proof_frames[#proof_frames + 1] = {alias = true}
    return
  end
  M.enter('proof')
  proof_frames[#proof_frames + 1] = {id = context_id, total = context_total}
end

function M.proof_end()
  local frame = assert(table.remove(proof_frames), 'Unbalanced proof context')
  if not frame.alias then M.leave('proof') end
  if M.flow then M.flow.proof_ending(false) end
end

-- A display listing (listings: lstlisting, \lstinputlisting) is a code block (decision 120): its
-- lines are one structural context, role code. Listings do not nest; a listing set in a box (a
-- table cell) stays text of its box.
local listing_open = false
function M.listing_begin()
  if listing_open then return end
  listing_open = true
  M.enter('code')
end

function M.listing_end()
  if not listing_open then return end
  listing_open = false
  M.leave('code')
end

-- Line-break material, like a layout hyphen; listings wraps its break marks ('...') in boxes.
local function mark_discretionary(head)
  for n in node.traverse(head) do
    if n.id == glyph then node.set_attribute(n, discretionary_attr, 1)
    elseif n.id == node.id('hlist') or n.id == node.id('vlist') then mark_discretionary(n.head) end
  end
end

character = function(n)
  local text, basis, evidence = glyphs.character(n)
  if active and basis == 'components' then
    local key = n.font .. ':' .. n.char .. ':' .. text
    if not glyph_mappings[key] then
      emit('diagnostic', {code = 'glyph_ligature_components', source = 'luatex-ligature-components',
        font = n.font, char = n.char, text = text})
      glyph_mappings[key] = true
    end
  end
  if active and basis == 'text-symbol' then
    local key = 'text:' .. n.font .. ':' .. n.char .. ':' .. text
    if not glyph_mappings[key] then
      evidence.code = 'glyph_declared_text_symbol'
      emit('diagnostic', evidence)
      glyph_mappings[key] = true
    end
  end
  if active and basis == 'math-symbol' then
    local key = 'math:' .. n.font .. ':' .. n.char .. ':' .. text
    if not glyph_mappings[key] then
      evidence.code = 'glyph_declared_math_symbol'
      emit('diagnostic', evidence)
      glyph_mappings[key] = true
    end
  end
  if basis == 'symbol-font' then return nil, evidence end
  return text
end

local function literal_hyphen(n)
  local before, replacement = n.pre, n.replace
  if before and replacement and before.id == glyph and replacement.id == glyph
      and not before.next and not replacement.next and before.char == 45
      and replacement.char == 45 and before.font == replacement.font then
    node.set_attribute(before, candidate_attr, node.has_attribute(replacement, candidate_attr) or -2147483647)
    node.set_attribute(before, position_attr, node.has_attribute(replacement, position_attr) or -2147483647)
  end
end

local function collect(head, pieces, positions, id, nested, style_font)
  if nested and not style_font and M.text_styles then
    for n in node.traverse(head) do
      if n.id == glyph then
        style_font = M.text_styles.identity(n)
        if style_font then break end
      end
    end
  end
  for n in node.traverse(head) do
    if n.id == glyph then
      local own_style = nested and M.text_styles and M.text_styles.identity(n)
      if not layout_discretionary(n) and not node.has_attribute(n, candidate_attr)
          and not (M.math_bridge and M.math_bridge.is_math(n) and not node.has_attribute(n, decoration_attr))
          and not (nested and M.text_styles and not own_style and not style_font) then
        local text, evidence = character(n)
        if not text then
          -- A known symbol font's glyph also names its source and command (marvosym's \Tumbler).
          local diagnostic = {code = 'unmapped_glyph', font = n.font, char = n.char}
          if evidence then diagnostic.source, diagnostic.command, diagnostic.name = evidence.source, evidence.command, evidence.name end
          emit('diagnostic', diagnostic)
          text = '[unmapped]'
        end
        positions[#positions + 1] = text
        pieces[#pieces + 1] = text
        node.set_attribute(n, candidate_attr, id)
        node.set_attribute(n, position_attr, #positions)
        if own_style then
          style_font = own_style
        elseif style_font and not (M.math_bridge and M.math_bridge.is_math(n)) then
          -- A math glyph (a blackboard R in a running head) is drawn by a math font: it never
          -- takes the surrounding text face, whose linked font lacks it.
          node.set_attribute(n, decoration_style_attr, style_font)
        end
      elseif own_style then
        style_font = own_style
      end
    elseif n.id == glue and n.subtype == 13 and n.width > 0 then
      pieces[#pieces + 1] = ' '
    -- Infinitely stretchable glue (\hfil between a footer's text and its page number) parts words.
    elseif n.id == glue and n.subtype == 0 and n.stretch_order >= 1 and n.stretch > 0 and #pieces > 0 and pieces[#pieces] ~= ' ' then
      pieces[#pieces + 1] = ' '
    elseif n.id == disc then
      mark_discretionary(n.pre)
      mark_discretionary(n.post)
      collect(n.replace, pieces, positions, id, nested, style_font)
      literal_hyphen(n)
    elseif nested and (n.id == hlist or n.id == vlist or n.id == node.id('math') or n.id == node.id('mlist')) then
      collect(n.head, pieces, positions, id, nested, style_font)
    end
  end
end

local function first_glyph_context(head)
  for n in node.traverse(head) do
    if n.id == glyph then return node.has_attribute(n, context_attr) end
    if n.id == hlist or n.id == vlist then
      local found = first_glyph_context(n.head)
      if found then return found end
    end
  end
end

-- A line packed while the page is being output (\@outputpage: its head and foot) is page decoration,
-- including the boxes it copies: llncs and svjour set the running title and authors in boxes at
-- \maketitle (\titrun, \authrun) and \copy them into each head, so their glyphs carry the attributes
-- of the title page, not the page's (papers2/0339 "H. Wang et al." read as body text).
local function mark_page_decoration(head, value)
  for n in node.traverse(head) do
    if (n.id == glyph or n.id == node.id('rule')) and not node.has_attribute(n, decoration_attr) then
      node.set_attribute(n, decoration_attr, value)
    elseif n.id == hlist or n.id == vlist then
      mark_page_decoration(n.head, value)
    elseif n.id == disc then
      if n.pre then mark_page_decoration(n.pre, value) end
      if n.post then mark_page_decoration(n.post, value) end
      if n.replace then mark_page_decoration(n.replace, value) end
    end
  end
end

local function capture(head, group)
  if not active then return head end
  -- A QED mark box is one symbol (relatex-flow, decision 176): what is packed inside it is not read apart.
  if M.flow and M.flow.in_qed() then return head end
  local decoration = tex.getattribute(decoration_attr)
  if group ~= 'paragraph' and decoration and decoration > 0 then mark_page_decoration(head, decoration) end
  if M.math_bridge then M.math_bridge.capture_box(head) end
  candidate = candidate + 1
  local pieces, positions = {}, {}
  local entity
  for n in node.traverse(head) do
    if n.id == glyph and not node.has_attribute(n, candidate_attr) then
      entity = node.has_attribute(n, context_attr) or 0
      break
    end
  end
  -- A paragraph with no glyph of its own (a theorem head broken off by the display that opens
  -- the theorem: testmath's lemma) belongs where its glyphs were set, not to the display whose
  -- start breaks it.
  if group == 'paragraph' and not entity and context_id == named_contexts.equation then
    entity = first_glyph_context(head)
  end
  entity = entity or context_id
  collect(head, pieces, positions, candidate, decoration and decoration > 0)
  if M.flow then M.flow.capture(head, group, entity, contexts[entity]) end
  if #positions > 0 then
    if contexts[entity] == 'proof' then proof_content[entity] = true end
    emit('candidate', {candidate = candidate, entity = entity, role = contexts[entity],
      group = group or 'paragraph', text = table.concat(pieces), glyphs = #positions})
  end
  if group == 'paragraph' and pending_runin == entity then
    pending_runin = nil
    if context_id == entity then M.context('paragraph', section_id) end
  end
  return head
end

-- A paragraph typeset before the collector started (relatex-preamble.lua), read now in the open
-- context: its boxes innermost first, as box packing reads them, then the paragraph.
local function capture_boxes(head)
  for n in node.traverse(head) do
    node.set_attribute(n, context_attr, context_id)
    if n.id == hlist or n.id == vlist then
      capture_boxes(n.head)
      capture(n.head, 'box')
    end
  end
end

function M.capture_detached(head)
  if not active then return end
  capture_boxes(head)
  capture(head, 'paragraph')
end

-- RELATEX_LEAN: a build that wants only the SyncTeX (the glyph map and the math records) skips the
-- readings of paragraphs and boxes, and below of each page.
local LEAN = os.getenv('RELATEX_LEAN') ~= nil

-- A float box can hold material no box packing has read: a float-package style's rules
-- (\@fs@pre, \@fs@mid, \@fs@post) are set in a \vbox. Read once the box is complete, in the
-- float's own context; otherwise the output routine's column packing reads it in whatever
-- context is open then (third cohort 539: an algorithm's rules owned by a later figure).
function M.float_box(number)
  local box = tex.box[number]
  if LEAN or not box or not box.head then return end
  capture(box.head, 'box')
end

if not LEAN then
  luatexbase.add_to_callback('pre_linebreak_filter', function(head) return capture(head, 'paragraph') end, 'relatex.paragraph')
  luatexbase.add_to_callback('hpack_filter', capture, 'relatex.box')
end

-- beamer typesets covered material (\uncover, \onslide, \visible, \pause on a slide that does not
-- show it) between \pgfsys@begininvisible and \pgfsys@endinvisible: literals that move the
-- placement 2000 bp off the page and back, nesting. Their text is expanded only at shipout, so
-- relatex-beamer.sty writes a marker beside each (cover_mark), in the same place of the stream.
-- What the shipped stream sets between the markers is off the sheet, no content of the slide.
local whatsit_id, user_subtype = node.id('whatsit'), node.subtype('user_defined')
local cover_id = luatexbase.new_whatsit('relatex.cover')
-- The glyph map counts the same markers (relatex-glyph-map.lua literal_box).
M.cover_id = cover_id
local covered_depth = 0

function M.cover_mark(shift)
  local mark = node.new(whatsit_id, user_subtype)
  mark.user_id, mark.type, mark.value = cover_id, 100, shift
  node.write(mark)
end

local function invisibility(n)
  if n.id == whatsit_id and n.subtype == user_subtype and n.user_id == cover_id then return n.value end
  return 0
end

local function retained(head, runs, math_runs, flow_runs, parent, citation_runs, bibliography_runs)
for n in node.traverse(head) do
  local shift = invisibility(n)
  if shift ~= 0 then covered_depth = math.max(covered_depth + shift, 0) end
  if covered_depth > 0 and shift == 0 then
    -- Covered: off the page. A box is still read for the literal that ends the cover inside it.
    if n.id == hlist or n.id == vlist then
      if M.flow then M.flow.enter(n) end
      retained(n.head, runs, math_runs, flow_runs, n, citation_runs, bibliography_runs)
      if M.flow then M.flow.leave() end
    end
  elseif M.graphics and M.graphics.limits.link_space_glyph(n) then
      -- ocgx2's tiny link-end space: link styling, never text.
    elseif (n.id == glyph or n.id == node.id('rule')) and node.has_attribute(n, decoration_attr) then
      local record = {code = 'excluded_page_decoration', page = page,
        source = node.has_attribute(n, decoration_attr) == 2 and 'acmart-copyright' or 'latex-outputpage'}
      if n.id == glyph then
        record.char, record.text = n.char, character(n) or '[unmapped]'
        record.candidate, record.position = node.has_attribute(n, candidate_attr), node.has_attribute(n, position_attr)
        if M.text_styles then
          record.style = M.text_styles.identity(n) or node.has_attribute(n, decoration_style_attr)
        end
        -- A glyph of a math font has no text face; it says so, so a text glyph without one is an error.
        if not record.style and M.math_bridge and M.math_bridge.is_math(n) then record.math = true end
      else
        record.width, record.height, record.depth = n.width, n.height, n.depth
      end
      emit('diagnostic', record)
    else
      if M.graphics then M.graphics.limits.retained(n, page) end
      if M.flow then
        M.flow.paint(n)
        M.flow.audit_text_color(n, page)
        M.flow.audit_text_language(n, page)
        M.flow.audit_running_rule(n, parent, page)
        local found = M.flow.retained(n, flow_runs)
        -- A literal hyphen can use a discretionary's pre-list while retaining
        -- its original logical slot. Only unmapped layout hyphens are omitted.
        -- A painted rule read in a bibliography entry is part of it, as a glyph is.
        if M.citations and ((n.id == glyph and (found or not node.has_attribute(n, discretionary_attr))) or (n.id == node.id('rule') and found)) then
          M.citations.retained(n, citation_runs, bibliography_runs)
        end
        if not found and n.id == glyph and not node.has_attribute(n, discretionary_attr) then
          emit('diagnostic', {code = 'unclassified_flow_glyph', char = n.char, page = page})
        end
      end
      if M.flow and (M.flow.is_symbol_box(n) or M.flow.graphic_of(n)) then
        -- The guarded default QED is one semantic box, including running rules; so is a
        -- graphic without an inspectable source.
      elseif M.math_bridge and M.math_bridge.retained(n, math_runs) then
        -- Formula evidence is reconciled separately from prose glyphs.
      elseif n.id == glyph then
        local id = node.has_attribute(n, candidate_attr)
        if id then
          local position = node.has_attribute(n, position_attr)
          local last = runs[#runs]
          if last and last.candidate == id and last.last + 1 == position then
            last.last = position
          else
            runs[#runs + 1] = {candidate = id, first = position, last = position}
          end
        elseif not node.has_attribute(n, discretionary_attr) then
          emit('diagnostic', {code = 'unclassified_glyph', char = n.char, page = page})
        end
      elseif n.id == hlist or n.id == vlist then
        if M.flow then M.flow.enter(n) end
        retained(n.head, runs, math_runs, flow_runs, n, citation_runs, bibliography_runs)
        if M.flow then M.flow.leave() end
      elseif n.id == node.id('glue') and n.leader and require('relatex-leaders').single_copy(n, parent) then
        -- A box leader set once (acmart's affiliation comma) ships its box's text there, as the
        -- flow read it (relatex-leaders).
        retained(n.leader, runs, math_runs, flow_runs, parent, citation_runs, bibliography_runs)
      elseif n.id == disc then
        retained(n.replace, runs, math_runs, flow_runs, parent, citation_runs, bibliography_runs)
      end
    end
  end
end

local function observe_page(box)
  if M.graphics then M.graphics.limits.mark(box.head, page) end
  if M.external_equations then M.external_equations.shipout(box.head) end
  if M.tables then M.tables.capture(box, page) end
  local runs, math_runs, flow_runs = {}, {}, {}
  if M.flow then M.flow.reset_boxes() end
  local citation_runs, bibliography_runs = {}, {}
  covered_depth = 0
  retained(box.head, runs, math_runs, flow_runs, box, citation_runs, bibliography_runs)
  for _, run in ipairs(runs) do
    run.page = page
    emit('occurrence', run)
  end
  for _, run in ipairs(math_runs) do
    run.page = page
    emit('math_occurrence', run)
  end
  for _, run in ipairs(flow_runs) do
    run.page = page
    emit('flow_occurrence', run)
  end
  if M.citations then M.citations.shipout(page, citation_runs, bibliography_runs) end
end

-- textpos in absolute mode keeps a page's textblocks in \TP@holdbox and paints them at the
-- shipout/foreground or background hook, after this page was observed at shipout/before: their
-- text was captured but never shipped (slides/0016's eleven "Source: ..." attributions). The box
-- belongs to the page just counted; it is read, not changed (textpos still paints it).
function M.textblock(number)
  if not active or LEAN then return end
  local box = tex.box[number]
  if not box or not box.head then return end
  local ok, problem = pcall(function()
    local runs, math_runs, flow_runs = {}, {}, {}
    retained(box.head, runs, math_runs, flow_runs, box, {}, {})
    for _, run in ipairs(runs) do run.page = page; emit('occurrence', run) end
    for _, run in ipairs(math_runs) do run.page = page; emit('math_occurrence', run) end
    for _, run in ipairs(flow_runs) do run.page = page; emit('flow_occurrence', run) end
  end)
  if not ok then emit('diagnostic', {code = 'collector_error', page = page, message = tostring(problem):sub(1, 300)}) end
end

-- Observation must never stop the ordinary build: an error is recorded in the stream (the
-- reader then refuses the incomplete evidence) and the page ships unchanged.
function M.shipout(box, printed)
  if not active then return end
  page = page + 1
  if LEAN then
    -- display rows are formulas of their own: their math records are what the SyncTeX places
    if M.external_equations then pcall(M.external_equations.shipout, box.head) end
    return
  end
  -- The printed page number (\thepage as it ships): what \pageref prints for a label on this page.
  if type(printed) == 'string' then emit('diagnostic', {code = 'page_number', page = page, printed = printed}) end
  local ok, problem = pcall(observe_page, box)
  if ok and M.index then ok, problem = pcall(M.index.shipout, box.head, page, printed) end
  if not ok then emit('diagnostic', {code = 'collector_error', page = page, message = tostring(problem):sub(1, 300)}) end
end

function M.label(name, number, counter, anchor, environment, row)
  if active then
    local record = {name = name, number = number, entity = context_id}
    if M.flow then
      record.anchor, record.counter = anchor, counter
      -- A subfigure label (subcaption, subfig) names the subcaption being typeset: the open
      -- caption of its float type.
      local parent_counter = type(counter) == 'string' and counter:match('^sub(%a+)$')
      record.target_entity = named_contexts[counter] or (parent_counter and named_contexts[parent_counter]) or 0
      if context == 'theorem' and theorem_counters[context_id] == false then record.target_entity = 0 end
      record.formula = M.math_bridge.current_formula()
      if record.formula then
        record.math_environment = environment
        if environment == 'align' or environment == 'align*' or environment == 'gather' or environment == 'gather*' then
          record.math_row = row
        end
      end
    end
    if subequations_open and M.flow then
      record.target_entity = 0
      subequation_labels[#subequation_labels + 1] = record
    elseif not (M.external_equations and M.external_equations.defer(record, environment, row)) then
      emit('label', record)
    end
  end
end

function M.external_equation_label(record)
  emit('label', record)
end

function M.external_equation_diagnostic(record)
  record.source = 'native-display-alignment'
  emit('diagnostic', record)
end

function M.math(xml)
  if active then
    math_events = math_events + 1
    emit('math', {mathml = xml, entity = context_id, retention = 'unresolved', provenance = 'converted-hook'})
  end
end

function M.external_math(xml, formula, entity, source)
  assert(active and os.getenv('RELATEX_STRATEGY') == 'external', 'External math outside external collection')
  emit('math', {mathml = xml, entity = entity or context_id, retention = 'unresolved',
    provenance = 'luamml-mlist', formula = formula, source = source})
end

function M.external_math_layout(xml, style)
  emit('diagnostic', {code = 'external_math_layout_only', source = 'luatex-ordinary-mlist',
    mathml = xml, style = style, entity = context_id})
end

-- A display environment's source lines (decision 064); its formulas name the group.
function M.math_source_group(fields)
  fields.code, fields.source = 'math_source_group', 'latex-math-source'
  emit('diagnostic', fields)
end

-- A candidate math macro's meaning (decision 064).
function M.math_macro(fields)
  fields.code, fields.source = 'math_macro', 'latex-math-source'
  emit('diagnostic', fields)
end

function M.external_math_diagnostic(code, fields)
  assert(active and os.getenv('RELATEX_STRATEGY') == 'external', 'External diagnostic outside collection')
  fields.code, fields.source = code, 'native-math-boxes'
  emit('diagnostic', fields)
end

local function sidecar_math(block)
  local first = block:find('<math[%s>]')
  if not first then return end
  local position, depth = first, 0
  while true do
    local start, last, closing = block:find('<(/?)math[%s>]', position)
    if not start then return end
    depth = depth + (closing == '/' and -1 or 1)
    if depth == 0 then return block:sub(first, last) end
    position = last + 1
  end
end

function M.finish()
  if not active then return end
  -- An environment whose end was never observed must not cost the paper its PDF: record
  -- what stayed open; the reader reports it.
  if #context_stack > 0 then
    local open = {}
    for _, entry in ipairs(context_stack) do open[#open + 1] = entry.entered end
    emit('diagnostic', {code = 'unclosed_structural_context', contexts = open})
    context_stack = {}
  end
  -- latex-lab replaces LuaMML's converted hook in file-output mode.
  -- Read its completed output as uncorrelated math evidence, not retained math.
  local mathml_path = os.getenv('RELATEX_MATHML') or (tex.jobname .. '-luamml-mathml.html')
  local file = io.open(mathml_path, 'rb')
  if file then
    local bytes = file:read(16777217)
    file:close()
    assert(#bytes <= 16777216, 'relatex math sidecar limit')
    if math_events == 0 then
      for block in bytes:gmatch('<div>.-</div>') do
        local xml = sidecar_math(block)
        if xml then
          local record = {mathml = xml, entity = 0, retention = 'unresolved', provenance = 'latex-lab-file'}
          if M.math_bridge then record.formula = tonumber(block:match('<h2>\\mml%s+(%d+)%s*</h2>')) end
          emit('math', record)
        end
      end
    end
  end
  if M.external_equations then M.external_equations.flush() end
  if M.math_bridge then M.math_bridge.finish() end
  if M.graphics then M.graphics.finish() end
  if M.external_links then M.external_links.finish() end
  if M.tables then M.tables.finish() end
  if M.page_totals then M.page_totals.finish() end
  if M.citations then M.citations.finish() end
  if M.flow then M.flow.finish() end
  if M.notes then M.notes.finish() end
  emit('end', {pages = page, records = sequence + 1})
  active = false
  output:close()
end

return M
