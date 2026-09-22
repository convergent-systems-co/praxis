package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Repair 3 composed regressions against a real encrypted GoalStore, the real
// lifecycle CLI, the production Goal-drive constructor, a real Git remote and a
// deterministic (no live provider) worker. They promote Astra Review #3's
// counterexamples (N1, N2, R1-G) and its independently constructed Path A
// end-to-end run into permanent tests.

// pathAValidatorScript is the validation entrypoint the plan binds. It passes the
// integrated run and acknowledges every candidate conformance predicate.
const pathAValidatorScript = `#!/bin/sh
set -eu
if [ "${1:-integrated}" = integrated ]; then exit 0; fi
printf 'PRAXIS-VALIDATION %s\n' "$1"
`

// strippedGeneration turns an attached safety-bearing generation into the
// downgraded form Astra reproduced: no safety binding, no kinds, no gate
// provenance, and a prerequisite pre-marked completed.
func strippedGeneration(h *gateHarness, version string) goals.GoalBaseline {
	b := h.successor
	plan := *b.WorkPlan
	plan.Safety = nil
	plan.Candidates = append([]contracts.WorkCandidate(nil), plan.Candidates...)
	for i := range plan.Candidates {
		plan.Candidates[i].Kind = ""
		plan.Candidates[i].Specification = nil
		if plan.Candidates[i].Provenance == contracts.ProvenanceAuthorityGate {
			plan.Candidates[i].Provenance = contracts.ProvenancePLAN
		}
	}
	plan.Relationships = append([]contracts.WorkRelationship(nil), plan.Relationships...)
	for i := range plan.Relationships {
		plan.Relationships[i].Specification = nil
	}
	plan.Candidates[0].Completed = true
	b.WorkPlan = &plan
	b.Version, b.Digest, b.PredecessorDigest = version, "", h.successor.Digest
	return b
}

