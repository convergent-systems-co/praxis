package goaldrive

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type passingSafetyActivation struct{}

func (passingSafetyActivation) Verify(context.Context, contracts.WorkPlanSafetyBinding) error {
	return nil
}

// stubSeals stands in for the GoalStore's storage-key-authenticated seal
// records. Tests that need to prove behavior on forged or foreign completions
// use sealingAuthority, which owns its own store.
var (
	stubSealMu sync.Mutex
	stubSeals  = map[string][]byte{}
)

type passingPlanAuthority struct{}

func (passingPlanAuthority) VerifyGoverningAuthority(context.Context, goals.GoalBaseline, time.Time) error {
	return nil
}

func (passingPlanAuthority) GoalSafetyClassified(context.Context, string) (bool, error) {
	return false, nil
}

func (passingPlanAuthority) SealCompletion(_ context.Context, baseline goals.GoalBaseline, payload []byte, _ time.Time) (string, error) {
	digest := payloadDigest(payload)
	stubSealMu.Lock()
	defer stubSealMu.Unlock()
	stubSeals[baseline.ID+"/"+baseline.Version+"/"+digest] = append([]byte(nil), payload...)
	return digest, nil
}

func (passingPlanAuthority) LoadSealedCompletion(_ context.Context, baseline goals.GoalBaseline, digest string, _ time.Time) ([]byte, error) {
	stubSealMu.Lock()
	defer stubSealMu.Unlock()
	if payload, ok := stubSeals[baseline.ID+"/"+baseline.Version+"/"+digest]; ok {
		return payload, nil
	}
	return nil, contracts.ErrCompletionUnauthenticated
}

func (passingPlanAuthority) VerifyGateCompletion(context.Context, goals.GoalBaseline, contracts.WorkCandidate, []contracts.GovernedArtifactEvidence, string, string, time.Time) error {
	return nil
}

type approvedGateCoordinator struct{ request contracts.AuthorityRequest }

func (g approvedGateCoordinator) ReconcileAuthorityGate(context.Context, goals.GoalBaseline, contracts.WorkCandidate, []contracts.GovernedArtifactEvidence, time.Time) (contracts.AuthorityGateResult, error) {
	return contracts.AuthorityGateResult{Request: g.request, Approved: true, DecisionDigest: "sha256:" + strings.Repeat("d", 64)}, nil
}

type countingWorker struct{ calls int }

func (w *countingWorker) Execute(context.Context, WorkerRequest) (WorkerResult, error) {
	w.calls++
	return WorkerResult{}, nil
}

type conformanceRepository struct {
	supervisionRepository
	output      string
	err         error
	artifacts   map[string][]byte
	noValidator bool
}

func (r *conformanceRepository) DeclaredValidation() (string, bool) {
	return "./.praxis/validate", !r.noValidator
}
func (r *conformanceRepository) ValidationProfileDigest() (string, error) {
	return protectedBinding().ValidationProfileDigest, nil
}
func (r *conformanceRepository) RunDeclaredValidation(context.Context) (string, error) {
	return r.output, r.err
}

// RunBoundValidation stands in for the checkpoint-and-profile-bound run.
func (r *conformanceRepository) RunBoundValidation(_ context.Context, checkpoint, profile string, _ ...string) (string, error) {
	if r.noValidator {
		return "", errors.New("validator missing")
	}
	if checkpoint != r.head || profile != protectedBinding().ValidationProfileDigest {
		return "", errors.New("validation not bound to the exact checkpoint and profile")
	}
	return r.output, r.err
}

func (r *conformanceRepository) VerifyValidationBinding(_ context.Context, checkpoint, profile string) error {
	if r.noValidator || checkpoint != r.head || profile != protectedBinding().ValidationProfileDigest {
		return errors.New("validation binding drifted")
	}
	return nil
}

func (r *conformanceRepository) ReadCheckpointArtifact(_ context.Context, checkpoint, sourceRef string) ([]byte, error) {
	if checkpoint != r.head {
		return nil, errors.New("wrong checkpoint")
	}
	body, ok := r.artifacts[sourceRef]
	if !ok {
		return nil, errors.New("missing artifact")
	}
	return append([]byte(nil), body...), nil
}

func protectedBinding() *contracts.WorkPlanSafetyBinding {
	return &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion, ActivationManifestDigest: "sha256:" + strings.Repeat("1", 64), ValidationProfileDigest: "sha256:" + strings.Repeat("2", 64), SpecificationBundleDigest: "sha256:" + strings.Repeat("3", 64)}
}

