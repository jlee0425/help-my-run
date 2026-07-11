package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"help-my-run/backend/internal/auth"
)

const sessionCookieMaxAge = 30 * 24 * 60 * 60 // 30 days, matches auth.sessionTTL

// sessionMeta captures device info for the sessions/devices list (M6).
// chi's RealIP middleware has already rewritten RemoteAddr when behind a proxy.
func sessionMeta(r *http.Request) auth.SessionMeta {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return auth.SessionMeta{UserAgent: r.UserAgent(), IP: ip}
}

// sessionDTO is one device row. IDHash is the SHA-256 of the cookie secret —
// exposing it grants nothing; it is the revocation key.
type sessionDTO struct {
	IDHash     string `json:"id_hash"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	UserAgent  string `json:"user_agent"`
	IP         string `json:"ip"`
	Current    bool   `json:"current"`
}

// currentSessionHash returns the SHA-256 of the caller's session cookie, or ""
// (bearer-token callers have no session).
func currentSessionHash(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return auth.HashSecret(c.Value)
}

// GET /api/auth/sessions (protected) — the devices list (expired rows purged).
func (h *handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.d.Auth.Sessions()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	current := currentSessionHash(r)
	out := make([]sessionDTO, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionDTO{
			IDHash:     s.IDHash,
			CreatedAt:  s.CreatedAt,
			LastSeenAt: s.LastSeenAt,
			UserAgent:  s.UserAgent,
			IP:         s.CreatedIP,
			Current:    current != "" && s.IDHash == current,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// DELETE /api/auth/sessions/{idHash} (protected) — revoke one device.
// Revoking the current session is allowed (acts as logout).
func (h *handlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	idHash := chi.URLParam(r, "idHash")
	if len(idHash) != 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad session id"})
		return
	}
	if err := h.d.Store.DeleteSession(idHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if currentSessionHash(r) == idHash {
		setSessionCookie(w, r, "", -1)
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/auth/sessions/revoke-others (protected, cookie callers only).
func (h *handlers) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "requires a session cookie"})
		return
	}
	switch err := h.d.Auth.RevokeOtherSessions(c.Value); {
	case errors.Is(err, auth.ErrUnknownSession):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "requires a live session cookie"})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/auth/state (public)
func (h *handlers) authState(w http.ResponseWriter, r *http.Request) {
	if h.d.Demo {
		writeJSON(w, http.StatusOK, authStateDTO{SetupRequired: false, Authed: true, Demo: true})
		return
	}
	setup, err := h.d.Auth.SetupRequired()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	authed := false
	if !setup {
		if c, cerr := r.Cookie(sessionCookieName); cerr == nil {
			authed = h.d.Auth.ValidateSession(c.Value)
		}
	}
	writeJSON(w, http.StatusOK, authStateDTO{SetupRequired: setup, Authed: authed})
}

// POST /api/setup (public, first run only)
func (h *handlers) setup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	sid, token, err := h.d.Auth.Setup(in.Password, sessionMeta(r))
	switch {
	case errors.Is(err, auth.ErrAlreadySetup):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already set up"})
		return
	case errors.Is(err, auth.ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	setSessionCookie(w, r, sid, sessionCookieMaxAge)
	writeJSON(w, http.StatusOK, map[string]string{"api_token": token})
}

// POST /api/login (public)
func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	sid, err := h.d.Auth.Login(in.Password, sessionMeta(r))
	switch {
	case errors.Is(err, auth.ErrThrottled):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "throttled"})
		return
	case errors.Is(err, auth.ErrBadCredentials), errors.Is(err, auth.ErrNotSetup):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	setSessionCookie(w, r, sid, sessionCookieMaxAge)
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/logout (protected)
func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = h.d.Auth.Logout(c.Value)
	}
	setSessionCookie(w, r, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/auth/password (protected)
func (h *handlers) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	err := h.d.Auth.ChangePassword(in.Current, in.New)
	switch {
	case errors.Is(err, auth.ErrBadCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is wrong"})
		return
	case errors.Is(err, auth.ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/auth/token (protected) — regenerate the API token.
func (h *handlers) regenerateToken(w http.ResponseWriter, r *http.Request) {
	token, err := h.d.Auth.RegenerateAPIToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"api_token": token})
}
