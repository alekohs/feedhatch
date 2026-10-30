package feed

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

	result, err := Fetch(store.Feed{URL: server.URL})
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

	result, err := Fetch(store.Feed{URL: "https://example.com/feed"})
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

	var logs strings.Builder
	logger := log.New(&logs, "", 0)
	if err := UpdateFeeds(s, logger); err != nil {
		t.Fatalf("UpdateFeeds() returnerade ett fel: %v", err)
	}
	if logs.String() != server.URL+": 1 new articles\n" {
		t.Errorf("logg = %q, vill ha en rad med antal nya artiklar", logs.String())
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

	if err := UpdateFeeds(s, logger); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Errorf("antal anrop = %d, vill ha 1 eftersom intervallet inte har passerat", requests)
	}
}
