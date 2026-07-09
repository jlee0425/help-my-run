package webpush

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"help-my-run/backend/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "wp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestVAPIDPersistsAcrossBoots(t *testing.T) {
	s := newStore(t)
	w1, err := New(s)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if w1.PublicKey() == "" {
		t.Fatal("empty public key")
	}
	w2, err := New(s)
	if err != nil {
		t.Fatalf("New again: %v", err)
	}
	if w2.PublicKey() != w1.PublicKey() {
		t.Fatalf("keypair regenerated across boots")
	}
}

// subscribeTo registers a fake browser subscription pointing at srv.
func subscribeTo(t *testing.T, s *store.Store, srv *httptest.Server, path string) {
	t.Helper()
	// Real-looking (but throwaway) P-256 client keys, base64url. webpush-go
	// encrypts against these; the fake push service just accepts the POST.
	if err := s.UpsertPushSubscription(store.PushSubscription{
		Endpoint: srv.URL + path,
		P256dh:   "BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM",
		Auth:     "tBHItJI5svbpez7KI4CCXg",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBroadcastDeliversAndPrunes(t *testing.T) {
	s := newStore(t)
	var okHits, goneHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/ok"):
			okHits.Add(1)
			w.WriteHeader(http.StatusCreated)
		case strings.HasPrefix(r.URL.Path, "/gone"):
			goneHits.Add(1)
			w.WriteHeader(http.StatusGone)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	subscribeTo(t, s, srv, "/ok/1")
	subscribeTo(t, s, srv, "/gone/2")

	w, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	w.HTTPClient = srv.Client()

	if err := w.Broadcast(context.Background(), "Today: SOFTEN — amber", "Trimmed to easy.", "/"); err != nil {
		t.Fatalf("Broadcast: %v (ok=%d gone=%d)", err, okHits.Load(), goneHits.Load())
	}
	if okHits.Load() != 1 || goneHits.Load() != 1 {
		t.Fatalf("hits ok=%d gone=%d, want 1/1", okHits.Load(), goneHits.Load())
	}
	// The 410 subscription is pruned; the healthy one remains.
	subs, _ := s.ListPushSubscriptions()
	if len(subs) != 1 || !strings.Contains(subs[0].Endpoint, "/ok/") {
		t.Fatalf("subs after prune = %+v", subs)
	}
}

func TestBroadcastNoSubscriptionsErrors(t *testing.T) {
	s := newStore(t)
	w, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Broadcast(context.Background(), "t", "b", "/"); err == nil {
		t.Fatal("expected error with zero subscriptions")
	}
}
