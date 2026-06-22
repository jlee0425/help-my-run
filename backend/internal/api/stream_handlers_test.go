package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
	"help-my-run/backend/internal/streams"
)

func newStreamsServer(t *testing.T, fs *fakeStreams) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "streams-api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("1", "x", "http://cb", "http://unused"),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) {
			return "ok", 0, nil, "ok", 0, nil
		},
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
		Agent:    &fakeAgent{},
		Pusher:   &fakePusher{},
		Progress: &fakeProgress{},
		Streams:  fs,
	}
	return NewRouter(deps)
}

func fpv(v float64) *float64 { return &v }

func TestActivityAnalysisRequiresAuth(t *testing.T) {
	h := newStreamsServer(t, &fakeStreams{})
	rec := do(t, h, http.MethodGet, "/api/activities/123/analysis", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
		t.Errorf("body = %q, want unauthorized", rec.Body.String())
	}
}

func TestActivityAnalysisInvalidID(t *testing.T) {
	h := newStreamsServer(t, &fakeStreams{})
	rec := do(t, h, http.MethodGet, "/api/activities/notanint/analysis", testToken)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestActivityAnalysisFetchedWithHR(t *testing.T) {
	fs := &fakeStreams{analysis: streams.StreamAnalysis{
		ActivityID: 14820001234,
		HasHR:      true,
		TimeInZone: []streams.ZoneTime{
			{Zone: 1, Seconds: 120, Pct: 4.0},
			{Zone: 2, Seconds: 2400, Pct: 80.0},
			{Zone: 3, Seconds: 480, Pct: 16.0},
			{Zone: 4, Seconds: 0, Pct: 0.0},
			{Zone: 5, Seconds: 0, Pct: 0.0},
		},
		DecouplingPct: fpv(4.2), PaHRFirst: fpv(0.0212), PaHRSecond: fpv(0.0203),
		Zones:      streams.ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170},
		Source:     "strava", ComputedAt: "2026-06-22T07:00:00Z",
	}}
	h := newStreamsServer(t, fs)
	rec := do(t, h, http.MethodGet, "/api/activities/14820001234/analysis", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fs.lastGetID != 14820001234 {
		t.Errorf("lastGetID = %d, want 14820001234", fs.lastGetID)
	}
	var body streamAnalysisDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.HasStream || !body.HasHR || body.ActivityID != 14820001234 {
		t.Errorf("dto = %+v, want has_stream/has_hr true id 14820001234", body)
	}
	if len(body.TimeInZone) != 5 || body.TimeInZone[1].Pct != 80.0 {
		t.Errorf("time_in_zone = %+v, want 5 zones z2=80%%", body.TimeInZone)
	}
	if body.DecouplingPct == nil || *body.DecouplingPct != 4.2 {
		t.Errorf("decoupling_pct = %v, want 4.2", body.DecouplingPct)
	}
	s := rec.Body.String()
	for _, tag := range []string{`"has_stream"`, `"has_hr"`, `"time_in_zone"`, `"decoupling_pct"`, `"pa_hr_first"`, `"z2_hi"`} {
		if !strings.Contains(s, tag) {
			t.Errorf("wire JSON missing %s: %s", tag, s)
		}
	}
}

func TestActivityAnalysisNotFetched(t *testing.T) {
	fs := &fakeStreams{getErr: store.ErrNotFound}
	h := newStreamsServer(t, fs)
	rec := do(t, h, http.MethodGet, "/api/activities/14820001234/analysis", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (not-fetched is 200, not 404)", rec.Code)
	}
	var body streamAnalysisDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.HasStream {
		t.Errorf("has_stream = true, want false (not fetched)")
	}
	if body.ActivityID != 14820001234 {
		t.Errorf("activity_id = %d, want echoed 14820001234", body.ActivityID)
	}
	if body.TimeInZone == nil || len(body.TimeInZone) != 0 {
		t.Errorf("time_in_zone = %v, want [] (non-nil empty)", body.TimeInZone)
	}
	if body.DecouplingPct != nil {
		t.Errorf("decoupling_pct = %v, want null", body.DecouplingPct)
	}
}

func TestActivityAnalysisNoHR(t *testing.T) {
	fs := &fakeStreams{analysis: streams.StreamAnalysis{
		ActivityID: 14820001234, HasHR: false, TimeInZone: []streams.ZoneTime{},
		Zones: streams.ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170},
		Source: "strava", ComputedAt: "2026-06-22T07:00:00Z",
	}}
	h := newStreamsServer(t, fs)
	rec := do(t, h, http.MethodGet, "/api/activities/14820001234/analysis", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body streamAnalysisDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.HasStream || body.HasHR {
		t.Errorf("dto = %+v, want has_stream true has_hr false", body)
	}
	if len(body.TimeInZone) != 0 {
		t.Errorf("time_in_zone = %+v, want []", body.TimeInZone)
	}
}

func TestActivityAnalysisEngineError(t *testing.T) {
	fs := &fakeStreams{getErr: context.DeadlineExceeded}
	h := newStreamsServer(t, fs)
	rec := do(t, h, http.MethodGet, "/api/activities/123/analysis", testToken)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestFetchStreamRequiresAuth(t *testing.T) {
	h := newStreamsServer(t, &fakeStreams{})
	rec := do(t, h, http.MethodPost, "/api/activities/123/stream/fetch", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestFetchStreamSuccess(t *testing.T) {
	fs := &fakeStreams{analysis: streams.StreamAnalysis{
		ActivityID: 14820001234, HasHR: true,
		TimeInZone: []streams.ZoneTime{{Zone: 2, Seconds: 1800, Pct: 100}},
		DecouplingPct: fpv(3.1),
		Zones:      streams.ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170},
		Source:     "strava", ComputedAt: "2026-06-22T07:00:00Z",
	}}
	h := newStreamsServer(t, fs)
	rec := do(t, h, http.MethodPost, "/api/activities/14820001234/stream/fetch", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fs.lastFetchID != 14820001234 {
		t.Errorf("lastFetchID = %d, want 14820001234", fs.lastFetchID)
	}
	var body streamAnalysisDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.HasStream || body.DecouplingPct == nil || *body.DecouplingPct != 3.1 {
		t.Errorf("dto = %+v, want has_stream true decoupling 3.1", body)
	}
}

func TestFetchStreamRateLimited(t *testing.T) {
	fs := &fakeStreams{fetchErr: &strava.ErrRateLimited{}}
	h := newStreamsServer(t, fs)
	rec := do(t, h, http.MethodPost, "/api/activities/123/stream/fetch", testToken)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"rate_limited"`) {
		t.Errorf("body = %q, want rate_limited", rec.Body.String())
	}
}

func TestFetchStreamOtherError(t *testing.T) {
	fs := &fakeStreams{fetchErr: errors.New("boom")}
	h := newStreamsServer(t, fs)
	rec := do(t, h, http.MethodPost, "/api/activities/123/stream/fetch", testToken)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestFetchStreamInvalidID(t *testing.T) {
	h := newStreamsServer(t, &fakeStreams{})
	rec := do(t, h, http.MethodPost, "/api/activities/xyz/stream/fetch", testToken)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