// N1 / I9. Once a Goal has been put under the safety kernel, no generic
// surface may give any generation of it a plan, whatever the plan claims about
// itself, and a plan-less successor cannot be driven as though it were legacy.
func TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	classified, err := repo.GoalSafetyClassified(h.ctx, repairGoalID)
	if err != nil || !classified {
		t.Fatalf("a safety-bearing proposal did not classify its Goal durably: %v %v", classified, err)
	}

	// Activation present and absent: classification, not activation, is what
	// refuses, so removing the manifest must not open the door.
	for _, withActivation := range []bool{true, false} {
		if !withActivation {
			h.removeActivation()
		}
		stripped := strippedGeneration(h, "3")
		if _, err := repo.Save(h.ctx, stripped, time.Now().UTC(), nil); !errors.Is(err, goalstore.ErrSafetyDowngrade) {
			t.Fatalf("Save persisted a stripped plan for a classified Goal (activation=%v): %v", withActivation, err)
		}
		fabricated := strippedGeneration(h, "4")
		plan := *fabricated.WorkPlan
		plan.AuthorityRef, plan.AuthorityDigest, plan.AcceptanceRef, plan.AcceptanceDigest = "fabricated", "fabricated", "fabricated", "fabricated"
		plan.AuthorityRequestID, plan.AuthorityRequestVersion, plan.AuthorityDecisionRef, plan.AuthorityDecisionVersion = "", "", "", ""
		fabricated.WorkPlan = &plan
		if _, err := repo.Save(h.ctx, fabricated, time.Now().UTC(), nil); !errors.Is(err, goalstore.ErrSafetyDowngrade) {
			t.Fatalf("Save persisted a plan with fabricated authority lineage for a classified Goal (activation=%v): %v", withActivation, err)
		}
		importPath := writeBaselineImportDocument(t, h.dir, "import-stripped.json", strippedGeneration(h, "5"), "")
		if _, err := h.run("goals-lifecycle", "--operation=import", "--input="+importPath); err == nil {
			t.Fatalf("the supported CLI import persisted a stripped plan (activation=%v)", withActivation)
		}
	}
	h.restoreActivation()

	// A legacy proposal cannot be introduced for the classified Goal either.
	legacy := repairSafetyProposal(h.successor)
	legacy.Safety = nil
	legacy.ID = "legacy-downgrade"
	for i := range legacy.Candidates {
		legacy.Candidates[i].Kind, legacy.Candidates[i].Specification = "", nil
		legacy.Candidates[i].Provenance = contracts.ProvenanceModelProposal
	}
	if _, err := repo.SaveWorkPlanProposal(h.ctx, legacy, "1", time.Now().UTC(), nil); !errors.Is(err, goalstore.ErrSafetyDowngrade) {
		t.Fatalf("a legacy proposal was accepted for a classified Goal: %v", err)
	}

	// A plan-less successor may exist (it is not a plan) but cannot be driven.
	successor, err := repo.SaveReplanningSuccessor(h.ctx, repairGoalID, "2", h.successor.Digest, "9", nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	ctrl := newGoalDriveController(h.ledger, nil, nil, repo, 0, h.env)
	ctrl.Worker = &recordingWorker{}
	_, err = ctrl.ExecuteTurn(h.ctx, goaldrive.TurnRequest{GoalID: repairGoalID, GoalVersion: "9", InvocationID: "n1", TurnID: "n1", GraphID: "g", GraphVersion: "1", StartHead: "head", ChildObjective: "one", Mode: goaldrive.ModeSupervised, GoalBaseline: &successor, Repository: contracts.RepositorySynced})
	if !errors.Is(err, goaldrive.ErrSafetyDowngrade) || ctrl.Worker.(*recordingWorker).calls != 0 {
		t.Fatalf("a plan-less successor of a classified Goal was driven as legacy: %v", err)
	}

	// The legitimate attached generation is untouched by all of the above.
	if err := repo.VerifyGoverningAuthority(h.ctx, h.successor, time.Now().UTC()); err != nil {
		t.Fatalf("the genuine attached generation no longer verifies: %v", err)
	}
}

type recordingWorker struct{ calls int }

func (w *recordingWorker) Execute(_ context.Context, _ goaldrive.WorkerRequest) (goaldrive.WorkerResult, error) {
	w.calls++
	return goaldrive.WorkerResult{}, nil
}

