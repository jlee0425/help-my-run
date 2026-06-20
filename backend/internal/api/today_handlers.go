package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

// resolveDate returns the ?date= param (validated) or today in UTC if absent.
func resolveDate(r *http.Request) (string, bool) {
	d := r.URL.Query().Get("date")
	if d == "" {
		return time.Now().UTC().Format("2006-01-02"), true
	}
	return d, validISODate(d)
}

func (h *handlers) today(w http.ResponseWriter, r *http.Request) {
	date, ok := resolveDate(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date must be an ISO date (YYYY-MM-DD)"})
		return
	}
	dec, err := h.d.Store.GetDailyDecision(date)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no decision for date"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := toTodayResponse(dec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) undoToday(w http.ResponseWriter, r *http.Request) {
	date, ok := resolveDate(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date must be an ISO date (YYYY-MM-DD)"})
		return
	}
	dec, err := h.d.Store.GetDailyDecision(date)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no decision for date"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	dec.AdjustedSessionJSON = dec.OriginalSessionJSON
	dec.Action = string(llm.ActionStand)
	dec.Rationale = "Reverted to original session."
	if err := h.d.Store.UpsertDailyDecision(dec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	updated, err := h.d.Store.GetDailyDecision(date)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := toTodayResponse(updated)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) agentRun(w http.ResponseWriter, r *http.Request) {
	date, ok := resolveDate(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date must be an ISO date (YYYY-MM-DD)"})
		return
	}
	force := r.URL.Query().Get("force") == "true"
	res := h.d.Agent.RunDaily(r.Context(), date, force)
	writeJSON(w, http.StatusOK, res)
}

// toTodayResponse maps a stored DailyDecision into the wire DTO.
func toTodayResponse(d store.DailyDecision) (todayResponseDTO, error) {
	var drivers readinessDriversDTO
	if err := json.Unmarshal([]byte(d.DriversJSON), &drivers); err != nil {
		return todayResponseDTO{}, errors.New("stored drivers corrupt: " + err.Error())
	}
	reasons := parseReasons(d.DriversJSON)
	stale := parseStale(d.DriversJSON)

	orig, err := planDayDTOPtr(d.OriginalSessionJSON)
	if err != nil {
		return todayResponseDTO{}, errors.New("stored original_session corrupt: " + err.Error())
	}
	eff, err := planDayDTOPtr(d.AdjustedSessionJSON)
	if err != nil {
		return todayResponseDTO{}, errors.New("stored adjusted_session corrupt: " + err.Error())
	}
	return todayResponseDTO{
		Date:             d.Date,
		ReadinessColor:   d.ReadinessColor,
		Drivers:          drivers,
		Reasons:          reasons,
		Action:           d.Action,
		OriginalSession:  orig,
		EffectiveSession: eff,
		Rationale:        d.Rationale,
		Source:           d.Source,
		Stale:            stale,
	}, nil
}

// parseReasons extracts the optional "reasons" array embedded in drivers_json by
// the agent. Absent -> empty slice.
func parseReasons(driversJSON string) []string {
	var wrap struct {
		Reasons []string `json:"reasons"`
	}
	_ = json.Unmarshal([]byte(driversJSON), &wrap)
	if wrap.Reasons == nil {
		return []string{}
	}
	return wrap.Reasons
}

// parseStale extracts the optional "stale" boolean embedded in drivers_json by the
// agent (marshalDriversWithReasons). Absent -> false.
func parseStale(driversJSON string) bool {
	var wrap struct {
		Stale bool `json:"stale"`
	}
	_ = json.Unmarshal([]byte(driversJSON), &wrap)
	return wrap.Stale
}

// planDayDTOPtr unmarshals a *string PlanDay JSON column into a *planDayDTO (nil
// when nil/empty — rest day or undone-to-null).
func planDayDTOPtr(js *string) (*planDayDTO, error) {
	if js == nil || *js == "" {
		return nil, nil
	}
	var pd llm.PlanDay
	if err := json.Unmarshal([]byte(*js), &pd); err != nil {
		return nil, err
	}
	dto := planDayDTO{
		Date: pd.Date, Dow: pd.Dow, RunType: pd.RunType, DistanceKm: pd.DistanceKm,
		PaceTarget: pd.PaceTarget, TimeNote: pd.TimeNote,
		OptionalIfCNS: pd.OptionalIfCNS, Rationale: pd.Rationale,
	}
	return &dto, nil
}
