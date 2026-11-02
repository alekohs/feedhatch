package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func plainLines(s string) []string { return strings.Split(ansi.Strip(s), "\n") }

func selectedRows(lines []string) []string {
	var rows []string
	for _, line := range lines {
		if strings.HasPrefix(line, ">") {
			rows = append(rows, line)
		}
	}
	return rows
}

func TestSidebarShowsVirtualEntriesAndHighlightsView(t *testing.T) {
	m := sizedModel(t)
	for _, tc := range []struct {
		view listView
		want string
	}{
		{unreadView, "Olästa"},
		{allView, "Alla artiklar"},
		{feedView, "Arch Linux"},
	} {
		m.view, m.keep = tc.view, nil
		lines := plainLines(m.sidebar(26, 20))
		if !strings.Contains(lines[0], "Olästa") || !strings.Contains(lines[0], "[5]") {
			t.Fatalf("first row should be Olästa [5], got %q", lines[0])
		}
		if !strings.Contains(lines[1], "Alla artiklar") || !strings.Contains(lines[1], "[8]") {
			t.Fatalf("second row should be Alla artiklar [8], got %q", lines[1])
		}
		if !strings.Contains(lines[0], "●") || strings.Contains(lines[1], "●") {
			t.Fatalf("only Olästa should have an unread dot, got %q and %q", lines[0], lines[1])
		}
		if strings.TrimSpace(lines[2]) != "" {
			t.Fatalf("a blank line should follow the virtual entries, got %q", lines[2])
		}
		rows := selectedRows(lines)
		if len(rows) != 1 || !strings.Contains(rows[0], tc.want) {
			t.Fatalf("view %d: want only %q selected, got %q", tc.view, tc.want, rows)
		}
	}
}

func TestSidebarUnreadCountIsLiveNotKeepSize(t *testing.T) {
	m := sizedModel(t)
	m.view = unreadView
	m.keep = map[int64]bool{m.feeds[0].articles[2].id: true}
	if first := plainLines(m.sidebar(26, 20))[0]; !strings.Contains(first, "[5]") {
		t.Fatalf("unread count must be live, got %q", first)
	}
}

func TestSidebarCategoryHeadersAreUppercase(t *testing.T) {
	m := sizedModel(t)
	out := ansi.Strip(m.sidebar(26, 20))
	for _, header := range []string{"TEKNIK", "UTVECKLING", "DESIGN"} {
		if !strings.Contains(out, header) {
			t.Fatalf("missing category header %s in %q", header, out)
		}
	}
}

