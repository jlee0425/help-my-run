package store

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"
)

// testSeries mirrors streams.Series' JSON wire shape (struct-of-arrays
// {t,hr,v,dist}). It is duplicated here, rather than imported from
// internal/streams, because internal/streams imports internal/store and a
// streams_test.go in package store importing internal/streams would form an
// import cycle ("import cycle not allowed in test"). The store layer treats the
// blob opaquely, so an independent codec with the same JSON shape exercises the
// BLOB round-trip identically.
type testSeries struct {
	T    []float64 `json:"t"`
	HR   []float64 `json:"hr"`
	V    []float64 `json:"v"`
	Dist []float64 `json:"dist"`
}

func (s testSeries) HasHR() bool { return len(s.HR) > 0 }
func (s testSeries) Len() int    { return len(s.T) }

// compressTestSeries marshals s to JSON and gzips it, matching
// streams.CompressSeries' wire format byte-for-byte at the JSON layer.
func compressTestSeries(t *testing.T, s testSeries) []byte {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal series: %v", err)
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatalf("new gzip writer: %v", err)
	}
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// decompressTestSeries gunzips gz and unmarshals it back to a testSeries,
// matching streams.DecompressSeries.
func decompressTestSeries(t *testing.T, gz []byte) testSeries {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatalf("new gzip reader: %v", err)
	}
	defer func() { _ = zr.Close() }()
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	var s testSeries
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal series: %v", err)
	}
	return s
}

// seedActivity inserts a minimal activities row so stream FKs resolve.
func seedActivity(t *testing.T, s *Store, id int64, startTime string) {
	t.Helper()
	if err := s.UpsertActivity(Activity{
		ActivityID: id, Name: "Run", Type: "Run",
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
	in := testSeries{T: []float64{0, 1, 2}, HR: []float64{100, 101, 102}, V: []float64{0, 1.5, 1.6}, Dist: []float64{0, 1.5, 3.1}}
	gz := compressTestSeries(t, in)
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
	back := decompressTestSeries(t, got.SeriesGz)
	if back.Len() != 3 || !back.HasHR() || back.HR[1] != 101 {
		t.Errorf("decompressed stored blob = %+v, want original 3-sample HR series", back)
	}

	// Re-upsert with a new blob -> update, not duplicate; FetchedAt refreshed.
	in2 := testSeries{T: []float64{0, 1}, HR: nil, V: []float64{0, 1.5}, Dist: []float64{0, 1.5}}
	gz2 := compressTestSeries(t, in2)
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
	back2 := decompressTestSeries(t, got2.SeriesGz)
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

func TestStreamFetchLogGetAndUpdate(t *testing.T) {
	s := newTestStore(t)

	// Seeded by migration 00006.
	fl, err := s.GetStreamFetchLog()
	if err != nil {
		t.Fatalf("GetStreamFetchLog seed = %v", err)
	}
	if fl.Source != "strava" || fl.Status != "never" {
		t.Errorf("seed = %+v, want source=strava status=never", fl)
	}
	if fl.LastFetched != 0 || fl.TotalFetched != 0 {
		t.Errorf("seed counters = (%d,%d), want (0,0)", fl.LastFetched, fl.TotalFetched)
	}
	if fl.CursorTime != nil || fl.LastRunAt != nil || fl.Error != nil || fl.RateLimitedUntil != nil {
		t.Errorf("seed nullable fields = %+v, want all nil", fl)
	}

	// OK update with counters + cursor.
	cursor := "2026-04-01T06:00:00Z"
	runAt := "2026-06-22T05:00:00Z"
	if err := s.UpdateStreamFetchLog(StreamFetchLog{
		Source: "strava", CursorTime: &cursor, LastRunAt: &runAt,
		LastFetched: 7, TotalFetched: 42, Status: "ok", Error: nil, RateLimitedUntil: nil,
	}); err != nil {
		t.Fatalf("UpdateStreamFetchLog ok = %v", err)
	}
	got, _ := s.GetStreamFetchLog()
	if got.Status != "ok" || got.LastFetched != 7 || got.TotalFetched != 42 {
		t.Errorf("after ok update = %+v, want status=ok last=7 total=42", got)
	}
	if got.CursorTime == nil || *got.CursorTime != cursor {
		t.Errorf("CursorTime = %v, want %s", got.CursorTime, cursor)
	}

	// rate_limited update: error + rate_limited_until set.
	rlErr := "strava 429"
	until := "2026-06-22T05:15:00Z"
	if err := s.UpdateStreamFetchLog(StreamFetchLog{
		Source: "strava", CursorTime: &cursor, LastRunAt: &runAt,
		LastFetched: 0, TotalFetched: 42, Status: "rate_limited",
		Error: &rlErr, RateLimitedUntil: &until,
	}); err != nil {
		t.Fatalf("UpdateStreamFetchLog rate_limited = %v", err)
	}
	rl, _ := s.GetStreamFetchLog()
	if rl.Status != "rate_limited" || rl.Error == nil || *rl.Error != rlErr {
		t.Errorf("rate_limited = %+v, want status=rate_limited error=%q", rl, rlErr)
	}
	if rl.RateLimitedUntil == nil || *rl.RateLimitedUntil != until {
		t.Errorf("RateLimitedUntil = %v, want %s", rl.RateLimitedUntil, until)
	}

	// Single-row table: update never inserts a second row.
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM stream_fetch_log`).Scan(&n)
	if n != 1 {
		t.Errorf("stream_fetch_log row count = %d, want 1", n)
	}
}

func TestListRecentRunsWithoutStream(t *testing.T) {
	s := newTestStore(t)
	seedActivity(t, s, 1, "2026-06-21T06:00:00Z") // recent, no stream -> included
	seedActivity(t, s, 2, "2026-06-20T06:00:00Z") // recent, HAS stream -> excluded
	seedActivity(t, s, 3, "2026-01-01T06:00:00Z") // too old -> excluded
	if err := s.UpsertActivityStream(ActivityStream{ActivityID: 2, Source: "strava", SeriesGz: []byte{1}}); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	ids, err := s.ListRecentRunsWithoutStream("2026-04-01T00:00:00Z", 10)
	if err != nil {
		t.Fatalf("ListRecentRunsWithoutStream: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Errorf("ids = %v, want [1] (2 has stream, 3 too old)", ids)
	}
}
