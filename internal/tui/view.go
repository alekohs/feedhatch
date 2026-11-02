package tui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"alekohs/feedhatch/internal/graphics"
	"alekohs/feedhatch/internal/parser"
)

func (m Model) View() tea.View {
	p := m.palette()
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
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
	wide := m.threeColumns()
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
		{"j/k", "välj"}, {"enter", "läs"}, {"]/[", "nästa oläst"}, {"{/}", "kategori"}, {"A", "allt läst"}, {"/", "sök"},
	}, true, m.width, p)
	if m.wide() {
		footer = footerBar([][2]string{
			{"j/k", "välj"}, {"enter", "läs i 3"}, {"]/[", "nästa oläst"}, {"{/}", "kategori"}, {"A", "allt läst"}, {"/", "sök"},
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

// Column limits: every panel stays usable however far a border is dragged.
const (
	minSidebar    = 18
	minRightPanel = 30
	minList       = 24
	minReader     = 40
	resizeStep    = 2
)

// columns splits the space right of the sidebar between the list and the reader.
func (m Model) columns() (list, reader int) {
	_, right, _ := m.dimensions()
	list = right * 2 / 5
	if m.listWidth > 0 {
		list = m.listWidth
	}
	list = max(minList, min(list, right-minReader))
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
	if m.sidebarWidth > 0 {
		left = m.sidebarWidth
	}
	left = max(minSidebar, min(left, m.width-minRightPanel))
	return left, m.width - left, max(7, m.height-3)
}

// threeColumns reports whether the list and the reader are shown side by side right now.
func (m Model) threeColumns() bool {
	return m.wide() && m.form == nil && !m.help && !m.logOpen
}

// border is a draggable panel border.
type border int

const (
	noBorder border = iota
	sidebarBorder
	listBorder
)

// borderAt finds the border under a mouse position: each border is two cells wide,
// the edge of the panel on either side.
func (m Model) borderAt(x, y int) border {
	_, _, height := m.dimensions()
	if y < 1 || y > height {
		return noBorder
	}
	left, _, _ := m.dimensions()
	if x == left-1 || x == left {
		return sidebarBorder
	}
	if list, _ := m.columns(); m.threeColumns() && (x == left+list-1 || x == left+list) {
		return listBorder
	}
	return noBorder
}

// moveBorder puts a border at screen column x, within the column limits.
func (m *Model) moveBorder(b border, x int) {
	switch b {
	case sidebarBorder:
		m.sidebarWidth = x + 1
		m.sidebarWidth, _, _ = m.dimensions()
	case listBorder:
		left, _, _ := m.dimensions()
		m.listWidth = x - left + 1
		m.listWidth, _ = m.columns()
	}
	// Wrapped row numbers change with the width; do not silently copy a new range.
	m.visual = false
	m.ensureLineVisible()
}

// resize moves the border next to the focused panel: the list/reader border when the
// list or reader has focus in three columns, otherwise the sidebar border.
func (m *Model) resize(delta int) {
	if m.threeColumns() && m.focus == articlesPanel {
		left, _, _ := m.dimensions()
		list, _ := m.columns()
		m.moveBorder(listBorder, left+list-1+delta)
		return
	}
	left, _, _ := m.dimensions()
	m.moveBorder(sidebarBorder, left-1+delta)
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

// sidebarEntry is one row of the sidebar: a list, a feed, a category header or a gap.
type sidebarEntry struct {
	view     listView // for list and feed rows
	feed     int      // index into m.feeds for feed rows, -1 otherwise
	category string   // for header rows
	header   bool
	gap      bool
}

// sidebarEntries are the sidebar's rows in order, and the row of the current selection.
func (m Model) sidebarEntries() (entries []sidebarEntry, selected int) {
	entries = []sidebarEntry{{view: unreadView, feed: -1}, {view: allView, feed: -1}}
	if m.view == allView {
		selected = 1
	}
	for _, name := range m.categories() {
		entries = append(entries, sidebarEntry{feed: -1, gap: true}, sidebarEntry{feed: -1, category: name, header: true})
		for i, feed := range m.feeds {
			if feed.category != name {
				continue
			}
			if m.view == feedView && i == m.feedIndex {
				selected = len(entries)
			}
			entries = append(entries, sidebarEntry{view: feedView, feed: i})
		}
	}
	return entries, selected
}

// sidebarStart is the first sidebar row shown, so the selection stays visible.
func sidebarStart(selected, height int) int { return max(0, selected-height+1) }

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
	category := lipgloss.NewStyle().Foreground(p.category).Bold(true)
	entries, selectedLine := m.sidebarEntries()
	var lines []string
	for _, e := range entries {
		switch {
		case e.gap:
			lines = append(lines, "")
		case e.header:
			lines = append(lines, category.Render(ansi.Truncate(strings.ToUpper(e.category), width, "…")))
		case e.view == unreadView:
			lines = append(lines, m.sidebarRow("Olästa", unread, true, m.view == unreadView, p.normal(), width))
		case e.view == allView:
			lines = append(lines, m.sidebarRow("Alla artiklar", total, false, m.view == allView, p.normal(), width))
		default:
			feed := m.feeds[e.feed]
			nameStyle := lipgloss.NewStyle().Foreground(p.muted)
			if unreadCount(feed) > 0 {
				nameStyle = p.normal()
			}
			name := feed.name
			if feed.notify {
				name += " ♪"
			}
			lines = append(lines, m.sidebarRow(name, unreadCount(feed), true, m.view == feedView && e.feed == m.feedIndex, nameStyle, width))
		}
	}
	start := sidebarStart(selectedLine, height)
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}

func (m Model) sidebarRow(name string, count int, unread, selected bool, nameStyle lipgloss.Style, width int) string {
	p := m.palette()
	marker, dot, badge := " ", " ", ""
	if selected {
		marker = ">"
	}
	if count > 0 {
		badge = fmt.Sprintf("[%d]", count)
		if unread {
			dot = "●"
		}
	}
	// Reserve a real badge column; never let a count wrap onto a new row.
	nameWidth := max(1, width-4-lipgloss.Width(badge)-1)
	name = ansi.Truncate(name, nameWidth, "…")
	if selected {
		return p.selected(m.focus == feedsPanel && m.form == nil && !m.help && !m.logOpen).Render(bar(marker+" "+dot+" "+name, badge, width))
	}
	gap := max(0, width-4-lipgloss.Width(name)-lipgloss.Width(badge))
	counts := lipgloss.NewStyle().Foreground(p.muted)
	if unread {
		counts = lipgloss.NewStyle().Foreground(p.unread)
	}
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
			return "Inga artiklar ännu.\n\n" + ansi.Wrap("Nya flöden hämtas direkt; artiklarna dyker upp här om en stund.", width, "")
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
	lines, _ := m.readerRows(width)
	return lines
}

// readerRows are the reader's rows, and the row each image (by URL) starts on.
func (m Model) readerRows(width int) (lines []string, imageAt map[string]int) {
	a := m.currentArticle()
	if a == nil {
		return []string{"Ingen artikel vald."}, nil
	}
	imageAt = map[string]int{}
	head := a.title + "\n" + a.date + " · " + m.currentFeed().name + "\n"
	lines = strings.Split(ansi.Wrap(head, max(1, width), ""), "\n")
	body := a.bodyLines()
	for i, line := range body {
		if code, ok := strings.CutPrefix(line, parser.Code); ok {
			// An empty code line above and below pads the block, as a web page does.
			if i == 0 || !strings.HasPrefix(body[i-1], parser.Code) {
				lines = append(lines, parser.Code)
			}
			for _, row := range strings.Split(ansi.Hardwrap(code, max(1, width-2), true), "\n") {
				lines = append(lines, parser.Code+row)
			}
			if i == len(body)-1 || !strings.HasPrefix(body[i+1], parser.Code) {
				lines = append(lines, parser.Code)
			}
			continue
		}
		if src, _, ok := parser.ParseImage(line); ok {
			if _, seen := imageAt[resolveURL(a.link, src)]; !seen {
				imageAt[resolveURL(a.link, src)] = len(lines)
			}
			lines = append(lines, m.imageRows(line, a.link, width)...)
			continue
		}
		lines = append(lines, strings.Split(ansi.Wrap(line, max(1, width), ""), "\n")...)
	}
	if len(a.links) > 0 {
		lines = append(lines, "", "## Länkar", "")
		seen := map[string]bool{}
		for i, href := range a.links {
			url := resolveURL(a.link, href)
			if seen[url] {
				continue
			}
			seen[url] = true
			item := fmt.Sprintf("%d. %s%s%s", len(seen), parser.LinkStart(i+1), strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://"), parser.LinkOff)
			lines = append(lines, strings.Split(ansi.Wrap(item, max(1, width), "/"), "\n")...)
		}
	}
	if a.link != "" {
		// Many feeds only carry a teaser; the last link is always the article itself.
		lines = append(lines, "", parser.LinkStart(len(a.links)+1)+"Läs hela artikeln"+parser.LinkOff)
	}
	return lines, imageAt
}

// articleLinks are the current article's links as absolute URLs, numbered like the
// parser's link markers, followed by the article's own link.
func (m Model) articleLinks() []string {
	a := m.currentArticle()
	if a == nil {
		return nil
	}
	var links []string
	for _, href := range a.links {
		links = append(links, resolveURL(a.link, href))
	}
	if a.link != "" {
		links = append(links, a.link)
	}
	return links
}

// lineLink is a link on one reader line: the part of its text on that line, and its URL.
type lineLink struct{ text, url string }

// linksInLine returns the links on reader line i, in order.
func (m Model) linksInLine(lines []string, i int) []lineLink {
	if i < 0 || i >= len(lines) || graphics.IsPlaceholderRow(lines[i]) {
		return nil
	}
	urls := m.articleLinks()
	var links []lineLink
	state, text := inlineAtStart(lines)[i], strings.Builder{}
	flush := func() {
		if state.link > 0 && state.link <= len(urls) && strings.TrimSpace(text.String()) != "" {
			links = append(links, lineLink{text: strings.TrimSpace(text.String()), url: urls[state.link-1]})
		}
		text.Reset()
	}
	for _, r := range lines[i] {
		if next := state; next.apply(r) {
			if next.link != state.link {
				flush()
			}
			state = next
			continue
		}
		text.WriteRune(r)
	}
	flush()
	return links
}

// linkInLine returns the URL of the link under display column col of reader line i.
func (m Model) linkInLine(lines []string, i, col int) string {
	if i < 0 || i >= len(lines) || graphics.IsPlaceholderRow(lines[i]) {
		return ""
	}
	links := m.articleLinks()
	state, x := inlineAtStart(lines)[i], 0
	for _, r := range lines[i] {
		if next := state; next.apply(r) {
			state = next
			continue
		}
		w := ansi.StringWidth(string(r))
		if state.link > 0 && state.link <= len(links) && col >= x && col < x+w {
			return links[state.link-1]
		}
		x += w
	}
	return ""
}

// linkAt returns the URL of the link at a mouse position in the reader, if any.
func (m Model) linkAt(x, y int) string {
	i, col, ok := m.readerLineAt(x, y)
	if !ok || col < 0 {
		return ""
	}
	return m.linkInLine(m.readerLines(m.readerWidth()), i, col)
}

// readerLineAt returns the reader line and text column under a mouse position. Rows
// above or below the visible text give the line just outside it, so dragging past the
// edge can scroll; ok is false outside the reader.
func (m Model) readerLineAt(x, y int) (line, col int, ok bool) {
	left, _, height := m.dimensions()
	panel := left
	switch {
	case m.threeColumns():
		list, _ := m.columns()
		panel = left + list
	case !m.reading || m.form != nil || m.help || m.logOpen:
		return 0, 0, false
	}
	if x < panel || x >= m.width {
		return 0, 0, false
	}
	// The reader's text starts after the border, padding and "> " prefix, and below
	// the header row, the border and the panel title.
	row, col := y-3, x-panel-4
	lines := m.readerLines(m.readerWidth())
	visible := height - 4
	scroll := m.scroll
	if !m.reading {
		scroll = 0
	}
	start := min(scroll, max(0, len(lines)-visible))
	return clamp(start+row, len(lines)), col, true
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
	levels, states := headingLevels(lines), inlineAtStart(lines)
	look := readerLook{pattern: pattern, match: matchStyle, links: m.articleLinks(), linkColor: p.feed,
		hintStyle: lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(p.warn)}
	if m.hints != nil {
		look.hints = m.hints.labels
	}
	// readerLines starts with the article title (possibly wrapped), then the date line.
	titleRows := 0
	if a := m.currentArticle(); a != nil {
		titleRows = len(strings.Split(ansi.Wrap(a.title, max(1, m.readerWidth()), ""), "\n"))
	}
	var rendered []string
	for i := start; i < min(len(lines), start+height); i++ {
		prefix := "  "
		if m.reading && i == m.line {
			prefix = "> "
		}
		style := headingStyle(p, levels[i])
		if i < titleRows {
			style = p.normal().Bold(true).Foreground(p.accent)
		} else if i == titleRows {
			style = p.normal().Foreground(p.muted)
		}
		if strings.HasPrefix(lines[i], parser.Code) && p.code != nil {
			style = style.Background(p.code)
		}
		switch {
		case m.reading && m.visual && i >= low && i <= high:
			style = p.selected(true)
		case m.reading && i == m.line:
			style = style.Underline(true)
		}
		if graphics.IsPlaceholderRow(lines[i]) {
			rendered = append(rendered, renderImageRow(prefix, lines[i], width, style))
			continue
		}
		rendered = append(rendered, renderReaderLine(prefix, lines[i], states[i], width, style, look))
	}
	return strings.Join(rendered, "\n")
}

// inline is the style state at a point in a reader line, switched by the parser's
// markers: bold, italic, and the number of the link the text belongs to (0 for none).
type inline struct {
	bold, italic bool
	link         int
}

// readerLook holds what every line of one reader render shares.
type readerLook struct {
	pattern   *regexp.Regexp // search matches to highlight, or nil
	match     lipgloss.Style
	links     []string // absolute URLs, by link number - 1
	linkColor color.Color
	hints     map[int]string // O's labels, by link number, drawn before each link
	hintStyle lipgloss.Style
}

var headingMarker = regexp.MustCompile(`^(#{1,6}) `)

// plainLine is a reader line as search and copy see it: no style markers, plain spaces.
func plainLine(line string) string {
	if graphics.IsPlaceholderRow(line) {
		return ""
	}
	return parser.Plain(line)
}

// apply updates the state for one marker and reports whether r was a marker.
func (s *inline) apply(r rune) bool {
	switch string(r) {
	case parser.BoldOn:
		s.bold = true
	case parser.BoldOff:
		s.bold = false
	case parser.ItalicOn:
		s.italic = true
	case parser.ItalicOff:
		s.italic = false
	case parser.LinkOff:
		s.link = 0
	case parser.Code:
	default:
		n, ok := parser.LinkNumber(r)
		if !ok {
			return false
		}
		s.link = n
	}
	return true
}

// headingLevels gives each line its heading level (0 for none). A heading wraps onto
// following lines until the blank line that ends it.
func headingLevels(lines []string) []int {
	levels := make([]int, len(lines))
	level := 0
	for i, line := range lines {
		if marker := headingMarker.FindStringSubmatch(line); marker != nil {
			level = len(marker[1])
		} else if strings.TrimSpace(line) == "" {
			level = 0
		}
		levels[i] = level
	}
	return levels
}

// inlineAtStart gives each line the bold/italic state left open by earlier lines, so
// styles continue across wrapped lines; a blank line ends a paragraph and closes them.
func inlineAtStart(lines []string) []inline {
	states := make([]inline, len(lines))
	var state inline
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			state = inline{}
		}
		states[i] = state
		for _, r := range line {
			state.apply(r)
		}
	}
	return states
}

func headingStyle(p palette, level int) lipgloss.Style {
	switch level {
	case 0:
		return p.normal()
	case 1:
		return p.normal().Bold(true).Foreground(p.accent)
	case 2:
		return p.normal().Bold(true).Foreground(p.category)
	case 3:
		return p.normal().Bold(true).Foreground(p.feed)
	}
	return p.normal().Bold(true)
}

// renderReaderLine draws one reader line at exactly width cells. Each piece is styled on
// its own, so bold/italic text and search matches don't reset the line's heading, cursor
// or selection style.
func renderReaderLine(prefix, line string, state inline, width int, style lipgloss.Style, look readerLook) string {
	out := style.Render(prefix)
	var part strings.Builder
	labelled := 0 // the link whose hint label was drawn last, so each link gets it once
	flush := func() {
		if part.Len() == 0 {
			return
		}
		partStyle := style.Bold(state.bold || style.GetBold()).Italic(state.italic)
		if label := look.hints[state.link]; label != "" && state.link != labelled {
			out += look.hintStyle.Render(label)
		}
		labelled = state.link
		if state.link > 0 && state.link <= len(look.links) {
			// An OSC 8 hyperlink: terminals that support it make the text clickable.
			url := look.links[state.link-1]
			out += ansi.SetHyperlink(url) +
				renderPart(part.String(), partStyle.Foreground(look.linkColor).Underline(true), look.pattern, look.match) +
				ansi.ResetHyperlink()
		} else {
			out += renderPart(part.String(), partStyle, look.pattern, look.match)
		}
		part.Reset()
	}
	for _, r := range line {
		if next := state; next.apply(r) {
			flush()
			state = next
			continue
		}
		part.WriteRune(r)
	}
	flush()
	if w := lipgloss.Width(out); w > width {
		// Truncating can cut off the end of a hyperlink; close it so it can't run on.
		return ansi.Truncate(out, width, "…") + ansi.ResetHyperlink()
	} else if w < width {
		out += style.Render(strings.Repeat(" ", width-w))
	}
	return out
}

// renderPart draws text in one style, with search matches highlighted.
func renderPart(part string, partStyle lipgloss.Style, pattern *regexp.Regexp, matchStyle lipgloss.Style) string {
	out, last := "", 0
	if pattern != nil {
		for _, match := range pattern.FindAllStringIndex(part, -1) {
			if match[0] > last {
				out += partStyle.Render(part[last:match[0]])
			}
			out += matchStyle.Render(part[match[0]:match[1]])
			last = match[1]
		}
	}
	if last < len(part) {
		out += partStyle.Render(part[last:])
	}
	return out
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
		intro = "Hur ofta flödet ska uppdateras, t.ex. 30m, 2h, 1d eller 1w."
	}
	if f.err != "" {
		intro = f.err
	}
	lines := []string{ansi.Wrap(intro, width, ""), ""}
	// Split the wrapped introduction so viewport positions count terminal rows.
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	activeLine := 0
	labels := []string{"URL", "Namn (valfritt)", "Kategori", "Intervall (t.ex. 15m, 2h, 1d)"}
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
Enter öppna · Tab byt panel · Alt+1/2 välj panel · Alt+3 fokusera läsvyn
5j  En siffra före en rörelse upprepar den: 5j, 12k, 3J, 2], 20G till rad 20
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
B  Notiser för valt flöde av/på (♪ i listan) · Alt+b alla notiser av/på
r  Uppdatera flödena

Övrigt
t  Växla tema
I  Visa/dölj bilder i artiklar (kitty och Ghostty)
o  Öppna länken på markörens rad (flera: välj med siffra), annars artikeln · klicka på en länk för att öppna den
O  Visa bokstäver vid alla synliga länkar; skriv bokstäverna för att öppna en
F  Hämta hela artikeln från webbsidan (för flöden med bara en ingress)
< / >  Smalare/bredare kolumn (kanten intill fokus) · = standardbredd
Dra en kolumnkant med musen för att ändra bredden
Mus: klicka för att välja (två klick öppnar artikeln) · hjulet rullar · dra över texten för att kopiera rader · klicka på statusraden för att kopiera meddelandet
?  Hjälp · Esc stäng · q avsluta (y bekräftar)
!  Visa loggen (r läser om, Esc stänger)
Ctrl+c  Avsluta, även i formulär
Urklipp använder OSC52 (t.ex. Ghostty).`
