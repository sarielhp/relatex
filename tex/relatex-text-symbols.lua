-- This file is part of relatex (https://github.com/sarielhp/relatex).
-- SPDX-License-Identifier: LPPL-1.3c
-- It may be distributed and/or modified under the conditions of the LaTeX Project Public License, either
-- version 1.3c of this license or (at your option) any later version (https://www.latex-project.org/lppl.txt).
-- This work has the LPPL maintenance status "author-maintained"; its current maintainer is the relatex author.
-- Text symbols require both a live encoding declaration and an observed NFSS
-- font selection. A font name or a numeric slot alone establishes nothing.
local M = {}
-- The kernel's text symbols (t1enc.def, ts1enc.def, omsenc.def) with their Unicode values.
-- Each is bound only while the document's live declaration still names that slot.
local declarations = {
  T1 = {{'textquotedblleft',16,0x201C},{'textquotedblright',17,0x201D},
    {'textendash',21,0x2013},{'textemdash',22,0x2014},
    {'textquoteleft',96,0x2018},{'textquoteright',39,0x2019},
    {'quotesinglbase',13,0x201A},{'guilsinglleft',14,0x2039},{'guilsinglright',15,0x203A},
    {'quotedblbase',18,0x201E},{'guillemetleft',19,0xAB},{'guillemetright',20,0xBB},
    {'guillemotleft',19,0xAB},{'guillemotright',20,0xBB},{'i',25,0x131},{'j',26,0x237},
    {'textvisiblespace',32,0x2423},{'textunderscore',95,0x5F},{'textcompwordmark',23,''},{'L',138,0x141},{'NG',141,0x14A},
    {'IJ',156,0x132},{'dj',158,0x111},{'textsection',159,0xA7},{'l',170,0x142},{'ng',173,0x14B},
    {'ij',188,0x133},{'textexclamdown',189,0xA1},{'textquestiondown',190,0xBF},{'textsterling',191,0xA3},
    {'AE',198,0xC6},{'DH',208,0xD0},{'OE',215,0x152},{'O',216,0xD8},{'TH',222,0xDE},{'ae',230,0xE6},
    {'dh',240,0xF0},{'oe',247,0x153},{'o',248,0xF8},{'th',254,0xFE},{'ss',255,0xDF},
    -- \SS, the capital of \ss, is the two letters (fontinst composes it so), not ß.
    {'SS',223,'SS'}},
  TS1 = {{'textleftarrow',24,0x2190},{'textrightarrow',25,0x2192},{'textblank',32,0x2422},
    {'textdollar',36,0x24},{'textquotesingle',39,0x27},{'textfractionsolidus',47,0x2044},
    {'textlangle',60,0x2329},{'textminus',61,0x2212},{'textrangle',62,0x232A},{'textmho',77,0x2127},
    {'textbigcircle',79,0x25EF},{'textohm',87,0x2126},{'textlbrackdbl',91,0x27E6},{'textrbrackdbl',93,0x27E7},
    {'textuparrow',94,0x2191},{'textdownarrow',95,0x2193},{'textasciigrave',96,0x60},
    {'textmusicalnote',110,0x266A},{'textdagger',132,0x2020},{'textdaggerdbl',133,0x2021},
    {'textbardbl',134,0x2016},{'textperthousand',135,0x2030},{'textbullet',136,0x2022},
    {'textcelsius',137,0x2103},{'textflorin',140,0x192},{'textcolonmonetary',141,0x20A1},
    {'textwon',142,0x20A9},{'textnaira',143,0x20A6},{'textguarani',144,0x20B2},{'textpeso',145,0x20B1},
    {'textlira',146,0x20A4},{'textrecipe',147,0x211E},{'textinterrobang',148,0x203D},{'textdong',150,0x20AB},
    {'texttrademark',151,0x2122},{'textpertenthousand',152,0x2031},{'textpilcrow',153,0xB6},
    {'textbaht',154,0xE3F},{'textnumero',155,0x2116},{'textestimated',157,0x212E},
    {'textopenbullet',158,0x25E6},{'textservicemark',159,0x2120},{'textcent',162,0xA2},
    {'textsterling',163,0xA3},{'textcurrency',164,0xA4},{'textyen',165,0xA5},{'textbrokenbar',166,0xA6},
    {'textsection',167,0xA7},{'textasciidieresis',168,0xA8},{'textcopyright',169,0xA9},
    {'textordfeminine',170,0xAA},{'textlnot',172,0xAC},{'textcircledP',173,0x2117},
    {'textregistered',174,0xAE},{'textasciimacron',175,0xAF},{'textdegree',176,0xB0},{'textpm',177,0xB1},
    {'texttwosuperior',178,0xB2},{'textthreesuperior',179,0xB3},{'textasciiacute',180,0xB4},
    {'textmu',181,0xB5},{'textparagraph',182,0xB6},{'textperiodcentered',183,0xB7},
    {'textreferencemark',184,0x203B},{'textonesuperior',185,0xB9},{'textordmasculine',186,0xBA},
    {'textsurd',187,0x221A},{'textonequarter',188,0xBC},{'textonehalf',189,0xBD},
    -- ts1enc.def's other symbols with a true equivalent (decision 104; a virtual TS1 font has no glyph
    -- names to read them by): old-style figures are the figures, the double hyphen ⹀, the genealogical
    -- signs ⚮ ⚭ (and † for \textdied, doubtful), the spacing accents, the quills ⁅ ⁆, ⸘, ⁒, 🄯.
    {'textcapitalcompwordmark',23,''},{'textascendercompwordmark',31,''},{'textdblhyphen',45,0x2E40},{'textzerooldstyle',48,0x30},{'textoneoldstyle',49,0x31},
    {'texttwooldstyle',50,0x32},{'textthreeoldstyle',51,0x33},{'textfouroldstyle',52,0x34},
    {'textfiveoldstyle',53,0x35},{'textsixoldstyle',54,0x36},{'textsevenoldstyle',55,0x37},
    {'texteightoldstyle',56,0x38},{'textnineoldstyle',57,0x39},{'textdivorced',99,0x26AE},
    {'textdied',100,0x2020},{'textmarried',109,0x26AD},{'texttildelow',126,0x2F7},
    {'textdblhyphenchar',127,0x2E40},{'textasciibreve',128,0x2D8},{'textasciicaron',129,0x2C7},
    {'textacutedbl',130,0x2DD},{'textgravedbl',131,0x2F5},{'textdollaroldstyle',138,0x24},
    {'textcentoldstyle',139,0xA2},{'textinterrobangdown',149,0x2E18},{'textdiscount',156,0x2052},
    {'textlquill',160,0x2045},{'textrquill',161,0x2046},{'textcopyleft',171,0x1F12F},
    {'textthreequarters',190,0xBE},{'texteuro',191,0x20AC},{'texttimes',214,0xD7},{'textdiv',246,0xF7}},
  -- ot1enc.def's text symbols; its accents are \accent slots, read through M.accent. The quotation
  -- marks are the typed ` ' " (cmr and cmtt alike paint quoteleft and quoteright at 96 and 39; slots
  -- 34 and 92 hold a typewriter font's own " and \, see M.observe); the dashes are left out: a
  -- typewriter font sets { | at their slots.
  OT1 = {{'i',16,0x131},{'j',17,0x237},{'ss',25,0xDF},{'ae',26,0xE6},{'oe',27,0x153},{'o',28,0xF8},
    {'AE',29,0xC6},{'OE',30,0x152},{'O',31,0xD8},{'textquotedblright',34,0x201D},
    {'textquotedblleft',92,0x201C},{'textquoteleft',96,0x2018},{'textquoteright',39,0x2019}},
  OMS = {{'textperiodcentered',1,0xB7},{'textasteriskcentered',3,0x2217},{'textbigcircle',13,0x25EF},
    {'textbullet',15,0x2022},{'textbraceleft',102,0x7B},{'textbraceright',103,0x7D},{'textbar',106,0x7C},
    {'textbardbl',107,0x2016},{'textbackslash',110,0x5C},{'textsection',120,0xA7},{'textdagger',121,0x2020},
    {'textdaggerdbl',122,0x2021},{'textparagraph',123,0xB6}}
}
-- T1 accented letters (\DeclareTextComposite): accent, letter, slot, Unicode. LaTeX stores each
-- as the macro \T1\<accent>-<letter> whose body is the slot's character.
local composites = {
    {'u','A',128,0x102},{'k','A',129,0x104},{'\'','C',130,0x106},{'v','C',131,0x10C},{'v','D',132,0x10E},{'v','E',133,0x11A},
    {'k','E',134,0x118},{'u','G',135,0x11E},{'\'','L',136,0x139},{'v','L',137,0x13D},{'\'','N',139,0x143},{'v','N',140,0x147},
    {'H','O',142,0x150},{'\'','R',143,0x154},{'v','R',144,0x158},{'\'','S',145,0x15A},{'v','S',146,0x160},{'c','S',147,0x15E},
    {'v','T',148,0x164},{'c','T',149,0x162},{'H','U',150,0x170},{'r','U',151,0x16E},{'"','Y',152,0x178},{'\'','Z',153,0x179},
    {'v','Z',154,0x17D},{'.','Z',155,0x17B},{'.','I',157,0x130},{'u','a',160,0x103},{'k','a',161,0x105},{'\'','c',162,0x107},
    {'v','c',163,0x10D},{'v','d',164,0x10F},{'v','e',165,0x11B},{'k','e',166,0x119},{'u','g',167,0x11F},{'\'','l',168,0x13A},
    {'v','l',169,0x13E},{'\'','n',171,0x144},{'v','n',172,0x148},{'H','o',174,0x151},{'\'','r',175,0x155},{'v','r',176,0x159},
    {'\'','s',177,0x15B},{'v','s',178,0x161},{'c','s',179,0x15F},{'v','t',180,0x165},{'c','t',181,0x163},{'H','u',182,0x171},
    {'r','u',183,0x16F},{'"','y',184,0xFF},{'\'','z',185,0x17A},{'v','z',186,0x17E},{'.','z',187,0x17C},{'`','A',192,0xC0},
    {'\'','A',193,0xC1},{'^','A',194,0xC2},{'~','A',195,0xC3},{'"','A',196,0xC4},{'r','A',197,0xC5},{'c','C',199,0xC7},
    {'`','E',200,0xC8},{'\'','E',201,0xC9},{'^','E',202,0xCA},{'"','E',203,0xCB},{'`','I',204,0xCC},{'\'','I',205,0xCD},
    {'^','I',206,0xCE},{'"','I',207,0xCF},{'~','N',209,0xD1},{'`','O',210,0xD2},{'\'','O',211,0xD3},{'^','O',212,0xD4},
    {'~','O',213,0xD5},{'"','O',214,0xD6},{'`','U',217,0xD9},{'\'','U',218,0xDA},{'^','U',219,0xDB},{'"','U',220,0xDC},
    {'\'','Y',221,0xDD},{'`','a',224,0xE0},{'\'','a',225,0xE1},{'^','a',226,0xE2},{'~','a',227,0xE3},{'"','a',228,0xE4},
    {'r','a',229,0xE5},{'c','c',231,0xE7},{'`','e',232,0xE8},{'\'','e',233,0xE9},{'^','e',234,0xEA},{'"','e',235,0xEB},
    {'`','i',236,0xEC},{'`','i',236,0xEC},{'\'','i',237,0xED},{'\'','i',237,0xED},{'^','i',238,0xEE},{'^','i',238,0xEE},
    {'"','i',239,0xEF},{'"','i',239,0xEF},{'~','n',241,0xF1},{'`','o',242,0xF2},{'\'','o',243,0xF3},{'^','o',244,0xF4},
    {'~','o',245,0xF5},{'"','o',246,0xF6},{'`','u',249,0xF9},{'\'','u',250,0xFA},{'^','u',251,0xFB},{'"','u',252,0xFC},
    {'\'','y',253,0xFD}
}
-- \DeclareTextAccent slots (ot1enc.def, t1enc.def): the combining mark each sets with \accent.
local accents = {
  OT1 = {[18]=0x300,[19]=0x301,[20]=0x30C,[21]=0x306,[22]=0x304,[23]=0x30A,[94]=0x302,[95]=0x307,
  [125]=0x30B,[126]=0x303,[127]=0x308},
  T1 = {[0]=0x300,[1]=0x301,[2]=0x302,[3]=0x303,[4]=0x308,[5]=0x30B,[6]=0x30A,[7]=0x30C,[8]=0x306,
  [9]=0x304,[10]=0x307,[11]=0x327,[12]=0x328},
  -- ts1enc.def's capital accents and ties (\t is the double inverted breve).
  TS1 = {[0]=0x300,[1]=0x301,[2]=0x302,[3]=0x303,[4]=0x308,[5]=0x30B,[6]=0x30A,[7]=0x30C,[8]=0x306,
  [9]=0x304,[10]=0x307,[26]=0x361,[27]=0x361,[28]=0x361,[29]=0x361}
}
-- Slots an encoding fixes to a mark that paints no character of its own, without a command to bind
-- them by: OT1's suppress (the stroke \L and \l set over their letter; a typewriter font has its
-- visible space there) and T1's perthousandzero (\textperthousand's second zero). They read as nothing.
local MARKS = {OT1 = {[32] = true}, T1 = {[24] = true}}
-- A spacing accent a font maps itself, as the combining mark \accent makes of it.
local spacing = {[0xB4]=0x301,[0x60]=0x300,[0x5E]=0x302,[0x2C6]=0x302,[0x7E]=0x303,[0x2DC]=0x303,[0xA8]=0x308,
  [0xAF]=0x304,[0x2C9]=0x304,[0x2D8]=0x306,[0x2D9]=0x307,[0x2DA]=0x30A,[0x2DD]=0x30B,[0x2C7]=0x30C,
  [0xB8]=0x327,[0x2DB]=0x328}
