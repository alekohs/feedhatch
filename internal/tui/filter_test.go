package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func listTitles(m Model) []string {
	var titles []string
	for _, r := range m.articleRefs() {
		titles = append(titles, m.feeds[r.feed].articles[r.article].title)
	}
	return titles
}

func allModel(t *testing.T) Model {
	t.Helper()
	m := sizedModel(t)
	m.selectView(allView, 0)
	m.focus = articlesPanel
	return m
}

func filterFor(m Model, query string) Model {
	return press(paste(press(m, "/"), query), "enter")
}

func TestFilterMatchesTitleAndBodyCaseInsensitive(t *testing.T) {
	m := filterFor(allModel(t), "GRÄNSSNITT")
	if got := listTitles(m); !slices.Equal(got, []string{"Vad gör ett gränssnitt behagligt?"}) {
		t.Fatalf("title match: %q", got)
	}
	m = filterFor(allModel(t), "RÖR")
	if got := listTitles(m); !slices.Equal(got, []string{"Ett litet underhållsfönster"}) {
		t.Fatalf("body-only match: %q", got)
	}
	m = filterFor(allModel(t), "[a.*]")
	if len(m.articleRefs()) != 0 {
		t.Fatal("regex punctuation must be matched literally")
	}
}

func TestFilterNarrowsWhileTyping(t *testing.T) {
	m := press(allModel(t), "/")
	if !m.filtering || m.focus != articlesPanel {
		t.Fatal("/ should open the filter input on the article panel")
	}
	all := len(m.articleRefs())
	m = press(m, "g")
	one := len(m.articleRefs())
	m = press(m, "o")
	two := len(m.articleRefs())
	if one >= all || two != 3 || m.filter != "go" {
		t.Fatalf("list should narrow per key: %d %d %d", all, one, two)
	}
	m = press(m, "backspace")
	if m.filter != "g" || len(m.articleRefs()) != one {
		t.Fatal("backspace should widen the list again")
	}
	m = paste(m, "åx")
	if m.filter != "gåx" || len(m.articleRefs()) != 0 {
		t.Fatal("paste should extend the filter")
	}
	m = press(m, "ctrl+u")
	if m.filter != "" || len(m.articleRefs()) != all || !m.filtering {
		t.Fatal("ctrl+u should clear the input and keep it open")
	}
}

func TestFilterFromSidebarMovesFocus(t *testing.T) {
	m := sizedModel(t)
	m.focus = feedsPanel
	if m = press(m, "/"); m.focus != articlesPanel || !m.filtering {
		t.Fatal("/ in the sidebar should focus the article panel and open the input")
	}
}

func TestFilterEnterKeepsAndEscClears(t *testing.T) {
	m := filterFor(allModel(t), "go")
	if m.filtering || m.filter != "go" || len(m.articleRefs()) != 3 {
		t.Fatal("enter should keep the filter and leave the input")
	}
	m = press(m, "j")
	if m.articleIndex != 1 {
		t.Fatal("normal keys should work with a kept filter")
	}
	m = press(m, "esc")
	if m.filter != "" || m.focus != articlesPanel || len(m.articleRefs()) != 8 {
		t.Fatal("esc in the list should clear the filter before leaving the panel")
	}
	m = press(m, "esc")
	if m.focus != feedsPanel {
		t.Fatal("a second esc should behave as before")
	}

	m = press(allModel(t), "/", "g", "o", "esc")
	if m.filtering || m.filter != "" || len(m.articleRefs()) != 8 {
		t.Fatal("esc in the input should clear the filter and leave it")
	}
}

func TestFilterEnterWithEmptyInputClears(t *testing.T) {
	m := filterFor(allModel(t), "go")
	m = press(m, "/", "ctrl+u", "enter")
	if m.filtering || m.filter != "" || len(m.articleRefs()) != 8 {
		t.Fatal("enter with an empty input should clear the filter")
	}
}

func TestFilterInputDoesNotRunShortcuts(t *testing.T) {
	m := press(allModel(t), "/")
	for _, key := range []string{"q", "D", "j", "m", "A", "r", "t", "?"} {
		updated, cmd := m.Update(keyMessage(key))
		if cmd != nil {
			t.Fatalf("%q must not run a command from the filter input", key)
		}
		m = updated.(Model)
	}
	if m.filter != "qDjmArt?" || m.pendingDelete || m.pendingReadAll || m.help || !m.terminalTheme || m.status != "" {
		t.Fatalf("shortcuts leaked out of the input: %+v", m.filter)
	}
	if _, cmd := m.Update(keyMessage("ctrl+c")); cmd == nil {
		t.Fatal("ctrl+c should still quit")
	}
}

func TestFilterKeepsCursorOnSameArticle(t *testing.T) {
	m := allModel(t)
	want := "Att tänka på före nästa uppdatering"
	for currentTitle(m) != want {
		m = press(m, "j")
	}
	m = press(m, "/")
	m = paste(m, "uppdatering")
	if currentTitle(m) != want || m.articleIndex != 1 {
		t.Fatalf("cursor should stay on %q, got %q at %d", want, currentTitle(m), m.articleIndex)
	}
	m = press(m, "esc")
	if currentTitle(m) != want {
		t.Fatal("clearing the filter should keep the cursor on the article")
	}
	m = paste(press(m, "/"), "verktyg")
	if currentTitle(m) != "Små verktyg som gör en sak bra" || m.articleIndex != 0 {
		t.Fatal("cursor should move to the first result when its article drops out")
	}
}

