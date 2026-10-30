package store

import (
	"database/sql"
	"time"
)

func (s *Store) AddFeed(f Feed) (int64, error) {
	if f.Interval == 0 {
		f.Interval = 15
	}

	res, err := s.db.Exec("INSERT INTO feeds (url, name, category, interval) VALUES ($1, $2, $3, $4)", f.URL, f.Name, f.Category, f.Interval)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *Store) Feeds() ([]Feed, error) {
	rows, err := s.db.Query("SELECT id, url, name, category, interval, COALESCE(fetched_at, 0) FROM feeds ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		if err := rows.Scan(&f.ID, &f.URL, &f.Name, &f.Category, &f.Interval, &f.FetchedAt); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}

	return feeds, rows.Err()
}

func (s *Store) Articles(feedID int64) ([]Article, error) {
	rows, err := s.db.Query(`SELECT id, feed_id, guid, title, link, content, published_at, read
		FROM articles WHERE feed_id = $1 ORDER BY published_at DESC`, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		var published sql.NullInt64
		if err := rows.Scan(&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.Link, &a.Content, &published, &a.Read); err != nil {
			return nil, err
		}
		if published.Valid {
			a.PublishedAt = time.Unix(published.Int64, 0)
		}
		articles = append(articles, a)
	}

	return articles, rows.Err()
}

// SaveArticles inserts articles that are not already stored and returns how many were new.
func (s *Store) SaveArticles(feedID int64, articles []Article) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO articles (feed_id, guid, title, link, content, published_at, read)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	added := 0
	for _, a := range articles {
		var published sql.NullInt64
		if !a.PublishedAt.IsZero() {
			published = sql.NullInt64{Int64: a.PublishedAt.Unix(), Valid: true}
		}
		res, err := stmt.Exec(feedID, a.GUID, a.Title, a.Link, a.Content, published, a.Read)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		added += int(n)
	}

	return added, tx.Commit()
}

func (s *Store) SetRead(articleID int64, read bool) error {
	_, err := s.db.Exec("UPDATE articles SET read = $1 WHERE id = $2", read, articleID)
	return err
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

func (s *Store) SetCategory(feedID int64, category string) error {
	_, err := s.db.Exec("UPDATE feeds SET category = $1 WHERE id = $2", category, feedID)
	return err
}
