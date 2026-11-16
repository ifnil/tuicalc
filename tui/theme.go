package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type theme struct {
	palette []color.Color

	text    lipgloss.Style
	muted   lipgloss.Style
	faint   lipgloss.Style
	errText lipgloss.Style
	accent  lipgloss.Style

	border        lipgloss.Style
	borderFocused lipgloss.Style
	title         lipgloss.Style
	titleFocused  lipgloss.Style

	axis     lipgloss.Style // chart frame
	label    lipgloss.Style // tick labels
	zeroAxis lipgloss.Style // x = 0 and y = 0 lines
	grid     lipgloss.Style

	badge       lipgloss.Style
	badgeTrace  lipgloss.Style
	badgeInsert lipgloss.Style
	selBar      lipgloss.Style
	selBarDim   lipgloss.Style
}

func newTheme(isDark bool) theme {
	ld := lipgloss.LightDark(isDark)
	c := func(light, dark string) color.Color { return ld(lipgloss.Color(light), lipgloss.Color(dark)) }

	var palette []color.Color
	if isDark {
		palette = hexes("#ff6b6b", "#4dabf7", "#51cf66", "#cc5de8", "#ffa94d", "#22b8cf", "#f06595", "#fcc419")
	} else {
		palette = hexes("#e03131", "#1971c2", "#2f9e44", "#9c36b5", "#e8590c", "#0c8599", "#c2255c", "#f08c00")
	}

	accent := c("#5f3dc4", "#9775fa")
	muted := c("#5c5f66", "#a6a7ab")
	faint := c("#adb5bd", "#5c5f66")
	subtle := c("#ced4da", "#373a40")

	return theme{
		palette: palette,

		text:    lipgloss.NewStyle(),
		muted:   lipgloss.NewStyle().Foreground(muted),
		faint:   lipgloss.NewStyle().Foreground(faint),
		errText: lipgloss.NewStyle().Foreground(c("#c92a2a", "#ff8787")),
		accent:  lipgloss.NewStyle().Foreground(accent),

		border:        lipgloss.NewStyle().Foreground(faint),
		borderFocused: lipgloss.NewStyle().Foreground(accent),
		title:         lipgloss.NewStyle().Foreground(muted),
		titleFocused:  lipgloss.NewStyle().Foreground(accent).Bold(true),

		axis:     lipgloss.NewStyle().Foreground(faint),
		label:    lipgloss.NewStyle().Foreground(muted),
		zeroAxis: lipgloss.NewStyle().Foreground(muted),
		grid:     lipgloss.NewStyle().Foreground(subtle),

		badge:       lipgloss.NewStyle().Foreground(c("#ffffff", "#1a1b1e")).Background(accent).Bold(true).Padding(0, 1),
		badgeTrace:  lipgloss.NewStyle().Foreground(c("#ffffff", "#1a1b1e")).Background(c("#e8590c", "#ffa94d")).Bold(true).Padding(0, 1),
		badgeInsert: lipgloss.NewStyle().Foreground(c("#ffffff", "#1a1b1e")).Background(c("#2f9e44", "#69db7c")).Bold(true).Padding(0, 1),
		selBar:      lipgloss.NewStyle().Foreground(accent),
		selBarDim:   lipgloss.NewStyle().Foreground(faint),
	}
}

func hexes(hs ...string) []color.Color {
	cs := make([]color.Color, len(hs))
	for i, h := range hs {
		cs[i] = lipgloss.Color(h)
	}
	return cs
}

func (t theme) color(i int) color.Color {
	return t.palette[i%len(t.palette)]
}

func (t theme) curve(i int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.color(i))
}
