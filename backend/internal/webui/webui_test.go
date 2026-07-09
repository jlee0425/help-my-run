package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func builtFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"manifest.webmanifest": {Data: []byte(`{"name":"Running on AI"}`)},
		"sw.js":                {Data: []byte("// sw")},
		"assets/app-abc123.js": {Data: []byte("console.log('x')")},
	}
}

func get(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	b, _ := io.ReadAll(rec.Result().Body)
	return rec, string(b)
}

func TestServesFilesAndSPAFallback(t *testing.T) {
	h := handlerFor(builtFS())

	rec, body := get(t, h, "/")
	if rec.Code != 200 || !strings.Contains(body, "app") {
		t.Fatalf("/ = %d %q", rec.Code, body)
	}
	rec, body = get(t, h, "/assets/app-abc123.js")
	if rec.Code != 200 || !strings.Contains(body, "console") {
		t.Fatalf("asset = %d %q", rec.Code, body)
	}
	// Client-side route falls back to index.html.
	rec, body = get(t, h, "/settings")
	if rec.Code != 200 || !strings.Contains(body, "app") {
		t.Fatalf("SPA fallback = %d %q", rec.Code, body)
	}
	rec, body = get(t, h, "/runs/123")
	if rec.Code != 200 || !strings.Contains(body, "app") {
		t.Fatalf("nested SPA fallback = %d %q", rec.Code, body)
	}
}

func TestCacheHeaders(t *testing.T) {
	h := handlerFor(builtFS())

	rec, _ := get(t, h, "/assets/app-abc123.js")
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("asset Cache-Control = %q, want immutable", cc)
	}
	for _, p := range []string{"/", "/sw.js", "/manifest.webmanifest"} {
		rec, _ := get(t, h, p)
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s Cache-Control = %q, want no-cache", p, cc)
		}
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	h := handlerFor(builtFS())
	rec, body := get(t, h, "/api/nope")
	if rec.Code != http.StatusNotFound || !strings.Contains(body, `"error"`) {
		t.Fatalf("/api/nope = %d %q, want JSON 404", rec.Code, body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
}

func TestUnbuiltUIGives503(t *testing.T) {
	h := handlerFor(fstest.MapFS{}) // no index.html
	rec, body := get(t, h, "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(body, "make build") {
		t.Fatalf("unbuilt = %d %q, want 503 hint", rec.Code, body)
	}
}
