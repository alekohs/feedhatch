package store

import "time"

type Feed struct {
	ID, Interval, FetchedAt int64
	URL, Name, Category     string
	Notification            bool
}

type Article struct {
	ID, FeedID        int64
	GUID, Title, Link string
	Content           string
	Image             string // a cover image URL, e.g. from the item's enclosure
	PublishedAt       time.Time
	Read              bool
}
