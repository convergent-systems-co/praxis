package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Repair 3 regressions for I9 (downgrade resistance), I10 (outward-effect
// equivalence) and I11 (authenticated completion consumption). Each names the
// Astra finding or surviving mutation it closes.

// nthCallGate fails from its n-th call on. Zero never fails.
type nthCallGate struct {
	mu     sync.Mutex
	calls  int
	failAt int
}

func (g *nthCallGate) hit() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return g.failAt > 0 && g.calls >= g.failAt
}

// driftingAuthority is governing authority that is revoked mid-turn.
type driftingAuthority struct {
	passingPlanAuthority
	gate *nthCallGate
}

func (a *driftingAuthority) VerifyGoverningAuthority(context.Context, goals.GoalBaseline, time.Time) error {
	if a.gate.hit() {
		return errors.New("governing authority revoked mid-turn")
	}
	return nil
}

type driftingActivation struct{ gate *nthCallGate }

func (a driftingActivation) Verify(context.Context, contracts.WorkPlanSafetyBinding) error {
	if a.gate.hit() {
		return errors.New("kernel activation drifted mid-turn")
	}
	return nil
}

// driftingRepository is a real GitRepository whose content binding drifts.
type driftingRepository struct {
	GitRepository
	gate *nthCallGate
}

func (r driftingRepository) VerifyValidationBinding(ctx context.Context, checkpoint, profile string) error {
	if r.gate.hit() {
		return errors.New("validated bytes are no longer attributable to the checkpoint")
	}
	return r.GitRepository.VerifyValidationBinding(ctx, checkpoint, profile)
}

func (f *boundGitFixture) turnWith(id, command string, authority PlanAuthorityVerifier, activation SafetyActivationVerifier, repo RepositoryAdapter) (TurnRecord, error) {
	f.t.Helper()
	worker := ProviderCLIWorker{ProviderID: "local-subscription-test", Dir: f.work, Command: []string{"/bin/sh", "-c", command}}
	controller := Controller{Ledger: f.ledger, Worker: worker, NoProgressLimit: 1, SafetyActivation: activation, GoverningAuthority: authority}
	return controller.ExecuteTurnWithRepository(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: id, TurnID: id + ":turn:1", GraphID: "g", GraphVersion: "1", Mode: ModeSupervised, GoalBaseline: f.baseline}, repo)
}

const commitWithoutClaim = `printf 'w\n' > w.txt && git add w.txt && git commit -q -m w`

// I10 / surviving mutations M2b, M2d, M6b and Astra R3-A3 (publication is not
// re-verified). The verifier that flips is counted per protected boundary: a
// claim turn establishes each predicate at admission (1), before publication
// (2), and before the completion is recorded (3). Publication is an outward
// effect: when a predicate fails before it, nothing may have been pushed. When
// it fails after it, the turn is durably recorded as published-but-ungoverned.
func TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens(t *testing.T) {
	type flip struct {
		name    string
		claim   bool
		build   func(f *boundGitFixture, gate *nthCallGate) (PlanAuthorityVerifier, SafetyActivationVerifier, RepositoryAdapter)
		failAt  int
		pushed  bool
		wantErr error
	}
	authorityFlip := func(f *boundGitFixture, gate *nthCallGate) (PlanAuthorityVerifier, SafetyActivationVerifier, RepositoryAdapter) {
		return &driftingAuthority{gate: gate}, passingSafetyActivation{}, f.repository
	}
	activationFlip := func(f *boundGitFixture, gate *nthCallGate) (PlanAuthorityVerifier, SafetyActivationVerifier, RepositoryAdapter) {
		return passingPlanAuthority{}, driftingActivation{gate: gate}, f.repository
	}
	bindingFlip := func(f *boundGitFixture, gate *nthCallGate) (PlanAuthorityVerifier, SafetyActivationVerifier, RepositoryAdapter) {
		return passingPlanAuthority{}, passingSafetyActivation{}, driftingRepository{GitRepository: f.repository, gate: gate}
	}
	cases := []flip{
		{name: "authority revoked before publication (progress-only turn)", build: authorityFlip, failAt: 2, wantErr: ErrPlanAuthority},
		{name: "authority revoked before publication (claim turn)", claim: true, build: authorityFlip, failAt: 2, wantErr: ErrPlanAuthority},
		{name: "authority revoked after publication, before completion", claim: true, build: authorityFlip, failAt: 3, pushed: true, wantErr: ErrPlanAuthority},
		{name: "activation drifts before publication (progress-only turn)", build: activationFlip, failAt: 2, wantErr: ErrSafetyActivation},
		{name: "activation drifts before publication (claim turn)", claim: true, build: activationFlip, failAt: 2, wantErr: ErrSafetyActivation},
		{name: "activation drifts after publication, before completion", claim: true, build: activationFlip, failAt: 3, pushed: true, wantErr: ErrSafetyActivation},
		{name: "validation binding drifts before publication (progress-only turn)", build: bindingFlip, failAt: 1},
		{name: "validation binding drifts before publication (claim turn)", claim: true, build: bindingFlip, failAt: 1},
		{name: "validation binding drifts after publication, before completion", claim: true, build: bindingFlip, failAt: 2, pushed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBoundGitFixture(t)
			before := f.remoteHead()
			gate := &nthCallGate{failAt: tc.failAt}
			authority, activation, repo := tc.build(f, gate)
			command := commitWithoutClaim
			if tc.claim {
				command = commitWithClaim
			}
			record, err := f.turnWith("inv-eff", command, authority, activation, repo)
			if err == nil {
				t.Fatalf("a predicate that failed at boundary %d was ignored: %+v", tc.failAt, record)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("unexpected refusal: %v", err)
			}
			moved := f.remoteHead() != before
			if moved != tc.pushed {
				t.Fatalf("outward effect mismatch: pushed=%v want=%v (err=%v)", moved, tc.pushed, err)
			}
			if completions, _ := f.ledger.LoadCompletions(context.Background(), "goal", "1"); len(completions) != 0 {
				t.Fatalf("a completion was recorded although an effect predicate failed: %+v", completions)
			}
			if !tc.pushed {
				return
			}
			// Reality changed: the record must say so rather than vanish.
			turns, loadErr := f.ledger.Load(context.Background(), "goal", "1")
			if loadErr != nil || len(turns) != 1 {
				t.Fatalf("a published-but-ungoverned checkpoint left no durable turn record: %+v %v", turns, loadErr)
			}
			if turns[0].Outcome != OutcomeBlocked || !turns[0].CheckpointPublished || turns[0].UnitCompleted || !strings.Contains(turns[0].Blocker, "published but governed completion was refused") {
				t.Fatalf("published-but-ungoverned checkpoint recorded untruthfully: %+v", turns[0])
			}
		})
	}
}

