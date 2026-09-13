package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const RunControlCapability = "run.control"

type RunControlLeaseSource interface {
	LeasesForPrincipal(ctx context.Context, principal contracts.PrincipalRef, capability string) ([]contracts.CapabilityLease, error)
}

// LeaseRunControlAuthorizer authorizes run mutation only when at least one
// current capability lease grants the exact operation for run:<run-id>.
//
// This authorizer is decision-only; it cannot atomically consume a finite-use
// lease in the same transaction as the run event. Finite-use leases therefore
// fail closed here to avoid a check/use race. A future transactional mutation
// gate may safely support consumable run-control leases.
type LeaseRunControlAuthorizer struct {
	Leases RunControlLeaseSource
	Now    func() time.Time
}

func (a LeaseRunControlAuthorizer) AuthorizeRunControl(ctx context.Context, actor contracts.PrincipalRef, run RunExecution, operation RunControlOperation) error {
	if a.Leases == nil {
		return errors.New("run control lease source is required")
	}
	if err := actor.Validate(); err != nil {
		return err
	}
	if run.RunID == "" {
		return errors.New("run id is required")
	}
	leases, err := a.Leases.LeasesForPrincipal(ctx, actor, RunControlCapability)
	if err != nil {
		return fmt.Errorf("load run control leases: %w", err)
	}
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	request := capability.Request{
		Principal:  actor,
		Capability: RunControlCapability,
		Operation:  string(operation),
		Scope:      "run:" + run.RunID,
		Now:        now,
	}
	var lastErr error
	for _, lease := range leases {
		if lease.RemainingUses != nil {
			lastErr = errors.New("finite-use run control lease requires atomic consumption")
			continue
		}
		if err := capability.Evaluate(lease, request); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return fmt.Errorf("no run control lease authorized request: %w", lastErr)
	}
	return errors.New("no run control lease authorized request")
}
