package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"alekohs/feedhatch/internal/store"
	"alekohs/feedhatch/internal/worker"
)

type Server struct {
	Store *store.Store
	// Serving is false when another feedhatch already owns the socket and runs the worker.
	Serving bool

	logger      *log.Logger
	listener    net.Listener
	stop        context.CancelFunc
	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
}

func Start(ctx context.Context, mock bool, logger *log.Logger) (*Server, error) {
	logger.Println("Starting server")

	st, err := openStore(mock, logger)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	ctx, stop := context.WithCancel(ctx)
	s := &Server{Store: st, logger: logger, stop: stop, subscribers: make(map[*subscriber]struct{})}

	// Mock feeds point at real URLs; fetching them would mix live articles into the sample data.
	if mock {
		return s, nil
	}

	l, err := listen(socketPath())
	if errors.Is(err, ErrRunning) {
		logger.Println("Another feedhatch is running, only listening for its updates")
		return s, nil
	}
	if err != nil {
		s.Close()
		return nil, err
	}

	s.listener, s.Serving = l, true
	go s.serve(l)
	go worker.Run(ctx, st, s.report, s.roundDone)

	logger.Println("Server has started")
	return s, nil
}

func (s *Server) Close() {
	s.logger.Println("Closing server")
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
		s.logger.Printf("%s: %v", feed, err)
		s.broadcast(Event{Type: "error", Feed: feed, Error: err.Error()})
		return
	}
	s.logger.Printf("%s: %d new articles", feed, added)
	s.broadcast(Event{Type: "feed", Feed: feed, New: added})
}

func (s *Server) roundDone(err error) {
	e := Event{Type: "round"}
	if err != nil {
		e.Error = err.Error()
	}
	s.broadcast(e)
}

func openStore(mock bool, logger *log.Logger) (*store.Store, error) {
	if !mock {
		logger.Println("Open real db")
		return store.Open()
	}

	logger.Println("Open mocked db")
	return store.OpenMock()
}
