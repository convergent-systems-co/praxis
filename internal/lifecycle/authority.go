package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// AuthoritySource is the narrow existing durable authority boundary used by
// lifecycle transitions. Implementations must apply current expiry,
// revocation, and generation-lineage rules when loading/validating records.
type AuthoritySource interface {
	LoadAuthorityRequest(context.Context, string, string, time.Time) (contracts.AuthorityRequest, error)
	LoadAuthorityDecision(context.Context, string, string, time.Time) (contracts.AuthorityDecision, error)
	ValidateAuthorityGeneration(context.Context, contracts.AuthorityDecision, time.Time) error
}

// DurableAuthorityValidator binds an accepted lifecycle step to the exact
// durable request, decision, and issuing generation. It derives no authority.
type DurableAuthorityValidator struct {
	Source AuthoritySource
	Now    func() time.Time
}

func (v DurableAuthorityValidator) ValidateLifecycleAuthority(ctx context.Context, plan contracts.LifecyclePlan, step contracts.LifecycleTransitionStep) (contracts.AuthorityDecision, error) {
	if v.Source == nil {
		return contracts.AuthorityDecision{}, errors.New("durable lifecycle authority source is required")
	}
	req := step.Authority
	if !req.Required || plan.PlanID == "" || plan.PlanVersion == "" || plan.Digest == "" || step.ID == "" {
		return contracts.AuthorityDecision{}, errors.New("lifecycle authority subject does not bind the exact plan step")
	}
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	request, err := v.Source.LoadAuthorityRequest(ctx, req.RequestRef, req.RequestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("load lifecycle authority request: %w", err)
	}
	requestDigest, err := request.Digest()
	if err != nil || requestDigest != req.RequestDigest || request.RequestedAuthority != req.Operation || request.RequestedScope != req.Scope {
		return contracts.AuthorityDecision{}, errors.New("lifecycle authority request binding mismatch")
	}
	decision, err := v.Source.LoadAuthorityDecision(ctx, req.RequestRef, req.RequestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("load lifecycle authority decision: %w", err)
	}
	decisionDigest, err := decision.Digest()
	if err != nil || decisionDigest != req.DecisionDigest || decision.DecisionRef != req.DecisionRef || decision.DecisionVersion != req.DecisionVersion || decision.Outcome != contracts.AuthorityApprove || decision.GrantedScope != req.Scope || decision.AuthorityRef != req.AuthorityRef || decision.AuthorityVersion != req.AuthorityVersion || decision.AuthorityGenerationDigest != req.AuthorityGenerationDigest {
		return contracts.AuthorityDecision{}, errors.New("lifecycle authority decision binding mismatch")
	}
	if err := v.Source.ValidateAuthorityGeneration(ctx, decision, now); err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("validate lifecycle authority generation: %w", err)
	}
	return decision, nil
}
