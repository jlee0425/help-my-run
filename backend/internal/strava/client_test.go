package strava

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
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
