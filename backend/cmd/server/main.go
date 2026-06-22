package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata" // embed the IANA tz DB for headless time.LoadLocation

	"help-my-run/backend/internal/agent"
	"help-my-run/backend/internal/api"
	"help-my-run/backend/internal/coach"
	"help-my-run/backend/internal/config"
	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/scheduler"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
	"help-my-run/backend/internal/streams"
	syncpkg "help-my-run/backend/internal/sync"
)

// syncInterval is how often the periodic sync ticker fires.
const syncInterval = 6 * time.Hour

// App is the wired application graph (returned by Wire so tests can drive it).
type App struct {
	Store    *store.Store
	Handler  http.Handler
	Strava   *strava.Client
	Runner   garmin.Runner
	Cfg      *config.Config
	Coach    *coach.Coach     // M2: shared coach engine (also drives the agent)
	Agent    *agent.Agent     // M2: daily readiness/adjust loop
	Pusher   *push.Client     // M2: Expo push transport
	Progress *progress.Engine // M3.1: deterministic trends + claude -p read
	Streams  *streams.Engine  // M3.2: per-run stream fetch + time-in-zone/decoupling
}

// Wire builds the full application graph from config: opens + migrates the
// store, constructs the Strava client and Garmin runner, and builds the router
// with a SyncFunc adapter that runs SyncAll.
func Wire(cfg *config.Config) (*App, error) {
	s, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := s.Migrate(); err != nil {
		_ = s.Close()
		return nil, err
	}

	stravaClient := strava.New(cfg.StravaClientID, cfg.StravaClientSecret, cfg.StravaRedirectURL)
	runner := garmin.Runner{Python: cfg.PythonBin, Script: cfg.WorkerScript}
	extraEnv := garminEnv(cfg)

	streamsEngine := streams.New(s, stravaClient, runner, extraEnv, cfg.GarminMatchToleranceS)

	syncFunc := func(ctx context.Context) (string, int, *string, string, int, *string) {
		res := syncpkg.SyncAll(ctx, s, stravaClient, runner, extraEnv, streamTrickle(cfg, streamsEngine))
		return res.Strava.Status, res.Strava.Synced, res.Strava.Error,
			res.Garmin.Status, res.Garmin.Synced, res.Garmin.Error
	}

	llmClient := &llm.Client{
		Runner:  llm.ExecRunner{Bin: cfg.ClaudeBin},
		Model:   cfg.ClaudeModel,
		Timeout: 120 * time.Second,
	}
	coachEngine := coach.New(s, llmClient, cfg.ClaudeModel, cfg.ImageDir)
	progressEngine := progress.New(s, llmClient, cfg.ClaudeModel)

	pushClient := push.NewClient(cfg.ExpoPushBaseURL)
	dailyAgent := agent.New(
		s,
		agent.NewRealSyncer(s, stravaClient, runner, extraEnv),
		coachEngine,
		pushClient,
		agentClock{},
		nil, // loc resolved in main() from profile; agent default UTC is fine for Wire
	)

	handler := api.NewRouter(api.Deps{
		Store:    s,
		Strava:   stravaClient,
		APIToken: cfg.APIToken,
		SyncFunc: syncFunc,
		Coach:    coachEngine,
		ImageDir: cfg.ImageDir,
		Agent:    apiAgent{a: dailyAgent, store: s},
		Pusher:   pushClient,
		Progress: progressEngine,
		Streams:  streamsEngine,
	})

	return &App{
		Store:    s,
		Handler:  handler,
		Strava:   stravaClient,
		Runner:   runner,
		Cfg:      cfg,
		Coach:    coachEngine,
		Agent:    dailyAgent,
		Pusher:   pushClient,
		Progress: progressEngine,
		Streams:  streamsEngine,
	}, nil
}

// streamTrickleFetcher adapts *streams.Engine to sync.streamFetcher: the engine's
// FetchAndAnalyze returns (StreamAnalysis, error) but the trickle only needs the
// error (it fetches + caches as a side effect, ignoring the returned analysis).
type streamTrickleFetcher struct{ e *streams.Engine }

func (f streamTrickleFetcher) FetchAndAnalyze(ctx context.Context, activityID int64) error {
	_, err := f.e.FetchAndAnalyze(ctx, activityID)
	return err
}

