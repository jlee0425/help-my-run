package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

func (h *handlers) crossfitParse(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad multipart form"})
		return
	}
	weekStart := r.FormValue("week_start")
	if weekStart == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "week_start required"})
		return
	}
	file, hdr, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image file required"})
		return
	}
	defer file.Close()

	imagePath, err := saveUploadedImage(h.d.ImageDir, weekStart, file, hdr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save image: " + err.Error()})
		return
	}

	week, raw, err := h.d.Coach.ParseCrossFit(r.Context(), weekStart, imagePath)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	parsedJSON, _ := json.Marshal(week)
	if err := h.d.Store.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:   weekStart,
		ImagePath:   &imagePath,
		ParsedJSON:  string(parsedJSON),
		RawResponse: &raw,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, week)
}

type generateRequest struct {
	WeekStart    string                  `json:"week_start"`
	CrossFitWeek *llm.CrossFitWeekParsed `json:"crossfit_week"`
}

func (h *handlers) planGenerate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if req.WeekStart == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "week_start required"})
		return
	}
	// If no edited week supplied, a stored week must exist.
	if req.CrossFitWeek == nil {
		if _, err := h.d.Store.GetCrossFitWeek(req.WeekStart); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "no crossfit week for that week"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	plan, ctxPack, model, err := h.d.Coach.GeneratePlan(r.Context(), req.WeekStart, req.CrossFitWeek)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	planJSON, _ := json.Marshal(plan)
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	id, err := h.d.Store.InsertPlan(store.Plan{
		WeekStart:       req.WeekStart,
		GeneratedAt:     generatedAt,
		Status:          "generated",
		PlanJSON:        string(planJSON),
		FitnessSummary:  plan.FitnessSummary,
		ContextPackJSON: &ctxPack,
		Model:           model,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toPlanResponse(id, req.WeekStart, generatedAt, plan))
}

func (h *handlers) plan(w http.ResponseWriter, r *http.Request) {
	week := r.URL.Query().Get("week")
	if week == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "week required"})
		return
	}
	p, err := h.d.Store.GetLatestPlan(week)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no plan for week"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var parsed llm.PlanParsed
	if err := json.Unmarshal([]byte(p.PlanJSON), &parsed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stored plan corrupt: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toPlanResponse(p.ID, p.WeekStart, p.GeneratedAt, parsed))
}

func (h *handlers) fitness(w http.ResponseWriter, r *http.Request) {
	m, err := h.d.Coach.Fitness(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func toPlanResponse(id int64, weekStart, generatedAt string, p llm.PlanParsed) planResponseDTO {
	days := make([]planDayDTO, 0, len(p.Days))
	for _, d := range p.Days {
		days = append(days, planDayDTO{
			Date: d.Date, Dow: d.Dow, RunType: d.RunType, DistanceKm: d.DistanceKm,
			PaceTarget: d.PaceTarget, TimeNote: d.TimeNote,
			OptionalIfCNS: d.OptionalIfCNS, Rationale: d.Rationale,
		})
	}
	return planResponseDTO{
		ID: id, WeekStart: weekStart, GeneratedAt: generatedAt,
		FitnessSummary: p.FitnessSummary, WeeklyTargetKm: p.WeeklyTargetKm,
		Days: days, WeekRationale: p.WeekRationale, OneFlag: p.OneFlag,
	}
}
