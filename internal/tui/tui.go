package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/parser"
	"alekohs/feedhatch/internal/store"
)

type article struct {
	id                int64
	title, date, body string
	link              string
	links             []string // the body's link URLs, by the parser's link numbers
	image             string   // cover image URL, from the feed item rather than the body
	published         time.Time
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

type listView int

const (
	feedView listView = iota
	unreadView
	allView
)

type ref struct{ feed, article int } // indices into m.feeds and its articles

type tone int

const (
	toneInfo tone = iota
	toneOK
	toneWarn
	toneErr
)

type form struct {
	single bool // edit only values[field] of the selected feed
	field  int
	values [4]string // URL, name, category, interval
	err    string
}

type Model struct {
	store           *store.Store
	updates         <-chan FetchedMsg
	announce        func(event.Event)
	width, height   int
	feeds           []subscription
	feedIndex       int
	articleIndex    int
	focus           panel
	reading         bool
	scroll          int
	line            int
	visual          bool
	anchor          int
	pendingZ        bool
	pendingDelete   bool
	status          string
	help            bool
	helpScroll      int
	logOpen         bool
	logPath         string
	logLines        []string
	logScroll       int
	logGen          int
	sidebarWidth    int // 0 uses the default width; set by dragging or < and >
	listWidth       int
	dragging        border
	gallery         *gallery
	imageDir        string
	imagesSupported bool
	imagesOn        bool // images load automatically where supported, until I turns them off
	terminalTheme   bool
	form            *form
	search          searchState
	filter          string
	filtering       bool
	mock            bool
	view            listView
	keep            map[int64]bool
	pendingReadAll  bool
	pendingLinks    []lineLink // o on a line with several links: waiting for a digit
	count           int        // a Vim-style count typed before a motion, e.g. the 5 in 5j
	updated         time.Time
	fetchFailed     bool
	tone            tone
}

// FetchedMsg tells the TUI that the worker finished a round and the store may have new articles.
type FetchedMsg struct{ Err error }

// New loads the feeds from st. Each FetchedMsg on updates reloads them, and announce
// is called after every change the TUI saves, so other listeners hear about it.
// mock marks the session as sample data. Both updates and announce may be nil.
// Config holds the TUI's settings from the command line and environment.
type Config struct {
	Mock     bool
	LogPath  string // the log file shown with !
	ImageDir string // the disk cache for article images
	Images   bool   // the terminal can show images (kitty graphics protocol)
}

func New(st *store.Store, updates <-chan FetchedMsg, announce func(event.Event), cfg Config) (Model, error) {
	m := Model{
		store: st, updates: updates, announce: announce, mock: cfg.Mock, logPath: cfg.LogPath, terminalTheme: true,
		imageDir: cfg.ImageDir, imagesSupported: cfg.Images, imagesOn: cfg.Images, gallery: newGallery(),
	}
	if err := m.load(); err != nil {
		return Model{}, err
	}
	if m.totalUnread() > 0 {
		m.selectView(unreadView, 0)
	} else if order := m.feedOrder(); len(order) > 0 {
		m.selectView(feedView, order[0])
	}
	return m, nil
}

// selectView switches the sidebar entry. keep only survives while the entry stays the same,
// so articles read in "Olästa" don't vanish until the user moves on.
func (m *Model) selectView(v listView, feedIndex int) {
	changed := v != m.view || (v == feedView && feedIndex != m.feedIndex)
	m.view = v
	if v == feedView {
		m.feedIndex = feedIndex
	}
	m.articleIndex, m.scroll, m.reading, m.visual = 0, 0, false, false
	if changed {
		m.keep = nil
		m.filter, m.filtering = "", false
		if v == unreadView {
			m.fillKeep()
		}
	}
}

func (m *Model) fillKeep() {
	m.keep = make(map[int64]bool)
	for _, feed := range m.feeds {
		for _, a := range feed.articles {
			if a.unread {
				m.keep[a.id] = true
			}
		}
	}
}

func (m Model) totalUnread() int {
	n := 0
	for _, feed := range m.feeds {
		n += unreadCount(feed)
	}
	return n
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
			body, links := parser.Text(a.Content)
			title, _ := parser.Text(a.Title)
			title = plainLine(title)

			sub.articles = append(sub.articles, article{
				id: a.ID, title: title, date: formatDate(a.PublishedAt), body: body, link: a.Link, links: links, image: a.Image,
				published: a.PublishedAt, unread: !a.Read,
			})
		}
		m.feeds = append(m.feeds, sub)
	}
	return nil
}

