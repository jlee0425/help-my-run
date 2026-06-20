package store

import "testing"

func TestGetAthleteProfileSeeded(t *testing.T) {
	s := newTestStore(t)

	p, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile() error = %v, want seeded row", err)
	}
	if p.TargetWeeklyKm != 20 || p.ProgressionMode != "build" {
		t.Errorf("seed = %+v, want target 20 mode build", p)
	}
	if p.RunConstraintsJSON != "{}" || p.GoalText != "" {
		t.Errorf("seed constraints/goal = %q/%q, want {}/empty", p.RunConstraintsJSON, p.GoalText)
	}
	if p.Zone2CeilingBpm != nil || p.ThresholdBpm != nil || p.MaxHRBpm != nil {
		t.Errorf("seed HR markers = %v/%v/%v, want all nil", p.Zone2CeilingBpm, p.ThresholdBpm, p.MaxHRBpm)
	}
	if p.UpdatedAt == "" {
		t.Error("seed UpdatedAt is empty, want non-empty")
	}
}

func TestUpsertAthleteProfileRoundTrip(t *testing.T) {
	s := newTestStore(t)

	z2 := int64(140)
	thr := int64(165)
	mx := int64(190)
	in := AthleteProfile{
		TargetWeeklyKm:     25,
		ProgressionMode:    "hold",
		Zone2CeilingBpm:    &z2,
		ThresholdBpm:       &thr,
		MaxHRBpm:           &mx,
		RunConstraintsJSON: `{"crossfit_days":["Mon","Tue"]}`,
		GoalText:           "Build cardio over time",
		UpdatedAt:          "ignored-server-sets-it",
	}
	if err := s.UpsertAthleteProfile(in); err != nil {
		t.Fatalf("UpsertAthleteProfile() error = %v", err)
	}

	got, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile() error = %v", err)
	}
	if got.TargetWeeklyKm != 25 || got.ProgressionMode != "hold" {
		t.Errorf("got = %+v, want target 25 mode hold", got)
	}
	if got.Zone2CeilingBpm == nil || *got.Zone2CeilingBpm != 140 {
		t.Errorf("zone2 = %v, want 140", got.Zone2CeilingBpm)
	}
	if got.ThresholdBpm == nil || *got.ThresholdBpm != 165 || got.MaxHRBpm == nil || *got.MaxHRBpm != 190 {
		t.Errorf("thr/max = %v/%v, want 165/190", got.ThresholdBpm, got.MaxHRBpm)
	}
	if got.RunConstraintsJSON != `{"crossfit_days":["Mon","Tue"]}` || got.GoalText != "Build cardio over time" {
		t.Errorf("constraints/goal = %q/%q", got.RunConstraintsJSON, got.GoalText)
	}
	if got.UpdatedAt == "ignored-server-sets-it" || got.UpdatedAt == "" {
		t.Errorf("UpdatedAt = %q, want server-set RFC3339", got.UpdatedAt)
	}

	// Upsert again -> still single row (id=1).
	in.TargetWeeklyKm = 30
	in.Zone2CeilingBpm = nil
	if err := s.UpsertAthleteProfile(in); err != nil {
		t.Fatalf("second UpsertAthleteProfile() error = %v", err)
	}
	got, _ = s.GetAthleteProfile()
	if got.TargetWeeklyKm != 30 || got.Zone2CeilingBpm != nil {
		t.Errorf("after re-upsert = %+v, want target 30 zone2 nil", got)
	}
	var rows int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM athlete_profile`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("athlete_profile row count = %d, want 1", rows)
	}
}

func TestAthleteProfileM2Columns(t *testing.T) {
	s := newTestStore(t)

	p, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile error = %v", err)
	}
	if p.DailyRunTime != "05:30" || p.Timezone != "UTC" || p.AgentEnabled != true {
		t.Errorf("seed M2 fields = (%q,%q,%v), want (05:30,UTC,true)", p.DailyRunTime, p.Timezone, p.AgentEnabled)
	}

	p.DailyRunTime = "06:15"
	p.Timezone = "Asia/Seoul"
	p.AgentEnabled = false
	if err := s.UpsertAthleteProfile(p); err != nil {
		t.Fatalf("UpsertAthleteProfile error = %v", err)
	}
	got, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile after upsert error = %v", err)
	}
	if got.DailyRunTime != "06:15" || got.Timezone != "Asia/Seoul" || got.AgentEnabled != false {
		t.Errorf("M2 fields = (%q,%q,%v), want (06:15,Asia/Seoul,false)", got.DailyRunTime, got.Timezone, got.AgentEnabled)
	}
	if got.TargetWeeklyKm != p.TargetWeeklyKm || got.ProgressionMode != p.ProgressionMode {
		t.Errorf("M1 fields drifted: got %+v", got)
	}
}
