package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"help-my-run/backend/internal/store"
)

// runTimeRe matches a 24h HH:MM (00:00–23:59).
var runTimeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

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
	if in.DailyRunTime == "" {
		in.DailyRunTime = "05:30"
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if !runTimeRe.MatchString(in.DailyRunTime) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "daily_run_time must be HH:MM (00:00-23:59)"})
		return
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timezone must be a valid IANA name"})
		return
	}
	if err := h.d.Store.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm:     in.TargetWeeklyKm,
		ProgressionMode:    in.ProgressionMode,
		Zone2CeilingBpm:    in.Zone2CeilingBpm,
		ThresholdBpm:       in.ThresholdBpm,
		MaxHRBpm:           in.MaxHRBpm,
		RunConstraintsJSON: in.RunConstraintsJSON,
		GoalText:           in.GoalText,
		DailyRunTime:       in.DailyRunTime,
		Timezone:           in.Timezone,
		AgentEnabled:       in.AgentEnabled,
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
		DailyRunTime:       p.DailyRunTime,
		Timezone:           p.Timezone,
		AgentEnabled:       p.AgentEnabled,
		UpdatedAt:          p.UpdatedAt,
	}
}
