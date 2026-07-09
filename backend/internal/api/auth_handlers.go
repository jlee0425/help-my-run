package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"help-my-run/backend/internal/auth"
)

const sessionCookieMaxAge = 30 * 24 * 60 * 60 // 30 days, matches auth.sessionTTL

// GET /api/auth/state (public)
func (h *handlers) authState(w http.ResponseWriter, r *http.Request) {
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
	sid, token, err := h.d.Auth.Setup(in.Password)
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
	sid, err := h.d.Auth.Login(in.Password)
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