func TestArticleListOneRowPerArticle(t *testing.T) {
	m := sizedModel(t)
	m.view = feedView
	lines := plainLines(m.articleList(70, 12))
	if len(lines) != len(m.feeds[0].articles) {
		t.Fatalf("want one row per article, got %d rows: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "Nyheter från pakethyllan") || !strings.Contains(lines[0], "●") {
		t.Fatalf("unread article row incomplete: %q", lines[0])
	}
	if strings.Contains(lines[0], "Arch Linux") {
		t.Fatalf("feed column must not appear in a feed list: %q", lines[0])
	}

	m.view = allView
	lines = plainLines(m.articleList(70, 12))
	if len(lines) != 8 {
		t.Fatalf("want 8 rows in the all list, got %d", len(lines))
	}
	for _, line := range lines {
		if !strings.Contains(line, "Arch Linux") && !strings.Contains(line, "Hacker News") &&
			!strings.Contains(line, "The Go Blog") && !strings.Contains(line, "Fältanteckningar") {
			t.Fatalf("virtual list row lacks feed column: %q", line)
		}
	}
}

func TestArticleListKeepsDateAndCursorVisible(t *testing.T) {
	m := sizedModel(t)
	m.view = allView
	m.articleIndex = 7
	lines := plainLines(m.articleList(40, 3))
	if len(lines) != 3 || !strings.HasPrefix(lines[2], ">") {
		t.Fatalf("cursor row should be last visible row: %q", lines)
	}
	for _, line := range lines {
		if !strings.Contains(line, " · ") || ansi.StringWidth(line) > 40 {
			t.Fatalf("date must survive truncation within width: %q", line)
		}
	}
}

func TestVirtualListKeepsTitlesAtNarrowWidth(t *testing.T) {
	for _, view := range []listView{unreadView, allView} {
		m := sizedModel(t)
		m.view, m.keep = view, nil
		m.width, m.height = 60, 20
		refs := m.articleRefs()
		lines := plainLines(m.articleList(60-24-4, 20-3-4))
		if len(lines) != len(refs) {
			t.Fatalf("view %d: want %d rows, got %d", view, len(refs), len(lines))
		}
		for i, line := range lines {
			title := []rune(m.feeds[refs[i].feed].articles[refs[i].article].title)[:4]
			if !strings.Contains(line, string(title)) {
				t.Fatalf("view %d: row lacks the start of its title: %q", view, line)
			}
			if ansi.StringWidth(line) != 32 {
				t.Fatalf("view %d: row must fill the panel width, got %q", view, line)
			}
		}
	}

	m := sizedModel(t)
	m.view = allView
	for _, line := range plainLines(m.articleList(96, 10)) {
		if !strings.Contains(line, "Arch Linux") && !strings.Contains(line, "Hacker News") &&
			!strings.Contains(line, "The Go Blog") && !strings.Contains(line, "Fältanteckningar") {
			t.Fatalf("feed column should remain at 100 columns: %q", line)
		}
	}
}

func TestArticleListEmptyStates(t *testing.T) {
	m := sizedModel(t)
	m.feeds[0].articles = nil
	m.view = feedView
	if out := m.articleList(60, 10); !strings.Contains(out, "Nya flöden hämtas direkt") {
		t.Fatalf("real mode empty feed text: %q", out)
	}
	m.mock = true
	if out := m.articleList(60, 10); !strings.Contains(out, "mockläget") || strings.Contains(out, "Hämtas") {
		t.Fatalf("mock empty feed text: %q", out)
	}

	m = sizedModel(t)
	for i := range m.feeds {
		for j := range m.feeds[i].articles {
			m.feeds[i].articles[j].unread = false
		}
	}
	m.view, m.keep = unreadView, nil
	out := ansi.Strip(m.articleList(80, 10))
	if !strings.Contains(out, "Inga olästa artiklar.") || !strings.Contains(out, "{ } kategori · Alla artiklar finns ovanför flödena") {
		t.Fatalf("unread empty state: %q", out)
	}

	m.feeds, m.view = nil, allView
	if out := m.articleList(60, 10); out != "Lägg till ett flöde med a." {
		t.Fatalf("no feeds text: %q", out)
	}
}

func TestListAndReaderTitles(t *testing.T) {
	m := sizedModel(t)
	m.view, m.keep = feedView, nil
	if got := m.listTitle(); got != "2 Artiklar" {
		t.Fatalf("feed list title: %q", got)
	}
	if got := m.readerTitle(); got != "2 Läsning · Arch Linux 1/3" {
		t.Fatalf("feed reader title: %q", got)
	}
	m.view = unreadView
	if got := m.listTitle(); got != "2 Olästa · 5 kvar av 5" {
		t.Fatalf("unread list title: %q", got)
	}
	m.articleIndex = 1
	if got := m.readerTitle(); got != "2 Läsning · Olästa 2/5" {
		t.Fatalf("unread reader title: %q", got)
	}
	m.view = allView
	if got := m.listTitle(); got != "2 Alla artiklar · 8 från 4 flöden" {
		t.Fatalf("all list title: %q", got)
	}
	if got := m.readerTitle(); got != "2 Läsning · Alla artiklar 2/8" {
		t.Fatalf("all reader title: %q", got)
	}
}

func TestReaderLinesUseOwningFeedInVirtualList(t *testing.T) {
	m := sizedModel(t)
	m.view = allView
	for i, r := range m.articleRefs() {
		if m.feeds[r.feed].name == "The Go Blog" {
			m.articleIndex = i
			break
		}
	}
	if meta := m.readerLines(80)[1]; !strings.HasSuffix(meta, "The Go Blog") {
		t.Fatalf("reader should name the owning feed, got %q", meta)
	}
}
