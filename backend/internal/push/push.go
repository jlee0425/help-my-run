// Package push sends notifications via the Expo Push HTTP API (v2). The base URL
// is injectable so tests drive it against httptest with no real network.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const (
	defaultExpoBaseURL = "https://exp.host"
	sendPath           = "/--/api/v2/push/send"
)

// Message is one Expo Push API message object.
type Message struct {
	To        string                 `json:"to"` // "ExponentPushToken[...]"
	Title     string                 `json:"title,omitempty"`
	Body      string                 `json:"body,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Sound     string                 `json:"sound,omitempty"`     // "default"
	Priority  string                 `json:"priority,omitempty"`  // "high"
	ChannelID string                 `json:"channelId,omitempty"` // "default"
}

// Sender is the injectable push transport (faked in agent tests).
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Client is the production Sender (HTTP to the Expo Push API).
type Client struct {
	BaseURL    string // injectable: prod "https://exp.host", test = httptest URL
	HTTPClient *http.Client
}

// NewClient builds a Client. An empty baseURL falls back to the Expo prod host.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultExpoBaseURL
	}
	return &Client{BaseURL: baseURL, HTTPClient: http.DefaultClient}
}

// ErrDeviceNotRegistered signals the caller to delete the token from device_tokens.
var ErrDeviceNotRegistered = errors.New("push: device not registered")

type ticketDetails struct {
	Error string `json:"error"` // e.g. "DeviceNotRegistered"
}
type ticket struct {
	Status  string        `json:"status"` // "ok"|"error"
	ID      string        `json:"id"`
	Message string        `json:"message"`
	Details ticketDetails `json:"details"`
}
type sendResponse struct {
	Data   []ticket `json:"data"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// Send POSTs one message to the Expo Push API and inspects the ticket. A
// DeviceNotRegistered ticket returns ErrDeviceNotRegistered; any other error
// ticket / request-level error returns a descriptive error.
func (c *Client) Send(ctx context.Context, msg Message) error {
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+sendPath, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out sendResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("push: decode response: %w", err)
	}
	if len(out.Data) == 0 {
		return fmt.Errorf("push: empty data (errors=%v)", out.Errors)
	}
	t := out.Data[0]
	if t.Status == "error" {
		if t.Details.Error == "DeviceNotRegistered" {
			return ErrDeviceNotRegistered
		}
		return fmt.Errorf("push: expo error: %s (%s)", t.Message, t.Details.Error)
	}
	return nil
}
