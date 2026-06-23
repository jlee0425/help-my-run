package api

import (
	"net/http"
	"strconv"
)

type handlers struct {
	d Deps
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResp{Status: "ok"})
}

func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	s := h.d.Store

	recoveryDays, _ := s.CountRecoveryDays()

	var activitiesCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activities`).Scan(&activitiesCount)

	garminLog, _ := s.GetSyncLog("garmin")
	// "connected" = the last worker invocation authenticated successfully
	// (spec §3.4/§7), derived from the garmin sync_log status — NOT a
	// recovery-data-presence proxy. A fresh/never-run DB has status "never"
	// (00001 seed) -> connected:false; a failed login -> status "error" ->
	// connected:false; a successful sync -> "ok" -> connected:true.
	garminConn := garminLog.Status == "ok"

	resp := statusResp{
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

func (h *handlers) sync(w http.ResponseWriter, r *http.Request) {
	gs, gsn, gErr := h.d.SyncFunc(r.Context())
	writeJSON(w, http.StatusOK, syncResp{
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
