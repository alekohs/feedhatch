package tui

import (
	"fmt"
	"path/filepath"
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
	header := bar(p.normal().Bold(true).Render("feedhatch"), m.headerMeta(), m.width)

	left := m.renderPanel("1 Flöden", m.sidebar(leftWidth-4, panelHeight-4), leftWidth, panelHeight,
		m.focus == feedsPanel && m.form == nil && !m.help && !m.logOpen)
	title := m.listTitle()
	var right, preview string
	wide := m.wide() && m.form == nil && !m.help && !m.logOpen
	switch {
	case m.form != nil:
		title = "Lägg till flöde"
		if m.form.single && m.form.field == 2 {
			title = "Ändra kategori"
		} else if m.form.single {
			title = "Ändra intervall"
		}
		right = m.formContent(rightWidth-4, panelHeight-4)
	case m.help:
		title, right = "Kortkommandon", m.helpContent(rightWidth-4, panelHeight-4)
	case m.logOpen:
		title, right = "Logg · "+filepath.Base(m.logPath), m.logContent(rightWidth-4, panelHeight-4)
	case wide:
		listWidth, readerWidth := m.columns()
		right = m.renderPanel(title, m.articleList(listWidth-4, panelHeight-4), listWidth, panelHeight,
			m.focus == articlesPanel && !m.reading)
		preview = m.renderPanel(m.readerTitle(), m.reader(readerWidth-4, panelHeight-4), readerWidth, panelHeight, m.reading)
	case m.reading:
		title = m.readerTitle()
		right = m.reader(rightWidth-4, panelHeight-4)
	default:
		right = m.articleList(rightWidth-4, panelHeight-4)
	}
	if !wide {
		right = m.renderPanel(title, right, rightWidth, panelHeight,
			m.focus == articlesPanel || m.form != nil || m.help || m.logOpen)
	}
	footer := footerBar([][2]string{
		{"j/k", "välj"}, {"enter", "läs"}, {"]/[", "nästa oläst"}, {"{/}", "kategori"}, {"1/2", "panel"}, {"A", "allt läst"}, {"/", "sök"},
	}, true, m.width, p)
	if m.wide() {
		footer = footerBar([][2]string{
			{"j/k", "välj"}, {"enter", "läs i 3"}, {"]/[", "nästa oläst"}, {"{/}", "kategori"}, {"1/2/3", "panel"}, {"A", "allt läst"}, {"/", "sök"},
		}, true, m.width, p)
	}
	if m.reading {
		footer = footerBar([][2]string{
			{"j/k", "rad"}, {"{/}", "stycke"}, {"J/K", "artikel"}, {"]/[", "nästa oläst"}, {"/", "sök"}, {"v", "markera"}, {"h", "tillbaka"},
		}, true, m.width, p)
	} else if m.form != nil {
		footer = footerBar([][2]string{
			{"tab", "byt fält"}, {"enter", "nästa/spara"}, {"ctrl+u", "töm fält"}, {"esc", "avbryt"},
		}, false, m.width, p)
	} else if m.help {
		footer = footerBar([][2]string{
			{"j/k", "rulla"}, {"d/u", "halv sida"}, {"f/b", "sida"}, {"g/G", "först/sist"}, {"esc", "stäng"}, {"q", "avsluta"},
		}, false, m.width, p)
	} else if m.logOpen {
		footer = footerBar([][2]string{
			{"j/k", "rulla"}, {"d/u", "halv sida"}, {"g/G", "först/sist"}, {"r", "läs om"}, {"esc", "stäng"},
		}, true, m.width, p)
	}
	status, statusTone := "", toneInfo
	if m.mock {
		status = "Mockläge · ändringar sparas inte · t växlar mellan bastema och terminalens färger"
	}
	if m.visual {
		start, end := m.selectionRange()
		status = fmt.Sprintf("VISUAL · %d markerade rader · y kopiera · esc avbryt", end-start+1)
	} else if m.status != "" {
		status, statusTone = m.status, m.tone
	}
	if m.search.active {
		footer = footerBar([][2]string{
			{"Enter", "sök"}, {"Esc", "avbryt"}, {"Ctrl+u", "töm"}, {"", "· bokstavlig text, skiftlägesokänslig"},
		}, false, m.width, p)
		status, statusTone = m.searchPrompt(m.width), toneInfo
	} else if m.reading && m.status != "" {
		// Search feedback must remain visible even during visual selection.
		status, statusTone = m.status, m.tone
	}
	if m.filtering {
		footer = m.filterFooter(p)
	}
	if statusTone == toneErr {
		status = "Fel: " + status
	}
	statusStyle := p.normal()
	switch statusTone {
	case toneErr:
		statusStyle = statusStyle.Foreground(p.err)
	case toneWarn:
		statusStyle = statusStyle.Foreground(p.warn)
	case toneOK:
		statusStyle = statusStyle.Foreground(p.ok)
	}
	if p.statusBg != nil {
		statusStyle = statusStyle.Background(p.statusBg)
	}
	v.SetContent(lipgloss.JoinVertical(lipgloss.Left,
		fit(header, m.width, 1),
		lipgloss.JoinHorizontal(lipgloss.Top, left, right, preview),
		fit(footer, m.width, 1),
		statusStyle.Render(fit(status, m.width, 1)),
	))
	return v
}