-- pifont's Zapf Dingbats (\ding, \Pisymbol{pzd}: the NFSS font U/pzd, pzdr): each slot's glyph
-- (a1 ... a191 in pzdr.afm) as the Unicode character the Zapf Dingbats glyph list names for it.
local DINGBATS = {
  [33]=0x2701,[34]=0x2702,[35]=0x2703,[36]=0x2704,[37]=0x260E,[38]=0x2706,[39]=0x2707,[40]=0x2708,
  [41]=0x2709,[42]=0x261B,[43]=0x261E,[44]=0x270C,[45]=0x270D,[46]=0x270E,[47]=0x270F,[48]=0x2710,
  [49]=0x2711,[50]=0x2712,[51]=0x2713,[52]=0x2714,[53]=0x2715,[54]=0x2716,[55]=0x2717,[56]=0x2718,
  [57]=0x2719,[58]=0x271A,[59]=0x271B,[60]=0x271C,[61]=0x271D,[62]=0x271E,[63]=0x271F,[64]=0x2720,
  [65]=0x2721,[66]=0x2722,[67]=0x2723,[68]=0x2724,[69]=0x2725,[70]=0x2726,[71]=0x2727,[72]=0x2605,
  [73]=0x2729,[74]=0x272A,[75]=0x272B,[76]=0x272C,[77]=0x272D,[78]=0x272E,[79]=0x272F,[80]=0x2730,
  [81]=0x2731,[82]=0x2732,[83]=0x2733,[84]=0x2734,[85]=0x2735,[86]=0x2736,[87]=0x2737,[88]=0x2738,
  [89]=0x2739,[90]=0x273A,[91]=0x273B,[92]=0x273C,[93]=0x273D,[94]=0x273E,[95]=0x273F,[96]=0x2740,
  [97]=0x2741,[98]=0x2742,[99]=0x2743,[100]=0x2744,[101]=0x2745,[102]=0x2746,[103]=0x2747,[104]=0x2748,
  [105]=0x2749,[106]=0x274A,[107]=0x274B,[108]=0x25CF,[109]=0x274D,[110]=0x25A0,[111]=0x274F,[112]=0x2750,
  [113]=0x2751,[114]=0x2752,[115]=0x25B2,[116]=0x25BC,[117]=0x25C6,[118]=0x2756,[119]=0x25D7,[120]=0x2758,
  [121]=0x2759,[122]=0x275A,[123]=0x275B,[124]=0x275C,[125]=0x275D,[126]=0x275E,[161]=0x2761,[162]=0x2762,
  [163]=0x2763,[164]=0x2764,[165]=0x2765,[166]=0x2766,[167]=0x2767,[168]=0x2663,[169]=0x2666,[170]=0x2665,
  [171]=0x2660,[172]=0x2460,[173]=0x2461,[174]=0x2462,[175]=0x2463,[176]=0x2464,[177]=0x2465,[178]=0x2466,
  [179]=0x2467,[180]=0x2468,[181]=0x2469,[182]=0x2776,[183]=0x2777,[184]=0x2778,[185]=0x2779,[186]=0x277A,
  [187]=0x277B,[188]=0x277C,[189]=0x277D,[190]=0x277E,[191]=0x277F,[192]=0x2780,[193]=0x2781,[194]=0x2782,
  [195]=0x2783,[196]=0x2784,[197]=0x2785,[198]=0x2786,[199]=0x2787,[200]=0x2788,[201]=0x2789,[202]=0x278A,
  [203]=0x278B,[204]=0x278C,[205]=0x278D,[206]=0x278E,[207]=0x278F,[208]=0x2790,[209]=0x2791,[210]=0x2792,
  [211]=0x2793,[212]=0x2794,[213]=0x2192,[214]=0x2194,[215]=0x2195,[216]=0x2798,[217]=0x2799,[218]=0x279A,
  [219]=0x279B,[220]=0x279C,[221]=0x279D,[222]=0x279E,[223]=0x279F,[224]=0x27A0,[225]=0x27A1,[226]=0x27A2,
  [227]=0x27A3,[228]=0x27A4,[229]=0x27A5,[230]=0x27A6,[231]=0x27A7,[232]=0x27A8,[233]=0x27A9,[234]=0x27AA,
  [235]=0x27AB,[236]=0x27AC,[237]=0x27AD,[238]=0x27AE,[239]=0x27AF,[241]=0x27B1,[242]=0x27B2,[243]=0x27B3,
  [244]=0x27B4,[245]=0x27B5,[246]=0x27B6,[247]=0x27B7,[248]=0x27B8,[249]=0x27B9,[250]=0x27BA,[251]=0x27BB,
  [252]=0x27BC,[253]=0x27BD,[254]=0x27BE
}
-- bbding's glyphs are unnamed (enc-N): its slots by the command names in bbding.sty, mapped only
-- where a Unicode dingbat or shape is the same symbol (\CheckmarkBold '042 ✔, \XSolidBrush '045 ✘:
-- papers 0073; \CircleSolid '141, \EllipseSolid '143: book 0003). Mirrored or broken variants share
-- their symbol's character; hands, nibs, flowers and most arrows without a clear equivalent stay
-- unmapped.
local BBDING = {
  [0]=0x2703, [1]=0x2702, [2]=0x2701, [3]=0x2703, [4]=0x2702, [5]=0x2701, [6]=0x2704, [7]=0x2704,
  [8]=0x260E, [11]=0x2708, [12]=0x2709, [21]=0x270C, [22]=0x270D, [23]=0x270F, [25]=0x2710,
  [27]=0x270E, [31]=0x2712, [33]=0x2713, [34]=0x2714, [35]=0x2717, [36]=0x2718, [37]=0x2718,
  [38]=0x2719, [39]=0x271A, [40]=0x271C, [41]=0x271B, [42]=0x271D, [43]=0x271E, [44]=0x271F,
  [46]=0x2720, [48]=0x2721, [53]=0x2726, [54]=0x2727, [56]=0x2605, [57]=0x2606, [58]=0x272A,
  [59]=0x272B, [61]=0x272D, [62]=0x272E, [64]=0x2730, [65]=0x2731, [66]=0x2732, [71]=0x2736,
  [72]=0x2734, [74]=0x2739, [75]=0x273A, [92]=0x2748, [93]=0x2747, [95]=0x2746, [96]=0x2744,
  [97]=0x25CF, [99]=0x2B2C, [100]=0x274D, [102]=0x25A1, [103]=0x25A0, [104]=0x274F, [105]=0x2750,
  [107]=0x2751, [108]=0x2752, [110]=0x25B2, [111]=0x25BC, [112]=0x25C6, [114]=0x25D7, [115]=0x25D6
}
-- Symbol fonts without encoding declarations, bound by the selection of their NFSS family U/<family>:
-- pifont's Zapf Dingbats; marvosym (its glyphs by their names, relatex-glyph-names); bbding (above).
-- Their slots are no Unicode values: a glyph neither listed nor named for a character has no text.
local SYMBOL_FONTS = {
  pzd = {slots=DINGBATS, source='pifont-dingbat-font', command='ding'},
  mvs = {slots={}, source='marvosym-symbol-font', command='mvchr'},
  ding = {slots=BBDING, source='bbding-symbol-font', command='symbol'},
}
local glyph_names = require('relatex-glyph-names')
-- dingbats: font id -> its known symbol font; u_fonts: the ids selected in the NFSS encoding U.
local bindings, encodings, dingbats, u_fonts = {}, {}, {}, {}
-- typewriters: the OT1 font ids selected in a typewriter family.
local typewriters = {}
-- symbol_fonts: the ids selected as text in a symbol encoding (the math encodings OML, OMS, OMX and
-- the text companion TS1), whose slots are no character codes either.
local symbol_fonts = {}
local SYMBOL_ENCODINGS = {OML = true, OMS = true, OMX = true, TS1 = true}

