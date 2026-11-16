package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func vimModel(t *testing.T, exprs ...string) Model {
	t.Helper()
	return sized(t, New(Options{Expressions: exprs, Vim: true}), 100, 30)
}

func texts(m Model) []string {
	var out []string
	for _, e := range m.entries {
		out = append(out, e.text)
	}
	return out
}

func wantTexts(t *testing.T, m Model, want ...string) {
	t.Helper()
	if got := texts(m); !slices.Equal(got, want) {
		t.Fatalf("entries = %q, want %q", got, want)
	}
}

// keysCmd presses keys and returns the last command produced.
func keysCmd(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyPressMsg
		if k == "enter" {
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		} else {
			r := []rune(k)[0]
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		var mm tea.Model
		mm, cmd = m.Update(msg)
		m = mm.(Model)
	}
	return m, cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestParseVim(t *testing.T) {
	cases := []struct {
		keys  string
		state parseState
		want  vimCmd
	}{
		{"w", parseDone, vimCmd{key: "w"}},
		{"3dw", parseDone, vimCmd{count: 3, op: "d", key: "w"}},
		{"2d3w", parseDone, vimCmd{count: 6, op: "d", key: "w"}},
		{"dd", parseDone, vimCmd{op: "d", key: "dd"}},
		{"ci(", parseDone, vimCmd{op: "c", key: "i("}},
		{"daw", parseDone, vimCmd{op: "d", key: "aw"}},
		{"f(", parseDone, vimCmd{key: "f", arg: '('}},
		{"dt)", parseDone, vimCmd{op: "d", key: "t", arg: ')'}},
		{"gg", parseDone, vimCmd{key: "gg"}},
		{"10j", parseDone, vimCmd{count: 10, key: "j"}},
		{"0", parseDone, vimCmd{key: "0"}},
		{"ra", parseDone, vimCmd{key: "r", arg: 'a'}},
		{"ZZ", parseDone, vimCmd{key: "ZZ"}},
		{"d", parsePending, vimCmd{}},
		{"ci", parsePending, vimCmd{}},
		{"g", parsePending, vimCmd{}},
		{"2", parsePending, vimCmd{}},
		{"dq", parseInvalid, vimCmd{}},
		{"ciz", parseInvalid, vimCmd{}},
		{"q", parseInvalid, vimCmd{}},
	}
	for _, c := range cases {
		var toks []string
		for _, r := range c.keys {
			toks = append(toks, string(r))
		}
		got, st := parseVim(toks)
		if st != c.state || (st == parseDone && got != c.want) {
			t.Errorf("%q: got %+v (state %d), want %+v (state %d)", c.keys, got, st, c.want, c.state)
		}
	}
}

func TestMotions(t *testing.T) {
	rs := []rune("sin(2x) + 1.5x")
	var v vimState
	step := func(pos int, key string, arg rune) int {
		p, _, ok := v.motion(rs, pos, vimCmd{key: key, arg: arg})
		if !ok {
			t.Fatalf("%s from %d failed", key, pos)
		}
		return p
	}
	// w stops at word and punctuation boundaries; 1.5x is one word
	pos, want := 0, []int{3, 4, 6, 8, 10, 14}
	for _, w := range want {
		if pos = step(pos, "w", 0); pos != w {
			t.Fatalf("w: got %d, want %d", pos, w)
		}
	}
	if p := step(14, "b", 0); p != 10 {
		t.Errorf("b: got %d", p)
	}
	if p := step(0, "e", 0); p != 2 {
		t.Errorf("e: got %d", p)
	}
	if p := step(0, "W", 0); p != 8 {
		t.Errorf("W: got %d", p)
	}
	if p := step(0, "f", ')'); p != 6 {
		t.Errorf("f): got %d", p)
	}
	if p := step(0, "t", ')'); p != 5 {
		t.Errorf("t): got %d", p)
	}
	if p := step(0, ";", 0); p != 5 { // repeats t)
		t.Errorf(";: got %d", p)
	}
	if p := step(13, "F", '('); p != 3 {
		t.Errorf("F(: got %d", p)
	}
	if p := step(0, "$", 0); p != 13 {
		t.Errorf("$: got %d", p)
	}
}

func TestTextObjects(t *testing.T) {
	cases := []struct {
		line string
		pos  int
		obj  string
		want string
	}{
		{"sin(2x+1)", 5, "i(", "2x+1"},
		{"sin(2x+1)", 5, "a(", "(2x+1)"},
		{"f(g(x), y)", 4, "i(", "x"},
		{"f(g(x), y)", 7, "ib", "g(x), y"},
		{"f(g(x), y)", 3, "i(", "x"}, // on an opening bracket
		{"2|x-1| + 3", 3, "i|", "x-1"},
		{"a = 12.5 + b", 5, "iw", "12.5"},
		{"a = 12.5 + b", 5, "aw", "12.5 "},
		{"sin(x)", 1, "iW", "sin(x)"},
	}
	for _, c := range cases {
		rs := []rune(c.line)
		from, to, ok := textObject(rs, c.pos, c.obj)
		if !ok || string(rs[from:to]) != c.want {
			t.Errorf("%s at %d in %q: got %q (ok %v), want %q", c.obj, c.pos, c.line, string(rs[from:to]), ok, c.want)
		}
	}
	if _, _, ok := textObject([]rune("x + 1"), 0, "i("); ok {
		t.Errorf("i( outside brackets should fail")
	}
}

func TestIncrementNumber(t *testing.T) {
	cases := []struct {
		line  string
		pos   int
		delta int
		want  string
	}{
		{"a = 3", 0, 1, "a = 4"},
		{"a = 1.05", 7, 1, "a = 1.06"},
		{"a = 1.05", 4, 1, "a = 2.05"},
		{"a = 0", 0, -1, "a = -1"},
		{"a = -1", 0, 1, "a = 0"},
		{"x-1", 0, 1, "x-2"}, // subtraction, not a sign
		{"f(x) = 2x^3", 0, 5, "f(x) = 7x^3"},
	}
	for _, c := range cases {
		out, _, ok := incrementNumber([]rune(c.line), c.pos, c.delta)
		if !ok || string(out) != c.want {
			t.Errorf("%q%+d: got %q", c.line, c.delta, string(out))
		}
	}
	if _, _, ok := incrementNumber([]rune("sin(x)"), 0, 1); ok {
		t.Errorf("no number: should fail")
	}
}

func TestVimEditing(t *testing.T) {
	m := vimModel(t)
	if m.vim.insert || !strings.Contains(m.renderStatus(), "NORMAL") {
		t.Fatalf("should start in normal mode")
	}
	m = press(t, m, "i", "sin(2x)", "esc")
	wantTexts(t, m, "sin(2x)")
	if m.vim.insert || m.input.Position() != 6 {
		t.Fatalf("esc: insert %v, pos %d", m.vim.insert, m.input.Position())
	}

	m = press(t, m, "o", "a = 3", "esc")
	wantTexts(t, m, "sin(2x)", "a = 3")

	m = press(t, m, "k", "0", "f2", "ci(", "a x", "esc")
	wantTexts(t, m, "sin(a x)", "a = 3")
	if r := m.results[0]; r.Err != nil || r.Fn == nil {
		t.Fatalf("sin(a x) should plot: %+v", r)
	}

	m = press(t, m, "u")
	wantTexts(t, m, "sin(2x)", "a = 3")
	m = press(t, m, "ctrl+r")
	wantTexts(t, m, "sin(a x)", "a = 3")

	m = press(t, m, "yyp")
	wantTexts(t, m, "sin(a x)", "sin(a x)", "a = 3")
	m = press(t, m, "dd")
	wantTexts(t, m, "sin(a x)", "a = 3")

	m = press(t, m, "j", "ctrl+a", "ctrl+a")
	wantTexts(t, m, "sin(a x)", "a = 5")
	if m.results[1].Value != 5 {
		t.Errorf("a = %v", m.results[1].Value)
	}
	m = press(t, m, "3", "ctrl+x")
	wantTexts(t, m, "sin(a x)", "a = 2")

	m = press(t, m, "gg", "dw")
	wantTexts(t, m, "(a x)", "a = 2")
	m = press(t, m, "u", "$", "x")
	wantTexts(t, m, "sin(a x", "a = 2")
	if p := m.input.Position(); p != 6 {
		t.Errorf("x at end should leave the cursor on the last char, got %d", p)
	}

	// An insert session, including the entry it created, undoes at once.
	m = press(t, m, "O", "b = 1", "enter", "c = 2", "esc")
	wantTexts(t, m, "b = 1", "c = 2", "sin(a x", "a = 2")
	m = press(t, m, "u")
	wantTexts(t, m, "sin(a x", "a = 2")

	m = press(t, m, "S", "cos(x)", "esc", "2G", "cc", "y = 1", "esc")
	wantTexts(t, m, "cos(x)", "y = 1")
}

func TestVimModesAndPanes(t *testing.T) {
	m := vimModel(t, "x", "x^2")

	m = press(t, m, "i", "ctrl+c")
	if m.vim.insert {
		t.Errorf("ctrl+c should leave insert mode")
	}
	mm, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = mm.(Model)
	if isQuit(cmd) || !strings.Contains(m.flash, ":q") {
		t.Errorf("ctrl+c in normal mode should hint at :q, flash %q", m.flash)
	}

	m = press(t, m, "ctrl+w", "l")
	if m.focus != focusGraph {
		t.Fatalf("ctrl+w l should focus the graph")
	}
	m = press(t, m, "ctrl+w", "h")
	if m.focus != focusList || m.vim.insert {
		t.Fatalf("ctrl+w h should focus the list in normal mode")
	}
	m = press(t, m, "tab", "i")
	if m.focus != focusList || !m.vim.insert {
		t.Fatalf("i in the graph should start inserting")
	}
	m = press(t, m, "esc", "tab")
	if m.focus != focusGraph || m.vim.insert {
		t.Fatalf("tab should leave insert mode")
	}

	m = press(t, m, ":", "1", "enter")
	if m.focus != focusList || m.sel != 0 {
		t.Errorf(":1 should select entry 1, sel %d", m.sel)
	}
	m = press(t, m, ":", "nope", "enter")
	if !m.flashErr || !strings.Contains(m.flash, "E492") {
		t.Errorf("unknown command: flash %q", m.flash)
	}
	m = press(t, m, ":", "set deg", "enter")
	if !m.degrees {
		t.Errorf(":set deg didn't switch to degrees")
	}
	if _, cmd := keysCmd(t, m, ":", "q", "enter"); !isQuit(cmd) {
		t.Errorf(":q should quit")
	}
	if _, cmd := keysCmd(t, m, "Z", "Z"); !isQuit(cmd) {
		t.Errorf("ZZ should quit")
	}
}

func TestVimOffKeepsPlainEditing(t *testing.T) {
	m := sized(t, New(Options{}), 100, 30)
	m = press(t, m, "dd", "esc")
	if m.entries[0].text != "dd" || m.focus != focusGraph {
		t.Errorf("without vim, typing should insert text: %q", m.entries[0].text)
	}
}
