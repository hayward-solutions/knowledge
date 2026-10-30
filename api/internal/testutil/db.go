package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/hayward-solutions/knowledge/api/internal/store"
)

var (
	startOnce sync.Once
	adminURL  string
	startErr  error
)

func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

func startContainer() {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		startErr = err
		return
	}
	adminURL, startErr = ctr.ConnectionString(ctx, "sslmode=disable")
}

func dsnFor(base, dbName string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

func NewDB(t *testing.T) *store.Store {
	t.Helper()
	s, _ := NewDBWithDSN(t)
	return s
}

func NewDBWithDSN(t *testing.T) (*store.Store, string) {
	t.Helper()
	if dsn := os.Getenv("KNOWLEDGE_TEST_POSTGRES_DSN"); dsn != "" {
		return newDatabase(t, dsn)
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	startOnce.Do(startContainer)
	if startErr != nil {
		t.Fatalf("start postgres container: %v", startErr)
	}
	return newDatabase(t, adminURL)
}

func newDatabase(t *testing.T, base string) (*store.Store, string) {
	t.Helper()
	ctx := context.Background()

	adminPool, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer adminPool.Close()

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	dbName := "kt_" + hex.EncodeToString(suffix)
	if _, err := adminPool.Exec(ctx, `CREATE DATABASE `+dbName); err != nil {
		t.Fatalf("create database: %v", err)
	}
	dsn, err := dsnFor(base, dbName)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, os.DirFS(migrationsDir()))
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = sqlDB.Close()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p, err := pgxpool.New(cctx, base)
		if err != nil {
			return
		}
		defer p.Close()
		_, _ = p.Exec(cctx, `DROP DATABASE IF EXISTS `+dbName+` WITH (FORCE)`)
	})
	return store.New(pool), dsn
}
