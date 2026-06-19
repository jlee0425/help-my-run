package strava

import (
	"net/url"
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
