package tui

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Vim emulation for the expression list. Each entry is treated as a line:
// motions and operators work within the selected entry, while j/k, dd, yy,
// p, o and friends work on whole entries.

type vimState struct {
	insert   bool     // insert mode (list pane only)
	pending  []string // keys of the command being typed
	reg      register
	lastFind vimCmd // last f/F/t/T, for ; and ,
	undo     []snapshot
	redo     []snapshot
	base     *snapshot // state before the current insert session
}

type register struct {
	text     string
	lines    []string
	linewise bool
}

type snapshot struct {
	entries []entry
	sel     int
	pos     int
}

const maxUndo = 200

// ---------------------------------------------------------------------------
// Parsing

type vimCmd struct {
	count int    // 0 when no count was typed
	op    string // "", "d", "c" or "y"
	key   string // command, motion or text object ("iw", "a(")
	arg   rune   // character argument of f/F/t/T/r
	next  string // second key of ctrl+w
}

type parseState int

const (
	parsePending parseState = iota
	parseDone
	parseInvalid
)

func setOf(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

var (
	charMotions = setOf("h", "l", "left", "right", "backspace", " ", "0", "^", "$", "home", "end",
		"w", "W", "b", "B", "e", "E", ";", ",")
	findMotions = setOf("f", "F", "t", "T")
	lineMotions = setOf("j", "k", "up", "down", "enter", "+", "-", "G")
	textObjects = setOf("w", "W", "(", ")", "b", "[", "]", "{", "}", "|")
	simpleCmds  = setOf("x", "X", "delete", "s", "S", "C", "D", "Y", "p", "P", "i", "a", "I", "A",
		"o", "O", "~", "u", "ctrl+r", "ctrl+a", "ctrl+x", "ctrl+t", ":", "esc")
	reverseFind = map[string]string{"f": "F", "F": "f", "t": "T", "T": "t"}
)

// vimToken turns a key press into the token the parser works with: the
// character itself for printable keys, otherwise the key's name.
func vimToken(msg tea.KeyPressMsg) string {
	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		if len([]rune(msg.Text)) == 1 {
			return msg.Text
		}
	}
	return msg.String()
}

// parseVim parses the keys typed so far as [count] [operator [count]]
// (motion | text object) or a simple command.
func parseVim(toks []string) (vimCmd, parseState) {
	var c vimCmd
	n1, i := readCount(toks, 0)
	if i == len(toks) {
		return c, parsePending
	}
	if op := toks[i]; op == "d" || op == "c" || op == "y" {
		c.op = op
		n2, j := readCount(toks, i+1)
		c.count = mulCount(n1, n2)
		if j == len(toks) {
			return c, parsePending
		}
		switch t := toks[j]; t {
		case op:
			c.key = op + op
			return c, parseDone
		case "i", "a":
			if j+1 == len(toks) {
				return c, parsePending
			}
			if !textObjects[toks[j+1]] {
				return c, parseInvalid
			}
			c.key = t + toks[j+1]
			return c, parseDone
		}
		return parseMotion(c, toks[j:])
	}

	c.count = n1
	rest := toks[i:]
	switch t := rest[0]; {
	case t == "g" || charMotions[t] || findMotions[t] || lineMotions[t]:
		return parseMotion(c, rest)
	case t == "Z":
		if len(rest) < 2 {
			return c, parsePending
		}
		if rest[1] == "Z" || rest[1] == "Q" {
			c.key = "Z" + rest[1]
			return c, parseDone
		}
	case t == "r":
		if len(rest) < 2 {
			return c, parsePending
		}
		if r, ok := single(rest[1]); ok {
			c.key, c.arg = "r", r
			return c, parseDone
		}
	case t == "ctrl+w":
		if len(rest) < 2 {
			return c, parsePending
		}
		c.key, c.next = t, rest[1]
		return c, parseDone
	case simpleCmds[t]:
		c.key = t
		return c, parseDone
	}
	return c, parseInvalid
}

