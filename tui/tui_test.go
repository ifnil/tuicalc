package tui

import (
	"math"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tcalc/expr"
)

func sized(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return mm.(Model)
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "alt+enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}
		case "shift+enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		case "ctrl+a", "ctrl+c", "ctrl+d", "ctrl+r", "ctrl+t", "ctrl+u", "ctrl+w", "ctrl+x":
			msg = tea.KeyPressMsg{Code: rune(k[5]), Mod: tea.ModCtrl}
		default:
			for _, r := range k {
				mm, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
				m = mm.(Model)
			}
			continue
		}
		mm, _ := m.Update(msg)
		m = mm.(Model)
	}
	return m
}

func TestRenderIsExactSize(t *testing.T) {
	m := New(Options{Expressions: []string{"sin(x)", "1/x", "a = 2", "2+2", "bad("}})
	for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 20}, {200, 60}} {
		m := sized(t, m, size[0], size[1])
		out := m.render()
		lines := strings.Split(out, "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: got %d lines", size[0], size[1], len(lines))
		}
		for i, l := range lines[:len(lines)-1] { // the key help line may be shorter
			if w := ansi.StringWidth(l); w != size[0] {
				t.Errorf("%dx%d: line %d is %d wide: %q", size[0], size[1], i, w, ansi.Strip(l))
				break
			}
		}
	}
}

func TestTinyTerminal(t *testing.T) {
	m := sized(t, New(Options{Expressions: []string{"x"}}), 20, 5)
	if !strings.Contains(m.render(), "too small") {
		t.Errorf("expected a too-small message")
	}
	m = press(t, m, "tab", "+", "t") // must not panic
	_ = m.render()
}

func TestEditing(t *testing.T) {
	m := sized(t, New(Options{}), 100, 30)
	m = press(t, m, "k = 3", "enter", "f(x) = kx", "enter", "f(2)")
	if len(m.entries) != 3 {
		t.Fatalf("got %d entries", len(m.entries))
	}
	if r := m.results[2]; r.Kind != expr.Value || r.Value != 6 {
		t.Errorf("f(2) = %+v", r)
	}

	// Backspacing an entry empty, then once more, deletes it.
	m = press(t, m, "backspace", "backspace", "backspace", "backspace", "backspace")
	if len(m.entries) != 2 || m.sel != 1 {
		t.Fatalf("after deleting: %d entries, sel %d", len(m.entries), m.sel)
	}

	m = press(t, m, "ctrl+t")
	if !m.entries[1].hidden {
		t.Errorf("ctrl+t should hide the entry")
	}
	m = press(t, m, "up", "ctrl+x") // deletes k = 3, selecting f
	if len(m.entries) != 1 || m.entries[0].text != "f(x) = kx" || m.sel != 0 {
		t.Errorf("after ctrl+x: %+v, sel %d", m.entries, m.sel)
	}
	if m.results[0].Err == nil {
		t.Errorf("f should now fail: k was deleted")
	}
}

func TestTraceAndFeatures(t *testing.T) {
	m := sized(t, New(Options{Expressions: []string{"sin(x)"}}), 100, 30)
	m = press(t, m, "tab", "t")
	if !m.tracing || m.traceIdx != 0 {
		t.Fatalf("trace not started")
	}
	m.traceX = 0
	m = press(t, m, "]")
	if m.traceNote != "maximum" || math.Abs(m.traceX-math.Pi/2) > 1e-6 {
		t.Errorf("got %s at %v", m.traceNote, m.traceX)
	}
	m = press(t, m, "]")
	if m.traceNote != "root" || math.Abs(m.traceX-math.Pi) > 1e-9 {
		t.Errorf("got %s at %v", m.traceNote, m.traceX)
	}
	m = press(t, m, "[", "[")
	if m.traceNote != "root" || math.Abs(m.traceX) > 1e-9 {
		t.Errorf("got %s at %v", m.traceNote, m.traceX)
	}
	m = press(t, m, "esc")
	if m.tracing {
		t.Errorf("esc should stop tracing")
	}
}