// Surviving mutation M12 (I1): a worker may claim completion only of the unit
// the controller selected. Another unit's predicates and evidence are not this
// turn's to qualify, and nothing is published.
func TestKernelRepair3ClaimOfANonSelectedUnitPublishesNothing(t *testing.T) {
	f := newBoundGitFixture(t)
	other := contracts.WorkCandidate{ID: "other", Kind: contracts.WorkCandidateOrdinary, Priority: 1, Sequence: 2, SourceRef: "other.json", SourceDigest: "sha256:" + strings.Repeat("5", 64), Provenance: contracts.ProvenancePLAN, QualificationPredicates: []string{"candidate/unit/conformance"}}
	f.baseline.WorkPlan.Candidates = append(f.baseline.WorkPlan.Candidates, other)
	before := f.remoteHead()
	record, err := f.turn("inv-m12", `printf 'w\n' > o.txt && git add o.txt && git commit -q -m o -m 'Praxis-Unit-Complete: other'`, passingPlanAuthority{})
	if err == nil || record.CheckpointPublished || record.UnitCompleted || f.remoteHead() != before {
		t.Fatalf("a claim for a unit the controller did not select was honoured: %+v err=%v", record, err)
	}
	if !strings.Contains(err.Error(), "worker claimed completion of other, which is not the selected unit") {
		t.Fatalf("the checkpoint inspection did not refuse the claim at its own layer: %v", err)
	}
	if completions, _ := f.ledger.LoadCompletions(context.Background(), "goal", "1"); len(completions) != 0 {
		t.Fatalf("completion recorded for a non-selected unit: %+v", completions)
	}
}

// The settlement layer refuses independently of the checkpoint inspection, so
// a future caller of settleCompletion cannot skip the selected-unit rule.
func TestKernelRepair3SettlementRefusesANonSelectedClaimByItself(t *testing.T) {
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}}
	unit := contracts.WorkCandidate{ID: "unit", QualificationPredicates: []string{"candidate/unit/conformance"}}
	other := contracts.WorkCandidate{ID: "other", QualificationPredicates: []string{"candidate/other/conformance"}}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{unit, other}}}
	record := TurnRecord{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "unit", EndHead: "head", Progress: true, CheckpointPublished: true, CompletionClaim: "other", CheckpointEvidence: []string{"repository:declared-validation-passed"}}
	repo := &conformanceRepository{supervisionRepository: supervisionRepository{head: "head"}, output: "PRAXIS-VALIDATION candidate/other/conformance\n"}
	_, err := controller.settleCompletion(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv", TurnID: "turn", ChildObjective: "unit", GoalBaseline: baseline, WorkCandidates: []contracts.WorkCandidate{unit, other}}, repo, record, nil)
	if err == nil || !strings.Contains(err.Error(), "not the selected unit") {
		t.Fatalf("settlement accepted a non-selected claim: %v", err)
	}
	if completions, _ := controller.Ledger.LoadCompletions(context.Background(), "goal", "1"); len(completions) != 0 {
		t.Fatalf("completion recorded: %+v", completions)
	}
}

// --- I11: authenticated completion consumption (N2) -----------------------

