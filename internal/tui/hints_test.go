package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"alekohs/feedhatch/internal/parser"
)

func TestHintLabels(t *testing.T) {
	if got := strings.Join(hintLabels(3), ","); got != "a,s,d" {
		t.Fatalf("hintLabels(3) = %s, want home row letters first", got)
	}
	labels := hintLabels(30)
	seen := map[string]bool{}
	for _, l := range labels {
		if len(l) != 2 || seen[l] {
			t.Fatalf("with more links than letters, labels should be distinct pairs: %v", labels)
		}
		seen[l] = true
	}
}

func TestOLabelsVisibleLinksAndOpensTheTypedOne(t *testing.T) {
	m, opened := linkModel(t)
	a := m.currentArticle()
	a.body, a.links = parser.Text(`<p>See <a href="/one">one</a> and <a href="/two">two</a>.</p>`)

	m = press(m, "O")
	if m.hints == nil || m.tone != toneWarn {
		t.Fatal("O should label the links")
	}
	if got := m.hints.labels[1] + m.hints.labels[2]; got != "as" {
		t.Fatalf("labels = %v, want a and s in reading order", m.hints.labels)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "See aone and stwo.") {
		t.Fatalf("labels should be drawn before each link:\n%s", view)
	}

	m = press(m, "s")
	if m.hints != nil || len(*opened) != 1 || (*opened)[0] != "https://blog.example/two" {
		t.Fatalf("typing s should open the second link, opened %v", *opened)
	}
	if strings.Contains(ansi.Strip(m.View().Content), "stwo") {
		t.Fatal("labels should disappear after opening")
	}
}

func TestOCancels(t *testing.T) {
	m, opened := linkModel(t)
	for _, keys := range [][]string{{"O", "esc"}, {"O", "z"}, {"O", "enter"}} {
		m = press(m, keys...)
		if m.hints != nil || len(*opened) != 0 {
			t.Fatalf("%v should cancel without opening", keys)
		}
	}

	a := m.currentArticle()
	a.body, a.links, a.link = "No links at all.", nil, ""
	m = press(m, "O")
	if m.hints != nil || m.tone != toneWarn {
		t.Fatal("O without visible links should say so")
	}
}
