package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/bootstrap"
	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	apphttp "github.com/hayward-solutions/knowledge/api/internal/http"
	"github.com/hayward-solutions/knowledge/api/internal/http/handlers"
	"github.com/hayward-solutions/knowledge/api/internal/storage"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func healthcheck() int {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	client := &nethttp.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if err := migrate(cfg); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	defer pool.Close()

	s := store.New(pool)

	if err := bootstrap.Admin(ctx, s, cfg.Admin); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	if err := handlers.SeedStarterTemplates(ctx, s); err != nil {
		log.Printf("seed starter templates: %v", err)
	}

	oidcCfg, err := auth.LoadOIDC(ctx, cfg.OIDC)
	if err != nil {
		return fmt.Errorf("oidc: %w", err)
	}
	if oidcCfg != nil {
		log.Printf("oidc enabled (provider=%s, admin_group=%q, groups_claim=%q)",
			oidcCfg.ProviderName, oidcCfg.AdminGroup, oidcCfg.GroupsClaim)
	}

	objects, err := storage.New(ctx, cfg.Storage)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	if objects != nil {
		log.Printf("object storage enabled (bucket=%s)", cfg.Storage.Bucket)
	} else {
		log.Printf("object storage disabled (STORAGE_S3_BUCKET unset)")
	}

	bus := events.NewBus()
	hub := events.NewHub()
	presence := events.NewPresence()
	defer presence.Close()

	go sessionCleanup(ctx, s)

	srv := &nethttp.Server{
		Addr: ":" + cfg.APIPort,
		Handler: apphttp.NewRouter(handlers.Deps{
			Store:    s,
			Bus:      bus,
			Hub:      hub,
			Presence: presence,
			Storage:  objects,
			Config:   cfg,
			OIDC:     oidcCfg,
			Version:  version,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("api %s listening on :%s", version, cfg.APIPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	log.Printf("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	bus.Wait()
	return nil
}

func migrate(cfg *config.Config) error {
	db, err := sql.Open("pgx", cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	defer db.Close()
	if err := waitForDB(db, 30*time.Second); err != nil {
		return fmt.Errorf("db wait: %w", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(db, cfg.MigrationsDir); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Printf("migrations applied")
	return nil
}

func waitForDB(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return lastErr
}

func sessionCleanup(ctx context.Context, s *store.Store) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		n, err := s.DeleteExpiredSessions(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("session cleanup: %v", err)
		} else if n > 0 {
			log.Printf("session cleanup: removed %d expired sessions", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
