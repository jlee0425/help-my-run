package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// BearerAuth returns middleware that requires Authorization: Bearer <token>.
// Comparison is constant-time. Failures return 401 with {"error":"unauthorized"}.
func BearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if token == "" || !strings.HasPrefix(h, bearerPrefix) ||
				subtle.ConstantTimeCompare(
					[]byte(strings.TrimPrefix(h, bearerPrefix)), []byte(token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
