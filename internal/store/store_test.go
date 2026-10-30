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
	if version != 1 {
		t.Fatalf("user_version = %d, want 1", version)
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
