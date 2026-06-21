package api

import (
	"context"

	"help-my-run/backend/internal/agent"
	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
)

// fakeAgent is the injected api.Agent for handler tests.
type fakeAgent struct {
	result    agent.RunResult
	lastDate  string
	lastForce bool
}

func (f *fakeAgent) RunDaily(ctx context.Context, localDate string, force bool) agent.RunResult {
	f.lastDate = localDate
	f.lastForce = force
	if f.result.Date == "" {
		f.result.Date = localDate
	}
	return f.result
}

// fakePusher is the injected api.Pusher for handler tests.
type fakePusher struct {
	sent    []push.Message
	sendErr error
}

func (f *fakePusher) Send(ctx context.Context, msg push.Message) error {
	f.sent = append(f.sent, msg)
	return f.sendErr
}

// fakeProgress is the injected api.Progress for handler tests.
type fakeProgress struct {
	report     progress.ProgressReport
	read       progress.ProgressRead
	reportErr  error
	analyzeErr error
	lastWeeks  int
}

func (f *fakeProgress) Report(ctx context.Context, weeks int) (progress.ProgressReport, error) {
	f.lastWeeks = weeks
	if f.reportErr != nil {
		return progress.ProgressReport{}, f.reportErr
	}
	if f.report.Weeks == 0 {
		f.report.Weeks = weeks
	}
	return f.report, nil
}

func (f *fakeProgress) Analyze(ctx context.Context, weeks int) (progress.ProgressRead, error) {
	f.lastWeeks = weeks
	if f.analyzeErr != nil {
		return progress.ProgressRead{}, f.analyzeErr
	}
	return f.read, nil
}

// Compile-time interface conformance checks (the RED-state assertions).
var (
	_ Coach    = (*fakeCoach)(nil)
	_ Agent    = (*fakeAgent)(nil)
	_ Pusher   = (*fakePusher)(nil)
	_ Progress = (*fakeProgress)(nil)
)

var _ = readiness.ColorGreen
var _ llm.DailyAction = llm.ActionStand
