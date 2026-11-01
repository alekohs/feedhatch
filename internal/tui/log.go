package tui

import (
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The log page shows only the end of the file, which can grow to the rotation limit.
const (
	logTailBytes = 256 << 10
	logMaxLines  = 1000
)

// logTickMsg re-reads the open log; its value is the logGen of the open page,
// so a tick left over from an earlier opening ends instead of starting a second loop.
type logTickMsg int

func (m Model) logTick() tea.Cmd {
	gen := m.logGen
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return logTickMsg(gen) })
}

// readLog returns the last lines of the log file at path; a missing file has no lines.
func readLog(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := max(0, info.Size()-logTailBytes)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if offset > 0 {
		lines = lines[1:] // the first line was cut by the seek
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines[max(0, len(lines)-logMaxLines):], nil
}

func (m *Model) loadLog() {
	lines, err := readLog(m.logPath)
	if err != nil {
		lines = []string{"Kunde inte läsa loggen: " + err.Error()}
	}
	atEnd := m.logScroll >= m.maxLogScroll()
	m.logLines = lines
	if atEnd {
		m.logScroll = m.maxLogScroll()
	}
	m.logScroll = clamp(m.logScroll, m.maxLogScroll()+1)
}

func (m Model) maxLogScroll() int {
	_, _, height := m.dimensions()
	return max(0, len(m.logLines)-max(1, height-4))
}

func (m Model) logContent(width, height int) string {
	p := m.palette()
	if len(m.logLines) == 0 {
		return strings.Join([]string{
			"Loggen är tom.",
			"",
			lipgloss.NewStyle().Foreground(p.muted).Render(ansi.Wrap(m.logPath, width, "/")),
			lipgloss.NewStyle().Foreground(p.muted).Render(ansi.Wrap("Kör daemonen fylls dess logg i journalen: make service-logs", width, "")),
		}, "\n")
	}
	start := min(m.logScroll, max(0, len(m.logLines)-height))
	var lines []string
	for _, line := range m.logLines[start:min(len(m.logLines), start+height)] {
		// The file keeps the date; the page only needs the time to leave room for the message.
		if len(line) > 11 && line[4] == '-' && line[7] == '-' && line[10] == ' ' {
			line = line[11:]
		}
		line = ansi.Truncate(line, width, "…")
		switch {
		case strings.Contains(line, " ERROR "):
			line = lipgloss.NewStyle().Foreground(p.err).Render(line)
		case strings.Contains(line, " WARN "):
			line = lipgloss.NewStyle().Foreground(p.warn).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