func parseMotion(c vimCmd, toks []string) (vimCmd, parseState) {
	switch t := toks[0]; {
	case t == "g":
		if len(toks) < 2 {
			return c, parsePending
		}
		if toks[1] == "g" {
			c.key = "gg"
			return c, parseDone
		}
	case findMotions[t]:
		if len(toks) < 2 {
			return c, parsePending
		}
		if r, ok := single(toks[1]); ok {
			c.key, c.arg = t, r
			return c, parseDone
		}
	case charMotions[t] || lineMotions[t]:
		c.key = t
		return c, parseDone
	}
	return c, parseInvalid
}

func readCount(toks []string, i int) (int, int) {
	n := 0
	for ; i < len(toks); i++ {
		t := toks[i]
		if len(t) != 1 || t[0] < '0' || t[0] > '9' || (t == "0" && n == 0) {
			break
		}
		n = min(n*10+int(t[0]-'0'), 99999)
	}
	return n, i
}

func mulCount(a, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	}
	return min(a*b, 99999)
}

func single(t string) (rune, bool) {
	if rs := []rune(t); len(rs) == 1 {
		return rs[0], true
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// Motions and text objects within one line

// class returns vim's character class at i: 0 blank, 1 word, 2 punctuation.
// With big (WORD motions) everything non-blank is one class. A '.' between
// digits counts as a word character so that numbers like 1.5 are one word.
func class(rs []rune, i int, big bool) int {
	r := rs[i]
	switch {
	case unicode.IsSpace(r):
		return 0
	case big:
		return 1
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	case r == '.' && ((i > 0 && unicode.IsDigit(rs[i-1])) || (i+1 < len(rs) && unicode.IsDigit(rs[i+1]))):
		return 1
	}
	return 2
}

func nextWordStart(rs []rune, pos int, big bool) int {
	n := len(rs)
	if pos >= n {
		return n
	}
	i := pos
	if c := class(rs, i, big); c != 0 {
		for i < n && class(rs, i, big) == c {
			i++
		}
	}
	for i < n && class(rs, i, big) == 0 {
		i++
	}
	return i
}

func prevWordStart(rs []rune, pos int, big bool) int {
	i := min(pos, len(rs)) - 1
	for i >= 0 && class(rs, i, big) == 0 {
		i--
	}
	if i < 0 {
		return 0
	}
	c := class(rs, i, big)
	for i > 0 && class(rs, i-1, big) == c {
		i--
	}
	return i
}

func wordEnd(rs []rune, pos int, big bool) int {
	n := len(rs)
	i := pos + 1
	for i < n && class(rs, i, big) == 0 {
		i++
	}
	if i >= n {
		return max(n-1, 0)
	}
	c := class(rs, i, big)
	for i+1 < n && class(rs, i+1, big) == c {
		i++
	}
	return i
}

func firstNonBlank(rs []rune) int {
	for i, r := range rs {
		if !unicode.IsSpace(r) {
			return i
		}
	}
	return 0
}

// motion returns where a motion moves the cursor within the line, whether
// it's inclusive (an operator then also covers the target character), and
// whether it succeeded.
func (v *vimState) motion(rs []rune, pos int, c vimCmd) (int, bool, bool) {
	n := max(c.count, 1)
	switch c.key {
	case "h", "left", "backspace":
		return max(pos-n, 0), false, pos > 0
	case "l", "right", " ":
		return min(pos+n, len(rs)), false, pos < len(rs)
	case "0", "home":
		return 0, false, true
	case "^":
		return firstNonBlank(rs), false, true
	case "$", "end":
		return max(len(rs)-1, 0), true, len(rs) > 0
	case "w", "W":
		for range n {
			pos = nextWordStart(rs, pos, c.key == "W")
		}
		return pos, false, true
	case "b", "B":
		for range n {
			pos = prevWordStart(rs, pos, c.key == "B")
		}
		return pos, false, true
	case "e", "E":
		for range n {
			pos = wordEnd(rs, pos, c.key == "E")
		}
		return pos, true, true
	case "f", "F", "t", "T":
		v.lastFind = vimCmd{key: c.key, arg: c.arg}
		return findChar(rs, pos, c.key, c.arg, n, false)
	case ";", ",":
		k := v.lastFind.key
		if k == "" {
			return pos, false, false
		}
		if c.key == "," {
			k = reverseFind[k]
		}
		return findChar(rs, pos, k, v.lastFind.arg, n, true)
	}
	return pos, false, false
}

func findChar(rs []rune, pos int, kind string, r rune, n int, repeat bool) (int, bool, bool) {
	switch kind {
	case "f", "t":
		start := pos + 1
		if kind == "t" && repeat {
			start++ // don't get stuck right before the last match
		}
		for i := start; i < len(rs); i++ {
			if rs[i] == r {
				if n--; n == 0 {
					if kind == "t" {
						return i - 1, true, true
					}
					return i, true, true
				}
			}
		}
	case "F", "T":
		start := pos - 1
		if kind == "T" && repeat {
			start--
		}
		for i := start; i >= 0; i-- {
			if rs[i] == r {
				if n--; n == 0 {
					if kind == "T" {
						return i + 1, false, true
					}
					return i, false, true
				}
			}
		}
	}
	return pos, false, false
}

// textObject returns the range [from, to) of a text object such as iw,
// a( or i| around pos.
func textObject(rs []rune, pos int, obj string) (int, int, bool) {
	if len(rs) == 0 {
		return 0, 0, false
	}
	pos = min(pos, len(rs)-1)
	inner := obj[0] == 'i'
	switch o := obj[1:]; o {
	case "w", "W":
		big := o == "W"
		c := class(rs, pos, big)
		from, to := pos, pos+1
		for from > 0 && class(rs, from-1, big) == c {
			from--
		}
		for to < len(rs) && class(rs, to, big) == c {
			to++
		}
		if !inner {
			switch {
			case c == 0 && to < len(rs): // blanks plus the following word
				c2 := class(rs, to, big)
				for to < len(rs) && class(rs, to, big) == c2 {
					to++
				}
			case to < len(rs) && class(rs, to, big) == 0: // trailing blanks
				for to < len(rs) && class(rs, to, big) == 0 {
					to++
				}
			default: // otherwise leading blanks
				for from > 0 && class(rs, from-1, big) == 0 {
					from--
				}
			}
		}
		return from, to, true
	case "(", ")", "b":
		return bracketObject(rs, pos, '(', ')', inner)
	case "[", "]":
		return bracketObject(rs, pos, '[', ']', inner)
	case "{", "}":
		return bracketObject(rs, pos, '{', '}', inner)
	case "|":
		return barObject(rs, pos, inner)
	}
	return 0, 0, false
}

func bracketObject(rs []rune, pos int, open, close rune, inner bool) (int, int, bool) {
	start, depth := -1, 0
	for i := pos; i >= 0 && start < 0; i-- {
		switch {
		case rs[i] == close && i != pos:
			depth++
		case rs[i] == open:
			if depth == 0 {
				start = i
			}
			depth--
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	depth = 0
	for j := start + 1; j < len(rs); j++ {
		switch rs[j] {
		case open:
			depth++
		case close:
			if depth == 0 {
				if inner {
					return start + 1, j, true
				}
				return start, j + 1, true
			}
			depth--
		}
	}
	return 0, 0, false
}

// barObject selects inside an |absolute value|. Bars are paired from the
// left, which is right for everything but nested bars.
func barObject(rs []rune, pos int, inner bool) (int, int, bool) {
	var bars []int
	for i, r := range rs {
		if r == '|' {
			bars = append(bars, i)
		}
	}
	for k := 0; k+1 < len(bars); k += 2 {
		if a, b := bars[k], bars[k+1]; a <= pos && pos <= b {
			if inner {
				return a + 1, b, true
			}
			return a, b + 1, true
		}
	}
	return 0, 0, false
}

// incrementNumber adds delta to the number under or after the cursor, like
// vim's ctrl+a. Digits after a decimal point are a number of their own, so
// with the cursor on the 5 of 1.05, ctrl+a gives 1.06. A '-' counts as a
// sign only where it can't be subtraction.
func incrementNumber(rs []rune, pos int, delta int) ([]rune, int, bool) {
	isDigit := func(r rune) bool { return r >= '0' && r <= '9' }
	j := min(pos, len(rs))
	if j < len(rs) && isDigit(rs[j]) {
		for j > 0 && isDigit(rs[j-1]) {
			j--
		}
	} else {
		for j < len(rs) && !isDigit(rs[j]) {
			j++
		}
		if j == len(rs) {
			return rs, pos, false
		}
	}
	start, end := j, j
	for end < len(rs) && isDigit(rs[end]) {
		end++
	}
	digits := string(rs[start:end])
	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return rs, pos, false
	}
	if start > 0 && rs[start-1] == '-' && isSignContext(rs, start-1) {
		v = -v
		start--
	}
	v += int64(delta)

	s := strconv.FormatInt(v, 10)
	if len(digits) > 1 && digits[0] == '0' && v >= 0 && len(s) < len(digits) {
		s = strings.Repeat("0", len(digits)-len(s)) + s // keep 1.05 → 1.06
	}
	out := slices.Concat(rs[:start], []rune(s), rs[end:])
	return out, start + len([]rune(s)) - 1, true
}

func isSignContext(rs []rune, minus int) bool {
	for i := minus - 1; i >= 0; i-- {
		if !unicode.IsSpace(rs[i]) {
			return strings.ContainsRune("=(,+-*/^%|", rs[i])
		}
	}
	return true
}

func toggleCase(r rune) rune {
	if unicode.IsUpper(r) {
		return unicode.ToLower(r)
	}
	return unicode.ToUpper(r)
}

// ---------------------------------------------------------------------------
// Model integration

func (m *Model) vimNormal() bool {
	return m.vimOn && m.focus == focusList && !m.vim.insert
}

// applyInputMode updates the text input's cursor and placeholder for the
// current mode: a steady block in normal mode, blinking while inserting.
func (m *Model) applyInputMode() {
	s := textinput.DefaultStyles(m.dark)
	m.input.Placeholder = "type an expression, e.g. sin(x)"
	if m.vimOn && !m.vim.insert {
		s.Cursor.Blink = false
		m.input.Placeholder = "press i to start typing"
	}
	m.input.SetStyles(s)
}

// clampNormalCursor keeps the cursor on a character, as in vim's normal
// mode, rather than after the end of the line.
func (m *Model) clampNormalCursor() {
	if !m.vimNormal() {
		return
	}
	if n := len([]rune(m.input.Value())); n > 0 && m.input.Position() >= n {
		m.input.SetCursor(n - 1)
	}
}

func (m *Model) enterInsert(pos int) tea.Cmd {
	m.vim.insert = true
	m.input.SetCursor(pos)
	m.applyInputMode()
	return m.input.Focus()
}

func (m *Model) leaveInsert() {
	m.vim.insert = false
	if p := m.input.Position(); p > 0 {
		m.input.SetCursor(p - 1)
	}
	if m.vim.base != nil {
		m.commitUndo(*m.vim.base)
		m.vim.base = nil
	}
	m.applyInputMode()
	m.clampNormalCursor()
}

func (m *Model) setLine(rs []rune, pos int) {
	m.input.SetValue(string(rs))
	m.input.SetCursor(pos)
	m.syncInput()
}

func (m *Model) selectEntryCol(i, col int) {
	m.selectEntry(i)
	m.input.SetCursor(col)
}

func (m Model) vimListKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	m.vim.pending = append(m.vim.pending, vimToken(msg))
	c, st := parseVim(m.vim.pending)
	switch st {
	case parsePending:
		return m, nil
	case parseInvalid:
		m.vim.pending = nil
		return m, nil
	}
	m.vim.pending = nil

	before := m.snapshot()
	cmd := m.vimExec(c)
	switch {
	case c.key == "u" || c.key == "ctrl+r":
	case m.vim.insert:
		// The whole change, including what's typed, is one undo step.
		if m.vim.base == nil {
			m.vim.base = &before
		}
	default:
		m.commitUndo(before)
	}
	m.clampNormalCursor()
	return m, cmd
}

func (m *Model) vimExec(c vimCmd) tea.Cmd {
	n := max(c.count, 1)
	rs := []rune(m.input.Value())
	pos := m.input.Position()

	// Shorthands for an operator and a motion.
	switch c.key {
	case "x", "delete":
		c.op, c.key = "d", "l"
	case "X":
		c.op, c.key = "d", "h"
	case "D":
		c.op, c.key = "d", "$"
	case "C":
		c.op, c.key = "c", "$"
	case "s":
		c.op, c.key = "c", "l"
	case "S":
		c.op, c.key = "c", "cc"
	case "Y":
		c.op, c.key = "y", "yy"
	}
	if c.op != "" {
		return m.vimOperate(c, rs, pos)
	}

	switch c.key {
	case "j", "down", "enter", "+":
		m.selectEntryCol(m.sel+n, pos)
	case "k", "up", "-":
		m.selectEntryCol(m.sel-n, pos)
	case "gg", "G":
		m.selectEntry(m.lineTarget(c))
		m.input.SetCursor(firstNonBlank([]rune(m.input.Value())))
	case "i":
		return m.enterInsert(pos)
	case "a":
		return m.enterInsert(min(pos+1, len(rs)))
	case "I":
		return m.enterInsert(firstNonBlank(rs))
	case "A":
		return m.enterInsert(len(rs))
	case "o", "O":
		at := m.sel + 1
		if c.key == "O" {
			at = m.sel
		}
		m.insertEntry(at)
		return m.enterInsert(0)
	case "p", "P":
		m.paste(c.key == "P", n)
	case "r":
		if len(rs) > 0 && pos+n <= len(rs) {
			for i := pos; i < pos+n; i++ {
				rs[i] = c.arg
			}
			m.setLine(rs, pos+n-1)
		}
	case "~":
		end := min(pos+n, len(rs))
		for i := pos; i < end; i++ {
			rs[i] = toggleCase(rs[i])
		}
		m.setLine(rs, end)
	case "u":
		for range n {
			if !m.undo() {
				break
			}
		}
	case "ctrl+r":
		for range n {
			if !m.redo() {
				break
			}
		}
	case "ctrl+a", "ctrl+x":
		d := n
		if c.key == "ctrl+x" {
			d = -n
		}
		if out, p, ok := incrementNumber(rs, pos, d); ok {
			m.setLine(out, p)
		}
	case "ctrl+t":
		m.entries[m.sel].hidden = !m.entries[m.sel].hidden
		m.validateTrace()
	case "ZZ", "ZQ":
		return tea.Quit
	case ":":
		return m.openCmdline()
	case "ctrl+w":
		return m.vimWindow(c.next)
	case "esc":
	default:
		if target, _, ok := m.vim.motion(rs, pos, c); ok {
			m.input.SetCursor(target)
		}
	}
	return nil
}

// lineTarget is the entry gg or G goes to: the count-th, or the first/last.
func (m *Model) lineTarget(c vimCmd) int {
	switch {
	case c.count > 0:
		return min(c.count, len(m.entries)) - 1
	case c.key == "gg":
		return 0
	}
	return len(m.entries) - 1
}

// lineRange returns the entries a linewise operator covers.
func (m *Model) lineRange(c vimCmd) (int, int, bool) {
	n := max(c.count, 1)
	last := len(m.entries) - 1
	switch c.key {
	case "dd", "cc", "yy":
		return m.sel, min(m.sel+n-1, last), true
	case "j", "down", "enter", "+":
		return m.sel, min(m.sel+n, last), m.sel < last
	case "k", "up", "-":
		return max(m.sel-n, 0), m.sel, m.sel > 0
	case "gg", "G":
		t := m.lineTarget(c)
		return min(m.sel, t), max(m.sel, t), true
	}
	return 0, 0, false
}

func (m *Model) vimOperate(c vimCmd, rs []rune, pos int) tea.Cmd {
	if from, to, ok := m.lineRange(c); ok {
		return m.operateLines(c.op, from, to)
	}

	var from, to int
	switch {
	case len(c.key) == 2 && (c.key[0] == 'i' || c.key[0] == 'a'):
		f, t, ok := textObject(rs, pos, c.key)
		if !ok {
			return nil
		}
		from, to = f, t
	case c.op == "c" && (c.key == "w" || c.key == "W") && pos < len(rs) && !unicode.IsSpace(rs[pos]):
		// Like vim, cw on a word changes to the end of the word only.
		big := c.key == "W"
		end, cl := pos, class(rs, pos, big)
		for end+1 < len(rs) && class(rs, end+1, big) == cl {
			end++
		}
		for range max(c.count, 1) - 1 {
			end = wordEnd(rs, end, big)
		}
		from, to = pos, end+1
	default:
		target, inclusive, ok := m.vim.motion(rs, pos, c)
		if !ok {
			return nil
		}
		from, to = min(pos, target), max(pos, target)
		if inclusive {
			to = min(to+1, len(rs))
		}
	}

	if from < to {
		m.vim.reg = register{text: string(rs[from:to])}
	}
	switch c.op {
	case "d":
		m.setLine(slices.Concat(rs[:from], rs[to:]), from)
	case "c":
		m.setLine(slices.Concat(rs[:from], rs[to:]), from)
		return m.enterInsert(from)
	case "y":
		m.input.SetCursor(from)
	}
	return nil
}

func (m *Model) operateLines(op string, from, to int) tea.Cmd {
	lines := make([]string, 0, to-from+1)
	for _, e := range m.entries[from : to+1] {
		lines = append(lines, e.text)
	}
	m.vim.reg = register{lines: lines, linewise: true}
	switch op {
	case "y":
		pos := m.input.Position()
		m.selectEntryCol(from, pos)
		if len(lines) > 1 {
			m.flash = strconv.Itoa(len(lines)) + " entries yanked"
		}
	case "d":
		m.removeEntries(from, to+1)
		m.selectEntry(min(from, len(m.entries)-1))
		m.input.SetCursor(firstNonBlank([]rune(m.input.Value())))
	case "c":
		m.entries[from].text = ""
		m.removeEntries(from+1, to+1)
		m.selectEntry(from)
		return m.enterInsert(0)
	}
	return nil
}

func (m *Model) paste(before bool, n int) {
	r := m.vim.reg
	if r.linewise {
		var texts []string
		for range n {
			texts = append(texts, r.lines...)
		}
		at := m.sel + 1
		if before {
			at = m.sel
		}
		m.insertEntries(at, texts)
		m.selectEntry(at)
		m.input.SetCursor(firstNonBlank([]rune(m.input.Value())))
		return
	}
	if r.text == "" {
		return
	}
	rs := []rune(m.input.Value())
	at := m.input.Position()
	if !before && len(rs) > 0 {
		at = min(at+1, len(rs))
	}
	ins := []rune(strings.Repeat(r.text, n))
	m.setLine(slices.Concat(rs[:at], ins, rs[at:]), at+len(ins)-1)
}

func (m *Model) vimWindow(next string) tea.Cmd {
	switch next {
	case "h", "left", "ctrl+h":
		return m.setFocus(focusList)
	case "l", "right", "ctrl+l":
		return m.setFocus(focusGraph)
	case "w", "ctrl+w", "p", "ctrl+p":
		return m.setFocus(1 - m.focus)
	}
	return nil
}

// vimGraphKey handles vim keys in the graph pane. It reports whether it
// consumed the key; anything else goes to the normal graph bindings.
func (m Model) vimGraphKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	tok := vimToken(msg)
	if len(m.vim.pending) > 0 || tok == "ctrl+w" || tok == "Z" {
		m.vim.pending = append(m.vim.pending, tok)
		c, st := parseVim(m.vim.pending)
		if st == parsePending {
			return m, nil, true
		}
		m.vim.pending = nil
		if st == parseDone {
			switch c.key {
			case "ctrl+w":
				return m, m.vimWindow(c.next), true
			case "ZZ", "ZQ":
				return m, tea.Quit, true
			}
		}
		return m, nil, true
	}

	switch tok {
	case ":":
		return m, m.openCmdline(), true
	case "i":
		cmd := m.setFocus(focusList)
		return m, tea.Batch(cmd, m.enterInsert(m.input.Position())), true
	case "esc":
		if !m.tracing {
			return m, m.setFocus(focusList), true
		}
	case "ctrl+d", "ctrl+u":
		half := 0.5 * float64(m.frame().dotsH()) * m.vp.sy
		if tok == "ctrl+d" {
			half = -half
		}
		m.vp.cy += half
		m.vp.clamp()
		return m, nil, true
	case "w", "b":
		if m.tracing {
			dir := 1
			if tok == "b" {
				dir = -1
			}
			m.jumpFeature(m.frame(), dir)
			return m, nil, true
		}
	}
	return m, nil, false
}

// vimCtrlC mirrors vim: ctrl+c leaves insert mode or cancels a pending
// command, and never quits.
func (m Model) vimCtrlC() (Model, tea.Cmd) {
	switch {
	case m.focus == focusList && m.vim.insert:
		m.leaveInsert()
	case len(m.vim.pending) > 0:
		m.vim.pending = nil
	default:
		m.flash = "Type :q and press Enter to quit"
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Undo

func (m *Model) snapshot() snapshot {
	return snapshot{entries: slices.Clone(m.entries), sel: m.sel, pos: m.input.Position()}
}

func (m *Model) commitUndo(before snapshot) {
	if slices.Equal(before.entries, m.entries) {
		return
	}
	m.vim.undo = append(m.vim.undo, before)
	if len(m.vim.undo) > maxUndo {
		m.vim.undo = m.vim.undo[1:]
	}
	m.vim.redo = nil
}

func (m *Model) undo() bool {
	if len(m.vim.undo) == 0 {
		m.flash = "Already at oldest change"
		return false
	}
	s := m.vim.undo[len(m.vim.undo)-1]
	m.vim.undo = m.vim.undo[:len(m.vim.undo)-1]
	m.vim.redo = append(m.vim.redo, m.snapshot())
	m.restore(s)
	return true
}

func (m *Model) redo() bool {
	if len(m.vim.redo) == 0 {
		m.flash = "Already at newest change"
		return false
	}
	s := m.vim.redo[len(m.vim.redo)-1]
	m.vim.redo = m.vim.redo[:len(m.vim.redo)-1]
	m.vim.undo = append(m.vim.undo, m.snapshot())
	m.restore(s)
	return true
}

func (m *Model) restore(s snapshot) {
	m.entries = slices.Clone(s.entries)
	m.compile()
	m.selectEntry(s.sel)
	m.input.SetCursor(s.pos)
}

// ---------------------------------------------------------------------------
// Command line

func (m *Model) openCmdline() tea.Cmd {
	m.cmdActive = true
	m.cmd.SetValue("")
	m.cmd.SetWidth(max(1, m.width-3))
	return m.cmd.Focus()
}

func (m Model) cmdKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.cmdActive = false
		return m, nil
	case "enter":
		m.cmdActive = false
		return m.runEx(m.cmd.Value())
	case "backspace":
		if m.cmd.Value() == "" {
			m.cmdActive = false
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.cmd, cmd = m.cmd.Update(msg)
	return m, cmd
}

func (m Model) runEx(s string) (Model, tea.Cmd) {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	switch {
	case s == "":
	case slices.Contains([]string{"q", "q!", "qa", "qa!", "qall", "quit", "quit!",
		"wq", "wq!", "wqa", "x", "xa", "xit", "exit"}, s):
		return m, tea.Quit
	case s == "w" || s == "wa":
		m.flash = "nothing to write: expressions live only in this session"
	case s == "h" || s == "help":
		m.showHelp, m.helpScroll = true, 0
	case isDigits(s):
		n, _ := strconv.Atoi(s)
		cmd := m.setFocus(focusList)
		m.selectEntry(max(0, min(n, len(m.entries))-1))
		m.input.SetCursor(firstNonBlank([]rune(m.input.Value())))
		return m, cmd
	case fields[0] == "set" && len(fields) == 2:
		switch fields[1] {
		case "deg", "degrees":
			m.degrees = true
			m.compile()
		case "rad", "radians":
			m.degrees = false
			m.compile()
		case "grid":
			m.grid = true
		case "nogrid":
			m.grid = false
		case "grid!", "invgrid":
			m.grid = !m.grid
		default:
			m.flash, m.flashErr = "E518: Unknown option: "+fields[1], true
		}
	default:
		m.flash, m.flashErr = "E492: Not an editor command: "+s, true
	}
	return m, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
