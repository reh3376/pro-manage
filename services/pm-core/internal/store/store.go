// Package store owns the Postgres connection pool, migrations, and the
// transaction helper every domain service uses. Audit rows are written in the
// same transaction as the mutation they record (FSD §08), which is why the
// helper hands out a sqlc Queries bound to the transaction.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"

	dbassets "github.com/reh3376/pro-manage/services/pm-core/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
)

// Store wraps the pool.
type Store struct {
	Pool *pgxpool.Pool
}

// Open connects and pings.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.Pool.Close() }

// Queries returns a sqlc handle on the pool (auto-commit, reads).
func (s *Store) Queries() *db.Queries { return db.New(s.Pool) }

// Tx runs fn inside a transaction with a sqlc handle bound to it.
func (s *Store) Tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	if err := fn(db.New(tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// Migrate applies every pending migration. Safe to run on each start.
func Migrate(ctx context.Context, log *slog.Logger, dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("store: migrate open: %w", err)
	}
	defer sqlDB.Close()
	provider, err := newProvider(sqlDB)
	if err != nil {
		return err
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("store: migrate up: %w", err)
	}
	v, _ := provider.GetDBVersion(ctx)
	log.Info("migrations applied", "applied_now", len(results), "version", v)
	return nil
}

// MigrateDownTo rolls back to the given version (tests and dev only).
func MigrateDownTo(ctx context.Context, dsn string, version int64) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	provider, err := newProvider(sqlDB)
	if err != nil {
		return err
	}
	_, err = provider.DownTo(ctx, version)
	return err
}

func newProvider(sqlDB *sql.DB) (*goose.Provider, error) {
	sub, err := fs.Sub(dbassets.Migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: migrations fs: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub)
	if err != nil {
		return nil, fmt.Errorf("store: goose provider: %w", err)
	}
	return p, nil
}