func gatePlanFixture() (*goals.GoalBaseline, contracts.WorkCandidate, contracts.WorkCandidate) {
	gate := contracts.WorkCandidate{ID: "gate", Kind: contracts.WorkCandidateAuthorityGate, Priority: 1, Sequence: 1, SourceRef: "gate.json", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenanceAuthorityGate}
	unit := contracts.WorkCandidate{ID: "unit", Kind: contracts.WorkCandidateOrdinary, Priority: 1, Sequence: 2, SourceRef: "unit.json", SourceDigest: "sha256:" + strings.Repeat("5", 64), Provenance: contracts.ProvenancePLAN, QualificationPredicates: []string{"candidate/unit/conformance"}}
	relationship := contracts.WorkRelationship{Dependent: "unit", Prerequisite: "gate", Kind: contracts.RelationshipHardDependency, SourceRef: "r.json", SourceDigest: "sha256:" + strings.Repeat("6", 64), Provenance: contracts.ProvenancePLAN}
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64), WorkPlan: &contracts.WorkPlan{Safety: protectedBinding(), Candidates: []contracts.WorkCandidate{gate, unit}, Relationships: []contracts.WorkRelationship{relationship}}}
	return baseline, gate, unit
}

func gateCompletion(gate contracts.WorkCandidate) UnitCompletion {
	requestDigest := "sha256:" + strings.Repeat("a", 64)
	decisionDigest := "sha256:" + strings.Repeat("d", 64)
	return UnitCompletion{GoalID: "goal", GoalVersion: "1", UnitID: gate.ID, InvocationID: "inv", TurnID: "turn", EndHead: "authority:" + decisionDigest, CompletedAt: time.Now().UTC(), AuthorityGate: true, SpecificationDigest: gate.SourceDigest, Evidence: []string{"dossier:sha256:" + strings.Repeat("e", 64), "authority-request:" + requestDigest, "authority-decision:" + decisionDigest}}
}

func newRepair3Controller(authority PlanAuthorityVerifier, worker Worker) Controller {
	return Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: authority}
}

