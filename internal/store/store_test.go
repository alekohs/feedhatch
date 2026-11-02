package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal("db store should open:", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen(t *testing.T) {
	s := openTest(t)

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("user_version = %d, want 3", version)
	}
}

func TestAddFeed(t *testing.T) {
	s := openTest(t)

	id, err := s.AddFeed(Feed{URL: "https://example.com/rss", Name: "Example", Category: "News"})
	if err != nil {
		t.Fatal(err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 1 {
		t.Fatalf("got %d feeds, want 1", len(feeds))
	}
	want := Feed{ID: id, Interval: 15, URL: "https://example.com/rss", Name: "Example", Category: "News"}
	if feeds[0] != want {
		t.Fatalf("got %+v, want %+v", feeds[0], want)
	}

	if _, err := s.AddFeed(Feed{URL: "https://example.com/rss"}); err == nil {
		t.Fatal("duplicate url should fail")
	}
}

func TestSaveArticles(t *testing.T) {
	s := openTest(t)

	feedID, err := s.AddFeed(Feed{URL: "https://example.com/rss"})
	if err != nil {
		t.Fatal(err)
	}

	older := time.Unix(1_700_000_000, 0)
	newer := older.Add(time.Hour)
	articles := []Article{
		{GUID: "a", Title: "Older", PublishedAt: older},
		{GUID: "b", Title: "Newer", PublishedAt: newer},
	}
	if added, err := s.SaveArticles(feedID, articles); err != nil || added != 2 {
		t.Fatalf("added = %d, err = %v, want 2 new articles", added, err)
	}
	if added, err := s.SaveArticles(feedID, articles); err != nil || added != 0 {
		t.Fatalf("added = %d, err = %v, want saving the same articles again to be ignored", added, err)
	}

	got, err := s.Articles(feedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d articles, want 2", len(got))
	}
	if got[0].Title != "Newer" || !got[0].PublishedAt.Equal(newer) {
		t.Fatalf("first article = %+v, want Newer at %v", got[0], newer)
	}
}

func TestSetRead(t *testing.T) {
	s := openTest(t)

	feedID, err := s.AddFeed(Feed{URL: "https://example.com/rss"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArticles(feedID, []Article{{GUID: "a"}}); err != nil {
		t.Fatal(err)
	}

	articles, err := s.Articles(feedID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRead(articles[0].ID, true); err != nil {
		t.Fatal(err)
	}

	articles, err = s.Articles(feedID)
	if err != nil {
		t.Fatal(err)
	}
	if !articles[0].Read {
		t.Fatal("article should be read")
	}
}

func TestSetCategory(t *testing.T) {
	s := openTest(t)

	id, err := s.AddFeed(Feed{URL: "https://example.com/rss", Category: "News"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCategory(id, "Tech"); err != nil {
		t.Fatal(err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if feeds[0].Category != "Tech" {
		t.Fatalf("category = %q, want Tech", feeds[0].Category)
	}
}

func TestSeedMock(t *testing.T) {
	s := openTest(t)

	if err := s.SeedMock(); err != nil {
		t.Fatal(err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 4 {
		t.Fatalf("got %d feeds, want 4", len(feeds))
	}
}

func TestDeleteFeedRemovesArticles(t *testing.T) {
	s := openTest(t)

	feedID, err := s.AddFeed(Feed{URL: "https://example.com/rss"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArticles(feedID, []Article{{GUID: "a"}}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteFeed(feedID); err != nil {
		t.Fatal(err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 0 {
		t.Fatalf("got %d feeds, want 0", len(feeds))
	}
	var articles int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM articles").Scan(&articles); err != nil {
		t.Fatal(err)
	}
	if articles != 0 {
		t.Fatalf("got %d articles, want the feed's articles deleted with it", articles)
	}
}

func TestSaveArticlesFillsInMissingImages(t *testing.T) {
	s := openTest(t)
	feedID, err := s.AddFeed(Feed{URL: "https://example.com/rss"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArticles(feedID, []Article{{GUID: "a"}, {GUID: "b", Image: "https://example.com/old.png"}}); err != nil {
		t.Fatal(err)
	}
	articles, _ := s.Articles(feedID)
	for _, a := range articles {
		if err := s.SetRead(a.ID, true); err != nil {
			t.Fatal(err)
		}
	}

	added, err := s.SaveArticles(feedID, []Article{
		{GUID: "a", Image: "https://example.com/a.png"},
		{GUID: "b", Image: "https://example.com/new.png"},
	})
	if err != nil || added != 0 {
		t.Fatalf("added = %d, err = %v, want no new articles", added, err)
	}
	images := map[string]string{}
	articles, _ = s.Articles(feedID)
	for _, a := range articles {
		images[a.GUID] = a.Image
		if !a.Read {
			t.Fatalf("%s lost its read state", a.GUID)
		}
	}
	if images["a"] != "https://example.com/a.png" || images["b"] != "https://example.com/old.png" {
		t.Fatalf("images = %v, want a filled in and b kept", images)
	}
}

func TestMoveFromCacheCopiesTheOldDatabaseOnce(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	old, err := OpenPath(filepath.Join(cache, "feedhatch", "feeds.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.AddFeed(Feed{URL: "https://example.com/old"}); err != nil {
		t.Fatal(err)
	}
	old.Close()

	if err := CreateWithExamples("feedhatch"); err != nil {
		t.Fatal(err)
	}
	s, err := Open("feedhatch")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	feeds, _ := s.Feeds()
	if len(feeds) != 1 || feeds[0].URL != "https://example.com/old" {
		t.Fatalf("feeds = %v, want only the old database's feed and no examples", feeds)
	}

	if err := s.DeleteFeed(feeds[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := MoveFromCache("feedhatch"); err != nil {
		t.Fatal(err)
	}
	if feeds, _ := s.Feeds(); len(feeds) != 0 {
		t.Fatal("an existing database must not be replaced by the old one")
	}
}

func TestCreateWithExamplesOnlyForANewDatabase(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := CreateWithExamples("feedhatch"); err != nil {
		t.Fatal(err)
	}
	s, err := Open("feedhatch")
	if err != nil {
		t.Fatal(err)
	}
	feeds, _ := s.Feeds()
	if len(feeds) != len(examples) || feeds[0].Notification {
		t.Fatalf("got %d feeds, want the %d examples, with notifications off", len(feeds), len(examples))
	}
	for _, f := range feeds {
		if err := s.DeleteFeed(f.ID); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	if err := CreateWithExamples("feedhatch"); err != nil {
		t.Fatal(err)
	}
	s, _ = Open("feedhatch")
	defer s.Close()
	if feeds, _ := s.Feeds(); len(feeds) != 0 {
		t.Fatal("an existing database must not get the examples again")
	}
}

func TestNotificationIsStoredPerFeed(t *testing.T) {
	s := openTest(t)
	id, err := s.AddFeed(Feed{URL: "https://example.com/rss"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotification(id, true); err != nil {
		t.Fatal(err)
	}
	if feeds, _ := s.Feeds(); !feeds[0].Notification {
		t.Fatal("the feed's notification choice should be read back")
	}
}
