package goaldrive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Regressions added by the consolidated guard/mutation inventory (repair 3).
// Each observes one guard that earlier tests only saw masked by a later layer.

func markerExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "ran.marker"))
	return err == nil
}

const touchMarker = `touch ran.marker`

// Dispatch boundary: a plan whose bound validation profile is not the repository's
// declared validator is refused BEFORE a worker runs (later layers would refuse
// the checkpoint only after the provider had already acted).
func TestKernelRepair3ProfileMismatchIsRefusedBeforeTheWorkerRuns(t *testing.T) {
	f := newBoundGitFixture(t)
	f.baseline.WorkPlan.Safety.ValidationProfileDigest = "sha256:" + strings.Repeat("1", 64)
	_, err := f.turn("inv-p", touchMarker, passingPlanAuthority{})
	if err == nil || !strings.Contains(err.Error(), "validation profile digest mismatch") || markerExists(f.work) {
		t.Fatalf("a mismatched validation profile reached the worker: err=%v ran=%v", err, markerExists(f.work))
	}
}

// Dispatch boundary: revoked governing authority stops execution BEFORE the
// worker runs; the publication boundary alone would refuse only after it had.
func TestKernelRepair3RevokedAuthorityIsRefusedBeforeTheWorkerRuns(t *testing.T) {
	f := newBoundGitFixture(t)
	_, err := f.turn("inv-r", touchMarker, &revocableAuthority{revoked: true})
	if !errors.Is(err, ErrPlanAuthority) || markerExists(f.work) {
		t.Fatalf("revoked authority did not stop dispatch: err=%v ran=%v", err, markerExists(f.work))
	}
}

// Dispatch boundary (activation): the same for an activation that fails.
func TestKernelRepair3FailedActivationIsRefusedBeforeTheWorkerRuns(t *testing.T) {
	f := newBoundGitFixture(t)
	_, err := f.turnWith("inv-a", touchMarker, passingPlanAuthority{}, driftingActivation{gate: &nthCallGate{failAt: 1}}, f.repository)
	if !errors.Is(err, ErrSafetyActivation) || markerExists(f.work) {
		t.Fatalf("a failed activation did not stop dispatch: err=%v ran=%v", err, markerExists(f.work))
	}
}

// Qualification requires the integrated-pass evidence bound to the checkpoint.
func TestKernelRepair3QualificationRequiresIntegratedPassEvidence(t *testing.T) {
	controller := newRepair3Controller(passingPlanAuthority{}, &countingWorker{})
	unit := contracts.WorkCandidate{ID: "unit", QualificationPredicates: []string{"candidate/unit/conformance"}}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{unit}}}
	repo := &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}, output: "PRAXIS-VALIDATION candidate/unit/conformance\n"}
	record := TurnRecord{ChildObjective: "unit", EndHead: "head", Progress: true}
	_, err := controller.qualifySafetyUnit(context.Background(), TurnRequest{GoalBaseline: baseline}, repo, record, unit)
	if err == nil || !strings.Contains(err.Error(), "integrated-pass evidence") {
		t.Fatalf("a unit qualified without integrated-pass evidence: %v", err)
	}
	record.CheckpointEvidence = []string{"repository:declared-validation-passed"}
	if _, err := controller.qualifySafetyUnit(context.Background(), TurnRequest{GoalBaseline: baseline}, repo, record, unit); err != nil {
		t.Fatalf("control: a unit with the evidence was refused: %v", err)
	}
}

