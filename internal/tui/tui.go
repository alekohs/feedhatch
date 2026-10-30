package tui

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"alekohs/feedhatch/internal/store"
)

type article struct {
	id                int64
	title, date, body string
	unread            bool
}

type subscription struct {
	id, interval        int64
	name, url, category string
	articles            []article
}

type panel int

const (
	feedsPanel panel = iota
	articlesPanel
)

type form struct {
	single bool // edit only values[field] of the selected feed
	field  int
	values [4]string // URL, name, category, interval
	err    string
}

type Model struct {
	store         *store.Store
	width, height int
	feeds         []subscription
	feedIndex     int
	articleIndex  int
	focus         panel
	reading       bool
	scroll        int
	line          int
	visual        bool
	anchor        int
	pendingZ      bool
	status        string
	help          bool
	helpScroll    int
	terminalTheme bool
	form          *form
	search        searchState
}

// FetchedMsg tells the TUI that the worker finished a round and the store may have new articles.
type FetchedMsg struct{ Err error }

func New(st *store.Store) (Model, error) {
	m := Model{store: st}
	if err := m.load(); err != nil {
		return Model{}, err
	}
	return m, nil
}

func (m *Model) load() error {
	feeds, err := m.store.Feeds()
	if err != nil {
		return err
	}

	m.feeds = nil
	for _, f := range feeds {
		articles, err := m.store.Articles(f.ID)
		if err != nil {
			return err
		}
		sub := subscription{id: f.ID, interval: f.Interval, name: f.Name, url: f.URL, category: f.Category}
		for _, a := range articles {
			sub.articles = append(sub.articles, article{
				id: a.ID, title: a.Title, date: formatDate(a.PublishedAt), body: a.Content, unread: !a.Read,
			})
		}
		m.feeds = append(m.feeds, sub)
	}
	return nil
}

// reload keeps the selected feed and article, since new articles shift their positions.
func (m *Model) reload() error {
	var feedID, articleID int64
	if len(m.feeds) > 0 {
		feedID = m.feeds[m.feedIndex].id
	}
	if a := m.currentArticle(); a != nil {
		articleID = a.id
	}
	if err := m.load(); err != nil {
		return err
	}

	m.feedIndex, m.articleIndex = clamp(m.feedIndex, len(m.feeds)), 0
	for i, f := range m.feeds {
		if f.id == feedID {
			m.feedIndex = i
		}
	}
	if len(m.feeds) > 0 {
		for i, a := range m.feeds[m.feedIndex].articles {
			if a.id == articleID {
				m.articleIndex = i
			}
		}
	}
	return nil
}

