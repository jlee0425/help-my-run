package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

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

// --- handlers completed in the next tasks; minimal compiling stubs for now ---

func (h *handlers) stravaCallback(w http.ResponseWriter, r *http.Request) {
	writeHTML(w, "You can close this tab.")
}

func (h *handlers) sync(w http.ResponseWriter, r *http.Request) {
	ss, ssn, sErr, gs, gsn, gErr := h.d.SyncFunc(r.Context())
	writeJSON(w, http.StatusOK, syncResp{
		Strava: syncSourceResult{Status: ss, Synced: ssn, Error: sErr},
		Garmin: syncSourceResult{Status: gs, Synced: gsn, Error: gErr},
	})
}

func (h *handlers) activities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, activitiesResp{Activities: []activityDTO{}})
}

func (h *handlers) recovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, recoveryResp{Recovery: []recoveryDayDTO{}})
}

func writeHTML(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<!doctype html><html><body><p>" + msg + "</p></body></html>"))
}

var _ = store.ErrNotFound // store import kept for upcoming handlers
