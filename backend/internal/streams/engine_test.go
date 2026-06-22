package streams

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
	// matchToleranceS=120 (M3.2.1 default); existing GetOrComputeAnalysis tests never call resolveGarminID.
	return New(s, strava.NewWithBase("1", "x", "http://cb", "http://unused"), garmin.Runner{}, nil, 120)
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

// With a raw stream stored but NO athlete_profile row, GetOrComputeAnalysis must
// SUCCEED using default zones — a missing profile must not be conflated with a
// missing stream (which the handler renders as has_stream:false).
func TestGetOrComputeAnalysisMissingProfileUsesDefaults(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	seedRawStream(t, s, 100, ser)

	// Remove the seeded (id=1) profile row so GetAthleteProfile returns ErrNotFound.
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

func TestResolveGarminID(t *testing.T) {
	// Strava reference run: starts 2026-06-22T05:00:00Z, moving 1800s, 5000m.
	const stravaID = int64(900)
	const startISO = "2026-06-22T05:00:00Z"

	type cand struct {
		id      int64
		start   string
		durS    *float64 // nil -> NULL duration_s
		distM   *float64 // nil -> NULL distance_m
		actType *string  // nil -> NULL activity_type (excluded by LIKE '%running%')
	}
	run := strp("running")
	trail := strp("trail_running")
	bike := strp("cycling")

	tests := []struct {
		name    string
		seedAct bool // upsert the Strava activities row?
		cands   []cand
		wantID  int64
		wantOK  bool
	}{
		{
			name:    "single match within tolerance",
			seedAct: true,
			cands: []cand{
				{id: 1, start: "2026-06-22T05:00:30Z", durS: f64p(1805), distM: f64p(5010), actType: run},
			},
			wantID: 1, wantOK: true,
		},
		{
			name:    "no candidate within tolerance -> false",
			seedAct: true,
			cands: []cand{
				{id: 2, start: "2026-06-22T05:10:00Z", durS: f64p(1800), distM: f64p(5000), actType: run}, // +600s > 120
			},
			wantID: 0, wantOK: false,
		},
		{
			name:    "tie-break by closest duration (vs MovingTimeS)",
			seedAct: true,
			cands: []cand{
				{id: 3, start: "2026-06-22T05:00:10Z", durS: f64p(1900), distM: f64p(5000), actType: run}, // |1900-1800|=100
				{id: 4, start: "2026-06-22T05:00:20Z", durS: f64p(1810), distM: f64p(9999), actType: run}, // |1810-1800|=10  (wins on duration)
			},
			wantID: 4, wantOK: true,
		},
		{
			name:    "tie-break by distance when duration ties",
			seedAct: true,
			cands: []cand{
				{id: 5, start: "2026-06-22T05:00:10Z", durS: f64p(1820), distM: f64p(5200), actType: run},   // durDelta=20, distDelta=200
				{id: 6, start: "2026-06-22T05:00:20Z", durS: f64p(1820), distM: f64p(5010), actType: trail}, // durDelta=20, distDelta=10 (wins)
			},
			wantID: 6, wantOK: true,
		},
		{
			name:    "non-run candidate excluded -> false",
			seedAct: true,
			cands: []cand{
				{id: 7, start: "2026-06-22T05:00:05Z", durS: f64p(1800), distM: f64p(5000), actType: bike},
			},
			wantID: 0, wantOK: false,
		},
		{
			name:    "unknown strava activity -> false",
			seedAct: false,
			cands: []cand{
				{id: 8, start: "2026-06-22T05:00:05Z", durS: f64p(1800), distM: f64p(5000), actType: run},
			},
			wantID: 0, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStreamsStore(t)
			if tt.seedAct {
				if err := s.UpsertActivity(store.Activity{
					StravaID: stravaID, Name: "run", Type: "Run",
					StartTime: startISO, DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
				}); err != nil {
					t.Fatalf("upsert activity: %v", err)
				}
			}
			for _, c := range tt.cands {
				if err := s.UpsertGarminActivity(store.GarminActivityRow{
					GarminActivityID: c.id, StartTime: c.start, DurationS: c.durS,
					DistanceM: c.distM, ActivityType: c.actType, RawJSON: "null",
				}); err != nil {
					t.Fatalf("upsert garmin activity %d: %v", c.id, err)
				}
			}
			e := newTestEngine(t, s)
			gid, ok := e.resolveGarminID(stravaID)
			if ok != tt.wantOK || gid != tt.wantID {
				t.Errorf("resolveGarminID = (%d,%v), want (%d,%v)", gid, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestFetchAndAnalyzeGarminFallbackActivates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh for the FIT runner stub")
	}
	const stravaID = int64(14820001234)
	const garminID = int64(555)
	const startISO = "2026-06-22T05:00:00Z"

	s := newStreamsStore(t)

	// (1) Strava activity row (so resolveGarminID can load start/duration/distance).
	if err := s.UpsertActivity(store.Activity{
		StravaID: stravaID, Name: "no-HR run", Type: "Run",
		StartTime: startISO, DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}

	// (2) Seeded garmin_activities row that matches within tolerance (run-type).
	if err := s.UpsertGarminActivity(store.GarminActivityRow{
		GarminActivityID: garminID, StartTime: "2026-06-22T05:00:20Z",
		DurationS: f64p(1805), DistanceM: f64p(5010), ActivityType: strp("running"), RawJSON: "null",
	}); err != nil {
		t.Fatalf("upsert garmin activity: %v", err)
	}

	// (3) Non-expired Strava token so accessToken() does NOT attempt a refresh HTTP call.
	if err := s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "live-token", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Scope: "read", AthleteID: 1,
	}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}

	// (4) Strava streams HTTP stub: NO "heartrate" key -> a no-HR stream.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"time":{"data":[0,1,2,3]},"velocity_smooth":{"data":[2.0,2.0,2.0,2.0]},"distance":{"data":[0,2,4,6]}}`))
	}))
	t.Cleanup(srv.Close)
	sc := strava.NewWithBase("1", "x", "http://cb", srv.URL)

	// (5) FIT runner stub: a /bin/sh script echoing the FIT JSON WITH HR. It
	// asserts it was called with the resolved garmin id + the strava echo id.
	const fitOut = `{"activity_id":14820001234,"source":"garmin","fetched_at":"2026-06-22T05:00:12Z","series":{"t":[0,1,2,3],"hr":[140,142,150,152],"v":[2.0,2.0,2.0,2.0],"dist":[0,2,4,6]}}`
	script := filepath.Join(t.TempDir(), "fitstub.sh")
	body := "#!/bin/sh\n" +
		"echo \"$@\" | grep -q -- '--activity-id 555' || { echo 'missing --activity-id 555' 1>&2; exit 2; }\n" +
		"echo \"$@\" | grep -q -- '--echo-id 14820001234' || { echo 'missing --echo-id 14820001234' 1>&2; exit 2; }\n" +
		"echo '" + fitOut + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fit stub: %v", err)
	}
	runner := garmin.Runner{Python: "/bin/sh", Script: script}

	// matchToleranceS=120: the 20s start delta is within tolerance.
	e := New(s, sc, runner, nil, 120)

	got, err := e.FetchAndAnalyze(context.Background(), stravaID)
	if err != nil {
		t.Fatalf("FetchAndAnalyze error = %v", err)
	}

	// Source flipped to garmin (the fallback supplied HR via the .FIT).
	if got.Source != "garmin" {
		t.Errorf("Source = %q, want garmin (FIT fallback activated)", got.Source)
	}
	if !got.HasHR {
		t.Errorf("HasHR = false, want true (Garmin .FIT carried HR)")
	}
	// Time-in-zone present (one bucket per zone) and decoupling computed.
	if len(got.TimeInZone) == 0 {
		t.Errorf("TimeInZone empty, want zone buckets from the Garmin HR series")
	}
	if got.DecouplingPct == nil {
		t.Errorf("DecouplingPct = nil, want computed (4-sample HR+pace series)")
	}

	// The stored raw stream is persisted with source=garmin.
	raw, err := s.GetActivityStream(stravaID)
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
		t.Errorf("stored series.HR = %v, want [140 142 150 152] (from the .FIT)", ser.HR)
	}
}
