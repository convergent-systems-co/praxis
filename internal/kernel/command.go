package kernel

import (
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/approval"
	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// PreflightRequest contains only deterministic inputs required before a
// security-sensitive command can be committed or dispatched.
type PreflightRequest struct {
	Intent        contracts.ActionIntent
	Approval      *contracts.ApprovalBinding
	Lease         *contracts.CapabilityLease
	Capability    string
	Operation     string
	Scope         string
	Now           time.Time
	RequireApproval bool
	RequireLease    bool
}

// PreflightResult captures the exact authorization material that must be
// persisted with the subsequent authoritative transition.
type PreflightResult struct {
	IntentDigest string
	ApprovalID   string
	LeaseID      string
}

func Preflight(req PreflightRequest) (PreflightResult, error) {
	if err := req.Intent.Validate(); err != nil {
		return PreflightResult{}, fmt.Errorf("invalid action intent: %w", err)
	}
	digest, err := req.Intent.Digest()
	if err != nil {
		return PreflightResult{}, fmt.Errorf("digest action intent: %w", err)
	}

	result := PreflightResult{IntentDigest: digest}

	if req.RequireApproval {
		if req.Approval == nil {
			return PreflightResult{}, errors.New("approval required")
		}
		if err := approval.ValidateExact(*req.Approval, req.Intent, req.Now); err != nil {
			return PreflightResult{}, fmt.Errorf("approval denied: %w", err)
		}
		result.ApprovalID = req.Approval.ID
	}

	if req.RequireLease {
		if req.Lease == nil {
			return PreflightResult{}, errors.New("capability lease required")
		}
		capabilityName := req.Capability
		if capabilityName == "" {
			return PreflightResult{}, errors.New("capability is required when lease is required")
		}
		operation := req.Operation
		if operation == "" {
			operation = req.Intent.Operation
		}
		scope := req.Scope
		if scope == "" {
			scope = req.Intent.Scope
		}
		if err := capability.Evaluate(*req.Lease, capability.Request{
			Principal: req.Intent.Actor,
			Capability: capabilityName,
			Operation: operation,
			Scope: scope,
			Now: req.Now,
		}); err != nil {
			return PreflightResult{}, fmt.Errorf("capability denied: %w", err)
		}
		result.LeaseID = req.Lease.ID
	}

	return result, nil
}
