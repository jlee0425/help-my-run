package store

import (
	"database/sql"
	"errors"
	"time"
)

// DeviceToken is the device_tokens row (one per registered device).
type DeviceToken struct {
	ExpoPushToken string // PK
	Platform      string // "ios"|"android"
	UpdatedAt     string
}

// DailyDecision is the daily_decisions row. Session JSONs are raw PlanDay JSON
// strings so the store stays agnostic of llm types; nil columns are *string.
type DailyDecision struct {
	Date                string // PK, local date YYYY-MM-DD
	ReadinessColor      string // "green"|"amber"|"red"
	DriversJSON         string
	OriginalSessionJSON *string // PlanDay JSON or nil
	AdjustedSessionJSON *string // PlanDay JSON or nil
	Action              string  // "STAND"|"SOFTEN"|"MOVE"|"REST_DAY"
	Rationale           string
	Source              string // "ai"|"fallback"
	RawResponse         *string
	CreatedAt           string
	UpdatedAt           string
}

// AgentRun is one agent_runs row (idempotency + last-run history).
type AgentRun struct {
	ID          int64
	LastRunDate string // local date YYYY-MM-DD
	Status      string // "ok"|"error"
	Error       *string
	RanAt       string // RFC3339 UTC
}

// UpsertDeviceToken inserts/refreshes a device token by its PK. updated_at is set
// server-side to now (UTC RFC3339).
func (s *Store) UpsertDeviceToken(t DeviceToken) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO device_tokens (expo_push_token, platform, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(expo_push_token) DO UPDATE SET
			platform   = excluded.platform,
			updated_at = excluded.updated_at`,
		t.ExpoPushToken, t.Platform, now)
	return err
}

// ListDeviceTokens returns all registered device tokens (most-recent-first).
func (s *Store) ListDeviceTokens() ([]DeviceToken, error) {
	rows, err := s.DB.Query(`
		SELECT expo_push_token, platform, updated_at
		FROM device_tokens ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DeviceToken, 0)
	for rows.Next() {
		var t DeviceToken
		if err := rows.Scan(&t.ExpoPushToken, &t.Platform, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteDeviceToken removes a token (the DeviceNotRegistered drop path). Deleting
// a missing token is a no-op.
func (s *Store) DeleteDeviceToken(expoPushToken string) error {
	_, err := s.DB.Exec(`DELETE FROM device_tokens WHERE expo_push_token = ?`, expoPushToken)
	return err
}

// UpsertDailyDecision upserts today's decision by date. created_at is set on
// insert and preserved on update; updated_at is always bumped to now.
func (s *Store) UpsertDailyDecision(d DailyDecision) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO daily_decisions
			(date, readiness_color, drivers_json, original_session_json,
			 adjusted_session_json, action, rationale, source, raw_response,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			readiness_color       = excluded.readiness_color,
			drivers_json          = excluded.drivers_json,
			original_session_json = excluded.original_session_json,
			adjusted_session_json = excluded.adjusted_session_json,
			action                = excluded.action,
			rationale             = excluded.rationale,
			source                = excluded.source,
			raw_response          = excluded.raw_response,
			updated_at            = excluded.updated_at`,
		d.Date, d.ReadinessColor, d.DriversJSON, d.OriginalSessionJSON,
		d.AdjustedSessionJSON, d.Action, d.Rationale, d.Source, d.RawResponse,
		now, now)
	return err
}

// GetDailyDecision returns the decision row for date, or ErrNotFound.
func (s *Store) GetDailyDecision(date string) (DailyDecision, error) {
	var d DailyDecision
	var orig, adj, raw sql.NullString
	err := s.DB.QueryRow(`
		SELECT date, readiness_color, drivers_json, original_session_json,
		       adjusted_session_json, action, rationale, source, raw_response,
		       created_at, updated_at
		FROM daily_decisions WHERE date = ?`, date).
		Scan(&d.Date, &d.ReadinessColor, &d.DriversJSON, &orig, &adj,
			&d.Action, &d.Rationale, &d.Source, &raw, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DailyDecision{}, ErrNotFound
	}
	if err != nil {
		return DailyDecision{}, err
	}
	if orig.Valid {
		d.OriginalSessionJSON = &orig.String
	}
	if adj.Valid {
		d.AdjustedSessionJSON = &adj.String
	}
	if raw.Valid {
		d.RawResponse = &raw.String
	}
	return d, nil
}

// UpsertAgentRun upserts the agent run for a local date (idempotency key).
// ran_at is set server-side to now (UTC RFC3339).
func (s *Store) UpsertAgentRun(r AgentRun) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO agent_runs (last_run_date, status, error, ran_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(last_run_date) DO UPDATE SET
			status = excluded.status,
			error  = excluded.error,
			ran_at = excluded.ran_at`,
		r.LastRunDate, r.Status, r.Error, now)
	return err
}

// GetAgentRun returns the agent run for date, or ErrNotFound.
func (s *Store) GetAgentRun(date string) (AgentRun, error) {
	var r AgentRun
	var errStr sql.NullString
	err := s.DB.QueryRow(`
		SELECT id, last_run_date, status, error, ran_at
		FROM agent_runs WHERE last_run_date = ?`, date).
		Scan(&r.ID, &r.LastRunDate, &r.Status, &errStr, &r.RanAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, ErrNotFound
	}
	if err != nil {
		return AgentRun{}, err
	}
	if errStr.Valid {
		r.Error = &errStr.String
	}
	return r, nil
}

// LatestAgentRun returns the most-recent agent run by date, or ErrNotFound.
func (s *Store) LatestAgentRun() (AgentRun, error) {
	var r AgentRun
	var errStr sql.NullString
	err := s.DB.QueryRow(`
		SELECT id, last_run_date, status, error, ran_at
		FROM agent_runs ORDER BY last_run_date DESC LIMIT 1`).
		Scan(&r.ID, &r.LastRunDate, &r.Status, &errStr, &r.RanAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, ErrNotFound
	}
	if err != nil {
		return AgentRun{}, err
	}
	if errStr.Valid {
		r.Error = &errStr.String
	}
	return r, nil
}

// DeleteAgentRun removes the agent_runs row for a local date, resetting the
// persistent once-per-day guard so POST /api/agent/run?force=true can re-run the
// day. Deleting a missing row is a no-op.
func (s *Store) DeleteAgentRun(date string) error {
	_, err := s.DB.Exec(`DELETE FROM agent_runs WHERE last_run_date = ?`, date)
	return err
}