// reload keeps the selected feed and article, since new articles shift their positions.
// With refill, the Olästa list is rebuilt before the cursor is placed.
func (m *Model) reload(refill bool) error {
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
	if refill && m.view == unreadView {
		m.fillKeep()
	}

	feedGone := feedID != 0
	m.feedIndex = clamp(m.feedIndex, len(m.feeds))
	for i, f := range m.feeds {
		if f.id == feedID {
			m.feedIndex, feedGone = i, false
		}
	}
	refs := m.articleRefs()
	m.articleIndex = clamp(m.articleIndex, len(refs))
	articleGone := articleID != 0
	for i, r := range refs {
		if m.feeds[r.feed].articles[r.article].id == articleID {
			m.articleIndex, articleGone = i, false
		}
	}
	if articleGone {
		m.reading, m.visual = false, false
	}
	if feedGone && m.view == feedView {
		if m.form != nil && m.form.single {
			m.form = nil
		}
		m.pendingDelete = false
		m.reading, m.visual = false, false
		m.status, m.tone = "Flödet togs bort av en annan klient.", toneWarn
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

func (m Model) Init() tea.Cmd { return m.waitForUpdate() }

func (m Model) waitForUpdate() tea.Cmd {
	if m.updates == nil {
		return nil
	}
	updates := m.updates
	return func() tea.Msg { return <-updates }
}

// Update handles msg, then starts loading and placing the images of the article on
// screen, since nearly any message can change which article or how much room it has.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.update(msg)
	next := updated.(Model)
	return next, tea.Batch(cmd, next.imageCmds())
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case FetchedMsg:
		m.updated, m.fetchFailed = time.Now(), msg.Err != nil
		if err := m.reload(false); err != nil {
			m.status, m.tone = "Kunde inte läsa in flödena: "+err.Error(), toneErr
		} else if msg.Err != nil && !m.pendingDelete && !m.pendingReadAll {
			m.status, m.tone = "Uppdatering misslyckades: "+strings.ReplaceAll(msg.Err.Error(), "\n", "; "), toneErr
		}
		m.ensureLineVisible()
		return m, m.waitForUpdate()
	case logTickMsg:
		if !m.logOpen || int(msg) != m.logGen {
			return m, nil
		}
		m.loadLog()
		return m, m.logTick()
	case imageLoadedMsg:
		m.imageLoaded(msg)
	case tea.MouseClickMsg:
		if mouse := msg.Mouse(); mouse.Button == tea.MouseLeft {
			m.dragging = m.borderAt(mouse.X, mouse.Y)
			if url := m.linkAt(mouse.X, mouse.Y); m.dragging == noBorder && url != "" {
				m.openLink(url)
			}
		}
	case tea.MouseMotionMsg:
		if m.dragging != noBorder {
			m.moveBorder(m.dragging, msg.Mouse().X)
		}
	case tea.MouseReleaseMsg:
		m.dragging = noBorder
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Wrapped row numbers change on resize; do not silently copy a new range.
		m.visual = false
		m.pendingZ = false
		m.ensureLineVisible()
	case tea.PasteMsg:
		if m.form != nil {
			m.appendInput(msg.Content)
		} else if m.filtering {
			m.setFilter(m.filter + printable(msg.Content))
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
		if m.filtering {
			m.updateFilter(msg)
			return m, nil
		}
		if m.search.active {
			m.updateSearch(msg)
			return m, nil
		}
		if m.logOpen {
			if delta, ok := m.pageMotion(key); ok {
				m.logScroll = clamp(m.logScroll+delta, m.maxLogScroll()+1)
				return m, nil
			}
			switch key {
			case "q":
				return m, tea.Quit
			case "!", "esc", "h":
				m.logOpen = false
			case "r":
				m.loadLog()
			case "t":
				m.terminalTheme = !m.terminalTheme
			case "j", "down":
				m.logScroll++
			case "k", "up":
				m.logScroll--
			case "g", "home":
				m.logScroll = 0
			case "G", "end":
				m.logScroll = 1 << 20
			}
			m.logScroll = clamp(m.logScroll, m.maxLogScroll()+1)
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
		m.status, m.tone = "", toneInfo
		if links := m.pendingLinks; links != nil {
			m.pendingLinks = nil
			if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(links) {
				m.openLink(links[n-1].url)
			} else {
				m.status = "Avbröts."
			}
			return m, nil
		}
		if m.pendingDelete {
			m.pendingDelete = false
			if key == "y" {
				m.deleteFeed()
			} else {
				m.status = "Borttagningen avbröts."
			}
			return m, nil
		}
		if m.pendingReadAll {
			m.pendingReadAll = false
			if key == "y" {
				m.markAllRead()
			} else {
				m.status = "Avbröts."
			}
			return m, nil
		}
		if len(key) == 1 && key >= "0" && key <= "9" && (key != "0" || m.count > 0) {
			m.count = min(m.count*10+int(key[0]-'0'), 9999)
			m.status = strconv.Itoa(m.count)
			return m, nil
		}
		// Every other key uses up the count; only motions repeat with it.
		n, counted := max(1, m.count), m.count > 0
		m.count = 0
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
		if key == "space" || key == " " {
			if m.reading && m.line >= len(m.readerLines(m.readerWidth()))-1 {
				m.openUnread(1)
				return m, nil
			}
		}
		if delta, ok := m.pageMotion(key); ok {
			m.move(delta * n)
			return m, nil
		}
		switch key {
		case "q":
			return m, tea.Quit
		case "]", "[", "J", "K":
			for range n {
				switch key {
				case "]":
					m.openUnread(1)
				case "[":
					m.openUnread(-1)
				case "J":
					m.stepArticle(1)
				case "K":
					m.stepArticle(-1)
				}
			}
		case "alt+1":
			m.focus = feedsPanel
			m.reading, m.scroll, m.visual = false, 0, false
		case "alt+2":
			m.focus = articlesPanel
			if m.wide() {
				m.reading, m.scroll, m.visual = false, 0, false
			}
		case "alt+3":
			m.focus = articlesPanel
			if !m.reading {
				m.openArticle()
			}
		case "r":
			if err := m.reload(true); err != nil {
				m.status, m.tone = "Kunde inte läsa in flödena: "+err.Error(), toneErr
				break
			}
			m.updated = time.Now()
			m.status, m.tone = "Uppdaterad.", toneOK
		case "/":
			if m.reading {
				m.search.active, m.search.input = true, ""
			} else {
				m.focus, m.filtering = articlesPanel, true
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
		case "o":
			if m.reading || m.threeColumns() {
				m.openFromLine()
			}
		case "I":
			switch {
			case !m.imagesSupported:
				m.status, m.tone = "Terminalen kan inte visa bilder (kräver kitty eller Ghostty).", toneWarn
			case m.imagesOn:
				m.imagesOn = false
				m.status = "Bilder avstängda."
			default:
				m.imagesOn = true
				m.status = "Bilder påslagna."
			}
			m.ensureLineVisible()
		case "<":
			m.resize(-resizeStep)
		case ">":
			m.resize(resizeStep)
		case "=":
			m.sidebarWidth, m.listWidth = 0, 0
			m.visual = false
			m.ensureLineVisible()
		case "!":
			m.logOpen, m.logScroll = true, 1<<20
			m.logGen++
			m.loadLog()
			return m, m.logTick()
		case "t":
			m.terminalTheme = !m.terminalTheme
		case "a":
			m.form = &form{}
			m.form.values[2], m.form.values[3] = "Övrigt", "15"
			if len(m.feeds) > 0 && m.view == feedView {
				m.form.values[2] = m.feeds[m.feedIndex].category
			}
		case "c":
			if m.requireFeed() && len(m.feeds) > 0 {
				m.form = &form{single: true, field: 2}
				m.form.values[2] = m.feeds[m.feedIndex].category
			}
		case "A":
			if m.view != feedView {
				if n := m.totalUnread(); n == 0 {
					m.status = "Inga olästa artiklar."
				} else {
					m.pendingReadAll = true
					m.status, m.tone = fmt.Sprintf("Markera alla %d olästa artiklar som lästa? y bekräftar, annan tangent avbryter.", n), toneWarn
				}
			} else if len(m.feeds) > 0 {
				m.markFeedRead()
			}
		case "D":
			if m.requireFeed() && len(m.feeds) > 0 {
				m.pendingDelete = true
				m.status, m.tone = fmt.Sprintf("Ta bort %s och alla dess artiklar? y tar bort, annan tangent avbryter.", m.feeds[m.feedIndex].name), toneWarn
			}
		case "i":
			if m.requireFeed() && len(m.feeds) > 0 {
				m.form = &form{single: true, field: 3}
				m.form.values[3] = strconv.FormatInt(m.feeds[m.feedIndex].interval, 10)
			}
		case "h", "left", "esc":
			if m.visual {
				m.visual = false
			} else if key == "esc" && m.filter != "" && !m.reading && m.focus == articlesPanel {
				m.setFilter("")
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
			m.move(n)
		case "k", "up":
			m.move(-n)
		case "g", "home":
			m.move(-1 << 20)
		case "G", "end":
			m.move(1 << 20)
			if counted { // like Vim: 20G goes to line 20
				m.move(-1 << 20)
				m.move(n - 1)
			}
		case "{", "}":
			dir := 1
			if key == "{" {
				dir = -1
			}
			for range n {
				if m.focus == feedsPanel && !m.reading {
					m.moveCategory(dir)
				} else {
					m.moveParagraph(dir)
				}
			}
		case "H", "M", "L":
			m.moveWithinScreen(key)
		case "m":
			if a := m.currentArticle(); a != nil {
				m.setUnread(a, !a.unread)
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

func (m Model) articleRefs() []ref {
	var refs []ref
	switch m.view {
	case feedView:
		if len(m.feeds) > 0 {
			for i := range m.feeds[m.feedIndex].articles {
				refs = append(refs, ref{m.feedIndex, i})
			}
		}
	case unreadView:
		for f, feed := range m.feeds {
			for i, a := range feed.articles {
				if a.unread || m.keep[a.id] {
					refs = append(refs, ref{f, i})
				}
			}
		}
	case allView:
		for f, feed := range m.feeds {
			for i := range feed.articles {
				refs = append(refs, ref{f, i})
			}
		}
	}
	if m.filter != "" {
		pattern := searchPattern(m.filter)
		refs = slices.DeleteFunc(refs, func(r ref) bool {
			a := m.feeds[r.feed].articles[r.article]
			return !pattern.MatchString(a.title) && !pattern.MatchString(plainLine(a.body))
		})
	}
	if m.view != feedView {
		slices.SortStableFunc(refs, func(a, b ref) int {
			return m.feeds[b.feed].articles[b.article].published.Compare(m.feeds[a.feed].articles[a.article].published)
		})
	}
	return refs
}

func (m *Model) currentArticle() *article {
	refs := m.articleRefs()
	if m.articleIndex < 0 || m.articleIndex >= len(refs) {
		return nil
	}
	r := refs[m.articleIndex]
	return &m.feeds[r.feed].articles[r.article]
}

func (m *Model) currentFeed() *subscription {
	refs := m.articleRefs()
	if m.articleIndex >= 0 && m.articleIndex < len(refs) {
		return &m.feeds[refs[m.articleIndex].feed]
	}
	if len(m.feeds) == 0 {
		return nil
	}
	return &m.feeds[m.feedIndex]
}

func (m *Model) openArticle() {
	if a := m.currentArticle(); a != nil {
		if a.unread {
			m.setUnread(a, false)
		}
		m.reading, m.scroll, m.line, m.visual = true, 0, 0, false
		if m.filter != "" {
			m.search.query = m.filter
		}
	}
}

// openUnread opens the next (dir 1) or previous (dir -1) unread article, continuing into
// the neighbouring feeds in sidebar order when a feed is selected.
func (m *Model) openUnread(dir int) {
	seq, pos := m.articleRefs(), m.articleIndex
	if m.view == feedView {
		seq, pos = nil, 0
		for _, f := range m.feedOrder() {
			if f == m.feedIndex {
				pos = len(seq) + m.articleIndex
				if dir > 0 && len(m.articleRefs()) == 0 {
					pos--
				}
				seq = append(seq, m.articleRefs()...)
				continue
			}
			for i := range m.feeds[f].articles {
				seq = append(seq, ref{f, i})
			}
		}
	}
	for i := pos + dir; i >= 0 && i < len(seq); i += dir {
		r := seq[i]
		if !m.feeds[r.feed].articles[r.article].unread {
			continue
		}
		if m.view == feedView {
			if r.feed != m.feedIndex {
				m.status, m.tone = "Nästa oläst i "+m.feeds[r.feed].name+".", toneInfo
			}
			if r.feed != m.feedIndex {
				m.feedIndex, m.filter = r.feed, ""
			}
			m.articleIndex = slices.Index(m.articleRefs(), r)
		} else {
			m.articleIndex = i
		}
		m.focus = articlesPanel
		m.openArticle()
		return
	}
	m.status, m.tone = "Inga fler olästa artiklar.", toneInfo
}

func (m *Model) stepArticle(delta int) {
	m.articleIndex = clamp(m.articleIndex+delta, len(m.articleRefs()))
	if m.reading {
		m.openArticle()
	}
}

func (m *Model) requireFeed() bool {
	if m.view != feedView {
		m.status, m.tone = "Välj ett riktigt flöde.", toneWarn
		return false
	}
	return true
}

func (m *Model) markFeedRead() {
	if len(m.feeds) == 0 {
		return
	}
	feed := &m.feeds[m.feedIndex]
	n := unreadCount(*feed)
	if n == 0 {
		m.status = "Inga olästa artiklar i " + feed.name + "."
		return
	}
	if err := m.store.SetFeedRead(feed.id, true); err != nil {
		m.status, m.tone = "Kunde inte markera flödet som läst: "+err.Error(), toneErr
		return
	}
	for i := range feed.articles {
		feed.articles[i].unread = false
	}
	m.status, m.tone = fmt.Sprintf("Markerade %d artiklar i %s som lästa.", n, feed.name), toneOK
	m.publish(event.Event{Type: event.FeedRead, Feed: feed.name})
}

func (m *Model) markAllRead() {
	n := 0
	for i := range m.feeds {
		feed := &m.feeds[i]
		count := unreadCount(*feed)
		if count == 0 {
			continue
		}
		if err := m.store.SetFeedRead(feed.id, true); err != nil {
			m.status, m.tone = "Kunde inte markera flödet som läst: "+err.Error(), toneErr
			return
		}
		for j := range feed.articles {
			if feed.articles[j].unread && m.view == unreadView {
				m.keep[feed.articles[j].id] = true
			}
			feed.articles[j].unread = false
		}
		n += count
		m.publish(event.Event{Type: event.FeedRead, Feed: feed.name})
	}
	if n == 0 {
		m.status = "Inga olästa artiklar."
		return
	}
	m.status, m.tone = fmt.Sprintf("Markerade %d artiklar som lästa.", n), toneOK
}

func (m *Model) deleteFeed() {
	if len(m.feeds) == 0 {
		return
	}
	feed := m.feeds[m.feedIndex]
	if err := m.store.DeleteFeed(feed.id); err != nil {
		m.status, m.tone = "Kunde inte ta bort flödet: "+err.Error(), toneErr
		return
	}
	order := m.feedOrder()
	position := slices.Index(order, m.feedIndex)
	next := 0
	if position+1 < len(order) {
		next = order[position+1]
	} else if position > 0 {
		next = order[position-1]
	}
	if next > m.feedIndex {
		next--
	}
	m.feeds = slices.Delete(m.feeds, m.feedIndex, m.feedIndex+1)
	m.selectView(feedView, next)
	m.focus = feedsPanel
	m.status, m.tone = "Tog bort "+feed.name+".", toneOK
	m.publish(event.Event{Type: event.FeedDeleted, Feed: feed.name})
}

// setUnread changes the current article. In Olästa it is kept listed, so the cursor stays on it.
func (m *Model) setUnread(a *article, unread bool) {
	feed := m.currentFeed().name
	if m.view == unreadView {
		if m.keep == nil {
			m.keep = make(map[int64]bool)
		}
		m.keep[a.id] = true
	}
	a.unread = unread
	m.saveRead(a, feed)
}

func (m *Model) saveRead(a *article, feed string) {
	if err := m.store.SetRead(a.id, !a.unread); err != nil {
		m.status, m.tone = "Kunde inte spara läsläget: "+err.Error(), toneErr
		return
	}
	e := event.Event{Type: event.Read, Feed: feed, ArticleID: a.id, Article: a.title}
	if a.unread {
		e.Type = event.Unread
	}
	m.publish(e)
}

func (m *Model) publish(e event.Event) {
	if m.announce != nil {
		m.announce(e)
	}
}

func (m *Model) move(delta int) {
	if m.focus == feedsPanel {
		order := m.feedOrder()
		position := 0
		switch m.view {
		case allView:
			position = 1
		case feedView:
			position = 2 + slices.Index(order, m.feedIndex)
		}
		target := clamp(position+delta, 2+len(order))
		if target == position {
			return
		}
		position = target
		switch position {
		case 0:
			m.selectView(unreadView, m.feedIndex)
		case 1:
			m.selectView(allView, m.feedIndex)
		default:
			m.selectView(feedView, order[position-2])
		}
	} else if m.reading {
		m.line = clamp(m.line+delta, len(m.readerLines(m.readerWidth())))
		m.ensureLineVisible()
	} else {
		m.articleIndex = clamp(m.articleIndex+delta, len(m.articleRefs()))
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
	if f.single && len(m.feeds) == 0 {
		m.form = nil
		return
	}
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
		m.publish(event.Event{Type: event.Category, Feed: m.feeds[m.feedIndex].name, Category: category})
	case f.single:
		if err := m.store.SetInterval(m.feeds[m.feedIndex].id, interval); err != nil {
			f.err = "Kunde inte spara intervallet: " + err.Error()
			return
		}
		m.feeds[m.feedIndex].interval = interval
		m.publish(event.Event{Type: event.Interval, Feed: m.feeds[m.feedIndex].name, Interval: interval})
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
		m.selectView(feedView, len(m.feeds)-1)
		m.publish(event.Event{Type: event.FeedAdded, Feed: name, Category: category, Interval: interval})
	}
	m.form, m.focus, m.reading, m.scroll, m.visual = nil, feedsPanel, false, 0, false
}

func (m *Model) ensureLineVisible() {
	_, _, height := m.dimensions()
	m.line = clamp(m.line, len(m.readerLines(m.readerWidth())))
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
	lines := m.readerLines(m.readerWidth())
	start, end := m.selectionRange()
	start, end = clamp(start, len(lines)), clamp(end, len(lines))
	return plainLine(strings.Join(lines[start:end+1], "\n")), end - start + 1
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

func printable(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, text)
}

// setFilter keeps the cursor on the same article when it is still listed, else moves to the first.
func (m *Model) setFilter(query string) {
	var id int64
	if a := m.currentArticle(); a != nil {
		id = a.id
	}
	runes := []rune(query)
	m.filter = string(runes[:min(len(runes), 2048)])
	m.articleIndex = 0
	for i, r := range m.articleRefs() {
		if m.feeds[r.feed].articles[r.article].id == id {
			m.articleIndex = i
		}
	}
}

func (m *Model) updateFilter(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		m.setFilter("")
		m.filtering = false
	case "enter":
		m.filtering = false
	case "backspace":
		runes := []rune(m.filter)
		if len(runes) > 0 {
			m.setFilter(string(runes[:len(runes)-1]))
		}
	case "ctrl+u":
		m.setFilter("")
	default:
		m.setFilter(m.filter + printable(msg.Text))
	}
}

// openURL opens a URL in the default browser; tests replace it.
var openURL = func(url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, url).Start()
}

func (m *Model) openLink(url string) {
	if err := openURL(url); err != nil {
		m.status, m.tone = "Kunde inte öppna länken: "+err.Error(), toneErr
		return
	}
	m.status, m.tone = "Öppnar "+url, toneInfo
}

// openFromLine opens the link on the cursor line; with several it asks which one, and
// without any it opens the article itself.
func (m *Model) openFromLine() {
	links := m.linksInLine(m.readerLines(m.readerWidth()), m.line)
	switch {
	case len(links) == 1:
		m.openLink(links[0].url)
	case len(links) > 1:
		links = links[:min(len(links), 9)]
		var choices []string
		for i, l := range links {
			choices = append(choices, fmt.Sprintf("%d %s", i+1, l.text))
		}
		m.pendingLinks = links
		m.status, m.tone = strings.Join(choices, " · ")+fmt.Sprintf(" — välj 1–%d, esc avbryter", len(links)), toneWarn
	default:
		if a := m.currentArticle(); a != nil && a.link != "" {
			m.openLink(a.link)
		} else {
			m.status, m.tone = "Artikeln har ingen länk.", toneWarn
		}
	}
}
