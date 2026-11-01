package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func update(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestResizeKeysMoveTheFocusedBorder(t *testing.T) {
	m := sizedModel(t)
	m = press(m, "alt+1")
	left, _, _ := m.dimensions()

	m = press(m, ">", ">")
	if got, _, _ := m.dimensions(); got != left+2*resizeStep {
		t.Fatalf("sidebar = %d, want %d after > >", got, left+2*resizeStep)
	}
	m = press(m, "<")
	if got, _, _ := m.dimensions(); got != left+resizeStep {
		t.Fatalf("sidebar = %d, want %d after <", got, left+resizeStep)
	}

	m.width, m.height = 160, 30
	m = press(m, "alt+2")
	list, _ := m.columns()
	sidebar, _, _ := m.dimensions()
	m = press(m, ">")
	if got, _ := m.columns(); got != list+resizeStep {
		t.Fatalf("list = %d, want %d: in three columns > moves the list/reader border", got, list+resizeStep)
	}
	if got, _, _ := m.dimensions(); got != sidebar {
		t.Fatal("the sidebar should not change while the list has focus")
	}

	m = press(m, "=")
	if m.sidebarWidth != 0 || m.listWidth != 0 {
		t.Fatal("= should reset both widths")
	}
}

func TestResizeStaysWithinLimits(t *testing.T) {
	m := sizedModel(t)
	m = press(m, "alt+1")
	for range 50 {
		m = press(m, "<")
	}
	if left, _, _ := m.dimensions(); left != minSidebar {
		t.Fatalf("sidebar = %d, want the minimum %d", left, minSidebar)
	}
	for range 100 {
		m = press(m, ">")
	}
	if _, right, _ := m.dimensions(); right != minRightPanel {
		t.Fatalf("right panel = %d, want the minimum %d", right, minRightPanel)
	}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Fatalf("line width %d exceeds %d", w, m.width)
		}
	}
}

func TestDragSidebarBorderWithMouse(t *testing.T) {
	m := sizedModel(t)
	left, _, _ := m.dimensions()

	m = update(m, tea.MouseClickMsg{X: left - 1, Y: 5, Button: tea.MouseLeft})
	m = update(m, tea.MouseMotionMsg{X: 40, Y: 5, Button: tea.MouseLeft})
	m = update(m, tea.MouseReleaseMsg{X: 40, Y: 5, Button: tea.MouseLeft})
	if got, _, _ := m.dimensions(); got != 41 {
		t.Fatalf("sidebar = %d, want 41 after dragging its border to column 40", got)
	}

	m = update(m, tea.MouseMotionMsg{X: 30, Y: 5})
	if got, _, _ := m.dimensions(); got != 41 {
		t.Fatal("moving the mouse after releasing must not resize")
	}

	m = update(m, tea.MouseClickMsg{X: 60, Y: 5, Button: tea.MouseLeft})
	m = update(m, tea.MouseMotionMsg{X: 70, Y: 5, Button: tea.MouseLeft})
	if got, _, _ := m.dimensions(); got != 41 {
		t.Fatal("dragging inside a panel must not resize")
	}
}

func TestDragListBorderInThreeColumns(t *testing.T) {
	m := sizedModel(t)
	m.width, m.height = 160, 30
	left, _, _ := m.dimensions()
	list, _ := m.columns()

	m = update(m, tea.MouseClickMsg{X: left + list, Y: 3, Button: tea.MouseLeft})
	m = update(m, tea.MouseMotionMsg{X: left + 60, Y: 3, Button: tea.MouseLeft})
	if got, _ := m.columns(); got != 61 {
		t.Fatalf("list = %d, want 61 after dragging its border", got)
	}

	m.width = 100 // two panels: there is no list/reader border to grab
	if m.borderAt(left+60, 3) == listBorder {
		t.Fatal("the list border only exists in three columns")
	}
}

func TestReaderTextUsesTheWholeColumn(t *testing.T) {
	m := sizedModel(t)
	m.width, m.height = 240, 40
	if _, reader := m.columns(); m.readerWidth() != reader-6 {
		t.Fatalf("reader text width = %d, want the whole reader column (%d)", m.readerWidth(), reader-6)
	}
	m.width = 100
	if _, right, _ := m.dimensions(); m.readerWidth() != right-6 {
		t.Fatal("a two-panel reader should use the whole right panel")
	}
}

func TestLinkNumbersStayWithTheirWord(t *testing.T) {
	m := motionReader(t)
	m.feeds[0].articles[0].body = strings.Repeat("word ", 13) + "TrustWave [5]) and more text"
	m = press(m, "esc", "enter")

	for _, line := range m.readerLines(m.readerWidth()) {
		if strings.HasPrefix(strings.TrimSpace(line), "[5]") {
			t.Fatalf("the link number was wrapped away from its word: %q", line)
		}
	}
	m = searchFor(m, "TrustWave [5]")
	if len(m.matchingRows()) == 0 {
		t.Fatal("search should treat the non-breaking space as a normal space")
	}
}
