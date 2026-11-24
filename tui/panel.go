package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tcalc/expr"
)

type listLine struct {
	text  string
	entry int
}

// listLines renders the expression list, one or two lines per entry.
func (m Model) listLines(width int) []listLine {
	var out []listLine
	textW := max(1, width-3)
	for i, e := range m.entries {
		r := m.results[i]
		sel := i == m.sel
		blank := strings.TrimSpace(e.text) == ""

		bar := " "
		if sel {
			if m.focus == focusList {
				bar = m.th.selBar.Render("▌")
			} else {
				bar = m.th.selBarDim.Render("▌")
			}
		}

		swatch := " "
		switch {
		case r.Plottable() && e.hidden:
			swatch = m.th.curve(e.color).Render("○")
		case r.Plottable():
			swatch = m.th.curve(e.color).Render("●")
		case r.Kind == expr.Table && e.hidden:
			swatch = m.th.faint.Render("≡")
		case r.Kind == expr.Table:
			swatch = m.th.accent.Render("≡")
		case r.Kind == expr.Invalid && !blank:
			swatch = m.th.errText.Render("!")
		}

		var body string
		switch {
		case sel && m.focus == focusList:
			body = m.input.View()
		case blank:
		case e.hidden:
			body = m.th.faint.Render(ansi.Truncate(e.text, textW, "…"))
		default:
			body = ansi.Truncate(e.text, textW, "…")
		}
		out = append(out, listLine{bar + swatch + " " + body, i})

		switch {
		case r.Kind == expr.Invalid && !blank:
			msg := ansi.Truncate(r.Err.Error(), textW, "…")
			out = append(out, listLine{bar + "  " + m.th.errText.Render(msg), i})
		case r.Kind == expr.Value || (r.Kind == expr.Variable && !r.Literal):
			val := ansi.Truncate("= "+expr.FormatNumber(r.Value), width-3, "…")
			pad := max(1, width-2-ansi.StringWidth(val))
			out = append(out, listLine{bar + strings.Repeat(" ", pad) + m.th.muted.Render(val), i})
		}
	}
	return out
}

func (m Model) renderList(r rect) string {
	lines := m.listLines(r.w - 2)
	h := r.h - 2
	scroll := max(0, min(m.scroll, len(lines)-h))
	end := min(len(lines), scroll+h)

	body := make([]string, 0, h)
	for _, l := range lines[scroll:end] {
		body = append(body, l.text)
	}

	var more string
	switch {
	case scroll > 0 && end < len(lines):
		more = "↑↓"
	case scroll > 0:
		more = "↑"
	case end < len(lines):
		more = "↓"
	}

	bs, ts := m.th.border, m.th.title
	if m.focus == focusList {
		bs, ts = m.th.borderFocused, m.th.titleFocused
	}
	if more != "" {
		more = m.th.muted.Render(more)
	}
	return box("Expressions", more, body, r.w, r.h, bs, ts)
}

func (m Model) renderHelpPage(r rect) string {
	lines := m.helpLines(r.w)
	h := r.h - 2
	note := "esc to close"
	if len(lines) > h {
		note = "↑↓ scroll · " + note
	}
	scroll := max(0, min(m.helpScroll, len(lines)-h))
	return box("Help", m.th.muted.Render(note), lines[scroll:], r.w, r.h, m.th.borderFocused, m.th.titleFocused)
}

