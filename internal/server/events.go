package server

import (
	"encoding/json"
	"time"

	"alekohs/feedhatch/internal/event"
)

func (s *Server) broadcast(e event.Event) {
	e.Timestamp = time.Now()
	line, _ := json.Marshal(e)
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	for sub := range s.subscribers {
		select {
		case sub.events <- line:
		default: // buffer full: slow client, drop it
			s.logger.Warn("dropping slow client")
			delete(s.subscribers, sub)
			close(sub.events)
		}
	}
}
