package store

import "testing"

func TestUpsertVo2maxAndListVo2max(t *testing.T) {
	s := newTestStore(t)

	// Insert three days out of order; ListVo2max returns most-recent-first.
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-16", Vo2max: f64p(51.0), RawJSON: `{"generic":{"vo2MaxValue":51.0}}`}); err != nil {
		t.Fatalf("UpsertVo2max 16: %v", err)
	}
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-18", Vo2max: f64p(52.0), RawJSON: `{"generic":{"vo2MaxValue":52.0}}`}); err != nil {
		t.Fatalf("UpsertVo2max 18: %v", err)
	}
	// A null vo2max value is permitted (column is REAL / nullable).
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-17", Vo2max: nil, RawJSON: `{"generic":null}`}); err != nil {
		t.Fatalf("UpsertVo2max 17 null: %v", err)
	}

	pts, err := s.ListVo2max(30)
	if err != nil {
		t.Fatalf("ListVo2max error = %v", err)
	}
	if len(pts) != 3 {
		t.Fatalf("ListVo2max len = %d, want 3", len(pts))
	}
	// Most-recent-first by date.
	if pts[0].Date != "2026-06-18" || pts[1].Date != "2026-06-17" || pts[2].Date != "2026-06-16" {
		t.Errorf("dates = [%s,%s,%s], want [18,17,16]", pts[0].Date, pts[1].Date, pts[2].Date)
	}
	if pts[0].Vo2max == nil || *pts[0].Vo2max != 52.0 {
		t.Errorf("06-18 vo2max = %v, want 52.0", pts[0].Vo2max)
	}
	if pts[1].Vo2max != nil {
		t.Errorf("06-17 vo2max = %v, want nil (stored null)", pts[1].Vo2max)
	}

	// LIMIT is honored.
	lim, err := s.ListVo2max(2)
	if err != nil {
		t.Fatalf("ListVo2max(2) error = %v", err)
	}
	if len(lim) != 2 || lim[0].Date != "2026-06-18" {
		t.Errorf("ListVo2max(2) = %+v, want 2 newest", lim)
	}

	// Re-upsert 06-18 with a new value -> update, not duplicate.
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-18", Vo2max: f64p(53.0), RawJSON: `{"generic":{"vo2MaxValue":53.0}}`}); err != nil {
		t.Fatalf("re-UpsertVo2max: %v", err)
	}
	pts, _ = s.ListVo2max(30)
	if len(pts) != 3 {
		t.Fatalf("after re-upsert len = %d, want 3", len(pts))
	}
	if pts[0].Vo2max == nil || *pts[0].Vo2max != 53.0 {
		t.Errorf("06-18 after re-upsert = %v, want 53.0", pts[0].Vo2max)
	}
}
