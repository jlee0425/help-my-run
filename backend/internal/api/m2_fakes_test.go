package api

import (
	"context"

	"help-my-run/backend/internal/agent"
	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/streams"
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

// fakeStreams is the injected api.Streams for handler tests.
type fakeStreams struct {
	analysis    streams.StreamAnalysis
	getErr      error
	fetchErr    error
	lastGetID   int64
	lastFetchID int64
}

func (f *fakeStreams) GetOrComputeAnalysis(ctx context.Context, activityID int64) (streams.StreamAnalysis, error) {
	f.lastGetID = activityID
	if f.getErr != nil {
		return streams.StreamAnalysis{}, f.getErr
	}
	return f.analysis, nil
}

func (f *fakeStreams) FetchAndAnalyze(ctx context.Context, activityID int64) (streams.StreamAnalysis, error) {
	f.lastFetchID = activityID
	if f.fetchErr != nil {
		return streams.StreamAnalysis{}, f.fetchErr
	}
	return f.analysis, nil
}

// fakeChat is the injected api.Chat for handler tests.
type fakeChat struct {
	msg       store.ChatMessage
	answerErr error
	lastMsg   string
}

func (f *fakeChat) Answer(ctx context.Context, message string) (store.ChatMessage, error) {
	f.lastMsg = message
	if f.answerErr != nil {
		return store.ChatMessage{}, f.answerErr
	}
	if f.msg.Role == "" {
		f.msg = store.ChatMessage{Role: "assistant", Content: "ok", CreatedAt: "2026-06-22T09:14:02Z"}
	}
	return f.msg, nil
}

// Compile-time interface conformance checks (the RED-state assertions).
var (
	_ Coach    = (*fakeCoach)(nil)
	_ Agent    = (*fakeAgent)(nil)
	_ Pusher   = (*fakePusher)(nil)
	_ Progress = (*fakeProgress)(nil)
	_ Streams  = (*fakeStreams)(nil)
	_ Chat     = (*fakeChat)(nil)
)

var _ = readiness.ColorGreen
var _ llm.DailyAction = llm.ActionStand
