package api

import (
	"encoding/json"
	"net/http"
	"os"

	"help-my-run/backend/internal/garmin"
	syncpkg "help-my-run/backend/internal/sync"
)

// GET /api/garmin/status
func (h *handlers) garminStatus(w http.ResponseWriter, r *http.Request) {
	connected := syncpkg.TokenStoreReady(h.d.GarminTokenstore)
	var lastSynced *string
	if log, err := h.d.Store.GetSyncLog("garmin"); err == nil {
		lastSynced = log.LastSyncedAt
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connected":      connected,
		"last_synced_at": lastSynced,
	})
}

// POST /api/garmin/login {email,password}
func (h *handlers) garminLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Email == "" || in.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password required"})
		return
	}
	res := h.d.GarminLogin.Start(r.Context(), in.Email, in.Password)
	writeLoginResult(w, res)
}

// POST /api/garmin/login/mfa {login_id,code}
func (h *handlers) garminLoginMFA(w http.ResponseWriter, r *http.Request) {
	var in struct {
		LoginID string `json:"login_id"`
		Code    string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.LoginID == "" || in.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "login_id and code required"})
		return
	}
	res := h.d.GarminLogin.SubmitMFA(r.Context(), in.LoginID, in.Code)
	writeLoginResult(w, res)
}

// writeLoginResult maps a garmin.LoginResult onto the M5 wire contract.
func writeLoginResult(w http.ResponseWriter, res garmin.LoginResult) {
	switch res.State {
	case garmin.LoginOK:
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case garmin.LoginMFARequired:
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "mfa_required", "login_id": res.LoginID})
	default:
		status := http.StatusBadGateway
		msg := "Garmin login failed."
		switch res.ErrKind {
		case "auth_failed":
			status = http.StatusUnauthorized
			msg = "Garmin rejected the email/password (or the MFA code)."
		case "rate_limited":
			status = http.StatusTooManyRequests
			msg = "Garmin rate-limited this network (HTTP 429). Wait 15–60 minutes before retrying — repeated attempts extend the lockout."
		case "busy":
			status = http.StatusConflict
			msg = "Another Garmin login is already in progress."
		case "timeout", "expired":
			status = http.StatusGone
			msg = "The login attempt expired — start again."
		case "bad_input":
			status = http.StatusBadRequest
			msg = "Malformed login input."
		}
		writeJSON(w, status, map[string]string{"error": msg, "kind": res.ErrKind})
	}
}

// POST /api/garmin/disconnect — delete stored OAuth tokens.
func (h *handlers) garminDisconnect(w http.ResponseWriter, r *http.Request) {
	expanded, ok := syncpkg.ExpandHome(h.d.GarminTokenstore)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot resolve token store path"})
		return
	}
	entries, err := os.ReadDir(expanded)
	if err != nil {
		// Nothing there — already disconnected.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for _, e := range entries {
		_ = os.RemoveAll(expanded + string(os.PathSeparator) + e.Name())
	}
	w.WriteHeader(http.StatusNoContent)
}
