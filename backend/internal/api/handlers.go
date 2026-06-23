package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"

	"help-my-run/backend/internal/store"
)

type handlers struct {
	d Deps
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResp{Status: "ok"})
}

func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	s := h.d.Store

	// Strava connection = a token row exists.
	var stravaConn bool
	var athleteID *int64
	if tok, err := s.GetStravaTokens(); err == nil {
		stravaConn = true
		id := tok.AthleteID
		athleteID = &id
	}

	recoveryDays, _ := s.CountRecoveryDays()
	garminConn := recoveryDays > 0

	var activitiesCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activities`).Scan(&activitiesCount)

	stravaLog, _ := s.GetSyncLog("strava")
	garminLog, _ := s.GetSyncLog("garmin")

	resp := statusResp{
		Strava: stravaStatus{
			sourceStatus: sourceStatus{
				Connected:    stravaConn,
				LastSyncedAt: stravaLog.LastSyncedAt,
				LastRunAt:    stravaLog.LastRunAt,
				Status:       stravaLog.Status,
				Error:        stravaLog.Error,
			},
			AthleteID: athleteID,
		},
		Garmin: sourceStatus{
			Connected:    garminConn,
			LastSyncedAt: garminLog.LastSyncedAt,
			LastRunAt:    garminLog.LastRunAt,
			Status:       garminLog.Status,
			Error:        garminLog.Error,
		},
		Counts: statusCounts{Activities: activitiesCount, RecoveryDays: recoveryDays},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) stravaConnect(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	if err := h.d.Store.SaveOAuthState(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	url := h.d.Strava.AuthorizeURL(state)
	writeJSON(w, http.StatusOK, connectResp{AuthorizeURL: url})
}

func randomState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "state"
	}
	return hex.EncodeToString(b)
}

func (h *handlers) stravaCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		writeHTML(w, "Strava connection failed. You can close this tab.")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" || h.d.Store.ConsumeOAuthState(state) != nil {
		writeHTML(w, "Strava connection failed (invalid state). You can close this tab.")
		return
	}
	code := r.URL.Query().Get("code")
	tok, err := h.d.Strava.Exchange(r.Context(), code)
	if err != nil {
		writeHTML(w, "Strava connection failed. You can close this tab.")
		return
	}
	st := store.StravaTokens{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.ExpiresAt,
		Scope:        tok.Scope,
	}
	if tok.Athlete != nil {
		st.AthleteID = tok.Athlete.ID
	}
	if err := h.d.Store.SaveStravaTokens(st); err != nil {
		writeHTML(w, "Strava connection failed. You can close this tab.")
		return
	}
	writeHTML(w, "Strava connected. You can close this tab.")
}

func (h *handlers) sync(w http.ResponseWriter, r *http.Request) {
	ss, ssn, sErr, gs, gsn, gErr := h.d.SyncFunc(r.Context())
	writeJSON(w, http.StatusOK, syncResp{
		Strava: syncSourceResult{Status: ss, Synced: ssn, Error: sErr},
		Garmin: syncSourceResult{Status: gs, Synced: gsn, Error: gErr},
	})
}

func (h *handlers) activities(w http.ResponseWriter, r *http.Request) {
	limit := clampQuery(r, "limit", 30, 1, 200)
	rows, err := h.d.Store.ListActivities(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]activityDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, activityDTO{
			ActivityID: a.ActivityID, Name: a.Name, Type: a.Type, SportType: a.SportType,
			StartTime: a.StartTime, StartTimeLocal: a.StartTimeLocal,
			DistanceM: a.DistanceM, MovingTimeS: a.MovingTimeS, ElapsedTimeS: a.ElapsedTimeS,
			AvgHR: a.AvgHR, MaxHR: a.MaxHR, AvgSpeed: a.AvgSpeed, MaxSpeed: a.MaxSpeed,
			AvgCadence: a.AvgCadence, ElevationGainM: a.ElevationGainM,
		})
	}
	writeJSON(w, http.StatusOK, activitiesResp{Activities: out})
}

func (h *handlers) recovery(w http.ResponseWriter, r *http.Request) {
	days := clampQuery(r, "days", 30, 1, 365)
	rows, err := h.d.Store.ListRecovery(days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]recoveryDayDTO, 0, len(rows))
	for _, d := range rows {
		rd := recoveryDayDTO{Date: d.Date}
		if d.Sleep != nil {
			rd.Sleep = &sleepDTO{
				DurationS: d.Sleep.DurationS, DeepS: d.Sleep.DeepS, LightS: d.Sleep.LightS,
				RemS: d.Sleep.RemS, AwakeS: d.Sleep.AwakeS, Score: d.Sleep.Score,
			}
		}
		if d.HRV != nil {
			rd.HRV = &hrvDTO{LastNightAvgMs: d.HRV.LastNightAvgMs, Status: d.HRV.Status}
		}
		if d.BodyBattery != nil {
			rd.BodyBattery = &bodyBatteryDTO{
				Charged: d.BodyBattery.Charged, Drained: d.BodyBattery.Drained,
				High: d.BodyBattery.High, Low: d.BodyBattery.Low,
			}
		}
		if d.RHR != nil {
			rd.RHR = &rhrDTO{RestingHR: d.RHR.RestingHR}
		}
		out = append(out, rd)
	}
	writeJSON(w, http.StatusOK, recoveryResp{Recovery: out})
}

// clampQuery parses an int query param, applying a default and [min,max] clamp.
func clampQuery(r *http.Request, key string, def, min, max int) int {
	v := def
	if raw := r.URL.Query().Get(key); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			v = n
		}
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return v
}

func writeHTML(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<!doctype html><html><body><p>" + msg + "</p></body></html>"))
}
