package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrDowngradeRefused   = errors.New("lifecycle downgrade refused")
	ErrAmbiguousApply     = errors.New("lifecycle apply outcome is ambiguous")
	ErrRetryNotSafe       = errors.New("lifecycle retry is not proven idempotent")
	ErrJournalConflict    = errors.New("lifecycle journal has conflicting history")
	ErrPlanDigestMismatch = errors.New("lifecycle step cannot resume under a different plan digest")
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
	return j.decodeHistory(events)
}

// TxCapableEventStore is implemented by eventstore.Store backends (the
// production SQLite-backed store) that can participate in a caller-owned
// SQL transaction. It is deliberately not part of eventstore.Store itself
// — that interface stays storage-agnostic (eventstore.MemoryStore has no
// notion of *sql.Tx) — so LoadInTx/AppendInTx type-assert for it instead.
type TxCapableEventStore interface {
	AppendInTx(ctx context.Context, tx *sql.Tx, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error)
	LoadAggregateInTx(ctx context.Context, tx *sql.Tx, aggregateID string, afterVersion int64) ([]eventstore.Event, error)
}

// LoadInTx reads journal history through a caller-supplied transaction
// rather than a fresh connection-pool query. This store's pool is capped
// at exactly one connection, so a query against the pool while a governed
// Apply transaction holds that one connection open would deadlock, not
// merely race — a StepDriver computing the next journal Sequence inside
// its own transaction (ADR-088 §13.3) MUST use LoadInTx, never Load,
// while that transaction is open.
func (j *Journal) LoadInTx(ctx context.Context, tx *sql.Tx) ([]contracts.LifecycleTransitionJournal, error) {
	if j == nil || j.store == nil {
		return nil, errors.New("lifecycle journal is required")
	}
	txStore, ok := j.store.(TxCapableEventStore)
	if !ok {
		return nil, errors.New("lifecycle journal store does not support tx-scoped reads")
	}
	events, err := txStore.LoadAggregateInTx(ctx, tx, lifecycleAggregate(j.installation), 0)
	if err != nil {
		return nil, err
	}
	return j.decodeHistory(events)
}

func (j *Journal) decodeHistory(events []eventstore.Event) ([]contracts.LifecycleTransitionJournal, error) {
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
	history, err := j.Load(ctx)
	if err != nil {
		return err
	}
	event, err := j.prepareAppend(entry, history)
	if err != nil {
		return err
	}
	_, err = j.store.Append(ctx, lifecycleAggregate(j.installation), int64(len(history)), []eventstore.Event{event})
	return err
}

// AppendInTx durably records entry inside the caller-supplied, already-open
// transaction, atomically with whatever domain mutation the caller
// performs in the same tx (ADR-088 §13.3). It enforces every invariant
// Append does — append-only history (no update/delete path exists for
// either), the expected-sequence check, transition legality via
// entry.Validate() (including validLifecycleTransition), and exact
// plan/step identity — but never opens or commits its own transaction:
// the caller (a StepDriver's Apply) owns the transaction lifecycle
// entirely, and must have performed any predicate/authority revalidation
// and the domain mutation itself, through the same *sql.Tx, before calling
// this. On success, the domain mutation and this outcome event commit or
// roll back together as one atomic unit when the caller commits or aborts
// tx — there is no window where one is durable and the other is not.
func (j *Journal) AppendInTx(ctx context.Context, tx *sql.Tx, entry contracts.LifecycleTransitionJournal) error {
	if j == nil || j.store == nil {
		return errors.New("lifecycle journal is required")
	}
	txStore, ok := j.store.(TxCapableEventStore)
	if !ok {
		return errors.New("lifecycle journal store does not support tx-scoped append")
	}
	history, err := j.LoadInTx(ctx, tx)
	if err != nil {
		return err
	}
	event, err := j.prepareAppend(entry, history)
	if err != nil {
		return err
	}
	_, err = txStore.AppendInTx(ctx, tx, lifecycleAggregate(j.installation), int64(len(history)), []eventstore.Event{event})
	return err
}

