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
	// observationResolutionFault is test-only failure injection. It is nil in
	// production and cannot alter normal transaction semantics.
	observationResolutionFault func(string) error
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

// CommitTransitionGuarded is CommitTransition with one additional generic
// hook: guard runs inside the same SQLite transaction, on the same tx handle
// (never a second pooled connection — this store's pool is capped at one
// connection, so any nested query against Store.DB() while this transaction
// is open would deadlock), after the command insert and aggregate advance
// (both writes, so this transaction already holds SQLite's single-writer
// lock, acquired immediately per _txlock=immediate) and strictly before the
// event/effect insert. Because SQLite allows only one writer at a time, no
// concurrent write (for example, an authority revocation or an
// expiry-relevant update) can be committed by another transaction between
// guard's check and this transaction's own commit; guard therefore observes
// authority state that is guaranteed to still be current at the moment
// durable execution admission (the event/effect insert) actually happens.
// If guard returns an error, the whole transaction — including the command
// insert — is rolled back and no command, event, or effect is admitted.
//
// This mirrors the existing precedent in CommitTransitionAuthorizedLease,
// which validates+consumes a capability lease inside the same transaction as
// the aggregate advance and command/event insert via the tx handle. Use this
// generic hook when the authorization re-check is not itself expressible as
// a single conditional UPDATE (as a capability lease consumption is), but
// still needs the same atomicity guarantee against durable admission; guard
// must perform its own checks using the supplied *sql.Tx exclusively.
func (s *Store) CommitTransitionGuarded(
	ctx context.Context,
	cmd CommandRecord,
	expectedAggregateVersion int64,
	event EventRecord,
	approvalID string,
	leaseID string,
	effect *EffectRecord,
	guard func(ctx context.Context, tx *sql.Tx) error,
) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
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
		return fmt.Errorf("begin guarded transition: %w", err)
	}
	defer tx.Rollback()

	if err := insertCommand(ctx, tx, cmd); err != nil {
		return err
	}
	if err := compareAndAdvanceAggregate(ctx, tx, event.AggregateID, event.AggregateType, expectedAggregateVersion, event.AggregateVersion); err != nil {
		return err
	}
	if guard != nil {
		if err := guard(ctx, tx); err != nil {
			return fmt.Errorf("guarded transition authorization check failed: %w", err)
		}
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
		return fmt.Errorf("commit guarded transition: %w", err)
	}
	return nil
}

// CommitGoalsPublicationAbandonment serializes the fixed-case owner fence with
// publication dispatch claims. It records only command/event history and does
// not rewrite effects or authority generations.
func (s *Store) CommitGoalsPublicationAbandonment(ctx context.Context, cmd CommandRecord, event EventRecord, requestID string) error {
	if s == nil || s.db == nil || requestID == "" || cmd.ID != event.ID || event.Type != "goals-publication.abandoned" {
		return errors.New("invalid Goals publication abandonment")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var claimed int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM effects e JOIN commands c ON c.command_id=e.command_id WHERE c.command_type='goals-publication.step' AND c.correlation_id=? AND e.state='dispatched'`, requestID).Scan(&claimed); err != nil {
		return err
	}
	if claimed != 0 {
		return errors.New("publication dispatch is in flight; abandonment refused")
	}
	if err = insertCommand(ctx, tx, cmd); err != nil {
		return err
	}
	if err = compareAndAdvanceAggregate(ctx, tx, event.AggregateID, event.AggregateType, 0, 1); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE commands SET status='committed', completed_at=? WHERE command_id=?`, event.CreatedAt.UTC().Format(time.RFC3339Nano), cmd.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// CommitGoalsPublicationRecoveryAbandonment records the recovery-specific
// terminal fence without rewriting any recovery effects or authority state.
func (s *Store) CommitGoalsPublicationRecoveryAbandonment(ctx context.Context, cmd CommandRecord, event EventRecord, executionID string) error {
	if s == nil || s.db == nil || executionID == "" || cmd.ID != event.ID || event.Type != "goals-publication-recovery.abandoned" {
		return errors.New("invalid Goals recovery abandonment")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var claimed int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM effects e JOIN commands c ON c.command_id=e.command_id WHERE c.command_type='goals-publication-recovery.step' AND c.correlation_id=? AND e.state='dispatched'`, executionID).Scan(&claimed); err != nil {
		return err
	}
	if claimed != 0 {
		return errors.New("recovery dispatch is in flight; abandonment refused")
	}
	if err = insertCommand(ctx, tx, cmd); err != nil {
		return err
	}
	if err = compareAndAdvanceAggregate(ctx, tx, event.AggregateID, event.AggregateType, 0, 1); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE commands SET status='committed', completed_at=? WHERE command_id=?`, event.CreatedAt.UTC().Format(time.RFC3339Nano), cmd.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// CommitGoalsPublicationCompletion serializes the old fixed execution's final
// event against its terminal abandonment fence.
func (s *Store) CommitGoalsPublicationCompletion(ctx context.Context, cmd CommandRecord, expected int64, event EventRecord, executionID string) error {
	if s == nil || s.db == nil || executionID == "" || event.Type != "goals-publication.completed" {
		return errors.New("invalid Goals publication completion")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fenced int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id=?`, executionID).Scan(&fenced); err != nil {
		return err
	}
	if fenced != 0 {
		return errors.New("abandoned Goals publication cannot complete")
	}
	if err = insertCommand(ctx, tx, cmd); err != nil {
		return err
	}
	if err = compareAndAdvanceAggregate(ctx, tx, event.AggregateID, event.AggregateType, expected, expected+1); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE commands SET status='committed',completed_at=? WHERE command_id=?`, event.CreatedAt.UTC().Format(time.RFC3339Nano), cmd.ID); err != nil {
		return err
	}
	return tx.Commit()
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
