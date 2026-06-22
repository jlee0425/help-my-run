package strava

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// defaultBaseURL is the real Strava host. Tests override it via NewWithBase.
const defaultBaseURL = "https://www.strava.com"

// Client talks to the Strava API. baseURL is injectable so tests can point it
// at an httptest server.
type Client struct {
	clientID     string
	clientSecret string
	redirectURL  string
	baseURL      string
	http         *http.Client
}

// New builds a Client against the real Strava base URL.
func New(clientID, clientSecret, redirectURL string) *Client {
	return NewWithBase(clientID, clientSecret, redirectURL, defaultBaseURL)
}

// NewWithBase builds a Client against an explicit base URL (for tests).
func NewWithBase(clientID, clientSecret, redirectURL, baseURL string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		baseURL:      baseURL,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

// AuthorizeURL builds the Strava OAuth authorize URL with the given CSRF state.
func (c *Client) AuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "activity:read_all")
	q.Set("approval_prompt", "auto")
	q.Set("state", state)
	return c.baseURL + "/oauth/authorize?" + q.Encode()
}

// tokenURL is the Strava token endpoint (NOT /api/v3/oauth/token).
func (c *Client) tokenURL() string { return c.baseURL + "/oauth/token" }

// Exchange swaps an authorization code for tokens (grant_type=authorization_code).
func (c *Client) Exchange(ctx context.Context, code string) (*TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	})
}

// Refresh exchanges a refresh token for a new access token
// (grant_type=refresh_token). Always persist the returned refresh_token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

func (c *Client) postToken(ctx context.Context, form url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("strava token endpoint: status %d: %s", resp.StatusCode, string(body))
	}

	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("strava token parse: %w", err)
	}
	return &tok, nil
}

const perPage = 200

// streamKeys is the fixed CSV requested for M3.2 streams.
const streamKeys = "time,heartrate,velocity_smooth,distance"

// ListActivities returns all activities after the given unix-second timestamp,
// paginating until Strava returns an empty page.
func (c *Client) ListActivities(ctx context.Context, accessToken string, after int64) ([]SummaryActivity, error) {
	var all []SummaryActivity
	for page := 1; ; page++ {
		q := url.Values{}
		if after > 0 {
			q.Set("after", strconv.FormatInt(after, 10))
		}
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", strconv.Itoa(perPage))

		var batch []SummaryActivity
		if err := c.getJSON(ctx, accessToken,
			"/api/v3/athlete/activities?"+q.Encode(), &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
	}
	return all, nil
}

// ListLaps returns the laps for an activity.
func (c *Client) ListLaps(ctx context.Context, accessToken string, activityID int64) ([]Lap, error) {
	var laps []Lap
	path := "/api/v3/activities/" + strconv.FormatInt(activityID, 10) + "/laps"
	if err := c.getJSON(ctx, accessToken, path, &laps); err != nil {
		return nil, err
	}
	return laps, nil
}

// ErrRateLimited is returned (via getJSON) on HTTP 429. RetryAfter is the
// time the 15-min window resets (next quarter-hour boundary :00/:15/:30/:45 UTC).
type ErrRateLimited struct {
	RetryAfter time.Time // when to resume
	ReadUsage  string    // raw X-ReadRateLimit-Usage header ("15min,daily")
	ReadLimit  string    // raw X-ReadRateLimit-Limit header
}

func (e *ErrRateLimited) Error() string {
	return fmt.Sprintf("strava rate limited (read usage %s / limit %s); retry after %s",
		e.ReadUsage, e.ReadLimit, e.RetryAfter.Format(time.RFC3339))
}

// nextQuarterHour returns the next :00/:15/:30/:45 boundary at/after t (UTC).
func nextQuarterHour(t time.Time) time.Time {
	t = t.UTC().Truncate(time.Minute)
	add := 15 - (t.Minute() % 15)
	return t.Add(time.Duration(add) * time.Minute)
}

// GetActivityStreams fetches the per-sample streams for an activity, keyed by type.
// Path: /api/v3/activities/{id}/streams?keys=...&key_by_type=true
// It REUSES the existing getJSON helper (request building + auth header + body
// read); getJSON now returns *ErrRateLimited on HTTP 429 (the trickle catches it).
// accessToken is supplied by the caller (the Engine refreshes it — FIX 4 / Task 11);
// keeping the token param preserves the httptest seam the tests inject.
func (c *Client) GetActivityStreams(ctx context.Context, accessToken string, activityID int64) (StreamSet, error) {
	q := url.Values{}
	q.Set("keys", streamKeys)
	q.Set("key_by_type", "true")
	path := "/api/v3/activities/" + strconv.FormatInt(activityID, 10) + "/streams?" + q.Encode()

	var ss StreamSet
	if err := c.getJSON(ctx, accessToken, path, &ss); err != nil {
		return nil, err
	}
	return ss, nil
}

// getJSON performs an authenticated GET and unmarshals the JSON body into dst.
func (c *Client) getJSON(ctx context.Context, accessToken, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return &ErrRateLimited{
			RetryAfter: nextQuarterHour(time.Now()),
			ReadUsage:  resp.Header.Get("X-ReadRateLimit-Usage"),
			ReadLimit:  resp.Header.Get("X-ReadRateLimit-Limit"),
		}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("strava GET %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, dst)
}
