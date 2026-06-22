package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// newProgressServer wires a server whose Progress seam is the given fake.
func newProgressServer(t *testing.T, fp *fakeProgress) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "prog-api.db"))
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
		Progress: fp,
		Streams:  &fakeStreams{},
		Chat:     &fakeChat{},
	}
	return NewRouter(deps)
}

func TestProgressRequiresAuth(t *testing.T) {
	h := newProgressServer(t, &fakeProgress{})
	rec := do(t, h, http.MethodGet, "/api/progress", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
		t.Errorf("body = %q, want unauthorized", rec.Body.String())
	}
}

func TestProgressHandlerServesReport(t *testing.T) {
	cur := 330.0
	base := 350.0
	delta := -20.0
	fp := &fakeProgress{report: progress.ProgressReport{
		Weeks:       12,
		GeneratedAt: "2026-06-21T07:00:00Z",
		EnoughData:  true,
		Signals: []progress.TrendSummary{{
			Key: "pace_at_hr", Label: "Pace @ Z2 HR", Unit: "s/km",
			Current: &cur, Baseline: &base, DeltaAbs: &delta,
			Direction: progress.DirectionDown, LowerIsBetter: true,
			Series: []*float64{&base, nil, &cur},
		}},
	}}
	h := newProgressServer(t, fp)

	rec := do(t, h, http.MethodGet, "/api/progress?weeks=12", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body progress.ProgressReport
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Weeks != 12 || !body.EnoughData || len(body.Signals) != 1 {
		t.Errorf("report = %+v", body)
	}
	if body.Signals[0].Key != "pace_at_hr" || body.Signals[0].Direction != progress.DirectionDown {
		t.Errorf("signal = %+v", body.Signals[0])
	}
	// snake_case wire tags present.
	if !strings.Contains(rec.Body.String(), `"delta_abs"`) || !strings.Contains(rec.Body.String(), `"lower_is_better"`) {
		t.Errorf("wire JSON not snake_case: %s", rec.Body.String())
	}
	if fp.lastWeeks != 12 {
		t.Errorf("weeks passed = %d, want 12", fp.lastWeeks)
	}
}

func TestProgressHandlerClampsWeeks(t *testing.T) {
	fp := &fakeProgress{}
	h := newProgressServer(t, fp)
	// weeks=999 clamps to MaxWeeks (52).
	do(t, h, http.MethodGet, "/api/progress?weeks=999", testToken)
	if fp.lastWeeks != progress.MaxWeeks {
		t.Errorf("weeks = %d, want %d (clamped)", fp.lastWeeks, progress.MaxWeeks)
	}
	// weeks=1 clamps to MinWeeks (4).
	do(t, h, http.MethodGet, "/api/progress?weeks=1", testToken)
	if fp.lastWeeks != progress.MinWeeks {
		t.Errorf("weeks = %d, want %d (clamped)", fp.lastWeeks, progress.MinWeeks)
	}
	// no param -> default 12.
	do(t, h, http.MethodGet, "/api/progress", testToken)
	if fp.lastWeeks != progress.DefaultWeeks {
		t.Errorf("weeks = %d, want %d (default)", fp.lastWeeks, progress.DefaultWeeks)
	}
}

func TestProgressHandlerReportError(t *testing.T) {
	fp := &fakeProgress{reportErr: context.DeadlineExceeded}
	h := newProgressServer(t, fp)
	rec := do(t, h, http.MethodGet, "/api/progress", testToken)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 on report error", rec.Code)
	}
}

func TestAnalyzeProgressRequiresAuth(t *testing.T) {
	h := newProgressServer(t, &fakeProgress{})
	rec := do(t, h, http.MethodPost, "/api/progress/analyze", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAnalyzeProgressHandlerAI(t *testing.T) {
	fp := &fakeProgress{read: progress.ProgressRead{Text: "Engine improving.", Source: "ai"}}
	h := newProgressServer(t, fp)
	rec := doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, `{"weeks":12}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body progress.ProgressRead
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Text != "Engine improving." || body.Source != "ai" {
		t.Errorf("read = %+v, want ai/Engine improving.", body)
	}
	if fp.lastWeeks != 12 {
		t.Errorf("weeks = %d, want 12", fp.lastWeeks)
	}
}

func TestAnalyzeProgressEmptyBodyDefaults(t *testing.T) {
	fp := &fakeProgress{read: progress.ProgressRead{Text: "x", Source: "fallback"}}
	h := newProgressServer(t, fp)
	// Empty body -> weeks defaults to 12.
	rec := doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, ``)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fp.lastWeeks != 12 {
		t.Errorf("weeks = %d, want 12 (default on empty body)", fp.lastWeeks)
	}
}

func TestAnalyzeProgressOutOfRangeDefaults(t *testing.T) {
	fp := &fakeProgress{}
	h := newProgressServer(t, fp)
	// weeks=2 (< MinWeeks) -> defaults to 12.
	doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, `{"weeks":2}`)
	if fp.lastWeeks != 12 {
		t.Errorf("weeks = %d, want 12 (out-of-range -> default)", fp.lastWeeks)
	}
}

func TestAnalyzeProgressError(t *testing.T) {
	fp := &fakeProgress{analyzeErr: context.DeadlineExceeded}
	h := newProgressServer(t, fp)
	rec := doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, `{"weeks":12}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 on analyze error", rec.Code)
	}
}
