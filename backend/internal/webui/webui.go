// Package webui embeds the built SPA (web/dist, copied here by `make
// build-web`) and serves it with SPA-fallback semantics: real files are served
// as-is, every other non-/api path gets index.html so client-side routing
// works on hard reloads.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded SPA build.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// embed of a committed directory cannot fail at runtime; guard anyway.
		panic("webui: " + err.Error())
	}
	return handlerFor(sub)
}

// handlerFor is the fs-injectable core (tests use fstest.MapFS).
func handlerFor(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")

		// Unknown /api/* paths stay JSON (the API router delegates un-routed
		// paths here; the UI must never shadow the API namespace).
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}

		// Real file? Serve it (hashed assets get immutable caching).
		if p != "" && fileExists(fsys, p) {
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for every route path.
		if !fileExists(fsys, "index.html") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("UI not built — run `make build` (or `make build-web`) first.\n"))
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// fileExists reports whether path is a regular file in fsys.
func fileExists(fsys fs.FS, path string) bool {
	info, err := fs.Stat(fsys, path)
	return err == nil && !info.IsDir()
}
