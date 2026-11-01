package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"alekohs/feedhatch/internal/event"
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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	s := startTest(t)
	if !s.Serving {
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
	if second.Serving {
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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := os.WriteFile(SocketPath("feedhatch"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	s := startTest(t)
	defer s.Close()
	if !s.Serving {
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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	real := startTest(t)
	defer real.Close()
	dev, err := Start(context.Background(), Options{Name: "feedhatch-dev", Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	if !real.Serving || !dev.Serving {
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
