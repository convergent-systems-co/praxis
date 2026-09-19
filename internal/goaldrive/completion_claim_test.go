package goaldrive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// liveClaimLayout is the exact paragraph structure of the live Weather II
// Turn 6 checkpoint (05b32563): the completion proposal stands alone in a
// paragraph above the Co-Authored-By trailer, so Git's own trailer block
// (the final paragraph) does not contain it (#164).
func liveClaimLayout(unit string) string {
	return "unit: prove and document clean-clone runnability\n\n" +
		"Add a clean-clone test that exports exactly the committed tree.\n\n" +
		"README: spell out clone -> cd -> npm start.\n\n" +
		CompletionTrailer + ": " + unit + "\n\n" +
		"Co-Authored-By: Worker <worker@example.invalid>\n"
}

func commitWithMessage(t *testing.T, root, workDir, file, message string) string {
	t.Helper()
	writeFile(t, filepath.Join(workDir, file), file+"\n")
	messagePath := filepath.Join(root, "message-"+file)
	if err := os.WriteFile(messagePath, []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, workDir, "add", file)
	runGitTest(t, workDir, "commit", "-q", "-F", messagePath)
	return strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "HEAD"))
}

// TestCompletionClaimIsRecognizedOutsideGitTrailerBlock is the #164 contract
// for proposal recognition: a line of the exact form
// `Praxis-Unit-Complete: <unit>` anywhere after the subject line of a commit
// in the turn's span is a proposal, whatever paragraph Git's trailer
// heuristic assigns it to. A mention inside prose and the subject line are
// not proposals.
func TestCompletionClaimIsRecognizedOutsideGitTrailerBlock(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	repo := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	base := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "HEAD"))

	live := commitWithMessage(t, root, workDir, "live.txt", liveClaimLayout("unit:runnable-from-repository"))
	units, err := repo.CompletionClaims(ctx, base, live)
	if err != nil || len(units) != 1 || units[0] != "unit:runnable-from-repository" {
		t.Fatalf("RED #164: the live Turn 6 layout must be recognised as a proposal for its unit: units=%v err=%v", units, err)
	}

	adjacent := commitWithMessage(t, root, workDir, "adjacent.txt", "subject\n\nbody\n\n"+CompletionTrailer+": unit:b\nCo-Authored-By: Worker <worker@example.invalid>\n")
	if units, err = repo.CompletionClaims(ctx, live, adjacent); err != nil || len(units) != 1 || units[0] != "unit:b" {
		t.Fatalf("the trailer-block layout must remain recognised: units=%v err=%v", units, err)
	}

	prose := commitWithMessage(t, root, workDir, "prose.txt", "subject\n\nDo not add "+CompletionTrailer+": unit:c yet; more turns are needed.\n")
	if units, err = repo.CompletionClaims(ctx, adjacent, prose); err != nil || len(units) != 0 {
		t.Fatalf("the key inside prose is not a proposal: units=%v err=%v", units, err)
	}

	malformed := commitWithMessage(t, root, workDir, "malformed.txt", "subject\n\n"+CompletionTrailer+": unit:c (not yet)\n")
	if _, err = repo.CompletionClaims(ctx, prose, malformed); !errors.Is(err, ErrAmbiguousCompletionClaim) {
		t.Fatalf("a key-led line that is not exactly `key: <unit>` is ambiguous and fails closed: %v", err)
	}
	runGitTest(t, workDir, "reset", "-q", "--hard", prose)

	subject := commitWithMessage(t, root, workDir, "subject.txt", CompletionTrailer+": unit:d\n")
	if units, err = repo.CompletionClaims(ctx, prose, subject); err != nil || len(units) != 0 {
		t.Fatalf("the subject line is a title, never a proposal: units=%v err=%v", units, err)
	}

	if units, err = repo.CompletionClaims(ctx, base, subject); err != nil || strings.Join(units, ",") != "unit:b,unit:runnable-from-repository" {
		t.Fatalf("the whole span is read in log order without duplicates: units=%v err=%v", units, err)
	}
}

// layoutWorker commits bounded work and proposes completion of the selected
// unit with the live Turn 6 message layout.
type layoutWorker struct {
	root, dir string
}

func (w layoutWorker) Execute(_ context.Context, request WorkerRequest) (WorkerResult, error) {
	name := strings.ReplaceAll(request.TurnID, ":", "-") + ".txt"
	if err := os.WriteFile(filepath.Join(w.dir, name), []byte("ok\n"), 0o644); err != nil {
		return WorkerResult{}, err
	}
	messagePath := filepath.Join(w.root, "message-"+name)
	if err := os.WriteFile(messagePath, []byte(liveClaimLayout(request.ChildObjective)), 0o644); err != nil {
		return WorkerResult{}, err
	}
	git := GitRepository{Dir: w.dir, Remote: "origin", Branch: "main"}
	if _, err := git.run(context.Background(), "add", "-A"); err != nil {
		return WorkerResult{}, err
	}
	if _, err := git.run(context.Background(), "commit", "-q", "-F", messagePath); err != nil {
		return WorkerResult{}, err
	}
	head, err := git.run(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		return WorkerResult{}, err
	}
	return WorkerResult{Outcome: OutcomeContinue, EndHead: strings.TrimSpace(head), CheckpointValid: true}, nil
}

func (w layoutWorker) RepositoryResultIsControllerOwned() bool { return true }

