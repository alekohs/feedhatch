package store

import (
	"database/sql"
	"time"
)

func (s *Store) Articles(feedID int64) ([]Article, error) {
	rows, err := s.db.Query(`SELECT id, feed_id, guid, title, link, content, image, published_at, read
		FROM articles WHERE feed_id = $1 ORDER BY published_at DESC`, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		var published sql.NullInt64
		if err := rows.Scan(&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.Link, &a.Content, &a.Image, &published, &a.Read); err != nil {
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

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO articles (feed_id, guid, title, link, content, image, published_at, read)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	// Articles saved before images were stored get theirs on the next fetch.
	fillImage, err := tx.Prepare(`UPDATE articles SET image = $1 WHERE feed_id = $2 AND guid = $3 AND image = ''`)
	if err != nil {
		return 0, err
	}
	defer fillImage.Close()

	added := 0
	for _, a := range articles {
		var published sql.NullInt64
		if !a.PublishedAt.IsZero() {
			published = sql.NullInt64{Int64: a.PublishedAt.Unix(), Valid: true}
		}
		res, err := stmt.Exec(feedID, a.GUID, a.Title, a.Link, a.Content, a.Image, published, a.Read)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		added += int(n)
		if n == 0 && a.Image != "" {
			if _, err := fillImage.Exec(a.Image, feedID, a.GUID); err != nil {
				return 0, err
			}
		}
	}

	return added, tx.Commit()
}

func (s *Store) SetFeedRead(feedID int64, read bool) error {
	_, err := s.db.Exec("UPDATE articles SET read = $1 WHERE feed_id = $2", read, feedID)
	return err
}

func (s *Store) SetRead(articleID int64, read bool) error {
	_, err := s.db.Exec("UPDATE articles SET read = $1 WHERE id = $2", read, articleID)
	return err
}

// SetContent replaces an article's content, e.g. with the full article from its page.
func (s *Store) SetContent(articleID int64, content string) error {
	_, err := s.db.Exec("UPDATE articles SET content = $1 WHERE id = $2", content, articleID)
	return err
}
