package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// RuntimeRecoveryPlanSpec is the construction boundary that guarantees the
// storage_schema and runtime_state transitions are cryptographically committed
// as two ordered steps of one LifecyclePlan.
type RuntimeRecoveryPlanSpec struct {
	PlanID, PlanVersion, InstallationID         string
	CurrentManifestDigest, TargetManifestDigest string
	StorageSchemaStep, RuntimeStateStep         contracts.LifecycleTransitionStep
	PreservedHistory                            []contracts.LifecycleEvidenceRef
	ExpectedReadiness                           contracts.LifecycleReadiness
}

func NewRuntimeRecoveryPlan(spec RuntimeRecoveryPlanSpec) (contracts.LifecyclePlan, error) {
	if spec.StorageSchemaStep.Class != contracts.LifecycleSchema || spec.RuntimeStateStep.Class != contracts.LifecycleRuntime {
		return contracts.LifecyclePlan{}, errors.New("runtime recovery plan requires storage_schema then runtime_state steps")
	}
	if spec.StorageSchemaStep.Sequence >= spec.RuntimeStateStep.Sequence {
		return contracts.LifecyclePlan{}, errors.New("runtime recovery plan step order must be storage_schema before runtime_state")
	}
	plan := contracts.LifecyclePlan{
		PlanID: spec.PlanID, PlanVersion: spec.PlanVersion, InstallationID: spec.InstallationID,
		CurrentManifestDigest: spec.CurrentManifestDigest, TargetManifestDigest: spec.TargetManifestDigest,
		Steps:            []contracts.LifecycleTransitionStep{spec.StorageSchemaStep, spec.RuntimeStateStep},
		PreservedHistory: spec.PreservedHistory, SnapshotRequired: spec.StorageSchemaStep.SnapshotRequired || spec.RuntimeStateStep.SnapshotRequired,
		ExpectedReadiness: spec.ExpectedReadiness,
	}
	digest, err := plan.ComputeDigest()
	if err != nil {
		return contracts.LifecyclePlan{}, fmt.Errorf("digest runtime recovery plan: %w", err)
	}
	plan.Digest = digest
	if err := plan.Validate(); err != nil {
		return contracts.LifecyclePlan{}, fmt.Errorf("validate runtime recovery plan: %w", err)
	}
	return plan, nil
}

type runtimeStateRunBinding struct {
	plan              contracts.LifecyclePlan
	runtimeStateStep  string
	storageSchemaStep string
}

// RuntimeStatePrerequisite implements WU6's sequencing precondition for the
// WU7 runtime_state driver. BindRunRequest receives the exact plan already
// verified by RunStep; Preflight then reuses lastStepState's WU2 identity
// lookup and requires the storage_schema outcome under that same plan digest.
type RuntimeStatePrerequisite struct {
	Journal *Journal
	binding *runtimeStateRunBinding
}

func (p *RuntimeStatePrerequisite) BindRunRequest(_ context.Context, req RunRequest, step contracts.LifecycleTransitionStep, _ *contracts.AuthorityDecision) error {
	if step.Class != contracts.LifecycleRuntime || req.StepID != step.ID {
		return errors.New("runtime_state prerequisite must bind the exact runtime_state step")
	}
	var storage *contracts.LifecycleTransitionStep
	for i := range req.Plan.Steps {
		candidate := &req.Plan.Steps[i]
		if candidate.Class == contracts.LifecycleSchema && candidate.Sequence < step.Sequence {
			if storage != nil {
				return errors.New("runtime recovery plan has ambiguous storage_schema prerequisite")
			}
			storage = candidate
		}
	}
	if storage == nil {
		return errors.New("runtime_state step lacks a prior storage_schema step in the same plan")
	}
	p.binding = &runtimeStateRunBinding{plan: req.Plan, runtimeStateStep: step.ID, storageSchemaStep: storage.ID}
	return nil
}

func (p *RuntimeStatePrerequisite) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	if p == nil || p.Journal == nil || p.binding == nil {
		return errors.New("runtime_state prerequisite is not bound to a journal and RunRequest")
	}
	if step.ID != p.binding.runtimeStateStep {
		return errors.New("runtime_state prerequisite step binding mismatch")
	}
	history, err := p.Journal.Load(ctx)
	if err != nil {
		return fmt.Errorf("load storage_schema journal prerequisite: %w", err)
	}
	state, planDigest, err := lastStepState(history, p.binding.plan.PlanID, p.binding.storageSchemaStep)
	if err != nil {
		return err
	}
	if state == "" {
		return errors.New("runtime_state requires storage_schema to commit first")
	}
	if planDigest != p.binding.plan.Digest {
		return ErrPlanDigestMismatch
	}
	if state != contracts.LifecycleCommitted {
		return fmt.Errorf("runtime_state requires committed storage_schema prerequisite, got %s", state)
	}
	return nil
}
