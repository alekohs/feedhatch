package worker

import (
	"context"
	"log"
	"time"

	"alekohs/feedhatch/internal/feed"
	"alekohs/feedhatch/internal/store"
)

// Run updates due feeds right away and then every minute until ctx is cancelled.
// Each fetched feed is logged to logger; done is called after each round with any fetch errors.
func Run(ctx context.Context, s *store.Store, logger *log.Logger, done func(error)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		done(feed.UpdateFeeds(s, logger))

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
