// Package tui implements the graphing calculator's terminal interface.
package tui

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"tcalc/expr"
)

type focusArea int

const (
	focusList focusArea = iota
	focusGraph
)

type entry struct {
	text   string
	hidden bool
	color  int
}

// Model is the root Bubble Tea model.
type Model struct {
	entries   []entry
	results   []expr.Result
	sel       int
	scroll    int // first visible line of the expression list
	input     textinput.Model
	focus     focusArea
	nextColor int

	vp      viewport
	vpReady bool
	degrees bool
	grid    bool

	tracing   bool
	traceIdx  int // entry being traced
	traceX    float64
	traceNote string // "root", "maximum", ...

	hoverOK        bool
	hoverX, hoverY float64

	dragging     bool
	dragMoved    bool
	dragX, dragY int

	width, height int
	dark          bool
	th            theme
	help          help.Model
	keys          keyMap
	showHelp      bool
	helpScroll    int
	flash         string // one-off status message
	flashErr      bool

	vimOn     bool
	vim       vimState
	cmdActive bool            // typing a : command
	cmd       textinput.Model // the : command line
}

// Options configures a new Model.
type Options struct {
	Expressions []string
	Degrees     bool
	Vim         bool // vim-style modal editing
}

// New returns a calculator preloaded with the given expressions.
func New(opts Options) Model {
	m := Model{
		degrees: opts.Degrees,
		vimOn:   opts.Vim,
		grid:    true,
		dark:    true,
		th:      newTheme(true),
		help:    help.New(),
		keys:    defaultKeys(),
	}
	m.help.Styles = help.DefaultStyles(true)

	m.input = textinput.New()
	m.input.Prompt = ""
	m.applyInputMode()
	m.input.Focus()

	m.cmd = textinput.New()
	m.cmd.Prompt = ":"
	m.cmd.SetStyles(textinput.DefaultStyles(true))

	for _, e := range opts.Expressions {
		if strings.TrimSpace(e) != "" {
			m.entries = append(m.entries, m.newEntry(e))
		}
	}
	m.entries = append(m.entries, m.newEntry(""))
	m.sel = len(m.entries) - 1
	m.input.SetValue(m.entries[m.sel].text)
	m.compile()
	m.clampNormalCursor()
	return m
}

func (m *Model) newEntry(text string) entry {
	e := entry{text: text, color: m.nextColor}
	m.nextColor++
	return e
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, textinput.Blink)
}

func (m *Model) compile() {
	srcs := make([]string, len(m.entries))
	for i, e := range m.entries {
		srcs[i] = e.text
	}
	m.results = expr.Compile(srcs, expr.Options{Degrees: m.degrees})
	m.validateTrace()
}

// ---------------------------------------------------------------------------
// Layout

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

type layout struct {
	list   rect // list box, including its border
	plot   rect // plot box, including its border
	canvas rect // chart canvas inside the plot box
}

const footerHeight = 2

func (m Model) layout() layout {
	mainH := max(0, m.height-footerHeight)
	listW := min(max(m.width*3/10, 26), 44)
	if m.width-listW < 30 {
		listW = max(16, m.width/2)
	}
	plotW := m.width - listW
	return layout{
		list:   rect{0, 0, listW, mainH},
		plot:   rect{listW, 0, plotW, mainH},
		canvas: rect{listW + 1, 1, max(0, plotW-2), max(0, mainH-2)},
	}
}

func (m Model) tooSmall() bool {
	l := m.layout()
	return l.canvas.w < 16 || l.canvas.h < 6 || l.list.w < 16
}

func (m Model) frame() *frame {
	c := m.layout().canvas
	return newFrame(c.w, c.h, m.vp, &m.th)
}

