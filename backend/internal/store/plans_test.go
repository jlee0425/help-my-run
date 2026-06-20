package store

import "testing"

func TestGetLatestPlanNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetLatestPlan("2026-06-22"); err != ErrNotFound {
		t.Fatalf("GetLatestPlan on empty = %v, want ErrNotFound", err)
	}
}

func TestInsertAndGetLatestPlan(t *testing.T) {
	s := newTestStore(t)

	ctx := `{"metrics":{}}`
	p1 := Plan{
		WeekStart:       "2026-06-22",
		GeneratedAt:     "2026-06-20T08:00:00Z",
		Status:          "generated",
		PlanJSON:        `{"weekly_target_km":20}`,
		FitnessSummary:  "first read",
		ContextPackJSON: &ctx,
		Model:           "claude-opus-4-8",
	}
	id1, err := s.InsertPlan(p1)
	if err != nil {
		t.Fatalf("InsertPlan(p1) error = %v", err)
	}
	if id1 <= 0 {
		t.Errorf("id1 = %d, want positive AUTOINCREMENT id", id1)
	}

	// Second plan, same week, later generated_at -> becomes the latest.
	p2 := Plan{
		WeekStart:      "2026-06-22",
		GeneratedAt:    "2026-06-20T09:30:00Z",
		Status:         "generated",
		PlanJSON:       `{"weekly_target_km":22}`,
		FitnessSummary: "second read",
		Model:          "claude-opus-4-8",
	}
	id2, err := s.InsertPlan(p2)
	if err != nil {
		t.Fatalf("InsertPlan(p2) error = %v", err)
	}
	if id2 == id1 {
		t.Errorf("id2 = %d equals id1, want distinct AUTOINCREMENT", id2)
	}

	got, err := s.GetLatestPlan("2026-06-22")
	if err != nil {
		t.Fatalf("GetLatestPlan() error = %v", err)
	}
	if got.ID != id2 {
		t.Errorf("latest ID = %d, want %d (most recent generated_at)", got.ID, id2)
	}
	if got.PlanJSON != `{"weekly_target_km":22}` || got.FitnessSummary != "second read" {
		t.Errorf("latest = %+v, want second plan", got)
	}
	if got.ContextPackJSON != nil {
		t.Errorf("p2 ContextPackJSON = %v, want nil", got.ContextPackJSON)
	}

	// A different week with no plan -> ErrNotFound.
	if _, err := s.GetLatestPlan("2026-06-29"); err != ErrNotFound {
		t.Errorf("GetLatestPlan(other week) = %v, want ErrNotFound", err)
	}
}