func driveOnce(controller Controller, baseline *goals.GoalBaseline) error {
	_, err := controller.ExecuteTurn(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv-x", TurnID: "turn-x", GraphID: "g", GraphVersion: "1", StartHead: "head", Mode: ModeSupervised, GoalBaseline: baseline, Repository: contracts.RepositorySynced})
	return err
}

// N2: a gate completion appended straight to the plaintext ledger, with no
// authenticated seal, must neither skip the gate nor make its dependents
// eligible. Refusal is loud (an error), never a silent drop.
func TestKernelRepair3ForgedLedgerGateCompletionIsRefusedNotTrusted(t *testing.T) {
	baseline, gate, _ := gatePlanFixture()
	worker := &countingWorker{}
	controller := newRepair3Controller(passingPlanAuthority{}, worker)
	if err := controller.Ledger.RecordCompletion(context.Background(), gateCompletion(gate)); err != nil {
		t.Fatal(err)
	}
	if err := driveOnce(controller, baseline); !errors.Is(err, ErrUnauthenticatedCompletion) || worker.calls != 0 {
		t.Fatalf("a forged gate completion was trusted: err=%v calls=%d", err, worker.calls)
	}
}

// The same holds for an ordinary unit completion: derived-from-ledger is only
// as strong as the ledger, so it must carry the seal too.
func TestKernelRepair3ForgedLedgerUnitCompletionIsRefused(t *testing.T) {
	baseline, gate, unit := gatePlanFixture()
	worker := &countingWorker{}
	authority := passingPlanAuthority{}
	controller := newRepair3Controller(authority, worker)
	// A genuine, sealed gate completion, then a forged unit completion that
	// claims full qualification.
	if err := controller.recordCompletion(context.Background(), baseline, gateCompletion(gate)); err != nil {
		t.Fatal(err)
	}
	forged := UnitCompletion{GoalID: "goal", GoalVersion: "1", UnitID: unit.ID, InvocationID: "inv", TurnID: "forged", EndHead: "deadbeef", CompletedAt: time.Now().UTC(), MechanismTestsPassed: true, ConformanceQualified: true, SpecificationDigest: unit.SourceDigest, ValidationProfileDigest: baseline.WorkPlan.Safety.ValidationProfileDigest, Evidence: []string{"forged"}}
	if err := controller.Ledger.RecordCompletion(context.Background(), forged); err != nil {
		t.Fatal(err)
	}
	if err := driveOnce(controller, baseline); !errors.Is(err, ErrUnauthenticatedCompletion) || worker.calls != 0 {
		t.Fatalf("a forged unit completion was trusted: err=%v calls=%d", err, worker.calls)
	}
}

// A sealed completion whose ledger row was later altered no longer matches its
// seal: the ledger cannot be edited into a different consequence.
func TestKernelRepair3LedgerRowAlteredAfterSealingIsRefused(t *testing.T) {
	baseline, gate, _ := gatePlanFixture()
	authority := passingPlanAuthority{}
	genuine := gateCompletion(gate)
	payload, err := completionPayload(genuine)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.SealCompletion(context.Background(), *baseline, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	tampered := genuine
	tampered.Evidence = append(append([]string(nil), genuine.Evidence...), "smuggled")
	if _, err := AuthenticateCompletions(context.Background(), authority, baseline, []UnitCompletion{tampered}, time.Now()); !errors.Is(err, ErrUnauthenticatedCompletion) {
		t.Fatalf("a ledger row that differs from its seal was accepted: %v", err)
	}
	// Control: the exact sealed bytes authenticate.
	if effective, err := AuthenticateCompletions(context.Background(), authority, baseline, []UnitCompletion{genuine}, time.Now()); err != nil || len(effective.Effective) != 1 {
		t.Fatalf("genuine sealed completion refused: %+v %v", effective, err)
	}
}

// A completion sealed for one Goal generation cannot be presented for another.
func TestKernelRepair3CompletionSealedForAnotherGenerationIsRefused(t *testing.T) {
	baseline, gate, _ := gatePlanFixture()
	authority := passingPlanAuthority{}
	other := *baseline
	other.Version = "2"
	genuine := gateCompletion(gate)
	genuine.GoalVersion = "2"
	payload, _ := completionPayload(genuine)
	if _, err := authority.SealCompletion(context.Background(), other, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	replayed := genuine
	replayed.GoalVersion = "1"
	if _, err := AuthenticateCompletions(context.Background(), authority, baseline, []UnitCompletion{replayed}, time.Now()); !errors.Is(err, ErrUnauthenticatedCompletion) {
		t.Fatalf("a completion crossed generations: %v", err)
	}
}

// gateVerifier is authority whose gate decision can be revoked after the fact.
type gateVerifier struct {
	passingPlanAuthority
	notEffective bool
	unresolved   bool
}

func (g *gateVerifier) VerifyGateCompletion(context.Context, goals.GoalBaseline, contracts.WorkCandidate, []contracts.GovernedArtifactEvidence, string, string, time.Time) error {
	switch {
	case g.unresolved:
		return fmt.Errorf("%w: no such decision", contracts.ErrCompletionUnauthenticated)
	case g.notEffective:
		return fmt.Errorf("%w: revoked", contracts.ErrGateAuthorityNotEffective)
	}
	return nil
}

// I3 + I11: revocation of a gate decision does not rewrite history (the
// completion stays in the ledger and is reported as historical), but the
// completion stops authorizing new work: the gate is undecided again and its
// hard dependents are not eligible.
func TestKernelRepair3RevokedGateDecisionKeepsHistoryButStopsAuthorizingWork(t *testing.T) {
	baseline, gate, unit := gatePlanFixture()
	worker := &countingWorker{}
	authority := &gateVerifier{}
	controller := newRepair3Controller(authority, worker)
	if err := controller.recordCompletion(context.Background(), baseline, gateCompletion(gate)); err != nil {
		t.Fatal(err)
	}
	// While the decision stands the dependent unit is the runnable work.
	if err := driveOnce(controller, baseline); err == nil || !strings.Contains(err.Error(), "unsupported Goal turn outcome") || worker.calls != 1 {
		t.Fatalf("control: the dependent of a decided gate was not runnable: err=%v calls=%d", err, worker.calls)
	}
	worker.calls = 0
	authority.notEffective = true
	effective, err := LoadEffectiveCompletions(context.Background(), controller.Ledger, authority, baseline, "goal", "1")
	if err != nil || len(effective.Effective) != 0 || len(effective.Historical) != 1 || effective.Historical[0].UnitID != gate.ID {
		t.Fatalf("revoked decision was not demoted to history: %+v %v", effective, err)
	}
	// History is untouched in the ledger.
	if raw, err := controller.Ledger.LoadCompletions(context.Background(), "goal", "1"); err != nil || len(raw) != 1 {
		t.Fatalf("historical completion was erased: %+v %v", raw, err)
	}
	// The gate is undecided again: it is selected, goes to gate coordination
	// (never a worker), and the dependent does not run.
	err = driveOnce(controller, baseline)
	if worker.calls != 0 || err == nil {
		t.Fatalf("a revoked gate still authorized its dependent: err=%v calls=%d", err, worker.calls)
	}
	if !errors.Is(err, ErrHumanAuthorityGate) {
		t.Fatalf("revoked gate was not routed back to gate coordination: %v", err)
	}
	_ = unit
}

// A completed dependent inherits the demotion: a unit that finished under a
// gate that is no longer decided cannot make ITS dependents eligible.
func TestKernelRepair3DemotionPropagatesToUnitsThatCompletedUnderTheGate(t *testing.T) {
	baseline, gate, unit := gatePlanFixture()
	authority := &gateVerifier{}
	controller := newRepair3Controller(authority, &countingWorker{})
	if err := controller.recordCompletion(context.Background(), baseline, gateCompletion(gate)); err != nil {
		t.Fatal(err)
	}
	qualified := UnitCompletion{GoalID: "goal", GoalVersion: "1", UnitID: unit.ID, InvocationID: "inv", TurnID: "t2", EndHead: "head", CompletedAt: time.Now().UTC(), MechanismTestsPassed: true, ConformanceQualified: true, SpecificationDigest: unit.SourceDigest, ValidationProfileDigest: baseline.WorkPlan.Safety.ValidationProfileDigest, Evidence: []string{"checkpoint:head"}}
	if err := controller.recordCompletion(context.Background(), baseline, qualified); err != nil {
		t.Fatal(err)
	}
	authority.notEffective = true
	effective, err := LoadEffectiveCompletions(context.Background(), controller.Ledger, authority, baseline, "goal", "1")
	if err != nil || len(effective.Effective) != 0 || len(effective.Historical) != 2 {
		t.Fatalf("dependent completion kept authorizing work: %+v %v", effective, err)
	}
}

// A gate completion whose citations do not resolve is a forgery, distinct
// from a revoked-but-authentic decision, and is refused rather than demoted.
func TestKernelRepair3GateCompletionWithUnresolvableLineageIsRefused(t *testing.T) {
	baseline, gate, _ := gatePlanFixture()
	authority := &gateVerifier{unresolved: true}
	controller := newRepair3Controller(authority, &countingWorker{})
	if err := controller.recordCompletion(context.Background(), baseline, gateCompletion(gate)); err != nil {
		t.Fatal(err)
	}
	if err := driveOnce(controller, baseline); !errors.Is(err, ErrUnauthenticatedCompletion) {
		t.Fatalf("a gate completion with unresolvable lineage was tolerated: %v", err)
	}
}

// --- I9: downgrade resistance (N1), controller half ------------------------

// classifiedAuthority reports durable safety classification for its Goal.
type classifiedAuthority struct{ passingPlanAuthority }

func (classifiedAuthority) GoalSafetyClassified(context.Context, string) (bool, error) {
	return true, nil
}

type unresolvableClassifier struct{ passingPlanAuthority }

func (unresolvableClassifier) GoalSafetyClassified(context.Context, string) (bool, error) {
	return false, errors.New("classification store unavailable")
}

// N1: a generation of a Goal that authenticated state classifies as
// safety-bearing, presented WITHOUT the safety binding (stripped, omitted, or
// with a legacy plan or none at all), is refused before any worker.
func TestKernelRepair3ClassifiedGoalWithoutSafetyBindingIsRefusedBeforeAnyWorker(t *testing.T) {
	ordinary := contracts.WorkCandidate{ID: "unit", Priority: 1, Sequence: 1, SourceRef: "u", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenancePLAN}
	cases := map[string]*goals.GoalBaseline{
		"binding stripped from a plan": {ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64), WorkPlan: &contracts.WorkPlan{Candidates: []contracts.WorkCandidate{ordinary}}},
		"plan omitted entirely":        {ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64)},
	}
	for name, baseline := range cases {
		t.Run(name, func(t *testing.T) {
			worker := &countingWorker{}
			controller := newRepair3Controller(classifiedAuthority{}, worker)
			err := driveOnce(controller, baseline)
			if !errors.Is(err, ErrSafetyDowngrade) || worker.calls != 0 {
				t.Fatalf("a downgraded generation was driven: err=%v calls=%d", err, worker.calls)
			}
			_, runtimeErr := safetyBearingGoal(context.Background(), classifiedAuthority{}, baseline)
			if !errors.Is(runtimeErr, ErrSafetyDowngrade) {
				t.Fatalf("the shared classifier accepted a downgraded generation: %v", runtimeErr)
			}
		})
	}
}

// An unresolvable classification fails closed; a store fault is never "legacy".
func TestKernelRepair3UnresolvableClassificationFailsClosed(t *testing.T) {
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64)}
	worker := &countingWorker{}
	controller := newRepair3Controller(unresolvableClassifier{}, worker)
	if err := driveOnce(controller, baseline); err == nil || worker.calls != 0 {
		t.Fatalf("a classification fault was treated as legacy: err=%v calls=%d", err, worker.calls)
	}
}

