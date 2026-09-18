package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func migrationDigest(seed string) string { return "sha256:" + strings.Repeat(seed, 64)[:64] }
func migrationReadiness() contracts.LifecycleReadiness {
	return contracts.LifecycleReadiness{CryptoBootstrap: "ready", StateStore: "ready", SchemaCompatibility: "ready", GovernanceRoot: "ready", AuthorityTopology: "ready", PackageRuntimeClosure: "ready", LifecycleRecovery: "ready", Installation: "ready"}
}

func assertReconcileEntriesValidate(t *testing.T, history []contracts.LifecycleTransitionJournal) {
	t.Helper()
	for _, entry := range history {
		if entry.State != contracts.LifecycleReconcileRequired {
			continue
		}
		if entry.RecoveryAction == "" {
			t.Fatalf("reconcile_required entry lacks a specific RecoveryAction: %+v", entry)
		}
		if err := entry.Validate(); err != nil {
			t.Fatalf("reconcile_required entry fails LifecycleTransitionJournal.Validate: %v entry=%+v", err, entry)
		}
	}
}

type migrationDriver struct {
	result       ApplyResult
	err          error
	preflightErr error
	idempotent   bool
	calls        int
}

type migrationAuthority struct{ decision contracts.AuthorityDecision }

func (a migrationAuthority) ValidateLifecycleAuthority(context.Context, contracts.LifecyclePlan, contracts.LifecycleTransitionStep) (contracts.AuthorityDecision, error) {
	return a.decision, nil
}

func (d *migrationDriver) Preflight(context.Context, contracts.LifecycleTransitionStep) error {
	return d.preflightErr
}
func (d *migrationDriver) Idempotent(contracts.LifecycleTransitionStep) bool { return d.idempotent }
func (d *migrationDriver) Apply(context.Context, contracts.LifecycleTransitionStep) (ApplyResult, error) {
	d.calls++
	return d.result, d.err
}

func migrationPlan(t *testing.T, reversible bool) contracts.LifecyclePlan {
	t.Helper()
	current := migrationDigest("a")
	target := migrationDigest("b")
	step := contracts.LifecycleTransitionStep{ID: "migrate", Sequence: 1, Class: contracts.LifecycleSchema, Current: contracts.LifecycleComponentRef{Class: contracts.LifecycleSchema, ID: "schema", Version: "1", Digest: current}, Target: contracts.LifecycleComponentRef{Class: contracts.LifecycleSchema, ID: "schema", Version: "2", Digest: target}, Preconditions: []contracts.LifecycleEvidenceRef{{ID: "pre", Kind: "test", Source: "fixture", Digest: migrationDigest("c")}}, Effect: contracts.LifecycleAutomatic, SnapshotRequired: true, Reversible: reversible, RecoveryStrategy: "retry-or-rollback", ReadinessImpact: "schema"}
	p := contracts.LifecyclePlan{PlanID: "plan-migration", PlanVersion: "1", InstallationID: "installation", CurrentManifestDigest: current, TargetManifestDigest: target, Steps: []contracts.LifecycleTransitionStep{step}, PreservedHistory: []contracts.LifecycleEvidenceRef{{ID: "history", Kind: "evidence", Source: "fixture", Digest: migrationDigest("d")}}, SnapshotRequired: true, ExpectedReadiness: migrationReadiness()}
	d, err := p.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	p.Digest = d
	return p
}

func TestRunStepCommitsAndSurvivesJournalReload(t *testing.T) {
	plan := migrationPlan(t, true)
	journal, err := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: plan.TargetManifestDigest}}
	if err := RunStep(context.Background(), journal, RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)}, driver); err != nil {
		t.Fatal(err)
	}
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 || history[len(history)-1].State != contracts.LifecycleCommitted {
		t.Fatalf("unexpected journal: %+v", history)
	}
	if err := RunStep(context.Background(), journal, RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}, driver); err != nil {
		t.Fatal(err)
	}
	if driver.calls != 1 {
		t.Fatal("committed step was applied twice")
	}
}

func TestRunStepRefusesAmbiguousRetryAndRecordsReconciliation(t *testing.T) {
	plan := migrationPlan(t, true)
	journal, _ := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	driver := &migrationDriver{err: ErrAmbiguousApply, idempotent: false}
	if err := RunStep(context.Background(), journal, RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}, driver); !errors.Is(err, ErrAmbiguousApply) {
		t.Fatalf("expected ambiguous apply error, got %v", err)
	}
	history, _ := journal.Load(context.Background())
	if history[len(history)-1].State != contracts.LifecycleApplying {
		t.Fatalf("failed apply must leave the durable retry boundary at applying: %+v", history)
	}
	if err := RunStep(context.Background(), journal, RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), RetryFailedRecoverable: true, Now: time.Now().UTC()}, driver); err == nil {
		t.Fatal("reconcile-required step was retried")
	}
}

