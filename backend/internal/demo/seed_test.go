package demo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/store"
)

// fixedNow is a deterministic seed time (Saturday 2026-07-11 07:00 local); its
// ISO week Monday is 2026-07-06.
func fixedNow() time.Time {
	return time.Date(2026, 7, 11, 7, 0, 0, 0, time.Local)
}

// seededAt opens a migrated in-memory store and seeds the demo dataset at `now`.
func seededAt(t *testing.T, now time.Time) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open(:memory:) error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := Seed(s, now); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	return s
}

// newSeededStore opens a migrated in-memory store and seeds the demo dataset.
func newSeededStore(t *testing.T) *store.Store {
	t.Helper()
	return seededAt(t, fixedNow())
}

func TestSeed_Activities(t *testing.T) {
	s := newSeededStore(t)
	acts, err := s.ListActivities(500)
	if err != nil {
		t.Fatalf("ListActivities error = %v", err)
	}
	if len(acts) < 30 {
		t.Fatalf("activities = %d, want >= 30", len(acts))
	}
}

func TestSeed_RecoveryDays(t *testing.T) {
	s := newSeededStore(t)
	n, err := s.CountRecoveryDays()
	if err != nil {
		t.Fatalf("CountRecoveryDays error = %v", err)
	}
	if n != 84 {
		t.Fatalf("recovery days = %d, want 84", n)
	}
	rec, err := s.ListRecovery(200)
	if err != nil {
		t.Fatalf("ListRecovery error = %v", err)
	}
	if len(rec) != 84 {
		t.Fatalf("ListRecovery len = %d, want 84", len(rec))
	}
}

func TestSeed_TodayDecisionIsAmberSoften(t *testing.T) {
	s := newSeededStore(t)
	today := fixedNow().Format("2006-01-02")
	dec, err := s.GetDailyDecision(today)
	if err != nil {
		t.Fatalf("GetDailyDecision(%s) error = %v", today, err)
	}
	if dec.ReadinessColor != "amber" {
		t.Errorf("today color = %q, want amber", dec.ReadinessColor)
	}
	if dec.Action != "SOFTEN" {
		t.Errorf("today action = %q, want SOFTEN", dec.Action)
	}
	if dec.OriginalSessionJSON == nil || dec.AdjustedSessionJSON == nil {
		t.Errorf("today decision must carry original + adjusted sessions")
	}
	if dec.Source != "ai" {
		t.Errorf("today source = %q, want ai", dec.Source)
	}
}

func TestSeed_PlanForCurrentWeek(t *testing.T) {
	s := newSeededStore(t)
	weekStart := "2026-07-06" // Monday of the fixed now
	p, err := s.GetLatestPlan(weekStart)
	if err != nil {
		t.Fatalf("GetLatestPlan(%s) error = %v", weekStart, err)
	}
	if p.PlanJSON == "" {
		t.Errorf("plan_json empty")
	}
}

func TestSeed_StreamAnalyses(t *testing.T) {
	s := newSeededStore(t)
	sa, err := s.ListStreamAnalyses(50)
	if err != nil {
		t.Fatalf("ListStreamAnalyses error = %v", err)
	}
	if len(sa) < 5 {
		t.Fatalf("stream analyses = %d, want >= 5", len(sa))
	}
}

func TestSeed_Chat(t *testing.T) {
	s := newSeededStore(t)
	msgs, err := s.ListChatMessages(100)
	if err != nil {
		t.Fatalf("ListChatMessages error = %v", err)
	}
	if len(msgs) < 4 {
		t.Fatalf("chat messages = %d, want >= 4", len(msgs))
	}
}

func TestSeed_Profile(t *testing.T) {
	s := newSeededStore(t)
	p, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile error = %v", err)
	}
	if p.TargetWeeklyKm <= 0 {
		t.Errorf("target_weekly_km = %v, want > 0", p.TargetWeeklyKm)
	}
	if !p.AgentEnabled {
		t.Errorf("agent_enabled = false, want true")
	}
}

