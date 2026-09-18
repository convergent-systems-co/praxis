package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// CommitTransitionAuthorizedLease performs capability selection, validation,
// consumption, aggregate advancement, command persistence, and event append in
// one SQLite transaction. This is the final authority boundary for mutations
// whose authorization is represented by a capability lease.
func (s *Store) CommitTransitionAuthorizedLease(
	ctx context.Context,
	cmd CommandRecord,
	expectedAggregateVersion int64,
	event EventRecord,
	requirement capability.Request,
) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("state store is required")
	}
	if cmd.ID == "" || event.ID == "" || event.AggregateID == "" {
		return "", errors.New("command, event, and aggregate identity are required")
	}
	if event.CommandID != cmd.ID {
		return "", errors.New("event command id does not match command")
	}
	if event.AggregateVersion != expectedAggregateVersion+1 {
		return "", fmt.Errorf("event aggregate version must be expected+1: expected %d got %d", expectedAggregateVersion+1, event.AggregateVersion)
	}
	if cmd.Actor != event.Actor || cmd.Actor != requirement.Principal {
		return "", errors.New("command, event, and capability principal must match")
	}
	if cmd.CreatedAt.IsZero() || event.CreatedAt.IsZero() {
		return "", errors.New("command and event timestamps are required")
	}
	requirement.Now = cmd.CreatedAt.UTC()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin authorized transition: %w", err)
	}
	defer tx.Rollback()

	if err := insertCommand(ctx, tx, cmd); err != nil {
		return "", err
	}
	if err := compareAndAdvanceAggregate(ctx, tx, event.AggregateID, event.AggregateType, expectedAggregateVersion, event.AggregateVersion); err != nil {
		return "", err
	}
	leaseID, err := selectAndConsumeCapabilityLease(ctx, tx, requirement)
	if err != nil {
		return "", err
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE commands SET status='committed', completed_at=? WHERE command_id=?`, event.CreatedAt.UTC().Format(time.RFC3339Nano), cmd.ID); err != nil {
		return "", fmt.Errorf("complete command: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit authorized transition: %w", err)
	}
	return leaseID, nil
}

func selectAndConsumeCapabilityLease(ctx context.Context, tx *sql.Tx, req capability.Request) (string, error) {
	if err := req.Principal.Validate(); err != nil {
		return "", err
	}
	if req.Capability == "" || req.Operation == "" || req.Scope == "" || req.Now.IsZero() {
		return "", errors.New("capability request is incomplete")
	}
	rows, err := tx.QueryContext(ctx, `SELECT lease_id,operations_json,scope,issued_at,expires_at,revoked_at,remaining_uses,required_enforcement_json,version FROM capability_leases WHERE principal_id=? AND principal_kind=? AND capability=? ORDER BY lease_id`, req.Principal.ID, req.Principal.Kind, req.Capability)
	if err != nil {
		return "", fmt.Errorf("load candidate capability leases: %w", err)
	}
	type candidate struct {
		lease   contracts.CapabilityLease
		version int64
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var id, scope, issued string
		var operationsJSON, enforcementJSON []byte
		var expires, revoked sql.NullString
		var remaining sql.NullInt64
		var version int64
		if err := rows.Scan(&id, &operationsJSON, &scope, &issued, &expires, &revoked, &remaining, &enforcementJSON, &version); err != nil {
			rows.Close()
			return "", fmt.Errorf("scan candidate capability lease: %w", err)
		}
		var operations []string
		if err := json.Unmarshal(operationsJSON, &operations); err != nil {
			rows.Close()
			return "", fmt.Errorf("decode lease %q operations: %w", id, err)
		}
		issuedAt, err := time.Parse(time.RFC3339Nano, issued)
		if err != nil {
			rows.Close()
			return "", fmt.Errorf("parse lease %q issued_at: %w", id, err)
		}
		lease := contracts.CapabilityLease{ID: id, Principal: req.Principal, Capability: req.Capability, Operations: operations, Scope: scope, IssuedAt: issuedAt.UTC()}
		if expires.Valid {
			value, err := time.Parse(time.RFC3339Nano, expires.String)
			if err != nil { rows.Close(); return "", fmt.Errorf("parse lease %q expiry: %w", id, err) }
			value = value.UTC(); lease.ExpiresAt = &value
		}
		if revoked.Valid {
			value, err := time.Parse(time.RFC3339Nano, revoked.String)
			if err != nil { rows.Close(); return "", fmt.Errorf("parse lease %q revocation: %w", id, err) }
			value = value.UTC(); lease.RevokedAt = &value
		}
		if remaining.Valid {
			if remaining.Int64 < 0 { rows.Close(); return "", fmt.Errorf("lease %q has negative remaining uses", id) }
			value := uint64(remaining.Int64); lease.RemainingUses = &value
		}
		if len(enforcementJSON) != 0 {
			if err := json.Unmarshal(enforcementJSON, &lease.RequiredEnforcement); err != nil {
				rows.Close(); return "", fmt.Errorf("decode lease %q enforcement: %w", id, err)
			}
		}
		if err := capability.Evaluate(lease, req); err == nil {
			candidates = append(candidates, candidate{lease: lease, version: version})
		}
	}
	if err := rows.Close(); err != nil {
		return "", fmt.Errorf("close capability lease rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate candidate capability leases: %w", err)
	}
	if len(candidates) == 0 {
		return "", ErrLeaseUnavailable
	}

	selected := candidates[0]
	result, err := tx.ExecContext(ctx, `UPDATE capability_leases SET remaining_uses=CASE WHEN remaining_uses IS NULL THEN NULL ELSE remaining_uses-1 END, version=version+1 WHERE lease_id=? AND version=? AND revoked_at IS NULL AND (remaining_uses IS NULL OR remaining_uses>0) AND (expires_at IS NULL OR expires_at>?)`, selected.lease.ID, selected.version, req.Now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return "", fmt.Errorf("consume authorized capability lease: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("inspect authorized capability lease consumption: %w", err)
	}
	if changed != 1 {
		return "", ErrLeaseUnavailable
	}
	return selected.lease.ID, nil
}
