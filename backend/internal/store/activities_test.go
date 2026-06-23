package store

import "testing"

func TestLatestActivityStartTimeEmpty(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.LatestActivityStartTime(); err != ErrNotFound {
		t.Fatalf("LatestActivityStartTime on empty = %v, want ErrNotFound", err)
	}
}

func TestLatestActivityStartTime(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertActivity(Activity{
		ActivityID: 1, Name: "older", Type: "Run", StartTime: "2026-06-10T06:00:00Z",
		DistanceM: 5000, MovingTimeS: 1500, ElapsedTimeS: 1500, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert older: %v", err)
	}
	if err := s.UpsertActivity(Activity{
		ActivityID: 2, Name: "newer", Type: "Run", StartTime: "2026-06-18T18:00:00Z",
		DistanceM: 8000, MovingTimeS: 2400, ElapsedTimeS: 2400, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert newer: %v", err)
	}

	got, err := s.LatestActivityStartTime()
	if err != nil {
		t.Fatalf("LatestActivityStartTime() error = %v", err)
	}
	if got != "2026-06-18T18:00:00Z" {
		t.Errorf("LatestActivityStartTime() = %q, want 2026-06-18T18:00:00Z", got)
	}
}