// headerMeta drops the least important parts first so it never overflows the header.
func (m Model) headerMeta() string {
	p := m.palette()
	muted := lipgloss.NewStyle().Foreground(p.muted)
	build := func(summary, theme, updated bool, failure string) string {
		var parts []string
		if summary {
			parts = append(parts, muted.Render(m.summary()))
		}
		if updated && !m.updated.IsZero() {
			parts = append(parts, muted.Render("uppdaterad "+m.updated.Format("15:04")))
		}
		if m.fetchFailed {
			parts = append(parts, lipgloss.NewStyle().Foreground(p.err).Render(failure))
		}
		if theme {
			parts = append(parts, muted.Render("tema: "+m.themeName()))
		}
		if m.mock {
			parts = append(parts, muted.Render("MOCK"))
		}
		return strings.Join(parts, muted.Render(" · "))
	}
	room := m.width - lipgloss.Width("feedhatch") - 1
	const failure, shortFailure = "! uppdatering misslyckades", "! fel"
	for _, meta := range []string{
		build(true, true, true, failure),
		build(true, false, true, failure),
		build(true, false, false, failure),
		build(true, false, false, shortFailure),
		build(false, false, false, shortFailure),
	} {
		if lipgloss.Width(meta) <= room {
			return meta
		}
	}
	return ""
}

// footerBar renders key/label pairs, dropping pairs from the end before the
// "? alla tangenter" hint is ever cut.
func footerBar(items [][2]string, hint bool, width int, p palette) string {
	key := lipgloss.NewStyle().Foreground(p.accent)
	label := lipgloss.NewStyle().Foreground(p.muted)
	right := ""
	if hint {
		right = key.Render("?") + label.Render(" alla tangenter")
	}
	for n := len(items); n > 0; n-- {
		parts := make([]string, n)
		for i, item := range items[:n] {
			parts[i] = label.Render(item[1])
			if item[0] != "" {
				parts[i] = key.Render(item[0]) + " " + parts[i]
			}
		}
		left := strings.Join(parts, "  ")
		if lipgloss.Width(left)+lipgloss.Width(right)+2 <= width {
			return bar(left, right, width)
		}
	}
	return right
}

func (m Model) filterFooter(p palette) string {
	key := lipgloss.NewStyle().Foreground(p.accent)
	label := lipgloss.NewStyle().Foreground(p.muted)
	hint := key.Render("enter") + " " + label.Render("klar") + "  " + key.Render("esc") + " " + label.Render("rensa")
	prompt := "/ " + m.filter + "▏"
	room := m.width - lipgloss.Width(hint) - 2
	if room < 8 {
		hint, room = "", m.width
	}
	if excess := lipgloss.Width(prompt) - room; excess > 0 {
		prompt = ansi.TruncateLeft(prompt, excess+1, "…")
	}
	return bar(prompt, hint, m.width)
}