func claimFixture(t *testing.T) (string, string, *state.SQLiteEventStore) {
	t.Helper()
	root, workDir := contractRepo(t)
	if err := os.MkdirAll(filepath.Join(workDir, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workDir, ".praxis", "validate"), "#!/bin/sh\ntrue\n")
	if err := os.Chmod(filepath.Join(workDir, ".praxis", "validate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, workDir, "add", ".praxis")
	runGitTest(t, workDir, "commit", "-q", "-m", "declare validation")
	runGitTest(t, workDir, "push", "-q", "origin", "main")
	db, err := state.OpenSQLite(context.Background(), filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return root, workDir, state.NewSQLiteEventStore(db)
}

// TestQualifiedCheckpointWithParagraphClaimCompletesTheUnit models the live
// Turn 6: the selected unit is incomplete, the worker's new commit carries
// the exact claim for it, the checkpoint qualifies (clean, HEAD moved,
// declared validation passed) and is published. Expected: completion.claimed,
// durable UnitCompletion, dependent eligibility advances.
func TestQualifiedCheckpointWithParagraphClaimCompletesTheUnit(t *testing.T) {
	ctx := context.Background()
	root, workDir, store := claimFixture(t)
	baseline := chainBaseline()
	actor := contracts.PrincipalRef{ID: "controller", Kind: "controller"}
	activity := &ActivityLog{Store: store, Actor: actor}
	controller := Controller{Ledger: Ledger{Store: store, Actor: actor}, Worker: layoutWorker{root: root, dir: workDir}, NoProgressLimit: 2, Activity: activity}
	repo := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	req := TurnRequest{GoalID: baseline.ID, GoalVersion: baseline.Version, InvocationID: "layout-1", TurnID: "layout-1:turn:1", GraphID: "g", GraphVersion: "1", Mode: ModeContinuous, ProviderID: "local", GoalBaseline: &baseline}
	record, err := controller.ExecuteTurnWithRepository(ctx, req, repo)
	if err != nil || record.ChildObjective != "unit:a" || record.Outcome != OutcomeContinue || !record.Progress || !record.CheckpointPublished || !containsString(record.CheckpointEvidence, "repository:declared-validation-passed") {
		t.Fatalf("the checkpoint must qualify and publish exactly as the live turn did: %+v %v", record, err)
	}
	if record.CompletionClaim != "unit:a" || !record.UnitCompleted {
		t.Fatalf("RED #164: a qualified, published checkpoint carrying the claim above the trailer block must complete the unit: claim=%q completed=%v", record.CompletionClaim, record.UnitCompleted)
	}
	types := strings.Join(activityTypesFor(t, activity, "layout-1", "layout-1:turn:1"), ",")
	if !strings.Contains(types, "completion.claimed") || !strings.Contains(types, "unit.completed") {
		t.Fatalf("completion must be a durable stage of the turn: %s", types)
	}
	completions, err := (Ledger{Store: store}).LoadCompletions(ctx, baseline.ID, baseline.Version)
	if err != nil || len(completions) != 1 || completions[0].UnitID != "unit:a" || completions[0].EndHead != record.EndHead || completions[0].TurnID != record.TurnID {
		t.Fatalf("UnitCompletion must be durable and bound to the checkpoint: %v %+v", err, completions)
	}
	candidates, relationships, _ := MaterializeGoalWork(baseline)
	if selected, err := contracts.SelectRunnableWork(ApplyCompletions(candidates, completions), relationships); err != nil || selected.ID != "unit:b" {
		t.Fatalf("the dependent unit must become eligible: %+v %v", selected, err)
	}
}

// TestContinuousModeDoesNotReselectAUnitWhoseCheckpointEarnedCompletion is
// the continuous consequence of #164: the turn after a checkpoint that
// proposed and earned completion selects the dependent unit, never the
// same one again.
func TestContinuousModeDoesNotReselectAUnitWhoseCheckpointEarnedCompletion(t *testing.T) {
	ctx := context.Background()
	root, workDir, store := claimFixture(t)
	baseline := chainBaseline()
	actor := contracts.PrincipalRef{ID: "controller", Kind: "controller"}
	activity := &ActivityLog{Store: store, Actor: actor}
	runtime := Runtime{Controller: Controller{Ledger: Ledger{Store: store, Actor: actor}, Worker: layoutWorker{root: root, dir: workDir}, NoProgressLimit: 2, Activity: activity}, Baselines: supervisionBaselineStore{baseline: baseline}, Repository: GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}, GraphID: "g", GraphVersion: "1", Activity: activity}
	_, err := runtime.Execute(ctx, InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: baseline.ID}, GoalVersion: baseline.Version, Mode: ModeContinuous, InvocationID: "layout-c", ProviderID: "local", MaxTurns: 2})
	if !errors.Is(err, ErrContinuousTurnLimit) {
		t.Fatalf("two turns of a three-unit chain end at the turn limit: %v", err)
	}
	turns, err := (Ledger{Store: store}).Load(ctx, baseline.ID, baseline.Version)
	if err != nil || len(turns) != 2 {
		t.Fatalf("two durable turns: %v %d", err, len(turns))
	}
	if turns[0].ChildObjective != "unit:a" || !turns[0].UnitCompleted || turns[1].ChildObjective != "unit:b" {
		t.Fatalf("RED #164: the next continuous turn must not reselect the unit whose checkpoint earned completion: turn1=%s completed=%v turn2=%s", turns[0].ChildObjective, turns[0].UnitCompleted, turns[1].ChildObjective)
	}
}
