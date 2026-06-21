package api

import (
	"encoding/json"
	"net/http"

	"help-my-run/backend/internal/progress"
)

// progress serves GET /api/progress?weeks=12 — the deterministic trend report
// (no Claude). The progress.ProgressReport is serialized directly (its json tags
// are snake_case, like FitnessMetrics served at /api/fitness).
func (h *handlers) progress(w http.ResponseWriter, r *http.Request) {
	weeks := clampQuery(r, "weeks", progress.DefaultWeeks, progress.MinWeeks, progress.MaxWeeks)
	rep, err := h.d.Progress.Report(r.Context(), weeks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// analyzeProgressRequest is the optional POST /api/progress/analyze body.
type analyzeProgressRequest struct {
	Weeks int `json:"weeks"`
}

// analyzeProgress serves POST /api/progress/analyze — the claude -p read over the
// computed trends, with a deterministic fallback (handled in the engine). An
// absent/invalid window defaults to DefaultWeeks, clamped to [MinWeeks,MaxWeeks].
func (h *handlers) analyzeProgress(w http.ResponseWriter, r *http.Request) {
	var req analyzeProgressRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // empty body OK
	weeks := req.Weeks
	if weeks < progress.MinWeeks || weeks > progress.MaxWeeks {
		weeks = progress.DefaultWeeks
	}
	read, err := h.d.Progress.Analyze(r.Context(), weeks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, read)
}