func (m Model) listTitle() string {
	title := m.baseListTitle()
	if m.filter == "" {
		return title
	}
	hits := "träffar"
	if len(m.articleRefs()) == 1 {
		hits = "träff"
	}
	if i := strings.Index(title, " · "); i >= 0 {
		title = title[:i]
	}
	return fmt.Sprintf("%s · %d %s för %q", title, len(m.articleRefs()), hits, m.filter)
}

func (m Model) baseListTitle() string {
	switch m.view {
	case unreadView:
		unread := 0
		for _, feed := range m.feeds {
			unread += unreadCount(feed)
		}
		return fmt.Sprintf("2 Olästa · %d kvar av %d", unread, len(m.articleRefs()))
	case allView:
		return fmt.Sprintf("2 Alla artiklar · %d från %d flöden", len(m.articleRefs()), len(m.feeds))
	}
	return "2 Artiklar"
}

func (m Model) readerTitle() string {
	name := "Alla artiklar"
	switch m.view {
	case unreadView:
		name = "Olästa"
	case feedView:
		if m.feedIndex >= 0 && m.feedIndex < len(m.feeds) {
			name = m.feeds[m.feedIndex].name
		}
	}
	number := "2"
	if m.wide() {
		number = "3"
		if len(m.articleRefs()) == 0 {
			return number + " Läsning"
		}
	}
	return fmt.Sprintf("%s Läsning · %s %d/%d", number, name, m.articleIndex+1, len(m.articleRefs()))
}

func (m Model) wide() bool { return m.width >= 140 }

// columns splits the space right of the sidebar between the list and the reader.
func (m Model) columns() (list, reader int) {
	_, right, _ := m.dimensions()
	list = right * 2 / 5
	return list, right - list
}

func (m Model) readerWidth() int {
	_, right, _ := m.dimensions()
	if m.wide() {
		_, right = m.columns()
	}
	return right - 6
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
	if len(m.feeds) == 0 {
		return "Inga flöden ännu.\nTryck a för att lägga till."
	}
	p := m.palette()
	unread, total := 0, 0
	for _, feed := range m.feeds {
		unread += unreadCount(feed)
		total += len(feed.articles)
	}
	lines := []string{
		m.sidebarRow("Olästa", unread, m.view == unreadView, p.normal(), width),
		m.sidebarRow("Alla artiklar", total, m.view == allView, p.normal(), width),
	}
	selectedLine := 0
	if m.view == allView {
		selectedLine = 1
	}
	category := lipgloss.NewStyle().Foreground(p.category).Bold(true)
	for _, name := range m.categories() {
		lines = append(lines, "", category.Render(ansi.Truncate(strings.ToUpper(name), width, "…")))
		for i, feed := range m.feeds {
			if feed.category != name {
				continue
			}
			selected := m.view == feedView && i == m.feedIndex
			if selected {
				selectedLine = len(lines)
			}
			nameStyle := lipgloss.NewStyle().Foreground(p.muted)
			if unreadCount(feed) > 0 {
				nameStyle = p.normal()
			}
			lines = append(lines, m.sidebarRow(feed.name, unreadCount(feed), selected, nameStyle, width))
		}
	}
	start := max(0, selectedLine-height+1)
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}

func (m Model) sidebarRow(name string, unread int, selected bool, nameStyle lipgloss.Style, width int) string {
	p := m.palette()
	marker, dot, badge := " ", " ", ""
	if selected {
		marker = ">"
	}
	if unread > 0 {
		dot, badge = "●", fmt.Sprintf("[%d]", unread)
	}
	// Reserve a real badge column; never let a count wrap onto a new row.
	nameWidth := max(1, width-4-lipgloss.Width(badge)-1)
	name = ansi.Truncate(name, nameWidth, "…")
	if selected {
		return p.selected(m.focus == feedsPanel && m.form == nil && !m.help && !m.logOpen).Render(bar(marker+" "+dot+" "+name, badge, width))
	}
	gap := max(0, width-4-lipgloss.Width(name)-lipgloss.Width(badge))
	counts := lipgloss.NewStyle().Foreground(p.unread)
	return p.normal().Render(marker+" ") + counts.Render(dot) + p.normal().Render(" ") +
		nameStyle.Render(name) + p.normal().Render(strings.Repeat(" ", gap)) + counts.Render(badge)
}

