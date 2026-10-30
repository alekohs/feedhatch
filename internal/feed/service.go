package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"alekohs/feedhatch/internal/store"
	"github.com/mmcdole/gofeed"
)

func Fetch(feed store.Feed) (*gofeed.Feed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	fp := gofeed.NewParser()
	content := string(body)

	result, err := fp.ParseString(content)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateFeeds fetches every feed whose interval has passed, saves its new articles
// and logs the outcome of each fetched feed.
func UpdateFeeds(s *store.Store, logger *log.Logger) error {
	feeds, err := s.Feeds()
	if err != nil {
		return err
	}

	now := time.Now()
	var errs []error
	for _, f := range feeds {
		if f.FetchedAt+f.Interval*60 > now.Unix() {
			continue
		}

		name := f.Name
		if name == "" {
			name = f.URL
		}

		result, err := Fetch(f)
		if err == nil {
			var added int
			added, err = s.SaveArticles(f.ID, articles(result))
			if err == nil {
				err = s.SetFetched(f.ID, now.Unix())
				logger.Printf("%s: %d new articles", name, added)
			}
		}
		if err != nil {
			logger.Printf("%s: %v", name, err)
			errs = append(errs, fmt.Errorf("%s: %w", f.URL, err))
		}
	}

	return errors.Join(errs...)
}

func articles(result *gofeed.Feed) []store.Article {
	var articles []store.Article
	for _, item := range result.Items {
		a := store.Article{GUID: item.GUID, Title: item.Title, Link: item.Link, Content: item.Content}
		if a.GUID == "" {
			a.GUID = item.Link
		}
		if a.Content == "" {
			a.Content = item.Description
		}
		if item.PublishedParsed != nil {
			a.PublishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			a.PublishedAt = *item.UpdatedParsed
		}
		articles = append(articles, a)
	}
	return articles
}
