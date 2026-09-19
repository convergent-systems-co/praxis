package goaldrive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// chainBaseline is a Goal with units A -> B -> C (hard dependencies) whose
// requirements cover three success criteria.
func chainBaseline() goals.GoalBaseline {
	req := func(n string) contracts.RequirementRef {
		return contracts.RequirementRef{ID: "req:" + n, SourceRef: "goal:chain/1#success_criteria/" + n, SourceDigest: "sha256:" + strings.Repeat(n, 64)}
	}
	unit := func(id string, order int, r contracts.RequirementRef) contracts.WorkCandidate {
		return contracts.WorkCandidate{ID: id, Priority: order, Sequence: order, SourceRef: "plan:" + id, SourceDigest: "sha256:" + strings.Repeat("a", 64), Provenance: contracts.ProvenancePLAN, Requirements: []contracts.RequirementRef{r}}
	}
	rel := func(dependent, prerequisite string) contracts.WorkRelationship {
		return contracts.WorkRelationship{Dependent: dependent, Prerequisite: prerequisite, Kind: contracts.RelationshipHardDependency, SourceRef: "plan:" + dependent, SourceDigest: "sha256:" + strings.Repeat("b", 64), Provenance: contracts.ProvenancePLAN}
	}
	return goals.GoalBaseline{ID: "goal:chain", Version: "2", Digest: "sha256:" + strings.Repeat("c", 64), OriginalIntent: "chain", RefinedOutcome: "A then B then C", SuccessCriteria: []string{"one", "two", "three"}, Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll,
		WorkPlan: &contracts.WorkPlan{BaselineDigest: "sha256:" + strings.Repeat("d", 64), AuthorityRef: "authority", AuthorityDigest: "sha256:" + strings.Repeat("e", 64), AcceptanceRef: "acceptance", AcceptanceDigest: "sha256:" + strings.Repeat("f", 64), AcceptedBy: contracts.PrincipalRef{ID: "human", Kind: "human"}, ProposalDigest: "sha256:" + strings.Repeat("1", 64),
			Candidates:    []contracts.WorkCandidate{unit("unit:a", 1, req("1")), unit("unit:b", 2, req("2")), unit("unit:c", 3, req("3"))},
			Relationships: []contracts.WorkRelationship{rel("unit:b", "unit:a"), rel("unit:c", "unit:b")}}}
}

type chainWorker struct {
	dir     string
	trailer string // unit named in the completion trailer, "" for progress only
	fail    bool   // make the declared validation fail
}

func (w chainWorker) Execute(_ context.Context, request WorkerRequest) (WorkerResult, error) {
	name := strings.ReplaceAll(request.TurnID, ":", "-")
	body := "ok\n"
	if w.fail {
		body = "FAIL\n"
	}
	if err := os.WriteFile(filepath.Join(w.dir, name+".txt"), []byte(body), 0o644); err != nil {
		return WorkerResult{}, err
	}
	args := []string{"commit", "-q", "-m", "work " + request.TurnID}
	if w.trailer != "" {
		args = append(args, "-m", CompletionTrailer+": "+w.trailer)
	}
	if _, err := (GitRepository{Dir: w.dir, Remote: "origin", Branch: "main"}).run(context.Background(), "add", "-A"); err != nil {
		return WorkerResult{}, err
	}
	if _, err := (GitRepository{Dir: w.dir, Remote: "origin", Branch: "main"}).run(context.Background(), args...); err != nil {
		return WorkerResult{}, err
	}
	head, _ := (GitRepository{Dir: w.dir, Remote: "origin", Branch: "main"}).run(context.Background(), "rev-parse", "HEAD")
	return WorkerResult{Outcome: OutcomeContinue, EndHead: strings.TrimSpace(head), CheckpointValid: true}, nil
}

func (w chainWorker) RepositoryResultIsControllerOwned() bool { return true }

