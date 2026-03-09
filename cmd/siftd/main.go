package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sift/internal/hosted"
	"sift/internal/pipeline"
	"sift/internal/postgres"
	"sift/internal/zitadel"
)

const (
	defaultListenAddr   = ":8080"
	defaultRegistryPath = "docs/contracts/source-registry.seed.json"
	defaultOutputDir    = "output"
	defaultSyncInterval = 5 * time.Minute
	defaultSyncTimeout  = 4 * time.Minute
	defaultRetention    = 30 * 24 * time.Hour
)

type config struct {
	ListenAddr      string
	PostgresDSN     string
	RegistryPath    string
	OutputDir       string
	SyncInterval    time.Duration
	SyncTimeout     time.Duration
	RetentionWindow time.Duration
	SyncOnStart     bool
	ZitadelIssuer   string
	ZitadelAudience string
	AllowedOrigins  []string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := loadConfigFromEnv()
	if err != nil {
		return err
	}

	store, err := postgres.OpenHostedStore(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer store.Close()

	validator, err := zitadel.NewOIDCValidator(ctx, cfg.ZitadelIssuer, cfg.ZitadelAudience)
	if err != nil {
		return fmt.Errorf("configure zitadel validator: %w", err)
	}

	api, err := hosted.New(hosted.Options{
		Store:                 store,
		Validator:             validator,
		OutputDir:             cfg.OutputDir,
		AllowedBrowserOrigins: cfg.AllowedOrigins,
		Now:                   func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return err
	}

	go runScheduler(ctx, store, api, cfg)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	fmt.Printf(
		"siftd started: addr=%s sync_interval=%s retention=%s\n",
		cfg.ListenAddr,
		cfg.SyncInterval,
		cfg.RetentionWindow,
	)

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http serve: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	return nil
}

func runScheduler(ctx context.Context, store *postgres.Store, api *hosted.Server, cfg config) {
	runOnce := func() {
		started := time.Now().UTC()

		syncCtx, cancel := context.WithTimeout(ctx, cfg.SyncTimeout)
		defer cancel()

		summary, err := pipeline.RunSync(syncCtx, store, pipeline.Options{
			Mode:            pipeline.ModeFull,
			RegistryPath:    cfg.RegistryPath,
			OutputDir:       cfg.OutputDir,
			RetentionWindow: cfg.RetentionWindow,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return
			}
			api.MarkSyncFailure(started, err)
			fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
			return
		}

		finished := time.Now().UTC()
		api.MarkSyncSuccess(summary.RunID, finished)
		fmt.Printf(
			"sync completed: run_id=%s status=%s events_rebuilt=%d sources_failed=%d\n",
			summary.RunID,
			summary.Status,
			summary.EventsRebuilt,
			summary.SourcesFailed,
		)
	}

	if cfg.SyncOnStart {
		runOnce()
	}

	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func loadConfigFromEnv() (config, error) {
	syncInterval, err := parseDurationEnv("SIFTD_SYNC_INTERVAL", defaultSyncInterval)
	if err != nil {
		return config{}, err
	}

	syncTimeout, err := parseDurationEnv("SIFTD_SYNC_TIMEOUT", defaultSyncTimeout)
	if err != nil {
		return config{}, err
	}

	retentionWindow, err := parseDurationEnv("SIFTD_RETENTION", defaultRetention)
	if err != nil {
		return config{}, err
	}

	syncOnStart, err := parseBoolEnv("SIFTD_SYNC_ON_START", true)
	if err != nil {
		return config{}, err
	}

	allowedOrigins := parseCSVEnv("SIFTD_ALLOWED_ORIGINS")
	if len(allowedOrigins) == 0 {
		allowedOrigins = parseCSVEnv("SIFTD_WS_ALLOWED_ORIGINS")
	}

	cfg := config{
		ListenAddr:      envOrDefault("SIFTD_ADDR", defaultListenAddr),
		PostgresDSN:     strings.TrimSpace(os.Getenv("SIFTD_POSTGRES_DSN")),
		RegistryPath:    envOrDefault("SIFTD_REGISTRY", defaultRegistryPath),
		OutputDir:       envOrDefault("SIFTD_OUTPUT_DIR", defaultOutputDir),
		SyncInterval:    syncInterval,
		SyncTimeout:     syncTimeout,
		RetentionWindow: retentionWindow,
		SyncOnStart:     syncOnStart,
		ZitadelIssuer:   strings.TrimSpace(os.Getenv("SIFTD_ZITADEL_ISSUER")),
		ZitadelAudience: strings.TrimSpace(os.Getenv("SIFTD_ZITADEL_AUDIENCE")),
		AllowedOrigins:  allowedOrigins,
	}

	if cfg.PostgresDSN == "" {
		return config{}, fmt.Errorf("SIFTD_POSTGRES_DSN is required")
	}
	if cfg.ZitadelIssuer == "" {
		return config{}, fmt.Errorf("SIFTD_ZITADEL_ISSUER is required")
	}
	if cfg.ZitadelAudience == "" {
		return config{}, fmt.Errorf("SIFTD_ZITADEL_AUDIENCE is required")
	}
	if cfg.SyncTimeout <= 0 {
		return config{}, fmt.Errorf("SIFTD_SYNC_TIMEOUT must be > 0")
	}
	if cfg.SyncInterval <= 0 {
		return config{}, fmt.Errorf("SIFTD_SYNC_INTERVAL must be > 0")
	}
	if cfg.RetentionWindow < 0 {
		return config{}, fmt.Errorf("SIFTD_RETENTION must be >= 0")
	}

	return cfg, nil
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	return duration, nil
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback, nil
	}

	switch raw {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be boolean", key)
	}
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}

	return out
}
