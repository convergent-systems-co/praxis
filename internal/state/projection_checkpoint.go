package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/projection"
)

type SQLiteCheckpointStore struct {
	db *sql.DB
}

func NewSQLiteCheckpointStore(db *sql.DB) *SQLiteCheckpointStore {
	return &SQLiteCheckpointStore{db: db}
}

func (s *SQLiteCheckpointStore) LoadCheckpoint(ctx context.Context, name, version string) (projection.Checkpoint, bool, error) {
	if s == nil || s.db == nil {
		return projection.Checkpoint{}, false, errors.New("sqlite checkpoint database is required")
	}
	var checkpoint projection.Checkpoint
	var consistency, updated string
	err := s.db.QueryRowContext(ctx, `SELECT projection_name,projection_version,consistency_class,last_sequence,updated_at FROM projection_checkpoints WHERE projection_name=? AND projection_version=?`, name, version).Scan(
		&checkpoint.Name, &checkpoint.Version, &consistency, &checkpoint.LastSequence, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return projection.Checkpoint{}, false, nil
	}
	if err != nil {
		return projection.Checkpoint{}, false, fmt.Errorf("load projection checkpoint: %w", err)
	}
	checkpoint.Consistency = projection.ConsistencyClass(consistency)
	parsed, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return projection.Checkpoint{}, false, fmt.Errorf("parse projection checkpoint time: %w", err)
	}
	checkpoint.UpdatedAt = parsed
	if err := checkpoint.Validate(); err != nil {
		return projection.Checkpoint{}, false, fmt.Errorf("validate persisted projection checkpoint: %w", err)
	}
	return checkpoint, true, nil
}

func (s *SQLiteCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint projection.Checkpoint) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite checkpoint database is required")
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	updatedAt := checkpoint.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO projection_checkpoints(projection_name,projection_version,consistency_class,last_sequence,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(projection_name,projection_version) DO UPDATE SET last_sequence=excluded.last_sequence,updated_at=excluded.updated_at WHERE excluded.consistency_class=projection_checkpoints.consistency_class AND excluded.last_sequence>=projection_checkpoints.last_sequence`,
		checkpoint.Name, checkpoint.Version, string(checkpoint.Consistency), checkpoint.LastSequence, updatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save projection checkpoint: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect projection checkpoint write: %w", err)
	}
	if changed != 1 {
		return errors.New("projection checkpoint rejected stale sequence or consistency-class change")
	}
	return nil
}

var _ projection.CheckpointStore = (*SQLiteCheckpointStore)(nil)
