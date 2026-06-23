package sync

import (
	"context"
	"time"

	"help-my-run/backend/internal/store"
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
// On any fetch error it stops and records "error". Returns the count fetched.
// Never errors the surrounding Garmin sync.
func TrickleStreams(ctx context.Context, s *store.Store, f streamFetcher, weeks, budget int, now time.Time) int {
	const source = "strava" // opaque single-row stream_fetch_log key (M4)

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