// Control: a genuinely unclassified legacy generation still drives.
func TestKernelRepair3GenuineLegacyGenerationIsStillSupported(t *testing.T) {
	baseline := &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64)}
	worker := &countingWorker{}
	controller := newRepair3Controller(passingPlanAuthority{}, worker)
	controller.Worker = worker
	_, err := controller.ExecuteTurn(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv-l", TurnID: "turn-l", ChildObjective: "unit", WorkCandidates: []contracts.WorkCandidate{{ID: "unit", SourceRef: "u", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenancePLAN}}, GraphID: "g", GraphVersion: "1", StartHead: "head", Mode: ModeSupervised, GoalBaseline: baseline, Repository: contracts.RepositorySynced})
	if worker.calls != 1 {
		t.Fatalf("the legacy path no longer reaches its worker: calls=%d err=%v", worker.calls, err)
	}
}

// leaseLosingAuthority passes every check but marks the turn's lease lost when
// its n-th call happens, so only the lease predicate can refuse the effect.
type leaseLosingAuthority struct {
	passingPlanAuthority
	lease *TurnLease
	gate  *nthCallGate
}

func (a *leaseLosingAuthority) VerifyGoverningAuthority(context.Context, goals.GoalBaseline, time.Time) error {
	if a.gate.hit() {
		a.lease.lost.Store(true)
	}
	return nil
}