func TestAuthorityGateNeverResolvesWorker(t *testing.T) {
	worker := &countingWorker{}
	candidate := contracts.WorkCandidate{ID: "gate-a", Kind: contracts.WorkCandidateAuthorityGate, Priority: 1, Sequence: 1, SourceRef: "gate.json", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenanceAuthorityGate}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64), WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{candidate}}}
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}}
	_, _, err := controller.prepare(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", Mode: ModeSupervised, GoalBaseline: baseline, WorkCandidates: []contracts.WorkCandidate{candidate}})
	if !errors.Is(err, ErrHumanAuthorityGate) || worker.calls != 0 {
		t.Fatalf("gate reached worker: err=%v calls=%d", err, worker.calls)
	}
}

func TestApprovedAuthorityGateRecordsControllerCompletionAndStillStopsTurn(t *testing.T) {
	worker := &countingWorker{}
	store := eventstore.NewMemoryStore()
	candidate := contracts.WorkCandidate{ID: "gate-a", Kind: contracts.WorkCandidateAuthorityGate, Priority: 1, Sequence: 1, SourceRef: "gate.json", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenanceAuthorityGate}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{candidate}}}
	request := contracts.AuthorityRequest{ID: "gate-request", Version: "1", BaselineID: "goal", BaselineVersion: "1", BaselineDigest: "sha256:" + strings.Repeat("b", 64), RequestedAuthority: "goal.gate.decide", RequestedScope: contracts.InstallationGovernanceScopePrefix + "sha256:" + strings.Repeat("a", 64), SubjectScope: "goal:goal/1", Reason: "question", Alternatives: []string{"a", "b"}, Status: contracts.AuthorityRequestPending, CeremonyProfile: "interactive-os-owner-v1", ActivationManifestDigest: protectedBinding().ActivationManifestDigest, GateCandidateID: "gate-a", GateSpecificationDigest: candidate.SourceDigest, DossierRef: "dossier", DossierDigest: "sha256:" + strings.Repeat("e", 64), DossierProducerCandidate: "dos", DossierRole: "gate-a", DossierEvidenceClass: "decision_dossier", DossierSchemaID: "gate-a/1", DossierCheckpoint: "head"}
	controller := Controller{Ledger: Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}, AuthorityGates: approvedGateCoordinator{request: request}}
	_, _, err := controller.prepare(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", Mode: ModeSupervised, GoalBaseline: baseline, WorkCandidates: []contracts.WorkCandidate{candidate}})
	if !errors.Is(err, ErrHumanAuthorityGate) || worker.calls != 0 {
		t.Fatalf("approved gate crossed into worker: %v calls=%d", err, worker.calls)
	}
	completions, loadErr := controller.Ledger.LoadCompletions(context.Background(), "goal", "1")
	if loadErr != nil || len(completions) != 1 || !completions[0].AuthorityGate {
		t.Fatalf("gate completion not durable: %+v %v", completions, loadErr)
	}
}

func TestSafetyPlanWithoutActivationVerifierFailsBeforeWorker(t *testing.T) {
	worker := &countingWorker{}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding()}}
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker}
	_, _, err := controller.prepare(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", Mode: ModeSupervised, GoalBaseline: baseline, ChildObjective: "unit"})
	if !errors.Is(err, ErrSafetyActivation) || worker.calls != 0 {
		t.Fatalf("inactive kernel reached worker: err=%v calls=%d", err, worker.calls)
	}
}

func TestSafetyPlanWithoutBoundValidatorFailsBeforeWorker(t *testing.T) {
	worker := &countingWorker{}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding()}}
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, SafetyActivation: passingSafetyActivation{}}
	_, err := controller.ExecuteTurnWithRepository(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", Mode: ModeSupervised, GoalBaseline: baseline, ChildObjective: "unit"}, &supervisionRepository{head: "a"})
	if err == nil || !strings.Contains(err.Error(), "digest-bound validator") || worker.calls != 0 {
		t.Fatalf("missing validator reached worker: err=%v calls=%d", err, worker.calls)
	}
}