func (m Model) articleList(width, height int) string {
	if len(m.feeds) == 0 {
		return "Lägg till ett flöde med a."
	}
	p := m.palette()
	muted := lipgloss.NewStyle().Foreground(p.muted)
	refs := m.articleRefs()
	if len(refs) == 0 {
		switch {
		case m.filter != "":
			return fmt.Sprintf("Inga träffar för %q.\n\n", m.filter) + muted.Render("esc rensar sökningen")
		case m.view == unreadView:
			return "Inga olästa artiklar.\n\n" + muted.Render(ansi.Wrap("{ } kategori · Alla artiklar finns ovanför flödena", width, ""))
		case m.view == feedView && m.mock:
			return "Inga artiklar ännu.\n\n" + ansi.Wrap("Flödet är tillagt lokalt. RSS-hämtningen är inte inkopplad i mockläget.", width, "")
		case m.view == feedView:
			return "Inga artiklar ännu.\n\n" + ansi.Wrap("Hämtas vid nästa runda (inom en minut).", width, "")
		}
		return "Inga artiklar ännu."
	}
	feedWidth, dateWidth := 0, 0
	for _, r := range refs {
		// In wide mode the preview next to the list already names the feed.
		if m.view != feedView && !m.wide() {
			feedWidth = min(18, max(feedWidth, lipgloss.Width(m.feeds[r.feed].name)))
		}
		dateWidth = max(dateWidth, lipgloss.Width(m.feeds[r.feed].articles[r.article].date))
	}
	const minTitle = 12
	if feedWidth > 0 {
		feedWidth = min(feedWidth, width-4-2-minTitle-dateWidth-2)
		if feedWidth < 6 {
			feedWidth = 0
		}
	}
	dateWidth = max(0, min(dateWidth, width-4-2-minTitle))
	titleWidth := max(1, width-4-dateWidth-2)
	if feedWidth > 0 {
		titleWidth = max(1, titleWidth-feedWidth-2)
	}
	pad := func(text string, w int) string {
		text = ansi.Truncate(text, w, "…")
		return text + strings.Repeat(" ", max(0, w-lipgloss.Width(text)))
	}
	start := max(0, m.articleIndex-height+1)
	var lines []string
	for i := start; i < min(len(refs), start+height); i++ {
		feed := m.feeds[refs[i].feed]
		a := feed.articles[refs[i].article]
		marker, dot := " ", " "
		if i == m.articleIndex {
			marker = ">"
		}
		if a.unread {
			dot = "●"
		}
		feedColumn := ""
		if feedWidth > 0 {
			feedColumn = pad(feed.name, feedWidth) + "  "
		}
		if i == m.articleIndex {
			selected := p.selected(m.focus == articlesPanel)
			detail := selected.Bold(false).Foreground(p.text)
			lines = append(lines, selected.Render(marker+" "+dot+" "+pad(a.title, titleWidth)+"  ")+
				detail.Render(feedColumn+pad(a.date, dateWidth)))
			continue
		}
		line := p.normal().Render(marker+" ") + lipgloss.NewStyle().Foreground(p.unread).Render(dot) +
			p.normal().Render(" ") + p.normal().Bold(a.unread).Render(pad(a.title, titleWidth)+"  ")
		if feedWidth > 0 {
			line += lipgloss.NewStyle().Foreground(p.feed).Render(feedColumn)
		}
		lines = append(lines, line+muted.Render(pad(a.date, dateWidth)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) readerLines(width int) []string {
	if a := m.currentArticle(); a != nil {
		text := a.title + "\n" + a.date + " · " + m.currentFeed().name + "\n\n" + a.body
		return strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	}
	return []string{"Ingen artikel vald."}
}

func (m Model) maxScroll() int {
	_, _, height := m.dimensions()
	return max(0, len(m.readerLines(m.readerWidth()))-max(1, height-4))
}

func (m Model) reader(width, height int) string {
	p := m.palette()
	if !m.reading && m.currentArticle() == nil {
		return lipgloss.NewStyle().Foreground(p.muted).Render("Välj en artikel.")
	}
	lines := m.readerLines(m.readerWidth())
	scroll := m.scroll
	if !m.reading {
		scroll = 0
	}
	start := min(scroll, max(0, len(lines)-height))
	low, high := m.selectionRange()
	pattern := searchPattern(m.search.query)
	matchStyle := lipgloss.NewStyle().Foreground(p.accent).Bold(true).Reverse(true)
	var rendered []string
	for i := start; i < min(len(lines), start+height); i++ {
		prefix := "  "
		if m.reading && i == m.line {
			prefix = "> "
		}
		line := fit(prefix+highlightSearch(lines[i], pattern, matchStyle), width, 1)
		switch {
		case m.reading && m.visual && i >= low && i <= high:
			line = p.selected(true).Render(line)
		case m.reading && i == m.line:
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
	intro := "Lägg till en prenumeration."
	if m.mock {
		intro += " Ingen hämtning sker i mockläget."
	}
	if f.single && f.field == 2 {
		intro = "Skriv en befintlig eller ny kategori. Flödet flyttas när du sparar."
	} else if f.single {
		intro = "Hur ofta flödet ska uppdateras, i minuter."
	}
	if f.err != "" {
		intro = f.err
	}
	lines := []string{ansi.Wrap(intro, width, ""), ""}
	// Split the wrapped introduction so viewport positions count terminal rows.
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	activeLine := 0
	labels := []string{"URL", "Namn (valfritt)", "Kategori", "Intervall (minuter)"}
	for i, label := range labels {
		if f.single && i != f.field {
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
	lines := strings.Split(ansi.Wrap(m.helpText(), max(1, right-4), ""), "\n")
	return max(0, len(lines)-max(1, height-4))
}

func (m Model) helpText() string {
	if m.mock {
		return helpContent + "\nMockdata; ändringar sparas inte."
	}
	return helpContent
}

func (m Model) helpContent(width, height int) string {
	lines := strings.Split(ansi.Wrap(m.helpText(), width, ""), "\n")
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

const helpContent = `Navigera
h/l  Byt panel · öppna artikel
j/k  Välj post eller flytta en rad
Enter öppna · Tab byt panel · 1/2 välj panel · 3 fokusera läsvyn
Från 140 kolumner förhandsvisas artikeln i en tredje kolumn; den markeras som läst när du öppnar den med Enter/l/3
g/G  Första/sista posten eller raden
d/u  Halv sida ner/upp (även Ctrl+d/u)
f/b  Sida ner/upp (även Ctrl+f/b)
Space / PgDn sida ner · PgUp sida upp
]/[  Nästa/föregående oläst artikel
J/K  Nästa/föregående artikel
{ / }  Föregående/nästa kategori i flödeslistan, stycke i läsvyn

Läsa
H/M/L  Översta/mittersta/understa synliga raden
zz  Centrera aktuell rad
/  Filtrera artikellistan (i listan) · sök i artikeln (i läsvyn)
n/N  Nästa/föregående sökträff i artikeln
v  Markera rader; rörelser utökar markeringen
y  Kopiera markeringen (annars aktuell rad)
Esc  Avbryt markeringen innan du lämnar läsvyn
m  Växla läst/oläst

Flöden
a  Lägg till flöde · c ändra kategori · i ändra intervall
D  Ta bort flödet och dess artiklar (bekräfta med y)
A  Markera alla artiklar som lästa
r  Uppdatera flödena

Övrigt
t  Växla tema
?  Hjälp · Esc stäng · q avsluta
!  Visa loggen (r läser om, Esc stänger)
Ctrl+c  Avsluta, även i formulär
Urklipp använder OSC52 (t.ex. Ghostty).`
