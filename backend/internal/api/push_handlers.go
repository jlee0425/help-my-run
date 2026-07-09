package api

import (
	"encoding/json"
	"net/http"

	"help-my-run/backend/internal/store"
)

// GET /api/push/vapid-public-key
func (h *handlers) pushVAPIDKey(w http.ResponseWriter, r *http.Request) {
	if h.d.Push == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "push not wired"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": h.d.Push.PublicKey()})
}

// POST /api/push/subscribe {endpoint, keys:{p256dh,auth}}
func (h *handlers) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if in.Endpoint == "" || in.Keys.P256dh == "" || in.Keys.Auth == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint and keys required"})
		return
	}
	if err := h.d.Store.UpsertPushSubscription(store.PushSubscription{
		Endpoint: in.Endpoint, P256dh: in.Keys.P256dh, Auth: in.Keys.Auth,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/push/subscribe {endpoint}
func (h *handlers) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint required"})
		return
	}
	if err := h.d.Store.DeletePushSubscription(in.Endpoint); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/push/test
func (h *handlers) pushTest(w http.ResponseWriter, r *http.Request) {
	if h.d.Push == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "push not wired"})
		return
	}
	if err := h.d.Push.Broadcast(r.Context(), "Test notification", "Push is working.", "/"); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
