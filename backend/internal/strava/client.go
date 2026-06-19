package strava

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
