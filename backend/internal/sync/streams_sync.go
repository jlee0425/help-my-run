package sync

import (
	"context"
	"errors"
	"time"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// streamFetcher is the per-run fetch+analyze seam (satisfied by *streams.Engine).
type streamFetcher interface {
	FetchAndAnalyze(ctx context.Context, activityID int64) error
}

// StreamTrickle is the optional recent-window stream-fetch hook, wired from
// main.go. nil = disabled (e.g. in pure activity-sync tests).
type StreamTrickle struct {
	Fetcher streamFetcher
	Weeks   int
	Budget  int
}

// TrickleStreams fetches up to budget recent-window (last `weeks`) runs lacking a
// stream, most-recent-first, recording resumable progress in stream_fetch_log.
// On *strava.ErrRateLimited it stops and records rate_limited + rate_limited_until.
// While now < rate_limited_until it skips entirely. Returns the count fetched.
// Never errors the surrounding Strava sync.
func TrickleStreams(ctx context.Context, s *store.Store, f streamFetcher, weeks, budget int, now time.Time) int {
	const source = "strava"

	if log, err := s.GetStreamFetchLog(); err == nil && log.RateLimitedUntil != nil {
		if until, perr := time.Parse(time.RFC3339, *log.RateLimitedUntil); perr == nil && now.Before(until) {
			return 0
		}
	}

	sinceISO := now.AddDate(0, 0, -7*weeks).UTC().Format(time.RFC3339)
	ids, err := s.ListRecentRunsWithoutStream(sinceISO, budget)
	if err != nil {
		recordTrickle(s, store.StreamFetchLog{Source: source, Status: "error",
			Error: strptrErr(err), LastRunAt: nowPtr(now)}, prevTotal(s))
		return 0
	}

	fetched := 0
	for _, id := range ids {
		if err := f.FetchAndAnalyze(ctx, id); err != nil {
			var rl *strava.ErrRateLimited
			if errors.As(err, &rl) {
				until := rl.RetryAfter.UTC().Format(time.RFC3339)
				recordTrickle(s, store.StreamFetchLog{Source: source, Status: "rate_limited",
					RateLimitedUntil: &until, LastFetched: int64(fetched), LastRunAt: nowPtr(now)},
					prevTotal(s)+int64(fetched))
				return fetched
			}
			recordTrickle(s, store.StreamFetchLog{Source: source, Status: "error",
				Error: strptrErr(err), LastFetched: int64(fetched), LastRunAt: nowPtr(now)},
				prevTotal(s)+int64(fetched))
			return fetched
		}
		fetched++
	}

	recordTrickle(s, store.StreamFetchLog{Source: source, Status: "ok",
		LastFetched: int64(fetched), LastRunAt: nowPtr(now)}, prevTotal(s)+int64(fetched))
	return fetched
}

func recordTrickle(s *store.Store, l store.StreamFetchLog, total int64) {
	l.TotalFetched = total
	_ = s.UpdateStreamFetchLog(l)
}

func prevTotal(s *store.Store) int64 {
	if l, err := s.GetStreamFetchLog(); err == nil {
		return l.TotalFetched
	}
	return 0
}

func nowPtr(now time.Time) *string {
	v := now.UTC().Format(time.RFC3339)
	return &v
}

func strptrErr(err error) *string {
	m := err.Error()
	return &m
}
