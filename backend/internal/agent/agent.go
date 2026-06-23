// Package agent orchestrates the daily M2 loop: sync -> readiness -> load today's
// session -> coach.AdjustToday (or readiness-only) -> persist -> push, guarded by
// a persistent once-per-day idempotency key (agent_runs.last_run_date).
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
	"help-my-run/backend/internal/store"
	syncpkg "help-my-run/backend/internal/sync"
)

// Syncer is the sync seam (real impl wraps sync.SyncAll with bound deps).
type Syncer interface {
	SyncAll(ctx context.Context) syncpkg.AllResult
}

// Adjuster is the coach seam used by the agent (*coach.Coach satisfies it).
type Adjuster interface {
	AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error)
	Fitness(ctx context.Context) (metrics.FitnessMetrics, error)
}

// Pusher is the push seam (*push.Client satisfies it).
type Pusher interface {
	Send(ctx context.Context, msg push.Message) error
}

// Clock is the agent's injectable time source (RFC3339 stamps in tests).
type Clock interface {
	Now() time.Time
}

// RunResult is the outcome of one daily agent run.
type RunResult struct {
	Date           string          `json:"date"`
	Skipped        bool            `json:"skipped"`
	ReadinessColor readiness.Color `json:"readiness_color"`
	Action         llm.DailyAction `json:"action"`
	Source         string          `json:"source"`
	Stale          bool            `json:"stale"`
	Pushed         bool            `json:"pushed"`
	Error          *string         `json:"error"`
}

// Agent owns the daily loop + persistent idempotency guard.
type Agent struct {
	store    *store.Store
	syncer   Syncer
	adjuster Adjuster
	pusher   Pusher
	clock    Clock
	loc      *time.Location
}

// New constructs an Agent. loc defaults to UTC if nil.
func New(s *store.Store, syncer Syncer, adjuster Adjuster, pusher Pusher, clock Clock, loc *time.Location) *Agent {
	if loc == nil {
		loc = time.UTC
	}
	return &Agent{store: s, syncer: syncer, adjuster: adjuster, pusher: pusher, clock: clock, loc: loc}
}

// mondayOf returns the Monday (YYYY-MM-DD) of the ISO week containing localDate.
func mondayOf(localDate string) (string, error) {
	d, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return "", err
	}
	off := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -off).Format("2006-01-02"), nil
}

// todaySession finds the PlanDay matching localDate in the latest plan for its
// week; returns (nil, "", nil) when no plan or no matching run day.
func (a *Agent) todaySession(localDate string) (*llm.PlanDay, string, error) {
	weekStart, err := mondayOf(localDate)
	if err != nil {
		return nil, "", err
	}
	p, err := a.store.GetLatestPlan(weekStart)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", nil
		}
		return nil, "", err
	}
	var parsed llm.PlanParsed
	if uerr := json.Unmarshal([]byte(p.PlanJSON), &parsed); uerr != nil {
		return nil, "", uerr
	}
	for i := range parsed.Days {
		if parsed.Days[i].Date == localDate && parsed.Days[i].RunType != "rest" {
			day := parsed.Days[i]
			return &day, parsed.WeekRationale, nil
		}
	}
	return nil, parsed.WeekRationale, nil
}

