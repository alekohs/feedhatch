package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func click(m Model, x, y int) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return updated.(Model), cmd
}

func wheel(m Model, x, y int, down bool) Model {
	button := tea.MouseWheelUp
	if down {
		button = tea.MouseWheelDown
	}
	return update(m, tea.MouseWheelMsg{X: x, Y: y, Button: button})
}

// sidebarY is the screen row of sidebar entry i.
func sidebarY(m Model, i int) int {
	_, selected := m.sidebarEntries()
	_, _, height := m.dimensions()
	return 3 + i - sidebarStart(selected, height-4)
}

func TestClickSelectsFeedsAndArticles(t *testing.T) {
	m := sizedModel(t)
	entries, _ := m.sidebarEntries()
	feedRow := -1
	for i, e := range entries {
		if e.view == feedView && e.feed >= 0 && feedRow < 0 && !e.gap && !e.header {
			feedRow = i
		}
	}
	m, _ = click(m, 3, sidebarY(m, feedRow))
	if m.view != feedView || m.feedIndex != entries[feedRow].feed || m.focus != feedsPanel {
		t.Fatalf("clicking a feed should select it: view=%d feed=%d", m.view, m.feedIndex)
	}
	m, _ = click(m, 3, sidebarY(m, feedRow-1)) // its category header
	if m.feedIndex != entries[feedRow].feed {
		t.Fatal("clicking a category header should do nothing")
	}

	left, _, _ := m.dimensions()
	m, _ = click(m, left+6, 4) // the second article
	if m.articleIndex != 1 || m.focus != articlesPanel || m.reading {
		t.Fatalf("clicking an article should select it: index=%d reading=%v", m.articleIndex, m.reading)
	}
	m, _ = click(m, left+6, 4)
	if !m.reading {
		t.Fatal("clicking the selected article should open it")
	}
}

func TestDragOverTheReaderSelectsAndCopiesLines(t *testing.T) {
	m := motionReader(t)
	left, _, _ := m.dimensions()

	m, _ = click(m, left+10, 6)
	if m.line != 3 || m.visual {
		t.Fatalf("a click in the text should move the cursor there: line=%d", m.line)
	}
	m = update(m, tea.MouseMotionMsg{X: left + 10, Y: 8, Button: tea.MouseLeft})
	if !m.visual || m.anchor != 3 || m.line != 5 {
		t.Fatalf("dragging should select lines 3–5: anchor=%d line=%d", m.anchor, m.line)
	}
	updated, cmd := m.Update(tea.MouseReleaseMsg{X: left + 10, Y: 8})
	m = updated.(Model)
	if cmd == nil || m.visual || m.status != "Kopierade 3 rader till urklipp." {
		t.Fatalf("releasing should copy the selection: status %q", m.status)
	}

	// A click without dragging copies nothing.
	m, _ = click(m, left+10, 6)
	updated, cmd = m.Update(tea.MouseReleaseMsg{X: left + 10, Y: 6})
	if cmd != nil {
		t.Fatal("a plain click should not copy")
	}
	m = updated.(Model)

	// Dragging past the bottom edge scrolls the reader.
	m, _ = click(m, left+10, 6)
	for range 20 {
		m = update(m, tea.MouseMotionMsg{X: left + 10, Y: m.height, Button: tea.MouseLeft})
	}
	if m.scroll == 0 || !m.visual {
		t.Fatalf("dragging below the text should scroll and keep selecting: scroll=%d", m.scroll)
	}
}

func TestClickingTheStatusLineCopiesIt(t *testing.T) {
	m := sizedModel(t)
	m.status, m.tone = "Kunde inte hämta flödet: timeout", toneErr
	m, cmd := click(m, 10, m.height-1)
	if cmd == nil || !strings.Contains(m.status, "Kopierade") {
		t.Fatalf("clicking the status line should copy the message, status %q", m.status)
	}
}

func TestWheelScrollsWhatIsUnderTheMouse(t *testing.T) {
	m := motionReader(t)
	left, _, _ := m.dimensions()
	m = wheel(m, left+10, 10, true)
	if m.line != wheelStep {
		t.Fatalf("the wheel over the reader should move %d lines, line=%d", wheelStep, m.line)
	}
	m = wheel(m, left+10, 10, false)
	if m.line != 0 {
		t.Fatal("wheel up should move back")
	}

	m = sizedModel(t)
	m = wheel(m, left+10, 10, true)
	if m.articleIndex != 1 {
		t.Fatalf("the wheel over the article list should move the selection, index=%d", m.articleIndex)
	}
	m = press(m, "?")
	m.height = 14
	m = wheel(m, left+10, 5, true)
	if m.helpScroll != wheelStep {
		t.Fatalf("the wheel over help should scroll it, helpScroll=%d", m.helpScroll)
	}
}