// TestSeed_TodayCenterpieceHoldsEveryWeekday seeds the demo booting on each of
// the 7 weekdays and asserts today's decision is present, amber, a SOFTEN, and
// that the context its rationale cites — "yesterday's CrossFit leg day" — actually
// exists in the seeded CrossFit data. The Monday boot is the edge case: yesterday
// is the PREVIOUS week's Sunday, which must still carry the leg day.
func TestSeed_TodayCenterpieceHoldsEveryWeekday(t *testing.T) {
	base := time.Date(2026, 7, 6, 7, 0, 0, 0, time.Local) // Monday
	for i := 0; i < 7; i++ {
		now := base.AddDate(0, 0, i)
		t.Run(now.Format("Mon"), func(t *testing.T) {
			s := seededAt(t, now)
			today := now.Format("2006-01-02")

			dec, err := s.GetDailyDecision(today)
			if err != nil {
				t.Fatalf("today decision missing: %v", err)
			}
			if dec.ReadinessColor != "amber" {
				t.Errorf("today color = %q, want amber", dec.ReadinessColor)
			}
			if dec.Action != "SOFTEN" {
				t.Errorf("today action = %q, want SOFTEN", dec.Action)
			}
			if !strings.Contains(dec.Rationale, "CrossFit leg day") {
				t.Fatalf("rationale does not cite the CrossFit leg day: %q", dec.Rationale)
			}

			// The cited context must exist: yesterday is a heavy-leg CrossFit day.
			yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
			wk, err := s.GetCrossFitWeek(mondayOfDate(yesterday))
			if err != nil {
				t.Fatalf("crossfit week for yesterday (%s) missing: %v", yesterday, err)
			}
			var parsed llm.CrossFitWeekParsed
			if err := json.Unmarshal([]byte(wk.ParsedJSON), &parsed); err != nil {
				t.Fatalf("parse crossfit week: %v", err)
			}
			legDay := false
			for _, d := range parsed.Days {
				if d.Date == yesterday && d.HasCrossFit && d.LegLoad == llm.LoadHigh {
					legDay = true
				}
			}
			if !legDay {
				t.Errorf("yesterday %s is not a heavy-leg CrossFit day — rationale cites a context that does not exist", yesterday)
			}
		})
	}
}

// TestSeed_PlanSumsToTargetEveryWeekday asserts the materialized plan's run
// distances sum to the 30 km target (with the long run the biggest single
// session and today shown as the planned tempo) regardless of boot weekday — the
// Plan page must not contradict its own "TARGET 30 KM".
func TestSeed_PlanSumsToTargetEveryWeekday(t *testing.T) {
	base := time.Date(2026, 7, 6, 7, 0, 0, 0, time.Local) // Monday
	for i := 0; i < 7; i++ {
		now := base.AddDate(0, 0, i)
		t.Run(now.Format("Mon"), func(t *testing.T) {
			s := seededAt(t, now)
			weekStart := mondayOfTime(now)
			p, err := s.GetLatestPlan(weekStart)
			if err != nil {
				t.Fatalf("GetLatestPlan(%s) error = %v", weekStart, err)
			}
			var plan llm.PlanParsed
			if err := json.Unmarshal([]byte(p.PlanJSON), &plan); err != nil {
				t.Fatalf("parse plan: %v", err)
			}
			var sum, longest float64
			var nLong int
			var todayType string
			today := now.Format("2006-01-02")
			for _, d := range plan.Days {
				sum += d.DistanceKm
				if d.DistanceKm > longest {
					longest = d.DistanceKm
				}
				if d.RunType == "long" {
					nLong++
				}
				if d.Date == today {
					todayType = d.RunType
				}
			}
			if sum != plan.WeeklyTargetKm {
				t.Errorf("plan distances sum to %.0f km, want %.0f (weekly target)", sum, plan.WeeklyTargetKm)
			}
			if nLong != 1 {
				t.Errorf("plan has %d long runs, want exactly 1", nLong)
			}
			if longest != 14 {
				t.Errorf("biggest session = %.0f km, want the 14 km long run", longest)
			}
			if todayType != "tempo" {
				t.Errorf("today's plan row = %q, want the planned tempo the Today page softens", todayType)
			}
		})
	}
}

