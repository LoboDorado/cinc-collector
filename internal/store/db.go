// Package store is the data-access layer for Cinc Collector's SereneDB
// backend. SereneDB speaks the PostgreSQL wire protocol, so this package
// uses pgx directly, but the SQL written here (and in migrations/) targets
// SereneDB's dialect, not plain PostgreSQL: no jsonb, no GIN/pg_trgm, no
// CREATE EXTENSION, foreign keys declared inline at CREATE TABLE time. See
// docs/specs for the full compatibility notes.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgx connection pool to the SereneDB instance.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects to SereneDB, retrying with backoff so that replica startup
// ordering (e.g. in Kubernetes) doesn't require an init-container dance
// beyond running the migrate job first.
func Open(ctx context.Context, dsn string, maxConns int32) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}

	var pool *pgxpool.Pool
	backoff := 500 * time.Millisecond
	const maxBackoff = 10 * time.Second
	for {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				break
			}
			pool.Close()
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("store: connect to db: %w", ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return &DB{Pool: pool}, nil
}

// Close releases the underlying connection pool.
func (db *DB) Close() {
	db.Pool.Close()
}

// Ping checks database connectivity, used by the /_status readiness check.
func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}
