package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() tea.View {
	p := m.palette()
	v := tea.NewView("")
	v.AltScreen = true
	v.WindowTitle = "feedhatch"
	v.BackgroundColor, v.ForegroundColor = p.background, p.text
	if m.width < 60 || m.height < 14 {
		message := "feedhatch · gör terminalen större (minst 60 × 14).\nq avsluta"
		v.SetContent(fit(message, max(1, m.width), max(1, m.height)))
		return v
	}
	leftWidth, rightWidth, panelHeight := m.dimensions()
	header := p.normal().Bold(true).Render("feedhatch") +
		lipgloss.NewStyle().Foreground(p.muted).Render("  /  en lugn plats för dina flöden")
	meta := m.summary() + "  ·  tema: " + m.themeName() + "  ·  MOCK"
	meta = lipgloss.NewStyle().Foreground(p.muted).Render(meta)
	header = bar(header, meta, m.width)

	left := m.renderPanel("1 Flöden", m.sidebar(leftWidth-4, panelHeight-4), leftWidth, panelHeight,
		m.focus == feedsPanel && m.form == nil && !m.help)
	title := "2 Artiklar"
	var right string
	switch {
	case m.form != nil:
		title = "Lägg till flöde"
		if m.form.categoryOnly {
			title = "Ändra kategori"
		}
		right = m.formContent(rightWidth-4, panelHeight-4)
	case m.help:
		title, right = "Kortkommandon", m.helpContent(rightWidth-4, panelHeight-4)
	case m.reading:
		title = "2 Läsning"
		right = m.reader(rightWidth-4, panelHeight-4)
	default:
		right = m.articleList(rightWidth-4, panelHeight-4)
	}
	right = m.renderPanel(title, right, rightWidth, panelHeight,
		m.focus == articlesPanel || m.form != nil || m.help)
	footer := "h/l panel  j/k välj  enter öppna  a lägg till  c kategori  ? hjälp  q avsluta"
	if m.reading {
		footer = "j/k line  {/} paragraph  d/u half-page  f/b page  v select  y copy  ? help"
	} else if m.form != nil {
		footer = "tab byt fält  enter nästa/spara  ctrl+u töm fält  esc avbryt"
	} else if m.help {
		footer = "j/k scroll  d/u half-page  f/b page  g/G first/last  esc close  q quit"
	}
	status := "Mockläge · ändringar sparas inte · t växlar mellan bastema och terminalens färger"
	if m.visual {
		start, end := m.selectionRange()
		status = fmt.Sprintf("VISUAL · %d markerade rader · y kopiera · esc avbryt", end-start+1)
	} else if m.status != "" {
		status = m.status
	}
	if m.search.active {
		footer = "Enter search  Esc cancel  Ctrl+u clear · literal, case-insensitive text"
		status = m.searchPrompt(m.width)
	} else if m.reading && m.status != "" {
		// Search feedback must remain visible even during visual selection.
		status = m.status
	}
	v.SetContent(lipgloss.JoinVertical(lipgloss.Left,
		fit(header, m.width, 1),
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		lipgloss.NewStyle().Foreground(p.muted).Render(fit(footer, m.width, 1)),
		lipgloss.NewStyle().Foreground(p.muted).Render(fit(status, m.width, 1)),
	))
	return v
}

func (m Model) dimensions() (left, right, height int) {
	left = min(34, max(24, m.width/3))
	return left, m.width - left, max(7, m.height-3)
}

func (m Model) renderPanel(title, content string, width, height int, focused bool) string {
	p := m.palette()
	border := p.border
	if focused {
		border = p.accent
		title = "> " + title
	}
	heading := lipgloss.NewStyle().Foreground(border).Bold(focused).Render(title)
	inside := heading + "\n" + fit(content, width-4, height-4)
	return p.normal().Border(lipgloss.NormalBorder()).BorderForeground(border).
		Padding(0, 1).Width(width).Height(height).
		Render(fit(inside, width-4, height-2))
}

