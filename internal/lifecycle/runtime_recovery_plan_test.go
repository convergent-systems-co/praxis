package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func runtimeStateAuthorityDecision(t *testing.T) contracts.AuthorityDecision {
	t.Helper()
	return contracts.AuthorityDecision{
		RequestID: "runtime-state-request", RequestVersion: "1", RequestDigest: migrationDigest("4"),
		DecisionRef: "runtime-state-decision", DecisionVersion: "1", AuthorityRef: "root-owner-generation", AuthorityVersion: "1",
		AuthorityGenerationDigest: migrationDigest("2"), DecidedBy: contracts.PrincipalRef{ID: "installation-owner", Kind: "human"},
		GrantedScope: "installation-repair:installation", Outcome: contracts.AuthorityApprove, AuthorityDigest: migrationDigest("3"),
		IssuedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
	}
}

func runtimeStateStep(t *testing.T) contracts.LifecycleTransitionStep {
	t.Helper()
	decision := runtimeStateAuthorityDecision(t)
	decisionDigest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.LifecycleTransitionStep{
		ID: "runtime-state", Sequence: 2, Class: contracts.LifecycleRuntime,
		Current:       contracts.LifecycleComponentRef{Class: contracts.LifecycleRuntime, ID: "invocation-runtime-state", Version: "1", Digest: migrationDigest("6")},
		Target:        contracts.LifecycleComponentRef{Class: contracts.LifecycleRuntime, ID: "invocation-runtime-state", Version: "2", Digest: migrationDigest("7")},
		Preconditions: []contracts.LifecycleEvidenceRef{{ID: "runtime-pre", Kind: "test", Source: "fixture", Digest: migrationDigest("8")}},
		Authority: contracts.LifecycleAuthorityRequirement{
			Required: true, Operation: contracts.GovernedInstallationRepairRuntimeState, Scope: decision.GrantedScope,
			RequestRef: decision.RequestID, RequestVersion: decision.RequestVersion, RequestDigest: decision.RequestDigest,
			DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest,
			AuthorityRef: decision.AuthorityRef, AuthorityVersion: decision.AuthorityVersion, AuthorityGenerationDigest: decision.AuthorityGenerationDigest,
		},
		Effect: contracts.LifecycleAuthorityBound, SnapshotRequired: true, Reversible: true,
		RecoveryStrategy: "reconcile-runtime-bindings", ReadinessImpact: "runtime",
	}
}

