package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type searchState struct {
	active bool
	input  string
	query  string
}

func searchPattern(query string) *regexp.Regexp {
	if query == "" {
		return nil
	}
	// Literal matching avoids surprising regex syntax and supports Unicode case.
	return regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
}

func (m *Model) appendSearchInput(text string) {
	text = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, text)
	runes := []rune(m.search.input + text)
	if len(runes) > 2048 {
		runes = runes[:2048]
	}
	m.search.input = string(runes)
}

func (m *Model) updateSearch(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		// Keep both the previous search and the current cursor/selection intact.
		m.search.active, m.search.input = false, ""
	case "enter":
		includeCurrent := m.search.input != ""
		if includeCurrent {
			m.search.query = m.search.input
		}
		m.search.active, m.search.input = false, ""
		m.findSearchMatch(1, includeCurrent)
	case "backspace":
		runes := []rune(m.search.input)
		if len(runes) > 0 {
			m.search.input = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		m.search.input = ""
	default:
		m.appendSearchInput(msg.Text)
	}
}

func (m Model) matchingRows() []int {
	pattern := searchPattern(m.search.query)
	if pattern == nil {
		return nil
	}
	_, right, _ := m.dimensions()
	var matches []int
	for i, line := range m.readerLines(right - 6) {
		if pattern.MatchString(line) {
			matches = append(matches, i)
		}
	}
	return matches
}

func (m *Model) findSearchMatch(direction int, includeCurrent bool) {
	if m.search.query == "" {
		m.status = "No previous search. Press / to search this article."
		return
	}
	matches := m.matchingRows()
	if len(matches) == 0 {
		m.status = fmt.Sprintf("No matches for %q.", m.search.query)
		return
	}
	index, wrapped := 0, false
	if direction > 0 {
		found := false
		for i, row := range matches {
			if row > m.line || (includeCurrent && row == m.line) {
				index, found = i, true
				break
			}
		}
		wrapped = !found
	} else {
		index = len(matches) - 1
		found := false
		for i := len(matches) - 1; i >= 0; i-- {
			if matches[i] < m.line || (includeCurrent && matches[i] == m.line) {
				index, found = i, true
				break
			}
		}
		wrapped = !found
	}
	m.line = matches[index]
	m.ensureLineVisible()
	m.status = fmt.Sprintf("/%s · matching row %d/%d · n next · N previous", m.search.query, index+1, len(matches))
	if wrapped {
		m.status += " · wrapped"
	}
}

func (m Model) searchPrompt(width int) string {
	value := "/" + m.search.input + "▏"
	if excess := lipgloss.Width(value) - width; excess > 0 {
		value = ansi.TruncateLeft(value, excess+1, "…")
	}
	return value
}

func highlightSearch(text string, pattern *regexp.Regexp, style lipgloss.Style) string {
	if pattern == nil {
		return text
	}
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		return style.Render(match)
	})
}