// TestUnitCompletionDrivesDependencyOrder is the regression test for #158:
// partial progress leaves a unit eligible, a verified completion proposal
// records durable completion, dependents unlock in order, a fresh process
// reconstructs the state, and the Goal completes only when every unit is
// complete and every success criterion is covered.
func TestUnitCompletionDrivesDependencyOrder(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	if err := os.MkdirAll(filepath.Join(workDir, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workDir, ".praxis", "validate"), "#!/bin/sh\n! grep -rq FAIL --include='*.txt' .\n")
	if err := os.Chmod(filepath.Join(workDir, ".praxis", "validate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, workDir, "add", ".praxis")
	runGitTest(t, workDir, "commit", "-m", "declare validation")
	runGitTest(t, workDir, "push", "origin", "main")
	db, err := state.OpenSQLite(ctx, filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := state.NewSQLiteEventStore(db)
	baseline := chainBaseline()
	repo := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	turn := 0
	drive := func(worker Worker) (TurnRecord, error) {
		turn++
		controller := Controller{Ledger: Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, NoProgressLimit: 3}
		req := TurnRequest{GoalID: baseline.ID, GoalVersion: baseline.Version, InvocationID: "chain-" + string(rune('0'+turn)), TurnID: "chain-" + string(rune('0'+turn)) + ":turn:" + string(rune('0'+turn)), GraphID: "g", GraphVersion: "1", Mode: ModeSupervised, ProviderID: "local", GoalBaseline: &baseline}
		return controller.ExecuteTurnWithRepository(ctx, req, repo)
	}
	// 2, 3: turn 1 selects A; progress without a proposal leaves A eligible.
	record, err := drive(chainWorker{dir: workDir})
	if err != nil || record.ChildObjective != "unit:a" || !record.Progress || record.UnitCompleted || record.Outcome != OutcomeContinue {
		t.Fatalf("turn 1 must progress A without completing it: %+v %v", record, err)
	}
	record, err = drive(chainWorker{dir: workDir})
	if err != nil || record.ChildObjective != "unit:a" {
		t.Fatalf("A remains eligible after partial progress: %+v %v", record, err)
	}
	// 11: a proposal naming a unit other than the selected one is refused.
	record, err = drive(chainWorker{dir: workDir, trailer: "unit:b"})
	if err == nil || record.Outcome != OutcomeBlocked || record.UnitCompleted || !strings.Contains(record.Blocker, "not the selected unit") {
		t.Fatalf("a false completion claim must block without completion: %+v %v", record, err)
	}
	runGitTest(t, workDir, "reset", "-q", "--hard", "origin/main")
	// 12: a proposal on a checkpoint whose declared validation fails is refused.
	record, err = drive(chainWorker{dir: workDir, trailer: "unit:a", fail: true})
	if err == nil || record.Outcome != OutcomeBlocked || record.UnitCompleted {
		t.Fatalf("failed validation must not complete a unit: %+v %v", record, err)
	}
	runGitTest(t, workDir, "reset", "-q", "--hard", "origin/main")
	if completions, _ := (Ledger{Store: store}).LoadCompletions(ctx, baseline.ID, baseline.Version); len(completions) != 0 {
		t.Fatalf("no completion may exist yet: %+v", completions)
	}
	// 4, 5: a verified proposal completes A durably; the next turn selects B.
	record, err = drive(chainWorker{dir: workDir, trailer: "unit:a"})
	if err != nil || record.ChildObjective != "unit:a" || !record.UnitCompleted || record.CompletionClaim != "unit:a" || record.Outcome != OutcomeContinue {
		t.Fatalf("verified proposal must complete A and continue the Goal: %+v %v", record, err)
	}
	completions, err := (Ledger{Store: store}).LoadCompletions(ctx, baseline.ID, baseline.Version)
	if err != nil || len(completions) != 1 || completions[0].UnitID != "unit:a" || completions[0].EndHead != record.EndHead || !containsString(completions[0].Evidence, "repository:declared-validation-passed") {
		t.Fatalf("A's completion must be durable with checkpoint evidence: %v %+v", err, completions)
	}
	// 8: a fresh ledger over the same store reconstructs the state.
	fresh := Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	candidates, relationships, _ := MaterializeGoalWork(baseline)
	restored, _ := fresh.LoadCompletions(ctx, baseline.ID, baseline.Version)
	selected, err := contracts.SelectRunnableWork(ApplyCompletions(candidates, restored), relationships)
	if err != nil || selected.ID != "unit:b" {
		t.Fatalf("restart must select B: %+v %v", selected, err)
	}
	record, err = drive(chainWorker{dir: workDir, trailer: "unit:b"})
	if err != nil || record.ChildObjective != "unit:b" || !record.UnitCompleted || record.Outcome != OutcomeContinue {
		t.Fatalf("B must be selected after A and complete: %+v %v", record, err)
	}
	// 7, 14: C unlocks after B; completing C records the GOAL_COMPLETION_CANDIDATE
	// and the deterministic evaluation, and stops for settlement. The chain
	// validator is bound to no criterion, so every criterion is UNKNOWN and
	// only the integrated validation is satisfied: outcome UNKNOWN.
	record, err = drive(chainWorker{dir: workDir, trailer: "unit:c"})
	if err != nil || record.ChildObjective != "unit:c" || !record.UnitCompleted || record.Outcome != OutcomeUserDecisionRequired || !record.GoalCandidate || record.GoalEvaluation != string(ResultUnknown) {
		t.Fatalf("completing C yields a candidate with an unknown evaluation and stops for settlement: %+v %v", record, err)
	}
	ledger := Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	goalState, err := ledger.LoadGoalCompletion(ctx, baseline.ID, baseline.Version)
	if err != nil || goalState.Candidate == nil || goalState.Decision != nil || goalState.Candidate.FinalHead != record.EndHead || goalState.Candidate.TurnID != record.TurnID || len(goalState.Evaluations) != 1 {
		t.Fatalf("candidate and deterministic evaluation must be durable and unsettled: %v %+v", err, goalState)
	}
	latest := goalState.Latest()
	if latest.EvaluatorKind != EvaluatorDeterministic || latest.Outcome != ResultUnknown {
		t.Fatalf("deterministic evaluation: %+v", latest)
	}
	for _, item := range latest.Items {
		switch item.Kind {
		case ItemSuccessCriterion:
			if item.Result != ResultUnknown || len(item.Coverage) == 0 || !strings.Contains(item.Predicate, "requires judgment") {
				t.Fatalf("an unbound criterion stays UNKNOWN with coverage recorded as evidence only: %+v", item)
			}
		case ItemIntegratedValidator:
			if item.Result != ResultSatisfied {
				t.Fatalf("integrated validation must have run at the final head: %+v", item)
			}
		}
	}
	if _, err := drive(chainWorker{dir: workDir}); !errors.Is(err, ErrGoalCompletionPending) {
		t.Fatalf("goal-drive must wait for settlement: %v", err)
	}
	// G: settlement cannot turn UNKNOWN into satisfaction.
	digest, _ := latest.Digest()
	owner := contracts.PrincipalRef{ID: "owner", Kind: "human"}
	complete := GoalCompletionDecision{GoalID: baseline.ID, GoalVersion: baseline.Version, GoalDigest: baseline.Digest, CandidateTurnID: record.TurnID, FinalHead: record.EndHead, EvaluationDigest: digest, Status: GoalComplete, DecidedBy: owner, DecidedAt: time.Now().UTC()}
	if err := ledger.RecordGoalCompletionDecision(ctx, complete); err == nil || !strings.Contains(err.Error(), "outcome is unknown") {
		t.Fatalf("COMPLETE must be refused on an unknown evaluation: %v", err)
	}
	// An evaluator resolves the unknown criteria with evidence and judgment.
	findings := []Finding{}
	for i := 1; i <= 3; i++ {
		findings = append(findings, Finding{Ref: "success_criteria/" + string(rune('0'+i)), Result: ResultSatisfied, Evidence: []string{"reviewed checkpoint " + record.EndHead}, Judgment: "criterion holds in the integrated result"})
	}
	composed, err := ComposeEvaluation(*latest, contracts.PrincipalRef{ID: "reviewer", Kind: "human"}, EvaluatorHuman, findings)
	if err != nil || composed.Outcome != ResultSatisfied || composed.BasedOn != digest {
		t.Fatalf("composition must resolve the unknowns into a satisfied evaluation based on the verifier's: %+v %v", composed, err)
	}
	// F: a stale binding is refused.
	stale := composed
	stale.FinalHead = "0000000"
	if _, err := ledger.RecordGoalEvaluation(ctx, stale); err == nil {
		t.Fatal("an evaluation bound to another checkpoint must be refused")
	}
	composedDigest, err := ledger.RecordGoalEvaluation(ctx, composed)
	if err != nil {
		t.Fatal(err)
	}
	// Settlement binds the exact latest evaluation.
	if err := ledger.RecordGoalCompletionDecision(ctx, complete); err == nil {
		t.Fatal("a decision bound to a superseded evaluation must be refused")
	}
	complete.EvaluationDigest = composedDigest
	if err := ledger.RecordGoalCompletionDecision(ctx, complete); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordGoalCompletionDecision(ctx, complete); err == nil {
		t.Fatal("exactly one settlement is admitted")
	}
	if _, err := drive(chainWorker{dir: workDir}); !errors.Is(err, ErrGoalSettled) {
		t.Fatalf("a settled generation refuses further turns: %v", err)
	}
}

// TestVerifierBindingsAndOverrides proves bound criteria are verified by the
// declared validator at the final head, unbound ones stay UNKNOWN, an
// integrated failure yields UNSATISFIED, and a judgment cannot override a
// deterministic UNSATISFIED.
func TestVerifierBindingsAndOverrides(t *testing.T) {
	ctx := context.Background()
	_, workDir := contractRepo(t)
	if err := os.MkdirAll(filepath.Join(workDir, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	// success_criteria/1 passes, success_criteria/2 fails, integrated passes.
	writeFile(t, filepath.Join(workDir, ".praxis", "validate"), "#!/bin/sh\ncase \"$1\" in success_criteria/1|integrated) exit 0;; success_criteria/2) echo 'criterion 2 not met'; exit 1;; *) exit 0;; esac\n")
	if err := os.Chmod(filepath.Join(workDir, ".praxis", "validate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, workDir, "add", ".praxis")
	runGitTest(t, workDir, "commit", "-m", "validator")
	runGitTest(t, workDir, "push", "origin", "main")
	head := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "HEAD"))
	baseline := chainBaseline()
	baseline.ValidityPredicates = []string{"verify success_criteria/1 with declared-validation", "verify success_criteria/2 with declared-validation", "the deployment stays keyless"}
	baseline.Constraints = []string{"no secrets"}
	candidate := GoalCompletionCandidate{GoalID: baseline.ID, GoalVersion: baseline.Version, GoalDigest: baseline.Digest, TurnID: "t:turn:1", InvocationID: "t", FinalHead: head, Assessment: GoalCompletionAssessment{AllUnitsComplete: true}}
	repo := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	evaluation, err := EvaluateDeterministically(ctx, baseline, candidate, repo, contracts.PrincipalRef{ID: "verifier", Kind: "controller"})
	if err != nil {
		t.Fatal(err)
	}
	results := map[string]EvaluationResult{}
	for _, item := range evaluation.Items {
		results[item.Ref] = item.Result
	}
	if results["success_criteria/1"] != ResultSatisfied || results["success_criteria/2"] != ResultUnsatisfied || results["success_criteria/3"] != ResultUnknown || results["constraint/1"] != ResultUnknown || results["validity_predicates/3"] != ResultUnknown || results[IntegratedValidator] != ResultSatisfied || evaluation.Outcome != ResultUnsatisfied {
		t.Fatalf("verifier results: %v outcome %s", results, evaluation.Outcome)
	}
	if _, err := ComposeEvaluation(evaluation, contracts.PrincipalRef{ID: "r", Kind: "human"}, EvaluatorHuman, []Finding{{Ref: "success_criteria/2", Result: ResultSatisfied, Evidence: []string{"x"}, Judgment: "looks fine"}}); err == nil || !strings.Contains(err.Error(), "cannot override") {
		t.Fatalf("a judgment must not override a deterministic UNSATISFIED: %v", err)
	}
	if _, err := ComposeEvaluation(evaluation, contracts.PrincipalRef{ID: "r", Kind: "human"}, EvaluatorHuman, []Finding{{Ref: "success_criteria/3", Result: ResultSatisfied, Judgment: "no evidence"}}); err == nil || !strings.Contains(err.Error(), "without evidence") {
		t.Fatalf("satisfaction without evidence must be refused: %v", err)
	}
	if _, err := ComposeEvaluation(evaluation, contracts.PrincipalRef{ID: "r", Kind: "human"}, EvaluatorHuman, []Finding{{Ref: "success_criteria/9", Result: ResultUnknown, Judgment: "?"}}); err == nil {
		t.Fatal("a finding must name a contract element")
	}
	// A stale checkout fails closed.
	writeFile(t, filepath.Join(workDir, "extra.txt"), "x\n")
	runGitTest(t, workDir, "add", "extra.txt")
	runGitTest(t, workDir, "commit", "-q", "-m", "moved on")
	if _, err := EvaluateDeterministically(ctx, baseline, candidate, repo, contracts.PrincipalRef{ID: "verifier", Kind: "controller"}); err == nil {
		t.Fatal("evaluation at a checkout that is not the final checkpoint must fail closed")
	}
	if DeriveOutcome(nil) != ResultUnknown || DeriveOutcome([]PredicateEvaluation{{Result: ResultSatisfied}}) != ResultSatisfied {
		t.Fatal("outcome derivation")
	}
}

// TestStructuralAssessmentIsCoverageOnly proves the assessment reports
// coverage and never satisfaction: an uncovered criterion is listed, and a
// fully covered plan is only structurally complete.
func TestStructuralAssessmentIsCoverageOnly(t *testing.T) {
	baseline := chainBaseline()
	baseline.SuccessCriteria = append(baseline.SuccessCriteria, "four")
	completions := []UnitCompletion{}
	for _, unit := range []string{"unit:a", "unit:b", "unit:c"} {
		completions = append(completions, UnitCompletion{GoalID: baseline.ID, GoalVersion: baseline.Version, UnitID: unit, InvocationID: "i", TurnID: "t", EndHead: "h", Evidence: []string{"x"}})
	}
	assessment, err := AssessGoalCompletion(baseline, completions)
	if err != nil || !assessment.AllUnitsComplete || assessment.StructurallyComplete || strings.Join(assessment.UncoveredCriteria, ",") != "success_criteria/4" {
		t.Fatalf("uncovered criterion must be listed: %+v %v", assessment, err)
	}
	baseline.SuccessCriteria = baseline.SuccessCriteria[:3]
	assessment, _ = AssessGoalCompletion(baseline, completions)
	if !assessment.StructurallyComplete || len(assessment.Coverage) != 3 || strings.Join(assessment.Coverage[2].Units, ",") != "unit:c" {
		t.Fatalf("full coverage is structural completeness with per-criterion units: %+v", assessment)
	}
	if _, err := AssessGoalCompletion(baseline, completions[:2]); err != nil {
		t.Fatal(err)
	}
	if got := ParseCompletionTrailers("unit:a\n\nunit:a\nunit:b\n"); strings.Join(got, ",") != "unit:a,unit:b" {
		t.Fatalf("trailer parsing: %v", got)
	}
}

// TestNoPushProposalDoesNotComplete proves a proposal on a checkpoint that
// was never published cannot complete the unit.
func TestNoPushProposalDoesNotComplete(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	db, err := state.OpenSQLite(ctx, filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := state.NewSQLiteEventStore(db)
	baseline := chainBaseline()
	controller := Controller{Ledger: Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: chainWorker{dir: workDir, trailer: "unit:a"}, NoProgressLimit: 1}
	req := TurnRequest{GoalID: baseline.ID, GoalVersion: baseline.Version, InvocationID: "np", TurnID: "np:turn:1", GraphID: "g", GraphVersion: "1", Mode: ModeSupervised, ProviderID: "local", GoalBaseline: &baseline, NoPush: true}
	record, err := controller.ExecuteTurnWithRepository(ctx, req, GitRepository{Dir: workDir, Remote: "origin", Branch: "main"})
	if err != nil || !record.Progress || record.CheckpointPublished || record.UnitCompleted || record.CompletionClaim != "unit:a" {
		t.Fatalf("an unpublished checkpoint records the claim but no completion: %+v %v", record, err)
	}
	if completions, _ := controller.Ledger.LoadCompletions(ctx, baseline.ID, baseline.Version); len(completions) != 0 {
		t.Fatalf("no completion may be recorded without publication: %+v", completions)
	}
}

// TestCompletionRecordsAreUniquePerGeneration proves that two Goal
// generations with identical turn, unit, and step names record their unit
// completions, candidates, and evaluations independently: command ids are
// store-wide idempotency keys and must carry the generation identity.
func TestCompletionRecordsAreUniquePerGeneration(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ledger := Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	for _, goal := range []string{"goal:one", "goal:two"} {
		completion := UnitCompletion{GoalID: goal, GoalVersion: "2", UnitID: "unit:a", InvocationID: "q-1", TurnID: "q-1:turn:1", EndHead: "h", Evidence: []string{"x"}, CompletedAt: time.Now().UTC()}
		if err := ledger.RecordCompletion(ctx, completion); err != nil {
			t.Fatalf("%s unit completion: %v", goal, err)
		}
		candidate := GoalCompletionCandidate{GoalID: goal, GoalVersion: "2", GoalDigest: "sha256:" + strings.Repeat("a", 64), InvocationID: "q-1", TurnID: "q-1:turn:1", FinalHead: "h", Assessment: GoalCompletionAssessment{AllUnitsComplete: true}, CandidateAt: time.Now().UTC()}
		if err := ledger.RecordGoalCompletionCandidate(ctx, candidate); err != nil {
			t.Fatalf("%s candidate: %v", goal, err)
		}
		evaluation := GoalCompletionEvaluation{GoalID: goal, GoalVersion: "2", GoalDigest: candidate.GoalDigest, CandidateTurnID: "q-1:turn:1", FinalHead: "h", Evaluator: ledger.Actor, EvaluatorKind: EvaluatorDeterministic, Items: []PredicateEvaluation{{Kind: ItemIntegratedValidator, Ref: IntegratedValidator, Result: ResultUnknown, Predicate: "none"}}, EvaluatedAt: time.Now().UTC()}
		if _, err := ledger.RecordGoalEvaluation(ctx, evaluation); err != nil {
			t.Fatalf("%s evaluation: %v", goal, err)
		}
		record := TurnRecord{GoalID: goal, GoalVersion: "2", InvocationID: "q-1", Mode: ModeSupervised, TurnID: "q-1:turn:1", ChildObjective: "unit:a", GraphID: "g", GraphVersion: "1", Outcome: OutcomeContinue, Progress: true}
		if err := (Controller{Ledger: ledger}).recordTurn(ctx, &record); err != nil {
			t.Fatalf("%s turn record with a turn id another generation also uses: %v", goal, err)
		}
	}
}