// I10: losing the turn's lease between qualification and an effect refuses the
// effect. Before publication nothing is pushed; after it the turn is recorded
// as published-but-ungoverned. (Pre-existing lease checks run before
// qualification; the one inside the effect boundary is the one that matters
// here, so the loss is injected only after them.)
func TestKernelRepair3LostLeaseRefusesTheEffect(t *testing.T) {
	for _, tc := range []struct {
		name   string
		loseAt int
		pushed bool
	}{{"before publication", 2, false}, {"after publication, before completion", 3, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBoundGitFixture(t)
			before := f.remoteHead()
			lease := &TurnLease{}
			authority := &leaseLosingAuthority{lease: lease, gate: &nthCallGate{failAt: tc.loseAt}}
			worker := ProviderCLIWorker{ProviderID: "local-subscription-test", Dir: f.work, Command: []string{"/bin/sh", "-c", commitWithClaim}}
			controller := Controller{Ledger: f.ledger, Worker: worker, NoProgressLimit: 1, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: authority}
			_, err := controller.ExecuteTurnWithRepository(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv-lease", TurnID: "inv-lease:turn:1", GraphID: "g", GraphVersion: "1", Mode: ModeSupervised, GoalBaseline: f.baseline, Lease: lease}, f.repository)
			if !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("a turn that lost its lease still produced an effect: %v", err)
			}
			if moved := f.remoteHead() != before; moved != tc.pushed {
				t.Fatalf("outward effect mismatch: pushed=%v want=%v", moved, tc.pushed)
			}
			if completions, _ := f.ledger.LoadCompletions(context.Background(), "goal", "1"); len(completions) != 0 {
				t.Fatalf("completion recorded without the lease: %+v", completions)
			}
			if turns, _ := f.ledger.Load(context.Background(), "goal", "1"); tc.pushed && len(turns) != 0 {
				t.Fatalf("a turn whose lease was lost recorded itself anyway (reconciliation owns the disposition): %+v", turns)
			}
		})
	}
}

// I11 at settlement: the Goal completion candidate is a plaintext ledger record
// that an owner is shown as "controller-verified" before a settlement decision.
// For a safety-bearing generation it is trusted only when it is exactly the
// authenticated, currently effective set.
func TestKernelRepair3GoalCompletionCandidateMustEqualTheAuthenticatedCompletions(t *testing.T) {
	baseline, gate, unit := gatePlanFixture()
	authority := &gateVerifier{}
	controller := newRepair3Controller(authority, &countingWorker{})
	gateDone := gateCompletion(gate)
	qualified := UnitCompletion{GoalID: "goal", GoalVersion: "1", UnitID: unit.ID, InvocationID: "inv", TurnID: "t2", EndHead: "head", CompletedAt: time.Now().UTC(), MechanismTestsPassed: true, ConformanceQualified: true, SpecificationDigest: unit.SourceDigest, ValidationProfileDigest: baseline.WorkPlan.Safety.ValidationProfileDigest, Evidence: []string{"checkpoint:head"}}
	for _, completion := range []UnitCompletion{gateDone, qualified} {
		if err := controller.recordCompletion(context.Background(), baseline, completion); err != nil {
			t.Fatal(err)
		}
	}
	verify := func(units ...UnitCompletion) error {
		return VerifyCandidateCompletions(context.Background(), controller.Ledger, authority, baseline, GoalCompletionCandidate{GoalID: "goal", GoalVersion: "1", Units: units})
	}
	if err := verify(gateDone, qualified); err != nil {
		t.Fatalf("control: the authenticated set was refused: %v", err)
	}
	forged := qualified
	forged.EndHead = "forged-checkpoint"
	if err := verify(gateDone, forged); !errors.Is(err, ErrUnauthenticatedCompletion) {
		t.Fatalf("a candidate carrying an altered completion was accepted: %v", err)
	}
	if err := verify(gateDone); !errors.Is(err, ErrUnauthenticatedCompletion) {
		t.Fatalf("a candidate omitting a completion was accepted: %v", err)
	}
	authority.notEffective = true
	if err := verify(gateDone, qualified); !errors.Is(err, contracts.ErrGateAuthorityNotEffective) {
		t.Fatalf("a candidate resting on a revoked gate decision supported settlement: %v", err)
	}
}

