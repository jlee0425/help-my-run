package api

import (
	"net/http"
	"strings"

	"help-my-run/backend/internal/auth"
)

const (
	bearerPrefix      = "Bearer "
	sessionCookieName = "hmr_session"
)

// RequireAuth returns middleware accepting EITHER the owner session cookie or
// an Authorization: Bearer <api-token> header. Cookie-authenticated non-GET
// requests additionally pass a same-site check (CSRF guard): a browser that
// labels the request cross-site is rejected. Bearer requests are exempt (no
// ambient credential to ride).
func RequireAuth(a *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, bearerPrefix) {
				if a.ValidateAPIToken(strings.TrimPrefix(h, bearerPrefix)) {
					next.ServeHTTP(w, r)
					return
				}
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			if c, err := r.Cookie(sessionCookieName); err == nil && a.ValidateSession(c.Value) {
				if r.Method != http.MethodGet && r.Method != http.MethodHead &&
					r.Header.Get("Sec-Fetch-Site") == "cross-site" {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request rejected"})
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		})
	}
}

// setSessionCookie writes the session cookie (Secure when the request arrived
// over TLS directly or via a reverse proxy).
func setSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}
