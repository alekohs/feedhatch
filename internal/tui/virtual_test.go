package tui

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/store"
)

func currentTitle(m Model) string {
	if a := m.currentArticle(); a != nil {
		return a.title
	}
	return ""
}

func recordEvents(m *Model) *[]event.Event {
	events := &[]event.Event{}
	m.announce = func(e event.Event) { *events = append(*events, e) }
	return events
}

func TestStartupSelection(t *testing.T) {
	m := sizedModel(t)
	if m.view != unreadView || len(m.keep) != 5 || len(m.articleRefs()) != 5 {
		t.Fatalf("with unread articles start in Olästa: view=%d keep=%d", m.view, len(m.keep))
	}

	for _, feed := range m.feeds {
		if err := m.store.SetFeedRead(feed.id, true); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(m.store, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if m.view != feedView || m.feedIndex != m.feedOrder()[0] || len(m.keep) != 0 {
		t.Fatalf("without unread articles start on the first feed: view=%d feed=%d", m.view, m.feedIndex)
	}
}

func TestOlastaKeepsReadArticleUntilYouLeave(t *testing.T) {
	m := press(sizedModel(t), "l", "enter")
	if m.feeds[0].articles[0].unread {
		t.Fatal("opening should mark the article read")
	}
	for _, keys := range [][]string{{"h"}, {"l"}, {"1"}, {"2"}, {"tab"}, {"tab"}, {"]"}, {"["}} {
		m = press(m, keys...)
		if len(m.articleRefs()) != 5 {
			t.Fatalf("%v dropped a read article from Olästa", keys)
		}
	}

	m = press(m, "1", "j", "k")
	if len(m.articleRefs()) != m.totalUnread() || len(m.articleRefs()) >= 5 {
		t.Fatalf("selecting another sidebar entry should drop read articles, got %d", len(m.articleRefs()))
	}

	m = press(sizedModel(t), "l", "enter", "h", "r")
	if len(m.articleRefs()) != 4 || m.status != "Uppdaterad." || m.tone != toneOK || m.updated.IsZero() {
		t.Fatalf("r should refresh and drop read articles: %d refs, %q", len(m.articleRefs()), m.status)
	}
}

func TestReadEventFromVirtualListNamesRightFeed(t *testing.T) {
	m := sizedModel(t)
	events := recordEvents(&m)
	m = press(m, "l", "j", "enter")
	if currentTitle(m) != "Små verktyg som gör en sak bra" {
		t.Fatalf("unexpected article %q", currentTitle(m))
	}
	if len(*events) != 1 || (*events)[0].Type != "read" || (*events)[0].Feed != "Hacker News" {
		t.Fatalf("events = %+v, want one read for Hacker News", *events)
	}
	m = press(m, "m")
	if len(*events) != 2 || (*events)[1].Type != "unread" || (*events)[1].Feed != "Hacker News" {
		t.Fatalf("events = %+v, want unread for Hacker News", *events)
	}
}

func TestMarkAllReadFromVirtualList(t *testing.T) {
	m := sizedModel(t)
	events := recordEvents(&m)
	m = press(m, "A")
	if !m.pendingReadAll || m.tone != toneWarn || m.status != "Markera alla 5 olästa artiklar som lästa? y bekräftar, annan tangent avbryter." {
		t.Fatalf("unexpected prompt: %q", m.status)
	}
	m = press(m, "n")
	if m.pendingReadAll || m.status != "Avbröts." || m.tone != toneInfo || len(*events) != 0 || m.totalUnread() != 5 {
		t.Fatalf("any other key should cancel: %q", m.status)
	}

	m = press(m, "A", "y")
	if m.status != "Markerade 5 artiklar som lästa." || m.tone != toneOK || m.totalUnread() != 0 {
		t.Fatalf("unexpected result: %q", m.status)
	}
	if len(*events) != 4 {
		t.Fatalf("got %d events, want one feed_read per feed", len(*events))
	}
	for _, e := range *events {
		if e.Type != "feed_read" {
			t.Fatalf("event = %+v, want feed_read", e)
		}
	}
	for _, feed := range m.feeds {
		stored, err := m.store.Articles(feed.id)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range stored {
			if !a.Read {
				t.Fatalf("%q should be stored as read", a.Title)
			}
		}
	}
	if len(m.articleRefs()) != 5 {
		t.Fatal("articles marked read should stay visible in Olästa until you leave")
	}
}

func TestMarkAllReadCancelLeavesStore(t *testing.T) {
	m := press(sizedModel(t), "A", "x")
	for _, feed := range m.feeds {
		stored, err := m.store.Articles(feed.id)
		if err != nil {
			t.Fatal(err)
		}
		for i, a := range stored {
			if a.Read == feed.articles[i].unread {
				t.Fatalf("%q read state in the store changed", a.Title)
			}
		}
	}
}

func TestFeedActionsNeedARealFeed(t *testing.T) {
	for _, view := range []string{"Olästa", "Alla"} {
		m := sizedModel(t)
		if view == "Alla" {
			m = press(m, "j")
		}
		for _, key := range []string{"c", "i", "D"} {
			m = press(m, key)
			if m.form != nil || m.pendingDelete || m.status != "Välj ett riktigt flöde." || m.tone != toneWarn {
				t.Fatalf("%s in %s: form=%v status=%q", key, view, m.form, m.status)
			}
		}
	}

	m := press(sizedModel(t), "a")
	if m.form == nil || m.form.values[2] != "Övrigt" {
		t.Fatal("a in a virtual list should default the category to Övrigt")
	}
}

func TestNextUnreadAcrossFeeds(t *testing.T) {
	m := press(feedModel(t), "l", "]")
	if m.feedIndex != 0 || currentTitle(m) != "Att tänka på före nästa uppdatering" || !m.reading {
		t.Fatalf("] should open the next unread article in the feed, got %q", currentTitle(m))
	}
	m = press(m, "]")
	if m.feedIndex != 1 || currentTitle(m) != "Små verktyg som gör en sak bra" || !m.reading || m.focus != articlesPanel {
		t.Fatalf("] should continue into the next feed, got feed %d %q", m.feedIndex, currentTitle(m))
	}
	if m.status != "Nästa oläst i Hacker News." || m.tone != toneInfo {
		t.Fatalf("status = %q", m.status)
	}
	m = press(m, "]", "]")
	if m.feedIndex != 3 || currentTitle(m) != "Hierarki utan dekoration" {
		t.Fatalf("got feed %d %q", m.feedIndex, currentTitle(m))
	}
	m = press(m, "]")
	if m.status != "Inga fler olästa artiklar." || m.feedIndex != 3 || !m.reading {
		t.Fatalf("status = %q", m.status)
	}

	m = press(feedModel(t), "G", "2", "[")
	if m.feedIndex != 2 || currentTitle(m) != "Att bygga vidare med Go" || m.status != "Nästa oläst i The Go Blog." {
		t.Fatalf("[ should open the previous unread in the previous feed, got feed %d %q (%q)", m.feedIndex, currentTitle(m), m.status)
	}
	m = press(m, "[", "[", "[")
	if m.feedIndex != 0 || currentTitle(m) != "Nyheter från pakethyllan" {
		t.Fatalf("got feed %d %q", m.feedIndex, currentTitle(m))
	}
}

func TestNextUnreadSkipsKeptReadArticles(t *testing.T) {
	m := press(sizedModel(t), "l", "j", "enter", "h", "k")
	if m.articleIndex != 0 || len(m.articleRefs()) != 5 {
		t.Fatalf("setup: index %d, %d refs", m.articleIndex, len(m.articleRefs()))
	}
	m = press(m, "]")
	if m.articleIndex != 2 || currentTitle(m) != "Att bygga vidare med Go" {
		t.Fatalf("] should skip the kept read article, got %d %q", m.articleIndex, currentTitle(m))
	}
	m = press(m, "[")
	if m.articleIndex != 0 || currentTitle(m) != "Nyheter från pakethyllan" {
		t.Fatalf("[ should skip it too, got %d %q", m.articleIndex, currentTitle(m))
	}
}

func TestStepArticle(t *testing.T) {
	m := press(sizedModel(t), "l", "J")
	if m.articleIndex != 1 || m.reading {
		t.Fatal("J in the list should only move the cursor")
	}
	m = press(m, "enter", "J")
	if m.articleIndex != 2 || !m.reading || m.currentArticle().unread {
		t.Fatal("J in the reader should open the next article and mark it read")
	}
	m = press(m, "K", "K", "K")
	if m.articleIndex != 0 || !m.reading || currentTitle(m) != "Nyheter från pakethyllan" {
		t.Fatalf("K should stop at the first article, got %d", m.articleIndex)
	}
}

func TestCategoryJumpsInSidebar(t *testing.T) {
	m := press(sizedModel(t), "}")
	if m.view != feedView || m.feedIndex != 0 {
		t.Fatal("} from the virtual entries should select the first feed of the first category")
	}
	m = press(m, "}")
	if m.feedIndex != 2 {
		t.Fatalf("} should jump to Utveckling, got feed %d", m.feedIndex)
	}
	m = press(m, "}", "}")
	if m.feedIndex != 3 {
		t.Fatalf("} should stop at the last category, got feed %d", m.feedIndex)
	}
	m = press(m, "{", "{")
	if m.view != feedView || m.feedIndex != 0 {
		t.Fatalf("{ should return to Teknik, got feed %d", m.feedIndex)
	}
	m = press(m, "{")
	if m.view != unreadView {
		t.Fatal("{ from the first category should select Olästa")
	}
	m = press(m, "j", "{")
	if m.view != unreadView {
		t.Fatal("{ inside the virtual group should select its first entry")
	}

	m = press(feedModel(t), "l", "enter", "}")
	if m.feedIndex != 0 || !m.reading {
		t.Fatal("} in the reader should keep moving by paragraph")
	}
}

func TestFocusKeys(t *testing.T) {
	m := press(sizedModel(t), "2")
	if m.focus != articlesPanel {
		t.Fatal("2 should focus the article panel")
	}
	m = press(m, "enter", "1")
	if m.focus != feedsPanel || m.reading {
		t.Fatal("1 should focus the sidebar and close the reader")
	}
	m = press(m, "2")
	if m.focus != articlesPanel {
		t.Fatal("2 should focus the article panel again")
	}
}

func TestFetchedMsgKeepsCursorInVirtualList(t *testing.T) {
	m := press(sizedModel(t), "l", "j", "j")
	articleID := m.currentArticle().id

	_, err := m.store.SaveArticles(m.feeds[0].id, []store.Article{{GUID: "new", Title: "Ny artikel", PublishedAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(FetchedMsg{})
	m = updated.(Model)
	if m.articleIndex != 3 || m.currentArticle().id != articleID {
		t.Fatalf("cursor should follow its article, index %d", m.articleIndex)
	}
	if m.updated.IsZero() || m.fetchFailed {
		t.Fatal("a fetch round should record when it finished")
	}

	updated, _ = m.Update(FetchedMsg{Err: errors.New("nere")})
	if m = updated.(Model); !m.fetchFailed || m.tone != toneErr {
		t.Fatal("a failed round should be flagged")
	}
}

func fetched(m Model, err error) Model {
	updated, _ := m.Update(FetchedMsg{Err: err})
	return updated.(Model)
}

func TestNewArticleFetchedInOlastaIsMarkedReadWhenOpened(t *testing.T) {
	m := sizedModel(t)
	events := recordEvents(&m)
	feed := m.feeds[1]
	if _, err := m.store.SaveArticles(feed.id, []store.Article{{GUID: "new", Title: "Ny artikel", PublishedAt: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	m = press(fetched(m, nil), "l", "g", "enter")
	if !m.reading || currentTitle(m) != "Ny artikel" {
		t.Fatalf("reader shows %q", currentTitle(m))
	}
	stored, err := m.store.Articles(feed.id)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range stored {
		if a.Title == "Ny artikel" && !a.Read {
			t.Fatal("the opened article should be stored as read")
		}
	}
	if len(*events) != 1 || (*events)[0].Type != "read" || (*events)[0].Feed != feed.name {
		t.Fatalf("events = %+v, want one read for %s", *events, feed.name)
	}

	if _, err := m.store.SaveArticles(feed.id, []store.Article{{GUID: "newer", Title: "Nyare", PublishedAt: time.Now().Add(2 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	m = press(fetched(m, nil), "h", "k", "m")
	if currentTitle(m) != "Nyare" || m.currentArticle().unread {
		t.Fatalf("m should mark %q read and keep it listed", currentTitle(m))
	}
}

func TestFeedDeletedElsewhereCancelsAction(t *testing.T) {
	for _, keys := range [][]string{{"D"}, {"c"}, {"i"}} {
		m := feedModel(t)
		target := m.feeds[m.feedIndex]
		m = press(m, keys...)
		if err := m.store.DeleteFeed(target.id); err != nil {
			t.Fatal(err)
		}
		m = fetched(m, nil)
		if m.pendingDelete || m.form != nil || m.status != "Flödet togs bort av en annan klient." || m.tone != toneWarn {
			t.Fatalf("%v should be cancelled: %q", keys, m.status)
		}
		count := len(m.feeds)
		m = press(m, "y")
		if len(m.feeds) != count {
			t.Fatalf("%v retargeted to a neighbouring feed", keys)
		}
	}

	m := press(feedModel(t), "a")
	target := m.feeds[m.feedIndex]
	if err := m.store.DeleteFeed(target.id); err != nil {
		t.Fatal(err)
	}
	if m = fetched(m, nil); m.form == nil {
		t.Fatal("the add feed form should survive")
	}
}

func TestReaderClosesWhenFeedDeletedElsewhere(t *testing.T) {
	m := press(feedModel(t), "l", "enter")
	if err := m.store.DeleteFeed(m.feeds[m.feedIndex].id); err != nil {
		t.Fatal(err)
	}
	if m = fetched(m, nil); m.reading {
		t.Fatal("the reader should close when its feed is gone")
	}
	m.View()
}

func TestAllFeedsDeletedElsewhere(t *testing.T) {
	for _, keys := range [][]string{{"l", "enter"}, {"c"}, {"i"}, {"D"}, {"a"}} {
		m := press(feedModel(t), keys...)
		for _, feed := range m.feeds {
			if err := m.store.DeleteFeed(feed.id); err != nil {
				t.Fatal(err)
			}
		}
		m = fetched(m, nil)
		m.View()
		m = press(m, "y", "c", "i", "D", "A", "m", "j", "k", "enter", "esc")
		m.View()
	}

	m := press(sizedModel(t), "1")
	for _, feed := range m.feeds {
		if err := m.store.DeleteFeed(feed.id); err != nil {
			t.Fatal(err)
		}
	}
	m = press(fetched(m, nil), "A", "y", "j", "J", "]")
	m.View()
}

func TestRefreshInOlastaKeepsCursorOnArticle(t *testing.T) {
	m := press(sizedModel(t), "l", "j", "j")
	id := m.currentArticle().id
	for _, r := range m.articleRefs()[:2] {
		if err := m.store.SetRead(m.feeds[r.feed].articles[r.article].id, true); err != nil {
			t.Fatal(err)
		}
	}
	m = press(m, "r")
	if m.currentArticle() == nil || m.currentArticle().id != id {
		t.Fatal("r should keep the cursor on the same article")
	}
	if len(m.articleRefs()) != 3 {
		t.Fatalf("r should drop read articles from Olästa, got %d", len(m.articleRefs()))
	}
}

func TestFetchErrorKeepsPendingPrompt(t *testing.T) {
	for _, key := range []string{"D", "A"} {
		m := feedModel(t)
		if key == "A" {
			m = sizedModel(t)
		}
		m = press(m, key)
		prompt := m.status
		m = fetched(m, errors.New("nere"))
		if m.status != prompt || m.tone != toneWarn || !m.fetchFailed {
			t.Fatalf("%s prompt was replaced by %q", key, m.status)
		}
	}
}

func TestSidebarEdgeKeepsArticleCursor(t *testing.T) {
	m := press(sizedModel(t), "l", "j", "h", "k")
	if m.articleIndex != 1 {
		t.Fatalf("k on the top entry reset the cursor to %d", m.articleIndex)
	}
	last := m.feeds[m.feedOrder()[len(m.feedOrder())-1]]
	extra := []store.Article{{GUID: "a", Title: "A", PublishedAt: time.Now()}, {GUID: "b", Title: "B", PublishedAt: time.Now()}}
	if _, err := m.store.SaveArticles(last.id, extra); err != nil {
		t.Fatal(err)
	}
	m = press(fetched(m, nil), "G", "l", "j", "h", "j")
	if m.articleIndex != 1 {
		t.Fatalf("j on the last entry reset the cursor to %d", m.articleIndex)
	}
}

func TestDeleteSelectsNextFeedInSidebarOrder(t *testing.T) {
	m := feedModel(t)
	m.feeds[0].category = "Design"
	m.selectView(feedView, 0)
	m = press(m, "D", "y")
	if m.view != feedView || m.feeds[m.feedIndex].name != "Fältanteckningar" {
		t.Fatalf("selected %q, want the next feed in sidebar order", m.feeds[m.feedIndex].name)
	}

	m = feedModel(t)
	last := m.feedOrder()[len(m.feedOrder())-1]
	m.selectView(feedView, last)
	m = press(m, "D", "y")
	order := m.feedOrder()
	if m.feedIndex != order[len(order)-1] {
		t.Fatal("deleting the last feed should select the previous one")
	}
}

func TestMarkAllReadReportsActualCount(t *testing.T) {
	m := press(sizedModel(t), "A")
	readElsewhere := unreadCount(m.feeds[0])
	if readElsewhere == 0 {
		t.Fatal("test needs unread articles in the first feed")
	}
	if err := m.store.SetFeedRead(m.feeds[0].id, true); err != nil {
		t.Fatal(err)
	}
	m = press(fetched(m, nil), "y")
	if want := fmt.Sprintf("Markerade %d artiklar som lästa.", 5-readElsewhere); m.status != want {
		t.Fatalf("status = %q, want %q", m.status, want)
	}
}

func TestMarkAllReadWithNothingLeft(t *testing.T) {
	m := press(sizedModel(t), "A")
	for _, feed := range m.feeds {
		if err := m.store.SetFeedRead(feed.id, true); err != nil {
			t.Fatal(err)
		}
	}
	m = press(fetched(m, nil), "y")
	if m.status != "Inga olästa artiklar." {
		t.Fatalf("status = %q", m.status)
	}
}

func TestSpaceAtEndOfArticleOpensNextUnread(t *testing.T) {
	m := press(sizedModel(t), "l", "enter")
	m = press(m, "G")
	if m.articleIndex != 0 {
		t.Fatal("setup: expected to be reading the first article")
	}
	m = press(m, "space")
	if m.articleIndex != 1 || !m.reading || m.line != 0 || currentTitle(m) != "Små verktyg som gör en sak bra" {
		t.Fatalf("space at the end should open the next unread, got %d %q", m.articleIndex, currentTitle(m))
	}
}
