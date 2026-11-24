package tui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tcalc/expr"
)

// Tables are entries like table(f(x)). The active one floats in the
// bottom-right corner of the graph, listing x and f(x) for TableRadius
// steps either side of the trace cursor, or of x = 0 when not tracing.

func (m Model) isTable(i int) bool {
	return i >= 0 && i < len(m.results) && m.results[i].Kind == expr.Table && !m.entries[i].hidden
}

// activeTable returns the table to show: the selected entry if it's a
// visible table, otherwise the last visible table, or -1 if there's none.
func (m Model) activeTable() int {
	if m.isTable(m.sel) {
		return m.sel
	}
	for i := len(m.results) - 1; i >= 0; i-- {
		if m.isTable(i) {
			return i
		}
	}
	return -1
}

// tableCenter is the x value of a table's middle row.
func (m Model) tableCenter() float64 {
	if m.tracing {
		return m.traceX
	}
	return 0
}

// renderTable draws table entry i with radius rows either side of the
// center row.
func (m Model) renderTable(i, radius int) string {
	r := m.results[i]
	x0 := m.tableCenter()
	n := 2*radius + 1

	xs, ys := make([]float64, n), make([]float64, n)
	yScale := 0.0
	for k := range n {
		xs[k] = snapZero(x0+float64(k-radius)*r.Step, r.Step)
		ys[k] = r.TableFn(xs[k])
		if finite(ys[k]) {
			yScale = math.Max(yScale, math.Abs(ys[k]))
		}
	}

	label := ansi.Truncate(r.Label, 18, "…")
	xt, yt := make([]string, n), make([]string, n)
	xw, yw := 1, ansi.StringWidth(label)
	for k := range n {
		xt[k] = expr.FormatPrec(xs[k], 8)
		yt[k] = expr.FormatPrec(snapZero(ys[k], yScale), 8)
		xw = max(xw, ansi.StringWidth(xt[k]))
		yw = max(yw, ansi.StringWidth(yt[k]))
	}

	sep := m.th.faint.Render("│")
	lines := []string{
		" " + m.th.muted.Bold(true).Render(padLeft("x", xw)) + " " + sep + " " +
			m.th.muted.Bold(true).Render(padLeft(label, yw)) + " ",
		m.th.faint.Render(strings.Repeat("─", xw+2) + "┼" + strings.Repeat("─", yw+2)),
	}
	for k := range n {
		row, lead := m.th.text, " "
		if k == radius {
			row, lead = m.th.accent.Bold(true), m.th.accent.Render("▸")
		}
		lines = append(lines, lead+row.Render(padLeft(xt[k], xw))+" "+sep+" "+row.Render(padLeft(yt[k], yw))+" ")
	}

	bs, ts := m.th.border, m.th.title
	if m.sel == i && m.focus == focusList {
		bs, ts = m.th.borderFocused, m.th.titleFocused
	}
	w := xw + yw + 5 + 2
	return box("table", "", lines, w, len(lines)+2, bs, ts)
}

// overlayTable floats the active table, if any, over the bottom-right
// corner of the plot area in the rendered graph box. Tables shrink to
// fewer rows on short terminals and are left out when even that won't fit.
func (m Model) overlayTable(graphBox string, f *frame, boxW int) string {
	ti := m.activeTable()
	if ti < 0 {
		return graphBox
	}
	// A table is its rows plus a header, a separator and two borders.
	radius := min(expr.TableRadius, (f.H-5)/2)
	if radius < 1 {
		return graphBox
	}
	tbl := m.renderTable(ti, radius)
	tw, th := lipgloss.Width(tbl), lipgloss.Height(tbl)
	if tw > f.W-2 {
		return graphBox
	}
	// Inside the box border, keep a column free on the right and sit just
	// above the x axis so its labels stay readable.
	x := boxW - 2 - tw
	y := 1 + f.H - th
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(graphBox),
		lipgloss.NewLayer(tbl).X(x).Y(y).Z(1),
	).Render()
}

func padLeft(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}