var months = []string{"jan", "feb", "mar", "apr", "maj", "jun", "jul", "aug", "sep", "okt", "nov", "dec"}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.Local()
	now := time.Now()
	switch {
	case sameDay(t, now):
		return "Idag · " + t.Format("15:04")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "Igår · " + t.Format("15:04")
	}
	return fmt.Sprintf("%d %s · %s", t.Day(), months[t.Month()-1], t.Format("15:04"))
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case FetchedMsg:
		if err := m.reload(); err != nil {
			m.status = "Kunde inte läsa in flödena: " + err.Error()
		} else if msg.Err != nil {
			m.status = "Uppdatering misslyckades: " + strings.ReplaceAll(msg.Err.Error(), "\n", "; ")
		}
		m.ensureLineVisible()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Wrapped row numbers change on resize; do not silently copy a new range.
		m.visual = false
		m.pendingZ = false
		m.ensureLineVisible()
	case tea.PasteMsg:
		if m.form != nil {
			m.appendInput(msg.Content)
		} else if m.search.active {
			m.appendSearchInput(msg.Content)
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.form != nil {
			m.updateForm(msg)
			return m, nil
		}
		if m.search.active {
			m.updateSearch(msg)
			return m, nil
		}
		if m.help {
			if delta, ok := m.pageMotion(key); ok {
				m.helpScroll = clamp(m.helpScroll+delta, m.maxHelpScroll()+1)
				return m, nil
			}
			switch key {
			case "q":
				return m, tea.Quit
			case "?", "esc", "h", "enter":
				m.help = false
			case "t":
				m.terminalTheme = !m.terminalTheme
			case "j", "down":
				m.helpScroll++
			case "k", "up":
				m.helpScroll--
			case "g", "home":
				m.helpScroll = 0
			case "G", "end":
				m.helpScroll = 1 << 20
			}
			m.helpScroll = clamp(m.helpScroll, m.maxHelpScroll()+1)
			return m, nil
		}
		m.status = ""
		previousZ := m.pendingZ
		m.pendingZ = false
		if key == "z" && m.reading {
			if previousZ {
				m.centerLine()
			} else {
				m.pendingZ = true
			}
			return m, nil
		}
		if delta, ok := m.pageMotion(key); ok {
			m.move(delta)
			return m, nil
		}
		switch key {
		case "q":
			return m, tea.Quit
		case "/":
			if m.reading {
				m.search.active, m.search.input = true, ""
			}
		case "n", "N":
			if m.reading {
				direction := 1
				if key == "N" {
					direction = -1
				}
				m.findSearchMatch(direction, false)
			}
		case "?":
			m.help = true
			m.helpScroll = 0
		case "t":
			m.terminalTheme = !m.terminalTheme
		case "a":
			m.form = &form{}
			m.form.values[2], m.form.values[3] = "Övrigt", "15"
			if len(m.feeds) > 0 {
				m.form.values[2] = m.feeds[m.feedIndex].category
			}
		case "c":
			if len(m.feeds) > 0 {
				m.form = &form{single: true, field: 2}
				m.form.values[2] = m.feeds[m.feedIndex].category
			}
		case "i":
			if len(m.feeds) > 0 {
				m.form = &form{single: true, field: 3}
				m.form.values[3] = strconv.FormatInt(m.feeds[m.feedIndex].interval, 10)
			}
		case "h", "left", "esc":
			if m.visual {
				m.visual = false
			} else if m.reading {
				m.reading, m.scroll = false, 0
			} else {
				m.focus = feedsPanel
			}
		case "l", "right", "enter":
			if m.focus == feedsPanel {
				m.focus = articlesPanel
			} else if !m.reading {
				m.openArticle()
			}
		case "tab", "shift+tab":
			if m.focus == feedsPanel {
				m.focus = articlesPanel
			} else {
				m.focus = feedsPanel
				m.reading, m.scroll, m.visual = false, 0, false
			}
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "g", "home":
			m.move(-1 << 20)
		case "G", "end":
			m.move(1 << 20)
		case "{":
			m.moveParagraph(-1)
		case "}":
			m.moveParagraph(1)
		case "H", "M", "L":
			m.moveWithinScreen(key)
		case "m":
			if a := m.currentArticle(); a != nil {
				a.unread = !a.unread
				m.saveRead(a)
			}
		case "v":
			if m.reading {
				m.visual = !m.visual
				m.anchor = m.line
			}
		case "y":
			if m.reading {
				text, count := m.selectedText()
				m.visual = false
				m.status = fmt.Sprintf("Skickade %d rad(er) till urklipp via OSC52.", count)
				return m, tea.SetClipboard(text)
			}
		}
	}
	return m, nil
}

func (m *Model) currentArticle() *article {
	if len(m.feeds) == 0 || len(m.feeds[m.feedIndex].articles) == 0 {
		return nil
	}
	return &m.feeds[m.feedIndex].articles[m.articleIndex]
}

func (m *Model) openArticle() {
	if a := m.currentArticle(); a != nil {
		if a.unread {
			a.unread = false
			m.saveRead(a)
		}
		m.reading, m.scroll, m.line, m.visual = true, 0, 0, false
	}
}

func (m *Model) saveRead(a *article) {
	if err := m.store.SetRead(a.id, !a.unread); err != nil {
		m.status = "Kunde inte spara läsläget: " + err.Error()
	}
}

func (m *Model) move(delta int) {
	if m.focus == feedsPanel {
		order := m.feedOrder()
		for i, index := range order {
			if index == m.feedIndex {
				m.feedIndex = order[clamp(i+delta, len(order))]
				m.articleIndex, m.scroll = 0, 0
				return
			}
		}
	} else if m.reading {
		_, right, _ := m.dimensions()
		m.line = clamp(m.line+delta, len(m.readerLines(right-6)))
		m.ensureLineVisible()
	} else if len(m.feeds) > 0 {
		m.articleIndex = clamp(m.articleIndex+delta, len(m.feeds[m.feedIndex].articles))
	}
}

func clamp(value, count int) int { return max(0, min(value, count-1)) }

func (m Model) categories() []string {
	var categories []string
	seen := make(map[string]bool)
	for _, feed := range m.feeds {
		if !seen[feed.category] {
			categories = append(categories, feed.category)
			seen[feed.category] = true
		}
	}
	return categories
}

func (m Model) feedOrder() []int {
	var order []int
	for _, category := range m.categories() {
		for i, feed := range m.feeds {
			if feed.category == category {
				order = append(order, i)
			}
		}
	}
	return order
}

