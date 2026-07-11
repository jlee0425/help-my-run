package demo

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestFixtureMatchesBuilder pins the committed fixture.json to BuildDataset().
// If BuildDataset changes without regenerating (`go generate ./internal/demo`),
// this fails instead of the demo silently drifting from its authoring source.
func TestFixtureMatchesBuilder(t *testing.T) {
	var embedded Dataset
	if err := json.Unmarshal(fixtureJSON, &embedded); err != nil {
		t.Fatalf("unmarshal embedded fixture: %v", err)
	}
	got, err := json.Marshal(embedded)
	if err != nil {
		t.Fatalf("re-marshal embedded: %v", err)
	}
	want, err := json.Marshal(BuildDataset())
	if err != nil {
		t.Fatalf("marshal BuildDataset: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("fixture.json is stale — run `go generate ./internal/demo`")
	}
}

// TestFixtureShape sanity-checks the authored counts so a truncated fixture is
// caught before the seeder runs.
func TestFixtureShape(t *testing.T) {
	ds := BuildDataset()
	if got := len(ds.Recovery); got != 84 {
		t.Errorf("recovery specs = %d, want 84", got)
	}
	if got := len(ds.Activities); got < 30 {
		t.Errorf("activity specs = %d, want >= 30", got)
	}
	if got := len(ds.Streams); got < 5 {
		t.Errorf("stream specs = %d, want >= 5", got)
	}
	if got := len(ds.Decisions); got != 14 {
		t.Errorf("decision specs = %d, want 14", got)
	}
	if got := len(ds.PlanWeek.Days); got != 7 {
		t.Errorf("plan days = %d, want 7", got)
	}
	// Every streamed run must reference a real activity (FK on activity_id).
	haveAct := map[int]bool{}
	for _, a := range ds.Activities {
		haveAct[a.DayOffset] = true
	}
	for _, s := range ds.Streams {
		if !haveAct[s.ActivityDayOffset] {
			t.Errorf("stream references missing activity at offset %d", s.ActivityDayOffset)
		}
	}
}

// TestFixtureNoRunsOnRedRestDays guards the readiness story: a red REST_DAY decision
// must never share a day with a seeded run (an overreached athlete rested), so the
// Trends/activities view never shows a hard run on a prescribed rest day.
func TestFixtureNoRunsOnRedRestDays(t *testing.T) {
	ds := BuildDataset()
	rest := map[int]bool{}
	for _, d := range ds.Decisions {
		if d.Color == "red" || d.Action == "REST_DAY" {
			rest[d.DayOffset] = true
		}
	}
	for _, a := range ds.Activities {
		if rest[a.DayOffset] {
			t.Errorf("run %q at day offset %d lands on a red REST_DAY — contradicts the readiness story", a.Name, a.DayOffset)
		}
	}
}
