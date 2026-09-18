package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/internal/plugin"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// ConsumePluginLease atomically verifies and consumes one use of a capability
// lease bound to the exact plugin instance/runtime session. It is intended to
// run immediately before dispatch; callers must not cache a successful result.
func (s *Store) ConsumePluginLease(ctx context.Context, binding plugin.LeaseBinding, instance plugin.InstanceIdentity, req capability.Request, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := binding.Evaluate(instance, req, now); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin plugin lease consumption: %w", err)
	}
	defer tx.Rollback()

	var principalID, principalKind, capabilityName, scope string
	var operationsJSON []byte
	var instanceID, sessionID sql.NullString
	var expiresAt, revokedAt sql.NullString
	var remainingUses sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT principal_id,principal_kind,capability,operations_json,scope,expires_at,revoked_at,remaining_uses,bound_instance_id,bound_session_id FROM capability_leases WHERE lease_id=?`, binding.Lease.ID).Scan(
		&principalID, &principalKind, &capabilityName, &operationsJSON, &scope, &expiresAt, &revokedAt, &remainingUses, &instanceID, &sessionID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseUnavailable
		}
		return fmt.Errorf("load plugin lease: %w", err)
	}

	if principalID != binding.Lease.Principal.ID || principalKind != binding.Lease.Principal.Kind || capabilityName != binding.Lease.Capability || scope != binding.Lease.Scope {
		return errors.New("persisted plugin lease differs from supplied binding")
	}
	if !instanceID.Valid || !sessionID.Valid || instanceID.String != instance.InstanceID || sessionID.String != instance.RuntimeSession {
		return errors.New("persisted capability lease is not bound to this plugin instance/session")
	}
	if revokedAt.Valid || (remainingUses.Valid && remainingUses.Int64 <= 0) {
		return ErrLeaseUnavailable
	}

	var operations []string
	if err := json.Unmarshal(operationsJSON, &operations); err != nil || len(operations) == 0 {
		return errors.New("persisted plugin lease operations are invalid")
	}
	persisted := contracts.CapabilityLease{ID: binding.Lease.ID, Principal: contracts.PrincipalRef{ID: principalID, Kind: principalKind}, Capability: capabilityName, Operations: operations, Scope: scope}
	if expiresAt.Valid {
		expiry, err := time.Parse(time.RFC3339Nano, expiresAt.String)
		if err != nil { return fmt.Errorf("parse plugin lease expiry: %w", err) }
		persisted.ExpiresAt = &expiry
	}
	if remainingUses.Valid { u := uint64(remainingUses.Int64); persisted.RemainingUses = &u }
	if err := capability.Evaluate(persisted, req); err != nil { return err }

	res, err := tx.ExecContext(ctx, `UPDATE capability_leases SET remaining_uses=CASE WHEN remaining_uses IS NULL THEN NULL ELSE remaining_uses-1 END, version=version+1 WHERE lease_id=? AND bound_instance_id=? AND bound_session_id=? AND revoked_at IS NULL AND (remaining_uses IS NULL OR remaining_uses>0) AND (expires_at IS NULL OR expires_at>?)`, binding.Lease.ID, instance.InstanceID, instance.RuntimeSession, now.UTC().Format(time.RFC3339Nano))
	if err != nil { return fmt.Errorf("consume plugin lease: %w", err) }
	changed, err := res.RowsAffected()
	if err != nil { return fmt.Errorf("inspect plugin lease consumption: %w", err) }
	if changed != 1 { return ErrLeaseUnavailable }
	if err := tx.Commit(); err != nil { return fmt.Errorf("commit plugin lease consumption: %w", err) }
	return nil
}