func (m Model) helpLines(boxWidth int) []string {
	width := boxWidth - 4
	head := m.th.titleFocused
	ex := func(code, desc string) string {
		return "  " + m.th.accent.Render(padRight(code, 16)) + m.th.muted.Render(desc)
	}

	lines := []string{
		head.Render("Entries"),
		ex("sin(x)", "plot a function of x"),
		ex("y = 2x + 1", "plot, explicit form"),
		ex("f(x) = x^2 - 1", "define a function (also plotted)"),
		ex("a = 3", "define a variable used by other entries"),
		ex("x = 2", "vertical line"),
		ex("2^10 / 3", "evaluate a number"),
		ex("table(f(x))", "table of values around the trace cursor, or x = 0"),
		ex("table(x^2, 0.5)", "same, with a step of 0.5 (default 1)"),
		"",
		head.Render("Syntax"),
		m.th.muted.Render("  + - * / ^   % mod   ! factorial   |x| abs"),
		m.th.muted.Render("  implicit products: 2x  3(x+1)  xsin(x)  2πx  sin x"),
		m.th.muted.Render("  e^2x is e^(2x), but e^2 x and x^2e^x are e²·x and x²·eˣ"),
		"",
		head.Render("Functions"),
	}
	lines = append(lines, wrapWords(expr.BuiltinNames(), width-2, "  ", m.th.muted)...)
	lines = append(lines,
		"",
		head.Render("Constants"),
		m.th.muted.Render("  pi π  tau τ  e  phi φ  inf ∞"),
		"",
		head.Render("Keys"),
	)
	for _, l := range strings.Split(m.keyTable(width-2), "\n") {
		lines = append(lines, "  "+l)
	}

	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return lines
}

// keyTable lays the key binding groups out in as many columns as fit.
func (m Model) keyTable(width int) string {
	var rows, row []string
	rowW := 0
	for _, g := range m.keys.fullHelp(m.vimOn) {
		keyW := 0
		for _, b := range g.bindings {
			keyW = max(keyW, ansi.StringWidth(b.Help().Key))
		}
		block := []string{m.th.muted.Bold(true).Render(g.title)}
		for _, b := range g.bindings {
			h := b.Help()
			block = append(block, m.th.accent.Render(padRight(h.Key, keyW+1))+m.th.muted.Render(h.Desc))
		}
		col := lipgloss.NewStyle().PaddingRight(3).Render(strings.Join(block, "\n"))
		w := lipgloss.Width(col)
		if rowW > 0 && rowW+w > width {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
			row, rowW = nil, 0
		}
		row = append(row, col)
		rowW += w
	}
	if len(row) > 0 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
	}
	return strings.Join(rows, "\n\n")
}

func wrapWords(words []string, width int, indent string, st lipgloss.Style) []string {
	var lines []string
	cur := ""
	for _, w := range words {
		if cur != "" && len(cur)+1+len(w) > width {
			lines = append(lines, indent+st.Render(cur))
			cur = ""
		}
		if cur != "" {
			cur += " "
		}
		cur += w
	}
	if cur != "" {
		lines = append(lines, indent+st.Render(cur))
	}
	return lines
}

func padRight(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s + " "
}

// box draws a rounded border of size w×h around body, with a title on the
// left of the top border and an optional note on its right.
func box(title, note string, body []string, w, h int, bs, ts lipgloss.Style) string {
	if w < 4 || h < 2 {
		return ""
	}
	inner := w - 2
	seg := func(s string) (string, int) {
		if s == "" {
			return "", 0
		}
		return " " + s + " ", ansi.StringWidth(s) + 2
	}
	t, tw := seg(ts.Render(title))
	n, nw := seg(note)
	if w-4-tw-nw < 0 {
		n, nw = "", 0
	}
	if w-4-tw-nw < 0 {
		t, tw = seg(ts.Render(ansi.Truncate(title, max(0, w-6), "…")))
	}
	fill := max(0, w-4-tw-nw)

	var sb strings.Builder
	sb.WriteString(bs.Render("╭─") + t + bs.Render(strings.Repeat("─", fill)) + n + bs.Render("─╮"))
	side := bs.Render("│")
	for i := range h - 2 {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		if lw := ansi.StringWidth(line); lw > inner {
			line = ansi.Truncate(line, inner, "")
		} else {
			line += strings.Repeat(" ", inner-lw)
		}
		sb.WriteString("\n" + side + line + side)
	}
	sb.WriteString("\n" + bs.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return sb.String()
}

func joinColumns(a, b string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, a, b)
}

// spread places left and right on one line of the given width.
func spread(left, right string, width int) string {
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if lw+rw+2 > width {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", width-lw-rw-1) + right + " "
}
