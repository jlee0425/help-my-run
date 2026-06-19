package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestSyncStravaRefreshesAndUpserts(t *testing.T) {
	s := newStore(t)
	// Store an EXPIRED token so the sync must refresh first.
	if err := s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "old-acc", RefreshToken: "old-ref",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(), Scope: "activity:read_all", AthleteID: 1,
	}); err != nil {
		t.Fatalf("seed tokens: %v", err)
	}

	var refreshed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			refreshed = true
			_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"fresh","refresh_token":"fresh-ref","expires_at":4102444800,"expires_in":21600,"scope":"activity:read_all","athlete":{"id":1}}`))
		case r.URL.Path == "/api/v3/athlete/activities":
			if r.URL.Query().Get("page") == "1" {
				_, _ = w.Write([]byte(`[{"id":900,"name":"Run","type":"Run","sport_type":"Run","start_date":"2026-06-18T06:00:00Z","start_date_local":"2026-06-18T08:00:00Z","distance":10000,"moving_time":3000,"elapsed_time":3050,"average_heartrate":150,"max_heartrate":170,"average_speed":3.3,"max_speed":4.5,"average_cadence":85,"total_elevation_gain":50}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		case r.URL.Path == "/api/v3/activities/900/laps":
			_, _ = w.Write([]byte(`[{"lap_index":1,"distance":5000,"elapsed_time":1500,"moving_time":1490,"average_heartrate":148,"max_heartrate":160,"average_speed":3.3}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := strava.NewWithBase("123", "secret", "http://cb", srv.URL)
	res := SyncStrava(context.Background(), s, client)

	if res.Status != "ok" || res.Error != nil {
		t.Fatalf("result = %+v, want ok/no-error", res)
	}
	if res.Synced != 1 {
		t.Errorf("synced = %d, want 1", res.Synced)
	}
	if !refreshed {
		t.Error("expected token refresh on expired token")
	}
	// Fresh token persisted.
	tok, _ := s.GetStravaTokens()
	if tok.AccessToken != "fresh" || tok.RefreshToken != "fresh-ref" {
		t.Errorf("tokens = %+v, want fresh/fresh-ref", tok)
	}
	// Activity + lap upserted.
	acts, _ := s.ListActivities(30)
	if len(acts) != 1 || acts[0].StravaID != 900 {
		t.Fatalf("activities = %+v, want one id=900", acts)
	}
	var nLaps int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activity_splits WHERE activity_id=900`).Scan(&nLaps)
	if nLaps != 1 {
		t.Errorf("laps = %d, want 1", nLaps)
	}
	// sync_log updated.
	sl, _ := s.GetSyncLog("strava")
	if sl.Status != "ok" || sl.LastSyncedAt == nil {
		t.Errorf("sync_log = %+v, want ok with last_synced_at", sl)
	}
}

func TestSyncStravaNotConnected(t *testing.T) {
	s := newStore(t)
	client := strava.NewWithBase("123", "secret", "http://cb", "http://unused")
	res := SyncStrava(context.Background(), s, client)
	if res.Status != "error" || res.Error == nil {
		t.Fatalf("result = %+v, want error when not connected", res)
	}
	sl, _ := s.GetSyncLog("strava")
	if sl.Status != "error" {
		t.Errorf("sync_log status = %q, want error", sl.Status)
	}
}