// A governed output must be present and within the byte bound.
func TestKernelRepair3GovernedOutputMustBePresentAndBounded(t *testing.T) {
	controller := newRepair3Controller(passingPlanAuthority{}, &countingWorker{})
	spec := []byte(`{"governed_outputs":[{"role":"gate-a","evidence_class":"decision-dossier","source_ref":"docs/gate-a.json","schema_id":"gate-a/1"}]}`)
	unit := contracts.WorkCandidate{ID: "dos", SourceDigest: "sha256:" + strings.Repeat("4", 64), QualificationPredicates: []string{"candidate/dos/conformance"}, Specification: spec}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{unit}}}
	record := TurnRecord{ChildObjective: "dos", EndHead: "head", Progress: true, CheckpointEvidence: []string{"repository:declared-validation-passed"}}
	for name, body := range map[string][]byte{
		"empty":     []byte{},
		"oversized": make([]byte, contracts.MaxGovernedArtifactBytes+1),
	} {
		repo := &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}, output: "PRAXIS-VALIDATION candidate/dos/conformance\n", artifacts: map[string][]byte{"docs/gate-a.json": body}}
		if _, err := controller.qualifySafetyUnit(context.Background(), TurnRequest{GoalBaseline: baseline}, repo, record, unit); err == nil || !strings.Contains(err.Error(), "missing or exceeds the byte bound") {
			t.Fatalf("%s governed output was accepted: %v", name, err)
		}
	}
}

// An explicit, pinned or recovered objective is classified against the accepted
// plan before any worker: an objective the plan does not contain, and an
// objective the ledger already completed, never reach a provider.
func TestKernelRepair3ExplicitObjectiveMustBeAnOpenCandidateOfTheAcceptedPlan(t *testing.T) {
	unit := contracts.WorkCandidate{ID: "unit", Kind: contracts.WorkCandidateOrdinary, Priority: 1, Sequence: 1, SourceRef: "unit.json", SourceDigest: "sha256:" + strings.Repeat("5", 64), Provenance: contracts.ProvenancePLAN, QualificationPredicates: []string{"candidate/unit/conformance"}}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64), WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{unit}}}
	run := func(controller Controller, objective string) error {
		_, err := controller.ExecuteTurn(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv-o", TurnID: "turn-o", ChildObjective: objective, GraphID: "g", GraphVersion: "1", StartHead: "head", Mode: ModeSupervised, GoalBaseline: baseline, Repository: contracts.RepositorySynced})
		return err
	}
	worker := &countingWorker{}
	controller := newRepair3Controller(passingPlanAuthority{}, worker)
	if err := run(controller, "not-in-the-plan"); err == nil || !strings.Contains(err.Error(), "not a candidate of the accepted safety-bearing plan") || worker.calls != 0 {
		t.Fatalf("an objective outside the accepted plan reached a worker: err=%v calls=%d", err, worker.calls)
	}
	qualified := UnitCompletion{GoalID: "goal", GoalVersion: "1", UnitID: "unit", InvocationID: "inv", TurnID: "t1", EndHead: "head", CompletedAt: time.Now().UTC(), MechanismTestsPassed: true, ConformanceQualified: true, SpecificationDigest: unit.SourceDigest, ValidationProfileDigest: baseline.WorkPlan.Safety.ValidationProfileDigest, Evidence: []string{"checkpoint:head"}}
	if err := controller.recordCompletion(context.Background(), baseline, qualified); err != nil {
		t.Fatal(err)
	}
	if err := run(controller, "unit"); err == nil || !strings.Contains(err.Error(), "already complete") || worker.calls != 0 {
		t.Fatalf("an already completed objective reached a worker: err=%v calls=%d", err, worker.calls)
	}
}

// The effect boundary carries its own verifier-configured check: it does not rely
// on the dispatch boundary having run first.
func TestKernelRepair3EffectBoundaryFailsClosedWithoutAnActivationVerifier(t *testing.T) {
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, GoverningAuthority: passingPlanAuthority{}}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding()}}
	if err := controller.authorizeEffect(context.Background(), TurnRequest{GoalBaseline: baseline}, "test-effect", nil, TurnRecord{}); !errors.Is(err, ErrSafetyActivation) {
		t.Fatalf("an effect was authorized with no activation verifier configured: %v", err)
	}
}

