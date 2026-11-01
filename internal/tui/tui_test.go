package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/store"
)

func sizedModel(t *testing.T) Model {
	t.Helper()
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SeedMock(); err != nil {
		t.Fatal(err)
	}
	m, err := New(st, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 28
	return m
}

func press(m Model, keys ...string) Model {
	for _, key := range keys {
		msg := keyMessage(key)
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	return m
}

func keyMessage(key string) tea.KeyPressMsg {
	codes := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab,
		"backspace": tea.KeyBackspace,
		"pgdown":    tea.KeyPgDown, "pgup": tea.KeyPgUp, "space": tea.KeySpace,
	}
	if code, ok := codes[key]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	if strings.HasPrefix(key, "ctrl+") {
		return tea.KeyPressMsg{Code: []rune(strings.TrimPrefix(key, "ctrl+"))[0], Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
}

func paste(m Model, text string) Model {
	updated, _ := m.Update(tea.PasteMsg{Content: text})
	return updated.(Model)
}

func TestNavigationAndReadStatus(t *testing.T) {
	m := sizedModel(t)
	before := unreadCount(m.feeds[0])
	m = press(m, "l")
	if m.focus != articlesPanel || m.reading {
		t.Fatal("l should focus the article list without opening an article")
	}
	if unreadCount(m.feeds[0]) != before {
		t.Fatal("focusing a feed must not mark its articles read")
	}
	m = press(m, "enter")
	if !m.reading || unreadCount(m.feeds[0]) != before-1 {
		t.Fatal("opening an article should enter the reader and mark it read")
	}
	m = press(m, "esc", "h", "j")
	if m.focus != feedsPanel || m.feedIndex != 1 || m.reading {
		t.Fatal("expected to return to feeds and select the next feed")
	}
	m = press(m, "G", "j")
	if m.feedIndex != len(m.feeds)-1 {
		t.Fatal("feed navigation should stop at the last feed")
	}
	m = press(m, "g", "k")
	if m.feedIndex != 0 {
		t.Fatal("feed navigation should stop at the first feed")
	}
}

func TestVisualSelectionAndYank(t *testing.T) {
	m := press(sizedModel(t), "l", "enter", "j", "v", "j", "j")
	_, right, _ := m.dimensions()
	lines := m.readerLines(right - 6)
	text, count := m.selectedText()
	if count != 3 || text != strings.Join(lines[1:4], "\n") {
		t.Fatalf("selection = %q (%d rows), expected reader rows 1–3", text, count)
	}
	updated, cmd := m.Update(keyMessage("y"))
	m = updated.(Model)
	if cmd == nil || m.visual || !m.reading || m.status == "" {
		t.Fatal("y should emit a clipboard command, leave visual mode and stay in the reader")
	}
	text, count = m.selectedText()
	if count != 1 || text != lines[m.line] {
		t.Fatal("without a selection y should copy the current row")
	}
}

func TestVisualSelectionBackwardsAndEscape(t *testing.T) {
	m := press(sizedModel(t), "l", "enter", "j", "j", "v", "k", "k")
	start, end := m.selectionRange()
	if start != 0 || end != 2 {
		t.Fatalf("backwards selection = [%d, %d], expected [0, 2]", start, end)
	}
	m = press(m, "esc")
	if m.visual || !m.reading {
		t.Fatal("first escape should only cancel selection")
	}
	m = press(m, "esc")
	if m.reading || m.focus != articlesPanel {
		t.Fatal("second escape should return to the article list")
	}
}

func TestReaderScrollingAndResize(t *testing.T) {
	m := sizedModel(t)
	m.width, m.height = 60, 18
	m = press(m, "l", "enter", "v", "G")
	if m.scroll == 0 || m.scroll != m.maxScroll() {
		t.Fatal("moving to the last row should scroll to the bottom")
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m = updated.(Model)
	if m.visual || m.scroll > m.maxScroll() {
		t.Fatal("resizing should clear wrapped-row selection and clamp scrolling")
	}
	m = press(m, "g")
	if m.line != 0 || m.scroll != 0 {
		t.Fatal("g should return to the first row")
	}
}

func TestAddFeedAndValidation(t *testing.T) {
	m := sizedModel(t)
	initial := len(m.feeds)
	m = press(m, "a")
	m = paste(m, "not a URL")
	m = press(m, "enter", "enter", "enter", "enter")
	if m.form == nil || m.form.err == "" || len(m.feeds) != initial {
		t.Fatal("invalid URLs must not create a subscription")
	}
	m = press(m, "ctrl+u")
	m = paste(m, "https://example.org/rss")
	m = press(m, "enter")
	m = paste(m, "Mitt flöde")
	m = press(m, "enter", "ctrl+u")
	m = paste(m, "Läsning")
	m = press(m, "enter", "ctrl+u")
	m = paste(m, "0")
	m = press(m, "enter")
	if m.form == nil || m.form.field != 3 || m.form.err == "" {
		t.Fatal("an interval below 1 minute must be rejected")
	}
	m = press(m, "ctrl+u")
	m = paste(m, "30")
	m = press(m, "enter")
	if m.form != nil || len(m.feeds) != initial+1 {
		t.Fatal("valid form should create a feed and close")
	}
	feed := m.feeds[m.feedIndex]
	if feed.name != "Mitt flöde" || feed.category != "Läsning" || feed.interval != 30 || len(feed.articles) != 0 {
		t.Fatalf("unexpected new feed: %+v", feed)
	}
	m = press(m, "l", "enter", "j", "m", "v", "y")
	if m.reading || m.visual {
		t.Fatal("a new feed with no articles should stay in its empty state")
	}
}

func TestCategoryAndGroupedNavigation(t *testing.T) {
	m := press(sizedModel(t), "c", "ctrl+u")
	m = paste(m, "Design")
	m = press(m, "enter")
	if m.feeds[0].category != "Design" {
		t.Fatal("category should be editable")
	}
	order := m.feedOrder()
	// Arch Linux and Fältanteckningar now share the first displayed category.
	if order[0] != 0 || order[1] != 3 {
		t.Fatalf("unexpected grouped order: %v", order)
	}
	m = press(m, "j")
	if m.feedIndex != 3 {
		t.Fatal("navigation must follow displayed category order")
	}
}

func TestFormHandlesUnicodeAndDoesNotRunShortcuts(t *testing.T) {
	m := press(sizedModel(t), "a", "enter")
	m = paste(m, "Åäö")
	m = press(m, "backspace", "q", "t", "v")
	if m.form.values[1] != "Åäqtv" || m.terminalTheme || m.visual {
		t.Fatalf("form input triggered a shortcut or damaged Unicode: %+v", m.form)
	}
	m = paste(m, "\x1b\n\t")
	if strings.ContainsAny(m.form.values[1], "\x1b\n\t") {
		t.Fatal("pasted terminal controls must be removed")
	}
	m = press(m, "esc")
	if m.form != nil {
		t.Fatal("escape should cancel the form")
	}
}

func TestTerminalThemeInheritsTerminalDefaults(t *testing.T) {
	m := press(sizedModel(t), "t")
	v := m.View()
	if v.BackgroundColor != nil || v.ForegroundColor != nil || !m.terminalTheme {
		t.Fatal("terminal theme must inherit terminal foreground and background")
	}
	m = press(m, "t")
	if m.View().BackgroundColor == nil || m.terminalTheme {
		t.Fatal("base theme should provide its own background")
	}
}

func TestUnreadBadgeStaysOnFeedRow(t *testing.T) {
	m := sizedModel(t)
	for _, width := range []int{60, 80, 100, 160} {
		m.width = width
		for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if strings.Contains(line, "[2]") && !strings.Contains(line, "Arch Linux") {
				t.Fatalf("unread count left its feed row at width %d: %q", width, line)
			}
		}
		if !strings.Contains(ansi.Strip(m.View().Content), "[2]") {
			t.Fatalf("unread badge missing at width %d", width)
		}
	}
}

func TestSmallTerminalKeepsActiveFormFieldVisible(t *testing.T) {
	m := sizedModel(t)
	m.width, m.height = 60, 14
	m = press(m, "a", "tab", "tab", "ctrl+u")
	m = paste(m, "Min kategori")
	if !strings.Contains(ansi.Strip(m.View().Content), "> Min kategori") {
		t.Fatalf("the active category input must remain visible in a small terminal:\n%s", ansi.Strip(m.View().Content))
	}
}

func TestSmallTerminalHelpCanScroll(t *testing.T) {
	m := sizedModel(t)
	m.width, m.height = 60, 14
	m = press(m, "?", "G")
	if !strings.Contains(ansi.Strip(m.View().Content), "OSC52") {
		t.Fatal("help should scroll to its last line")
	}
	m = press(m, "g")
	if m.helpScroll != 0 {
		t.Fatal("g should return to the top of help")
	}
}

func TestViewsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{100, 28}, {60, 14}, {80, 24}, {160, 48}, {30, 8}, {1, 1}} {
		for _, state := range []string{"feeds", "reader", "visual", "form", "category", "help", "terminal", "search", "search-results"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], state), func(t *testing.T) {
				m := sizedModel(t)
				m.width, m.height = size[0], size[1]
				switch state {
				case "reader":
					m = press(m, "l", "enter")
				case "visual":
					m = press(m, "l", "enter", "v", "j")
				case "form":
					m = press(m, "a")
				case "category":
					m = press(m, "c")
				case "help":
					m = press(m, "?")
				case "terminal":
					m = press(m, "t")
				case "search":
					m = press(m, "l", "enter", "/")
					m = paste(m, strings.Repeat("Long query ", 30))
				case "search-results":
					m = searchFor(press(m, "l", "enter"), "artikel")
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

func TestChangeInterval(t *testing.T) {
	m := press(sizedModel(t), "i")
	if m.form == nil || m.form.values[3] != "15" {
		t.Fatal("i should open the interval form with the current value")
	}
	m = press(m, "ctrl+u")
	m = paste(m, "abc")
	m = press(m, "enter")
	if m.form == nil || m.form.err == "" {
		t.Fatal("a non-numeric interval must be rejected")
	}
	m = press(m, "ctrl+u")
	m = paste(m, "60")
	m = press(m, "enter")
	if m.form != nil || m.feeds[m.feedIndex].interval != 60 {
		t.Fatal("a valid interval should be saved")
	}

	feeds, err := m.store.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if feeds[m.feedIndex].Interval != 60 {
		t.Fatalf("stored interval = %d, want 60", feeds[m.feedIndex].Interval)
	}
}

func TestFetchedMsgReloadsAndKeepsSelection(t *testing.T) {
	m := press(sizedModel(t), "l", "j")
	feedID, articleID := m.feeds[m.feedIndex].id, m.currentArticle().id

	_, err := m.store.SaveArticles(feedID, []store.Article{{GUID: "new", Title: "Ny artikel", PublishedAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(FetchedMsg{})
	m = updated.(Model)

	if m.feeds[m.feedIndex].articles[0].title != "Ny artikel" {
		t.Fatal("the new article should be loaded first")
	}
	if m.currentArticle().id != articleID {
		t.Fatal("the selected article should stay selected after a reload")
	}

	updated, _ = m.Update(FetchedMsg{Err: fmt.Errorf("a: down\nb: down")})
	if status := updated.(Model).status; !strings.Contains(status, "a: down; b: down") {
		t.Fatalf("status = %q, want both errors on one line", status)
	}
}

func TestSavedChangesAreAnnounced(t *testing.T) {
	m := sizedModel(t)
	var events []event.Event
	m.announce = func(e event.Event) { events = append(events, e) }

	m = press(m, "l", "enter", "h", "m") // open (read), then toggle back to unread
	m = press(m, "c", "ctrl+u")
	m = paste(m, "Ny kategori")
	m = press(m, "enter", "i", "ctrl+u")
	m = paste(m, "45")
	m = press(m, "enter", "a")
	m = paste(m, "https://example.org/rss")
	m = press(m, "enter", "enter", "enter", "enter")

	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	want := []string{"read", "unread", "category", "interval", "feed_added"}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("announced %v, want %v", types, want)
	}
	if events[0].Feed != "Arch Linux" || events[0].Article == "" || events[0].ArticleID == 0 {
		t.Fatalf("read event = %+v, want feed and article", events[0])
	}
	if events[2].Category != "Ny kategori" || events[3].Interval != 45 || events[4].Feed != "example.org" {
		t.Fatalf("unexpected events: %+v", events[2:])
	}
}

func TestDeleteFeedNeedsConfirmation(t *testing.T) {
	m := sizedModel(t)
	var events []event.Event
	m.announce = func(e event.Event) { events = append(events, e) }
	initial := len(m.feeds)
	name := m.feeds[m.feedIndex].name

	m = press(m, "D", "n")
	if len(m.feeds) != initial || m.pendingDelete {
		t.Fatal("any key other than y should cancel the deletion")
	}

	m = press(m, "D", "y")
	if len(m.feeds) != initial-1 {
		t.Fatalf("got %d feeds, want %d", len(m.feeds), initial-1)
	}
	for _, feed := range m.feeds {
		if feed.name == name {
			t.Fatalf("%s should be deleted", name)
		}
	}
	if len(events) != 1 || events[0].Type != "feed_deleted" || events[0].Feed != name {
		t.Fatalf("events = %+v, want one feed_deleted for %s", events, name)
	}

	stored, err := m.store.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != initial-1 {
		t.Fatalf("store has %d feeds, want %d", len(stored), initial-1)
	}
}

func TestMarkFeedRead(t *testing.T) {
	m := sizedModel(t)
	var events []event.Event
	m.announce = func(e event.Event) { events = append(events, e) }
	feed := m.feeds[m.feedIndex]
	if unreadCount(feed) == 0 {
		t.Fatal("the first mock feed should start with unread articles")
	}

	m = press(m, "A")
	if unreadCount(m.feeds[m.feedIndex]) != 0 {
		t.Fatal("A should mark every article in the feed as read")
	}
	if len(events) != 1 || events[0].Type != "feed_read" || events[0].Feed != feed.name {
		t.Fatalf("events = %+v, want one feed_read for %s", events, feed.name)
	}

	articles, err := m.store.Articles(feed.id)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range articles {
		if !a.Read {
			t.Fatalf("%q should be stored as read", a.Title)
		}
	}

	m = press(m, "A")
	if len(events) != 1 || !strings.Contains(m.status, "Inga olästa") {
		t.Fatalf("a feed without unread articles should not announce again, status = %q", m.status)
	}
}
