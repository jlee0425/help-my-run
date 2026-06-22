package strava

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthorizeURL(t *testing.T) {
	c := New("12345", "secret", "http://localhost:8080/api/strava/callback")
	got := c.AuthorizeURL("abc123")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("AuthorizeURL parse error = %v", err)
	}
	if u.Scheme != "https" || u.Host != "www.strava.com" || u.Path != "/oauth/authorize" {
		t.Errorf("base = %s://%s%s, want https://www.strava.com/oauth/authorize", u.Scheme, u.Host, u.Path)
	}
	q := u.Query()
	checks := map[string]string{
		"client_id":       "12345",
		"redirect_uri":    "http://localhost:8080/api/strava/callback",
		"response_type":   "code",
		"scope":           "activity:read_all",
		"approval_prompt": "auto",
		"state":           "abc123",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query[%q] = %q, want %q", k, got, want)
		}
	}
}

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewWithBase("12345", "secret", "http://localhost:8080/api/strava/callback", srv.URL)
	return c, srv
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func TestExchange(t *testing.T) {
	var gotGrant, gotCode string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			t.Errorf("got %s %s, want POST /oauth/token", r.Method, r.URL.Path)
		}
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotCode = r.Form.Get("code")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "strava_token.json"))
	})

	tok, err := c.Exchange(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("Exchange error = %v", err)
	}
	if gotGrant != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", gotGrant)
	}
	if gotCode != "the-code" {
		t.Errorf("code = %q, want the-code", gotCode)
	}
	if tok.AccessToken != "new-access" || tok.RefreshToken != "new-refresh" || tok.ExpiresAt != 1737000000 {
		t.Errorf("token = %+v, want access=new-access refresh=new-refresh exp=1737000000", tok)
	}
	if tok.Athlete == nil || tok.Athlete.ID != 12345678 {
		t.Errorf("athlete = %+v, want id 12345678", tok.Athlete)
	}
}

func TestRefresh(t *testing.T) {
	var gotGrant, gotRefresh string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "strava_token.json"))
	})

	tok, err := c.Refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("Refresh error = %v", err)
	}
	if gotGrant != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", gotGrant)
	}
	if gotRefresh != "old-refresh" {
		t.Errorf("refresh_token sent = %q, want old-refresh", gotRefresh)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want new-access", tok.AccessToken)
	}
}

func TestExchangeNon200(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Bad Request"}`))
	})
	if _, err := c.Exchange(context.Background(), "x"); err == nil {
		t.Fatal("Exchange on 400 error = nil, want error")
	}
}

func TestListActivitiesPaginates(t *testing.T) {
	var sawAuth string
	var afterParam string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/athlete/activities" {
			t.Errorf("path = %s, want /api/v3/athlete/activities", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization")
		afterParam = r.URL.Query().Get("after")
		w.Header().Set("Content-Type", "application/json")
		// Page 1 -> fixture (2 activities); page 2+ -> empty array (stop).
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write(loadFixture(t, "strava_activities.json"))
		} else {
			_, _ = w.Write([]byte(`[]`))
		}
	})

	acts, err := c.ListActivities(context.Background(), "access-tok", 1718600000)
	if err != nil {
		t.Fatalf("ListActivities error = %v", err)
	}
	if sawAuth != "Bearer access-tok" {
		t.Errorf("Authorization = %q, want %q", sawAuth, "Bearer access-tok")
	}
	if afterParam != "1718600000" {
		t.Errorf("after = %q, want 1718600000", afterParam)
	}
	if len(acts) != 2 {
		t.Fatalf("activities len = %d, want 2", len(acts))
	}
	if acts[0].ID != 14820001234 || acts[0].SportType != "Run" {
		t.Errorf("act0 = id %d sport %q, want 14820001234 Run", acts[0].ID, acts[0].SportType)
	}
	if acts[0].AverageHeartrate == nil || *acts[0].AverageHeartrate != 152.3 {
		t.Errorf("act0.AverageHeartrate = %v, want 152.3", acts[0].AverageHeartrate)
	}
	// Second run has no HR -> pointer nil.
	if acts[1].AverageHeartrate != nil {
		t.Errorf("act1.AverageHeartrate = %v, want nil", acts[1].AverageHeartrate)
	}
}

