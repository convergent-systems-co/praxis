package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	ExactDispatchCapability = "inference.executor.dispatch"
	ExactDispatchOperation  = "dispatch"
)

var ErrExactDispatchAuthorityUnavailable = errors.New("exact dispatch authority unavailable")

type ExactDispatchGrant struct {
	IntentDigest string
	ApprovalID   string
	LeaseID      string
}

type DispatchInvocation struct {
	EffectID     string
	IntentDigest string
	ApprovalID   string
	LeaseID      string
}

func ExactDispatchScope(intentDigest string) string { return "dispatch-intent:" + intentDigest }

// IssueExactDispatchAuthority is the protected persistence primitive beneath
// policy-specific issuance paths. It creates no routing authority and accepts
// only a one-shot human approval paired with a one-shot exact-scope lease.
func (s *Store) IssueExactDispatchAuthority(ctx context.Context, intent contracts.ActionIntent, approver contracts.PrincipalRef, issuedAt time.Time, expiresAt time.Time) (ExactDispatchGrant, error) {
	if s == nil || s.db == nil {
		return ExactDispatchGrant{}, errors.New("state store is required")
	}
	if err := validateExactDispatchIntent(intent); err != nil {
		return ExactDispatchGrant{}, err
	}
	if err := approver.Validate(); err != nil || approver.Kind != "human" {
		return ExactDispatchGrant{}, errors.New("exact dispatch approval requires a human approver")
	}
	issuedAt = issuedAt.UTC()
	expiresAt = expiresAt.UTC()
	if issuedAt.IsZero() || !expiresAt.After(issuedAt) {
		return ExactDispatchGrant{}, errors.New("exact dispatch grant requires a bounded future expiry")
	}
	digest, err := intent.Digest()
	if err != nil {
		return ExactDispatchGrant{}, err
	}
	canonical, err := intent.CanonicalBytes()
	if err != nil {
		return ExactDispatchGrant{}, err
	}
	suffix := strings.TrimPrefix(digest, "sha256:")
	grant := ExactDispatchGrant{IntentDigest: digest, ApprovalID: "approval:dispatch:" + suffix, LeaseID: "lease:dispatch:" + suffix}
	ops, _ := json.Marshal([]string{ExactDispatchOperation})
	constraints, _ := json.Marshal(map[string]string{"intent_digest": digest})
	enforcement, _ := json.Marshal([]string{"issued-route-revalidation"})

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return ExactDispatchGrant{}, fmt.Errorf("begin exact dispatch issuance: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,expires_at,remaining_uses) VALUES(?,?,?,?,?,?,1)`, grant.ApprovalID, approver.ID, approver.Kind, digest, issuedAt.Format(time.RFC3339Nano), expiresAt.Format(time.RFC3339Nano)); err != nil {
		return ExactDispatchGrant{}, fmt.Errorf("persist exact dispatch approval: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,constraints_json,required_enforcement_json,issued_at,expires_at,remaining_uses) VALUES(?,?,?,?,?,?,?,?,?,?,1)`, grant.LeaseID, intent.Actor.ID, intent.Actor.Kind, ExactDispatchCapability, ops, ExactDispatchScope(digest), constraints, enforcement, issuedAt.Format(time.RFC3339Nano), expiresAt.Format(time.RFC3339Nano)); err != nil {
		return ExactDispatchGrant{}, fmt.Errorf("persist exact dispatch lease: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO exact_dispatch_grants(intent_digest,intent_json,approval_id,lease_id,approver_id,approver_kind,issued_at,expires_at) VALUES(?,?,?,?,?,?,?,?)`, digest, canonical, grant.ApprovalID, grant.LeaseID, approver.ID, approver.Kind, issuedAt.Format(time.RFC3339Nano), expiresAt.Format(time.RFC3339Nano)); err != nil {
		return ExactDispatchGrant{}, fmt.Errorf("persist exact dispatch grant: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ExactDispatchGrant{}, fmt.Errorf("commit exact dispatch issuance: %w", err)
	}
	return grant, nil
}

// AuthorizeExactDispatch atomically revalidates and consumes both exact grants
// while durably recording the invocation intent. It never invokes an executor.
func (s *Store) AuthorizeExactDispatch(ctx context.Context, intent contracts.ActionIntent, now time.Time) (DispatchInvocation, error) {
	if s == nil || s.db == nil {
		return DispatchInvocation{}, errors.New("state store is required")
	}
	if err := validateExactDispatchIntent(intent); err != nil {
		return DispatchInvocation{}, err
	}
	now = now.UTC()
	digest, err := intent.Digest()
	if err != nil {
		return DispatchInvocation{}, err
	}
	canonical, _ := intent.CanonicalBytes()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return DispatchInvocation{}, fmt.Errorf("begin exact dispatch authorization: %w", err)
	}
	defer tx.Rollback()

	var storedIntent, operationsJSON, constraintsJSON, enforcementJSON []byte
	var approvalID, leaseID, approverID, approverKind, principalID, principalKind, capabilityName, scope, grantExpiry string
	var approvalDigest string
	var approvalExpiry, approvalRevoked, leaseExpiry, leaseRevoked, grantRevoked sql.NullString
	var approvalUses, leaseUses int64
	err = tx.QueryRowContext(ctx, `SELECT g.intent_json,g.approval_id,g.lease_id,g.approver_id,g.approver_kind,g.expires_at,g.revoked_at,
		a.intent_digest,a.expires_at,a.revoked_at,a.remaining_uses,
		l.principal_id,l.principal_kind,l.capability,l.operations_json,l.scope,l.constraints_json,l.required_enforcement_json,l.expires_at,l.revoked_at,l.remaining_uses
		FROM exact_dispatch_grants g JOIN approvals a ON a.approval_id=g.approval_id JOIN capability_leases l ON l.lease_id=g.lease_id WHERE g.intent_digest=?`, digest).Scan(
		&storedIntent, &approvalID, &leaseID, &approverID, &approverKind, &grantExpiry, &grantRevoked,
		&approvalDigest, &approvalExpiry, &approvalRevoked, &approvalUses,
		&principalID, &principalKind, &capabilityName, &operationsJSON, &scope, &constraintsJSON, &enforcementJSON, &leaseExpiry, &leaseRevoked, &leaseUses)
	if errors.Is(err, sql.ErrNoRows) {
		return DispatchInvocation{}, ErrExactDispatchAuthorityUnavailable
	}
	if err != nil {
		return DispatchInvocation{}, fmt.Errorf("load exact dispatch authority: %w", err)
	}
	if !bytes.Equal(storedIntent, canonical) || approverID == "" || approverKind != "human" || approvalDigest != digest || approvalUses != 1 || approvalRevoked.Valid || leaseUses != 1 || leaseRevoked.Valid || principalID != intent.Actor.ID || principalKind != intent.Actor.Kind || capabilityName != ExactDispatchCapability || scope != ExactDispatchScope(digest) || grantRevoked.Valid {
		return DispatchInvocation{}, ErrExactDispatchAuthorityUnavailable
	}
	for _, stamp := range []string{grantExpiry, approvalExpiry.String, leaseExpiry.String} {
		expiry, parseErr := time.Parse(time.RFC3339Nano, stamp)
		if parseErr != nil || !now.Before(expiry) {
			return DispatchInvocation{}, ErrExactDispatchAuthorityUnavailable
		}
	}
	var operations, required []string
	var constraints map[string]string
	if json.Unmarshal(operationsJSON, &operations) != nil || len(operations) != 1 || operations[0] != ExactDispatchOperation || json.Unmarshal(constraintsJSON, &constraints) != nil || constraints["intent_digest"] != digest || json.Unmarshal(enforcementJSON, &required) != nil || len(required) != 1 || required[0] != "issued-route-revalidation" {
		return DispatchInvocation{}, ErrExactDispatchAuthorityUnavailable
	}
	lease := contracts.CapabilityLease{ID: leaseID, Principal: intent.Actor, Capability: capabilityName, Operations: operations, Scope: scope, IssuedAt: now.Add(-time.Nanosecond), RemainingUses: uint64Ptr(1)}
	if err := capability.Evaluate(lease, capability.Request{Principal: intent.Actor, Capability: ExactDispatchCapability, Operation: ExactDispatchOperation, Scope: ExactDispatchScope(digest), Now: now}); err != nil {
		return DispatchInvocation{}, ErrExactDispatchAuthorityUnavailable
	}
	approval := contracts.ApprovalBinding{ID: approvalID, Approver: contracts.PrincipalRef{ID: approverID, Kind: approverKind}, IntentDigest: approvalDigest, IssuedAt: now.Add(-time.Nanosecond), RemainingUses: uint64(approvalUses)}
	if err := approval.AuthorizesIntent(intent, now); err != nil {
		return DispatchInvocation{}, ErrExactDispatchAuthorityUnavailable
	}

	suffix := strings.TrimPrefix(digest, "sha256:")
	invocation := DispatchInvocation{EffectID: "effect:dispatch:" + suffix, IntentDigest: digest, ApprovalID: approvalID, LeaseID: leaseID}
	cmdID := "command:dispatch:" + suffix
	runID, routeID := intent.Parameters["run_id"], intent.Parameters["route_record_id"]
	cmd := CommandRecord{ID: cmdID, Type: "inference.executor.dispatch", Version: "1", Actor: intent.Actor, Scope: intent.Scope, CorrelationID: runID, CausationID: routeID, Payload: canonical, CreatedAt: now}
	event := EventRecord{ID: "event:dispatch-authorized:" + suffix, AggregateID: "dispatch:" + digest, AggregateType: "exact_dispatch", AggregateVersion: 1, Type: "inference.dispatch.authorized", Version: "1", Actor: intent.Actor, CommandID: cmdID, CorrelationID: runID, CausationID: routeID, TrustClass: contracts.TrustUserConfirmed, Payload: canonical, CreatedAt: now}
	effect := EffectRecord{ID: invocation.EffectID, CommandID: cmdID, ActionIntentDigest: digest, TargetAdapter: intent.Parameters["executor_id"], TargetPrincipal: intent.Parameters["provider_id"], CapabilityLeaseID: leaseID, ApprovalID: approvalID, PreconditionsJSON: mustJSON(intent.Preconditions), State: string(EffectPending), RequestPayload: canonical, CreatedAt: now, UpdatedAt: now}
	if err := insertCommand(ctx, tx, cmd); err != nil {
		return DispatchInvocation{}, err
	}
	if err := compareAndAdvanceAggregate(ctx, tx, event.AggregateID, event.AggregateType, 0, 1); err != nil {
		return DispatchInvocation{}, err
	}
	if err := consumeExactApproval(ctx, tx, approvalID, digest, now); err != nil {
		return DispatchInvocation{}, err
	}
	if err := consumeExactLease(ctx, tx, leaseID, intent, digest, now); err != nil {
		return DispatchInvocation{}, err
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return DispatchInvocation{}, err
	}
	if err := insertEffect(ctx, tx, effect); err != nil {
		return DispatchInvocation{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE commands SET status='committed',completed_at=? WHERE command_id=?`, now.Format(time.RFC3339Nano), cmdID); err != nil {
		return DispatchInvocation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DispatchInvocation{}, fmt.Errorf("commit exact dispatch authorization: %w", err)
	}
	return invocation, nil
}

func (s *Store) RevokeExactDispatchAuthority(ctx context.Context, intentDigest string, now time.Time) error {
	if s == nil || s.db == nil || intentDigest == "" || now.IsZero() {
		return errors.New("store, intent digest, and revocation time are required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var approvalID, leaseID string
	if err := tx.QueryRowContext(ctx, `SELECT approval_id,lease_id FROM exact_dispatch_grants WHERE intent_digest=? AND revoked_at IS NULL`, intentDigest).Scan(&approvalID, &leaseID); err != nil {
		return ErrExactDispatchAuthorityUnavailable
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE exact_dispatch_grants SET revoked_at=? WHERE intent_digest=?`, stamp, intentDigest); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE approvals SET revoked_at=?,version=version+1 WHERE approval_id=?`, stamp, approvalID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE capability_leases SET revoked_at=?,version=version+1 WHERE lease_id=?`, stamp, leaseID); err != nil {
		return err
	}
	return tx.Commit()
}

func validateExactDispatchIntent(intent contracts.ActionIntent) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	if intent.Version != "v1" || intent.Operation != ExactDispatchCapability || intent.Target == "" {
		return errors.New("exact dispatch intent operation is invalid")
	}
	required := []string{"goal_ref", "graph_id", "graph_version", "node_id", "agent_id", "agent_generation", "run_id", "request_id", "route_record_id", "surface_id", "executor_id", "provider_id"}
	if len(intent.Parameters) != len(required) {
		return errors.New("exact dispatch intent parameters are not closed")
	}
	for _, key := range required {
		if intent.Parameters[key] == "" {
			return fmt.Errorf("exact dispatch intent missing %s", key)
		}
	}
	if intent.Actor.ID != intent.Parameters["agent_id"] || intent.Target != "executor:"+intent.Parameters["executor_id"]+"@provider:"+intent.Parameters["provider_id"] || intent.Scope != "issued-route:"+intent.Parameters["route_record_id"] || intent.Preconditions["request_id"] != intent.Parameters["request_id"] || intent.Preconditions["route_record_id"] != intent.Parameters["route_record_id"] || len(intent.Preconditions) != 2 {
		return errors.New("exact dispatch intent identity or preconditions mismatch")
	}
	return nil
}

func consumeExactApproval(ctx context.Context, tx *sql.Tx, id, digest string, now time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE approvals SET remaining_uses=0,version=version+1 WHERE approval_id=? AND intent_digest=? AND remaining_uses=1 AND revoked_at IS NULL AND expires_at>?`, id, digest, now.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrExactDispatchAuthorityUnavailable
	}
	return nil
}

func consumeExactLease(ctx context.Context, tx *sql.Tx, id string, intent contracts.ActionIntent, digest string, now time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE capability_leases SET remaining_uses=0,version=version+1 WHERE lease_id=? AND principal_id=? AND principal_kind=? AND capability=? AND scope=? AND remaining_uses=1 AND revoked_at IS NULL AND expires_at>?`, id, intent.Actor.ID, intent.Actor.Kind, ExactDispatchCapability, ExactDispatchScope(digest), now.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrExactDispatchAuthorityUnavailable
	}
	return nil
}

func uint64Ptr(v uint64) *uint64 { return &v }
func mustJSON(v any) []byte      { b, _ := json.Marshal(v); return b }
