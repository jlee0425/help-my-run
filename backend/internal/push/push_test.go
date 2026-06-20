package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"help-my-run/backend/internal/push"
)

func TestSendOK(t *testing.T) {
	var gotBody push.Message
	var gotPath, gotCT, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"status":"ok","id":"abc-123"}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Send(context.Background(), push.Message{
		To: "ExponentPushToken[x]", Title: "Today: AMBER", Body: "Trimmed tempo.",
		Data:  map[string]interface{}{"date": "2026-06-20", "action": "SOFTEN"},
		Sound: "default", Priority: "high", ChannelID: "default",
	})
	if err != nil {
		t.Fatalf("Send error = %v", err)
	}
	if gotPath != "/--/api/v2/push/send" {
		t.Errorf("path = %q, want /--/api/v2/push/send", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if gotBody.To != "ExponentPushToken[x]" || gotBody.Title != "Today: AMBER" || gotBody.Body != "Trimmed tempo." {
		t.Errorf("decoded body = %+v", gotBody)
	}
	if gotBody.Data["action"] != "SOFTEN" {
		t.Errorf("Data = %+v, want action=SOFTEN", gotBody.Data)
	}
}

func TestSendDeviceNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"status":"error","message":"not registered","details":{"error":"DeviceNotRegistered"}}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Send(context.Background(), push.Message{To: "ExponentPushToken[dead]"})
	if !errors.Is(err, push.ErrDeviceNotRegistered) {
		t.Fatalf("Send err = %v, want ErrDeviceNotRegistered", err)
	}
}

func TestSendOtherError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"status":"error","message":"too big","details":{"error":"MessageTooBig"}}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Send(context.Background(), push.Message{To: "ExponentPushToken[big]"})
	if err == nil {
		t.Fatal("Send err = nil, want non-nil for MessageTooBig")
	}
	if errors.Is(err, push.ErrDeviceNotRegistered) {
		t.Errorf("MessageTooBig must not be ErrDeviceNotRegistered, got %v", err)
	}
}

func TestSendEmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"errors":[{"code":"PUSH_TOO_MANY","message":"bad"}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if err := c.Send(context.Background(), push.Message{To: "ExponentPushToken[x]"}); err == nil {
		t.Fatal("Send err = nil, want non-nil for empty data")
	}
}

func TestNewClientDefaultsBaseURL(t *testing.T) {
	c := push.NewClient("")
	if c.BaseURL != "https://exp.host" {
		t.Errorf("BaseURL = %q, want https://exp.host", c.BaseURL)
	}
	if c.HTTPClient == nil {
		t.Error("HTTPClient = nil, want non-nil")
	}
	c2 := push.NewClient("http://localhost:9")
	if c2.BaseURL != "http://localhost:9" {
		t.Errorf("BaseURL = %q, want override preserved", c2.BaseURL)
	}
}

var _ push.Sender = (*push.Client)(nil)
