package store

import (
	"database/sql"
	"errors"
	"time"
)

// AthleteProfile is the single-row athlete_profile record (id=1). Nullable HR
// markers are pointers; run_constraints_json is a raw JSON string.
type AthleteProfile struct {
	TargetWeeklyKm     float64
	ProgressionMode    string // "build" | "hold"
	Zone2CeilingBpm    *int64
	ThresholdBpm       *int64
	MaxHRBpm           *int64
	RunConstraintsJSON string
	GoalText           string
	DailyRunTime       string // "HH:MM" 24h local (M2)
	Timezone           string // IANA, e.g. "Asia/Seoul" (M2)
	AgentEnabled       bool   // M2 daily agent on/off
	GoalsJSON          string // M5: JSON array, e.g. ["crossfit","fitness"]
	WeekJSON           string // M5: {"runs_per_week":4,"crossfit_days":3,"rest_day":"monday"}
	GuardrailsJSON     string // M5: {"no_b2b_hard":true,...} five coach guardrails
	UpdatedAt          string
}

// GetAthleteProfile returns the single profile row (id=1), or ErrNotFound.
func (s *Store) GetAthleteProfile() (AthleteProfile, error) {
	var p AthleteProfile
	var z2, thr, mx sql.NullInt64
	var agentEnabled int64
	err := s.DB.QueryRow(`
		SELECT target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
		       max_hr_bpm, run_constraints_json, goal_text,
		       daily_run_time, timezone, agent_enabled,
		       goals_json, week_json, guardrails_json, updated_at
		FROM athlete_profile WHERE id = 1`).
		Scan(&p.TargetWeeklyKm, &p.ProgressionMode, &z2, &thr, &mx,
			&p.RunConstraintsJSON, &p.GoalText,
			&p.DailyRunTime, &p.Timezone, &agentEnabled,
			&p.GoalsJSON, &p.WeekJSON, &p.GuardrailsJSON, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AthleteProfile{}, ErrNotFound
	}
	if err != nil {
		return AthleteProfile{}, err
	}
	if z2.Valid {
		p.Zone2CeilingBpm = &z2.Int64
	}
	if thr.Valid {
		p.ThresholdBpm = &thr.Int64
	}
	if mx.Valid {
		p.MaxHRBpm = &mx.Int64
	}
	p.AgentEnabled = agentEnabled != 0
	return p, nil
}

// UpsertAthleteProfile upserts the single profile row (id always 1). updated_at
// is set server-side to now (UTC RFC3339).
func (s *Store) UpsertAthleteProfile(p AthleteProfile) error {
	now := time.Now().UTC().Format(time.RFC3339)
	agentEnabled := int64(0)
	if p.AgentEnabled {
		agentEnabled = 1
	}
	goals, week, guardrails := p.GoalsJSON, p.WeekJSON, p.GuardrailsJSON
	if goals == "" {
		goals = "[]"
	}
	if week == "" {
		week = "{}"
	}
	if guardrails == "" {
		guardrails = "{}"
	}
	_, err := s.DB.Exec(`
		INSERT INTO athlete_profile
			(id, target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
			 max_hr_bpm, run_constraints_json, goal_text,
			 daily_run_time, timezone, agent_enabled,
			 goals_json, week_json, guardrails_json, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			target_weekly_km     = excluded.target_weekly_km,
			progression_mode     = excluded.progression_mode,
			zone2_ceiling_bpm    = excluded.zone2_ceiling_bpm,
			threshold_bpm        = excluded.threshold_bpm,
			max_hr_bpm           = excluded.max_hr_bpm,
			run_constraints_json = excluded.run_constraints_json,
			goal_text            = excluded.goal_text,
			daily_run_time       = excluded.daily_run_time,
			timezone             = excluded.timezone,
			agent_enabled        = excluded.agent_enabled,
			goals_json           = excluded.goals_json,
			week_json            = excluded.week_json,
			guardrails_json      = excluded.guardrails_json,
			updated_at           = excluded.updated_at`,
		p.TargetWeeklyKm, p.ProgressionMode, p.Zone2CeilingBpm, p.ThresholdBpm,
		p.MaxHRBpm, p.RunConstraintsJSON, p.GoalText,
		p.DailyRunTime, p.Timezone, agentEnabled,
		goals, week, guardrails, now)
	return err
}
