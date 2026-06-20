package api

import (
	"context"

	"help-my-run/backend/internal/agent"
	"help-my-run/backend/internal/llm"
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

// Compile-time interface conformance checks (the RED-state assertions).
var (
	_ Coach  = (*fakeCoach)(nil)
	_ Agent  = (*fakeAgent)(nil)
	_ Pusher = (*fakePusher)(nil)
)

var _ = readiness.ColorGreen
var _ llm.DailyAction = llm.ActionStand