func (m Model) sidebar(width, height int) string {
	p := m.palette()
	var lines []string
	selectedLine := 0
	for _, category := range m.categories() {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(p.muted).Render(ansi.Truncate(category, width, "…")))
		for i, feed := range m.feeds {
			if feed.category != category {
				continue
			}
			marker := " "
			if i == m.feedIndex {
				marker, selectedLine = ">", len(lines)
			}
			badge, dot := "", " "
			if n := unreadCount(feed); n > 0 {
				badge, dot = fmt.Sprintf("[%d]", n), "●"
			}
			// Reserve a real badge column; never let a count wrap onto a new row.
			prefix := marker + " " + dot + " "
			nameWidth := max(1, width-lipgloss.Width(prefix)-lipgloss.Width(badge)-1)
			line := bar(prefix+ansi.Truncate(feed.name, nameWidth, "…"), badge, width)
			if i == m.feedIndex {
				line = p.selected(m.focus == feedsPanel && m.form == nil && !m.help).Render(line)
			} else if badge != "" {
				line = p.normal().Render(line)
			} else {
				line = lipgloss.NewStyle().Foreground(p.muted).Render(line)
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "Inga flöden ännu.\nTryck a för att lägga till."
	}
	start := max(0, selectedLine-height+1)
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}

func (m Model) articleList(width, height int) string {
	if len(m.feeds) == 0 {
		return "Lägg till ett flöde med a."
	}
	p := m.palette()
	feed := m.feeds[m.feedIndex]
	muted := lipgloss.NewStyle().Foreground(p.muted)
	lines := []string{
		p.normal().Bold(true).Render(ansi.Truncate(feed.name, width, "…")),
		muted.Render(ansi.Truncate(feed.url, width, "…")),
		"",
	}
	if len(feed.articles) == 0 {
		return strings.Join(append(lines, "Inga artiklar ännu.", "", ansi.Wrap("Flödet är tillagt lokalt. RSS-hämtningen är inte inkopplad i mockläget.", width, "")), "\n")
	}
	// Each article occupies two rows. Keep the selected one in view.
	visible := max(1, (height-3)/2)
	start := max(0, m.articleIndex-visible+1)
	for i := start; i < min(len(feed.articles), start+visible); i++ {
		a := feed.articles[i]
		marker, dot := " ", " "
		if i == m.articleIndex {
			marker = ">"
		}
		if a.unread {
			dot = "●"
		}
		line := fit(marker+" "+dot+" "+a.title, width, 1)
		if i == m.articleIndex {
			line = p.selected(m.focus == articlesPanel).Render(line)
		} else {
			line = p.normal().Bold(a.unread).Render(line)
		}
		lines = append(lines, line, muted.Render(fit("    "+a.date, width, 1)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) readerLines(width int) []string {
	if a := m.currentArticle(); a != nil {
		text := a.title + "\n" + a.date + " · " + m.feeds[m.feedIndex].name + "\n\n" + a.body
		return strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	}
	return []string{"Ingen artikel vald."}
}

func (m Model) maxScroll() int {
	_, right, height := m.dimensions()
	return max(0, len(m.readerLines(right-6))-max(1, height-4))
}

func (m Model) reader(width, height int) string {
	p := m.palette()
	lines := m.readerLines(width - 2)
	start := min(m.scroll, max(0, len(lines)-height))
	low, high := m.selectionRange()
	pattern := searchPattern(m.search.query)
	matchStyle := lipgloss.NewStyle().Foreground(p.accent).Bold(true).Reverse(true)
	var rendered []string
	for i := start; i < min(len(lines), start+height); i++ {
		prefix := "  "
		if i == m.line {
			prefix = "> "
		}
		line := fit(prefix+highlightSearch(lines[i], pattern, matchStyle), width, 1)
		switch {
		case m.visual && i >= low && i <= high:
			line = p.selected(true).Render(line)
		case i == m.line:
			line = p.normal().Underline(true).Render(line)
		default:
			line = p.normal().Render(line)
		}
		rendered = append(rendered, line)
	}
	return strings.Join(rendered, "\n")
}

func (m Model) formContent(width, height int) string {
	f, p := m.form, m.palette()
	intro := "Lägg till en prenumeration. Ingen hämtning sker i mockläget."
	if f.categoryOnly {
		intro = "Skriv en befintlig eller ny kategori. Flödet flyttas när du sparar."
	}
	if f.err != "" {
		intro = f.err
	}
	lines := []string{ansi.Wrap(intro, width, ""), ""}
	// Split the wrapped introduction so viewport positions count terminal rows.
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	activeLine := 0
	labels := []string{"URL", "Namn (valfritt)", "Kategori"}
	for i, label := range labels {
		if f.categoryOnly && i != 2 {
			continue
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(p.muted).Render(label))
		value := f.values[i]
		if i == f.field {
			activeLine = len(lines)
			value += "▏"
			// Show the end of long inputs so the insertion point stays visible.
			if excess := lipgloss.Width(value) - (width - 2); excess > 0 {
				value = ansi.TruncateLeft(value, excess+1, "…")
			}
			value = p.selected(true).Render(fit("> "+value, width, 1))
		} else {
			value = fit("  "+value, width, 1)
		}
		lines = append(lines, value, "")
	}
	start := max(0, activeLine-height+2)
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}

func (m Model) maxHelpScroll() int {
	_, right, height := m.dimensions()
	lines := strings.Split(ansi.Wrap(helpContent, max(1, right-4), ""), "\n")
	return max(0, len(lines)-max(1, height-4))
}

func (m Model) helpContent(width, height int) string {
	lines := strings.Split(ansi.Wrap(helpContent, width, ""), "\n")
	start := min(m.helpScroll, max(0, len(lines)-height))
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}

func bar(left, right string, width int) string {
	if right == "" {
		return fit(left, width, 1)
	}
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > width {
		return fit(left, width, 1)
	}
	return left + strings.Repeat(" ", width-lipgloss.Width(left)-lipgloss.Width(right)) + right
}

// fit constrains rendered terminal cells, not bytes (Swedish and wide text work).
func fit(text string, width, height int) string {
	width, height = max(1, width), max(1, height)
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		line = ansi.Truncate(line, width, "…")
		lines[i] = line + strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

const helpContent = `h/l  Switch panel / open article
j/k  Select item or move one text row
Enter open · Tab switch · Esc back
g/G  First/last item or text row
d/u  Half-page down/up (also Ctrl+d/u)
f/b  Page down/up (also Ctrl+f/b)
Space / PgDown page down · PgUp page up

READER MOTIONS
{ / }  Previous/next paragraph start
H/M/L  Top/middle/bottom visible row
zz  Center the current row in the reader
/  Search article text · n/N next/previous
v  Toggle line selection; motions extend it
y  Copy selection (otherwise current row)
Esc cancels selection before leaving reader

a  Add feed · c change category
m  Toggle read/unread · t switch theme
? / Esc close help · q quit
Ctrl+c quits even while editing forms

Mock data; changes are not saved.
Clipboard uses OSC52 (e.g. Ghostty).`
