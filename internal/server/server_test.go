package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/settings"
	"alekohs/feedhatch/internal/store"
)

func startTest(t *testing.T) *Server {
	t.Helper()
	s, err := Start(context.Background(), Options{Name: "feedhatch", Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStartServesAndSubscribeReceivesEvents(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	s := startTest(t)
	if !s.Serving() {
		t.Fatal("the first server should own the socket")
	}

	events := make(chan event.Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	Connect(ctx, "feedhatch", func(e event.Event) { events <- e })

	if e := waitForEvent(t, events); e.Type != event.Connected {
		t.Fatalf("first event = %q, want connected", e.Type)
	}
	// The worker's first round may finish before the subscriber connects, so trigger one.
	time.Sleep(50 * time.Millisecond)
	s.roundDone(nil)
	if e := waitForEvent(t, events); e.Type != event.Round {
		t.Fatalf("event = %q, want round", e.Type)
	}

	second := startTest(t)
	if second.Serving() {
		t.Fatal("a second server should see that the socket is taken")
	}
	second.Close()

	s.Close()
	if _, err := os.Stat(SocketPath("feedhatch")); !os.IsNotExist(err) {
		t.Fatalf("socket should be removed on Close, stat err = %v", err)
	}
}

func TestStartReplacesStaleSocket(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := os.WriteFile(SocketPath("feedhatch"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	s := startTest(t)
	defer s.Close()
	if !s.Serving() {
		t.Fatal("a stale socket file should be replaced")
	}
}

func waitForEvent(t *testing.T, events <-chan event.Event) event.Event {
	t.Helper()
	select {
	case e := <-events:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
		return event.Event{}
	}
}

func TestClientEventsReachOtherClients(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	s := startTest(t)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sender := Connect(ctx, "feedhatch", func(event.Event) {})
	events := make(chan event.Event, 16)
	Connect(ctx, "feedhatch", func(e event.Event) {
		if e.Type != event.Connected && e.Type != event.Round {
			events <- e
		}
	})

	// Give both clients time to connect and be registered by the server.
	time.Sleep(100 * time.Millisecond)
	sender.Send(event.Event{Type: "bogus"}) // not a client event: must not arrive before the read below
	sender.Send(event.Event{Type: event.Read, Feed: "Arch Linux", ArticleID: 7, Article: "News"})

	e := waitForEvent(t, events)
	if e.Type != event.Read || e.Feed != "Arch Linux" || e.ArticleID != 7 || e.Article != "News" {
		t.Fatalf("event = %+v, want the read event from the other client", e)
	}
}

func TestDevNameKeepsItsOwnSocketAndDatabase(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	real := startTest(t)
	defer real.Close()
	dev, err := Start(context.Background(), Options{Name: "feedhatch-dev", Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	if !real.Serving() || !dev.Serving() {
		t.Fatal("feedhatch and feedhatch-dev should each own their own socket")
	}
	if _, err := dev.Store.AddFeed(store.Feed{URL: "https://example.com/dev.xml"}); err != nil {
		t.Fatal(err)
	}
	feeds, err := real.Store.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 0 {
		t.Fatalf("the real database has %d feeds, want the dev feed kept out of it", len(feeds))
	}
}

func TestRunningSeesTheSocketOwner(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if Running("feedhatch") {
		t.Fatal("nothing should be running yet")
	}
	s := startTest(t)
	if !Running("feedhatch") || Running("feedhatch-dev") {
		t.Fatal("Running should see only the feedhatch that owns its socket")
	}
	s.Close()
	if Running("feedhatch") {
		t.Fatal("Running should be false after Close")
	}
}

func TestCloseWaitsForAFetchInProgress(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	started := make(chan struct{})
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(feedServer.Close)

	st, err := store.Open("feedhatch")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFeed(store.Feed{URL: feedServer.URL}); err != nil {
		t.Fatal(err)
	}
	st.Close()

	s := startTest(t)
	<-started // the worker's first round is now waiting on the feed

	done := make(chan struct{})
	go func() {
		s.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close should cancel the fetch and return")
	}
}

func TestListenerTakesOverWhenTheOwnerStops(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	previous := takeOverInterval
	takeOverInterval = 20 * time.Millisecond
	t.Cleanup(func() { takeOverInterval = previous })

	owner := startTest(t)
	listener := startTest(t)
	defer listener.Close()
	if !owner.Serving() || listener.Serving() {
		t.Fatal("the first process should own the socket, the second only listen")
	}

	events := make(chan event.Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	Connect(ctx, "feedhatch", func(e event.Event) { events <- e })
	waitForEvent(t, events) // connected to the owner

	owner.Close()
	deadline := time.After(5 * time.Second)
	for !listener.Serving() {
		select {
		case <-deadline:
			t.Fatal("the listener should take over the socket")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !Running("feedhatch") {
		t.Fatal("the socket should be served again")
	}

	// The client reconnects to the new owner and hears its events.
	for e := waitForEvent(t, events); e.Type != event.Connected; e = waitForEvent(t, events) {
	}
	time.Sleep(50 * time.Millisecond)
	listener.roundDone(nil)
	for e := waitForEvent(t, events); e.Type != event.Round; e = waitForEvent(t, events) {
	}
}

func TestAddedFeedIsFetchedRightAway(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>T</title>
<item><title>First post</title><guid>1</guid></item></channel></rss>`))
	}))
	t.Cleanup(feedServer.Close)

	s := startTest(t)
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	events := make(chan event.Event, 16)
	client := Connect(ctx, "feedhatch", func(e event.Event) { events <- e })
	waitForEvent(t, events) // connected
	time.Sleep(50 * time.Millisecond)

	id, err := s.Store.AddFeed(store.Feed{URL: feedServer.URL, Name: "New feed"})
	if err != nil {
		t.Fatal(err)
	}
	client.Send(event.Event{Type: event.FeedAdded, Feed: "New feed", FeedID: id})

	deadline := time.After(5 * time.Second)
	for got := map[string]bool{}; !got[event.Fetched] || !got[event.Round]; {
		select {
		case e := <-events:
			if e.Type == event.Fetched && e.Feed == "New feed" && e.New == 1 || e.Type == event.Round && got[event.Fetched] {
				got[e.Type] = true
			}
		case <-deadline:
			t.Fatal("the added feed should be fetched right away, followed by a round event")
		}
	}
	if articles, _ := s.Store.Articles(id); len(articles) != 1 {
		t.Fatalf("got %d articles, want the new feed's article saved", len(articles))
	}
}

func TestNewArticlesNotifyWhenTheFeedAndTheSwitchAllowIt(t *testing.T) {
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>T</title>
<item><title>One</title><guid>1</guid></item><item><title>Two</title><guid>2</guid></item></channel></rss>`))
	}))
	t.Cleanup(feedServer.Close)

	for _, c := range []struct {
		name         string
		feedNotifies bool
		switchOn     bool
		wantNotified bool
	}{
		{"feed and switch on", true, true, true},
		{"feed off", false, true, false},
		{"switch off", true, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
			notified := make(chan string, 4)
			previous := sendNotification
			sendNotification = func(title, body string) error {
				notified <- title + ": " + body
				return nil
			}
			t.Cleanup(func() { sendNotification = previous })

			st, err := store.Open("feedhatch")
			if err != nil {
				t.Fatal(err)
			}
			id, err := st.AddFeed(store.Feed{URL: feedServer.URL, Name: "Blog"})
			if err != nil {
				t.Fatal(err)
			}
			if err := st.SetNotification(id, c.feedNotifies); err != nil {
				t.Fatal(err)
			}
			st.Close()
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			if err := (settings.Config{Notifications: c.switchOn}).Save(settingsPath); err != nil {
				t.Fatal(err)
			}

			s, err := Start(context.Background(), Options{Name: "feedhatch", Logger: slog.New(slog.DiscardHandler), SettingsPath: settingsPath})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			deadline := time.After(3 * time.Second)
			for {
				if articles, _ := s.Store.Articles(id); len(articles) == 2 {
					break
				}
				select {
				case <-deadline:
					t.Fatal("the feed should be fetched")
				case <-time.After(10 * time.Millisecond):
				}
			}
			select {
			case got := <-notified:
				if !c.wantNotified || got != "Blog: 2 nya artiklar" {
					t.Fatalf("notified %q, want notified = %v", got, c.wantNotified)
				}
			case <-time.After(200 * time.Millisecond):
				if c.wantNotified {
					t.Fatal("new articles should notify")
				}
			}
		})
	}
}
