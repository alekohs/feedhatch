package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func writeLog(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func numberedLines(n int) []string {
	var lines []string
	for i := range n {
		lines = append(lines, fmt.Sprintf("2026-11-01 12:00:00 INFO  server: line %03d", i))
	}
	return lines
}

func TestReadLogMissingFileIsEmpty(t *testing.T) {
	lines, err := readLog(filepath.Join(t.TempDir(), "missing.log"))
	if err != nil || len(lines) != 0 {
		t.Fatalf("lines = %q, err = %v, want no lines and no error", lines, err)
	}
}

func TestReadLogKeepsOnlyTheEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedhatch.log")
	writeLog(t, path, numberedLines(logMaxLines+50)...)

	lines, err := readLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != logMaxLines || !strings.HasSuffix(lines[len(lines)-1], fmt.Sprintf("line %03d", logMaxLines+49)) {
		t.Fatalf("got %d lines ending %q, want the last %d", len(lines), lines[len(lines)-1], logMaxLines)
	}
}

func TestReadLogDropsLineCutBySeek(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedhatch.log")
	long := strings.Repeat("x", logTailBytes) // pushes the first line out of the tail window
	writeLog(t, path, "first "+long, "last")

	lines, err := readLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "last" {
		t.Fatalf("lines = %d, want only the complete last line", len(lines))
	}
}

func TestLogPageOpensAtNewestLine(t *testing.T) {
	m := sizedModel(t)
	writeLog(t, m.logPath, numberedLines(100)...)

	m = press(m, "!")
	view := ansi.Strip(m.View().Content)
	if !m.logOpen || !strings.Contains(view, "Logg · feedhatch.log") || !strings.Contains(view, "line 099") {
		t.Fatalf("! should open the log at the newest line:\n%s", view)
	}

	m = press(m, "g")
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "line 000") || strings.Contains(view, "line 099") {
		t.Fatal("g should scroll to the oldest line")
	}
	m = press(m, "j")
	if m.logScroll != 1 {
		t.Fatalf("logScroll = %d, want 1 after j", m.logScroll)
	}

	m = press(m, "esc")
	if m.logOpen {
		t.Fatal("esc should close the log")
	}
	m = press(m, "!", "!")
	if m.logOpen {
		t.Fatal("! should also close the log")
	}
}

func TestLogPageKeysDoNotRunShortcuts(t *testing.T) {
	m := sizedModel(t)
	feeds := len(m.feeds)

	m = press(m, "!", "D", "y", "a", "A")
	if !m.logOpen || m.form != nil || m.pendingDelete || len(m.feeds) != feeds {
		t.Fatal("keys on the log page must not reach the feed shortcuts")
	}
}

func TestLogPageFollowsNewLinesAtTheEnd(t *testing.T) {
	m := sizedModel(t)
	writeLog(t, m.logPath, numberedLines(50)...)
	m = press(m, "!")

	writeLog(t, m.logPath, numberedLines(60)...)
	updated, cmd := m.Update(logTickMsg(m.logGen))
	m = updated.(Model)
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "line 059") {
		t.Fatal("a tick should refresh the open log and stay at the end")
	}
	if cmd == nil {
		t.Fatal("the log should keep ticking while it is open")
	}

	m = press(m, "g")
	writeLog(t, m.logPath, numberedLines(70)...)
	m = press(m, "r")
	if m.logScroll != 0 {
		t.Fatalf("logScroll = %d, want r to keep the position when scrolled up", m.logScroll)
	}
}

func TestLogPageEmptyState(t *testing.T) {
	m := sizedModel(t)
	m = press(m, "!")
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Loggen är tom.") {
		t.Fatalf("missing empty state:\n%s", view)
	}
}

func TestLogPageFitsTerminal(t *testing.T) {
	m := sizedModel(t)
	writeLog(t, m.logPath, strings.Repeat("y", 400), "2026-11-01 12:00:00 ERROR <feedhatch/main.go:1> broken")
	for _, size := range [][2]int{{60, 14}, {100, 28}, {160, 40}} {
		m.width, m.height = size[0], size[1]
		m = press(m, "!")
		for _, line := range strings.Split(m.View().Content, "\n") {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Fatalf("line width %d exceeds %d at %dx%d: %q", w, size[0], size[0], size[1], ansi.Strip(line))
			}
		}
		m = press(m, "esc")
	}
}

func TestLogPageShowsTimeWithoutDate(t *testing.T) {
	m := sizedModel(t)
	writeLog(t, m.logPath, "2026-11-01 12:00:05 WARN  <server/server.go:88> server: fetch failed")

	view := ansi.Strip(press(m, "!").View().Content)
	if strings.Contains(view, "2026-11-01") || !strings.Contains(view, "12:00:05 WARN") {
		t.Fatalf("the page should show the time without the date:\n%s", view)
	}
}

func TestLogTickStopsWhenClosedOrStale(t *testing.T) {
	m := sizedModel(t)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("opening the log should start the tick")
	}
	stale := m.logGen

	m = press(m, "esc")
	if _, cmd := m.Update(logTickMsg(stale)); cmd != nil {
		t.Fatal("a tick after closing must not schedule another")
	}

	m = press(m, "!")
	if _, cmd := m.Update(logTickMsg(stale)); cmd != nil {
		t.Fatal("a tick from an earlier opening must end, so only one loop runs")
	}
	if _, cmd := m.Update(logTickMsg(m.logGen)); cmd == nil {
		t.Fatal("the current tick should keep the loop going")
	}
}