// streamTrickle builds the recent-window trickle hook for SyncAll. A nil engine
// yields a nil hook (trickle disabled).
func streamTrickle(cfg *config.Config, e *streams.Engine) *syncpkg.StreamTrickle {
	if e == nil {
		return nil
	}
	return &syncpkg.StreamTrickle{
		Fetcher: streamTrickleFetcher{e: e},
		Weeks:   cfg.StreamRecentWeeks,
		Budget:  cfg.StreamFetchBudget,
	}
}

// garminEnv builds the env passed through to the worker subprocess.
func garminEnv(cfg *config.Config) []string {
	return []string{
		"GARMIN_EMAIL=" + cfg.GarminEmail,
		"GARMIN_PASSWORD=" + cfg.GarminPassword,
		"GARMIN_TOKENSTORE=" + cfg.GarminTokenstore,
	}
}

// agentClock backs the agent with the real clock.
type agentClock struct{}

func (agentClock) Now() time.Time { return time.Now() }

// loadAgentLocation loads the IANA timezone for the daily schedule. Empty -> UTC.
func loadAgentLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(tz)
}

// parseRunTime splits "HH:MM" 24h into hour, minute; defaults to 05:30 on a
// malformed value.
func parseRunTime(s string) (int, int) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 {
		h, herr := strconv.Atoi(parts[0])
		m, merr := strconv.Atoi(parts[1])
		if herr == nil && merr == nil && h >= 0 && h < 24 && m >= 0 && m < 60 {
			return h, m
		}
	}
	return 5, 30
}

// apiAgent adapts *agent.Agent to the api.Agent seam, adding force semantics:
// force deletes the persistent once-per-day guard before running.
type apiAgent struct {
	a     *agent.Agent
	store *store.Store
}

func (p apiAgent) RunDaily(ctx context.Context, localDate string, force bool) agent.RunResult {
	if force {
		_ = p.store.DeleteAgentRun(localDate) // reset the persistent once-per-day guard
	}
	return p.a.RunDaily(ctx, localDate)
}

// runSyncOnBoot invokes the sync fn once immediately so a fresh instance pulls
// data without waiting a full ticker interval (M0 follow-up #2). It runs in a
// goroutine so server startup is not blocked.
func runSyncOnBoot(ctx context.Context, fn func(context.Context)) {
	go fn(ctx)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app, err := Wire(cfg)
	if err != nil {
		log.Fatalf("wire: %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Periodic sync ticker (the agentic schedule is M2; this is plain periodic).
	stravaClient := app.Strava
	runner := app.Runner
	extraEnv := garminEnv(cfg)
	syncOnce := func(c context.Context) {
		res := syncpkg.SyncAll(c, app.Store, stravaClient, runner, extraEnv, streamTrickle(cfg, app.Streams))
		log.Printf("sync: strava=%s/%d garmin=%s/%d",
			res.Strava.Status, res.Strava.Synced, res.Garmin.Status, res.Garmin.Synced)
	}
	// M0 follow-up #2: run once on boot, then on the interval.
	runSyncOnBoot(ctx, syncOnce)
	go syncpkg.RunTicker(ctx, syncInterval, syncOnce)

	// scheduleProvider re-reads the live schedule from athlete_profile on every
	// scheduler loop iteration; env values are first-boot fallbacks only.
	scheduleProvider := func() (scheduler.Config, bool, error) {
		runTime := cfg.AgentRunTime
		runTz := cfg.AgentTimezone
		enabled := cfg.AgentEnabledDefault
		if prof, perr := app.Store.GetAthleteProfile(); perr == nil {
			if prof.DailyRunTime != "" {
				runTime = prof.DailyRunTime
			}
			if prof.Timezone != "" {
				runTz = prof.Timezone
			}
			enabled = prof.AgentEnabled
		}
		loc, lerr := loadAgentLocation(runTz)
		if lerr != nil {
			return scheduler.Config{}, false, fmt.Errorf("scheduler tz %q: %w", runTz, lerr)
		}
		hh, mm := parseRunTime(runTime)
		return scheduler.Config{Hour: hh, Minute: mm, Loc: loc}, enabled, nil
	}
	go scheduler.Run(ctx, scheduler.RealClock{}, scheduleProvider,
		func(c context.Context, localDate string) {
			res := app.Agent.RunDaily(c, localDate)
			log.Printf("agent: date=%s skipped=%v color=%s action=%s source=%s pushed=%v",
				res.Date, res.Skipped, res.ReadinessColor, res.Action, res.Source, res.Pushed)
		})
	log.Printf("agent scheduler: started (schedule re-read from profile each cycle)")

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	_ = os.Stdout.Sync()
}