func TestWorkerContextDoesNotDescribeWeakerRelationshipAsPrerequisite(t *testing.T) {
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:x"}
	candidates := []contracts.WorkCandidate{{ID: "unit", SourceRef: "u", SourceDigest: "d", Provenance: contracts.ProvenancePLAN}, {ID: "context", SourceRef: "c", SourceDigest: "d", Provenance: contracts.ProvenancePLAN}}
	relationships := []contracts.WorkRelationship{{Dependent: "unit", Prerequisite: "context", Kind: contracts.RelationshipAdvisory, SourceRef: "r", SourceDigest: "d", Provenance: contracts.ProvenancePLAN}}
	ctx, err := BuildWorkerContext(baseline, candidates, relationships, "unit", WorkerRepositoryContext{}, nil, false, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Unit.Prerequisites) != 0 || len(ctx.Unit.Relationships) != 1 || ctx.Unit.Relationships[0].Blocking {
		t.Fatalf("weaker relationship misrepresented: %+v", ctx.Unit)
	}
}

func TestSafetyCompletionRejectsMalformedConformanceEvidence(t *testing.T) {
	store := eventstore.NewMemoryStore()
	controller := Controller{Ledger: Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}}
	candidate := contracts.WorkCandidate{ID: "unit", QualificationPredicates: []string{"candidate/unit/conformance"}}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{candidate}}}
	record := TurnRecord{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "unit", EndHead: "head", Progress: true, CheckpointPublished: true, CompletionClaim: "unit", CheckpointEvidence: []string{"repository:declared-validation-passed"}}
	repo := &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}, output: "PASS without structured acknowledgement"}
	_, err := controller.settleCompletion(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "unit", GoalBaseline: baseline, WorkCandidates: []contracts.WorkCandidate{candidate}}, repo, record, nil)
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed conformance was accepted: %v", err)
	}
	completions, loadErr := controller.Ledger.LoadCompletions(context.Background(), "goal", "1")
	if loadErr != nil || len(completions) != 0 {
		t.Fatalf("completion persisted after failed qualification: %+v %v", completions, loadErr)
	}
}

func TestQualifiedCompletionCapturesGovernedArtifactFromExactCheckpoint(t *testing.T) {
	store := eventstore.NewMemoryStore()
	controller := Controller{Ledger: Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}}
	spec := []byte(`{"governed_outputs":[{"role":"gate-a","evidence_class":"decision-dossier","source_ref":"docs/gate-a.json","schema_id":"gate-a/1"}]}`)
	candidate := contracts.WorkCandidate{ID: "dos", SourceDigest: "sha256:" + strings.Repeat("4", 64), QualificationPredicates: []string{"candidate/dos/conformance"}, Specification: spec}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64), WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{candidate}}}
	record := TurnRecord{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "dos", EndHead: "head", Progress: true, CheckpointPublished: true, CompletionClaim: "dos", CheckpointEvidence: []string{"repository:declared-validation-passed"}}
	repo := &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}, output: "PRAXIS-VALIDATION candidate/dos/conformance\n", artifacts: map[string][]byte{"docs/gate-a.json": []byte(`{"status":"undecided"}`)}}
	_, err := controller.settleCompletion(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "dos", GoalBaseline: baseline, WorkCandidates: []contracts.WorkCandidate{candidate}}, repo, record, nil)
	if err != nil {
		t.Fatal(err)
	}
	completions, err := controller.Ledger.LoadCompletions(context.Background(), "goal", "1")
	if err != nil || len(completions) != 1 || len(completions[0].GovernedArtifacts) != 1 || string(completions[0].GovernedArtifacts[0].Bytes) != `{"status":"undecided"}` {
		t.Fatalf("governed artifact not preserved: %+v %v", completions, err)
	}
}

// TestValidationOutputIsBounded was removed in repair 3: it called Write on the
// collector directly, which is exactly how it stayed green while the production
// path (os/exec io.Copy -> promoted ReadFrom) bypassed the bound (Astra N6). The
// bound is now proven through a real process in git_validation_output_test.go.

