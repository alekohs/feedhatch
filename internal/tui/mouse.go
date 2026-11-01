package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// wheelStep is how many reader lines one wheel notch scrolls.
const wheelStep = 3

// contentRow is the panel content row under y (the header, a border and the panel
// title come first), or -1 outside the panels' content.
func (m Model) contentRow(y int) int {
	_, _, height := m.dimensions()
	if r := y - 3; r >= 0 && r < height-4 {
		return r
	}
	return -1
}

// inList reports whether x is over the article list.
func (m Model) inList(x int) bool {
	left, _, _ := m.dimensions()
	if m.threeColumns() {
		list, _ := m.columns()
		return x >= left && x < left+list
	}
	return x >= left && !m.reading && m.form == nil && !m.help && !m.logOpen
}

// mouseClick handles a left click: a border starts a drag, a link opens, the status
// line is copied, rows are selected, and reader text moves the cursor and may start a
// selection by dragging.
func (m *Model) mouseClick(x, y int) tea.Cmd {
	if m.dragging = m.borderAt(x, y); m.dragging != noBorder || m.form != nil {
		return nil
	}
	if url := m.linkAt(x, y); url != "" {
		m.openLink(url)
		return nil
	}
	if y == m.height-1 && m.status != "" {
		text := m.status
		m.status, m.tone = "Kopierade meddelandet till urklipp.", toneOK
		return tea.SetClipboard(text)
	}
	r := m.contentRow(y)
	if r < 0 {
		return nil
	}
	left, _, height := m.dimensions()
	switch {
	case x < left:
		entries, selected := m.sidebarEntries()
		if i := sidebarStart(selected, height-4) + r; i < len(entries) && !entries[i].gap && !entries[i].header {
			m.focus = feedsPanel
			m.selectView(entries[i].view, max(0, entries[i].feed))
		}
	case m.inList(x):
		refs := m.articleRefs()
		i := max(0, m.articleIndex-(height-4)+1) + r
		if i >= len(refs) {
			return nil
		}
		// A click selects; a click on the selected article opens it.
		if i == m.articleIndex && m.focus == articlesPanel && !m.reading {
			m.openArticle()
			return nil
		}
		m.focus, m.articleIndex = articlesPanel, i
		if m.reading {
			m.reading, m.scroll, m.visual = false, 0, false
		}
	default:
		line, _, ok := m.readerLineAt(x, y)
		if !ok {
			return nil
		}
		if !m.reading {
			m.focus = articlesPanel
			m.openArticle()
		}
		m.line, m.anchor, m.visual, m.selecting = line, line, false, true
		m.ensureLineVisible()
	}
	return nil
}

// mouseMotion drags a border or extends a selection in the reader; dragging past the
// top or bottom edge scrolls.
func (m *Model) mouseMotion(x, y int) {
	switch {
	case m.dragging != noBorder:
		m.moveBorder(m.dragging, x)
	case m.selecting:
		if line, _, ok := m.readerLineAt(x, y); ok {
			m.line = line
			m.visual = line != m.anchor
			m.ensureLineVisible()
		}
	}
}

// mouseRelease ends a drag: a border's width is remembered, and selected reader lines
// are copied to the clipboard.
func (m *Model) mouseRelease() tea.Cmd {
	if m.dragging != noBorder {
		m.dragging = noBorder
		m.saveSettings()
	}
	if !m.selecting {
		return nil
	}
	m.selecting = false
	if !m.visual {
		return nil
	}
	text, count := m.selectedText()
	m.visual = false
	m.status, m.tone = "Kopierade "+plural(count, "rad", "rader")+" till urklipp.", toneOK
	return tea.SetClipboard(text)
}

// mouseWheel scrolls whatever is under the mouse.
func (m *Model) mouseWheel(x, y int, down bool) {
	dir := -1
	if down {
		dir = 1
	}
	left, _, _ := m.dimensions()
	switch {
	case m.contentRow(y) < 0 || m.form != nil:
	case m.help && x >= left:
		m.helpScroll = clamp(m.helpScroll+dir*wheelStep, m.maxHelpScroll()+1)
	case m.logOpen && x >= left:
		m.logScroll = clamp(m.logScroll+dir*wheelStep, m.maxLogScroll()+1)
	case x < left:
		m.focus = feedsPanel
		m.move(dir)
	case m.inList(x):
		m.articleIndex = clamp(m.articleIndex+dir, len(m.articleRefs()))
	case m.reading:
		m.move(dir * wheelStep)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
