package feed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"alekohs/feedhatch/internal/store"
)

func TestFetch(t *testing.T) {
	// Servern ersätter ett riktigt RSS-flöde, så testet inte behöver internet.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Testflöde</title>
    <link>https://example.com</link>
    <description>Ett flöde för testet</description>
    <item>
      <title>Första inlägget</title>
      <link>https://example.com/posts/1</link>
      <guid>https://example.com/posts/1</guid>
    </item>
  </channel>
</rss>`)
	}))
	t.Cleanup(server.Close)

	// Hindrar den hårdkodade externa URL:en från att nå internet.
	// När Fetch använder feed.url går anropet till den lokala servern.
	client := *server.Client()
	transport := &http.Transport{Proxy: nil}
	client.Transport = testRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != server.Listener.Addr().String() {
			return nil, fmt.Errorf("oväntad URL: %s; vill ha testservern %s", r.URL, server.URL)
		}
		return transport.RoundTrip(r)
	})
	t.Cleanup(transport.CloseIdleConnections)
	previousClient := http.DefaultClient
	http.DefaultClient = &client
	t.Cleanup(func() { http.DefaultClient = previousClient })

	result, err := Fetch(context.Background(), store.Feed{URL: server.URL})
	if err != nil {
		t.Fatalf("Fetch() returnerade ett fel: %v", err)
	}
	if result == nil {
		t.Fatal("Fetch() returnerade nil utan fel")
	}
	if result.Title != "Testflöde" {
		t.Errorf("titel = %q, vill ha %q", result.Title, "Testflöde")
	}
	if len(result.Items) != 1 {
		t.Fatalf("antal inlägg = %d, vill ha 1", len(result.Items))
	}
	if result.Items[0].Title != "Första inlägget" {
		t.Errorf("inläggets titel = %q, vill ha %q", result.Items[0].Title, "Första inlägget")
	}
}

func TestFetchInvalidRSS(t *testing.T) {
	// Här isoleras parsningen från HTTP-hämtningen.
	previousClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("inte ett RSS-flöde")),
				Request:    r,
			}, nil
		}),
	}
	t.Cleanup(func() { http.DefaultClient = previousClient })

	result, err := Fetch(context.Background(), store.Feed{URL: "https://example.com/feed"})
	if err == nil {
		t.Fatal("Fetch() ska returnera ett fel för ogiltig RSS")
	}
	if result != nil {
		t.Errorf("Fetch() = %v, vill ha nil vid ogiltig RSS", result)
	}
}

// Gör det möjligt att ersätta HTTP-anrop utan externa beroenden.
type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestUpdateFeeds(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Testflöde</title>
    <item>
      <title>Första inlägget</title>
      <link>https://example.com/posts/1</link>
      <guid>https://example.com/posts/1</guid>
      <description>Innehåll</description>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
    </item>
  </channel>
</rss>`)
	}))
	t.Cleanup(server.Close)
	previousClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = previousClient })

	s, err := store.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	feedID, err := s.AddFeed(store.Feed{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}

	var reports []string
	report := func(f store.Feed, added int, err error) {
		reports = append(reports, fmt.Sprintf("%s: %d %v", f.URL, added, err))
	}
	if err := UpdateFeeds(context.Background(), s, report); err != nil {
		t.Fatalf("UpdateFeeds() returnerade ett fel: %v", err)
	}
	if len(reports) != 1 || reports[0] != server.URL+": 1 <nil>" {
		t.Errorf("rapporter = %q, vill ha en rad med antal nya artiklar", reports)
	}
	articles, err := s.Articles(feedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].Title != "Första inlägget" || articles[0].Content != "Innehåll" {
		t.Fatalf("artiklar = %+v, vill ha det hämtade inlägget", articles)
	}
	if articles[0].PublishedAt.IsZero() {
		t.Error("publiceringsdatum saknas")
	}

	if err := UpdateFeeds(context.Background(), s, report); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Errorf("antal anrop = %d, vill ha 1 eftersom intervallet inte har passerat", requests)
	}
}

func TestArticlesTakeTheImageFromTheEnclosure(t *testing.T) {
	result, err := gofeed.NewParser().ParseString(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>T</title>
<item><title>With image</title><guid>1</guid>
<enclosure url="https://x.example/audio.mp3" type="audio/mpeg" length="0"/>
<enclosure url="https://x.example/cover.png" type="image/png" length="0"/></item>
<item><title>Without</title><guid>2</guid></item>
</channel></rss>`)
	if err != nil {
		t.Fatal(err)
	}
	got := articles(result)
	if got[0].Image != "https://x.example/cover.png" || got[1].Image != "" {
		t.Fatalf("images = %q, %q", got[0].Image, got[1].Image)
	}
}

func TestUpdateFeedsStopsWithoutReportingWhenCancelled(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done() // hang until the client gives up
	}))
	t.Cleanup(server.Close)

	s, err := store.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, path := range []string{"/a", "/b"} {
		if _, err := s.AddFeed(store.Feed{URL: server.URL + path}); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	var reports []string
	err = UpdateFeeds(ctx, s, func(f store.Feed, added int, err error) { reports = append(reports, f.URL) })
	if err != nil || len(reports) != 0 {
		t.Fatalf("err = %v, reports = %v: a cancelled round should stop quietly", err, reports)
	}
}

func TestUpdateFeedFetchesOnlyThatFeedRightAway(t *testing.T) {
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		_, _ = io.WriteString(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title>
<item><title>New post</title><guid>1</guid></item></channel></rss>`)
	}))
	t.Cleanup(server.Close)

	s, err := store.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	added, err := s.AddFeed(store.Feed{URL: server.URL + "/added"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeed(store.Feed{URL: server.URL + "/other"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFetched(added, time.Now().Unix()); err != nil { // not due: must be fetched anyway
		t.Fatal(err)
	}

	var reports []string
	report := func(f store.Feed, n int, err error) { reports = append(reports, fmt.Sprint(f.URL, n, err)) }
	if err := UpdateFeed(context.Background(), s, added, report); err != nil {
		t.Fatal(err)
	}
	if requests["/added"] != 1 || requests["/other"] != 0 || len(reports) != 1 {
		t.Fatalf("requests = %v, reports = %v: only the added feed should be fetched", requests, reports)
	}
	if articles, _ := s.Articles(added); len(articles) != 1 {
		t.Fatalf("got %d articles, want the fetched one", len(articles))
	}

	if err := UpdateFeed(context.Background(), s, 999, report); err == nil {
		t.Fatal("an unknown feed id should fail")
	}
}
