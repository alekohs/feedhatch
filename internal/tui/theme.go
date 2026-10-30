package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type palette struct {
	text, muted, border, accent, selection color.Color
	background                             color.Color
}

func (m Model) palette() palette {
	if m.terminalTheme {
		// ANSI slots use the user's terminal palette, including Ghostty themes.
		// Leave foreground/background unset to inherit terminal defaults.
		return palette{
			muted: lipgloss.Color("8"), border: lipgloss.Color("8"),
			accent: lipgloss.Color("6"),
		}
	}
	return palette{
		text: lipgloss.Color("#c0caf5"), muted: lipgloss.Color("#8792b0"),
		border: lipgloss.Color("#414868"), accent: lipgloss.Color("#7dcfff"),
		selection: lipgloss.Color("#28364d"), background: lipgloss.Color("#1a1b26"),
	}
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
