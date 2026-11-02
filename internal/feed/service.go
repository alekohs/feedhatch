package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"alekohs/feedhatch/internal/constants"
	"alekohs/feedhatch/internal/store"
	"github.com/mmcdole/gofeed"
)

const (
	maxBody    = 50 << 20 // larger responses are cut off, so a feed fails to parse
	retryDelay = 20 * 60  // seconds until a failed feed is tried again
)

func Fetch(ctx context.Context, feed store.Feed) (*gofeed.Feed, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", constants.UserAgent)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	// An error page is not a feed: say what the server said instead of failing to parse it.
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", res.Status)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
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
// and reports the outcome of each fetched feed.
// When ctx is cancelled it stops after the current request, without reporting it.
func UpdateFeeds(ctx context.Context, s *store.Store, report func(f store.Feed, added int, err error)) error {
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
		if err := update(ctx, s, f, now, report); ctx.Err() != nil {
			return nil
		} else if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// UpdateFeed fetches the feed with id right away, due or not, e.g. just after it was added.
func UpdateFeed(ctx context.Context, s *store.Store, id int64, report func(f store.Feed, added int, err error)) error {
	feeds, err := s.Feeds()
	if err != nil {
		return err
	}
	for _, f := range feeds {
		if f.ID == id {
			if err := update(ctx, s, f, time.Now(), report); ctx.Err() == nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("no feed with id %d", id)
}

// update fetches one feed and saves its new articles; a cancelled fetch is not reported.
func update(ctx context.Context, s *store.Store, f store.Feed, now time.Time, report func(f store.Feed, added int, err error)) error {
	var added int
	result, err := Fetch(ctx, f)
	if ctx.Err() != nil {
		return nil
	}
	if err == nil {
		added, err = s.SaveArticles(f.ID, articles(result))
	}
	// Deleted while it was being fetched: nobody is waiting for it, so it is no failure.
	if err != nil && s.FeedGone(f.ID) {
		return nil
	}
	fetched := now.Unix()
	if err != nil {
		// Dated so that the feed is due again after retryDelay, or its interval if shorter.
		fetched -= max(0, f.Interval*60-retryDelay)
	}
	if setErr := s.SetFetched(f.ID, fetched); err == nil {
		err = setErr
	}
	report(f, added, err)
	if err != nil {
		return fmt.Errorf("%s: %w", f.URL, err)
	}
	return nil
}

func articles(result *gofeed.Feed) []store.Article {
	var articles []store.Article
	for _, item := range result.Items {
		a := store.Article{GUID: item.GUID, Title: item.Title, Link: item.Link, Content: item.Content}
		if a.GUID == "" {
			a.GUID = item.Link
		}
		if a.GUID == "" {
			a.GUID = item.Title + "|" + item.Published
		}
		if a.Content == "" {
			a.Content = item.Description
		}
		if item.Image != nil {
			a.Image = item.Image.URL
		}
		for _, e := range item.Enclosures {
			if a.Image == "" && strings.HasPrefix(e.Type, "image/") {
				a.Image = e.URL
			}
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
