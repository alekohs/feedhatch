package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/feed"
	"alekohs/feedhatch/internal/store"
	"alekohs/feedhatch/internal/worker"
)

// takeOverInterval is how often a process that only listens checks whether the
// socket's owner has stopped.
var takeOverInterval = 2 * time.Second

type Options struct {
	// Name picks the data directory and socket: "feedhatch", or "feedhatch-dev" to keep
	// development away from the real data.
	Name string
	// Mock uses sample data in a temporary database and starts neither socket nor worker.
	Mock   bool
	Logger *slog.Logger
}

type Server struct {
	Store *store.Store

	logger      *slog.Logger
	serving     atomic.Bool
	listener    net.Listener    // guarded by mu
	closed      bool            // guarded by mu; set by Close so late connections are refused
	ctx         context.Context // the server's lifetime, for work started later
	stop        context.CancelFunc
	worker      sync.WaitGroup
	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
}

func Start(ctx context.Context, opts Options) (*Server, error) {
	logger := opts.Logger.With("component", "server")
	logger.Info("starting", "name", opts.Name, "mock", opts.Mock)

	st, err := openStore(opts)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	ctx, stop := context.WithCancel(ctx)
	s := &Server{Store: st, logger: logger, ctx: ctx, stop: stop, subscribers: make(map[*subscriber]struct{})}

	// Mock feeds point at real URLs; fetching them would mix live articles into the sample data.
	if opts.Mock {
		return s, nil
	}

	path := SocketPath(opts.Name)
	l, err := listen(path)
	if errors.Is(err, ErrRunning) {
		logger.Info("another feedhatch owns the socket, only listening for its updates", "socket", path)
		s.worker.Go(func() { s.takeOver(ctx, path) })
		return s, nil
	}
	if err != nil {
		s.Close()
		return nil, err
	}

	s.claim(ctx, l)
	logger.Info("started", "socket", path)
	return s, nil
}

// Serving reports whether this process owns the socket and runs the worker; it turns
// true when it takes over from an owner that stopped.
func (s *Server) Serving() bool { return s.serving.Load() }

// takeOver waits for the socket's owner to stop, then claims the socket so fetching
// goes on, e.g. a TUI started after the daemon keeps fetching when the daemon stops.
func (s *Server) takeOver(ctx context.Context, path string) {
	ticker := time.NewTicker(takeOverInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if l, err := listen(path); err == nil {
			s.logger.Info("took over from the stopped owner", "socket", path)
			s.claim(ctx, l)
			return
		}
	}
}

func (s *Server) claim(ctx context.Context, l net.Listener) {
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	s.serving.Store(true)
	go s.serve(l)
	s.worker.Go(func() { worker.Run(ctx, s.Store, s.report, s.roundDone) })
}

// Close stops the worker, waiting for a fetch in progress so it doesn't write to a
// closed database, then closes the socket, its client connections and the store.
func (s *Server) Close() {
	s.logger.Info("closing")
	s.mu.Lock()
	s.closed = true // no new work after this, so the wait below is final
	s.mu.Unlock()
	s.stop()
	s.worker.Wait()
	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
	}
	// Closing a subscriber's events ends its writer, which closes the connection, so
	// clients notice and reconnect to whoever takes over.
	for sub := range s.subscribers {
		delete(s.subscribers, sub)
		close(sub.events)
	}
	s.mu.Unlock()
	s.Store.Close()
}

func (s *Server) GetStatus() string {
	return "OK"
}

func (s *Server) report(feed string, added int, err error) {
	if err != nil {
		s.logger.Warn("fetch failed", "feed", feed, "err", err)
		s.broadcast(event.Event{Type: event.Failed, Feed: feed, Error: err.Error()})
		return
	}
	s.logger.Info("fetched feed", "feed", feed, "new", added)
	s.broadcast(event.Event{Type: event.Fetched, Feed: feed, New: added})
}

func (s *Server) roundDone(err error) {
	e := event.Event{Type: event.Round}
	if err != nil {
		e.Error = err.Error()
	}
	s.broadcast(e)
}

func openStore(opts Options) (*store.Store, error) {
	if opts.Mock {
		return store.OpenMock()
	}
	return store.Open(opts.Name)
}

// fetchNow fetches one feed right away, e.g. one a client just added, then tells
// clients that a round finished so they show its articles.
func (s *Server) fetchNow(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.Serving() {
		return
	}
	s.worker.Go(func() {
		err := feed.UpdateFeed(s.ctx, s.Store, id, s.report)
		if s.ctx.Err() == nil {
			s.roundDone(err)
		}
	})
}