// ---------------------------------------------------------------------------
// Update

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.update(msg)
	m.clampNormalCursor()
	m.ensureVisible()
	return m, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.input.SetWidth(max(1, m.layout().list.w-5))
		if !m.vpReady && !m.tooSmall() {
			m.resetView()
			m.vpReady = true
		}
		return m, nil

	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.th = newTheme(m.dark)
		m.help.Styles = help.DefaultStyles(m.dark)
		m.applyInputMode()
		m.cmd.SetStyles(textinput.DefaultStyles(m.dark))
		return m, nil

	case tea.KeyPressMsg:
		m.flash, m.flashErr = "", false
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	var cmds []tea.Cmd
	if m.cmdActive {
		var cmd tea.Cmd
		m.cmd, cmd = m.cmd.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.focus == focusList {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.syncInput()
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := m.keys
	switch {
	case m.cmdActive:
		return m.cmdKey(msg)
	case key.Matches(msg, k.Quit):
		if m.vimOn {
			return m.vimCtrlC()
		}
		return m, tea.Quit
	case m.showHelp:
		return m.helpKey(msg), nil
	case key.Matches(msg, k.Focus):
		return m, m.setFocus(1 - m.focus)
	case msg.String() == "f1":
		m.showHelp, m.helpScroll = true, 0
		return m, nil
	}
	if m.focus == focusList {
		return m.listKey(msg)
	}
	if m.vimOn {
		if m2, cmd, ok := m.vimGraphKey(msg); ok {
			return m2, cmd
		}
	}
	return m.graphKey(msg)
}

func (m Model) helpKey(msg tea.KeyPressMsg) Model {
	switch msg.String() {
	case "esc", "q", "?", "f1":
		m.showHelp = false
	case "up", "k":
		m.helpScroll--
	case "down", "j":
		m.helpScroll++
	case "pgup":
		m.helpScroll -= m.layout().list.h / 2
	case "pgdown", "space":
		m.helpScroll += m.layout().list.h / 2
	case "home", "g":
		m.helpScroll = 0
	}
	m.clampHelpScroll()
	return m
}

func (m *Model) clampHelpScroll() {
	h := m.layout().list.h - 2
	m.helpScroll = max(0, min(m.helpScroll, len(m.helpLines(m.width))-h))
}

func (m *Model) setFocus(f focusArea) tea.Cmd {
	if m.vimOn && m.vim.insert && f != focusList {
		m.leaveInsert()
	}
	m.vim.pending = nil
	m.focus = f
	if f == focusList {
		m.applyInputMode()
		return m.input.Focus()
	}
	m.input.Blur()
	return nil
}

func (m Model) listKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.vimOn {
		if !m.vim.insert {
			return m.vimListKey(msg)
		}
		switch msg.String() {
		case "esc":
			m.leaveInsert()
			return m, nil
		case "ctrl+x": // vim's ctrl+x is a normal-mode command
			return m, nil
		}
	}
	k := m.keys
	switch {
	case key.Matches(msg, k.ToGraph):
		return m, m.setFocus(focusGraph)

	case key.Matches(msg, k.Up):
		m.selectEntry(m.sel - 1)
		return m, nil

	case key.Matches(msg, k.Down):
		if m.sel == len(m.entries)-1 && strings.TrimSpace(m.entries[m.sel].text) != "" {
			m.insertEntry(m.sel + 1)
		} else {
			m.selectEntry(m.sel + 1)
		}
		return m, nil

	case key.Matches(msg, k.NewEntry):
		if strings.TrimSpace(m.entries[m.sel].text) != "" {
			m.insertEntry(m.sel + 1)
		}
		return m, nil

	case key.Matches(msg, k.NewAbove):
		if strings.TrimSpace(m.entries[m.sel].text) != "" {
			m.insertEntry(m.sel)
		}
		return m, nil

	case key.Matches(msg, k.Delete):
		m.deleteEntry(m.sel, false)
		return m, nil

	case key.Matches(msg, k.ToggleShow):
		m.entries[m.sel].hidden = !m.entries[m.sel].hidden
		m.validateTrace()
		return m, nil

	case msg.String() == "backspace" && m.input.Value() == "" && len(m.entries) > 1:
		m.deleteEntry(m.sel, true)
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.syncInput()
	return m, cmd
}

// syncInput copies the text input back into the selected entry.
func (m *Model) syncInput() {
	if v := m.input.Value(); v != m.entries[m.sel].text {
		m.entries[m.sel].text = v
		m.compile()
	}
}

func (m *Model) selectEntry(i int) {
	i = min(max(i, 0), len(m.entries)-1)
	m.sel = i
	m.input.SetValue(m.entries[i].text)
	m.input.CursorEnd()
}

func (m *Model) insertEntry(at int) {
	m.insertEntries(at, []string{""})
	m.selectEntry(at)
}

func (m *Model) insertEntries(at int, texts []string) {
	es := make([]entry, len(texts))
	for i, t := range texts {
		es[i] = m.newEntry(t)
	}
	m.entries = slices.Insert(m.entries, at, es...)
	if m.traceIdx >= at {
		m.traceIdx += len(es) // keep tracing the same curve
	}
	m.compile()
}

// removeEntries deletes entries [from, to), leaving at least one entry.
func (m *Model) removeEntries(from, to int) {
	m.entries = slices.Delete(m.entries, from, to)
	if m.traceIdx >= to {
		m.traceIdx -= to - from
	}
	if len(m.entries) == 0 {
		m.entries = []entry{m.newEntry("")}
	}
	m.compile()
}

// deleteEntry removes entry i and selects its neighbour: the one above when
// backspacing out of an entry, otherwise the one that takes its place.
func (m *Model) deleteEntry(i int, selectAbove bool) {
	m.removeEntries(i, i+1)
	if selectAbove {
		i--
	}
	m.selectEntry(min(max(i, 0), len(m.entries)-1))
}

func (m Model) graphKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := m.keys
	f := m.frame()

	if m.tracing {
		switch {
		case key.Matches(msg, k.StopTrace):
			m.tracing = false
			return m, nil
		case key.Matches(msg, k.TraceLeft):
			m.moveTrace(f, -1)
			return m, nil
		case key.Matches(msg, k.TraceRight):
			m.moveTrace(f, 1)
			return m, nil
		case key.Matches(msg, k.TraceFastL):
			m.moveTrace(f, -10)
			return m, nil
		case key.Matches(msg, k.TraceFastR):
			m.moveTrace(f, 10)
			return m, nil
		case key.Matches(msg, k.TracePrevC):
			m.cycleTrace(-1)
			return m, nil
		case key.Matches(msg, k.TraceNextC):
			m.cycleTrace(1)
			return m, nil
		case key.Matches(msg, k.NextPOI):
			m.jumpFeature(f, 1)
			return m, nil
		case key.Matches(msg, k.PrevPOI):
			m.jumpFeature(f, -1)
			return m, nil
		}
	}

	panX, panY := 0.1*float64(f.dotsW())*m.vp.sx, 0.1*float64(f.dotsH())*m.vp.sy
	switch {
	case key.Matches(msg, k.QuitGraph):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp, m.helpScroll = true, 0
	case key.Matches(msg, k.ToList):
		return m, m.setFocus(focusList)
	case key.Matches(msg, k.PanLeft):
		m.vp.cx -= panX
	case key.Matches(msg, k.PanRight):
		m.vp.cx += panX
	case key.Matches(msg, k.PanUp):
		m.vp.cy += panY
	case key.Matches(msg, k.PanDown):
		m.vp.cy -= panY
	case key.Matches(msg, k.ZoomIn):
		x, y := m.zoomCenter()
		m.vp.zoomAt(x, y, 0.8, 0.8)
	case key.Matches(msg, k.ZoomOut):
		x, y := m.zoomCenter()
		m.vp.zoomAt(x, y, 1.25, 1.25)
	case key.Matches(msg, k.Reset):
		m.resetView()
	case key.Matches(msg, k.Fit):
		m.fitY()
	case key.Matches(msg, k.Grid):
		m.grid = !m.grid
	case key.Matches(msg, k.Angle):
		m.degrees = !m.degrees
		m.compile()
		if m.degrees {
			m.flash = "angles in degrees"
		} else {
			m.flash = "angles in radians"
		}
	case key.Matches(msg, k.Trace):
		m.startTrace(f)
	}
	m.vp.clamp()
	return m, nil
}

