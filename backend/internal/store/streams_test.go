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

func TestStreamAnalysisRoundTrip(t *testing.T) {
	s := newTestStore(t)
	seedActivity(t, s, 100, "2026-06-18T06:00:00Z")

	if _, err := s.GetStreamAnalysis(100); err != ErrNotFound {
		t.Fatalf("GetStreamAnalysis(100) on empty = %v, want ErrNotFound", err)
	}

	dp, p1, p2 := 4.2, 0.0212, 0.0203
	row := StreamAnalysisRow{
		ActivityID:     100,
		TimeInZoneJSON: `[{"zone":1,"seconds":120,"pct":4},{"zone":2,"seconds":2400,"pct":80}]`,
		DecouplingPct:  &dp,
		PaHRFirst:      &p1,
		PaHRSecond:     &p2,
		ZonesJSON:      `{"z1_hi":116,"z2_hi":145,"z3_hi":157.5,"z4_hi":170}`,
		HasHR:          true,
		ComputedAt:     "2026-06-22T07:00:00Z",
	}
	if err := s.UpsertStreamAnalysis(row); err != nil {
		t.Fatalf("UpsertStreamAnalysis: %v", err)
	}

	got, err := s.GetStreamAnalysis(100)
	if err != nil {
		t.Fatalf("GetStreamAnalysis: %v", err)
	}
	if got.ActivityID != 100 || !got.HasHR || got.ComputedAt != "2026-06-22T07:00:00Z" {
		t.Errorf("got = %+v, want id=100 has_hr=true computed_at set", got)
	}
	if got.DecouplingPct == nil || *got.DecouplingPct != 4.2 {
		t.Errorf("DecouplingPct = %v, want 4.2", got.DecouplingPct)
	}
	if got.PaHRFirst == nil || *got.PaHRFirst != 0.0212 {
		t.Errorf("PaHRFirst = %v, want 0.0212", got.PaHRFirst)
	}
	if got.TimeInZoneJSON != row.TimeInZoneJSON || got.ZonesJSON != row.ZonesJSON {
		t.Errorf("json columns mismatch: tiz=%q zones=%q", got.TimeInZoneJSON, got.ZonesJSON)
	}

	// No-HR row: nullable REALs stored null -> nil pointers; has_hr=false.
	seedActivity(t, s, 200, "2026-06-17T06:00:00Z")
	if err := s.UpsertStreamAnalysis(StreamAnalysisRow{
		ActivityID: 200, TimeInZoneJSON: "[]", DecouplingPct: nil, PaHRFirst: nil, PaHRSecond: nil,
		ZonesJSON: `{"z1_hi":116,"z2_hi":145,"z3_hi":157.5,"z4_hi":170}`, HasHR: false,
		ComputedAt: "2026-06-22T07:01:00Z",
	}); err != nil {
		t.Fatalf("UpsertStreamAnalysis no-HR: %v", err)
	}
	got2, _ := s.GetStreamAnalysis(200)
	if got2.HasHR {
		t.Error("no-HR row HasHR = true, want false")
	}
	if got2.DecouplingPct != nil || got2.PaHRFirst != nil || got2.PaHRSecond != nil {
		t.Errorf("no-HR row nullable REALs = (%v,%v,%v), want all nil",
			got2.DecouplingPct, got2.PaHRFirst, got2.PaHRSecond)
	}

	// ListStreamAnalyses: most-recent-first by joined activity start_time (200=06-17 older, 100=06-18 newer).
	list, err := s.ListStreamAnalyses(30)
	if err != nil {
		t.Fatalf("ListStreamAnalyses: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListStreamAnalyses len = %d, want 2", len(list))
	}
	if list[0].ActivityID != 100 || list[1].ActivityID != 200 {
		t.Errorf("order = [%d,%d], want [100,200] (newest start_time first)", list[0].ActivityID, list[1].ActivityID)
	}

	// Re-upsert 100 -> update not duplicate.
	newDp := 3.1
	row.DecouplingPct = &newDp
	row.ComputedAt = "2026-06-22T08:00:00Z"
	if err := s.UpsertStreamAnalysis(row); err != nil {
		t.Fatalf("re-UpsertStreamAnalysis: %v", err)
	}
	got3, _ := s.GetStreamAnalysis(100)
	if got3.DecouplingPct == nil || *got3.DecouplingPct != 3.1 || got3.ComputedAt != "2026-06-22T08:00:00Z" {
		t.Errorf("after re-upsert = %+v, want decoupling=3.1 computed_at=08:00", got3)
	}
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM stream_analyses`).Scan(&n)
	if n != 2 {
		t.Errorf("stream_analyses count = %d, want 2", n)
	}

	// LIMIT honored.
	one, _ := s.ListStreamAnalyses(1)
	if len(one) != 1 || one[0].ActivityID != 100 {
		t.Errorf("ListStreamAnalyses(1) = %+v, want single [100]", one)
	}
}
