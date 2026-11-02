package opml

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"alekohs/feedhatch/internal/store"
)

func openTest(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenPath(filepath.Join(t.TempDir(), "feeds.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func feedsOf(t *testing.T, s *store.Store) []store.Feed {
	t.Helper()
	feeds, err := s.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	return feeds
}

func importString(t *testing.T, s *store.Store, doc string) (added, skipped int) {
	t.Helper()
	parsed, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	added, skipped, err = Import(parsed, s)
	if err != nil {
		t.Fatal(err)
	}
	return added, skipped
}

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Subscriptions</title></head>
  <body>
    <outline text="Loose" xmlUrl="https://example.com/loose.xml"/>
    <outline text="Tech">
      <outline type="rss" text="Go" title="Ignored" xmlUrl=" https://go.dev/blog/feed.atom "/>
      <outline title="Only title" xmlUrl="https://example.com/title.xml"/>
      <outline text="Deep">
        <outline text="Nested" xmlUrl="https://example.com/nested.xml"/>
      </outline>
    </outline>
    <outline text="Bad scheme" xmlUrl="file:///etc/passwd"/>
    <outline text="Twice" xmlUrl="https://example.com/loose.xml"/>
  </body>
</opml>`

func TestImportAddsFeedsWithCategories(t *testing.T) {
	s := openTest(t)
	if added, skipped := importString(t, s, sample); added != 4 || skipped != 2 {
		t.Fatalf("added %d, skipped %d; want 4 and 2", added, skipped)
	}

	want := []store.Feed{
		{URL: "https://example.com/loose.xml", Name: "Loose"},
		{URL: "https://go.dev/blog/feed.atom", Name: "Go", Category: "Tech"},
		{URL: "https://example.com/title.xml", Name: "Only title", Category: "Tech"},
		{URL: "https://example.com/nested.xml", Name: "Nested", Category: "Deep"},
	}
	feeds := feedsOf(t, s)
	if len(feeds) != len(want) {
		t.Fatalf("got %d feeds, want %d: %+v", len(feeds), len(want), feeds)
	}
	for i, w := range want {
		f := feeds[i]
		if f.URL != w.URL || f.Name != w.Name || f.Category != w.Category {
			t.Errorf("feed %d = %+v, want %+v", i, f, w)
		}
		if f.Interval != 15 {
			t.Errorf("feed %d interval = %d, want the default 15", i, f.Interval)
		}
	}
}

func TestImportTwiceAddsNothingAndKeepsExistingFeeds(t *testing.T) {
	s := openTest(t)
	if _, err := s.AddFeed(store.Feed{URL: "https://example.com/loose.xml", Name: "Mine", Category: "Own"}); err != nil {
		t.Fatal(err)
	}
	importString(t, s, sample)
	if added, skipped := importString(t, s, sample); added != 0 || skipped != 6 {
		t.Fatalf("added %d, skipped %d; want 0 and 6", added, skipped)
	}

	feeds := feedsOf(t, s)
	if len(feeds) != 4 {
		t.Fatalf("got %d feeds, want 4: %+v", len(feeds), feeds)
	}
	if feeds[0].Name != "Mine" || feeds[0].Category != "Own" {
		t.Fatalf("existing feed changed: %+v", feeds[0])
	}
}

func TestParseReadsOtherEncodings(t *testing.T) {
	doc := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><opml><body>" +
		"<outline text=\"Nyheter fr\xe5n\" xmlUrl=\"https://example.com/a.xml\"/></body></opml>"

	parsed, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Body.Outlines[0].Text; got != "Nyheter från" {
		t.Fatalf("text = %q", got)
	}
}

func TestParseRejectsOtherDocuments(t *testing.T) {
	for _, doc := range []string{"<html><body></body></html>", "not xml", ""} {
		if _, err := Parse(strings.NewReader(doc)); err == nil {
			t.Errorf("Parse(%q) should fail", doc)
		}
	}
}

func TestExportWritesValidOPML(t *testing.T) {
	s := openTest(t)
	for _, f := range []store.Feed{
		{URL: "https://example.com/a.xml", Name: "A & B", Category: "Tech"},
		{URL: "https://example.com/b.xml"},
		{URL: "https://example.com/c.xml", Name: "C", Category: "Tech"},
	} {
		if _, err := s.AddFeed(f); err != nil {
			t.Fatal(err)
		}
	}

	exported, err := Export(s)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Write(&out, exported); err != nil {
		t.Fatal(err)
	}
	doc := out.String()

	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<opml version="2.0">`,
		`<title>feedhatch</title>`,
		`<outline text="Tech">`,
		`type="rss" text="A &amp; B" title="A &amp; B" xmlUrl="https://example.com/a.xml"`,
		`text="https://example.com/b.xml"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in:\n%s", want, doc)
		}
	}
	if strings.Count(doc, `<outline text="Tech">`) != 1 {
		t.Errorf("feeds in one category should share a folder:\n%s", doc)
	}
}

func TestExportThenImportKeepsFeeds(t *testing.T) {
	from := openTest(t)
	importString(t, from, sample)

	exported, err := Export(from)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Write(&out, exported); err != nil {
		t.Fatal(err)
	}
	to := openTest(t)
	importString(t, to, out.String())

	type key struct{ url, name, category string }
	got := map[key]bool{}
	for _, f := range feedsOf(t, to) {
		got[key{f.URL, f.Name, f.Category}] = true
	}
	want := feedsOf(t, from)
	if len(got) != len(want) {
		t.Fatalf("got %d feeds, want %d", len(got), len(want))
	}
	for _, f := range want {
		if !got[key{f.URL, f.Name, f.Category}] {
			t.Errorf("lost %+v", f)
		}
	}
}
