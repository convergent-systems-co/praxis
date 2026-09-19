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
	// 7, 14, 15: C unlocks after B; completing C completes the Goal only
	// because every success criterion is covered.
	record, err = drive(chainWorker{dir: workDir, trailer: "unit:c"})
	if err != nil || record.ChildObjective != "unit:c" || !record.UnitCompleted || record.Outcome != OutcomeUserDecisionRequired {
		t.Fatalf("completing C makes Goal completion provisional and stops for the owner's evaluation: %+v %v", record, err)
	}
	ledger := Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	goalState, err := ledger.LoadGoalCompletion(ctx, baseline.ID, baseline.Version)
	if err != nil || goalState.Claim == nil || goalState.Decision != nil || goalState.Claim.FinalHead != record.EndHead || goalState.Claim.TurnID != record.TurnID {
		t.Fatalf("the provisional claim must be durable and undecided: %v %+v", err, goalState)
	}
	if _, err := drive(chainWorker{dir: workDir}); !errors.Is(err, ErrGoalCompletionPending) {
		t.Fatalf("goal-drive must wait for the owner's evaluation: %v", err)
	}
	// The owner re-evaluates the original contract; only then is the Goal complete.
	bad := GoalCompletionDecision{GoalID: baseline.ID, GoalVersion: baseline.Version, GoalDigest: baseline.Digest, ClaimTurnID: "other", FinalHead: record.EndHead, Status: GoalComplete, DecidedBy: contracts.PrincipalRef{ID: "owner", Kind: "human"}, DecidedAt: time.Now().UTC()}
	if err := ledger.RecordGoalCompletionDecision(ctx, bad); err == nil {
		t.Fatal("a decision must bind the exact claim")
	}
	good := bad
	good.ClaimTurnID = record.TurnID
	if err := ledger.RecordGoalCompletionDecision(ctx, good); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordGoalCompletionDecision(ctx, good); err == nil {
		t.Fatal("exactly one decision is admitted")
	}
	if _, err := drive(chainWorker{dir: workDir}); !errors.Is(err, ErrGoalComplete) {
		t.Fatalf("a complete Goal refuses further turns: %v", err)
	}
}

// TestGoalCompletionRequiresCriteriaCoverage proves all units complete is not
// enough: an uncovered success criterion keeps the Goal incomplete.
func TestGoalCompletionRequiresCriteriaCoverage(t *testing.T) {
	baseline := chainBaseline()
	baseline.SuccessCriteria = append(baseline.SuccessCriteria, "four")
	completions := []UnitCompletion{}
	for _, unit := range []string{"unit:a", "unit:b", "unit:c"} {
		completions = append(completions, UnitCompletion{GoalID: baseline.ID, GoalVersion: baseline.Version, UnitID: unit, InvocationID: "i", TurnID: "t", EndHead: "h", Evidence: []string{"x"}})
	}
	assessment, err := AssessGoalCompletion(baseline, completions)
	if err != nil || !assessment.AllUnitsComplete || assessment.Complete || strings.Join(assessment.UncoveredCriteria, ",") != "success_criteria/4" {
		t.Fatalf("uncovered criterion must keep the Goal incomplete: %+v %v", assessment, err)
	}
	baseline.SuccessCriteria = baseline.SuccessCriteria[:3]
	assessment, _ = AssessGoalCompletion(baseline, completions)
	if !assessment.Complete {
		t.Fatalf("all units complete and all criteria covered must complete the Goal: %+v", assessment)
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
