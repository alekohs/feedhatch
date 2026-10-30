package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func motionReader() Model {
	m := sizedModel()
	m.width, m.height = 80, 18
	var body []string
	for i := range 80 {
		body = append(body, fmt.Sprintf("Body row %02d", i))
	}
	m.feeds = []subscription{{name: "Test feed", articles: []article{{
		title: "Title", date: "Today", body: strings.Join(body, "\n"),
	}}}}
	return press(m, "l", "enter")
}

func TestPageMotions(t *testing.T) {
	for _, test := range []struct {
		key   string
		delta int
	}{
		{"d", 5}, {"u", -5}, {"ctrl+d", 5}, {"ctrl+u", -5},
		{"f", 11}, {"b", -11}, {"ctrl+f", 11}, {"ctrl+b", -11},
		{"space", 11}, {"pgdown", 11}, {"pgup", -11},
	} {
		t.Run(test.key, func(t *testing.T) {
			m := motionReader()
			m.line = 30
			m.ensureLineVisible()
			m = press(m, test.key)
			if m.line != 30+test.delta {
				t.Fatalf("line = %d, want %d", m.line, 30+test.delta)
			}
			if m.line < m.scroll || m.line >= m.scroll+11 {
				t.Fatal("page motion moved the cursor outside the viewport")
			}
		})
	}
}

func TestParagraphMotions(t *testing.T) {
	m := motionReader()
	m.feeds[0].articles[0].body = "First line\nSecond line\nThird line\n\n  \nNext paragraph\nContinued\n\nLast paragraph"
	for _, test := range []struct {
		name string
		from int
		key  string
		want int
	}{
		{"skip header", 0, "}", 3},
		{"next paragraph", 3, "}", 8},
		{"skip rest of paragraph", 4, "}", 8},
		{"skip blank rows", 6, "}", 8},
		{"current paragraph start", 9, "{", 8},
		{"previous paragraph", 8, "{", 3},
		{"backwards from blank", 7, "{", 3},
		{"start boundary", 0, "{", 0},
		{"end boundary", 11, "}", 11},
	} {
		t.Run(test.name, func(t *testing.T) {
			m.line = test.from
			m = press(m, test.key)
			if m.line != test.want {
				t.Fatalf("line = %d, want %d", m.line, test.want)
			}
		})
	}
}

func TestParagraphMotionSkipsWrappedRows(t *testing.T) {
	m := motionReader()
	m.feeds[0].articles[0].body = strings.Repeat("Long paragraph with wrapped text. ", 30) + "\n\nNext paragraph"
	m.line = 3
	m = press(m, "}")
	_, right, _ := m.dimensions()
	lines := m.readerLines(right - 6)
	if lines[m.line] != "Next paragraph" {
		t.Fatalf("paragraph motion stopped inside wrapped text: %q", lines[m.line])
	}
}

func TestMotionsExtendVisualSelection(t *testing.T) {
	m := motionReader()
	m = press(m, "v", "d", "f")
	_, count := m.selectedText()
	if !m.visual || m.anchor != 0 || m.line != 16 || count != 17 {
		t.Fatal("page motions must extend selection without moving its anchor")
	}
	m = press(m, "{", "}")
	if !m.visual || m.anchor != 0 {
		t.Fatal("paragraph motions must preserve selection")
	}
}

func TestScreenMotions(t *testing.T) {
	for _, test := range []struct {
		key  string
		want int
	}{{"H", 15}, {"M", 20}, {"L", 25}} {
		t.Run(test.key, func(t *testing.T) {
			m := motionReader()
			m.line, m.scroll = 20, 15
			m = press(m, "v", test.key)
			if m.line != test.want || m.scroll != 15 || m.anchor != 20 || !m.visual {
				t.Fatalf("line=%d scroll=%d anchor=%d, want line=%d scroll=15 anchor=20", m.line, m.scroll, m.anchor, test.want)
			}
		})
	}
}

func TestCenterLine(t *testing.T) {
	m := motionReader()
	m.line = 30
	m.ensureLineVisible()
	before := m.scroll
	m = press(m, "v", "z")
	if m.scroll != before {
		t.Fatal("a single z must not scroll")
	}
	m = press(m, "z")
	if m.scroll != 25 || m.line != 30 || !m.visual || m.anchor != 30 {
		t.Fatal("zz should center the cursor without changing selection or cursor row")
	}
	for _, key := range []string{"g", "G"} {
		m = press(m, key, "z", "z")
		if m.scroll < 0 || m.scroll > m.maxScroll() {
			t.Fatal("centering near an edge must not scroll outside the document")
		}
	}
}

func TestCenterSequenceInterrupted(t *testing.T) {
	m := motionReader()
	m.line = 30
	m.ensureLineVisible()
	m = press(m, "z", "k")
	before := m.scroll
	m = press(m, "z")
	if m.scroll != before || !m.pendingZ {
		t.Fatal("another key must interrupt the zz sequence")
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = updated.(Model)
	if m.pendingZ {
		t.Fatal("resize must cancel pending key sequences")
	}
}

func TestDocumentStartAndEndMotions(t *testing.T) {
	m := motionReader()
	_, right, _ := m.dimensions()
	last := len(m.readerLines(right-6)) - 1
	m = press(m, "v", "G")
	_, count := m.selectedText()
	if m.line != last || m.scroll != m.maxScroll() || count != last+1 {
		t.Fatal("vG should select through the last row and scroll to the end")
	}
	m = press(m, "g", "g")
	if m.line != 0 || m.scroll != 0 || !m.visual || m.anchor != 0 {
		t.Fatal("g and gg should return to the beginning without leaving visual mode")
	}
	// The same shortcuts retain their existing list behavior.
	m = press(sizedModel(), "G")
	if m.feedIndex != len(m.feeds)-1 {
		t.Fatal("G should select the last feed")
	}
	m = press(m, "g", "l", "G")
	if m.articleIndex != len(m.feeds[m.feedIndex].articles)-1 {
		t.Fatal("G should select the last article in list view")
	}
	m = press(m, "g")
	if m.articleIndex != 0 {
		t.Fatal("g should select the first article in list view")
	}
}

func TestPagingHelpAndClamping(t *testing.T) {
	m := motionReader()
	m = press(m, "?", "d")
	if m.helpScroll != 5 {
		t.Fatalf("help scroll = %d, want 5", m.helpScroll)
	}
	m = press(m, "u", "u")
	if m.helpScroll != 0 {
		t.Fatal("help paging must stop at the beginning")
	}
	m = press(m, "f", "f", "f")
	if m.helpScroll != m.maxHelpScroll() {
		t.Fatal("help paging must stop at the end")
	}
}

func TestMotionKeysRemainTextInForms(t *testing.T) {
	m := press(sizedModel(), "a", "enter")
	m = press(m, "d", "u", "f", "b", "{", "}", "H", "M", "L", "z", "z")
	if m.form.values[1] != "dufb{}HMLzz" || m.reading || m.pendingZ {
		t.Fatal("motion keys must remain ordinary text in forms")
	}
}
