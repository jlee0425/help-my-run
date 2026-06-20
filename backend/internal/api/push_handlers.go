package api

import (
	"encoding/json"
	"net/http"

	"help-my-run/backend/internal/store"
)

func (h *handlers) pushRegister(w http.ResponseWriter, r *http.Request) {
	var in pushRegisterRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if in.ExpoPushToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expo_push_token required"})
		return
	}
	if in.Platform != "ios" && in.Platform != "android" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform must be ios or android"})
		return
	}
	if err := h.d.Store.UpsertDeviceToken(store.DeviceToken{
		ExpoPushToken: in.ExpoPushToken,
		Platform:      in.Platform,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	toks, err := h.d.Store.ListDeviceTokens()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	updatedAt := ""
	for _, t := range toks {
		if t.ExpoPushToken == in.ExpoPushToken {
			updatedAt = t.UpdatedAt
			break
		}
	}
	writeJSON(w, http.StatusOK, pushRegisterResponseDTO{
		ExpoPushToken: in.ExpoPushToken,
		Platform:      in.Platform,
		UpdatedAt:     updatedAt,
	})
}
