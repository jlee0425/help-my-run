package store

import (
	"database/sql"
	"errors"
)

// Plan maps to one plans row. Multiple plans per week_start are allowed
// (regenerate appends); "latest" is the most recent generated_at.
type Plan struct {
	ID              int64
	WeekStart       string
	GeneratedAt     string
	Status          string
	PlanJSON        string // Stage-2 parsed object, verbatim
	FitnessSummary  string
	ContextPackJSON *string
	Model           string
}

// InsertPlan inserts a new plan row and returns its AUTOINCREMENT id.
func (s *Store) InsertPlan(p Plan) (int64, error) {
	res, err := s.DB.Exec(`
		INSERT INTO plans
			(week_start, generated_at, status, plan_json, fitness_summary,
			 context_pack_json, model)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.WeekStart, p.GeneratedAt, p.Status, p.PlanJSON, p.FitnessSummary,
		p.ContextPackJSON, p.Model)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLatestPlan returns the most recent plan for weekStart, or ErrNotFound.
func (s *Store) GetLatestPlan(weekStart string) (Plan, error) {
	var p Plan
	var ctx sql.NullString
	err := s.DB.QueryRow(`
		SELECT id, week_start, generated_at, status, plan_json, fitness_summary,
		       context_pack_json, model
		FROM plans
		WHERE week_start = ?
		ORDER BY generated_at DESC, id DESC
		LIMIT 1`, weekStart).
		Scan(&p.ID, &p.WeekStart, &p.GeneratedAt, &p.Status, &p.PlanJSON,
			&p.FitnessSummary, &ctx, &p.Model)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	if ctx.Valid {
		p.ContextPackJSON = &ctx.String
	}
	return p, nil
}