func (m Model) zoomCenter() (float64, float64) {
	if m.tracing {
		if fn := m.results[m.traceIdx].Fn; fn != nil {
			if y := fn(m.traceX); finite(y) {
				return m.traceX, y
			}
			return m.traceX, m.vp.cy
		}
	}
	return m.vp.cx, m.vp.cy
}

// resetView shows -10 ≤ x ≤ 10 with equal scales on both axes.
func (m *Model) resetView() {
	m.vp = viewport{sx: 0.1, sy: 0.1}
	for range 2 { // the y labels' width depends on the scale
		f := m.frame()
		if f.W <= 0 {
			return
		}
		s := 20 / float64(f.dotsW())
		m.vp = viewport{sx: s, sy: s}
	}
}

func (m *Model) fitY() {
	// Wider y labels narrow the plot and with it the x range, so fit
	// again once the labels for the new range are known.
	for range 2 {
		if !m.fitYOnce(m.frame()) {
			return
		}
	}
}

func (m *Model) fitYOnce(f *frame) bool {
	var ys []float64
	for i, r := range m.results {
		if r.Fn == nil || m.entries[i].hidden {
			continue
		}
		// Sample every dot column plus the exact edges of the view.
		for d := -0.5; d <= float64(f.dotsW())-0.5; d += 0.5 {
			if y := r.Fn(f.dotToX(d)); finite(y) {
				ys = append(ys, y)
			}
		}
	}
	lo, hi, ok := fitRange(ys)
	if !ok {
		m.flash = "nothing to fit"
		return false
	}
	m.vp.cy = (lo + hi) / 2
	m.vp.sy = (hi - lo) / float64(f.dotsH())
	m.vp.clamp()
	return true
}