func TestListLaps(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/activities/14820001234/laps" {
			t.Errorf("path = %s, want /api/v3/activities/14820001234/laps", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access-tok" {
			t.Errorf("Authorization = %q, want Bearer access-tok", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "strava_laps.json"))
	})

	laps, err := c.ListLaps(context.Background(), "access-tok", 14820001234)
	if err != nil {
		t.Fatalf("ListLaps error = %v", err)
	}
	if len(laps) != 2 {
		t.Fatalf("laps len = %d, want 2", len(laps))
	}
	if laps[0].LapIndex != 1 || laps[0].Distance != 1000.0 {
		t.Errorf("lap0 = idx %d dist %v, want 1 1000", laps[0].LapIndex, laps[0].Distance)
	}
	if laps[1].AverageHeartrate != nil {
		t.Errorf("lap1.AverageHeartrate = %v, want nil", laps[1].AverageHeartrate)
	}
}

func TestGetActivityStreams(t *testing.T) {
	var gotPath, gotKeys, gotKeyByType, sawAuth string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKeys = r.URL.Query().Get("keys")
		gotKeyByType = r.URL.Query().Get("key_by_type")
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "strava_streams.json"))
	})

	ss, err := c.GetActivityStreams(context.Background(), "access-tok", 14820001234)
	if err != nil {
		t.Fatalf("GetActivityStreams error = %v", err)
	}
	if gotPath != "/api/v3/activities/14820001234/streams" {
		t.Errorf("path = %s, want /api/v3/activities/14820001234/streams", gotPath)
	}
	if gotKeys != "time,heartrate,velocity_smooth,distance" {
		t.Errorf("keys = %q, want time,heartrate,velocity_smooth,distance", gotKeys)
	}
	if gotKeyByType != "true" {
		t.Errorf("key_by_type = %q, want true", gotKeyByType)
	}
	if sawAuth != "Bearer access-tok" {
		t.Errorf("Authorization = %q, want Bearer access-tok", sawAuth)
	}
	hr, ok := ss["heartrate"]
	if !ok {
		t.Fatal("heartrate stream missing")
	}
	if len(hr.Data) != 4 || hr.Data[0] != 104 {
		t.Errorf("heartrate data = %v, want [104 105 106 107]", hr.Data)
	}
	if ss["velocity_smooth"].Data[1] != 1.59 {
		t.Errorf("velocity[1] = %v, want 1.59", ss["velocity_smooth"].Data[1])
	}
}

func TestGetActivityStreamsNoHR(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// HR key omitted entirely (no-sensor run).
		_, _ = w.Write([]byte(`{"time":{"type":"time","data":[0,1],"series_type":"time","original_size":2,"resolution":"high"},"distance":{"type":"distance","data":[0,3],"series_type":"time","original_size":2,"resolution":"high"},"velocity_smooth":{"type":"velocity_smooth","data":[0,3],"series_type":"time","original_size":2,"resolution":"high"}}`))
	})
	ss, err := c.GetActivityStreams(context.Background(), "tok", 99)
	if err != nil {
		t.Fatalf("GetActivityStreams error = %v", err)
	}
	if _, ok := ss["heartrate"]; ok {
		t.Error("heartrate key present, want absent for no-HR run")
	}
}

func TestGetActivityStreams429(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-ReadRateLimit-Limit", "100,1000")
		w.Header().Set("X-ReadRateLimit-Usage", "100,512")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"Rate Limit Exceeded"}`))
	})
	_, err := c.GetActivityStreams(context.Background(), "tok", 7)
	if err == nil {
		t.Fatal("GetActivityStreams on 429 error = nil, want *ErrRateLimited")
	}
	var rl *ErrRateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("error = %T, want *ErrRateLimited", err)
	}
	if rl.ReadUsage != "100,512" || rl.ReadLimit != "100,1000" {
		t.Errorf("usage/limit = %q/%q, want 100,512 / 100,1000", rl.ReadUsage, rl.ReadLimit)
	}
	// RetryAfter is the next quarter-hour boundary, strictly in the future.
	if !rl.RetryAfter.After(time.Now()) {
		t.Errorf("RetryAfter = %v, want a future quarter-hour boundary", rl.RetryAfter)
	}
	if m := rl.RetryAfter.Minute() % 15; m != 0 {
		t.Errorf("RetryAfter minute = %d, want a multiple of 15", rl.RetryAfter.Minute())
	}
}
