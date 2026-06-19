package strava

import (
	"net/http"
	"net/url"
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
