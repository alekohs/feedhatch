package worker

import (
	"context"
	"time"

	"alekohs/feedhatch/internal/feed"
	"alekohs/feedhatch/internal/store"
)

// Run updates due feeds right away and then every minute until ctx is cancelled.
// report is called for each fetched feed; done is called after each round with any fetch errors.
func Run(ctx context.Context, s *store.Store, report func(feed string, added int, err error), done func(error)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		done(feed.UpdateFeeds(s, report))

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
