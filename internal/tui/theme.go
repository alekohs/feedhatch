package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type palette struct {
	text, muted, border, accent, selection color.Color
	background                             color.Color
	category, unread, feed, statusBg       color.Color
	err, warn, ok                          color.Color
	code                                   color.Color // background of code blocks
}

func (m Model) palette() palette {
	if m.terminalTheme {
		// ANSI slots use the user's terminal palette, including Ghostty themes.
		// Leave foreground/background unset to inherit terminal defaults.
		return palette{
			muted: lipgloss.Color("8"), border: lipgloss.Color("8"),
			accent:   lipgloss.Color("6"),
			category: lipgloss.Color("5"), unread: lipgloss.Color("3"), feed: lipgloss.Color("4"),
			err: lipgloss.Color("1"), warn: lipgloss.Color("3"), ok: lipgloss.Color("2"),
			code: m.codeBackground(),
		}
	}
	return palette{
		text: lipgloss.Color("#c0caf5"), muted: lipgloss.Color("#8792b0"),
		border: lipgloss.Color("#414868"), accent: lipgloss.Color("#7dcfff"),
		selection: lipgloss.Color("#283457"), background: lipgloss.Color("#1a1b26"),
		category: lipgloss.Color("#bb9af7"), unread: lipgloss.Color("#ff9e64"), feed: lipgloss.Color("#7aa2f7"),
		statusBg: lipgloss.Color("#24283b"), err: lipgloss.Color("#f7768e"), warn: lipgloss.Color("#e0af68"),
		ok: lipgloss.Color("#9ece6a"), code: lipgloss.Color("#24283b"),
	}
}

// codeBackground is a shade off the terminal's own background, so code blocks fit any
// terminal theme; until the terminal says which background it has, there is none.
func (m Model) codeBackground() color.Color {
	switch {
	case m.terminalBg.Color == nil:
		return nil
	case m.terminalBg.IsDark():
		return lipgloss.Lighten(m.terminalBg.Color, 0.06)
	}
	return lipgloss.Darken(m.terminalBg.Color, 0.06)
}

func (p palette) normal() lipgloss.Style {
	s := lipgloss.NewStyle()
	if p.text != nil {
		s = s.Foreground(p.text)
	}
	return s
}

func (p palette) selected(focused bool) lipgloss.Style {
	s := p.normal().Bold(true)
	if !focused {
		return s.Foreground(p.accent)
	}
	if p.selection == nil {
		return s.Reverse(true)
	}
	return s.Background(p.selection).Foreground(p.accent)
}
