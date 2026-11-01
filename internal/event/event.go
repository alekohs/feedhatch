package event

import "time"

// Event is sent to socket clients as one JSON object per line.
type Event struct {
	Type      string    `json:"type"`
	Feed      string    `json:"feed,omitempty"`
	FeedID    int64     `json:"feed_id,omitempty"`
	New       int       `json:"new,omitempty"`
	Error     string    `json:"error,omitempty"`
	ArticleID int64     `json:"article_id,omitempty"`
	Article   string    `json:"article,omitempty"`
	Category  string    `json:"category,omitempty"`
	Interval  int64     `json:"interval,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Sent by the worker.
const (
	Fetched = "feed"  // New articles were saved for Feed
	Failed  = "error" // fetching Feed failed with Error
	Round   = "round" // a fetch round finished; Error joins its failures
)

// Connected is reported locally by a client each time it (re)connects to the socket.
const Connected = "connected"

// Sent by clients after they changed the store, all naming the Feed.
const (
	Read        = "read"       // ArticleID, Article
	Unread      = "unread"     // ArticleID, Article
	FeedAdded   = "feed_added" // FeedID: the fetching process fetches it right away
	FeedDeleted = "feed_deleted"
	FeedRead    = "feed_read" // all its articles were marked read
	Category    = "category"  // Category
	Interval    = "interval"  // Interval in minutes
)

// FromClient reports whether clients may send events of type t; the server ignores anything else.
func FromClient(t string) bool {
	switch t {
	case Read, Unread, FeedAdded, FeedDeleted, FeedRead, Category, Interval:
		return true
	}
	return false
}
