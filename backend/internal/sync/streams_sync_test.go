package sync

import (
	"context"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

func newSyncTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/sync.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

// fakeFetcher records FetchAndAnalyze calls and can return *strava.ErrRateLimited.
type fakeFetcher struct {
	calls      []int64
	failAfter  int // return ErrRateLimited on the (failAfter+1)-th call; 0 = never
	retryAfter time.Time
}

func (f *fakeFetcher) FetchAndAnalyze(ctx context.Context, id int64) error {
	if f.failAfter > 0 && len(f.calls) >= f.failAfter {
		return &strava.ErrRateLimited{RetryAfter: f.retryAfter, ReadUsage: "100,1", ReadLimit: "100,1000"}
	}
	f.calls = append(f.calls, id)
	return nil
}

func seedRun(t *testing.T, s *store.Store, id int64, st string) {
	t.Helper()
	if err := s.UpsertActivity(store.Activity{StravaID: id, Name: "r", Type: "Run",
		StartTime: st, DistanceM: 5000, MovingTimeS: 1500, ElapsedTimeS: 1500, RawJSON: "{}"}); err != nil {
		t.Fatalf("seed run %d: %v", id, err)
	}
}

func TestTrickleStreamsRespectsBudget(t *testing.T) {
	s := newSyncTestStore(t)
	now := time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 5; i++ {
		seedRun(t, s, i, now.AddDate(0, 0, -int(i)).Format(time.RFC3339))
	}
	f := &fakeFetcher{}
	n := TrickleStreams(context.Background(), s, f, 12, 3, now)
	if n != 3 || len(f.calls) != 3 {
		t.Fatalf("fetched = %d / calls %d, want 3 / 3 (budget)", n, len(f.calls))
	}
	log, err := s.GetStreamFetchLog()
	if err != nil {
		t.Fatalf("GetStreamFetchLog: %v", err)
	}
	if log.Status != "ok" || log.LastFetched != 3 || log.TotalFetched != 3 {
		t.Errorf("log = %+v, want ok / last 3 / total 3", log)
	}
}

func TestTrickleStreams429PausesAndRecords(t *testing.T) {
	s := newSyncTestStore(t)
	now := time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 5; i++ {
		seedRun(t, s, i, now.AddDate(0, 0, -int(i)).Format(time.RFC3339))
	}
	retry := now.Add(15 * time.Minute)
	f := &fakeFetcher{failAfter: 2, retryAfter: retry}
	n := TrickleStreams(context.Background(), s, f, 12, 10, now)
	if n != 2 {
		t.Errorf("fetched = %d, want 2 before 429", n)
	}
	log, _ := s.GetStreamFetchLog()
	if log.Status != "rate_limited" || log.RateLimitedUntil == nil ||
		*log.RateLimitedUntil != retry.UTC().Format(time.RFC3339) {
		t.Errorf("log = %+v, want rate_limited + rateUntil %s", log, retry.UTC().Format(time.RFC3339))
	}
}

func TestTrickleStreamsSkipsWhileRateLimited(t *testing.T) {
	s := newSyncTestStore(t)
	now := time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)
	seedRun(t, s, 1, now.AddDate(0, 0, -1).Format(time.RFC3339))
	future := now.Add(10 * time.Minute).Format(time.RFC3339)
	if err := s.UpdateStreamFetchLog(store.StreamFetchLog{Source: "strava",
		Status: "rate_limited", RateLimitedUntil: &future}); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	f := &fakeFetcher{}
	n := TrickleStreams(context.Background(), s, f, 12, 10, now)
	if n != 0 || len(f.calls) != 0 {
		t.Errorf("fetched = %d / calls %d, want 0 / 0 (paused)", n, len(f.calls))
	}
}