// TestSeed_PaceAtHRTrendImproves verifies the coach's claim — easy pace ≈6:35→6:20
// per km at the same ~140 bpm — against what the Trends/progress engine actually
// computes from the seeded activities, so Coach and Trends agree.
func TestSeed_PaceAtHRTrendImproves(t *testing.T) {
	s := newSeededStore(t)
	now := fixedNow()

	acts, err := s.ListActivities(500)
	if err != nil {
		t.Fatalf("ListActivities error = %v", err)
	}
	rec, err := s.ListRecovery(600)
	if err != nil {
		t.Fatalf("ListRecovery error = %v", err)
	}
	vo2, err := s.ListVo2max(600)
	if err != nil {
		t.Fatalf("ListVo2max error = %v", err)
	}
	saRows, err := s.ListStreamAnalyses(500)
	if err != nil {
		t.Fatalf("ListStreamAnalyses error = %v", err)
	}
	prof, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile error = %v", err)
	}
	startByID := map[int64]string{}
	for _, a := range acts {
		startByID[a.ActivityID] = a.StartTime
	}
	var streamPts []progress.StreamAnalysisPoint
	for _, r := range saRows {
		if st, ok := startByID[r.ActivityID]; ok {
			streamPts = append(streamPts, progress.StreamAnalysisPoint{StartTime: st, DecouplingPct: r.DecouplingPct})
		}
	}

	rep := progress.ComputeProgress(acts, rec, vo2, streamPts, prof, progress.DefaultWeeks, now)
	sig := findSignal(t, rep, progress.SignalPaceAtHR)
	if sig.Direction != progress.DirectionDown {
		t.Errorf("pace@HR direction = %q, want down (improving) — Trends contradicts the coach", sig.Direction)
	}
	if sig.Baseline == nil || sig.Current == nil {
		t.Fatalf("pace@HR baseline/current nil — no in-band trend computed")
	}
	if *sig.Baseline < 390 || *sig.Baseline > 398 { // ~6:35/km = 395 s/km
		t.Errorf("pace@HR baseline = %.1f s/km, want ~395 (6:35/km)", *sig.Baseline)
	}
	if *sig.Current < 377 || *sig.Current > 383 { // ~6:20/km = 380 s/km
		t.Errorf("pace@HR current = %.1f s/km, want ~380 (6:20/km)", *sig.Current)
	}

	// Long-run efficiency claim: decoupling holds under 5% in the most recent week.
	dec := findSignal(t, rep, progress.SignalDecoupling)
	if dec.Current != nil && *dec.Current >= 5 {
		t.Errorf("current decoupling = %.1f%%, want < 5%% (claimed by the coach)", *dec.Current)
	}
}

func findSignal(t *testing.T, rep progress.ProgressReport, key string) progress.TrendSummary {
	t.Helper()
	for _, s := range rep.Signals {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("signal %q not found in progress report", key)
	return progress.TrendSummary{}
}

// TestSeed_Idempotent verifies a second Seed does not error and does not
// duplicate the append-only rows (plan, chat).
func TestSeed_Idempotent(t *testing.T) {
	s := newSeededStore(t)
	if err := Seed(s, fixedNow()); err != nil {
		t.Fatalf("second Seed() error = %v", err)
	}
	// Plans are append-only (InsertPlan); Seed must guard against a duplicate.
	var planCount int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM plans WHERE week_start = ?`, "2026-07-06").Scan(&planCount); err != nil {
		t.Fatalf("count plans error = %v", err)
	}
	if planCount != 1 {
		t.Errorf("plans for week = %d, want 1 after double seed", planCount)
	}
	msgs, err := s.ListChatMessages(100)
	if err != nil {
		t.Fatalf("ListChatMessages error = %v", err)
	}
	if len(msgs) != 4 {
		t.Errorf("chat messages = %d, want 4 after double seed", len(msgs))
	}
}