func unreadCount(feed subscription) int {
	n := 0
	for _, a := range feed.articles {
		if a.unread {
			n++
		}
	}
	return n
}

func (m *Model) appendInput(text string) {
	f := m.form
	// Pasted terminal controls must never become part of a rendered label.
	text = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, text)
	value := []rune(f.values[f.field] + text)
	if len(value) > 2048 {
		value = value[:2048]
	}
	f.values[f.field], f.err = string(value), ""
}

func (m *Model) updateForm(msg tea.KeyPressMsg) {
	f := m.form
	switch msg.String() {
	case "esc":
		m.form = nil
	case "tab", "down":
		if !f.single {
			f.field = (f.field + 1) % len(f.values)
		}
	case "shift+tab", "up":
		if !f.single {
			f.field = (f.field + len(f.values) - 1) % len(f.values)
		}
	case "backspace":
		runes := []rune(f.values[f.field])
		if len(runes) > 0 {
			f.values[f.field] = string(runes[:len(runes)-1])
		}
		f.err = ""
	case "ctrl+u":
		f.values[f.field], f.err = "", ""
	case "enter":
		if !f.single && f.field < len(f.values)-1 {
			f.field++
			return
		}
		m.saveForm()
	default:
		m.appendInput(msg.Text)
	}
}

func (m *Model) saveForm() {
	f := m.form
	category := strings.TrimSpace(f.values[2])
	if category == "" {
		category = "Övrigt"
	}
	interval, err := strconv.ParseInt(strings.TrimSpace(f.values[3]), 10, 64)
	if (!f.single || f.field == 3) && (err != nil || interval < 1) {
		f.err, f.field = "Ange intervallet i hela minuter, minst 1.", 3
		return
	}
	switch {
	case f.single && f.field == 2:
		if err := m.store.SetCategory(m.feeds[m.feedIndex].id, category); err != nil {
			f.err = "Kunde inte spara kategorin: " + err.Error()
			return
		}
		m.feeds[m.feedIndex].category = category
	case f.single:
		if err := m.store.SetInterval(m.feeds[m.feedIndex].id, interval); err != nil {
			f.err = "Kunde inte spara intervallet: " + err.Error()
			return
		}
		m.feeds[m.feedIndex].interval = interval
	default:
		address := strings.TrimSpace(f.values[0])
		u, err := url.Parse(address)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			f.err, f.field = "Ange en giltig http:// eller https://-adress.", 0
			return
		}
		name := strings.TrimSpace(f.values[1])
		if name == "" {
			name = u.Hostname()
		}
		for _, feed := range m.feeds {
			if feed.url == address {
				f.err, f.field = "Flödet finns redan.", 0
				return
			}
		}
		id, err := m.store.AddFeed(store.Feed{URL: address, Name: name, Category: category, Interval: interval})
		if err != nil {
			f.err, f.field = "Kunde inte spara flödet: "+err.Error(), 0
			return
		}
		m.feeds = append(m.feeds, subscription{id: id, interval: interval, name: name, url: address, category: category})
		m.feedIndex, m.articleIndex = len(m.feeds)-1, 0
	}
	m.form, m.focus, m.reading, m.scroll, m.visual = nil, feedsPanel, false, 0, false
}

func (m *Model) ensureLineVisible() {
	_, right, height := m.dimensions()
	m.line = clamp(m.line, len(m.readerLines(right-6)))
	visible := max(1, height-4)
	if m.line < m.scroll {
		m.scroll = m.line
	}
	if m.line >= m.scroll+visible {
		m.scroll = m.line - visible + 1
	}
	m.scroll = min(m.scroll, m.maxScroll())
}

func (m Model) selectedText() (string, int) {
	_, right, _ := m.dimensions()
	lines := m.readerLines(right - 6)
	start, end := m.selectionRange()
	start, end = clamp(start, len(lines)), clamp(end, len(lines))
	return strings.Join(lines[start:end+1], "\n"), end - start + 1
}

func (m Model) selectionRange() (int, int) {
	if !m.visual {
		return m.line, m.line
	}
	return min(m.anchor, m.line), max(m.anchor, m.line)
}

func (m Model) themeName() string {
	if m.terminalTheme {
		return "terminal"
	}
	return "bas"
}

func (m Model) summary() string {
	n := 0
	for _, feed := range m.feeds {
		n += unreadCount(feed)
	}
	return fmt.Sprintf("%d flöden · %d olästa", len(m.feeds), n)
}
