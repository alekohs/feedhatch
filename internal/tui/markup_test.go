package tui

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"alekohs/feedhatch/internal/parser"
)

const (
	bOn, bOff = parser.BoldOn, parser.BoldOff
	iOn, iOff = parser.ItalicOn, parser.ItalicOff
)

func TestHeadingLevelsFollowWrappedLines(t *testing.T) {
	lines := []string{"## A long heading", "that wraps", "", "Text", "", "#### Deep", "", "#nohash"}
	want := []int{2, 2, 0, 0, 0, 4, 0, 0}
	if got := headingLevels(lines); !slices.Equal(got, want) {
		t.Fatalf("levels = %v, want %v", got, want)
	}
}

func TestInlineStylesStayOpenAcrossWrappedLines(t *testing.T) {
	lines := []string{
		"start " + bOn + "bold",
		"still bold" + bOff + " and " + iOn + "italic",
		"still italic" + iOff + " done",
		bOn + "open",
		"",
		"new paragraph",
	}
	want := []inline{{}, {bold: true}, {italic: true}, {}, {}, {}}
	if got := inlineAtStart(lines); !slices.Equal(got, want) {
		t.Fatalf("states = %v, want %v", got, want)
	}
}

func TestRenderReaderLineHidesMarkersAndFitsWidth(t *testing.T) {
	style := lipgloss.NewStyle()
	line := renderReaderLine("  ", "a "+bOn+"bold"+bOff+" "+iOn+"word"+iOff, inline{}, 20, style, readerLook{match: style})
	if plain := ansi.Strip(line); plain != "  a bold word       " {
		t.Fatalf("line = %q, want the markers hidden and padded to 20 cells", plain)
	}

	long := renderReaderLine("  ", "a "+bOn+"very"+bOff+" long line that does not fit", inline{}, 12, style, readerLook{match: style})
	if w := ansi.StringWidth(long); w != 12 {
		t.Fatalf("width = %d, want 12", w)
	}
}

// sgr matches text drawn with an SGR sequence that contains the given parameter,
// e.g. 1 for bold and 3 for italic.
func sgr(param, text string) *regexp.Regexp {
	return regexp.MustCompile(`\x1b\[(?:[0-9;]*;)?` + param + `(?:;[0-9;]*)?m` + regexp.QuoteMeta(text))
}

func TestReaderShowsHeadingsBoldAndItalic(t *testing.T) {
	m := sizedModel(t)
	m.terminalTheme = false
	a := m.currentArticle()
	a.body = "## Section\n\nSome " + bOn + "bold" + bOff + " and " + iOn + "slanted" + iOff + " text."
	m = press(m, "alt+2", "enter")

	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "## Section") || !strings.Contains(plain, "Some bold and slanted text.") {
		t.Fatalf("reader should show the heading marker and the text without style markers:\n%s", plain)
	}
	if !sgr("1", "bold").MatchString(view) {
		t.Fatal("the bold text should be drawn bold")
	}
	if !sgr("3", "slanted").MatchString(view) {
		t.Fatal("the italic text should be drawn italic")
	}
}

func TestReaderTitleIsAccentedAndDateMuted(t *testing.T) {
	m := sizedModel(t)
	m.width, m.height = 60, 20
	a := m.currentArticle()
	a.title = "A title long enough to wrap onto a second line in a narrow reader"
	m = press(m, "alt+2", "enter", "j", "j", "j", "j") // move the cursor off the header rows

	lines := m.readerLines(m.readerWidth())
	rows := strings.Split(m.reader(m.readerWidth()+4, 10), "\n")
	p := m.palette()
	titleStyle := p.normal().Bold(true).Foreground(p.accent)
	dateStyle := p.normal().Foreground(p.muted)
	titleRows := len(strings.Split(ansi.Wrap(a.title, m.readerWidth(), ""), "\n"))
	if titleRows < 2 {
		t.Fatalf("the title should wrap in this test, got %d row", titleRows)
	}
	for i := 0; i <= titleRows; i++ {
		want := titleStyle
		if i == titleRows {
			want = dateStyle
		}
		if !strings.Contains(rows[i], want.Render(lines[i])) {
			t.Fatalf("row %d %q should use the title/date style", i, ansi.Strip(rows[i]))
		}
	}
	body := strings.TrimSpace(lines[titleRows+2])
	if body != "" && strings.Contains(rows[titleRows+2], titleStyle.Render(body)) {
		t.Fatal("the body must not use the title style")
	}
}

func TestSearchAndCopyIgnoreStyleMarkers(t *testing.T) {
	m := motionReader(t)
	m.feeds[0].articles[0].body = "Some " + bOn + "bold" + bOff + " " + iOn + "text" + iOff + "\n\nLast line"
	m = press(m, "esc", "enter")

	if rows := m.matchingRows(); len(rows) != 0 {
		t.Fatalf("no search yet, got matches %v", rows)
	}
	m = searchFor(m, "bold text")
	if len(m.matchingRows()) == 0 {
		t.Fatal("search should match text across style markers")
	}
	text, _ := m.selectedText()
	if strings.ContainsAny(text, bOn+bOff+iOn+iOff) {
		t.Fatalf("copied text %q should not contain style markers", text)
	}
}
