package store

import "testing"

func TestGetCrossFitWeekNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetCrossFitWeek("2026-06-22"); err != ErrNotFound {
		t.Fatalf("GetCrossFitWeek on empty = %v, want ErrNotFound", err)
	}
}

func TestUpsertCrossFitWeekRoundTrip(t *testing.T) {
	s := newTestStore(t)

	img := "/data/crossfit/2026-06-22.jpg"
	raw := "```json\n{...}\n```"
	in := CrossFitWeek{
		WeekStart:   "2026-06-22",
		ImagePath:   &img,
		ParsedJSON:  `{"week_start":"2026-06-22","days":[]}`,
		RawResponse: &raw,
		CreatedAt:   "set-by-store",
		UpdatedAt:   "set-by-store",
	}
	if err := s.UpsertCrossFitWeek(in); err != nil {
		t.Fatalf("UpsertCrossFitWeek() error = %v", err)
	}

	got, err := s.GetCrossFitWeek("2026-06-22")
	if err != nil {
		t.Fatalf("GetCrossFitWeek() error = %v", err)
	}
	if got.WeekStart != "2026-06-22" {
		t.Errorf("WeekStart = %q, want 2026-06-22", got.WeekStart)
	}
	if got.ImagePath == nil || *got.ImagePath != img {
		t.Errorf("ImagePath = %v, want %q", got.ImagePath, img)
	}
	if got.ParsedJSON != `{"week_start":"2026-06-22","days":[]}` {
		t.Errorf("ParsedJSON = %q", got.ParsedJSON)
	}
	if got.RawResponse == nil || *got.RawResponse != raw {
		t.Errorf("RawResponse = %v, want %q", got.RawResponse, raw)
	}
	if got.CreatedAt == "" || got.CreatedAt == "set-by-store" {
		t.Errorf("CreatedAt = %q, want server-set", got.CreatedAt)
	}
	createdFirst := got.CreatedAt

	// Re-upsert (same PK week_start): updates parsed_json, preserves created_at.
	in.ParsedJSON = `{"week_start":"2026-06-22","days":[{"dow":"Mon"}]}`
	in.ImagePath = nil
	if err := s.UpsertCrossFitWeek(in); err != nil {
		t.Fatalf("second UpsertCrossFitWeek() error = %v", err)
	}
	got, _ = s.GetCrossFitWeek("2026-06-22")
	if got.ParsedJSON != `{"week_start":"2026-06-22","days":[{"dow":"Mon"}]}` {
		t.Errorf("after re-upsert ParsedJSON = %q", got.ParsedJSON)
	}
	if got.ImagePath != nil {
		t.Errorf("ImagePath = %v, want nil after re-upsert", got.ImagePath)
	}
	if got.CreatedAt != createdFirst {
		t.Errorf("CreatedAt changed on update: %q -> %q, want preserved", createdFirst, got.CreatedAt)
	}
	var rows int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM crossfit_weeks`).Scan(&rows)
	if rows != 1 {
		t.Errorf("row count = %d, want 1 (same PK)", rows)
	}
}
