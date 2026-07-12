package main

import (
	"context"
	"flag"
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
	"help-my-run/backend/internal/auth"
	"help-my-run/backend/internal/backup"
	"help-my-run/backend/internal/chat"
	"help-my-run/backend/internal/coach"
	"help-my-run/backend/internal/config"
	"help-my-run/backend/internal/demo"
	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/scheduler"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/streams"
	syncpkg "help-my-run/backend/internal/sync"
	"help-my-run/backend/internal/webpush"
)

// syncInterval is how often the periodic sync ticker fires.
const syncInterval = 6 * time.Hour

// App is the wired application graph (returned by Wire so tests can drive it).
type App struct {
	Store    *store.Store
	Handler  http.Handler
	Runner   garmin.Runner
	Cfg      *config.Config
	Auth     *auth.Service    // M5: owner sessions + API token
	Coach    *coach.Coach     // M2: shared coach engine (also drives the agent)
	Agent    *agent.Agent     // M2: daily readiness/adjust loop
	Progress *progress.Engine // M3.1: deterministic trends + claude -p read
	Streams  *streams.Engine  // M3.2: per-run stream fetch + time-in-zone/decoupling
	Chat     *chat.Engine     // M3.3: curated-pack chat-with-your-data engine
}

// Wire builds the full application graph from config: opens + migrates the
// store, constructs the Garmin runner, and builds the router with a SyncFunc
// adapter that runs SyncAll.
func Wire(cfg *config.Config) (*App, error) {
	dbPath := cfg.DBPath
	if cfg.Demo {
		dbPath = ":memory:" // M6.5: zero residue, no interference with a real DB
	}
	s, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := s.Migrate(); err != nil {
		_ = s.Close()
		return nil, err
	}
	if cfg.Demo {
		// Seed on the UTC calendar date: /api/today (resolveDate) and the web
		// default to UTC, so day-0 must be the UTC date or the centerpiece 404s
		// whenever the server's local date differs from UTC.
		if err := demo.Seed(s, time.Now().UTC()); err != nil {
			_ = s.Close()
			return nil, err
		}
		// Any accidental disk write (e.g. an image upload) lands in a throwaway
		// dir, never the real IMAGE_DIR — keeps the zero-residue promise.
		if tmp, terr := os.MkdirTemp("", "helpmyrun-demo-"); terr == nil {
			cfg.ImageDir = tmp
		}
	}

	runner := garmin.Runner{Python: cfg.PythonBin, Script: cfg.WorkerScript}
	extraEnv := garminEnv(cfg)

	streamsEngine := streams.New(s, runner, extraEnv)

	syncFunc := func(ctx context.Context) (string, int, *string) {
		res := syncpkg.SyncAll(ctx, s, runner, extraEnv, streamTrickle(cfg, streamsEngine))
		return res.Garmin.Status, res.Garmin.Synced, res.Garmin.Error
	}

	// claudeEnv injects the stored setup-token (if any) into every claude -p
	// call — the subscription path for headless hosts. No token -> claude's own
	// `claude auth login` credentials are used.
	claudeEnv := func() []string {
		tok, err := s.GetSetting(store.SettingClaudeToken)
		if err != nil || tok == "" {
			return nil
		}
		return []string{"CLAUDE_CODE_OAUTH_TOKEN=" + tok}
	}
	var llmRunner llm.Runner = llm.ExecRunner{Bin: cfg.ClaudeBin, EnvFunc: claudeEnv}
	if cfg.Demo {
		llmRunner = demo.Runner{} // curated sample outputs, no claude -p
	}
	llmClient := &llm.Client{
		Runner:  llmRunner,
		Model:   cfg.ClaudeModel,
		Timeout: 120 * time.Second,
	}
	coachEngine := coach.New(s, llmClient, cfg.ClaudeModel, cfg.ImageDir)
	progressEngine := progress.New(s, llmClient, cfg.ClaudeModel)
	chatEngine := chat.New(s, llmClient, progressEngine, cfg.ClaudeModel, cfg.ChatHistoryTurns)

	authService := auth.New(s)

	pushService, err := webpush.New(s)
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	// The agent's syncer is a no-op when we must never auto-pull Garmin: demo
	// mode (canned data) and manual mode (sync only via "Sync now"). In both,
	// "Run coach now" still works — it coaches on last-synced data.
	var syncer agent.Syncer = agent.NewRealSyncer(s, runner, extraEnv)
	switch {
	case cfg.Demo:
		syncer = noopSyncer{note: "demo mode: sync skipped"}
	case cfg.ManualSync:
		syncer = noopSyncer{note: "manual mode: sync only via Sync now"}
	}
	dailyAgent := agent.New(
		s,
		syncer,
		coachEngine,
		pushService, // Web Push briefing (M5)
		agentClock{},
		nil, // loc resolved in main() from profile; agent default UTC is fine for Wire
	)

	handler := api.NewRouter(api.Deps{
		Store:    s,
		Auth:     authService,
		SyncFunc: syncFunc,
		Coach:    coachEngine,
		ImageDir: cfg.ImageDir,
		Agent:    apiAgent{a: dailyAgent, store: s},
		Progress: progressEngine,
		Streams:  streamsEngine,
		Chat:     chatEngine,

		GarminLogin:      garmin.NewLoginManager(cfg.PythonBin, cfg.WorkerScript, extraEnv),
		GarminTokenstore: cfg.GarminTokenstore,
		Claude:           &api.ClaudeProbe{Bin: cfg.ClaudeBin, Model: cfg.ClaudeModel, Runner: llmRunner},
		Push:             pushService,
		Demo:             cfg.Demo,
		ManualSync:       cfg.ManualSync,
	})

	return &App{
		Store:    s,
		Handler:  handler,
		Runner:   runner,
		Cfg:      cfg,
		Auth:     authService,
		Coach:    coachEngine,
		Agent:    dailyAgent,
		Progress: progressEngine,
		Streams:  streamsEngine,
		Chat:     chatEngine,
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

// garminEnv builds the env passed through to the worker subprocess. Fetch and
// stream resume from the tokenstore; credentials never ride the environment.
func garminEnv(cfg *config.Config) []string {
	return []string{
		"GARMIN_TOKENSTORE=" + cfg.GarminTokenstore,
	}
}

// noopSyncer is a no-op agent syncer: reports "skipped" and touches nothing, so
// the coach loop runs against existing data without ever spawning the Garmin
// worker or reading the tokenstore. Used by demo mode (M6.5) and manual mode
// (M6.6).
type noopSyncer struct{ note string }

func (n noopSyncer) SyncAll(context.Context) syncpkg.AllResult {
	note := n.note
	return syncpkg.AllResult{Garmin: syncpkg.SourceResult{Status: "skipped", Error: &note}}
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

// runNightlyBackup snapshots the DB + Garmin tokenstore after the daily agent
// run (M6). Failures are loud in the log but must never crash the agent loop.
func runNightlyBackup(s *store.Store, cfg *config.Config) {
	tokenstore, _ := syncpkg.ExpandHome(cfg.GarminTokenstore) // unexpanded ~ -> copy skipped
	path, err := backup.Run(s, tokenstore, cfg.ResolvedBackupDir(), cfg.BackupKeep)
	if err != nil {
		log.Printf("backup: FAILED (data is NOT protected tonight): %v", err)
		return
	}
	log.Printf("backup: wrote %s (keep=%d)", path, cfg.BackupKeep)
}

// runSyncOnBoot invokes the sync fn once immediately so a fresh instance pulls
// data without waiting a full ticker interval (M0 follow-up #2). It runs in a
// goroutine so server startup is not blocked.
func runSyncOnBoot(ctx context.Context, fn func(context.Context)) {
	go fn(ctx)
}

func main() {
	resetPassword := flag.Bool("reset-password", false,
		"clear the owner password, API token, and all sessions, then exit (setup runs again on next visit)")
	demoMode := flag.Bool("demo", false,
		"demo mode: in-memory DB seeded with synthetic data, no Garmin or Claude needed (M6.5)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	cfg.Demo = *demoMode

	app, err := Wire(cfg)
	if err != nil {
		log.Fatalf("wire: %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	if *resetPassword {
		if err := app.Auth.ResetPassword(); err != nil {
			log.Fatalf("reset-password: %v", err)
		}
		log.Printf("owner password, API token, and sessions cleared — open the web UI to run setup again")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.Demo {
		// M6.5: no sync, no scheduler — the fixture carries today's decision.
		// Serve and nothing else; the DB evaporates on exit.
		log.Printf("=== DEMO MODE === synthetic data, sample coach responses, in-memory DB; nothing is saved")
		serve(ctx, cfg, app)
		return
	}

	// Periodic sync ticker (the agentic schedule is M2; this is plain periodic).
	runner := app.Runner
	extraEnv := garminEnv(cfg)
	syncOnce := func(c context.Context) {
		if !syncpkg.TokenStoreReady(cfg.GarminTokenstore) {
			log.Printf("sync: skipped — Garmin token store not found at %s; run `make garmin-login` once (avoids hitting Garmin's login rate limit)", cfg.GarminTokenstore)
			return
		}
		res := syncpkg.SyncAll(c, app.Store, runner, extraEnv, streamTrickle(cfg, app.Streams))
		log.Printf("sync: garmin=%s/%d", res.Garmin.Status, res.Garmin.Synced)
	}
	// M0 follow-up #2: run once on boot, then on the interval. M6.6: manual mode
	// disables all automatic pulling — sync happens only via POST /api/sync.
	if cfg.ManualSync {
		log.Printf("sync: MANUAL mode — no boot sync, no periodic ticker; pull with `Sync now`")
	} else {
		runSyncOnBoot(ctx, syncOnce)
		go syncpkg.RunTicker(ctx, syncInterval, syncOnce)
	}

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
		func(c context.Context, localDate string, enabled bool) {
			switch {
			case cfg.ManualSync:
				// M6.6: no automatic verdict; this timer only carries the backup.
				log.Printf("agent: date=%s skipped (manual mode); nightly backup still runs", localDate)
			case enabled:
				res := app.Agent.RunDaily(c, localDate)
				log.Printf("agent: date=%s skipped=%v color=%s action=%s source=%s pushed=%v",
					res.Date, res.Skipped, res.ReadinessColor, res.Action, res.Source, res.Pushed)
			default:
				log.Printf("agent: date=%s skipped (disabled in profile); nightly backup still runs", localDate)
			}
			runNightlyBackup(app.Store, cfg) // M6: backup rides the cadence regardless
		})
	log.Printf("agent scheduler: started (schedule re-read from profile each cycle)")

	serve(ctx, cfg, app)
}

// serve runs the HTTP server until ctx is cancelled (shared by demo + normal).
func serve(ctx context.Context, cfg *config.Config, app *App) {
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