func TestPredecessorAdmissionFenceSurvivesGenerationChange(t *testing.T) {
	ledger := Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	lease, err := Admit(context.Background(), ledger, nil, "goal", "1", "old-inv", ModeSupervised, "scope", "head", time.Minute, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	unresolved, err := ledger.UnreleasedOtherGenerationAdmissions(context.Background(), "goal", "2")
	if err != nil || len(unresolved) != 1 {
		t.Fatalf("predecessor was not fenced: %+v %v", unresolved, err)
	}
	if err := lease.Release(context.Background(), "complete"); err != nil {
		t.Fatal(err)
	}
	unresolved, err = ledger.UnreleasedOtherGenerationAdmissions(context.Background(), "goal", "2")
	if err != nil || len(unresolved) != 0 {
		t.Fatalf("released predecessor remained fenced: %+v %v", unresolved, err)
	}
}

type pendingGateCoordinator struct{ calls int }

func (g *pendingGateCoordinator) ReconcileAuthorityGate(_ context.Context, _ goals.GoalBaseline, candidate contracts.WorkCandidate, _ []contracts.GovernedArtifactEvidence, _ time.Time) (contracts.AuthorityGateResult, error) {
	g.calls++
	return contracts.AuthorityGateResult{Request: contracts.AuthorityRequest{ID: "pending-" + candidate.ID, Version: "1"}}, nil
}

func gateAndWorkBaseline() (*goals.GoalBaseline, contracts.WorkCandidate) {
	gate := contracts.WorkCandidate{ID: "gate-a", Kind: contracts.WorkCandidateAuthorityGate, Priority: 1, Sequence: 1, SourceRef: "gate.json", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenanceAuthorityGate}
	return &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{gate}}}, gate
}

// Path F: a gate objective supplied explicitly, as a pinned/recovered
// objective, or with no candidates loaded by the caller must reach gate
// coordination and never a provider, however it arrived.
func TestGateObjectiveFromEveryRouteReachesCoordinationAndNeverAProvider(t *testing.T) {
	baseline, gate := gateAndWorkBaseline()
	routes := map[string]TurnRequest{
		"automatic selection":                 {},
		"explicit objective":                  {ChildObjective: "gate-a"},
		"explicit objective, caller supplied": {ChildObjective: "gate-a", WorkCandidates: []contracts.WorkCandidate{gate}},
		"recovered or pinned objective":       {ChildObjective: "gate-a", Recovery: &WorkerRecoveryContext{RecoveredTurnID: "earlier"}},
	}
	for name, route := range routes {
		t.Run(name, func(t *testing.T) {
			worker := &countingWorker{}
			registered := NewRegistry()
			if err := registered.Register("p", worker); err != nil {
				t.Fatal(err)
			}
			coordinator := &pendingGateCoordinator{}
			controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Providers: registered, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}, AuthorityGates: coordinator}
			req := route
			req.GoalID, req.GoalVersion, req.InvocationID, req.TurnID, req.Mode, req.ProviderID, req.GoalBaseline = "goal", "1", "inv", "turn", ModeSupervised, "p", baseline
			_, err := controller.ExecuteTurn(context.Background(), req)
			var required *AuthorityRequiredError
			if !errors.As(err, &required) || coordinator.calls != 1 || worker.calls != 0 {
				t.Fatalf("gate did not route to coordination without a provider: err=%v coordinator=%d worker=%d", err, coordinator.calls, worker.calls)
			}
		})
	}
}

// The provider resolver itself refuses a gate objective, so a future
// controller entry cannot dispatch one by resolving a worker directly.
func TestWorkerResolutionRefusesAuthorityGateObjective(t *testing.T) {
	baseline, _ := gateAndWorkBaseline()
	worker := &countingWorker{}
	controller := Controller{Worker: worker}
	if _, err := controller.worker(TurnRequest{ChildObjective: "gate-a", GoalBaseline: baseline}); !errors.Is(err, ErrHumanAuthorityGate) {
		t.Fatalf("resolver handed a provider to a gate: %v", err)
	}
}

func TestSafetyPlanRefusesAnObjectiveOutsideTheAcceptedPlan(t *testing.T) {
	baseline, _ := gateAndWorkBaseline()
	worker := &countingWorker{}
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}}
	_, err := controller.ExecuteTurn(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", Mode: ModeSupervised, GoalBaseline: baseline, ChildObjective: "invented"})
	if err == nil || worker.calls != 0 {
		t.Fatalf("an objective outside the accepted plan reached a worker: %v calls=%d", err, worker.calls)
	}
}