// RunDaily is the scheduler callback. It checks agent_runs.last_run_date for
// idempotency, runs the §5 flow, persists daily_decisions + agent_runs, and
// pushes the briefing. localDate is YYYY-MM-DD in the configured tz.
func (a *Agent) RunDaily(ctx context.Context, localDate string) RunResult {
	res := RunResult{Date: localDate}

	if _, err := a.store.GetAgentRun(localDate); err == nil {
		res.Skipped = true
		return res
	} else if !errors.Is(err, store.ErrNotFound) {
		return a.recordErr(localDate, res, err)
	}

	sr := a.syncer.SyncAll(ctx)
	res.Stale = sr.Garmin.Status != "ok"

	rec, err := a.store.ListRecovery(60)
	if err != nil {
		return a.recordErr(localDate, res, err)
	}
	rd := readiness.Assess(rec, a.clock.Now())
	res.ReadinessColor = rd.Color

	today, _, err := a.todaySession(localDate)
	if err != nil {
		return a.recordErr(localDate, res, err)
	}

	now := a.clock.Now().UTC().Format(time.RFC3339)
	dd := store.DailyDecision{
		Date:           localDate,
		ReadinessColor: string(rd.Color),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	dd.DriversJSON = marshalDriversWithReasons(rd, res.Stale)

	var decision llm.DailyDecisionParsed
	if today == nil {
		decision = llm.DailyDecisionParsed{Action: llm.ActionRestDay, Rationale: restRationale(rd.Color)}
		res.Source = "fallback" // matches daily_decisions.source + the TS RunResult ai|fallback union
		dd.Source = "fallback"
		dd.Action = string(decision.Action)
		dd.Rationale = decision.Rationale
	} else {
		dec, raw, source, aerr := a.adjuster.AdjustToday(ctx, localDate, rd, today)
		if aerr != nil {
			return a.recordErr(localDate, res, aerr)
		}
		decision = dec
		res.Source = source
		dd.Source = source
		dd.Action = string(dec.Action)
		dd.Rationale = dec.Rationale
		if oj, oerr := json.Marshal(today); oerr == nil {
			dd.OriginalSessionJSON = strPtr(string(oj))
		}
		if dec.AdjustedSession != nil {
			if aj, aerr2 := json.Marshal(dec.AdjustedSession); aerr2 == nil {
				dd.AdjustedSessionJSON = strPtr(string(aj))
			}
		}
		if source == "ai" && raw != "" {
			dd.RawResponse = strPtr(raw)
		}
	}
	res.Action = decision.Action

	if perr := a.store.UpsertDailyDecision(dd); perr != nil {
		return a.recordErr(localDate, res, perr)
	}

	res.Pushed = a.pushBriefing(ctx, localDate, rd.Color, decision)

	_ = a.store.UpsertAgentRun(store.AgentRun{
		LastRunDate: localDate, Status: "ok", RanAt: now,
	})
	return res
}

// marshalDriversWithReasons serializes ReadinessDrivers with the readiness Reasons
// AND the run's staleness embedded as sibling keys ("reasons", "stale") so the API
// can surface both without a schema change. Falls back to plain drivers JSON on any
// error. The DTO/parseReasons in today_handlers.go ignore unknown driver keys and
// read these siblings back.
func marshalDriversWithReasons(rd readiness.Readiness, stale bool) string {
	dj, err := json.Marshal(rd.Drivers)
	if err != nil {
		return "{}"
	}
	var m map[string]interface{}
	if uerr := json.Unmarshal(dj, &m); uerr != nil {
		return string(dj)
	}
	m["reasons"] = rd.Reasons
	m["stale"] = stale
	out, merr := json.Marshal(m)
	if merr != nil {
		return string(dj)
	}
	return string(out)
}

// pushBriefing sends to every registered device token; returns true if >=1 ok.
// A DeviceNotRegistered token is deleted. Returns false (not fatal) on errors.
func (a *Agent) pushBriefing(ctx context.Context, localDate string, color readiness.Color, dec llm.DailyDecisionParsed) bool {
	tokens, err := a.store.ListDeviceTokens()
	if err != nil || len(tokens) == 0 {
		return false
	}
	body := dec.Rationale
	if body == "" {
		body = restRationale(color)
	}
	title := fmt.Sprintf("%s — %s", localDate, string(color))
	delivered := false
	for _, tk := range tokens {
		msg := push.Message{
			To:        tk.ExpoPushToken,
			Title:     title,
			Body:      body,
			Data:      map[string]interface{}{"date": localDate, "action": string(dec.Action)},
			Sound:     "default",
			Priority:  "high",
			ChannelID: "default",
		}
		if serr := a.pusher.Send(ctx, msg); serr != nil {
			if errors.Is(serr, push.ErrDeviceNotRegistered) {
				_ = a.store.DeleteDeviceToken(tk.ExpoPushToken)
			}
			log.Printf("agent: push to %s failed: %v", tk.ExpoPushToken, serr)
			continue
		}
		delivered = true
	}
	return delivered
}

// recordErr stores an error agent_run and returns res with the error set.
func (a *Agent) recordErr(localDate string, res RunResult, err error) RunResult {
	msg := err.Error()
	res.Error = &msg
	now := a.clock.Now().UTC().Format(time.RFC3339)
	_ = a.store.UpsertAgentRun(store.AgentRun{
		LastRunDate: localDate, Status: "error", Error: &msg, RanAt: now,
	})
	log.Printf("agent.RunDaily(%s): %v", localDate, err)
	return res
}

func restRationale(color readiness.Color) string {
	switch color {
	case readiness.ColorRed:
		return "Rest day — readiness low, stay recovered."
	case readiness.ColorAmber:
		return "Rest day."
	default:
		return "Rest day as planned; you're well recovered."
	}
}

func strPtr(s string) *string { return &s }
