package store

import (
	"database/sql"
	"errors"
)

func (s *Store) AddFeed(f Feed) (int64, error) {
	if f.Interval == 0 {
		f.Interval = 15
	}

	res, err := s.db.Exec("INSERT INTO feeds (url, name, category, interval, notification) VALUES ($1, $2, $3, $4, $5)", f.URL, f.Name, f.Category, f.Interval, f.Notification)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *Store) RemoveFeed(f Feed) (bool, error) {
	res, err := s.db.Exec("DELETE FROM feeds WHERE id = $1", f.ID)
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows > 0, nil
}

func (s *Store) Feeds() ([]Feed, error) {
	rows, err := s.db.Query("SELECT id, url, name, category, interval, notification, COALESCE(fetched_at, 0) FROM feeds ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		if err := rows.Scan(&f.ID, &f.URL, &f.Name, &f.Category, &f.Interval, &f.Notification, &f.FetchedAt); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}

	return feeds, rows.Err()
}

// SetFetched set fetched at for feed
//
// feedID the id of the feed
//
// time unix time of when it was fetched
func (s *Store) SetFetched(feedID int64, time int64) error {
	_, err := s.db.Exec("UPDATE feeds SET fetched_at = $1 WHERE id = $2", time, feedID)
	return err
}

func (s *Store) SetInterval(feedID int64, interval int64) error {
	_, err := s.db.Exec("UPDATE feeds SET interval = $1 WHERE id = $2", interval, feedID)
	return err
}

// DeleteFeed removes the feed; its articles are removed with it (ON DELETE CASCADE).
func (s *Store) DeleteFeed(feedID int64) error {
	_, err := s.db.Exec("DELETE FROM feeds WHERE id = $1", feedID)
	return err
}

// FeedGone reports whether the feed has been deleted.
func (s *Store) FeedGone(feedID int64) bool {
	var id int64
	err := s.db.QueryRow("SELECT id FROM feeds WHERE id = $1", feedID).Scan(&id)
	return errors.Is(err, sql.ErrNoRows)
}

// SetNotification turns OS notifications for the feed's new articles on or off.
func (s *Store) SetNotification(feedID int64, on bool) error {
	_, err := s.db.Exec("UPDATE feeds SET notification = $1 WHERE id = $2", on, feedID)
	return err
}

func (s *Store) SetCategory(feedID int64, category string) error {
	_, err := s.db.Exec("UPDATE feeds SET category = $1 WHERE id = $2", category, feedID)
	return err
}