// prepareAppend performs the validation and event-shaping shared by
// Append and AppendInTx, so the two entry points cannot drift and enforce
// different invariants for the same durable record.
func (j *Journal) prepareAppend(entry contracts.LifecycleTransitionJournal, history []contracts.LifecycleTransitionJournal) (eventstore.Event, error) {
	if err := entry.Validate(); err != nil {
		return eventstore.Event{}, err
	}
	if entry.InstallationID != j.installation {
		return eventstore.Event{}, errors.New("lifecycle journal installation mismatch")
	}
	if entry.Sequence != len(history)+1 {
		return eventstore.Event{}, errors.New("lifecycle journal sequence is not append-only")
	}
	var actualPrevious contracts.LifecycleTransitionState
	for _, prior := range history {
		if prior.PlanID == entry.PlanID && prior.StepID == entry.StepID {
			actualPrevious = prior.State
		}
	}
	if entry.PreviousState != actualPrevious {
		return eventstore.Event{}, errors.New("lifecycle journal previous state does not match the exact plan step history")
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return eventstore.Event{}, err
	}
	return eventstore.Event{
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
	}, nil
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
	// OutcomeAlreadyRecorded is set by a driver whose Apply already
	// durably committed this exact outcome as a journal entry itself, via
	// Journal.AppendInTx inside its own atomic transaction (ADR-088
	// §13.3) — i.e. the domain mutation and the outcome journal entry
	// already committed together, in one transaction, before Apply
	// returned. When set, RunStep MUST NOT append a second outcome entry
	// for this attempt: doing so would either violate the append-only
	// sequence check (the driver's entry already advanced it) or, worse,
	// attempt an illegal transition out of an already-recorded terminal
	// state. A driver setting this MUST NOT also return a non-nil error
	// from Apply — an error implies its transaction rolled back and
	// nothing was recorded, which is the mutually exclusive case.
	OutcomeAlreadyRecorded bool
}

type StepDriver interface {
	// Preflight proves compatibility and refuses downgrade before mutation.
	Preflight(context.Context, contracts.LifecycleTransitionStep) error
	// Idempotent reports whether retrying the exact step/precondition is safe.
	Idempotent(contracts.LifecycleTransitionStep) bool
	Apply(context.Context, contracts.LifecycleTransitionStep) (ApplyResult, error)
}

