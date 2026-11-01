package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"alekohs/feedhatch/internal/event"
)

var ErrRunning = errors.New("feedhatch is already running")

type subscriber struct {
	events chan []byte
}

func listen(path string) (net.Listener, error) {
	if conn, err := net.Dial("unix", path); err == nil {
		conn.Close()
		return nil, ErrRunning // someone is listening: be a client instead
	}

	os.Remove(path)

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	return l, os.Chmod(path, 0o600)
}

// SocketPath is where the feedhatch called name listens, e.g. feedhatch.sock or feedhatch-dev.sock.
func SocketPath(name string) string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}

	return filepath.Join(dir, name+".sock")
}

func (s *Server) serve(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return // listener closed in Close
		}
		s.add(conn)
	}
}

func (s *Server) add(conn net.Conn) {
	sub := &subscriber{events: make(chan []byte, 16)}
	s.mu.Lock()
	s.subscribers[sub] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer conn.Close()
		for line := range sub.events {
			if _, err := conn.Write(line); err != nil {
				s.remove(sub)
				return
			}
		}
	}()

	// Rebroadcast what clients changed, so every listener hears about it.
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			var e event.Event
			if json.Unmarshal(scanner.Bytes(), &e) == nil && event.FromClient(e.Type) {
				s.logger.Info("client event", "type", e.Type, "feed", e.Feed)
				s.broadcast(e)
			}
		}
		s.remove(sub)
	}()
}

func (s *Server) remove(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[sub]; ok {
		delete(s.subscribers, sub)
		close(sub.events)
	}
}

type Client struct {
	mu   sync.Mutex
	conn net.Conn
}

// Connect passes every event from the socket of the feedhatch called name to handle
// until ctx is cancelled, reconnecting when the server goes away. Each (re)connect is
// reported as an event.Connected, since events sent while disconnected are lost.
func Connect(ctx context.Context, name string, handle func(event.Event)) *Client {
	c := &Client{}
	go c.run(ctx, SocketPath(name), handle)
	return c
}

func (c *Client) run(ctx context.Context, path string, handle func(event.Event)) {
	for {
		if conn, err := net.Dial("unix", path); err == nil {
			c.setConn(conn)
			stopClose := context.AfterFunc(ctx, func() { conn.Close() })
			handle(event.Event{Type: event.Connected, Timestamp: time.Now()})

			scanner := bufio.NewScanner(conn)
			for scanner.Scan() {
				var e event.Event
				if json.Unmarshal(scanner.Bytes(), &e) == nil {
					handle(e)
				}
			}

			stopClose()
			c.setConn(nil)
			conn.Close()
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Client) setConn(conn net.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

// Send publishes e to every listener through the server. It is dropped while disconnected.
func (c *Client) Send(e event.Event) {
	line, _ := json.Marshal(e)
	line = append(line, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_, _ = c.conn.Write(line) // a failed write means the server is gone; run reconnects
	}
}
