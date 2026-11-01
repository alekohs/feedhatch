package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"alekohs/feedhatch/internal/parser"
)

// linkModel opens an article whose body has a relative link, with openURL recorded
// instead of starting a browser.
func linkModel(t *testing.T) (Model, *[]string) {
	t.Helper()
	opened := &[]string{}
	previous := openURL
	openURL = func(url string) error {
		*opened = append(*opened, url)
		return nil
	}
	t.Cleanup(func() { openURL = previous })

	m := sizedModel(t)
	a := m.currentArticle()
	a.link = "https://blog.example/posts/weak-keys"
	a.body, a.links = parser.Text(`<p>Read the <a href="/notes">release notes</a> first.</p><p>No link here.</p>`)
	return press(m, "alt+2", "enter"), opened
}

// screenPosition finds where text is drawn in the two-panel reader.
func screenPosition(t *testing.T, m Model, text string) (x, y int) {
	t.Helper()
	left, _, _ := m.dimensions()
	for i, line := range m.readerLines(m.readerWidth()) {
		if col := strings.Index(parser.Plain(line), text); col >= 0 {
			return left + 4 + ansi.StringWidth(parser.Plain(line)[:col]), 3 + i - m.scroll
		}
	}
	t.Fatalf("%q is not in the reader", text)
	return 0, 0
}

func TestLinksAreColoredClickableAndAbsolute(t *testing.T) {
	m, _ := linkModel(t)
	view := m.View().Content

	if !strings.Contains(ansi.Strip(view), "Read the release notes first.") {
		t.Fatalf("the link text should read like normal text:\n%s", ansi.Strip(view))
	}
	if !strings.Contains(view, ansi.SetHyperlink("https://blog.example/notes")) {
		t.Fatal("the link should be an OSC 8 hyperlink to the absolute URL")
	}
	// Lip Gloss underlines character by character, so look for the first letter.
	underlined := regexp.MustCompile(regexp.QuoteMeta(ansi.SetHyperlink("https://blog.example/notes")) + `\x1b\[(?:[0-9;]*;)?4(?:;[0-9;]*)?mr`)
	if !underlined.MatchString(view) {
		t.Fatal("the link text should be underlined")
	}
	if !strings.Contains(ansi.Strip(view), "Läs hela artikeln") {
		t.Fatal("the article should end with a link to the full article")
	}
}

func TestClickOpensTheLinkUnderTheMouse(t *testing.T) {
	m, opened := linkModel(t)

	x, y := screenPosition(t, m, "notes")
	m = update(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if len(*opened) != 1 || (*opened)[0] != "https://blog.example/notes" {
		t.Fatalf("opened %v, want the link under the mouse", *opened)
	}
	if !strings.Contains(m.status, "Öppnar https://blog.example/notes") {
		t.Fatalf("status = %q", m.status)
	}

	x, y = screenPosition(t, m, "No link")
	m = update(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	x, y = screenPosition(t, m, "first.")
	m = update(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if len(*opened) != 1 {
		t.Fatalf("clicking text without a link must not open anything, opened %v", *opened)
	}

	x, y = screenPosition(t, m, "Läs hela")
	update(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if len(*opened) != 2 || (*opened)[1] != "https://blog.example/posts/weak-keys" {
		t.Fatalf("Läs hela artikeln should open the article, opened %v", *opened)
	}
}

func TestOOpensTheCursorLinesLinkOrTheArticle(t *testing.T) {
	m, opened := linkModel(t)
	lines := m.readerLines(m.readerWidth())
	for i, line := range lines {
		if strings.Contains(parser.Plain(line), "release notes") {
			m.line = i
		}
	}

	m = press(m, "o")
	m.line = 0 // the title has no link: o opens the article
	m = press(m, "o")
	if len(*opened) != 2 || (*opened)[0] != "https://blog.example/notes" || (*opened)[1] != "https://blog.example/posts/weak-keys" {
		t.Fatalf("opened %v", *opened)
	}

	m.currentArticle().link = ""
	m = press(m, "o")
	if m.tone != toneWarn || len(*opened) != 2 {
		t.Fatal("o should warn when the article has no link")
	}
}

func TestCopyAndSearchSeeLinkTextWithoutMarkers(t *testing.T) {
	m, _ := linkModel(t)
	m = searchFor(m, "release notes first")
	if len(m.matchingRows()) == 0 {
		t.Fatal("search should match across link markers")
	}
	text, _ := m.selectedText()
	if text != parser.Plain(text) {
		t.Fatalf("copied text %q contains markers", text)
	}
}

func TestOAsksWhichLinkWhenALineHasSeveral(t *testing.T) {
	m, opened := linkModel(t)
	a := m.currentArticle()
	a.body, a.links = parser.Text(`<p>Use <a href="/waf">WAF</a>, <a href="/cache">caching</a> and <a href="/bots">bot management</a>.</p>`)
	for i, line := range m.readerLines(m.readerWidth()) {
		if strings.Contains(parser.Plain(line), "WAF") {
			m.line = i
		}
	}

	m = press(m, "o")
	if len(*opened) != 0 || m.tone != toneWarn {
		t.Fatal("o should ask which link to open, not open one")
	}
	if !strings.Contains(m.status, "1 WAF · 2 caching · 3 bot management") {
		t.Fatalf("status = %q, want the numbered links", m.status)
	}
	m = press(m, "2")
	if len(*opened) != 1 || (*opened)[0] != "https://blog.example/cache" {
		t.Fatalf("opened %v, want the second link", *opened)
	}

	m = press(m, "o", "x")
	if len(*opened) != 1 || m.status != "Avbröts." || m.pendingLinks != nil {
		t.Fatal("any other key should cancel the choice")
	}
	m = press(m, "o", "9")
	if len(*opened) != 1 {
		t.Fatal("a digit without a link should cancel, not open anything")
	}
}
