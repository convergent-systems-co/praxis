package goalstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// ErrSafetyActivationRequired is returned when a durable mutation whose
// correctness depends on the pre-v4 safety kernel is attempted without an
// activation verifier, or when the verifier refuses the current activation.
var ErrSafetyActivationRequired = errors.New("safety-bearing WorkPlan mutation requires current verified kernel activation")

// SafetyActivationVerifier proves that the exact safety kernel a plan binds is
// the one currently active. Implementations must fail closed on absent or
// skewed activation evidence; they never repair or refresh it.
type SafetyActivationVerifier interface {
	Verify(context.Context, contracts.WorkPlanSafetyBinding) error
}

// requireSafetyActivation is the persistence-boundary predicate shared by
// every safety-dependent mutation, so no CLI or orchestration path can reach
// durable state without it. A nil binding means the artifact is not
// safety-bearing and the predicate does not apply. A safety-bearing artifact
// with no configured verifier is refused rather than assumed active.
func (r Repository) requireSafetyActivation(ctx context.Context, binding *contracts.WorkPlanSafetyBinding) error {
	if binding == nil {
		return nil
	}
	if r.SafetyActivation == nil {
		return fmt.Errorf("%w: no activation verifier is configured", ErrSafetyActivationRequired)
	}
	if err := r.SafetyActivation.Verify(ctx, *binding); err != nil {
		return fmt.Errorf("%w: %v", ErrSafetyActivationRequired, err)
	}
	return nil
}

// safetyBindingForRequest resolves the activation binding an authority
// request depends on from durable, immutable state: a WorkPlan acceptance
// depends on its exact proposal, a goal gate on the Goal generation's
// accepted WorkPlan. Other request kinds do not depend on the kernel.
// found=false with no error means the referenced record does not exist yet;
// the mutation that would consume it re-verifies against the record itself.
func (r Repository) safetyBindingForRequest(ctx context.Context, request contracts.AuthorityRequest) (*contracts.WorkPlanSafetyBinding, error) {
	switch {
	case request.RequestedAuthority == contracts.GovernedWorkPlanAccept && request.ProposalID != "":
		proposal, err := r.LoadWorkPlanProposal(ctx, request.ProposalID, request.ProposalVersion, time.Now().UTC())
		if err != nil {
			if errors.Is(err, state.ErrSecureBlobNotFound) {
				// A protected request exists only for the safety kernel (frozen
				// plan section 9). Its governed context is the proposal it
				// names; without it the activation requirement cannot be
				// established, so it must fail closed rather than pass through
				// as if no activation were required (I14).
				if request.CeremonyProfile != "" {
					return nil, fmt.Errorf("a protected request's proposal %s/%s is missing, so its governed context cannot be established: %w", request.ProposalID, request.ProposalVersion, err)
				}
				return nil, nil
			}
			return nil, fmt.Errorf("resolve safety binding for WorkPlan request: %w", err)
		}
		return proposal.Safety, nil
	case request.RequestedAuthority == "goal.gate.decide":
		baseline, err := r.Load(ctx, request.BaselineID, request.BaselineVersion, time.Now().UTC())
		if err != nil {
			return nil, fmt.Errorf("resolve safety binding for goal gate: %w", err)
		}
		if baseline.Digest != request.BaselineDigest || baseline.WorkPlan == nil || baseline.WorkPlan.Safety == nil {
			return nil, errors.New("goal gate is not bound to a safety-bearing accepted WorkPlan of the exact Goal generation")
		}
		return baseline.WorkPlan.Safety, nil
	}
	return nil, nil
}
