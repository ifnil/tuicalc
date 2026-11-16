package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit      key.Binding
	QuitGraph key.Binding
	Help      key.Binding
	Focus     key.Binding

	// expression list
	Up         key.Binding
	Down       key.Binding
	NewEntry   key.Binding
	NewAbove   key.Binding
	Delete     key.Binding
	ToggleShow key.Binding
	ToGraph    key.Binding

	// graph
	Pan      key.Binding // help only
	PanLeft  key.Binding
	PanRight key.Binding
	PanUp    key.Binding
	PanDown  key.Binding
	ZoomIn   key.Binding
	ZoomOut  key.Binding
	Reset    key.Binding
	Fit      key.Binding
	Grid     key.Binding
	Angle    key.Binding
	Trace    key.Binding
	ToList   key.Binding

	// trace
	TraceMove  key.Binding // help only
	TraceLeft  key.Binding
	TraceRight key.Binding
	TraceFastL key.Binding
	TraceFastR key.Binding
	TraceCurve key.Binding // help only
	TracePrevC key.Binding
	TraceNextC key.Binding
	NextPOI    key.Binding
	PrevPOI    key.Binding
	StopTrace  key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Quit:      key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		QuitGraph: key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Help:      key.NewBinding(key.WithKeys("?", "f1"), key.WithHelp("?", "help")),
		Focus:     key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "switch pane")),

		Up:       key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/↓", "select")),
		Down:     key.NewBinding(key.WithKeys("down", "ctrl+n")),
		NewEntry: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "new")),
		// shift+enter only arrives on terminals with enhanced keyboard
		// reporting (kitty, ghostty, wezterm, foot…); alt+enter works widely.
		NewAbove:   key.NewBinding(key.WithKeys("alt+enter", "shift+enter"), key.WithHelp("alt+enter", "new above")),
		Delete:     key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl+x", "delete")),
		ToggleShow: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "show/hide")),
		ToGraph:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "graph")),

		Pan:      key.NewBinding(key.WithKeys("left", "right", "up", "down"), key.WithHelp("←↓↑→/hjkl", "pan")),
		PanLeft:  key.NewBinding(key.WithKeys("left", "h")),
		PanRight: key.NewBinding(key.WithKeys("right", "l")),
		PanUp:    key.NewBinding(key.WithKeys("up", "k")),
		PanDown:  key.NewBinding(key.WithKeys("down", "j")),
		ZoomIn:   key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+/-", "zoom")),
		ZoomOut:  key.NewBinding(key.WithKeys("-", "_")),
		Reset:    key.NewBinding(key.WithKeys("0", "r"), key.WithHelp("0", "reset view")),
		Fit:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "fit y")),
		Grid:     key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "grid")),
		Angle:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "deg/rad")),
		Trace:    key.NewBinding(key.WithKeys("t", "enter"), key.WithHelp("t", "trace")),
		ToList:   key.NewBinding(key.WithKeys("esc", "i"), key.WithHelp("i", "edit")),

		TraceMove:  key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "move (shift: ×10)")),
		TraceLeft:  key.NewBinding(key.WithKeys("left", "h")),
		TraceRight: key.NewBinding(key.WithKeys("right", "l")),
		TraceFastL: key.NewBinding(key.WithKeys("shift+left", "H")),
		TraceFastR: key.NewBinding(key.WithKeys("shift+right", "L")),
		TraceCurve: key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "curve")),
		TracePrevC: key.NewBinding(key.WithKeys("up", "k")),
		TraceNextC: key.NewBinding(key.WithKeys("down", "j")),
		NextPOI:    key.NewBinding(key.WithKeys("]", "n"), key.WithHelp("[/]", "roots & extrema")),
		PrevPOI:    key.NewBinding(key.WithKeys("[", "N")),
		StopTrace:  key.NewBinding(key.WithKeys("esc", "t", "enter"), key.WithHelp("esc", "stop")),
	}
}

func (k keyMap) listHelp() []key.Binding {
	// Terser labels than the full help so the line fits ~100 columns; the
	// help bubble truncates from the right, so the least needed go last.
	return []key.Binding{k.Up, k.NewEntry, k.NewAbove, k.Delete, hint("ctrl+t", "hide"),
		hint("tab", "graph"), hint("f1", "help"), k.Quit}
}

func (k keyMap) helpPageHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	}
}

