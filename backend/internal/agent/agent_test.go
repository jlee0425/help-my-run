package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
	"help-my-run/backend/internal/store"
	syncpkg "help-my-run/backend/internal/sync"
)

func newAgentStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeSyncer struct {
	res    syncpkg.AllResult
	called int
}

func (f *fakeSyncer) SyncAll(ctx context.Context) syncpkg.AllResult {
	f.called++
	return f.res
}

type fakeAdjuster struct {
	dec    llm.DailyDecisionParsed
	raw    string
	source string
	err    error
	fit    metrics.FitnessMetrics
	called int
	gotRd  readiness.Readiness
	gotDay *llm.PlanDay
}

func (f *fakeAdjuster) AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error) {
	f.called++
	f.gotRd = rd
	f.gotDay = today
	return f.dec, f.raw, f.source, f.err
}

func (f *fakeAdjuster) Fitness(ctx context.Context) (metrics.FitnessMetrics, error) {
	return f.fit, nil
}

type fakePusher struct {
	msgs []push.Message
	err  error
}

func (f *fakePusher) Send(ctx context.Context, msg push.Message) error {
	f.msgs = append(f.msgs, msg)
	return f.err
}

func okSync() syncpkg.AllResult {
	return syncpkg.AllResult{
		Strava: syncpkg.SourceResult{Status: "ok"},
		Garmin: syncpkg.SourceResult{Status: "ok"},
	}
}

