package server

import (
	"encoding/json"
	"time"
)

// Event is sent to socket clients as one JSON object per line.
//
// From the worker: "feed" (New articles saved), "error" (fetching Feed failed), "round" (a fetch round finished).
// From clients, after they changed the store: "read"/"unread" (ArticleID, Article), "feed_added",
// "feed_deleted", "feed_read" (all its articles marked read), "category" (Category) and
// "interval" (Interval in minutes), all naming the Feed.
type Event struct {
	Type      string    `json:"type"`
	Feed      string    `json:"feed,omitempty"`
	New       int       `json:"new,omitempty"`
	Error     string    `json:"error,omitempty"`
	ArticleID int64     `json:"article_id,omitempty"`
	Article   string    `json:"article,omitempty"`
	Category  string    `json:"category,omitempty"`
	Interval  int64     `json:"interval,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// clientEvents are the event types clients may send; anything else is ignored.
var clientEvents = map[string]bool{
	"read": true, "unread": true, "feed_added": true, "feed_deleted": true, "feed_read": true, "category": true, "interval": true,
}

func (s *Server) broadcast(e Event) {
	e.Timestamp = time.Now()
	line, _ := json.Marshal(e)
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	for sub := range s.subscribers {
		select {
		case sub.events <- line:
		default: // buffer full: slow client, drop it
			delete(s.subscribers, sub)
			close(sub.events)
		}
	}
}
