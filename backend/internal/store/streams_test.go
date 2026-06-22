package store

import (
	"testing"

	"help-my-run/backend/internal/streams"
)

// seedActivity inserts a minimal activities row so stream FKs resolve.
func seedActivity(t *testing.T, s *Store, id int64, startTime string) {
	t.Helper()
	if err := s.UpsertActivity(Activity{
		StravaID: id, Name: "Run", Type: "Run",
		StartTime: startTime, DistanceM: 5000, MovingTimeS: 1500, ElapsedTimeS: 1500,
		RawJSON: "{}",
	}); err != nil {
		t.Fatalf("seedActivity(%d): %v", id, err)
	}
}

func TestActivityStreamRoundTrip(t *testing.T) {
	s := newTestStore(t)
	seedActivity(t, s, 100, "2026-06-18T06:00:00Z")

	// Not stored yet.
	if has, err := s.HasActivityStream(100); err != nil || has {
		t.Fatalf("HasActivityStream(100) on empty = (%v, %v), want (false, nil)", has, err)
	}
	if _, err := s.GetActivityStream(100); err != ErrNotFound {
		t.Fatalf("GetActivityStream(100) on empty = %v, want ErrNotFound", err)
	}

	// gzip a real series and store it as a BLOB.
	in := streams.Series{T: []float64{0, 1, 2}, HR: []float64{100, 101, 102}, V: []float64{0, 1.5, 1.6}, Dist: []float64{0, 1.5, 3.1}}
	gz, err := streams.CompressSeries(in)
	if err != nil {
		t.Fatalf("CompressSeries: %v", err)
	}
	if err := s.UpsertActivityStream(ActivityStream{ActivityID: 100, Source: "strava", SeriesGz: gz}); err != nil {
		t.Fatalf("UpsertActivityStream: %v", err)
	}

	if has, err := s.HasActivityStream(100); err != nil || !has {
		t.Fatalf("HasActivityStream(100) after upsert = (%v, %v), want (true, nil)", has, err)
	}

	got, err := s.GetActivityStream(100)
	if err != nil {
		t.Fatalf("GetActivityStream(100): %v", err)
	}
	if got.ActivityID != 100 || got.Source != "strava" {
		t.Errorf("got = %+v, want activity_id=100 source=strava", got)
	}
	if got.FetchedAt == "" {
		t.Error("FetchedAt empty, want server-set RFC3339")
	}
	// BLOB survives the DB round-trip: decompress to the original Series.
	back, err := streams.DecompressSeries(got.SeriesGz)
	if err != nil {
		t.Fatalf("DecompressSeries(stored blob): %v", err)
	}
	if back.Len() != 3 || !back.HasHR() || back.HR[1] != 101 {
		t.Errorf("decompressed stored blob = %+v, want original 3-sample HR series", back)
	}

	// Re-upsert with a new blob -> update, not duplicate; FetchedAt refreshed.
	in2 := streams.Series{T: []float64{0, 1}, HR: nil, V: []float64{0, 1.5}, Dist: []float64{0, 1.5}}
	gz2, _ := streams.CompressSeries(in2)
	if err := s.UpsertActivityStream(ActivityStream{ActivityID: 100, Source: "garmin", SeriesGz: gz2}); err != nil {
		t.Fatalf("re-UpsertActivityStream: %v", err)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM activity_streams WHERE activity_id=100`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("row count = %d, want 1 (upsert, not duplicate)", n)
	}
	got2, _ := s.GetActivityStream(100)
	if got2.Source != "garmin" {
		t.Errorf("after re-upsert source = %q, want garmin", got2.Source)
	}
	back2, _ := streams.DecompressSeries(got2.SeriesGz)
	if back2.HasHR() {
		t.Errorf("re-upserted no-HR blob HasHR() = true, want false")
	}
}
