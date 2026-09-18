package kernel

import (
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/approval"
	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/internal/policy"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// PreflightRequest contains only deterministic inputs required before a
// security-sensitive command can be committed or dispatched.
type PreflightRequest struct {
	Intent          contracts.ActionIntent
	Approval        *contracts.ApprovalBinding
	Lease           *contracts.CapabilityLease
	PolicyRules     []policy.Rule
	Capability      string
	Operation       string
	Scope           string
	Now             time.Time
	RequireApproval bool
	RequireLease    bool
	RequirePolicy   bool
}

// PreflightResult captures the exact authorization material that must be
// persisted with the subsequent authoritative transition.
type PreflightResult struct {
	IntentDigest string
	ApprovalID   string
	LeaseID      string
	PolicyRuleID string
	PolicyVersion string
}

func Preflight(req PreflightRequest) (PreflightResult, error) {
	if err := req.Intent.Validate(); err != nil {
		return PreflightResult{}, fmt.Errorf("invalid action intent: %w", err)
	}
	digest, err := req.Intent.Digest()
	if err != nil {
		return PreflightResult{}, fmt.Errorf("digest action intent: %w", err)
	}

	capabilityName := req.Capability
	operation := req.Operation
	if operation == "" {
		operation = req.Intent.Operation
	}
	scope := req.Scope
	if scope == "" {
		scope = req.Intent.Scope
	}
	result := PreflightResult{IntentDigest: digest}

	policyRequiresApproval := false
	if req.RequirePolicy {
		if capabilityName == "" {
			return PreflightResult{}, errors.New("capability is required when policy evaluation is required")
		}
		decision, err := policy.Evaluate(req.PolicyRules, policy.Request{
			Actor: req.Intent.Actor, Capability: capabilityName, Operation: operation, Scope: scope,
		})
		if err != nil {
			return PreflightResult{}, fmt.Errorf("policy evaluation failed: %w", err)
		}
		result.PolicyRuleID = decision.RuleID
		result.PolicyVersion = decision.RuleVersion
		switch decision.Effect {
		case policy.Deny:
			return PreflightResult{}, fmt.Errorf("policy denied action: %s", decision.ReasonCode)
		case policy.RequireApproval:
			policyRequiresApproval = true
		case policy.Allow:
		default:
			return PreflightResult{}, errors.New("policy returned unknown effect")
		}
	}

	if req.RequireApproval || policyRequiresApproval {
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
		if capabilityName == "" {
			return PreflightResult{}, errors.New("capability is required when lease is required")
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
