package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/streams"
)

// parseActivityID reads the {id} path param as an int64.
func parseActivityID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

// streamAnalysisToDTO maps the engine output to the wire DTO with has_stream=true.
func streamAnalysisToDTO(a streams.StreamAnalysis) streamAnalysisDTO {
	tiz := make([]zoneTimeDTO, 0, len(a.TimeInZone))
	for _, z := range a.TimeInZone {
		tiz = append(tiz, zoneTimeDTO{Zone: z.Zone, Seconds: z.Seconds, Pct: z.Pct})
	}
	return streamAnalysisDTO{
		ActivityID:    a.ActivityID,
		HasStream:     true,
		HasHR:         a.HasHR,
		TimeInZone:    tiz,
		DecouplingPct: a.DecouplingPct,
		PaHRFirst:     a.PaHRFirst,
		PaHRSecond:    a.PaHRSecond,
		Zones: zoneBoundsDTO{
			Z1Hi: a.Zones.Z1Hi, Z2Hi: a.Zones.Z2Hi, Z3Hi: a.Zones.Z3Hi, Z4Hi: a.Zones.Z4Hi,
		},
		Source:     a.Source,
		ComputedAt: a.ComputedAt,
	}
}

// notFetchedDTO is the 200 not-fetched body (has_stream:false), echoing the id.
func notFetchedDTO(activityID int64) streamAnalysisDTO {
	return streamAnalysisDTO{
		ActivityID: activityID,
		HasStream:  false,
		HasHR:      false,
		TimeInZone: []zoneTimeDTO{},
	}
}

// activityAnalysis serves GET /api/activities/{id}/analysis — the cached
// time-in-zone + decoupling. Not-fetched is 200 + has_stream:false (NOT 404).
func (h *handlers) activityAnalysis(w http.ResponseWriter, r *http.Request) {
	id, ok := parseActivityID(w, r)
	if !ok {
		return
	}
	a, err := h.d.Streams.GetOrComputeAnalysis(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, notFetchedDTO(id))
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, streamAnalysisToDTO(a))
}

// fetchStream serves POST /api/activities/{id}/stream/fetch — fetch-if-missing,
// compute, cache, return. 500 on fetch error.
func (h *handlers) fetchStream(w http.ResponseWriter, r *http.Request) {
	id, ok := parseActivityID(w, r)
	if !ok {
		return
	}
	a, err := h.d.Streams.FetchAndAnalyze(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, streamAnalysisToDTO(a))
}