func (k keyMap) graphHelp() []key.Binding {
	return []key.Binding{k.Pan, k.ZoomIn, k.Trace, k.Fit, k.Reset, k.Grid, k.Angle, k.ToList, k.Help, k.QuitGraph}
}

func (k keyMap) traceHelp() []key.Binding {
	return []key.Binding{k.TraceMove, k.TraceCurve, k.NextPOI, k.ZoomIn, k.StopTrace, k.Help}
}

func hint(keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
}

func (k keyMap) vimNormalHelp() []key.Binding {
	return []key.Binding{hint("i/a/o/O", "insert"), hint("dd", "delete"), hint("p", "paste"),
		hint("u", "undo"), hint("ctrl+a/x", "±1"), hint(":", "command"), hint("tab", "graph"),
		hint("f1", "help")}
}

func (k keyMap) vimInsertHelp() []key.Binding {
	return []key.Binding{hint("esc", "normal mode"), k.NewEntry, k.NewAbove, hint("↑/↓", "select"),
		hint("tab", "graph"), hint("f1", "help")}
}

func (k keyMap) vimGraphHelp() []key.Binding {
	return []key.Binding{k.Pan, k.ZoomIn, k.Trace, k.Fit, k.Reset, hint("i", "insert"),
		hint(":", "command"), hint("ctrl+w", "pane"), k.Help}
}

func (k keyMap) cmdlineHelp() []key.Binding {
	return []key.Binding{hint("enter", "run"), hint("esc", "cancel"),
		hint(":q", "quit"), hint(":N", "go to entry"), hint(":set deg/rad/grid", "options")}
}

func vimHelpGroups() []helpGroup {
	return []helpGroup{
		{"Vim: move & insert", []key.Binding{
			hint("i a I A", "insert / append"),
			hint("o O", "new entry below / above"),
			hint("esc ctrl+c", "back to normal mode"),
			hint("h l w b e", "move in entry"),
			hint("0 ^ $", "start / end of entry"),
			hint("f t F T ; ,", "find character"),
			hint("j k gg G", "move between entries"),
		}},
		{"Vim: edit", []key.Binding{
			hint("d c y", "delete / change / yank"),
			hint("dd cc yy", "whole entry"),
			hint("iw a( i|", "text objects: w W ( [ { |"),
			hint("x X s S C D", "shorthands"),
			hint("r ~", "replace / toggle case"),
			hint("p P", "paste after / before"),
			hint("u ctrl+r", "undo / redo"),
			hint("ctrl+a ctrl+x", "increment / decrement"),
		}},
		{"Vim: commands", []key.Binding{
			hint(":q ZZ", "quit"),
			hint(":help", "this page"),
			hint(":N", "go to entry N"),
			hint(":set deg|rad", "angle unit"),
			hint(":set [no]grid", "grid"),
			hint("ctrl+w h/l/w", "switch pane"),
			hint("ctrl+d ctrl+u", "graph: pan half page"),
			hint("w b", "trace: next / previous"),
		}},
	}
}

type helpGroup struct {
	title    string
	bindings []key.Binding
}

// fullHelp returns the groups for the help page. With vim on, the list
// keys that vim repurposes (esc, ctrl+x, ctrl+c) are left out; the vim
// groups cover them.
func (k keyMap) fullHelp(vim bool) []helpGroup {
	mouse := func(keys, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(), key.WithHelp(keys, desc))
	}
	expressions := []key.Binding{k.Up, k.NewEntry, k.NewAbove, k.ToggleShow, k.Delete, k.ToGraph}
	general := []key.Binding{k.Focus, k.Help, k.QuitGraph, k.Quit}
	if vim {
		expressions = []key.Binding{k.Up, k.NewEntry, k.NewAbove, k.ToggleShow}
		general = []key.Binding{k.Focus, k.Help, k.QuitGraph}
	}
	groups := []helpGroup{
		{"Expressions", expressions},
		{"Graph", []key.Binding{k.Pan, k.ZoomIn, k.Reset, k.Fit, k.Grid, k.Angle, k.ToList}},
		{"Trace", []key.Binding{k.Trace, k.TraceMove, k.TraceCurve, k.NextPOI, k.StopTrace}},
		{"Mouse", []key.Binding{mouse("drag", "pan"), mouse("scroll", "zoom"), mouse("click", "trace / select")}},
		{"General", general},
	}
	if vim {
		groups = append(groups, vimHelpGroups()...)
	}
	return groups
}