-- The combining mark of a glyph set by \accent: by the slot of its font's live encoding, or
-- by the spacing accent the font maps it to (text, the glyph's own decoding).
function M.accent(n, text)
  local slots = accents[encodings[n.font]]
  local code = slots and slots[n.char]
  if not code and text and utf8.len(text) == 1 then code = spacing[utf8.codepoint(text)] end
  return code and utf8.char(code)
end

-- The selected NFSS family is the document's typewriter family, or a known monospaced one (cmtt,
-- Courier, TeX Gyre Cursor, DejaVuSansMono-TLF: their OT1 fonts keep " and \ at 34 and 92).
local function typewriter_family()
  local family = token.get_macro('f@family') or ''
  local default = token.get_macro('ttdefault')
  return family == default or family:find('tt') ~= nil or family:find('^pcr') ~= nil
    or family:find('[Mm]ono') ~= nil or family:find('^qcr') ~= nil
end

function M.observe()
  local encoding, id = token.get_macro('f@encoding'), font.current()
  -- A symbol font has no encoding declarations: the selection of U/<family> itself binds it.
  local symbols = encoding == 'U' and SYMBOL_FONTS[token.get_macro('f@family')]
  if symbols and id and id > 0 then dingbats[id] = symbols end
  if encoding == 'U' and id and id > 0 then u_fonts[id] = true end
  if SYMBOL_ENCODINGS[encoding] and id and id > 0 then symbol_fonts[id] = true end
  local entries = declarations[encoding]
  if not entries or not id or id <= 0 then return end
  if encodings[id] and encodings[id] ~= encoding then bindings[id] = false end
  encodings[id] = encoding
  if bindings[id] == false then return end
  local selected = bindings[id] or {}
  bindings[id] = selected
  local typewriter = encoding == 'OT1' and typewriter_family()
  if typewriter then typewriters[id] = true end
  for _, spec in ipairs(entries) do
    -- OT1 typewriter fonts keep the ASCII double quote and backslash in slots 34 and 92 (cmtt), unlike the
    -- roman fonts' quotation marks the encoding declares there: those slots stay unbound, their own characters.
    if not (typewriter and (spec[2] == 34 or spec[2] == 92)) then
      local command = token.create(encoding .. '\\' .. spec[1])
      local valid = command.cmdname == 'char_given' and command.mode == spec[2]
      local previous = selected[spec[2]]
      if not valid or previous == false or (previous and previous.unicode ~= spec[3]) then
        selected[spec[2]] = false
      else
        selected[spec[2]] = {encoding=encoding,command=spec[1],unicode=spec[3]}
      end
    end
  end
  if encoding == 'T1' then
    for _, spec in ipairs(composites) do
      local body = token.get_macro('\\T1\\' .. spec[1] .. '-' .. spec[2])
      local valid = body ~= nil and utf8.len(body) == 1 and utf8.codepoint(body) == spec[3]
      local previous = selected[spec[3]]
      if not valid or previous == false or (previous and previous.unicode ~= spec[4]) then
        selected[spec[3]] = false
      else
        selected[spec[3]] = {encoding=encoding,command=spec[1] .. spec[2],unicode=spec[4]}
      end
    end
  end
end

function M.decode(n)
  local symbols = dingbats[n.font]
  if symbols and symbols.slots[n.char] then
    local text = utf8.char(symbols.slots[n.char])
    return text, {source=symbols.source,encoding='U',command=symbols.command,font=n.font,char=n.char,text=text,
      binding='font-selection'}
  end
  if symbols or u_fonts[n.font] then
    -- A glyph of a U-encoded font whose Type 1 name stands for a character (marvosym's Football,
    -- wasysym's Euro).
    local name, tfm = glyph_names.name(n.font, n.char)
    local text = glyph_names.unicode(name, tfm)
    local source = symbols and symbols.source or 'type1-glyph-name'
    if text then
      return text, {source=source,encoding='U',command=symbols and symbols.command or nil,font=n.font,
        char=n.char,text=text,name=name,binding='glyph-name'}
    end
    -- A known symbol font's slot is no Unicode value: its other glyphs have no text, only evidence. So
    -- is a named glyph of any other U font whose name stands for no character (msam's circleS is no
    -- 's', wasy's phone no '\x07'): a symbol font's slot is no character code (decision 104).
    if symbols or name then
      return nil, 'unmapped-symbol', {source=source,encoding='U',command=symbols and symbols.command or nil,
        font=n.font,char=n.char,name=name}
    end
  end
  if bindings[n.font] == false then return nil, 'conflict' end
  local binding = bindings[n.font] and bindings[n.font][n.char]
  if binding == false then return nil, 'conflict' end
  if not binding then
    local marks = MARKS[encodings[n.font]]
    if marks and marks[n.char] and not typewriters[n.font] then
      return '', {source='latex-encoding-mark',encoding=encodings[n.font],font=n.font,char=n.char,text='',
        binding='font-selection'}
    end
    return
  end
  local text = type(binding.unicode) == 'string' and binding.unicode or utf8.char(binding.unicode)
  return text, {source='latex-textsymbol-font',encoding=binding.encoding,command=binding.command,
    font=n.font,char=n.char,text=text,binding='font-selection'}
end

-- Any other 8-bit font's glyph by its Type 1 name (decision 104): the glyph a slot paints is the one
-- the font's map line, encoding file, AFM or program names there (a virtual font's: its base
-- glyph's), so a slot no declaration binds reads as its glyph, not as the Latin-1 character of its
-- code (an OT1 font's \textendash at "{", its ¡ at "<", cmsy's ∪ at "[", T5's ư, IL2's ě, LGR's
-- Greek). Called after the declarations and the letter ligatures, before the slot. A virtual
-- small-caps font sets a capital for a small letter (or a font a base glyph of the same
-- letter): that letter reads as its slot. A glyph of a
-- font selected as text in a math encoding (OML, OMS, OMX) whose name stands for no character has
-- none, not its slot's (cmex's summationtext is no "P"), and so has an unnamed one (a virtual
-- TS1 font's empty \textborn is no "b"): the second value is then its name, or true.
-- A glyph the reviewed names say paints no character of its own (OT1's suppress, the stroke \L and
-- \l overlay on their letter; T1's compound word mark) has no text in any other font: an empty one.
local named_cache = {}
function M.named(n)
  if n.char > 255 then return nil end
  local cache = named_cache[n.font]
  if not cache then
    cache = {}
    named_cache[n.font] = cache
  end
  local entry = cache[n.char]
  if entry == nil then
    local name, tfm, virtual = glyph_names.name(n.font, n.char)
    local text = glyph_names.unicode(name, tfm)
    if text and virtual and n.char < 128 and text:lower() == utf8.char(n.char):lower() then text = utf8.char(n.char) end
    entry = {text or false, name, text == false}
    cache[n.char] = entry
  end
  if entry[1] then return entry[1] end
  if symbol_fonts[n.font] then return nil, entry[2] or true end
  if entry[3] then return '' end
  return nil
end

return M
