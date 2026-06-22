package streams

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

func newStreamsStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "streams.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func newTestEngine(t *testing.T, s *store.Store) *Engine {
	t.Helper()
	// Strava client + runner are unused by GetOrComputeAnalysis; pass minimal values.
	return New(s, strava.NewWithBase("1", "x", "http://cb", "http://unused"), garmin.Runner{}, nil)
}

// seedRawStream stores an activity + its gzipped raw stream (no analysis yet).
func seedRawStream(t *testing.T, s *store.Store, id int64, ser Series) {
	t.Helper()
	if err := s.UpsertActivity(store.Activity{
		StravaID: id, Name: "run", Type: "Run",
		StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000, MovingTimeS: 3300, ElapsedTimeS: 3300, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}
	gz, err := CompressSeries(ser)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := s.UpsertActivityStream(store.ActivityStream{
		ActivityID: id, Source: "strava", SeriesGz: gz, FetchedAt: "2026-06-20T07:00:00Z",
	}); err != nil {
		t.Fatalf("upsert stream: %v", err)
	}
}

func TestGetOrComputeAnalysisFirstCompute(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	seedRawStream(t, s, 100, ser)
	e := newTestEngine(t, s)

	got, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetOrComputeAnalysis error = %v", err)
	}
	if !got.HasHR || len(got.TimeInZone) != 5 {
		t.Errorf("analysis = %+v, want HasHR + 5 zones", got)
	}
	if got.ComputedAt == "" {
		t.Error("ComputedAt empty, want set on compute")
	}
	if got.Source != "strava" {
		t.Errorf("Source = %q, want strava (carried from raw)", got.Source)
	}
	if _, err := s.GetStreamAnalysis(100); err != nil {
		t.Errorf("analysis not cached: %v", err)
	}
}

func TestGetOrComputeAnalysisReturnsCached(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	seedRawStream(t, s, 100, ser)
	e := newTestEngine(t, s)

	first, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ComputedAt != first.ComputedAt {
		t.Errorf("recomputed unexpectedly: %q != %q", second.ComputedAt, first.ComputedAt)
	}
}

func TestGetOrComputeAnalysisRecomputesOnZoneChange(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{
		T:    []float64{0, 1, 2, 3},
		HR:   []float64{140, 140, 150, 150}, // 2 below 145, 2 above (default)
		V:    []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6},
	}
	seedRawStream(t, s, 100, ser)
	e := newTestEngine(t, s)

	first, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	firstZones := first.Zones

	z2 := int64(155)
	thr := int64(170)
	if err := s.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm: 20, ProgressionMode: "build", RunConstraintsJSON: "{}",
		Zone2CeilingBpm: &z2, ThresholdBpm: &thr,
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}

	second, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Zones == firstZones {
		t.Errorf("zones unchanged after profile change: %+v", second.Zones)
	}
	if second.Zones.Z2Hi != 155 {
		t.Errorf("recomputed Z2Hi = %v, want 155", second.Zones.Z2Hi)
	}
	row, err := s.GetStreamAnalysis(100)
	if err != nil {
		t.Fatalf("get analysis row: %v", err)
	}
	want, _ := json.Marshal(ZonesFromProfile(store.AthleteProfile{Zone2CeilingBpm: &z2, ThresholdBpm: &thr}))
	if row.ZonesJSON != string(want) {
		t.Errorf("cached zones_json = %s, want %s", row.ZonesJSON, want)
	}
}

func TestGetOrComputeAnalysisNotFetched(t *testing.T) {
	s := newStreamsStore(t)
	if err := s.UpsertActivity(store.Activity{
		StravaID: 100, Name: "run", Type: "Run",
		StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000, MovingTimeS: 3300, ElapsedTimeS: 3300, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}
	e := newTestEngine(t, s)
	_, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != store.ErrNotFound {
		t.Errorf("err = %v, want store.ErrNotFound (no raw stream stored)", err)
	}
}
