package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func searchReader() Model {
	m := motionReader()
	m.feeds[0].articles[0].body = "First needle\nAnother line\nNEEDLE twice needle\n\nLast needle\nÅÄÖ are letters\nLiteral [a.*] text"
	return m
}

func searchFor(m Model, query string) Model {
	m = press(m, "/")
	m = paste(m, query)
	return press(m, "enter")
}

func TestSearchAndRepeatWrap(t *testing.T) {
	m := searchFor(searchReader(), "needle")
	if m.search.active || m.line != 3 || m.search.query != "needle" {
		t.Fatal("search should close the prompt and jump to the first matching row")
	}
	for _, want := range []int{5, 7, 3} {
		m = press(m, "n")
		if m.line != want {
			t.Fatalf("n moved to %d, want %d", m.line, want)
		}
	}
	if !strings.Contains(m.status, "wrapped") {
		t.Fatal("search should report wrapping back to the beginning")
	}
	m = press(m, "N")
	if m.line != 7 || !strings.Contains(m.status, "wrapped") {
		t.Fatal("N should wrap backwards to the last matching row")
	}
	m = press(m, "N")
	if m.line != 5 {
		t.Fatal("N should find the previous matching row")
	}
}

func TestSearchLiteralAndUnicode(t *testing.T) {
	m := searchFor(searchReader(), "åäö")
	if m.line != 8 {
		t.Fatal("search should match Unicode text case-insensitively")
	}
	m = searchFor(m, "[a.*]")
	if m.line != 9 || len(m.matchingRows()) != 1 {
		t.Fatal("regex punctuation must be matched literally")
	}
}

func TestSearchMissingQueryAndNoMatch(t *testing.T) {
	m := press(searchReader(), "n")
	if !strings.Contains(m.status, "No previous search") || m.line != 0 {
		t.Fatal("n without a query should explain how to start searching")
	}
	m = searchFor(m, "not present")
	if !strings.Contains(m.status, "No matches") || m.line != 0 {
		t.Fatal("a failed search should report no matches and preserve the cursor")
	}
	m = press(m, "N")
	if m.line != 0 {
		t.Fatal("repeating a failed search must not move the cursor")
	}
}

func TestSearchCancelAndEmptyRepeat(t *testing.T) {
	m := searchFor(searchReader(), "needle")
	m = press(m, "/")
	m = paste(m, "another query")
	m = press(m, "esc")
	if m.search.active || m.search.query != "needle" || m.line != 3 || !m.reading {
		t.Fatal("Escape should cancel only the prompt, retaining query and cursor")
	}
	m = press(m, "/", "enter")
	if m.line != 5 {
		t.Fatal("an empty submitted prompt should repeat the previous search")
	}
}

func TestSearchInputDoesNotTriggerShortcuts(t *testing.T) {
	m := press(searchReader(), "/", "q", "d", "u", "v", "n", "z", "z", "?")
	if m.search.input != "qduvnzz?" || m.help || m.visual || m.pendingZ || m.line != 0 {
		t.Fatal("while searching, shortcuts must be entered as ordinary text")
	}
	m = press(m, "ctrl+u")
	m = paste(m, "Åäö\n\x1b\t")
	m = press(m, "backspace")
	if m.search.input != "Åä" {
		t.Fatal("search input should handle Unicode backspace and strip pasted controls")
	}
	m = paste(m, strings.Repeat("x", 3000))
	if len([]rune(m.search.input)) != 2048 {
		t.Fatal("search input should be bounded")
	}
}

func TestSearchExtendsVisualSelectionAndCopiesPlainText(t *testing.T) {
	m := press(searchReader(), "v")
	m = searchFor(m, "needle")
	m = press(m, "n")
	text, count := m.selectedText()
	if !m.visual || m.anchor != 0 || m.line != 5 || count != 6 {
		t.Fatal("search motions should extend an existing line selection")
	}
	if strings.Contains(text, "\x1b") || !strings.Contains(text, "First needle") {
		t.Fatal("copied selection must contain plain original text, not highlight escapes")
	}
}

func TestSearchHighlightPreservesText(t *testing.T) {
	input := "A needle and a NEEDLE."
	style := lipgloss.NewStyle().Reverse(true)
	result := highlightSearch(input, searchPattern("needle"), style)
	if result == input || ansi.Strip(result) != input {
		t.Fatal("highlighting should style matches without altering text")
	}
	if highlightSearch(input, nil, style) != input {
		t.Fatal("no query should leave text unchanged")
	}
}

func TestSearchRecomputesMatchesAfterResize(t *testing.T) {
	m := searchReader()
	m.feeds[0].articles[0].body = strings.Repeat("Some wrapped text. ", 40) + "\nLast needle"
	m = searchFor(m, "needle")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(Model)
	m = press(m, "n")
	_, right, _ := m.dimensions()
	lines := m.readerLines(right - 6)
	if lines[m.line] != "Last needle" {
		t.Fatal("search navigation must recompute row positions after resizing")
	}
}

func TestSearchIsScopedToReader(t *testing.T) {
	m := press(sizedModel(), "/", "n", "N")
	if m.search.active || m.feedIndex != 0 {
		t.Fatal("reader search should not intercept feed navigation")
	}
	m = press(m, "a", "enter", "/", "n", "N")
	if m.form.values[1] != "/nN" || m.search.active {
		t.Fatal("search shortcuts should remain ordinary text in forms")
	}
}
