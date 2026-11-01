package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/store"
	"alekohs/feedhatch/internal/worker"
)

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
	// Serving is false when another feedhatch already owns the socket and runs the worker.
	Serving bool

	logger      *slog.Logger
	listener    net.Listener
	stop        context.CancelFunc
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
	s := &Server{Store: st, logger: logger, stop: stop, subscribers: make(map[*subscriber]struct{})}

	// Mock feeds point at real URLs; fetching them would mix live articles into the sample data.
	if opts.Mock {
		return s, nil
	}

	path := SocketPath(opts.Name)
	l, err := listen(path)
	if errors.Is(err, ErrRunning) {
		logger.Info("another feedhatch owns the socket, only listening for its updates", "socket", path)
		return s, nil
	}
	if err != nil {
		s.Close()
		return nil, err
	}

	s.listener, s.Serving = l, true
	go s.serve(l)
	go worker.Run(ctx, st, s.report, s.roundDone)

	logger.Info("started", "socket", path)
	return s, nil
}

func (s *Server) Close() {
	s.logger.Info("closing")
	s.stop()
	if s.listener != nil {
		s.listener.Close()
	}
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
