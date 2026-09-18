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
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin event append: %w", err)
	}
	defer tx.Rollback()

	appended, err := appendEventsInTx(ctx, tx, aggregateID, expectedVersion, proposed)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit event append: %w", err)
	}
	return appended, nil
}

// AppendInTx records events inside a caller-supplied, already-open
// transaction: it enforces the identical append-only, expected-version,
// and event-identity invariants Append does, but never opens or commits
// its own transaction — the caller owns the transaction lifecycle
// entirely. This lets a governed Apply boundary (ADR-088 §13.3) commit a
// domain mutation and its corresponding lifecycle outcome event together,
// in one SQLite transaction and one commit, so neither can durably exist
// without the other.
func (s *SQLiteEventStore) AppendInTx(ctx context.Context, tx *sql.Tx, aggregateID string, expectedVersion int64, proposed []eventstore.Event) ([]eventstore.Event, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("sqlite event store database is required")
	}
	if tx == nil {
		return nil, errors.New("sqlite event store requires an active transaction")
	}
	return appendEventsInTx(ctx, tx, aggregateID, expectedVersion, proposed)
}

// appendEventsInTx is the shared insert path for both Append (which opens
// and commits its own transaction) and AppendInTx (which uses a
// caller-supplied one). Keeping this logic in one place, rather than
// duplicated per entry point, is deliberate: the two entry points must
// enforce identical invariants, and a drifted duplicate would be exactly
// the kind of defect this event store's own append-only/sequence
// guarantees exist to prevent.
func appendEventsInTx(ctx context.Context, tx *sql.Tx, aggregateID string, expectedVersion int64, proposed []eventstore.Event) ([]eventstore.Event, error) {
	if aggregateID == "" || len(proposed) == 0 {
		return nil, errors.New("aggregate id and events are required")
	}
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
	return appended, nil
}

func (s *SQLiteEventStore) LoadAggregate(ctx context.Context, aggregateID string, afterVersion int64) ([]eventstore.Event, error) {
	if s == nil || s.db == nil || aggregateID == "" {
		return nil, errors.New("sqlite event store and aggregate id are required")
	}
	return loadAggregateEvents(ctx, s.db, aggregateID, afterVersion)
}

// LoadAggregateInTx reads aggregate history through a caller-supplied
// transaction rather than a fresh connection-pool query. This store's
// pool is capped at exactly one connection
// (see CommitTransitionGuarded's doc comment in store.go), so any query
// against the pool while a governed Apply transaction holds that one
// connection open would deadlock, not merely race; a StepDriver computing
// the next lifecycle journal Sequence inside its own transaction (WU3)
// MUST use this method, never LoadAggregate, while that transaction is
// open.
func (s *SQLiteEventStore) LoadAggregateInTx(ctx context.Context, tx *sql.Tx, aggregateID string, afterVersion int64) ([]eventstore.Event, error) {
	if s == nil || s.db == nil || aggregateID == "" {
		return nil, errors.New("sqlite event store and aggregate id are required")
	}
	if tx == nil {
		return nil, errors.New("sqlite event store requires an active transaction")
	}
	return loadAggregateEvents(ctx, tx, aggregateID, afterVersion)
}

// sqlQueryer is satisfied by both *sql.DB and *sql.Tx, letting
// loadAggregateEvents serve LoadAggregate (pool query) and
// LoadAggregateInTx (caller-owned transaction) from one implementation.
type sqlQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func loadAggregateEvents(ctx context.Context, q sqlQueryer, aggregateID string, afterVersion int64) ([]eventstore.Event, error) {
	rows, err := q.QueryContext(ctx, `SELECT sequence,event_id,aggregate_id,aggregate_type,aggregate_version,event_type,event_version,actor_id,actor_kind,command_id,correlation_id,COALESCE(causation_id,''),COALESCE(trust_class,''),payload,created_at FROM events WHERE aggregate_id=? AND aggregate_version>? ORDER BY aggregate_version`, aggregateID, afterVersion)
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
