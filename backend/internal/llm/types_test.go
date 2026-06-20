package llm

import (
	"encoding/json"
	"testing"
)

func TestCrossFitWeekParsedRoundTrip(t *testing.T) {
	src := `{"week_start":"2026-06-22","days":[{"date":"2026-06-22","dow":"Mon","has_crossfit":true,"focus":"Back squat","cns_load":"high","leg_load":"high","notes":"Heavy"}]}`
	var w CrossFitWeekParsed
	if err := json.Unmarshal([]byte(src), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.WeekStart != "2026-06-22" || len(w.Days) != 1 {
		t.Fatalf("parsed = %+v", w)
	}
	d := w.Days[0]
	if d.Dow != "Mon" || !d.HasCrossFit || d.CNSLoad != LoadHigh || d.LegLoad != LoadHigh {
		t.Errorf("day = %+v, want Mon/has/high/high", d)
	}
	b, _ := json.Marshal(w)
	if !json_contains(b, `"has_crossfit":true`) || !json_contains(b, `"cns_load":"high"`) {
		t.Errorf("marshal = %s, want snake_case keys", b)
	}
}

func TestPlanParsedRoundTrip(t *testing.T) {
	src := `{"fitness_summary":"ok","weekly_target_km":20,"days":[{"date":"2026-06-23","dow":"Tue","run_type":"easy","distance_km":5,"pace_target":"6:00/km","time_note":"~20:00","optional_if_cns":true,"rationale":"why"}],"week_rationale":"para","one_flag":"flag"}`
	var p PlanParsed
	if err := json.Unmarshal([]byte(src), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.WeeklyTargetKm != 20 || p.OneFlag != "flag" || len(p.Days) != 1 {
		t.Fatalf("parsed = %+v", p)
	}
	d := p.Days[0]
	if d.RunType != "easy" || d.DistanceKm != 5 || !d.OptionalIfCNS {
		t.Errorf("day = %+v, want easy/5/optional", d)
	}
	b, _ := json.Marshal(p)
	if !json_contains(b, `"optional_if_cns":true`) || !json_contains(b, `"weekly_target_km":20`) {
		t.Errorf("marshal = %s, want snake_case keys", b)
	}
}

func json_contains(b []byte, sub string) bool { return contains(string(b), sub) }
