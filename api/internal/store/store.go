package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrLastAdmin = errors.New("cannot remove the last active system administrator")
)

const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

type Store struct {
	Pool *pgxpool.Pool
}

const DefaultStatementTimeout = 30 * time.Second

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func NewPool(ctx context.Context, dsn string, statementTimeout time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if statementTimeout > 0 {
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func (s *Store) WithTxTimeout(ctx context.Context, timeout time.Duration, fn func(pgx.Tx) error) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('statement_timeout', $1, true)`, strconv.FormatInt(timeout.Milliseconds(), 10)); err != nil {
			return err
		}
		return fn(tx)
	})
}

func (s *Store) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func IsUniqueViolation(err error) bool { return pgCode(err) == pgUniqueViolation }

func IsForeignKeyViolation(err error) bool { return pgCode(err) == pgForeignKeyViolation }

func notFoundIf(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
