package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// tableRows returns the plain text of the table window's value rows.
func tableRows(t *testing.T, m Model) []string {
	t.Helper()
	i := m.activeTable()
	if i < 0 {
		t.Fatalf("no active table")
	}
	lines := strings.Split(ansi.Strip(m.renderTable(i, 4)), "\n")
	return lines[3 : len(lines)-1] // skip border, header, separator
}

func TestTableAroundOrigin(t *testing.T) {
	m := sized(t, New(Options{Expressions: []string{"f(x) = x^2", "table(f(x))"}}), 100, 30)
	rows := tableRows(t, m)
	if len(rows) != 9 {
		t.Fatalf("got %d rows", len(rows))
	}
	for i, want := range [][2]string{{"-4", "16"}, {"0", "0"}, {"4", "16"}} {
		row := rows[[]int{0, 4, 8}[i]]
		f := strings.Fields(strings.NewReplacer("│", " ", "▸", " ").Replace(row))
		if len(f) != 2 || f[0] != want[0] || f[1] != want[1] {
			t.Errorf("row %q, want x=%s y=%s", row, want[0], want[1])
		}
	}
	if !strings.Contains(rows[4], "▸") {
		t.Errorf("center row should be marked: %q", rows[4])
	}
	if !strings.Contains(ansi.Strip(m.render()), "╭─ table") {
		t.Errorf("table window missing from the graph")
	}
}

func TestTableFollowsTrace(t *testing.T) {
	m := sized(t, New(Options{Expressions: []string{"x", "table(2x, 0.5)"}}), 100, 30)
	m = press(t, m, "tab", "t")
	m.traceX = 3
	rows := tableRows(t, m)
	for i, want := range map[int]string{0: "1", 4: "3", 8: "5"} {
		f := strings.Fields(strings.NewReplacer("│", " ", "▸", " ").Replace(rows[i]))
		if f[0] != want || f[1] != map[string]string{"1": "2", "3": "6", "5": "10"}[want] {
			t.Errorf("row %d = %q, want x=%s", i, rows[i], want)
		}
	}
}

func TestTableChoiceAndHiding(t *testing.T) {
	m := sized(t, New(Options{Expressions: []string{"table(x)", "table(2x)"}}), 100, 30)
	if got := m.activeTable(); got != 1 {
		t.Errorf("with no table selected, the last one shows; got %d", got)
	}
	m = press(t, m, "up", "up")
	if got := m.activeTable(); got != 0 {
		t.Errorf("the selected table shows; got %d", got)
	}
	m = press(t, m, "ctrl+t")
	if got := m.activeTable(); got != 1 {
		t.Errorf("hidden tables don't show; got %d", got)
	}
}

func TestTableShrinksOnShortTerminals(t *testing.T) {
	m := sized(t, New(Options{Expressions: []string{"table(x)"}}), 100, 14)
	out := ansi.Strip(m.render())
	if !strings.Contains(out, "▸ 0") {
		t.Fatalf("table missing")
	}
	if strings.Contains(out, "-4 │") || !strings.Contains(out, "-1 │") {
		t.Errorf("expected a smaller table:\n%s", out)
	}
}
