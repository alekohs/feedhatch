package tui

import (
	"slices"
	"strings"
)

func (m *Model) centerLine() {
	_, _, height := m.dimensions()
	visible := max(1, height-4)
	m.scroll = max(0, min(m.maxScroll(), m.line-visible/2))
}

// Page motions are shared by the reader, lists and help. Forms handle keys first
// so typing d, u, f or b never triggers navigation while editing a field.
func (m Model) pageMotion(key string) (int, bool) {
	_, _, height := m.dimensions()
	page := max(1, height-4)
	half := max(1, page/2)
	switch key {
	case "d", "ctrl+d":
		return half, true
	case "u", "ctrl+u":
		return -half, true
	case "f", "ctrl+f", "pgdown", "space", " ":
		return page, true
	case "b", "ctrl+b", "pgup", "shift+space":
		return -page, true
	default:
		return 0, false
	}
}

// Paragraphs are runs of nonblank rendered rows. Jump to paragraph starts rather
// than wrapped line boundaries, including when the cursor is on a blank row.
func (m *Model) moveParagraph(direction int) {
	if !m.reading {
		return
	}
	lines := m.readerLines(m.readerWidth())
	blank := func(i int) bool { return strings.TrimSpace(lines[i]) == "" }
	target := m.line
	if direction > 0 {
		for target < len(lines) && !blank(target) {
			target++
		}
		for target < len(lines) && blank(target) {
			target++
		}
	} else {
		// From the middle of a paragraph, first return to its start. From the
		// start (or a blank row), jump to the previous paragraph's start.
		target--
		for target >= 0 && blank(target) {
			target--
		}
		for target > 0 && !blank(target-1) {
			target--
		}
	}
	m.line = clamp(target, len(lines))
	m.ensureLineVisible()
}

// moveCategory jumps to the first entry of the previous or next sidebar group.
// The two virtual entries count as the first group.
func (m *Model) moveCategory(direction int) {
	categories := m.categories()
	group := 0
	if m.view == feedView && len(m.feeds) > 0 {
		group = 1 + slices.Index(categories, m.feeds[m.feedIndex].category)
	}
	group = clamp(group+direction, 1+len(categories))
	if group == 0 {
		m.selectView(unreadView, m.feedIndex)
		return
	}
	for _, index := range m.feedOrder() {
		if m.feeds[index].category == categories[group-1] {
			m.selectView(feedView, index)
			return
		}
	}
}

func (m *Model) moveWithinScreen(key string) {
	if !m.reading {
		return
	}
	_, _, height := m.dimensions()
	lines := m.readerLines(m.readerWidth())
	first := m.scroll
	last := min(len(lines)-1, first+max(1, height-4)-1)
	switch key {
	case "H":
		m.line = first
	case "M":
		m.line = first + (last-first)/2
	case "L":
		m.line = last
	}
	m.ensureLineVisible()
}