// I3/I11 (Astra R2-F5): an explicit, pinned or recovered objective is new work
// like a selected one. A unit whose hard prerequisite gate is not currently
// decided must not reach a worker on the strength of an objective id alone,
// including when the gate was decided once and that decision was since revoked.
func TestKernelRepair3ExplicitObjectiveMustBeCurrentlyRunnable(t *testing.T) {
	baseline, gate, _ := gatePlanFixture()
	run := func(controller Controller) error {
		_, err := controller.ExecuteTurn(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv-e", TurnID: "turn-e", ChildObjective: "unit", GraphID: "g", GraphVersion: "1", StartHead: "head", Mode: ModeSupervised, GoalBaseline: baseline, Repository: contracts.RepositorySynced})
		return err
	}
	// The gate was never decided.
	worker := &countingWorker{}
	if err := run(newRepair3Controller(&gateVerifier{}, worker)); err == nil || !strings.Contains(err.Error(), "not currently runnable") || worker.calls != 0 {
		t.Fatalf("an explicit objective behind an undecided gate reached a worker: err=%v calls=%d", err, worker.calls)
	}
	// Decided, then revoked: history stays, but the objective is not runnable.
	authority := &gateVerifier{}
	controller := newRepair3Controller(authority, worker)
	if err := controller.recordCompletion(context.Background(), baseline, gateCompletion(gate)); err != nil {
		t.Fatal(err)
	}
	authority.notEffective = true
	if err := run(controller); err == nil || !strings.Contains(err.Error(), "not currently runnable") || worker.calls != 0 {
		t.Fatalf("an explicit objective resumed on the strength of a revoked gate decision: err=%v calls=%d", err, worker.calls)
	}
	// Control: while the gate decision stands, the same objective runs.
	authority.notEffective = false
	worker.calls = 0
	_ = run(controller)
	if worker.calls != 1 {
		t.Fatalf("control: a runnable explicit objective no longer reaches its worker: calls=%d", worker.calls)
	}
}

// I9 at the runtime boundary: a generation of a classified Goal that carries no
// safety binding is refused BEFORE a turn is admitted, so it takes no lease and
// leaves no admission behind (the controller would refuse it later, after
// admission, so the controller check alone does not give this).
func TestKernelRepair3RuntimeRefusesADowngradedGenerationBeforeAdmission(t *testing.T) {
	store := eventstore.NewMemoryStore()
	actor := contracts.PrincipalRef{ID: "controller", Kind: "controller"}
	baseline := goals.GoalBaseline{ID: "goal:contract", Version: "2", Digest: "sha256:" + strings.Repeat("9", 64)}
	runtime := Runtime{Controller: Controller{Ledger: Ledger{Store: store, Actor: actor}, Worker: &countingWorker{}, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: classifiedAuthority{}}, Baselines: supervisionBaselineStore{baseline: baseline}, Repository: &phaseRepository{head: "base", remote: "base"}, GraphID: "g", GraphVersion: "1"}
	_, err := runtime.Execute(context.Background(), phaseInvocation("downgrade"))
	if !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("a downgraded generation was not refused as a downgrade: %v", err)
	}
	state, loadErr := runtime.Controller.Ledger.LoadAdmissions(context.Background(), "goal:contract", "2")
	if loadErr != nil || len(state.Admissions) != 0 {
		t.Fatalf("a downgraded generation was admitted before it was refused: %+v %v", state.Admissions, loadErr)
	}
}

// Materialization of a completion after the fact is refused for a classified
// Goal even when the generation in hand lacks the safety binding.
func TestKernelRepair3MaterializationRefusesAClassifiedGoalWithoutTheBinding(t *testing.T) {
	controller := newRepair3Controller(classifiedAuthority{}, &countingWorker{})
	baseline := goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64)}
	_, err := controller.MaterializeTurnCompletion(context.Background(), "goal", "1", "turn", baseline, nil, contracts.PrincipalRef{ID: "owner", Kind: "human"})
	if !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("materialization treated a classified Goal without the binding as legacy: %v", err)
	}
}

// globalSealAuthority is a governance store whose seals are keyed by digest alone
// (it ignores the Goal generation) and which can be told to return other bytes.
type globalSealAuthority struct {
	passingPlanAuthority
	seals   map[string][]byte
	replace []byte
}

func (a *globalSealAuthority) SealCompletion(_ context.Context, _ goals.GoalBaseline, payload []byte, _ time.Time) (string, error) {
	digest := payloadDigest(payload)
	a.seals[digest] = append([]byte(nil), payload...)
	return digest, nil
}

func (a *globalSealAuthority) LoadSealedCompletion(_ context.Context, _ goals.GoalBaseline, digest string, _ time.Time) ([]byte, error) {
	if a.replace != nil {
		return a.replace, nil
	}
	if payload, ok := a.seals[digest]; ok {
		return payload, nil
	}
	return nil, contracts.ErrCompletionUnauthenticated
}

