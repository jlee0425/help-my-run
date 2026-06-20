package coach

import (
	"context"
	"errors"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/readiness"
)

// failRunner returns an is_error envelope (the fallback path trigger).
type failRunner struct{ calls int }

func (r *failRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	r.calls++
	return []byte(`{"type":"result","is_error":true,"result":"please login"}`), nil
}

const adjustEnv = `{"type":"result","subtype":"success","is_error":false,"result":"{\"action\":\"SOFTEN\",\"adjusted_session\":{\"date\":\"2026-06-20\",\"dow\":\"Fri\",\"run_type\":\"easy\",\"distance_km\":4.5,\"pace_target\":\"6:00/km\",\"time_note\":\"~20:00 after CrossFit\",\"optional_if_cns\":true,\"rationale\":\"trimmed\"},\"rationale\":\"HRV down, sleep short\"}"}`

func TestAdjustTodayAIPath(t *testing.T) {
	s := newCoachStore(t)
	r := &captureRunner{out: []byte(adjustEnv)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	rd := readiness.Readiness{Color: readiness.ColorAmber, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"}

	dec, raw, source, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if source != "ai" {
		t.Errorf("source = %q, want ai", source)
	}
	if dec.Action != llm.ActionSoften || dec.AdjustedSession == nil || dec.AdjustedSession.DistanceKm != 4.5 {
		t.Errorf("decision = %+v", dec)
	}
	if raw == "" {
		t.Error("raw empty, want re-marshaled decision JSON for ai source")
	}
	if !strings.Contains(r.body, `"readiness"`) || !strings.Contains(r.body, `"today_session"`) {
		t.Errorf("stdin missing readiness/today_session: %s", r.body)
	}
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "SINGLE-DAY") {
		t.Errorf("args missing daily-adjust prompt: %v", r.args)
	}
}

func TestAdjustTodayFallbackOnFailure(t *testing.T) {
	s := newCoachStore(t)
	r := &failRunner{}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	rd := readiness.Readiness{Color: readiness.ColorRed, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 8, PaceTarget: "5:05/km"}

	dec, raw, source, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday fallback returned error = %v, want nil", err)
	}
	if source != "fallback" {
		t.Errorf("source = %q, want fallback", source)
	}
	if raw != "" {
		t.Errorf("raw = %q, want empty on fallback", raw)
	}
	if dec.Action != llm.ActionMove {
		t.Errorf("action = %q, want MOVE", dec.Action)
	}
	if dec.AdjustedSession == nil || dec.AdjustedSession.RunType != "recovery" || dec.AdjustedSession.DistanceKm != 4 {
		t.Errorf("adjusted = %+v, want recovery 4km", dec.AdjustedSession)
	}
	if !dec.AdjustedSession.OptionalIfCNS {
		t.Error("adjusted optional_if_cns = false, want true on RED move")
	}
}

func TestAdjustTodayFallbackNoRunRestDay(t *testing.T) {
	s := newCoachStore(t)
	c := New(s, &llm.Client{Runner: &failRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	rd := readiness.Readiness{Color: readiness.ColorGreen, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}

	dec, _, source, err := c.AdjustToday(context.Background(), "2026-06-20", rd, nil)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if source != "fallback" {
		t.Errorf("source = %q, want fallback", source)
	}
	if dec.Action != llm.ActionRestDay || dec.AdjustedSession != nil {
		t.Errorf("decision = %+v, want REST_DAY/nil", dec)
	}
}

func TestAdjustTodayFallbackAmberQualitySoften(t *testing.T) {
	s := newCoachStore(t)
	c := New(s, &llm.Client{Runner: &failRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	rd := readiness.Readiness{Color: readiness.ColorAmber, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"}

	dec, _, _, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if dec.Action != llm.ActionSoften {
		t.Errorf("action = %q, want SOFTEN", dec.Action)
	}
	if dec.AdjustedSession == nil || dec.AdjustedSession.DistanceKm != 4.5 {
		t.Errorf("adjusted distance = %v, want 4.5", dec.AdjustedSession)
	}
}

func TestAdjustTodayFallbackGreenStand(t *testing.T) {
	s := newCoachStore(t)
	c := New(s, &llm.Client{Runner: &failRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	rd := readiness.Readiness{Color: readiness.ColorGreen, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"}

	dec, _, _, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if dec.Action != llm.ActionStand {
		t.Errorf("action = %q, want STAND", dec.Action)
	}
	if dec.AdjustedSession == nil || dec.AdjustedSession.DistanceKm != 6 {
		t.Errorf("adjusted = %+v, want unchanged 6km", dec.AdjustedSession)
	}
}

var _ = errors.Is // keep errors import live if unused above