func seedPlanWithToday(t *testing.T, s *store.Store, weekStart, date string) {
	t.Helper()
	plan := llm.PlanParsed{
		WeeklyTargetKm: 20,
		Days: []llm.PlanDay{
			{Date: date, Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"},
		},
		WeekRationale: "build",
	}
	b, _ := json.Marshal(plan)
	if _, err := s.InsertPlan(store.Plan{
		WeekStart: weekStart, GeneratedAt: "2026-06-19T00:00:00Z", Status: "ok",
		PlanJSON: string(b), FitnessSummary: "x", Model: "m",
	}); err != nil {
		t.Fatalf("InsertPlan: %v", err)
	}
	if err := s.UpsertDeviceToken(store.DeviceToken{
		ExpoPushToken: "ExponentPushToken[x]", Platform: "ios", UpdatedAt: "2026-06-19T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertDeviceToken: %v", err)
	}
}

func TestRunDailyHappyPathPersistsAndPushes(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")

	adj := &fakeAdjuster{
		dec: llm.DailyDecisionParsed{
			Action:          llm.ActionSoften,
			AdjustedSession: &llm.PlanDay{Date: "2026-06-19", Dow: "Fri", RunType: "easy", DistanceKm: 4.5, PaceTarget: "6:00/km"},
			Rationale:       "trimmed",
		},
		raw: `{"action":"SOFTEN"}`, source: "ai",
	}
	pu := &fakePusher{}
	a := New(s, &fakeSyncer{res: okSync()}, adj, pu, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)

	res := a.RunDaily(context.Background(), "2026-06-19")
	if res.Skipped {
		t.Fatalf("res.Skipped = true, want false")
	}
	if res.Action != llm.ActionSoften || res.Source != "ai" || !res.Pushed {
		t.Errorf("res = %+v", res)
	}
	if adj.gotDay == nil || adj.gotDay.RunType != "tempo" {
		t.Errorf("adjuster got today = %+v, want tempo from plan", adj.gotDay)
	}
	d, err := s.GetDailyDecision("2026-06-19")
	if err != nil {
		t.Fatalf("GetDailyDecision: %v", err)
	}
	if d.Action != "SOFTEN" || d.Source != "ai" || d.AdjustedSessionJSON == nil {
		t.Errorf("stored decision = %+v", d)
	}
	if d.OriginalSessionJSON == nil {
		t.Error("original_session_json nil, want the plan's tempo day")
	}
	run, err := s.GetAgentRun("2026-06-19")
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if run.Status != "ok" {
		t.Errorf("agent_run status = %q, want ok", run.Status)
	}
	if len(pu.msgs) != 1 || pu.msgs[0].To != "ExponentPushToken[x]" {
		t.Errorf("push msgs = %+v", pu.msgs)
	}
}

func TestRunDailyFallbackSource(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{
		dec: llm.DailyDecisionParsed{Action: llm.ActionMove, AdjustedSession: &llm.PlanDay{Date: "2026-06-19", RunType: "recovery", DistanceKm: 4}, Rationale: "fb"},
		raw: "", source: "fallback",
	}
	a := New(s, &fakeSyncer{res: okSync()}, adj, &fakePusher{}, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)
	res := a.RunDaily(context.Background(), "2026-06-19")
	if res.Source != "fallback" || res.Action != llm.ActionMove {
		t.Errorf("res = %+v", res)
	}
	d, _ := s.GetDailyDecision("2026-06-19")
	if d.Source != "fallback" || d.RawResponse != nil {
		t.Errorf("stored = %+v, want fallback/nil raw", d)
	}
}

func TestRunDailyNoRunTodayReadinessOnly(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{}
	a := New(s, &fakeSyncer{res: okSync()}, adj, &fakePusher{}, fakeClock{now: time.Date(2026, 6, 21, 5, 30, 0, 0, time.UTC)}, time.UTC)

	res := a.RunDaily(context.Background(), "2026-06-21")
	if res.Skipped {
		t.Fatalf("skipped, want a run")
	}
	if res.Action != llm.ActionRestDay {
		t.Errorf("action = %q, want REST_DAY (no run today)", res.Action)
	}
	if adj.called != 0 {
		t.Errorf("adjuster called %d times, want 0 on no-run day", adj.called)
	}
	d, err := s.GetDailyDecision("2026-06-21")
	if err != nil {
		t.Fatalf("GetDailyDecision: %v", err)
	}
	if d.Action != "REST_DAY" || d.OriginalSessionJSON != nil || d.AdjustedSessionJSON != nil {
		t.Errorf("stored = %+v, want REST_DAY/nil sessions", d)
	}
}

func TestRunDailyIdempotentSecondRunSkips(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{
		dec:    llm.DailyDecisionParsed{Action: llm.ActionStand, AdjustedSession: &llm.PlanDay{Date: "2026-06-19", RunType: "tempo", DistanceKm: 6}},
		source: "ai",
	}
	syncer := &fakeSyncer{res: okSync()}
	pu := &fakePusher{}
	a := New(s, syncer, adj, pu, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)

	first := a.RunDaily(context.Background(), "2026-06-19")
	if first.Skipped {
		t.Fatal("first run skipped, want full run")
	}
	second := a.RunDaily(context.Background(), "2026-06-19")
	if !second.Skipped {
		t.Errorf("second run skipped = false, want true (idempotency)")
	}
	if adj.called != 1 || syncer.called != 1 || len(pu.msgs) != 1 {
		t.Errorf("second run was not a no-op: adj=%d sync=%d push=%d", adj.called, syncer.called, len(pu.msgs))
	}
}

func TestRunDailyStaleWhenSyncFails(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{
		dec:    llm.DailyDecisionParsed{Action: llm.ActionStand, AdjustedSession: &llm.PlanDay{Date: "2026-06-19", RunType: "tempo", DistanceKm: 6}},
		source: "ai",
	}
	bad := syncpkg.AllResult{
		Strava: syncpkg.SourceResult{Status: "ok"},
		Garmin: syncpkg.SourceResult{Status: "error"},
	}
	a := New(s, &fakeSyncer{res: bad}, adj, &fakePusher{}, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)
	res := a.RunDaily(context.Background(), "2026-06-19")
	if !res.Stale {
		t.Errorf("res.Stale = false, want true when garmin sync errored")
	}
}