// Consumption does not rely on the store alone: a completion that belongs to
// another Goal generation is refused even when a (buggy or hostile) store would
// vouch for its digest, and so are sealed bytes that are not the ledger's bytes.
func TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore(t *testing.T) {
	baseline, gate, _ := gatePlanFixture()
	authority := &globalSealAuthority{seals: map[string][]byte{}}
	genuine := gateCompletion(gate)
	payload, _ := completionPayload(genuine)
	if _, err := authority.SealCompletion(context.Background(), *baseline, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	if effective, err := AuthenticateCompletions(context.Background(), authority, baseline, []UnitCompletion{genuine}, time.Now()); err != nil || len(effective.Effective) != 1 {
		t.Fatalf("control: %+v %v", effective, err)
	}
	foreign := genuine
	foreign.GoalVersion = "2"
	foreignPayload, _ := completionPayload(foreign)
	authority.seals[payloadDigest(foreignPayload)] = foreignPayload
	if _, err := AuthenticateCompletions(context.Background(), authority, baseline, []UnitCompletion{foreign}, time.Now()); !errors.Is(err, ErrUnauthenticatedCompletion) {
		t.Fatalf("a completion of another generation authenticated because the store vouched for its digest: %v", err)
	}
	authority.replace = []byte(`{"unit_id":"someone-else"}`)
	if _, err := AuthenticateCompletions(context.Background(), authority, baseline, []UnitCompletion{genuine}, time.Now()); !errors.Is(err, ErrUnauthenticatedCompletion) {
		t.Fatalf("sealed bytes that are not the ledger's bytes authenticated a completion: %v", err)
	}
}

// A gate completion must cite exactly one request and the decision it names.
func TestKernelRepair3GateCompletionWithoutItsCitationsIsRefused(t *testing.T) {
	baseline, gate, _ := gatePlanFixture()
	authority := passingPlanAuthority{}
	for name, mutate := range map[string]func(*UnitCompletion){
		"no request cited":        func(c *UnitCompletion) { c.Evidence = []string{c.Evidence[0], c.Evidence[2]} },
		"no decision cited":       func(c *UnitCompletion) { c.Evidence = []string{c.Evidence[0], c.Evidence[1]} },
		"checkpoint not decision": func(c *UnitCompletion) { c.EndHead = "authority:sha256:" + strings.Repeat("f", 64) },
		"two requests cited": func(c *UnitCompletion) {
			c.Evidence = append(c.Evidence, "authority-request:sha256:"+strings.Repeat("b", 64))
		},
	} {
		completion := gateCompletion(gate)
		mutate(&completion)
		payload, _ := completionPayload(completion)
		if _, err := authority.SealCompletion(context.Background(), *baseline, payload, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := AuthenticateCompletions(context.Background(), authority, baseline, []UnitCompletion{completion}, time.Now()); !errors.Is(err, ErrUnauthenticatedCompletion) {
			t.Fatalf("%s: a malformed gate completion authenticated: %v", name, err)
		}
	}
}

// Demotion follows HARD dependencies only: a unit related to the revoked gate by
// a non-blocking relationship keeps its (authentic) completion effective.
func TestKernelRepair3DemotionDoesNotFollowNonBlockingRelationships(t *testing.T) {
	baseline, gate, unit := gatePlanFixture()
	advisory := contracts.WorkCandidate{ID: "advisory", Kind: contracts.WorkCandidateOrdinary, Priority: 1, Sequence: 3, SourceRef: "a.json", SourceDigest: "sha256:" + strings.Repeat("7", 64), Provenance: contracts.ProvenancePLAN, QualificationPredicates: []string{"candidate/advisory/conformance"}}
	baseline.WorkPlan.Candidates = append(baseline.WorkPlan.Candidates, advisory)
	baseline.WorkPlan.Relationships = append(baseline.WorkPlan.Relationships, contracts.WorkRelationship{Dependent: "advisory", Prerequisite: "gate", Kind: contracts.RelationshipAdvisory, SourceRef: "r2.json", SourceDigest: "sha256:" + strings.Repeat("8", 64), Provenance: contracts.ProvenancePLAN})
	authority := &gateVerifier{}
	controller := newRepair3Controller(authority, &countingWorker{})
	qualified := func(id string, spec string) UnitCompletion {
		return UnitCompletion{GoalID: "goal", GoalVersion: "1", UnitID: id, InvocationID: "inv", TurnID: "t-" + id, EndHead: "head", CompletedAt: time.Now().UTC(), MechanismTestsPassed: true, ConformanceQualified: true, SpecificationDigest: spec, ValidationProfileDigest: baseline.WorkPlan.Safety.ValidationProfileDigest, Evidence: []string{"checkpoint:head"}}
	}
	for _, completion := range []UnitCompletion{gateCompletion(gate), qualified("unit", unit.SourceDigest), qualified("advisory", advisory.SourceDigest)} {
		if err := controller.recordCompletion(context.Background(), baseline, completion); err != nil {
			t.Fatal(err)
		}
	}
	authority.notEffective = true
	effective, err := LoadEffectiveCompletions(context.Background(), controller.Ledger, authority, baseline, "goal", "1")
	if err != nil || len(effective.Effective) != 1 || effective.Effective[0].UnitID != "advisory" || len(effective.Historical) != 2 {
		t.Fatalf("demotion did not follow exactly the hard dependencies: %+v %v", effective, err)
	}
}
