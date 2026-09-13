package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// SQLiteEventStore implements the canonical append-only event store over the
// authoritative Praxis SQLite database.
type SQLiteEventStore struct {
	db *sql.DB
}

func NewSQLiteEventStore(db *sql.DB) *SQLiteEventStore { return &SQLiteEventStore{db: db} }

func (s *SQLiteEventStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, proposed []eventstore.Event) ([]eventstore.Event, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("sqlite event store database is required")
	}
	if aggregateID == "" || len(proposed) == 0 {
		return nil, errors.New("aggregate id and events are required")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin event append: %w", err)
	}
	defer tx.Rollback()

	aggregateType := proposed[0].AggregateType
	if aggregateType == "" {
		return nil, errors.New("aggregate type is required")
	}
	nextVersion := expectedVersion + int64(len(proposed))
	if err := compareAndAdvanceAggregate(ctx, tx, aggregateID, aggregateType, expectedVersion, nextVersion); err != nil {
		if errors.Is(err, ErrAggregateVersion) {
			return nil, eventstore.ErrVersionConflict
		}
		return nil, err
	}

	appended := make([]eventstore.Event, len(proposed))
	for i, candidate := range proposed {
		if candidate.ID == "" || candidate.Type == "" || candidate.Version == "" || candidate.CommandID == "" || candidate.CorrelationID == "" {
			return nil, fmt.Errorf("event %d missing required identity/metadata", i)
		}
		if err := candidate.Actor.Validate(); err != nil {
			return nil, fmt.Errorf("event %d actor: %w", i, err)
		}
		if candidate.AggregateID != "" && candidate.AggregateID != aggregateID {
			return nil, errors.New("event aggregate id mismatch")
		}
		if candidate.AggregateType != aggregateType {
			return nil, errors.New("event aggregate type mismatch")
		}
		candidate.AggregateID = aggregateID
		candidate.AggregateVersion = expectedVersion + int64(i) + 1
		createdAt := candidate.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		candidate.CreatedAt = createdAt.UTC()
		if err := ensureEventCommand(ctx, tx, aggregateID, candidate); err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO events(event_id, aggregate_id, aggregate_type, aggregate_version, event_type, event_version, actor_id, actor_kind, command_id, correlation_id, causation_id, trust_class, payload, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			candidate.ID, candidate.AggregateID, candidate.AggregateType, candidate.AggregateVersion,
			candidate.Type, candidate.Version, candidate.Actor.ID, candidate.Actor.Kind,
			candidate.CommandID, candidate.CorrelationID, nullable(candidate.CausationID), nullable(string(candidate.Trust)), candidate.Payload,
			candidate.CreatedAt.Format(time.RFC3339Nano))
		if err != nil {
			return nil, fmt.Errorf("insert event %d: %w", i, err)
		}
		sequence, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("read event sequence: %w", err)
		}
		candidate.Sequence = sequence
		appended[i] = candidate
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit event append: %w", err)
	}
	return appended, nil
}

func (s *SQLiteEventStore) LoadAggregate(ctx context.Context, aggregateID string, afterVersion int64) ([]eventstore.Event, error) {
	if s == nil || s.db == nil || aggregateID == "" {
		return nil, errors.New("sqlite event store and aggregate id are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,event_id,aggregate_id,aggregate_type,aggregate_version,event_type,event_version,actor_id,actor_kind,command_id,correlation_id,COALESCE(causation_id,''),COALESCE(trust_class,''),payload,created_at FROM events WHERE aggregate_id=? AND aggregate_version>? ORDER BY aggregate_version`, aggregateID, afterVersion)
	if err != nil {
		return nil, fmt.Errorf("load aggregate events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *SQLiteEventStore) ReadFrom(ctx context.Context, afterSequence int64, limit int) ([]eventstore.Event, error) {
	if s == nil || s.db == nil || limit <= 0 {
		return nil, errors.New("sqlite event store and positive read limit are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,event_id,aggregate_id,aggregate_type,aggregate_version,event_type,event_version,actor_id,actor_kind,command_id,correlation_id,COALESCE(causation_id,''),COALESCE(trust_class,''),payload,created_at FROM events WHERE sequence>? ORDER BY sequence LIMIT ?`, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("read event sequence: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func ensureEventCommand(ctx context.Context, tx *sql.Tx, scope string, event eventstore.Event) error {
	created := event.CreatedAt.UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO commands(command_id, command_type, command_version, actor_id, actor_kind, scope, correlation_id, causation_id, payload, status, created_at, completed_at) VALUES(?,?,?,?,?,?,?,?,?,'committed',?,?)`,
		event.CommandID, "eventstore.append", "1", event.Actor.ID, event.Actor.Kind, scope, event.CorrelationID, nullable(event.CausationID), []byte(`{}`), created, created)
	if err != nil {
		return fmt.Errorf("ensure event command: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 1 {
		return nil
	}
	var actorID, actorKind, correlationID string
	if err := tx.QueryRowContext(ctx, `SELECT actor_id, actor_kind, correlation_id FROM commands WHERE command_id=?`, event.CommandID).Scan(&actorID, &actorKind, &correlationID); err != nil {
		return fmt.Errorf("verify reused event command: %w", err)
	}
	if actorID != event.Actor.ID || actorKind != event.Actor.Kind || correlationID != event.CorrelationID {
		return errors.New("reused command id metadata mismatch")
	}
	return nil
}

func scanEvents(rows *sql.Rows) ([]eventstore.Event, error) {
	out := make([]eventstore.Event, 0)
	for rows.Next() {
		var event eventstore.Event
		var actorID, actorKind, trust, created string
		if err := rows.Scan(&event.Sequence, &event.ID, &event.AggregateID, &event.AggregateType, &event.AggregateVersion,
			&event.Type, &event.Version, &actorID, &actorKind, &event.CommandID, &event.CorrelationID, &event.CausationID,
			&trust, &event.Payload, &created); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.Actor = contracts.PrincipalRef{ID: actorID, Kind: actorKind}
		event.Trust = contracts.TrustClass(trust)
		parsed, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse event time: %w", err)
		}
		event.CreatedAt = parsed.UTC()
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return out, nil
}

var _ eventstore.Store = (*SQLiteEventStore)(nil)