// Recording a gate completion is a governed effect of its own: authority that
// lapses between the coordinator's answer and the record refuses the record.
func TestKernelRepair3GateCompletionIsAuthorizedAtTheMomentItIsRecorded(t *testing.T) {
	candidate := contracts.WorkCandidate{ID: "gate-a", Kind: contracts.WorkCandidateAuthorityGate, Priority: 1, Sequence: 1, SourceRef: "gate.json", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenanceAuthorityGate}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{candidate}}}
	request := contracts.AuthorityRequest{ID: "gate-request", Version: "1", BaselineID: "goal", BaselineVersion: "1", BaselineDigest: "sha256:" + strings.Repeat("b", 64), RequestedAuthority: "goal.gate.decide", RequestedScope: contracts.InstallationGovernanceScopePrefix + "sha256:" + strings.Repeat("a", 64), SubjectScope: "goal:goal/1", Reason: "question", Alternatives: []string{"a", "b"}, Status: contracts.AuthorityRequestPending, CeremonyProfile: "interactive-os-owner-v1", ActivationManifestDigest: protectedBinding().ActivationManifestDigest, GateCandidateID: "gate-a", GateSpecificationDigest: candidate.SourceDigest, DossierRef: "dossier", DossierDigest: "sha256:" + strings.Repeat("e", 64), DossierProducerCandidate: "dos", DossierRole: "gate-a", DossierEvidenceClass: "decision_dossier", DossierSchemaID: "gate-a/1", DossierCheckpoint: "head"}
	// Governing authority holds at dispatch (call 1) and lapses at the record (call 2).
	authority := &driftingAuthority{gate: &nthCallGate{failAt: 2}}
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: &countingWorker{}, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: authority, AuthorityGates: approvedGateCoordinator{request: request}}
	_, err := controller.ExecuteTurn(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", GraphID: "g", GraphVersion: "1", StartHead: "head", Mode: ModeSupervised, GoalBaseline: baseline, Repository: contracts.RepositorySynced})
	if !errors.Is(err, ErrPlanAuthority) {
		t.Fatalf("a gate completion was recorded although authority lapsed before it: %v", err)
	}
	if completions, _ := controller.Ledger.LoadCompletions(context.Background(), "goal", "1"); len(completions) != 0 {
		t.Fatalf("a gate completion was recorded: %+v", completions)
	}
}

// The Goal completion candidate is derived from the AUTHENTICATED, effective
// completions. When the only thing keeping the plan structurally complete is a
// completion whose gate authority is gone, no candidate is derived.
func TestKernelRepair3GoalCandidateIsDerivedOnlyFromEffectiveCompletions(t *testing.T) {
	baseline, gate, unit := gatePlanFixture()
	authority := &gateVerifier{}
	controller := newRepair3Controller(authority, &countingWorker{})
	qualified := UnitCompletion{GoalID: "goal", GoalVersion: "1", UnitID: unit.ID, InvocationID: "inv", TurnID: "t2", EndHead: "head", CompletedAt: time.Now().UTC(), MechanismTestsPassed: true, ConformanceQualified: true, SpecificationDigest: unit.SourceDigest, ValidationProfileDigest: baseline.WorkPlan.Safety.ValidationProfileDigest, Evidence: []string{"checkpoint:head"}}
	for _, completion := range []UnitCompletion{gateCompletion(gate), qualified} {
		if err := controller.recordCompletion(context.Background(), baseline, completion); err != nil {
			t.Fatal(err)
		}
	}
	req := TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv-c", TurnID: "t3", GoalBaseline: baseline}
	authority.notEffective = true
	if _, err := controller.deriveGoalCandidate(context.Background(), req, &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}}, TurnRecord{EndHead: "head"}); err != nil {
		t.Fatal(err)
	}
	state, err := controller.Ledger.LoadGoalCompletion(context.Background(), "goal", "1")
	if err != nil || state.Candidate != nil {
		t.Fatalf("a Goal completion candidate was derived from completions whose authority is gone: %+v %v", state.Candidate, err)
	}
	// Control: while the authority stands, the candidate is derived.
	authority.notEffective = false
	if _, err := controller.deriveGoalCandidate(context.Background(), req, &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}}, TurnRecord{EndHead: "head"}); err != nil {
		t.Fatal(err)
	}
	if state, err = controller.Ledger.LoadGoalCompletion(context.Background(), "goal", "1"); err != nil || state.Candidate == nil {
		t.Fatalf("control: no candidate was derived from a fully effective plan: %+v %v", state.Candidate, err)
	}
}
