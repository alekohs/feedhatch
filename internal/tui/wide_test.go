package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func wideModel(t *testing.T) Model {
	t.Helper()
	m := feedModel(t)
	m.width, m.height = 160, 30
	return m
}

func wideReader(t *testing.T, width int) Model {
	t.Helper()
	m := feedModel(t)
	m.width, m.height = width, 30
	var body []string
	for i := range 60 {
		body = append(body, fmt.Sprintf("Row %02d needle with some longer text to wrap around in narrow columns %02d", i, i))
	}
	m.feeds = []subscription{{name: "Test feed", articles: []article{
		{title: "Title", date: "Today", body: strings.Join(body, "\n"), unread: true},
		{title: "Other", date: "Today", body: "Other body", unread: true},
	}}}
	return press(m, "l", "enter")
}

func panelCount(m Model) int {
	return strings.Count(ansi.Strip(strings.Split(m.View().Content, "\n")[1]), "┌")
}

func TestWideShowsThreePanels(t *testing.T) {
	m := wideModel(t)
	content := ansi.Strip(m.View().Content)
	for _, want := range []string{"1 Flöden", m.listTitle(), "3 Läsning · "} {
		if !strings.Contains(content, want) {
			t.Fatalf("wide view lacks %q", want)
		}
	}
	if panelCount(m) != 3 {
		t.Fatalf("panels = %d, expected 3", panelCount(m))
	}
	m.width = 139
	if panelCount(m) != 2 || strings.Contains(ansi.Strip(m.View().Content), "3 Läsning") {
		t.Fatal("139 columns should keep two panels")
	}
}

func TestWidePreviewDoesNotMarkRead(t *testing.T) {
	m := press(wideModel(t), "l")
	unread := m.totalUnread()
	first := m.currentArticle().title
	m = press(m, "j", "j", "G", "g", "d", "J", "K")
	if m.reading || m.totalUnread() != unread {
		t.Fatal("moving in the list must not mark articles read or open the reader")
	}
	check := m
	if err := check.load(); err != nil || check.totalUnread() != unread {
		t.Fatal("the store must be untouched by previewing")
	}
	m = press(m, "j")
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, m.currentArticle().title) || m.currentArticle().title == first {
		t.Fatal("the preview should follow the list cursor")
	}
	if !strings.Contains(content, fmt.Sprintf("3 Läsning · %s %d/", m.currentFeed().name, m.articleIndex+1)) {
		t.Fatal("the preview title should show the position")
	}
}

func TestWideEnterReadsAndHReturns(t *testing.T) {
	m := press(wideModel(t), "l")
	unread := m.totalUnread()
	m = press(m, "enter")
	if !m.reading || m.totalUnread() != unread-1 {
		t.Fatal("enter should mark the article read and focus the reader")
	}
	check := m
	if err := check.load(); err != nil || check.totalUnread() != unread-1 {
		t.Fatal("opening should be saved in the store")
	}
	m = press(m, "h")
	if m.reading || m.focus != articlesPanel {
		t.Fatal("h should return focus to the list")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "3 Läsning · ") || panelCount(m) != 3 {
		t.Fatal("the preview should stay after leaving the reader")
	}
	m = press(m, "alt+3")
	if !m.reading {
		t.Fatal("3 should focus the reader")
	}
	m = press(m, "alt+2")
	if m.reading || m.focus != articlesPanel {
		t.Fatal("2 should focus the list in wide mode")
	}
}

func TestKeyThreeFocusesReader(t *testing.T) {
	for _, width := range []int{160, 100} {
		m := feedModel(t)
		m.width = width
		unread := m.totalUnread()
		m = press(m, "alt+3")
		if !m.reading || m.focus != articlesPanel || m.totalUnread() != unread-1 {
			t.Fatalf("3 should open the article at width %d", width)
		}
	}
}

func TestWideReaderSearchAndSelection(t *testing.T) {
	for _, width := range []int{160, 100} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := wideReader(t, width)
			lines := m.readerLines(m.readerWidth())
			m = searchFor(m, "needle")
			if !strings.Contains(lines[m.line], "needle") {
				t.Fatalf("search landed on %q", lines[m.line])
			}
			m = press(m, "n")
			if !strings.Contains(lines[m.line], "needle") {
				t.Fatalf("n landed on %q", lines[m.line])
			}
			m = press(m, "g", "v", "j", "j")
			text, count := m.selectedText()
			if count != 3 || text != strings.Join(lines[0:3], "\n") {
				t.Fatalf("selection = %q (%d rows)", text, count)
			}
			updated, cmd := m.Update(keyMessage("y"))
			if cmd == nil || updated.(Model).visual {
				t.Fatal("y should copy and leave visual mode")
			}
		})
	}
	wide, narrow := wideReader(t, 160), wideReader(t, 100)
	if wide.readerWidth() <= narrow.readerWidth() {
		t.Fatal("the wide reader should be wider than the narrow one")
	}
}

func TestWideResizeKeepsReading(t *testing.T) {
	m := wideReader(t, 160)
	m = press(m, "G")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	if !m.reading || m.line != len(m.readerLines(m.readerWidth()))-1 || m.scroll > m.maxScroll() {
		t.Fatal("resizing should keep reading and clamp the line")
	}
	content := ansi.Strip(m.View().Content)
	if panelCount(m) != 2 || !strings.Contains(content, "2 Läsning · ") {
		t.Fatal("narrow reader should render two panels")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = updated.(Model)
	if !m.reading || panelCount(m) != 3 || m.line >= len(m.readerLines(m.readerWidth())) {
		t.Fatal("resizing back should keep reading")
	}
}

func TestWidePreviewWithEmptyList(t *testing.T) {
	m := press(wideModel(t), "l", "/")
	m = paste(m, "zzzz no such text")
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "Välj en artikel.") || !strings.Contains(content, "3 Läsning") {
		t.Fatal("an empty list should show the placeholder preview")
	}
	m = press(m, "enter", "alt+3")
	if m.reading {
		t.Fatal("3 without an article must not open the reader")
	}
	m.feeds = nil
	_ = m.View()
}

func TestWideViewsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{140, 24}, {200, 40}} {
		for _, state := range []string{"list", "reader", "visual", "help", "form"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], state), func(t *testing.T) {
				m := sizedModel(t)
				m.width, m.height = size[0], size[1]
				switch state {
				case "reader":
					m = press(m, "l", "enter")
				case "visual":
					m = press(m, "l", "enter", "v", "j")
				case "help":
					m = press(m, "?")
				case "form":
					m = press(m, "a")
				}
				content := m.View().Content
				if lipgloss.Height(content) > m.height {
					t.Fatalf("view height %d exceeds %d", lipgloss.Height(content), m.height)
				}
				for _, line := range strings.Split(content, "\n") {
					if lipgloss.Width(line) > m.width {
						t.Fatalf("row width %d exceeds %d: %q", lipgloss.Width(line), m.width, ansi.Strip(line))
					}
				}
			})
		}
	}
}

func TestWideListDropsFeedColumn(t *testing.T) {
	m := sizedModel(t)
	m.view = allView

	m.width, m.height = 160, 30
	list, _ := m.columns()
	if strings.Contains(m.articleList(list-4, 20), "Hacker News") {
		t.Fatal("the wide list should leave the feed name to the preview")
	}

	m.width = 100
	_, right, _ := m.dimensions()
	if !strings.Contains(m.articleList(right-4, 20), "Hacker News") {
		t.Fatal("the two-panel list should still show the feed column")
	}
}