func runtimeRecoveryPlan(t *testing.T, storageTarget string) contracts.LifecyclePlan {
	t.Helper()
	storageStep := storageSchemaStep(t, storageTarget)
	runtimeStep := runtimeStateStep(t)
	plan, err := NewRuntimeRecoveryPlan(RuntimeRecoveryPlanSpec{
		PlanID: "runtime-recovery-plan", PlanVersion: "1", InstallationID: "installation",
		CurrentManifestDigest: storageStep.Current.Digest, TargetManifestDigest: runtimeStep.Target.Digest,
		StorageSchemaStep: storageStep, RuntimeStateStep: runtimeStep,
		PreservedHistory:  []contracts.LifecycleEvidenceRef{{ID: "history", Kind: "test", Source: "fixture", Digest: migrationDigest("9")}},
		ExpectedReadiness: migrationReadiness(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

type runtimeStateSequencingProbe struct {
	prerequisite RuntimeStatePrerequisite
	calls        int
}

func (d *runtimeStateSequencingProbe) BindRunRequest(ctx context.Context, req RunRequest, step contracts.LifecycleTransitionStep, authority *contracts.AuthorityDecision) error {
	return d.prerequisite.BindRunRequest(ctx, req, step, authority)
}
func (d *runtimeStateSequencingProbe) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	return d.prerequisite.Preflight(ctx, step)
}
func (d *runtimeStateSequencingProbe) Idempotent(contracts.LifecycleTransitionStep) bool { return true }
func (d *runtimeStateSequencingProbe) Apply(_ context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	d.calls++
	return ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: step.Target.Digest}, nil
}

func runtimeStateRequest(t *testing.T, plan contracts.LifecyclePlan) RunRequest {
	t.Helper()
	return RunRequest{
		Plan: plan, StepID: "runtime-state", PreconditionDigest: migrationDigest("a"), SnapshotDigest: migrationDigest("b"),
		Authority: migrationAuthority{decision: runtimeStateAuthorityDecision(t)}, Now: time.Now().UTC(),
	}
}

func TestRuntimeStateRefusesUncommittedStorageSchemaBeforeApplying(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	plan := runtimeRecoveryPlan(t, canonicalDigest(t))
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &runtimeStateSequencingProbe{prerequisite: RuntimeStatePrerequisite{Journal: journal}}
	if err := RunStep(ctx, journal, runtimeStateRequest(t, plan), driver); err == nil {
		t.Fatal("runtime_state reached Apply without committed storage_schema")
	}
	if driver.calls != 0 {
		t.Fatalf("runtime_state Apply called %d times before prerequisite committed", driver.calls)
	}
	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("runtime_state prerequisite refusal must occur before journaling: %+v", history)
	}
}

func TestRuntimeStateRefusesStorageSchemaFromStalePlanDigest(t *testing.T) {
	ctx := context.Background()
	db := newHistoricalFixtureDB(t, nil)
	plan := runtimeRecoveryPlan(t, canonicalDigest(t))
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	storageStep := plan.Steps[0]
	storageDriver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, storageStep, storageDriver), storageDriver); err != nil {
		t.Fatal(err)
	}

	stale := plan
	stale.Steps = append([]contracts.LifecycleTransitionStep(nil), plan.Steps...)
	stale.Steps[1].Target.Version = "substituted"
	staleDigest, err := stale.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	stale.Digest = staleDigest
	driver := &runtimeStateSequencingProbe{prerequisite: RuntimeStatePrerequisite{Journal: journal}}
	if err := RunStep(ctx, journal, runtimeStateRequest(t, stale), driver); !errors.Is(err, ErrPlanDigestMismatch) {
		t.Fatalf("runtime_state accepted storage_schema committed under stale plan digest: %v", err)
	}
	if driver.calls != 0 {
		t.Fatal("runtime_state Apply ran under a substituted plan")
	}
	state, _, err := lastStepState(mustLoadHistory(t, journal), stale.PlanID, stale.Steps[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if state != "" {
		t.Fatalf("stale-plan refusal entered runtime_state lifecycle: %s", state)
	}
}

func TestRuntimeRecoveryTwoStepSequenceRebuildAndCanonicalNoOp(t *testing.T) {
	for _, test := range []struct {
		name       string
		historical bool
	}{
		{name: "historical rebuild", historical: true},
		{name: "canonical no-op", historical: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			var db *sql.DB
			if test.historical {
				db = newHistoricalFixtureDB(t, []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true}})
			} else {
				var err error
				db, err = state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
			}
			plan := runtimeRecoveryPlan(t, canonicalDigest(t))
			if !test.historical {
				plan.Steps[0].Current.Digest = canonicalDigest(t)
				plan.CurrentManifestDigest = canonicalDigest(t)
				updatedDigest, computeErr := plan.ComputeDigest()
				if computeErr != nil {
					t.Fatal(computeErr)
				}
				plan.Digest = updatedDigest
			}
			if len(plan.Steps) != 2 || plan.Steps[0].Class != contracts.LifecycleSchema || plan.Steps[1].Class != contracts.LifecycleRuntime {
				t.Fatalf("runtime recovery was not constructed as one ordered two-step plan: %+v", plan)
			}
			journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
			if err != nil {
				t.Fatal(err)
			}
			runtimeDriver := &runtimeStateSequencingProbe{prerequisite: RuntimeStatePrerequisite{Journal: journal}}
			if err := RunStep(ctx, journal, runtimeStateRequest(t, plan), runtimeDriver); err == nil {
				t.Fatal("runtime_state was reachable before storage_schema committed")
			}
			storageDriver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
			if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, plan.Steps[0], storageDriver), storageDriver); err != nil {
				t.Fatal(err)
			}
			if err := RunStep(ctx, journal, runtimeStateRequest(t, plan), runtimeDriver); err != nil {
				t.Fatalf("runtime_state did not become reachable after storage_schema committed: %v", err)
			}
			history := mustLoadHistory(t, journal)
			storageState, storageDigest, _ := lastStepState(history, plan.PlanID, plan.Steps[0].ID)
			runtimeState, runtimeDigest, _ := lastStepState(history, plan.PlanID, plan.Steps[1].ID)
			if storageState != contracts.LifecycleCommitted || runtimeState != contracts.LifecycleCommitted || storageDigest != plan.Digest || runtimeDigest != plan.Digest {
				t.Fatalf("two-step sequence did not commit under one plan digest: storage=%s/%s runtime=%s/%s plan=%s", storageState, storageDigest, runtimeState, runtimeDigest, plan.Digest)
			}
		})
	}
}

func mustLoadHistory(t *testing.T, journal *Journal) []contracts.LifecycleTransitionJournal {
	t.Helper()
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return history
}
