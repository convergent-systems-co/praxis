package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	sqlitemigrations "github.com/convergent-systems-co/praxis/migrations/sqlite"
	_ "modernc.org/sqlite"
)

// OpenSQLite opens Praxis authoritative state with connection-level invariants
// applied to every physical SQLite connection. The writer pool is deliberately
// serialized: SQLite has one writer and deterministic authority transitions are
// more important than speculative write concurrency.
func OpenSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}

	u := &url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Set("_foreign_keys", "1")
	q.Set("_busy_timeout", "5000")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	q.Set("_txlock", "immediate")
	q.Add("_pragma", "secure_delete(1)")
	u.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Serialize authoritative writes at the process boundary. Read-heavy
	// projections can later use an independent read-only pool if profiling
	// demonstrates a need.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := sqlitemigrations.Apply(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	return db, nil
}
