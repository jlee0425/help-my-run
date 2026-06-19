package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"help-my-run/backend/internal/api"
	"help-my-run/backend/internal/config"
	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
	syncpkg "help-my-run/backend/internal/sync"
)

// syncInterval is how often the periodic sync ticker fires.
const syncInterval = 6 * time.Hour

// App is the wired application graph (returned by Wire so tests can drive it).
type App struct {
	Store   *store.Store
	Handler http.Handler
	Strava  *strava.Client
	Runner  garmin.Runner
	Cfg     *config.Config
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

	syncFunc := func(ctx context.Context) (string, int, *string, string, int, *string) {
		res := syncpkg.SyncAll(ctx, s, stravaClient, runner, extraEnv)
		return res.Strava.Status, res.Strava.Synced, res.Strava.Error,
			res.Garmin.Status, res.Garmin.Synced, res.Garmin.Error
	}

	handler := api.NewRouter(api.Deps{
		Store:    s,
		Strava:   stravaClient,
		APIToken: cfg.APIToken,
		SyncFunc: syncFunc,
	})

	return &App{Store: s, Handler: handler, Strava: stravaClient, Runner: runner, Cfg: cfg}, nil
}

// garminEnv builds the env passed through to the worker subprocess.
func garminEnv(cfg *config.Config) []string {
	return []string{
		"GARMIN_EMAIL=" + cfg.GarminEmail,
		"GARMIN_PASSWORD=" + cfg.GarminPassword,
		"GARMIN_TOKENSTORE=" + cfg.GarminTokenstore,
	}
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
	go syncpkg.RunTicker(ctx, syncInterval, func(c context.Context) {
		res := syncpkg.SyncAll(c, app.Store, stravaClient, runner, extraEnv)
		log.Printf("periodic sync: strava=%s/%d garmin=%s/%d",
			res.Strava.Status, res.Strava.Synced, res.Garmin.Status, res.Garmin.Synced)
	})

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