// ---------------------------------------------------------------------------
// Trace

func (m Model) traceable(i int) bool {
	return i >= 0 && i < len(m.results) && m.results[i].Fn != nil && !m.entries[i].hidden
}

func (m *Model) startTrace(f *frame) {
	idx := -1
	if m.traceable(m.sel) {
		idx = m.sel
	} else {
		for i := range m.results {
			if m.traceable(i) {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		m.flash = "nothing to trace — add a function first"
		return
	}
	m.tracing = true
	m.traceIdx = idx
	m.traceNote = ""
	if lo, hi := f.xRange(); m.traceX < lo || m.traceX > hi {
		m.traceX = m.vp.cx
	}
}

func (m *Model) validateTrace() {
	if !m.tracing || m.traceable(m.traceIdx) {
		return
	}
	for i := range m.results {
		if m.traceable(i) {
			m.traceIdx = i
			return
		}
	}
	m.tracing = false
}

func (m *Model) cycleTrace(dir int) {
	n := len(m.results)
	for step := 1; step <= n; step++ {
		i := ((m.traceIdx+dir*step)%n + n) % n
		if m.traceable(i) {
			m.traceIdx = i
			m.traceNote = ""
			return
		}
	}
}

func (m *Model) moveTrace(f *frame, cells float64) {
	m.traceX += cells * 2 * m.vp.sx
	m.traceNote = ""
	m.keepTraceVisible(f)
}

func (m *Model) jumpFeature(f *frame, dir int) {
	fn := m.results[m.traceIdx].Fn
	x, kind, ok := nextFeature(fn, m.traceX, m.vp.sx, dir, 4*f.dotsW())
	if !ok {
		m.flash = "no root or extremum nearby"
		return
	}
	m.traceX = x
	m.traceNote = kind
	m.keepTraceVisible(f)
	if y := fn(x); finite(y) {
		if lo, hi := f.yRange(); y < lo || y > hi {
			m.vp.cy = y
		}
	}
}

// keepTraceVisible pans horizontally so the trace cursor stays on screen.
func (m *Model) keepTraceVisible(f *frame) {
	lo, hi := f.xRange()
	margin := 0.05 * (hi - lo)
	switch {
	case m.traceX < lo+margin:
		m.vp.cx += m.traceX - (lo + margin)
	case m.traceX > hi-margin:
		m.vp.cx += m.traceX - (hi - margin)
	}
}

// ---------------------------------------------------------------------------
// Mouse

func (m Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.tooSmall() {
		return m, nil
	}
	if m.showHelp {
		if w, ok := msg.(tea.MouseWheelMsg); ok {
			switch w.Button {
			case tea.MouseWheelUp:
				m.helpScroll = max(0, m.helpScroll-2)
			case tea.MouseWheelDown:
				m.helpScroll += 2
			}
			m.clampHelpScroll()
		}
		return m, nil
	}
	mouse := msg.Mouse()
	lay := m.layout()
	f := m.frame()
	j, k, inPlot := f.canvasCell(mouse.X-lay.canvas.x, mouse.Y-lay.canvas.y)

	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		if inPlot {
			x, y := f.cellCenter(j, k)
			switch msg.Button {
			case tea.MouseWheelUp:
				m.vp.zoomAt(x, y, 0.85, 0.85)
			case tea.MouseWheelDown:
				m.vp.zoomAt(x, y, 1/0.85, 1/0.85)
			}
		} else if lay.list.contains(mouse.X, mouse.Y) {
			switch msg.Button {
			case tea.MouseWheelUp:
				m.selectEntry(m.sel - 1)
			case tea.MouseWheelDown:
				m.selectEntry(m.sel + 1)
			}
		}

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		if lay.plot.contains(mouse.X, mouse.Y) {
			m.dragging, m.dragMoved = true, false
			m.dragX, m.dragY = mouse.X, mouse.Y
			return m, m.setFocus(focusGraph)
		}
		if lay.list.contains(mouse.X, mouse.Y) {
			m.clickList(mouse.Y - lay.list.y - 1)
			return m, m.setFocus(focusList)
		}

	case tea.MouseMotionMsg:
		if m.dragging {
			dx, dy := mouse.X-m.dragX, mouse.Y-m.dragY
			if dx != 0 || dy != 0 {
				m.vp.cx -= float64(dx) * 2 * m.vp.sx
				m.vp.cy += float64(dy) * 4 * m.vp.sy
				m.vp.clamp()
				m.dragX, m.dragY = mouse.X, mouse.Y
				m.dragMoved = true
			}
		}
		m.hoverOK = inPlot
		if inPlot {
			m.hoverX, m.hoverY = f.cellCenter(j, k)
		}

	case tea.MouseReleaseMsg:
		if m.dragging && !m.dragMoved && inPlot {
			m.traceAt(f, j, k)
		}
		m.dragging = false
	}
	return m, nil
}

// traceAt starts tracing the curve closest to the clicked cell.
func (m *Model) traceAt(f *frame, j, k int) {
	x, y := f.cellCenter(j, k)
	best, bestDist := -1, math.Inf(1)
	for i, r := range m.results {
		if !m.traceable(i) {
			continue
		}
		if v := r.Fn(x); finite(v) {
			if d := math.Abs(v - y); d < bestDist {
				best, bestDist = i, d
			}
		}
	}
	if best < 0 {
		return
	}
	m.tracing = true
	m.traceIdx = best
	m.traceX = x
	m.traceNote = ""
}

func (m *Model) clickList(line int) {
	lines := m.listLines(m.layout().list.w - 2)
	idx := line + m.scroll
	if line < 0 {
		return
	}
	if idx >= len(lines) {
		m.selectEntry(len(m.entries) - 1)
		return
	}
	m.selectEntry(lines[idx].entry)
}

// ensureVisible scrolls the list so the selected entry is on screen.
func (m *Model) ensureVisible() {
	lay := m.layout()
	h := lay.list.h - 2
	if h <= 0 {
		return
	}
	lines := m.listLines(lay.list.w - 2)
	first, last := -1, -1
	for i, l := range lines {
		if l.entry == m.sel {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return
	}
	if first < m.scroll {
		m.scroll = first
	}
	if last >= m.scroll+h {
		m.scroll = last - h + 1
	}
	m.scroll = max(0, min(m.scroll, len(lines)-h))
}

// ---------------------------------------------------------------------------
// View

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.WindowTitle = "tcalc"
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.tooSmall() {
		return "Terminal too small — please enlarge the window."
	}
	lay := m.layout()
	var main string
	if m.showHelp {
		main = m.renderHelpPage(rect{0, 0, m.width, lay.list.h})
	} else {
		main = joinColumns(m.renderList(lay.list), m.renderPlot(lay.plot))
	}
	return main + "\n" + m.renderStatus() + "\n" + m.renderHelpLine()
}

func (m Model) curves() []curve {
	var cs []curve
	for i, r := range m.results {
		if m.entries[i].hidden || !r.Plottable() {
			continue
		}
		c := curve{fn: r.Fn, style: m.th.curve(m.entries[i].color)}
		if r.Kind == expr.VLine {
			if !finite(r.Value) {
				continue
			}
			c.vline, c.x = true, r.Value
		}
		cs = append(cs, c)
	}
	return cs
}

func (m Model) renderPlot(r rect) string {
	f := m.frame()
	var mk *marker
	if m.tracing {
		if y := m.results[m.traceIdx].Fn(m.traceX); finite(y) {
			mk = &marker{x: m.traceX, y: y, style: m.th.curve(m.entries[m.traceIdx].color)}
		}
	}
	chart := f.render(&m.th, m.curves(), m.grid, mk)

	angle := "rad"
	if m.degrees {
		angle = "deg"
	}
	bs, ts := m.th.border, m.th.title
	if m.focus == focusGraph {
		bs, ts = m.th.borderFocused, m.th.titleFocused
	}
	out := box("Graph", m.th.muted.Render(angle), strings.Split(chart, "\n"), r.w, r.h, bs, ts)
	return m.overlayTable(out, f, r.w)
}

func (m Model) renderStatus() string {
	if m.cmdActive {
		return m.cmd.View()
	}
	var badge, body string
	switch {
	case m.vimOn && m.focus == focusList && m.vim.insert:
		badge = m.th.badgeInsert.Render("INSERT")
	case m.vimOn && m.focus == focusList:
		badge = m.th.badge.Render("NORMAL")
	case m.focus == focusGraph && m.tracing:
		badge = m.th.badgeTrace.Render("TRACE")
	case m.focus == focusGraph:
		badge = m.th.badge.Render("GRAPH")
	default:
		badge = m.th.badge.Render("EDIT")
	}

	switch {
	case m.flash != "" && m.flashErr:
		body = m.th.errText.Render(m.flash)
	case m.flash != "":
		body = m.th.muted.Render(m.flash)
	case m.focus == focusList && m.results[m.sel].Err != nil:
		body = m.th.errText.Render(m.results[m.sel].Err.Error())
	case m.tracing:
		e := m.entries[m.traceIdx]
		y := m.results[m.traceIdx].Fn(m.traceX)
		name := strings.TrimSpace(e.text)
		if len([]rune(name)) > 24 {
			name = string([]rune(name)[:23]) + "…"
		}
		// Extrema are located to about 8 digits; don't show more.
		digits := 10
		if m.traceNote == "minimum" || m.traceNote == "maximum" {
			digits = 8
		}
		body = m.th.curve(e.color).Render("● "+name) + "  " + fmt.Sprintf("x = %s  y = %s",
			expr.FormatPrec(snapZero(m.traceX, m.vp.sx), digits),
			expr.FormatPrec(snapZero(y, m.vp.sy), digits))
		if m.traceNote != "" {
			body += "  " + m.th.accent.Render(m.traceNote)
		}
	case m.focus == focusList && m.isTable(m.sel):
		r := m.results[m.sel]
		body = m.th.muted.Render(fmt.Sprintf("table around x = %s, step %s · trace a curve (t) to move it",
			expr.FormatPrec(m.tableCenter(), 8), expr.FormatNumber(r.Step)))
	case m.hoverOK:
		body = m.th.muted.Render(fmt.Sprintf("x = %s  y = %s",
			formatCoord(m.hoverX, m.vp.sx), formatCoord(m.hoverY, m.vp.sy)))
	}

	f := m.frame()
	x0, x1 := f.xRange()
	y0, y1 := f.yRange()
	rng := m.th.faint.Render(fmt.Sprintf("x %s…%s  y %s…%s",
		formatCoord(x0, m.vp.sx), formatCoord(x1, m.vp.sx),
		formatCoord(y0, m.vp.sy), formatCoord(y1, m.vp.sy)))

	if len(m.vim.pending) > 0 { // like vim's showcmd
		rng = m.th.accent.Render(strings.Join(m.vim.pending, "")) + "   " + rng
	}
	return spread(badge+" "+body, rng, m.width)
}

func (m Model) renderHelpLine() string {
	var bs []key.Binding
	switch {
	case m.cmdActive:
		bs = m.keys.cmdlineHelp()
	case m.showHelp:
		bs = m.keys.helpPageHelp()
	case m.vimOn && m.focus == focusList && m.vim.insert:
		bs = m.keys.vimInsertHelp()
	case m.vimOn && m.focus == focusList:
		bs = m.keys.vimNormalHelp()
	case m.vimOn && !m.tracing:
		bs = m.keys.vimGraphHelp()
	case m.focus == focusList:
		bs = m.keys.listHelp()
	case m.tracing:
		bs = m.keys.traceHelp()
	default:
		bs = m.keys.graphHelp()
	}
	return " " + m.help.ShortHelpView(bs)
}

// snapZero hides floating point noise around zero, such as the
// 1.2e-16 that sin(pi) evaluates to.
func snapZero(v, scale float64) float64 {
	if math.Abs(v) < scale*1e-9 {
		return 0
	}
	return v
}

// formatCoord rounds a coordinate to the precision one dot represents.
func formatCoord(v, scale float64) string {
	step := expr.NiceStep(scale) / 10
	return expr.FormatNumber(math.Round(v/step) * step)
}
