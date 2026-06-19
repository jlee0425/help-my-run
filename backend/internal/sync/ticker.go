package sync

import (
	"context"
	"time"
)

// RunTicker calls fn every interval until ctx is cancelled. fn receives the
// same ctx so a cancellation also aborts an in-flight sync. RunTicker does not
// run fn immediately on start; the first call happens after one interval.
func RunTicker(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}
