package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func viewText(m Model) string { return ansi.Strip(m.View().Content) }

// chromeText is the header, footer and status rows; seeded article bodies may say "mockad".
func chromeText(m Model) string {
	lines := strings.Split(viewText(m), "\n")
	return lines[0] + "\n" + strings.Join(lines[len(lines)-2:], "\n")
}

func TestMockTextOnlyInMockMode(t *testing.T) {
	for _, reading := range []bool{false, true} {
		m := sizedModel(t)
		m.reading = reading
		if text := strings.ToLower(chromeText(m)); strings.Contains(text, "mock") {
			t.Fatalf("real mode must not mention mock (reading=%v)", reading)
		}
		m.help = true
		if text := strings.ToLower(viewText(m)); strings.Contains(text, "mock") {
			t.Fatal("real mode help must not mention mock")
		}
		m.help, m.mock = false, true
		if !strings.Contains(chromeText(m), "MOCK") {
			t.Fatal("mock mode should show MOCK in the header")
		}
	}
	m := sizedModel(t)
	m = press(m, "a")
	if strings.Contains(strings.ToLower(viewText(m)), "mock") {
		t.Fatal("real mode form must not mention mock")
	}
	m.mock = true
	if !strings.Contains(strings.ToLower(viewText(m)), "mock") {
		t.Fatal("mock mode form should mention mock")
	}
}

func TestHeaderKeepsFailureMarkerAtNarrowWidth(t *testing.T) {
	m := sizedModel(t)
	m.width, m.mock, m.fetchFailed = 60, true, true
	for len(m.feeds) < 1234 {
		m.feeds = append(m.feeds, m.feeds[0])
	}
	meta := ansi.Strip(m.headerMeta())
	if !strings.Contains(meta, "! ") || !strings.Contains(meta, "MOCK") {
		t.Fatalf("failure marker and MOCK must survive, got %q", meta)
	}
	if first := strings.Split(viewText(m), "\n")[0]; ansi.StringWidth(first) > 60 || !strings.Contains(first, "! ") {
		t.Fatalf("header must fit with the failure marker, got %q", first)
	}
}

func TestViewWithoutFeedsDoesNotPanic(t *testing.T) {
	for _, view := range []listView{feedView, unreadView, allView} {
		for _, reading := range []bool{false, true} {
			m := sizedModel(t)
			m.feeds, m.view, m.reading = nil, view, reading
			viewText(m)
		}
		m := sizedModel(t)
		m.feeds, m.view, m.help = nil, view, true
		viewText(m)
		m = press(sizedModel(t), "a")
		m.feeds, m.view = nil, view
		viewText(m)
	}
}

func TestFooterKeepsHelpHint(t *testing.T) {
	for _, width := range []int{60, 100} {
		for _, reading := range []bool{false, true} {
			m := sizedModel(t)
			m.width, m.height, m.reading = width, 28, reading
			if !strings.Contains(viewText(m), "? alla tangenter") {
				t.Fatalf("footer hint missing at width %d (reading=%v)", width, reading)
			}
		}
	}
}

func TestStatusTone(t *testing.T) {
	m := sizedModel(t)
	m.status, m.tone = "något gick snett", toneErr
	if !strings.Contains(viewText(m), "Fel: något gick snett") {
		t.Fatal("error status should have the Fel: prefix")
	}
	m.tone = toneInfo
	if strings.Contains(viewText(m), "Fel:") {
		t.Fatal("info status must not have the Fel: prefix")
	}
}

func TestHeaderUpdatedAndFailed(t *testing.T) {
	m := sizedModel(t)
	if strings.Contains(viewText(m), "uppdaterad") || strings.Contains(viewText(m), "misslyckades") {
		t.Fatal("header should not show update info by default")
	}
	m.updated = time.Date(2026, 1, 2, 9, 5, 0, 0, time.Local)
	if !strings.Contains(viewText(m), "uppdaterad 09:05") {
		t.Fatal("header should show the update time")
	}
	m.fetchFailed = true
	if !strings.Contains(viewText(m), "! uppdatering misslyckades") {
		t.Fatal("header should show the fetch-failed marker")
	}
}

func TestHelpHasGroupHeadings(t *testing.T) {
	m := sizedModel(t)
	if text := m.helpText(); !strings.Contains(text, "OSC52") {
		t.Fatal("help should mention OSC52")
	}
	for _, heading := range []string{"Navigera", "Läsa", "Flöden", "Övrigt"} {
		if !strings.Contains(m.helpText(), "\n"+heading+"\n") && !strings.HasPrefix(m.helpText(), heading+"\n") {
			t.Fatalf("help is missing heading %q", heading)
		}
	}
}