func TestRunStepRequiresSnapshotAndRefusesDowngrade(t *testing.T) {
	plan := migrationPlan(t, true)
	journal, _ := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: plan.TargetManifestDigest}}
	if err := RunStep(context.Background(), journal, RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), Now: time.Now().UTC()}, driver); err == nil {
		t.Fatal("missing snapshot accepted")
	}
	driver2 := &migrationDriver{preflightErr: ErrDowngradeRefused}
	if err := RunStep(context.Background(), journal, RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}, driver2); !errors.Is(err, ErrDowngradeRefused) {
		t.Fatalf("downgrade was not refused: %v", err)
	}
}

func TestRunStepRequiresAndJournalsExactAuthorityProof(t *testing.T) {
	plan := migrationPlan(t, true)
	decision := contracts.AuthorityDecision{RequestID: "request-1", RequestVersion: "1", RequestDigest: migrationDigest("a"), DecisionRef: "decision-1", DecisionVersion: "1", AuthorityRef: "generation-1", AuthorityVersion: "1", AuthorityGenerationDigest: migrationDigest("b"), DecidedBy: contracts.PrincipalRef{ID: "operator", Kind: "human"}, GrantedScope: "installation:schema", Outcome: contracts.AuthorityApprove, AuthorityDigest: migrationDigest("c"), IssuedAt: time.Now().UTC()}
	decisionDigest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Steps[0].Authority = contracts.LifecycleAuthorityRequirement{Required: true, Operation: "schema.migrate", Scope: "installation:schema", RequestRef: "request-1", RequestVersion: "1", RequestDigest: migrationDigest("a"), DecisionRef: "decision-1", DecisionVersion: "1", DecisionDigest: decisionDigest, AuthorityRef: "generation-1", AuthorityVersion: "1", AuthorityGenerationDigest: migrationDigest("b")}
	digest, err := plan.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Digest = digest
	j, err := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: plan.TargetManifestDigest}}
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Authority: migrationAuthority{decision: decision}, Now: time.Now().UTC()}
	if err := RunStep(context.Background(), j, req, driver); err != nil {
		t.Fatal(err)
	}
	history, err := j.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if history[0].AuthorityRef != decision.AuthorityRef || history[0].AuthorityVersion != decision.AuthorityVersion || history[0].AuthorityDecisionRef != decision.DecisionRef || history[0].AuthorityGenerationDigest != decision.AuthorityGenerationDigest {
		t.Fatalf("authority provenance was not journaled exactly: %+v", history[0])
	}
}

func TestRunStepRetriesRecoverableIdempotentFailure(t *testing.T) {
	plan := migrationPlan(t, true)
	j, err := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyFailedRecoverable, RecoveryAction: "retry"}, idempotent: true}
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	if err := RunStep(context.Background(), j, req, driver); err != nil {
		t.Fatal(err)
	}
	driver.result = ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: plan.TargetManifestDigest}
	req.RetryFailedRecoverable = true
	if err := RunStep(context.Background(), j, req, driver); err != nil {
		t.Fatal(err)
	}
	history, err := j.Load(context.Background())
	if err != nil || history[len(history)-1].State != contracts.LifecycleCommitted {
		t.Fatalf("recoverable retry did not commit: %v %+v", err, history)
	}
}

func TestRunStepFencesUnsafeRollbackAndUnknownOutcome(t *testing.T) {
	for _, test := range []struct {
		name       string
		reversible bool
		result     ApplyResult
		want       contracts.LifecycleTransitionState
	}{
		{name: "rollback", reversible: true, result: ApplyResult{Outcome: ApplyRolledBack, RecoveryAction: "restore-previous"}, want: contracts.LifecycleRolledBack},
		{name: "unknown", reversible: true, result: ApplyResult{Outcome: ApplyOutcome("unexpected"), RecoveryAction: "fence"}, want: contracts.LifecycleReconcileRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := migrationPlan(t, test.reversible)
			j, err := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
			if err != nil {
				t.Fatal(err)
			}
			driver := &migrationDriver{result: test.result}
			req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
			if err := RunStep(context.Background(), j, req, driver); err != nil {
				t.Fatal(err)
			}
			history, err := j.Load(context.Background())
			assertReconcileEntriesValidate(t, history)
			if err != nil || history[len(history)-1].State != test.want {
				t.Fatalf("unexpected recovery state: %v %+v", err, history)
			}
		})
	}
	plan := migrationPlan(t, false)
	j, _ := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyRolledBack, RecoveryAction: "unsafe"}}
	if err := RunStep(context.Background(), j, RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}, driver); err == nil {
		t.Fatal("irreversible rollback was accepted")
	}
}
