package streams

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
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
	// M4: runner is unused by GetOrComputeAnalysis; pass a zero Runner.
	return New(s, garmin.Runner{}, nil)
}

// seedRawStream stores an activity + its gzipped raw stream (no analysis yet).
func seedRawStream(t *testing.T, s *store.Store, id int64, ser Series) {
	t.Helper()
	if err := s.UpsertActivity(store.Activity{
		ActivityID: id, Name: "run", Type: "Run",
		StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000, MovingTimeS: 3300, ElapsedTimeS: 3300, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}
	gz, err := CompressSeries(ser)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := s.UpsertActivityStream(store.ActivityStream{
		ActivityID: id, Source: "garmin", SeriesGz: gz, FetchedAt: "2026-06-20T07:00:00Z",
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
	if got.Source != "garmin" {
		t.Errorf("Source = %q, want garmin (carried from raw)", got.Source)
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
		T:  []float64{0, 1, 2, 3},
		HR: []float64{140, 140, 150, 150}, // 2 below 145, 2 above (default)
		V:  []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6},
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
		ActivityID: 100, Name: "run", Type: "Run",
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

// With a raw stream stored but NO athlete_profile row, GetOrComputeAnalysis must
// SUCCEED using default zones — a missing profile must not be conflated with a
// missing stream.
func TestGetOrComputeAnalysisMissingProfileUsesDefaults(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	seedRawStream(t, s, 100, ser)

	if _, err := s.DB.Exec(`DELETE FROM athlete_profile`); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if _, err := s.GetAthleteProfile(); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("precondition: GetAthleteProfile err = %v, want store.ErrNotFound", err)
	}

	e := newTestEngine(t, s)
	got, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetOrComputeAnalysis with missing profile error = %v, want success with default zones", err)
	}
	want := ZonesFromProfile(store.AthleteProfile{})
	if got.Zones != want {
		t.Errorf("zones = %+v, want defaults %+v", got.Zones, want)
	}
}

// M4: resolveGarminID is identity — the activity id IS the Garmin download id.
func TestResolveGarminIDIsIdentity(t *testing.T) {
	s := newStreamsStore(t)
	e := newTestEngine(t, s)
	for _, id := range []int64{1, 14820001234, 999} {
		gid, ok := e.resolveGarminID(id)
		if !ok || gid != id {
			t.Errorf("resolveGarminID(%d) = (%d,%v), want (%d,true)", id, gid, ok, id)
		}
	}
}

// M4: FetchAndAnalyze uses the .FIT worker as the SOLE stream source.
func TestFetchAndAnalyzeFITOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh for the FIT runner stub")
	}
	const activityID = int64(14820001234)
	const startISO = "2026-06-22T05:00:00Z"

	s := newStreamsStore(t)
	if err := s.UpsertActivity(store.Activity{
		ActivityID: activityID, Name: "run", Type: "Run",
		StartTime: startISO, DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}

	// FIT runner stub: /bin/sh script echoing the FIT JSON with HR. It asserts
	// it was called with the identity garmin id == echo id.
	const fitOut = `{"activity_id":14820001234,"source":"garmin","fetched_at":"2026-06-22T05:00:12Z","series":{"t":[0,1,2,3],"hr":[140,142,150,152],"v":[2.0,2.0,2.0,2.0],"dist":[0,2,4,6]}}`
	script := filepath.Join(t.TempDir(), "fitstub.sh")
	body := "#!/bin/sh\n" +
		"echo \"$@\" | grep -q -- '--activity-id 14820001234' || { echo 'missing --activity-id 14820001234' 1>&2; exit 2; }\n" +
		"echo \"$@\" | grep -q -- '--echo-id 14820001234' || { echo 'missing --echo-id 14820001234' 1>&2; exit 2; }\n" +
		"echo '" + fitOut + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fit stub: %v", err)
	}
	runner := garmin.Runner{Python: "/bin/sh", Script: script}

	e := New(s, runner, nil)

	got, err := e.FetchAndAnalyze(context.Background(), activityID)
	if err != nil {
		t.Fatalf("FetchAndAnalyze error = %v", err)
	}
	if got.Source != "garmin" {
		t.Errorf("Source = %q, want garmin", got.Source)
	}
	if !got.HasHR {
		t.Errorf("HasHR = false, want true (Garmin .FIT carried HR)")
	}
	if len(got.TimeInZone) == 0 {
		t.Errorf("TimeInZone empty, want zone buckets from the Garmin HR series")
	}
	if got.DecouplingPct == nil {
		t.Errorf("DecouplingPct = nil, want computed (4-sample HR+pace series)")
	}

	raw, err := s.GetActivityStream(activityID)
	if err != nil {
		t.Fatalf("GetActivityStream: %v", err)
	}
	if raw.Source != "garmin" {
		t.Errorf("stored activity_streams.source = %q, want garmin", raw.Source)
	}
	ser, err := DecompressSeries(raw.SeriesGz)
	if err != nil {
		t.Fatalf("decompress stored series: %v", err)
	}
	if len(ser.HR) != 4 || ser.HR[0] != 140 {
		t.Errorf("stored series.HR = %v, want [140 142 150 152]", ser.HR)
	}
}

// M4: FIT fetch error propagates from FetchAndAnalyze (no Strava degrade path).
func TestFetchAndAnalyzeFITErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh for the FIT runner stub")
	}
	s := newStreamsStore(t)
	if err := s.UpsertActivity(store.Activity{
		ActivityID: 7, Name: "run", Type: "Run",
		StartTime: "2026-06-22T05:00:00Z", DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'fit boom' 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	e := New(s, garmin.Runner{Python: "/bin/sh", Script: script}, nil)
	if _, err := e.FetchAndAnalyze(context.Background(), 7); err == nil {
		t.Fatal("FetchAndAnalyze err = nil, want propagated FIT error")
	}
}
