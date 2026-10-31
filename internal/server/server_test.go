package server

import (
	"context"
	"io"
	"log"
	"os"
	"testing"
	"time"
)

func startTest(t *testing.T) *Server {
	t.Helper()
	s, err := Start(context.Background(), false, log.New(io.Discard, "", 0))
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

	events := make(chan Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	Connect(ctx, func(e Event) { events <- e })

	if e := waitForEvent(t, events); e.Type != "connected" {
		t.Fatalf("first event = %q, want connected", e.Type)
	}
	// The worker's first round may finish before the subscriber connects, so trigger one.
	time.Sleep(50 * time.Millisecond)
	s.roundDone(nil)
	if e := waitForEvent(t, events); e.Type != "round" {
		t.Fatalf("event = %q, want round", e.Type)
	}

	second := startTest(t)
	if second.Serving {
		t.Fatal("a second server should see that the socket is taken")
	}
	second.Close()

	s.Close()
	if _, err := os.Stat(socketPath()); !os.IsNotExist(err) {
		t.Fatalf("socket should be removed on Close, stat err = %v", err)
	}
}

func TestStartReplacesStaleSocket(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := os.WriteFile(socketPath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	s := startTest(t)
	defer s.Close()
	if !s.Serving {
		t.Fatal("a stale socket file should be replaced")
	}
}

func waitForEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case e := <-events:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
		return Event{}
	}
}

func TestClientEventsReachOtherClients(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	s := startTest(t)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sender := Connect(ctx, func(Event) {})
	events := make(chan Event, 16)
	Connect(ctx, func(e Event) {
		if e.Type != "connected" && e.Type != "round" {
			events <- e
		}
	})

	// Give both clients time to connect and be registered by the server.
	time.Sleep(100 * time.Millisecond)
	sender.Send(Event{Type: "bogus"}) // not a client event: must not arrive before the read below
	sender.Send(Event{Type: "read", Feed: "Arch Linux", ArticleID: 7, Article: "News"})

	e := waitForEvent(t, events)
	if e.Type != "read" || e.Feed != "Arch Linux" || e.ArticleID != 7 || e.Article != "News" {
		t.Fatalf("event = %+v, want the read event from the other client", e)
	}
}