// B5: execution-derived completion never survives the overlay unless a
// qualified ledger record supports it, and unsupported ledger records are not
// accepted evidence for a safety-bearing plan.
func TestCompletionIsDerivedOnlyFromQualifiedLedgerEvidence(t *testing.T) {
	forged := contracts.WorkCandidate{ID: "gate-a", Kind: contracts.WorkCandidateAuthorityGate, Completed: true}
	if out := ApplyCompletions([]contracts.WorkCandidate{forged}, nil); out[0].Completed {
		t.Fatal("a caller-supplied Completed flag survived the overlay")
	}
	baseline, gate := gateAndWorkBaseline()
	plan := baseline.WorkPlan
	ok := UnitCompletion{UnitID: gate.ID, AuthorityGate: true, SpecificationDigest: gate.SourceDigest}
	if err := VerifyPlanCompletions(plan, []UnitCompletion{ok}); err != nil {
		t.Fatalf("a genuine gate completion was refused: %v", err)
	}
	for name, bad := range map[string]UnitCompletion{
		"unknown unit":               {UnitID: "ghost", AuthorityGate: true, SpecificationDigest: gate.SourceDigest},
		"wrong specification":        {UnitID: gate.ID, AuthorityGate: true, SpecificationDigest: "sha256:" + strings.Repeat("7", 64)},
		"gate without gate evidence": {UnitID: gate.ID, SpecificationDigest: gate.SourceDigest},
		"gate with mechanism claims": {UnitID: gate.ID, AuthorityGate: true, SpecificationDigest: gate.SourceDigest, MechanismTestsPassed: true, ConformanceQualified: true},
	} {
		if err := VerifyPlanCompletions(plan, []UnitCompletion{bad}); err == nil {
			t.Fatalf("%s: unsupported completion evidence was accepted", name)
		}
	}
	work := contracts.WorkCandidate{ID: "unit", Kind: contracts.WorkCandidateOrdinary, SourceDigest: "sha256:" + strings.Repeat("5", 64)}
	plan = &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{work}}
	unqualified := UnitCompletion{UnitID: "unit", SpecificationDigest: work.SourceDigest}
	if err := VerifyPlanCompletions(plan, []UnitCompletion{unqualified}); err == nil {
		t.Fatal("an unqualified unit completion counted toward a safety-bearing plan")
	}
	wrongProfile := UnitCompletion{UnitID: "unit", SpecificationDigest: work.SourceDigest, MechanismTestsPassed: true, ConformanceQualified: true, ValidationProfileDigest: "sha256:" + strings.Repeat("8", 64)}
	if err := VerifyPlanCompletions(plan, []UnitCompletion{wrongProfile}); err == nil {
		t.Fatal("a completion under a different validation profile was accepted")
	}
}

// B10: with the validator gone at the post-worker inspection, a safety
// checkpoint is blocked; and a completion cannot be settled from a
// no-declared-validation record.
func TestMissingValidatorAfterWorkerBlocksSafetyCheckpoint(t *testing.T) {
	c := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding()}}
	for name, tc := range map[string]struct {
		repo RepositoryAdapter
		want string
	}{
		"no validator interface": {&supervisionRepository{head: "after"}, "lost its checkpoint-bound validator"},
		"validator vanished":     {&conformanceRepository{supervisionRepository: supervisionRepository{head: "after"}, noValidator: true}, "required validator is missing or not executable after worker execution"},
	} {
		rec, err := c.deriveRepositoryOutcome(context.Background(), TurnRequest{GoalBaseline: baseline, StartHead: "before"}, tc.repo, TurnRecord{})
		if err == nil || rec.Progress || containsEvidence(rec.CheckpointEvidence, "repository:no-declared-validation") {
			t.Fatalf("%s: unvalidated safety checkpoint qualified: %+v %v", name, rec, err)
		}
		// Each disappearance is refused at its own check, not by a later layer.
		if !strings.Contains(rec.Blocker, tc.want) {
			t.Fatalf("%s: refused for another reason than the missing validator: %q", name, rec.Blocker)
		}
	}
}

func TestNoDeclaredValidationCannotSettleASafetyCompletion(t *testing.T) {
	c := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}}
	unit := contracts.WorkCandidate{ID: "unit", SourceDigest: "sha256:" + strings.Repeat("4", 64), QualificationPredicates: []string{"candidate/unit/conformance"}}
	b := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{unit, {ID: "other"}}}}
	rec := TurnRecord{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "unit", EndHead: "head", Progress: true, CheckpointPublished: true, CompletionClaim: "unit", CheckpointEvidence: []string{"repository:no-declared-validation"}}
	repo := &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}, output: "PRAXIS-VALIDATION candidate/unit/conformance\n"}
	if _, err := c.settleCompletion(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "unit", GoalBaseline: b, WorkCandidates: b.WorkPlan.Candidates}, repo, rec, nil); err != nil {
		t.Fatal(err)
	}
	if completions, err := c.Ledger.LoadCompletions(context.Background(), "goal", "1"); err != nil || len(completions) != 0 {
		t.Fatalf("no-declared-validation was promoted to a qualified completion: %+v %v", completions, err)
	}
}
