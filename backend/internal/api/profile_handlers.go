package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"help-my-run/backend/internal/store"
)

func (h *handlers) profile(w http.ResponseWriter, r *http.Request) {
	p, err := h.d.Store.GetAthleteProfile()
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

func (h *handlers) updateProfile(w http.ResponseWriter, r *http.Request) {
	var in profileDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if in.ProgressionMode == "" {
		in.ProgressionMode = "build"
	}
	if in.RunConstraintsJSON == "" {
		in.RunConstraintsJSON = "{}"
	}
	if err := h.d.Store.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm:     in.TargetWeeklyKm,
		ProgressionMode:    in.ProgressionMode,
		Zone2CeilingBpm:    in.Zone2CeilingBpm,
		ThresholdBpm:       in.ThresholdBpm,
		MaxHRBpm:           in.MaxHRBpm,
		RunConstraintsJSON: in.RunConstraintsJSON,
		GoalText:           in.GoalText,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	p, err := h.d.Store.GetAthleteProfile()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

func toProfileDTO(p store.AthleteProfile) profileDTO {
	return profileDTO{
		TargetWeeklyKm:     p.TargetWeeklyKm,
		ProgressionMode:    p.ProgressionMode,
		Zone2CeilingBpm:    p.Zone2CeilingBpm,
		ThresholdBpm:       p.ThresholdBpm,
		MaxHRBpm:           p.MaxHRBpm,
		RunConstraintsJSON: p.RunConstraintsJSON,
		GoalText:           p.GoalText,
		UpdatedAt:          p.UpdatedAt,
	}
}
