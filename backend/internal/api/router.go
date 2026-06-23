package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"help-my-run/backend/internal/agent"
	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/streams"
)

// SyncFunc runs the Garmin sync and returns the flattened result:
// (status, synced, err). Wiring (main.go) adapts the sync package to this
// signature so the api package does not import sync (avoids an import cycle).
type SyncFunc func(ctx context.Context) (string, int, *string)

// Coach is the M1 plan-engine seam, injected from main.go (avoids an import
// cycle: api must not import coach). Extended in M2 with AdjustToday.
// *coach.Coach satisfies it structurally.
type Coach interface {
	ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, string, error)
	GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, string, string, error)
	Fitness(ctx context.Context) (metrics.FitnessMetrics, error)
	AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error)
}

// Agent is the M2 daily-loop seam. *agent.Agent satisfies it structurally (after
// the force-capable RunDaily is wired — see Task 26).
type Agent interface {
	RunDaily(ctx context.Context, localDate string, force bool) agent.RunResult
}

// Pusher is the M2 push transport seam. *push.Client satisfies it structurally.
type Pusher interface {
	Send(ctx context.Context, msg push.Message) error
}

// Progress is the M3.1 progress-engine seam, injected from main.go (avoids an
// import cycle: api must not import the concrete progress.Engine). *progress.Engine
// satisfies it structurally. Report builds the deterministic trends; Analyze adds
// the claude -p read with deterministic fallback.
type Progress interface {
	Report(ctx context.Context, weeks int) (progress.ProgressReport, error)
	Analyze(ctx context.Context, weeks int) (progress.ProgressRead, error)
}

// Streams is the M3.2 streams-engine seam, injected from main.go (avoids an
// import cycle: api must not import the concrete streams.Engine). *streams.Engine
// satisfies it structurally. GetOrComputeAnalysis returns the cached analysis
// (recomputing from raw on a zone change); FetchAndAnalyze fetch-if-missing +
// computes + caches + returns.
type Streams interface {
	GetOrComputeAnalysis(ctx context.Context, activityID int64) (streams.StreamAnalysis, error)
	FetchAndAnalyze(ctx context.Context, activityID int64) (streams.StreamAnalysis, error)
}

// Chat is the M3.3 chat-engine seam, injected from main.go (avoids an import
// cycle: api must not import the concrete chat.Engine). *chat.Engine satisfies
// it structurally. Answer persists both turns and returns the assistant turn;
// on claude -p failure it returns a typed error (handler -> 502). GET/DELETE
// use Store directly.
type Chat interface {
	Answer(ctx context.Context, message string) (store.ChatMessage, error)
}

// Deps are the handler dependencies injected by main.go (and tests).
type Deps struct {
	Store    *store.Store
	APIToken string
	SyncFunc SyncFunc
	Coach    Coach    // M1
	ImageDir string   // M1: where uploaded CrossFit images are saved
	Agent    Agent    // M2: daily loop (POST /api/agent/run)
	Pusher   Pusher   // M2: push transport
	Progress Progress // M3.1: progress engine (GET /api/progress, POST /api/progress/analyze)
	Streams  Streams  // M3.2: streams engine (GET .../analysis, POST .../stream/fetch)
	Chat     Chat     // M3.3: chat engine (POST /api/chat); GET/DELETE use Store directly
}

// NewRouter builds the chi router with public + bearer-protected routes.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	h := &handlers{d: d}

	// Public (no auth).
	r.Get("/health", h.health)

	// Protected.
	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(d.APIToken))
		r.Get("/api/status", h.status)
		r.Post("/api/sync", h.sync)
		r.Get("/api/activities", h.activities)
		r.Get("/api/recovery", h.recovery)

		// M1
		r.Get("/api/profile", h.profile)
		r.Put("/api/profile", h.updateProfile)
		r.Post("/api/crossfit/parse", h.crossfitParse)
		r.Post("/api/plan/generate", h.planGenerate)
		r.Get("/api/plan", h.plan)
		r.Get("/api/fitness", h.fitness)

		// M2
		r.Post("/api/push/register", h.pushRegister)
		r.Get("/api/today", h.today)
		r.Post("/api/today/undo", h.undoToday)
		r.Post("/api/agent/run", h.agentRun)

		// M3.1
		r.Get("/api/progress", h.progress)
		r.Post("/api/progress/analyze", h.analyzeProgress)

		// M3.2
		r.Get("/api/activities/{id}/analysis", h.activityAnalysis)
		r.Post("/api/activities/{id}/stream/fetch", h.fetchStream)

		// M3.3
		r.Post("/api/chat", h.chat)
		r.Get("/api/chat", h.chatHistory)
		r.Delete("/api/chat", h.clearChat)
	})

	return r
}
