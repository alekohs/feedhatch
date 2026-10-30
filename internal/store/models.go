package store

import "time"

type Feed struct {
	ID, Interval, FetchedAt int64
	URL, Name, Category     string
}

type Article struct {
	ID, FeedID        int64
	GUID, Title, Link string
	Content           string
	PublishedAt       time.Time
	Read              bool
}