func TestNextFeatureSkipsPoles(t *testing.T) {
	x, kind, ok := nextFeature(math.Tan, 0.1, 0.01, 1, 1000)
	if !ok || kind != "root" || math.Abs(x-math.Pi) > 1e-9 {
		t.Errorf("tan: got %v %s %v", x, kind, ok)
	}
	inv := func(x float64) float64 { return 1 / (x * x) }
	if x, kind, ok := nextFeature(inv, -1, 0.01, 1, 200); ok {
		t.Errorf("1/x²: reported %s at %v", kind, x)
	}
}

func TestPlotterHandlesHostileFunctions(t *testing.T) {
	th := newTheme(true)
	f := newFrame(80, 30, viewport{sx: 0.1, sy: 0.1}, &th)
	fns := map[string]func(float64) float64{
		"nan":      func(float64) float64 { return math.NaN() },
		"inf":      func(float64) float64 { return math.Inf(1) },
		"huge":     func(x float64) float64 { return 1e300 * x },
		"sin(1/x)": func(x float64) float64 { return math.Sin(1 / x) },
		"sin(1e4x)": func(x float64) float64 {
			return math.Sin(1e4 * x)
		},
		"tan": math.Tan,
	}
	for name, fn := range fns {
		start := time.Now()
		var cs []curve
		cs = append(cs, curve{fn: fn, style: th.curve(0)})
		f := *f
		f.chart.Canvas.Clear()
		_ = f.render(&th, cs, true, nil)
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Errorf("%s took %v", name, d)
		}
	}
}

func TestClipLine(t *testing.T) {
	x0, y0, x1, y1, ok := clipLine(-10, 5, 20, 5, 0, 0, 10, 10)
	if !ok || x0 != 0 || x1 != 10 || y0 != 5 || y1 != 5 {
		t.Errorf("got %v %v %v %v %v", x0, y0, x1, y1, ok)
	}
	if _, _, _, _, ok := clipLine(-10, -5, 20, -5, 0, 0, 10, 10); ok {
		t.Errorf("line outside the box should be rejected")
	}
}

func TestResetViewIsSquare(t *testing.T) {
	m := sized(t, New(Options{}), 100, 30)
	f := m.frame()
	x0, x1 := f.xRange()
	if math.Abs(x0+10) > 1e-9 || math.Abs(x1-10) > 1e-9 {
		t.Errorf("x range %v..%v", x0, x1)
	}
	if m.vp.sx != m.vp.sy {
		t.Errorf("scales differ: %v %v", m.vp.sx, m.vp.sy)
	}
}

func TestInsertAbove(t *testing.T) {
	m := sized(t, New(Options{}), 100, 30)
	// Write a function first, then define its variable above it.
	m = press(t, m, "f(x) = kx", "alt+enter", "k = 3")
	if len(m.entries) != 2 || m.sel != 0 {
		t.Fatalf("got %d entries, sel %d", len(m.entries), m.sel)
	}
	if m.entries[0].text != "k = 3" || m.entries[1].text != "f(x) = kx" {
		t.Fatalf("wrong order: %+v", m.entries)
	}
	if r := m.results[1]; r.Err != nil || r.Fn(2) != 6 {
		t.Errorf("f should see k: %+v", r)
	}

	// shift+enter does the same; an empty entry doesn't spawn another.
	m = press(t, m, "shift+enter", "shift+enter")
	if len(m.entries) != 3 || m.sel != 0 || m.entries[0].text != "" {
		t.Errorf("shift+enter: %+v, sel %d", m.entries, m.sel)
	}
}

func TestInsertKeepsTrace(t *testing.T) {
	m := sized(t, New(Options{Expressions: []string{"x", "x^2"}}), 100, 30)
	m = press(t, m, "up", "tab", "t") // trace x^2
	if !m.tracing || m.traceIdx != 1 {
		t.Fatalf("trace idx %d", m.traceIdx)
	}
	m = press(t, m, "tab", "up", "alt+enter") // tracing continues while editing
	if !m.tracing || m.traceIdx != 2 || m.entries[m.traceIdx].text != "x^2" {
		t.Errorf("trace moved to %d (%q)", m.traceIdx, m.entries[m.traceIdx].text)
	}
}
