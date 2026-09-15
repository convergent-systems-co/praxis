package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrDowngradeRefused = errors.New("lifecycle downgrade refused")
	ErrAmbiguousApply   = errors.New("lifecycle apply outcome is ambiguous")
	ErrRetryNotSafe     = errors.New("lifecycle retry is not proven idempotent")
	ErrJournalConflict  = errors.New("lifecycle journal has conflicting history")
)

const lifecycleJournalAggregateType = "lifecycle_transition"

// Journal is the lifecycle-specific view over Praxis's existing append-only
// event store. It does not create authority; it records transition evidence.
type Journal struct {
	store        eventstore.Store
	installation string
	actor        contracts.PrincipalRef
}

func NewJournal(store eventstore.Store, installation string, actor contracts.PrincipalRef) (*Journal, error) {
	if store == nil || installation == "" {
		return nil, errors.New("lifecycle journal requires store and installation")
	}
	if err := actor.Validate(); err != nil {
		return nil, err
	}
	return &Journal{store: store, installation: installation, actor: actor}, nil
}

func (j *Journal) Load(ctx context.Context) ([]contracts.LifecycleTransitionJournal, error) {
	if j == nil || j.store == nil {
		return nil, errors.New("lifecycle journal is required")
	}
	events, err := j.store.LoadAggregate(ctx, lifecycleAggregate(j.installation), 0)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.LifecycleTransitionJournal, 0, len(events))
	for _, event := range events {
		var entry contracts.LifecycleTransitionJournal
		if err := json.Unmarshal(event.Payload, &entry); err != nil {
			return nil, fmt.Errorf("decode lifecycle journal event: %w", err)
		}
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("validate lifecycle journal event: %w", err)
		}
		if entry.InstallationID != j.installation || event.AggregateVersion != int64(len(out)+1) {
			return nil, ErrJournalConflict
		}
		out = append(out, entry)
	}
	return out, nil
}

