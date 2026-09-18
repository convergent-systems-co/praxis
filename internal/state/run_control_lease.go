package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// LeasesForPrincipal returns persisted leases for deterministic capability
// evaluation. It does not consume a lease; run-control operations are state
// transitions and lease consumption, when required by policy, remains a
// separate authoritative transition concern.
func (s *Store) LeasesForPrincipal(ctx context.Context, principal contracts.PrincipalRef, capabilityName string) ([]contracts.CapabilityLease, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	if err := principal.Validate(); err != nil {
		return nil, err
	}
	if capabilityName == "" {
		return nil, errors.New("capability name is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT lease_id,operations_json,scope,issued_at,expires_at,revoked_at,remaining_uses,required_enforcement_json FROM capability_leases WHERE principal_id=? AND principal_kind=? AND capability=?`, principal.ID, principal.Kind, capabilityName)
	if err != nil {
		return nil, fmt.Errorf("load capability leases: %w", err)
	}
	defer rows.Close()

	out := make([]contracts.CapabilityLease, 0)
	for rows.Next() {
		var id, scope, issued string
		var operationsJSON []byte
		var expires, revoked sql.NullString
		var remaining sql.NullInt64
		var enforcementJSON []byte
		if err := rows.Scan(&id, &operationsJSON, &scope, &issued, &expires, &revoked, &remaining, &enforcementJSON); err != nil {
			return nil, fmt.Errorf("scan capability lease: %w", err)
		}
		var operations []string
		if err := json.Unmarshal(operationsJSON, &operations); err != nil || len(operations) == 0 {
			return nil, fmt.Errorf("lease %q has invalid operations", id)
		}
		issuedAt, err := time.Parse(time.RFC3339Nano, issued)
		if err != nil {
			return nil, fmt.Errorf("parse lease %q issued_at: %w", id, err)
		}
		lease := contracts.CapabilityLease{ID: id, Principal: principal, Capability: capabilityName, Operations: operations, Scope: scope, IssuedAt: issuedAt.UTC()}
		if expires.Valid {
			value, err := time.Parse(time.RFC3339Nano, expires.String)
			if err != nil {
				return nil, fmt.Errorf("parse lease %q expiry: %w", id, err)
			}
			value = value.UTC()
			lease.ExpiresAt = &value
		}
		if revoked.Valid {
			value, err := time.Parse(time.RFC3339Nano, revoked.String)
			if err != nil {
				return nil, fmt.Errorf("parse lease %q revocation: %w", id, err)
			}
			value = value.UTC()
			lease.RevokedAt = &value
		}
		if remaining.Valid {
			if remaining.Int64 < 0 {
				return nil, fmt.Errorf("lease %q has negative remaining uses", id)
			}
			value := uint64(remaining.Int64)
			lease.RemainingUses = &value
		}
		if len(enforcementJSON) != 0 {
			if err := json.Unmarshal(enforcementJSON, &lease.RequiredEnforcement); err != nil {
				return nil, fmt.Errorf("lease %q has invalid required enforcement", id)
			}
		}
		out = append(out, lease)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capability leases: %w", err)
	}
	return out, nil
}