func TestFilterClearedBySidebarChange(t *testing.T) {
	m := filterFor(allModel(t), "go")
	m = press(m, "alt+1", "k")
	if m.view != unreadView || m.filter != "" {
		t.Fatal("changing the sidebar entry should clear the filter")
	}
	m = filterFor(m, "go")
	m = press(m, "alt+1", "j", "j")
	if m.view != feedView || m.filter != "" {
		t.Fatal("moving to a feed should clear the filter")
	}
}

func TestFilterClearedWhenStepCrossesFeeds(t *testing.T) {
	m := filterFor(feedModel(t), "pakethyllan")
	m.focus = articlesPanel
	if got := listTitles(m); len(got) != 1 {
		t.Fatalf("filtered feed list: %q", got)
	}
	first := m.feedIndex
	m = press(m, "]")
	if m.feedIndex == first || m.filter != "" || !m.reading {
		t.Fatal("] into another feed should clear the filter and open the article")
	}

	m = filterFor(feedModel(t), "uppdatering")
	m.focus = articlesPanel
	m = press(m, "]")
	if m.feedIndex != first || m.filter != "uppdatering" || currentTitle(m) != "Att tänka på före nästa uppdatering" {
		t.Fatal("] within the feed should stay inside the filtered list")
	}
}

func TestFilterSurvivesReload(t *testing.T) {
	m := filterFor(allModel(t), "go")
	m = fetched(m, nil)
	if m.filter != "go" || len(m.articleRefs()) != 3 {
		t.Fatal("FetchedMsg should keep the filter")
	}
	m = press(m, "r")
	if m.filter != "go" || len(m.articleRefs()) != 3 {
		t.Fatal("r should keep the filter")
	}
}

func TestFilterOpensArticleWithSearch(t *testing.T) {
	m := filterFor(allModel(t), "uppdatering")
	m = press(m, "enter")
	if !m.reading || m.search.query != "uppdatering" {
		t.Fatalf("opening from a filtered list should search for the filter, got %q", m.search.query)
	}
	m = press(m, "n")
	if !strings.Contains(m.status, "träff på rad") || m.line == 0 {
		t.Fatalf("n should jump to a match: line=%d status=%q", m.line, m.status)
	}
	if got := len(m.articleRefs()); got != 2 {
		t.Fatalf("reader should see the filtered list, got %d", got)
	}
	m = press(m, "J")
	if currentTitle(m) != "Att tänka på före nästa uppdatering" || m.search.query != "uppdatering" {
		t.Fatal("J should stay inside the filtered list and keep the search")
	}
}

func TestFilterInEveryView(t *testing.T) {
	m := filterFor(sizedModel(t), "uppdatering")
	if m.view != unreadView || len(m.articleRefs()) != 2 || !strings.HasPrefix(m.listTitle(), "2 Olästa · 2 träffar för") {
		t.Fatalf("unread view: %q", m.listTitle())
	}
	m = filterFor(feedModel(t), "uppdatering")
	if m.view != feedView || len(m.articleRefs()) != 2 || m.listTitle() != `2 Artiklar · 2 träffar för "uppdatering"` {
		t.Fatalf("feed view: %q", m.listTitle())
	}
	m = filterFor(allModel(t), "uppdatering")
	if len(m.articleRefs()) != 2 {
		t.Fatal("all view should filter too")
	}
}

func TestFilterRendering(t *testing.T) {
	m := filterFor(allModel(t), "go")
	if got := m.listTitle(); got != `2 Alla artiklar · 3 träffar för "go"` {
		t.Fatalf("title: %q", got)
	}
	if got := filterFor(allModel(t), "gränssnitt").listTitle(); got != `2 Alla artiklar · 1 träff för "gränssnitt"` {
		t.Fatalf("singular title: %q", got)
	}

	empty := filterFor(allModel(t), "zzz")
	text := viewText(empty)
	if !strings.Contains(text, `Inga träffar för "zzz".`) || !strings.Contains(text, "esc rensar sökningen") {
		t.Fatal("empty result should explain and hint")
	}

	typing := press(allModel(t), "/", "g", "o")
	lines := strings.Split(viewText(typing), "\n")
	footer := lines[len(lines)-2]
	if !strings.HasPrefix(footer, "/ go▏") || !strings.HasSuffix(strings.TrimRight(footer, " "), "enter klar  esc rensa") {
		t.Fatalf("footer: %q", footer)
	}
	if !strings.Contains(viewText(m), "? alla tangenter") {
		t.Fatal("list footer should keep the help hint")
	}
}

func TestFilterFooterFitsNarrowTerminal(t *testing.T) {
	for _, width := range []int{60, 80, 100} {
		m := press(allModel(t), "/")
		m.width = width
		m = paste(m, strings.Repeat("långtext ", 20))
		for _, line := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d: line overflows: %q", width, ansi.Strip(line))
			}
		}
		lines := strings.Split(viewText(m), "\n")
		if !strings.Contains(lines[len(lines)-2], "▏") {
			t.Fatalf("width %d: the cursor must stay visible", width)
		}
	}
}

func TestListFooterShowsFilterHint(t *testing.T) {
	m := sizedModel(t)
	lines := strings.Split(viewText(m), "\n")
	footer := lines[len(lines)-2]
	if !strings.Contains(footer, "/ sök") || !strings.Contains(footer, "? alla tangenter") {
		t.Fatalf("footer: %q", footer)
	}
	m.width = 70
	lines = strings.Split(viewText(m), "\n")
	if !strings.Contains(lines[len(lines)-2], "? alla tangenter") {
		t.Fatal("the help hint must stay visible when the footer is squeezed")
	}
}
