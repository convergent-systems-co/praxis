package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrCommandExists       = errors.New("command already exists")
	ErrAggregateVersion    = errors.New("aggregate version mismatch")
	ErrApprovalUnavailable = errors.New("approval unavailable")
	ErrLeaseUnavailable    = errors.New("capability lease unavailable")
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

// DB exposes the already-open authoritative database to repository-level
// atomic compatibility-record writers; it does not create a second store.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

type CommandRecord struct {
	ID            string
	Type          string
	Version       string
	Actor         contracts.PrincipalRef
	Scope         string
	CorrelationID string
	CausationID   string
	Payload       []byte
	CreatedAt     time.Time
}

type EventRecord struct {
	ID               string
	AggregateID      string
	AggregateType    string
	AggregateVersion int64
	Type             string
	Version          string
	Actor            contracts.PrincipalRef
	CommandID        string
	CorrelationID    string
	CausationID      string
	ProvenanceJSON   []byte
	TrustClass       contracts.TrustClass
	Payload          []byte
	CreatedAt        time.Time
	IntegrityJSON    []byte
}

type EffectRecord struct {
	ID                 string
	CommandID          string
	ActionIntentDigest string
	TargetAdapter      string
	TargetPrincipal    string
	CapabilityLeaseID  string
	ApprovalID         string
	IdempotencyKey     string
	PreconditionsJSON  []byte
	CryptoProfile      contracts.CryptoProfile
	State              string
	RequestPayload     []byte
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CommitTransition atomically records one authoritative local transition.
// External effects are represented as durable intent only; dispatch occurs after commit.
func (s *Store) CommitTransition(
	ctx context.Context,
	cmd CommandRecord,
	expectedAggregateVersion int64,
	event EventRecord,
	approvalID string,
	leaseID string,
	effect *EffectRecord,
) error {
	if cmd.ID == "" || event.ID == "" || event.AggregateID == "" {
		return errors.New("command, event, and aggregate identity are required")
	}
	if event.CommandID != cmd.ID {
		return errors.New("event command id does not match command")
	}
	if event.AggregateVersion != expectedAggregateVersion+1 {
		return fmt.Errorf("event aggregate version must be expected+1: expected %d got %d", expectedAggregateVersion+1, event.AggregateVersion)
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transition: %w", err)
	}
	defer tx.Rollback()

	if err := insertCommand(ctx, tx, cmd); err != nil {
		return err
	}
	if err := compareAndAdvanceAggregate(ctx, tx, event.AggregateID, event.AggregateType, expectedAggregateVersion, event.AggregateVersion); err != nil {
		return err
	}
	if approvalID != "" {
		if err := consumeApproval(ctx, tx, approvalID, cmd.CreatedAt); err != nil {
			return err
		}
	}
	if leaseID != "" {
		if err := consumeLease(ctx, tx, leaseID, cmd.CreatedAt); err != nil {
			return err
		}
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if effect != nil {
		if effect.CommandID != cmd.ID {
			return errors.New("effect command id does not match command")
		}
		if err := insertEffect(ctx, tx, *effect); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE commands SET status='committed', completed_at=? WHERE command_id=?`, event.CreatedAt.UTC().Format(time.RFC3339Nano), cmd.ID); err != nil {
		return fmt.Errorf("complete command: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transition: %w", err)
	}
	return nil
}

func insertCommand(ctx context.Context, tx *sql.Tx, c CommandRecord) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO commands(command_id, command_type, command_version, actor_id, actor_kind, scope, correlation_id, causation_id, payload, status, created_at) VALUES(?,?,?,?,?,?,?,?,?,'processing',?)`,
		c.ID, c.Type, c.Version, c.Actor.ID, c.Actor.Kind, c.Scope, c.CorrelationID, nullable(c.CausationID), c.Payload, c.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert command: %w", err)
	}
	return nil
}

func compareAndAdvanceAggregate(ctx context.Context, tx *sql.Tx, id, typ string, expected, next int64) error {
	if expected == 0 {
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO aggregate_versions(aggregate_id, aggregate_type, version) VALUES(?,?,?)`, id, typ, next)
		if err != nil {
			return fmt.Errorf("initialize aggregate: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			return nil
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE aggregate_versions SET version=? WHERE aggregate_id=? AND aggregate_type=? AND version=?`, next, id, typ, expected)
	if err != nil {
		return fmt.Errorf("advance aggregate: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return ErrAggregateVersion
	}
	return nil
}

func consumeApproval(ctx context.Context, tx *sql.Tx, id string, now time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE approvals SET remaining_uses=remaining_uses-1, version=version+1 WHERE approval_id=? AND revoked_at IS NULL AND remaining_uses>0 AND (expires_at IS NULL OR expires_at>?)`, id, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("consume approval: %w", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrApprovalUnavailable
	}
	return nil
}

func consumeLease(ctx context.Context, tx *sql.Tx, id string, now time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE capability_leases SET remaining_uses=CASE WHEN remaining_uses IS NULL THEN NULL ELSE remaining_uses-1 END, version=version+1 WHERE lease_id=? AND revoked_at IS NULL AND (remaining_uses IS NULL OR remaining_uses>0) AND (expires_at IS NULL OR expires_at>?)`, id, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("consume lease: %w", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrLeaseUnavailable
	}
	return nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, e EventRecord) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO events(event_id, aggregate_id, aggregate_type, aggregate_version, event_type, event_version, actor_id, actor_kind, command_id, correlation_id, causation_id, provenance_json, trust_class, payload, created_at, integrity_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.AggregateID, e.AggregateType, e.AggregateVersion, e.Type, e.Version, e.Actor.ID, e.Actor.Kind, e.CommandID, e.CorrelationID, nullable(e.CausationID), bytesOrNil(e.ProvenanceJSON), nullable(string(e.TrustClass)), e.Payload, e.CreatedAt.UTC().Format(time.RFC3339Nano), bytesOrNil(e.IntegrityJSON))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func insertEffect(ctx context.Context, tx *sql.Tx, e EffectRecord) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO effects(effect_id, command_id, action_intent_digest, target_adapter, target_principal, capability_lease_id, approval_id, idempotency_key, preconditions_json, crypto_profile, state, request_payload, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.CommandID, e.ActionIntentDigest, e.TargetAdapter, nullable(e.TargetPrincipal), nullable(e.CapabilityLeaseID), nullable(e.ApprovalID), nullable(e.IdempotencyKey), bytesOrNil(e.PreconditionsJSON), nullable(string(e.CryptoProfile)), e.State, e.RequestPayload, e.CreatedAt.UTC().Format(time.RFC3339Nano), e.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert effect: %w", err)
	}
	return nil
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func bytesOrNil(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}