// R1-G. An acceptance granted for one Goal must not attach to another Goal that
// has identical content (the baseline digest does not cover Goal identity).
func TestKernelRepair3AcceptedPlanCannotAttachToADifferentGoal(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source, err := repo.Load(h.ctx, repairGoalID, "1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	clone := source
	clone.ID, clone.Digest = "goal:r3-clone", ""
	saved, err := repo.Save(h.ctx, clone, time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("control: an unrelated plan-less Goal must persist: %v", err)
	}
	plan := h.successor.WorkPlan
	if _, err := repo.AttachAcceptedWorkPlan(h.ctx, saved.ID, saved.Version, saved.Digest, plan.AcceptanceRef, "1", "2", time.Now().UTC(), nil); err == nil || !strings.Contains(err.Error(), "accepted for Goal") {
		t.Fatalf("an acceptance crossed Goals: %v", err)
	}
}

// N2 (R1-H), real store and ledger. A gate completion appended straight to the
// plaintext ledger, with no authenticated seal and no decision, must neither
// complete the gate nor open its dependents. Refusal is an error, not a silent
// skip, and no gate request is manufactured to paper over it.
func TestKernelRepair3N2ForgedLedgerGateCompletionIsRefused(t *testing.T) {
	h := newGateHarness(t)
	gate := h.successor.WorkPlan.Candidates[1]
	forged := goaldrive.UnitCompletion{GoalID: repairGoalID, GoalVersion: "2", UnitID: gate.ID, InvocationID: "forge", TurnID: "forge", EndHead: "authority:sha256:" + strings.Repeat("e", 64), CompletedAt: time.Now().UTC(), AuthorityGate: true, SpecificationDigest: gate.SourceDigest, Evidence: []string{"forged"}}
	if err := h.ledger.RecordCompletion(h.ctx, forged); err != nil {
		t.Fatal(err)
	}
	_, err := h.drive(h.ledger, "n2-forged")
	if !errors.Is(err, contracts.ErrCompletionUnauthenticated) {
		t.Fatalf("a forged gate completion was trusted: %v", err)
	}
	repo, db, openErr := openGovernedRepository(h.ctx, h.env)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer db.Close()
	if pending, _ := repo.PendingAuthorityRequests(h.ctx, repairGoalID, "2", time.Now().UTC()); len(pending) != 0 {
		t.Fatalf("a request was manufactured over a forged completion: %+v", pending)
	}
}

// N2 / I3 / I11 (R2's revocation probe). Revoking a gate decision keeps the
// historical completion true and readable, but it stops authorizing new work:
// the drive refuses, the gate and everything that depended on it are history,
// and the Goal is no longer structurally complete.
func TestKernelRepair3N2RevokedGateDecisionKeepsHistoryButStopsAuthorizingWork(t *testing.T) {
	h := newGateHarness(t)
	request := h.pendingGate(h.ledger, "turn-1")
	digest, _ := request.Digest()
	if _, err := h.decide(digest, "approve", "finish and reconcile", "DECIDE-APPROVE "+digest+" ALTERNATIVE finish and reconcile"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.drive(h.ledger, "turn-2"); !errors.Is(err, goaldrive.ErrHumanAuthorityGate) {
		t.Fatalf("approve/complete: %v", err)
	}
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	baseline := h.successor

	// Control: while the decision stands, the completion is authentic and effective.
	effective, err := goaldrive.LoadEffectiveCompletions(h.ctx, h.ledger, repo, &baseline, repairGoalID, "2")
	if err != nil || len(effective.Historical) != 0 || len(effective.Effective) != 2 {
		t.Fatalf("control: an approved gate's completion was not effective: %+v %v", effective, err)
	}

	now := time.Now().UTC()
	evidence, err := repo.LoadAuthorityDecisionEvidence(h.ctx, request.ID, request.Version, now)
	if err != nil {
		t.Fatal(err)
	}
	edigest, _ := evidence.Digest()
	revocation := contracts.AuthorityRevocation{RequestID: evidence.RequestID, RequestVersion: evidence.RequestVersion, DecisionRef: evidence.DecisionRef, DecisionVersion: evidence.DecisionVersion, DecisionDigest: edigest, RevocationRef: "r3-gate-revoke", RevocationVersion: "1", RevokedBy: h.root.Principal, AuthorityDigest: h.root.AuthorityModelDigest, EffectiveAt: now, Reason: "revoke the gate decision only"}
	if err := repo.SaveAuthorityRevocation(h.ctx, evidence.RequestID, evidence.RequestVersion, revocation, now, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	// History is untouched and readable.
	if raw, err := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2"); err != nil || len(raw) != 2 {
		t.Fatalf("historical completions were rewritten: %+v %v", raw, err)
	}
	if _, err := repo.LoadAuthorityDecisionEvidence(h.ctx, request.ID, request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("revoked decision evidence must stay readable: %v", err)
	}
	// But it no longer authorizes anything.
	effective, err = goaldrive.LoadEffectiveCompletions(h.ctx, h.ledger, repo, &baseline, repairGoalID, "2")
	if err != nil || len(effective.Effective) != 1 || effective.Effective[0].UnitID != "one" || len(effective.Historical) != 1 || effective.Historical[0].UnitID != "gate" {
		t.Fatalf("revocation did not demote the gate completion to history: %+v %v", effective, err)
	}
	assessment, err := goaldrive.AssessGoalCompletion(baseline, effective.Effective)
	if err != nil || assessment.AllUnitsComplete {
		t.Fatalf("a revoked gate still counts toward Goal completion: %+v %v", assessment, err)
	}
	if _, err := h.drive(h.ledger, "turn-3"); !errors.Is(err, goalstore.ErrAuthorityDecisionRevoked) {
		t.Fatalf("drive was not refused by the revoked gate decision: %v", err)
	}
}

// Path A end to end (Astra Review #3 disposition REQUIRES_SINGLE_END_TO_END_TEST).
// One composed run over the production boundaries and no live provider:
//
//	real lifecycle proposal -> review -> protected request -> owner decision ->
//	continuation acceptance and attachment -> production Goal-drive constructor
//	over a real SQLite ledger, GoalStore verifiers and activation check -> a
//	deterministic worker over a real Git remote -> bound integrated and
//	conformance validation on an exact checkpoint export -> pre-effect
//	revalidation -> publication -> sealed qualified completion -> restart (a
//	fresh runtime) -> the gate request -> the real owner ceremony -> gate
//	completion -> a completed plan selects nothing further.
func TestPathAEndToEndFromAuthorityThroughRestartAndGate(t *testing.T) {
	s := newSafetyLifecycle(t)
	digest := s.reviewAndRequest()
	s.approveWorkPlan(digest)
	if _, err := s.continueGoal("1"); err != nil {
		t.Fatalf("continue: %v", err)
	}
	h := &gateHarness{safetyLifecycle: s}

	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	pathAGit(t, root, "init", "-q", "--bare", remote)
	pathAGit(t, root, "init", "-q", "--initial-branch=main", work)
	pathAGit(t, work, "config", "user.email", "pathA@example.invalid")
	pathAGit(t, work, "config", "user.name", "pathA")
	pathAGit(t, work, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(work, "README"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".praxis", "validate"), []byte(pathAValidatorScript), 0o755); err != nil {
		t.Fatal(err)
	}
	pathAGit(t, work, "add", "-A")
	pathAGit(t, work, "commit", "-q", "-m", "base")
	pathAGit(t, work, "push", "-q", "-u", "origin", "main")

	body, err := json.Marshal(repairDossier("finish and reconcile", "authenticated interrupt"))
	if err != nil {
		t.Fatal(err)
	}
	dossierFile := filepath.Join(root, "dossier.json")
	if err := os.WriteFile(dossierFile, body, 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "worker.sh")
	workerBody := "#!/bin/sh\nset -eu\nmkdir -p docs\ncp " + dossierFile + " docs/dossier.json\ngit add docs/dossier.json\ngit commit -q -m 'unit one' -m 'Praxis-Unit-Complete: one'\nprintf '{\"outcome\":\"CONTINUE\",\"end_head\":\"%s\",\"checkpoint_valid\":true}' \"$(git rev-parse HEAD)\"\n"
	if err := os.WriteFile(script, []byte(workerBody), 0o755); err != nil {
		t.Fatal(err)
	}
	argv, _ := json.Marshal([]string{"/bin/sh", script})
	getenv := func(key string) string {
		if key == "PRAXIS_GOAL_WORKER_ARGV" {
			return string(argv)
		}
		return s.env(key)
	}
	execute := func(id string) (goaldrive.TurnRecord, error) {
		inv := goaldrive.InvocationRequest{ProviderID: "p", RepositoryPath: work, Branch: "main", GoalVersion: "2", InvocationID: id, Mode: goaldrive.ModeSupervised}
		inv.Input = contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: repairGoalID}
		runtime, db, err := buildGoalDriveRuntime(s.ctx, normalizedOutput{GraphID: "g", GraphVersion: "1"}, inv, getenv)
		if err != nil {
			t.Fatalf("build runtime: %v", err)
		}
		defer db.Close()
		return runtime.Execute(s.ctx, inv)
	}

	before := pathAGit(t, work, "ls-remote", "origin", "refs/heads/main")
	record, err := execute("pathA-1")
	if err != nil || !record.UnitCompleted || !record.CheckpointPublished {
		t.Fatalf("Path A did not complete end to end: %v %+v", err, record)
	}
	if pathAGit(t, work, "ls-remote", "origin", "refs/heads/main") == before {
		t.Fatal("the qualified checkpoint was not published")
	}

	// The completion the run earned is authentic: sealed under the storage key
	// and effective through the same boundary every consumer uses.
	repo, db, err := openGovernedRepository(s.ctx, s.env)
	if err != nil {
		t.Fatal(err)
	}
	attached, err := repo.Load(s.ctx, repairGoalID, "2", time.Now().UTC())
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	durableLedger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	effective, err := goaldrive.LoadEffectiveCompletions(s.ctx, durableLedger, repo, &attached, repairGoalID, "2")
	db.Close()
	if err != nil || len(effective.Effective) != 1 || effective.Effective[0].UnitID != "one" || !effective.Effective[0].MechanismTestsPassed || !effective.Effective[0].ConformanceQualified {
		t.Fatalf("the completion the turn earned is not authenticated evidence: %+v %v", effective, err)
	}

	// Restart (a fresh runtime over the same durable state) reaches the gate.
	_, err = execute("pathA-2")
	var required *goaldrive.AuthorityRequiredError
	if !errors.As(err, &required) || len(required.Requests) != 1 {
		t.Fatalf("expected exactly one pending gate request after restart: %v", err)
	}
	requestDigest, _ := required.Requests[0].Digest()
	if _, err := h.decide(requestDigest, "approve", "finish and reconcile", "DECIDE-APPROVE "+requestDigest+" ALTERNATIVE finish and reconcile"); err != nil {
		t.Fatalf("owner ceremony: %v", err)
	}
	if _, err := execute("pathA-3"); !errors.Is(err, goaldrive.ErrHumanAuthorityGate) {
		t.Fatalf("gate completion expected: %v", err)
	}
	if _, err := execute("pathA-4"); err == nil {
		t.Fatal("a complete plan selected more work")
	}
}

func pathAGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// N7 at the settlement surfaces. The Goal completion candidate an owner is
// shown before settling is a plaintext ledger record. A candidate that names a
// completion the authenticated set does not contain (here: a gate that was never
// decided) must be refused by BOTH the evaluation and the settlement operation
// before any evaluation is composed or any confirmation is offered.
func TestKernelRepair3N7SettlementSurfacesRefuseAForgedCompletionCandidate(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	durable := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	// Authentic: the DOS producer completion, sealed and appended like the controller does.
	h.completeProducer(durable, "2", h.dossier)
	authentic, err := durable.LoadCompletions(h.ctx, repairGoalID, "2")
	if err != nil || len(authentic) != 1 {
		t.Fatalf("setup: %+v %v", authentic, err)
	}
	gate := h.successor.WorkPlan.Candidates[1]
	forgedGate := goaldrive.UnitCompletion{GoalID: repairGoalID, GoalVersion: "2", UnitID: gate.ID, InvocationID: "forge", TurnID: "forge", EndHead: "authority:sha256:" + strings.Repeat("e", 64), CompletedAt: time.Now().UTC(), AuthorityGate: true, SpecificationDigest: gate.SourceDigest, Evidence: []string{"forged"}}
	candidate := goaldrive.GoalCompletionCandidate{GoalID: repairGoalID, GoalVersion: "2", GoalDigest: h.successor.Digest, InvocationID: "inv", TurnID: "forged-turn", FinalHead: "head-1", Units: []goaldrive.UnitCompletion{authentic[0], forgedGate}, Assessment: goaldrive.GoalCompletionAssessment{AllUnitsComplete: true}, CandidateAt: time.Now().UTC()}
	if err := durable.RecordGoalCompletionCandidate(h.ctx, candidate); err != nil {
		t.Fatal(err)
	}
	_ = repo

	options := map[string]string{"goal-id": repairGoalID, "goal-version": "2", "status": "incomplete", "reason": "the contract still lacks something"}
	err = runGoalCompleteWithTerminal(h.ctx, options, h.env, strings.NewReader("\n"), &bytes.Buffer{})
	if err == nil || !errors.Is(err, contracts.ErrCompletionUnauthenticated) {
		t.Fatalf("settlement accepted a candidate naming a completion that is not authenticated: %v", err)
	}
	doc, _ := json.Marshal(map[string]any{"evaluator": contracts.PrincipalRef{ID: "eval", Kind: "agent"}, "evaluator_kind": "agent", "based_on": "", "goal_digest": h.successor.Digest, "candidate_turn_id": "forged-turn", "final_head": "head-1", "findings": []any{}})
	err = runGoalEvaluate(h.ctx, map[string]string{"goal-id": repairGoalID, "goal-version": "2"}, doc, h.env, &bytes.Buffer{})
	if err == nil || !errors.Is(err, contracts.ErrCompletionUnauthenticated) {
		t.Fatalf("evaluation composed over a candidate naming a completion that is not authenticated: %v", err)
	}
}

// I9: classification exists from the moment a safety-bearing proposal exists,
// before any review, request, acceptance or attachment.
func TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted(t *testing.T) {
	// The fixture ends after the safety-bearing PROPOSAL: no review, request,
	// decision, acceptance or attachment exists yet.
	s := newSafetyLifecycle(t)
	repo, db, err := openGovernedRepository(s.ctx, s.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if classified, err := repo.GoalSafetyClassified(s.ctx, repairGoalID); err != nil || !classified {
		t.Fatalf("a safety-bearing proposal did not classify its Goal: %v %v", classified, err)
	}
	if got := s.durable(); got.acceptances != 0 || got.successors != 0 {
		t.Fatalf("test premise violated: something was accepted or attached: %+v", got)
	}
}

// N2 at the store: a gate completion's lineage is re-resolved from authenticated
// state at every consumption. Each citation, each recorded fact and the ceremony
// behind the decision is checked on its own.
func TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState(t *testing.T) {
	h := newGateHarness(t)
	request := h.pendingGate(h.ledger, "turn-1")
	requestDigest, _ := request.Digest()
	if _, err := h.decide(requestDigest, "approve", "finish and reconcile", "DECIDE-APPROVE "+requestDigest+" ALTERNATIVE finish and reconcile"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.drive(h.ledger, "turn-2"); !errors.Is(err, goaldrive.ErrHumanAuthorityGate) {
		t.Fatalf("approve/complete: %v", err)
	}
	completions, err := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if err != nil || len(completions) != 2 {
		t.Fatalf("setup: %+v %v", completions, err)
	}
	var citedRequest, citedDecision string
	for _, item := range completions[1].Evidence {
		if strings.HasPrefix(item, "authority-request:") {
			citedRequest = strings.TrimPrefix(item, "authority-request:")
		}
		if strings.HasPrefix(item, "authority-decision:") {
			citedDecision = strings.TrimPrefix(item, "authority-decision:")
		}
	}
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gate := h.successor.WorkPlan.Candidates[1]
	artifacts := []contracts.GovernedArtifactEvidence{h.dossier}
	verify := func(req, dec string) error {
		return repo.VerifyGateCompletion(h.ctx, h.successor, gate, artifacts, req, dec, time.Now().UTC())
	}
	if err := verify(citedRequest, citedDecision); err != nil {
		t.Fatalf("control: a genuine gate completion did not verify: %v", err)
	}
	zeros := "sha256:" + strings.Repeat("0", 64)
	if err := verify(zeros, citedDecision); !errors.Is(err, contracts.ErrCompletionUnauthenticated) || !strings.Contains(err.Error(), "does not cite the request its dossier implies") {
		t.Fatalf("a completion citing a request its dossier does not imply verified: %v", err)
	}
	if err := verify(citedRequest, zeros); !errors.Is(err, contracts.ErrCompletionUnauthenticated) || !strings.Contains(err.Error(), "differs from the completion's citation") {
		t.Fatalf("a completion citing a decision other than the recorded one verified: %v", err)
	}
	// The ceremony record behind the decision is required at consumption too:
	// deleting it (which needs the database file, not the storage key) must not
	// leave a gate that still counts.
	gateDecision, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE namespace = 'owner_ceremony' AND object_id = ?`, gateDecision.CeremonyEvidenceDigest); err != nil {
		t.Fatal(err)
	}
	if err := verify(citedRequest, citedDecision); !errors.Is(err, contracts.ErrCompletionUnauthenticated) || !strings.Contains(err.Error(), "ceremony") {
		t.Fatalf("a gate decision whose ceremony record is gone still verified: %v", err)
	}
}

// N2 at the store (recorded request differs): a request stored under the gate's
// deterministic identity that is not the one the dossier implies never
// authenticates a completion, even when the completion cites the implied digest.
func TestKernelRepair3N2GateCompletionRefusesARecordedRequestThatDiffersFromTheImpliedOne(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gate := h.successor.WorkPlan.Candidates[1]
	contract, err := contracts.ParseAuthorityGateContract(gate.Specification)
	if err != nil {
		t.Fatal(err)
	}
	artifact, dossier, err := contracts.ResolveGateDossier(gate.ID, contract, []contracts.GovernedArtifactEvidence{h.dossier})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := contracts.InstallationGovernanceScope(strings.TrimPrefix(h.root.Ref, contracts.InstallationGovernanceScopePrefix))
	if err != nil {
		t.Fatal(err)
	}
	implied, err := goalstore.BuildAuthorityGateRequest(h.successor, gate, contract, artifact, dossier, scope)
	if err != nil {
		t.Fatal(err)
	}
	impliedDigest, _ := implied.Digest()
	different := implied
	different.Reason = "a different question under the same identity"
	if _, err := repo.SaveAuthorityRequest(h.ctx, different, time.Now().UTC(), nil); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err = repo.VerifyGateCompletion(h.ctx, h.successor, gate, []contracts.GovernedArtifactEvidence{h.dossier}, impliedDigest, "sha256:"+strings.Repeat("1", 64), time.Now().UTC())
	if !errors.Is(err, contracts.ErrCompletionUnauthenticated) || !strings.Contains(err.Error(), "recorded gate request differs") {
		t.Fatalf("a recorded request that differs from the implied one authenticated a completion: %v", err)
	}
}

// A completion that cites a REJECTED gate decision is not a completed gate: the
// lineage is authentic history, but a rejection is not an approval, so the
// completion is demoted rather than trusted.
func TestKernelRepair3N2CompletionCitingARejectedGateDecisionIsNotEffective(t *testing.T) {
	h := newGateHarness(t)
	request := h.pendingGate(h.ledger, "turn-1")
	requestDigest, _ := request.Digest()
	if out, err := h.decide(requestDigest, "reject", "", "DECIDE-REJECT "+requestDigest); err != nil {
		t.Fatalf("owner rejection must be decidable: %v %s", err, out)
	}
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rejected, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	decisionDigest, _ := rejected.Digest()
	err = repo.VerifyGateCompletion(h.ctx, h.successor, h.successor.WorkPlan.Candidates[1], []contracts.GovernedArtifactEvidence{h.dossier}, requestDigest, decisionDigest, time.Now().UTC())
	if !errors.Is(err, contracts.ErrGateAuthorityNotEffective) {
		t.Fatalf("a completion citing a rejected decision was not demoted: %v", err)
	}
}