func (j *Journal) Append(ctx context.Context, entry contracts.LifecycleTransitionJournal) error {
	if j == nil || j.store == nil {
		return errors.New("lifecycle journal is required")
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	if entry.InstallationID != j.installation {
		return errors.New("lifecycle journal installation mismatch")
	}
	history, err := j.Load(ctx)
	if err != nil {
		return err
	}
	if entry.Sequence != len(history)+1 {
		return errors.New("lifecycle journal sequence is not append-only")
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = j.store.Append(ctx, lifecycleAggregate(j.installation), int64(len(history)), []eventstore.Event{{
		ID:            "lifecycle-journal:" + entry.JournalID,
		AggregateType: lifecycleJournalAggregateType,
		Type:          "lifecycle.transition",
		Version:       "v1",
		Actor:         j.actor,
		CommandID:     "lifecycle-transition:" + entry.JournalID,
		CorrelationID: entry.PlanID,
		Trust:         contracts.TrustPolicy,
		Payload:       payload,
		CreatedAt:     entry.RecordedAt,
	}})
	return err
}

func lifecycleAggregate(installation string) string { return "lifecycle:" + installation }

type ApplyOutcome string

const (
	ApplyCommitted         ApplyOutcome = "committed"
	ApplyFailedRecoverable ApplyOutcome = "failed_recoverable"
	ApplyReconcileRequired ApplyOutcome = "reconcile_required"
	ApplyRolledBack        ApplyOutcome = "rolled_back"
)

type ApplyResult struct {
	Outcome                 ApplyOutcome
	ResultingManifestDigest string
	RecoveryAction          string
}

type StepDriver interface {
	// Preflight proves compatibility and refuses downgrade before mutation.
	Preflight(context.Context, contracts.LifecycleTransitionStep) error
	// Idempotent reports whether retrying the exact step/precondition is safe.
	Idempotent(contracts.LifecycleTransitionStep) bool
	Apply(context.Context, contracts.LifecycleTransitionStep) (ApplyResult, error)
}

// AuthorityValidator is the existing authority system's lifecycle adapter.
// It must reload the exact request/decision/generation, validate current
// lineage and revocation state, and bind the decision to plan and step. A
// caller assertion is deliberately not part of this interface.
type AuthorityValidator interface {
	ValidateLifecycleAuthority(context.Context, contracts.LifecyclePlan, contracts.LifecycleTransitionStep) (contracts.AuthorityDecision, error)
}

type RunRequest struct {
	Plan                   contracts.LifecyclePlan
	StepID                 string
	PreconditionDigest     string
	SnapshotDigest         string
	Authority              AuthorityValidator
	RetryFailedRecoverable bool
	Now                    time.Time
}

// RunStep performs one bounded lifecycle transition. All state changes are
// represented by journal records; the driver owns only its domain operation.
func RunStep(ctx context.Context, journal *Journal, req RunRequest, driver StepDriver) error {
	if journal == nil || driver == nil {
		return errors.New("lifecycle run requires journal and driver")
	}
	if err := req.Plan.Validate(); err != nil {
		return err
	}
	if err := req.Plan.VerifyDigest(); err != nil {
		return err
	}
	if req.Now.IsZero() {
		return errors.New("lifecycle run time is required")
	}
	if err := contracts.ValidateSHA256Digest(req.PreconditionDigest); err != nil {
		return err
	}
	step, ok := findStep(req.Plan, req.StepID)
	if !ok {
		return fmt.Errorf("unknown lifecycle step %q", req.StepID)
	}
	if step.SnapshotRequired {
		if err := contracts.ValidateSHA256Digest(req.SnapshotDigest); err != nil {
			return fmt.Errorf("snapshot: %w", err)
		}
	}
	var authority *contracts.AuthorityDecision
	if step.Authority.Required {
		if err := exactAuthorityRequirement(step.Authority); err != nil {
			return err
		}
		if req.Authority == nil {
			return errors.New("lifecycle authority validator is required")
		}
		decision, err := req.Authority.ValidateLifecycleAuthority(ctx, req.Plan, step)
		if err != nil {
			return fmt.Errorf("lifecycle authority: %w", err)
		}
		if decision.RequestID != step.Authority.RequestRef || decision.GrantedScope != step.Authority.Scope || decision.Outcome != contracts.AuthorityApprove {
			return errors.New("lifecycle authority decision does not bind the exact step")
		}
		decisionDigest, err := decision.Digest()
		if err != nil {
			return fmt.Errorf("lifecycle authority decision: %w", err)
		}
		if decision.DecisionRef != step.Authority.DecisionRef || decision.DecisionVersion != step.Authority.DecisionVersion || decisionDigest != step.Authority.DecisionDigest || decision.AuthorityRef != step.Authority.AuthorityRef || decision.AuthorityVersion != step.Authority.AuthorityVersion || decision.AuthorityGenerationDigest != step.Authority.AuthorityGenerationDigest {
			return errors.New("lifecycle authority provenance does not bind the exact step requirement")
		}
		if err := contracts.ValidateSHA256Digest(decision.AuthorityGenerationDigest); err != nil {
			return fmt.Errorf("lifecycle authority generation: %w", err)
		}
		if decision.AuthorityRef == "" || decision.AuthorityVersion == "" || decision.DecidedBy.ID == "" {
			return errors.New("lifecycle authority decision lacks exact generation and principal binding")
		}
		authority = &decision
	}
	if err := driver.Preflight(ctx, step); err != nil {
		if errors.Is(err, ErrDowngradeRefused) {
			return err
		}
		return fmt.Errorf("lifecycle preflight: %w", err)
	}
	history, err := journal.Load(ctx)
	if err != nil {
		return err
	}
	current, err := lastStepState(history, req.Plan.PlanID, req.StepID)
	if err != nil {
		return err
	}
	if current == contracts.LifecycleCommitted {
		return nil
	}
	if current == contracts.LifecycleReconcileRequired || current == contracts.LifecycleRolledBack {
		return fmt.Errorf("lifecycle step is terminal in state %s", current)
	}
	if current == contracts.LifecycleFailedRecoverable {
		if !req.RetryFailedRecoverable || !driver.Idempotent(step) {
			return ErrRetryNotSafe
		}
	}
	sequence := len(history) + 1
	appendState := func(state contracts.LifecycleTransitionState, recovery string) error {
		entry := journalEntry(req, step, sequence, state, current, recovery, authority)
		if err := journal.Append(ctx, entry); err != nil {
			return err
		}
		sequence++
		current = state
		return nil
	}
	if current == "" {
		if err := appendState(contracts.LifecyclePlanned, ""); err != nil {
			return err
		}
		if err := appendState(contracts.LifecycleApproved, ""); err != nil {
			return err
		}
		if err := appendState(contracts.LifecyclePrepared, ""); err != nil {
			return err
		}
	} else if current == contracts.LifecycleFailedRecoverable {
		if err := appendState(contracts.LifecyclePrepared, "retry"); err != nil {
			return err
		}
	}
	if current == contracts.LifecyclePrepared {
		// A prepared state is already the retry boundary.
	} else if current != contracts.LifecycleApplying {
		return errors.New("lifecycle step is not at an apply boundary")
	}
	if err := appendState(contracts.LifecycleApplying, ""); err != nil {
		return err
	}
	result, applyErr := driver.Apply(ctx, step)
	if applyErr != nil {
		if errors.Is(applyErr, ErrAmbiguousApply) {
			return appendState(contracts.LifecycleReconcileRequired, "reconcile")
		}
		return appendState(contracts.LifecycleFailedRecoverable, "retry-or-rollback")
	}
	switch result.Outcome {
	case ApplyCommitted:
		if result.ResultingManifestDigest != step.Target.Digest {
			return appendState(contracts.LifecycleReconcileRequired, "resulting-manifest-mismatch")
		}
		return appendState(contracts.LifecycleCommitted, "")
	case ApplyFailedRecoverable:
		return appendState(contracts.LifecycleFailedRecoverable, result.RecoveryAction)
	case ApplyReconcileRequired:
		return appendState(contracts.LifecycleReconcileRequired, result.RecoveryAction)
	case ApplyRolledBack:
		if !step.Reversible {
			return errors.New("irreversible lifecycle step cannot roll back")
		}
		return appendState(contracts.LifecycleRolledBack, result.RecoveryAction)
	default:
		return appendState(contracts.LifecycleReconcileRequired, "unknown-outcome")
	}
}

func exactAuthorityRequirement(requirement contracts.LifecycleAuthorityRequirement) error {
	if requirement.RequestRef == "" || requirement.RequestVersion == "" || requirement.RequestDigest == "" || requirement.DecisionRef == "" || requirement.DecisionVersion == "" || requirement.DecisionDigest == "" || requirement.AuthorityRef == "" || requirement.AuthorityVersion == "" || requirement.AuthorityGenerationDigest == "" {
		return errors.New("authority-bound lifecycle execution requires exact request, decision, and generation bindings")
	}
	return nil
}

func findStep(plan contracts.LifecyclePlan, id string) (contracts.LifecycleTransitionStep, bool) {
	for _, step := range plan.Steps {
		if step.ID == id {
			return step, true
		}
	}
	return contracts.LifecycleTransitionStep{}, false
}

func lastStepState(history []contracts.LifecycleTransitionJournal, planID, stepID string) (contracts.LifecycleTransitionState, error) {
	var state contracts.LifecycleTransitionState
	for _, entry := range history {
		if entry.PlanID == planID && entry.StepID == stepID {
			state = entry.State
		}
	}
	return state, nil
}

func journalEntry(req RunRequest, step contracts.LifecycleTransitionStep, sequence int, state, previous contracts.LifecycleTransitionState, recovery string, authority *contracts.AuthorityDecision) contracts.LifecycleTransitionJournal {
	now := req.Now.UTC()
	entry := contracts.LifecycleTransitionJournal{JournalID: fmt.Sprintf("%s:%s:%d", req.Plan.PlanID, step.ID, sequence), Version: "1", PlanID: req.Plan.PlanID, PlanDigest: req.Plan.Digest, InstallationID: req.Plan.InstallationID, Sequence: sequence, StepID: step.ID, PreviousState: previous, State: state, PreconditionDigest: req.PreconditionDigest, SnapshotDigest: req.SnapshotDigest, RecoveryAction: recovery, ReadinessDigest: req.Plan.TargetManifestDigest, RecordedAt: now}
	if authority != nil {
		entry.AuthorityRef = authority.AuthorityRef
		entry.AuthorityVersion = authority.AuthorityVersion
		entry.AuthorityDecisionRef = authority.DecisionRef
		entry.AuthorityDecisionDigest, _ = authority.Digest()
		entry.AuthorityGenerationDigest = authority.AuthorityGenerationDigest
	}
	return entry
}