// RunRequestBoundDriver is implemented by drivers that own their outcome
// journal append inside Apply's transaction. RunStep supplies the exact,
// already-validated invocation identity instead of requiring those drivers to
// trust a second set of caller-populated plan/evidence fields.
type RunRequestBoundDriver interface {
	BindRunRequest(context.Context, RunRequest, contracts.LifecycleTransitionStep, *contracts.AuthorityDecision) error
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
	history, err := journal.Load(ctx)
	if err != nil {
		return err
	}
	current, currentPlanDigest, err := lastStepState(history, req.Plan.PlanID, req.StepID)
	if err != nil {
		return err
	}
	sequence := len(history) + 1
	appendState := func(state contracts.LifecycleTransitionState, recovery string) error {
		entry := journalEntry(req, step, sequence, state, current, recovery, nil)
		if err := journal.Append(ctx, entry); err != nil {
			return err
		}
		sequence++
		current = state
		return nil
	}
	if current != "" && currentPlanDigest != req.Plan.Digest {
		if current == contracts.LifecycleApplying {
			return appendState(contracts.LifecycleReconcileRequired, "plan-digest-mismatch")
		}
		return ErrPlanDigestMismatch
	}
	if current != "" {
		prior, ok := lastExactStepEntry(history, req.Plan.PlanID, req.StepID)
		if !ok || prior.PreconditionDigest != req.PreconditionDigest || prior.SnapshotDigest != req.SnapshotDigest || prior.ReadinessDigest != req.Plan.TargetManifestDigest {
			return errors.New("lifecycle restart evidence identity does not match the exact prior attempt")
		}
	}
	if current == contracts.LifecycleCommitted {
		return nil
	}
	if current == contracts.LifecycleReconcileRequired || current == contracts.LifecycleRolledBack {
		return fmt.Errorf("lifecycle step is terminal in state %s", current)
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
	if bound, ok := driver.(RunRequestBoundDriver); ok {
		if err := bound.BindRunRequest(ctx, req, step, authority); err != nil {
			return fmt.Errorf("bind lifecycle run request: %w", err)
		}
	}
	if err := driver.Preflight(ctx, step); err != nil {
		if errors.Is(err, ErrDowngradeRefused) {
			return err
		}
		return fmt.Errorf("lifecycle preflight: %w", err)
	}
	appendState = func(state contracts.LifecycleTransitionState, recovery string) error {
		entry := journalEntry(req, step, sequence, state, current, recovery, authority)
		if err := journal.Append(ctx, entry); err != nil {
			return err
		}
		sequence++
		current = state
		return nil
	}
	// Restart-safe identity binding: a non-empty prior state may only be
	// resumed under the exact PlanDigest it was previously advanced
	// under. A plan edited (or substituted) under the same PlanID/StepID
	// must never silently resume against a stale/different step
	// definition. Discovered before applying was ever entered for this
	// step, the step simply never advances (no illegal state is
	// entered). Discovered after applying was already durably entered by
	// a prior attempt, the step is fenced through the one legal edge
	// applying->reconcile_required, never resumed silently.
	if current == contracts.LifecycleFailedRecoverable {
		if !req.RetryFailedRecoverable || !driver.Idempotent(step) {
			return ErrRetryNotSafe
		}
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
	// Applying is entered exactly once per attempt: a second entry into
	// Applying is illegal (validLifecycleTransition has no Applying ->
	// Applying edge) and would otherwise be indistinguishable from a
	// second real attempt for journal-reading purposes. Resuming with
	// current already at Applying therefore does not append another
	// Applying entry; it re-attempts Apply directly, subject to the
	// driver's own Idempotent contract, and transitions out of Applying
	// through one of its four legal successors based on the fresh
	// attempt's outcome via the unchanged switch below.
	resumingApplying := current == contracts.LifecycleApplying
	if current == contracts.LifecyclePrepared {
		if err := appendState(contracts.LifecycleApplying, ""); err != nil {
			return err
		}
	} else if !resumingApplying {
		return errors.New("lifecycle step is not at an apply boundary")
	}
	if resumingApplying && !driver.Idempotent(step) {
		return ErrRetryNotSafe
	}
	result, applyErr := driver.Apply(ctx, step)
	if applyErr != nil {
		if result.OutcomeAlreadyRecorded {
			return errors.New("lifecycle apply reported an already-recorded outcome alongside an error")
		}
		return applyErr
	}
	if result.OutcomeAlreadyRecorded {
		// The driver's own transaction already durably committed both the
		// domain mutation and the corresponding lifecycle outcome journal
		// entry together, in one commit (ADR-088 §13.3, via
		// Journal.AppendInTx). RunStep must not append a second outcome
		// entry for this attempt — doing so is both unnecessary (the
		// outcome is already durable) and, for a Committed outcome,
		// illegal (validLifecycleTransition has no Committed successor).
		// The ResultingManifestDigest-vs-step.Target.Digest comparison
		// that gates a Committed outcome for the non-atomic path below is
		// therefore the driver's own responsibility here, performed
		// against live observation before its own commit — RunStep
		// cannot and must not re-check it post hoc against a mutation it
		// no longer has an open transaction to inspect.
		switch result.Outcome {
		case ApplyCommitted, ApplyFailedRecoverable, ApplyReconcileRequired:
			current = lifecycleStateFor(result.Outcome)
			return nil
		case ApplyRolledBack:
			if !step.Reversible {
				return errors.New("irreversible lifecycle step cannot roll back")
			}
			current = contracts.LifecycleRolledBack
			return nil
		default:
			return fmt.Errorf("lifecycle apply reported an already-recorded but unrecognized outcome %q", result.Outcome)
		}
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

func lastExactStepEntry(history []contracts.LifecycleTransitionJournal, planID, stepID string) (contracts.LifecycleTransitionJournal, bool) {
	var found contracts.LifecycleTransitionJournal
	ok := false
	for _, entry := range history {
		if entry.PlanID == planID && entry.StepID == stepID {
			found, ok = entry, true
		}
	}
	return found, ok
}

// lifecycleStateFor maps an already-recorded ApplyCommitted/
// ApplyFailedRecoverable/ApplyReconcileRequired outcome to the
// LifecycleTransitionState it corresponds to, for updating RunStep's
// in-memory understanding of current after a driver's own transaction has
// already durably recorded the journal entry itself.
func lifecycleStateFor(outcome ApplyOutcome) contracts.LifecycleTransitionState {
	switch outcome {
	case ApplyCommitted:
		return contracts.LifecycleCommitted
	case ApplyFailedRecoverable:
		return contracts.LifecycleFailedRecoverable
	case ApplyReconcileRequired:
		return contracts.LifecycleReconcileRequired
	default:
		return ""
	}
}

// NewApplyOutcomeJournalEntry constructs the LifecycleTransitionJournal
// entry a StepDriver's Apply must append via Journal.AppendInTx when it
// owns its own atomic outcome (ApplyResult.OutcomeAlreadyRecorded, ADR-088
// §13.3). It mirrors the executor's own journalEntry field construction
// exactly, so a driver-recorded outcome and an executor-recorded outcome
// are indistinguishable in the journal's own shape. authority may be nil
// when the step does not require authority binding.
func NewApplyOutcomeJournalEntry(planID, planDigest, installationID, stepID string, sequence int, state, previous contracts.LifecycleTransitionState, preconditionDigest, snapshotDigest, readinessDigest, recovery string, now time.Time, authority *contracts.AuthorityDecision) contracts.LifecycleTransitionJournal {
	entry := contracts.LifecycleTransitionJournal{JournalID: fmt.Sprintf("%s:%s:%d", planID, stepID, sequence), Version: "1", PlanID: planID, PlanDigest: planDigest, InstallationID: installationID, Sequence: sequence, StepID: stepID, PreviousState: previous, State: state, PreconditionDigest: preconditionDigest, SnapshotDigest: snapshotDigest, RecoveryAction: recovery, ReadinessDigest: readinessDigest, RecordedAt: now.UTC()}
	if authority != nil {
		entry.AuthorityRef = authority.AuthorityRef
		entry.AuthorityVersion = authority.AuthorityVersion
		entry.AuthorityDecisionRef = authority.DecisionRef
		entry.AuthorityDecisionDigest, _ = authority.Digest()
		entry.AuthorityGenerationDigest = authority.AuthorityGenerationDigest
	}
	return entry
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

// lastStepState returns the most recent journal state recorded for
// (planID, stepID), together with the PlanDigest that entry was recorded
// under. Callers resuming a non-empty state MUST verify that digest
// against the plan they are about to run before treating the state as
// resumable (see RunStep) — a plan edited under the same PlanID/StepID
// must never silently resume against a stale step definition.
func lastStepState(history []contracts.LifecycleTransitionJournal, planID, stepID string) (contracts.LifecycleTransitionState, string, error) {
	var state contracts.LifecycleTransitionState
	var planDigest string
	for _, entry := range history {
		if entry.PlanID == planID && entry.StepID == stepID {
			state = entry.State
			planDigest = entry.PlanDigest
		}
	}
	return state, planDigest, nil
}

func nextStepJournalPosition(history []contracts.LifecycleTransitionJournal, planID, planDigest, stepID string) (contracts.LifecycleTransitionState, int, error) {
	previous, observedPlanDigest, err := lastStepState(history, planID, stepID)
	if err != nil {
		return "", 0, err
	}
	if previous != contracts.LifecycleApplying {
		return "", 0, fmt.Errorf("lifecycle outcome requires exact step in applying, got %q", previous)
	}
	if observedPlanDigest != planDigest {
		return "", 0, ErrPlanDigestMismatch
	}
	return previous, len(history) + 1, nil
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
