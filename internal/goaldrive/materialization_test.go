package goaldrive

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// legacyRecognizer reproduces the exact pre-#164 controller: the checkpoint
// is inspected, validated and published by the real adapter, but proposals
// are read through Git's trailer block only, so a claim above the
// Co-Authored-By trailer is invisible. It manufactures the historical
// Weather II turn 6 shape: qualified, published, no UnitCompletion.
type legacyRecognizer struct{ GitRepository }

func (r legacyRecognizer) CompletionClaims(ctx context.Context, startHead, endHead string) ([]string, error) {
	span := endHead
	if startHead != "" {
		span = startHead + ".." + endHead
	}
	output, err := r.run(ctx, "log", "--format=%(trailers:key="+CompletionTrailer+",valueonly)", span)
	if err != nil {
		return nil, err
	}
	var units []string
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			units = append(units, line)
		}
	}
	return units, nil
}

type materializationFixture struct {
	root, workDir string
	store         *state.SQLiteEventStore
	baseline      goals.GoalBaseline
	controller    Controller
	activity      *ActivityLog
	repo          GitRepository
	turn          int
}

func newMaterializationFixture(t *testing.T, baseline goals.GoalBaseline) *materializationFixture {
	t.Helper()
	root, workDir, store := claimFixture(t)
	actor := contracts.PrincipalRef{ID: "controller", Kind: "controller"}
	activity := &ActivityLog{Store: store, Actor: actor}
	f := &materializationFixture{root: root, workDir: workDir, store: store, baseline: baseline, activity: activity, repo: GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}}
	f.controller = Controller{Ledger: Ledger{Store: store, Actor: actor}, Worker: layoutWorker{root: root, dir: workDir}, NoProgressLimit: 2, Activity: activity}
	return f
}

func (f *materializationFixture) request() TurnRequest {
	f.turn++
	id := "hist-" + string(rune('0'+f.turn))
	return TurnRequest{GoalID: f.baseline.ID, GoalVersion: f.baseline.Version, InvocationID: id, TurnID: id + ":turn:" + string(rune('0'+f.turn)), GraphID: "g", GraphVersion: "1", Mode: ModeContinuous, ProviderID: "local", GoalBaseline: &f.baseline}
}

// historicalTurn runs one turn through the legacy recognizer with the given
// worker and asserts the Weather II turn 6 shape.
func (f *materializationFixture) historicalTurn(t *testing.T, worker Worker) TurnRecord {
	t.Helper()
	f.controller.Worker = worker
	record, err := f.controller.ExecuteTurnWithRepository(context.Background(), f.request(), legacyRecognizer{f.repo})
	if err != nil || record.Outcome != OutcomeContinue || !record.Progress || !record.CheckpointPublished || record.CompletionClaim != "" || record.UnitCompleted {
		t.Fatalf("the legacy recognizer must leave a qualified, published checkpoint without completion: %+v %v", record, err)
	}
	return record
}

func (f *materializationFixture) materialize(turnID string) (Materialization, error) {
	return f.controller.MaterializeTurnCompletion(context.Background(), f.baseline.ID, f.baseline.Version, turnID, f.baseline, f.repo, contracts.PrincipalRef{ID: "operator", Kind: "human"})
}

func (f *materializationFixture) turnEvents(t *testing.T) []byte {
	t.Helper()
	events, err := f.store.LoadAggregate(context.Background(), "goal-drive:"+f.baseline.ID+":"+f.baseline.Version, 0)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(events)
	return encoded
}

// TestMaterializationDerivesMissingCompletionFromExactConsequence covers
// R1 (exact qualified historical consequence materializes), R6 (replay is
// refused without a duplicate), R7 (eligibility advances exactly as if the
// completion had materialized originally) and R8 (the historical turn and
// its events are immutable; provenance is honest about when it happened).
func TestMaterializationDerivesMissingCompletionFromExactConsequence(t *testing.T) {
	ctx := context.Background()
	f := newMaterializationFixture(t, chainBaseline())
	record := f.historicalTurn(t, layoutWorker{root: f.root, dir: f.workDir})
	before := f.turnEvents(t)
	activitiesBefore := len(activityTypesFor(t, f.activity, record.InvocationID, record.TurnID))
	started := time.Now().UTC().Add(-time.Second)

	result, err := f.materialize(record.TurnID)
	if err != nil {
		t.Fatalf("R1: the missing completion must be derivable from the exact qualified consequence: %v", err)
	}
	c := result.Completion
	if c.UnitID != "unit:a" || c.TurnID != record.TurnID || c.EndHead != record.EndHead || c.InvocationID != record.InvocationID || c.Materialization == nil || c.Materialization.By.ID != "operator" || c.CompletedAt.Before(started) || !containsString(c.Evidence, "checkpoint:"+record.EndHead) || !containsString(c.Evidence, "repository:declared-validation-passed") {
		t.Fatalf("the completion must bind the historical turn and checkpoint with materialization provenance and an honest instant: %+v", c)
	}
	if result.GoalCandidate {
		t.Fatalf("two units remain; no Goal candidate may be derived: %+v", result)
	}
	completions, err := (Ledger{Store: f.store}).LoadCompletions(ctx, f.baseline.ID, f.baseline.Version)
	if err != nil || len(completions) != 1 || completions[0].UnitID != "unit:a" {
		t.Fatalf("exactly one durable completion: %v %+v", err, completions)
	}
	// R8: the historical turn record and its aggregate are untouched.
	if after := f.turnEvents(t); string(after) != string(before) {
		t.Fatalf("R8: materialization must never rewrite the historical turn aggregate")
	}
	turns, _ := (Ledger{Store: f.store}).Load(ctx, f.baseline.ID, f.baseline.Version)
	if len(turns) != 1 || turns[0].UnitCompleted || turns[0].CompletionClaim != "" {
		t.Fatalf("R8: the recorded turn keeps its historical shape: %+v", turns)
	}
	types := activityTypesFor(t, f.activity, record.InvocationID, record.TurnID)
	if len(types) <= activitiesBefore || !containsString(types, "unit.completed") || types[activitiesBefore-1] != "execution.state_changed" {
		t.Fatalf("materialization appends after the historical terminal disposition, never before it: %v", types)
	}
	// R6: replay is refused and nothing is duplicated.
	if _, err := f.materialize(record.TurnID); !errors.Is(err, ErrAlreadyMaterialized) {
		t.Fatalf("R6: replay must be refused as already materialized: %v", err)
	}
	if completions, _ = (Ledger{Store: f.store}).LoadCompletions(ctx, f.baseline.ID, f.baseline.Version); len(completions) != 1 {
		t.Fatalf("R6: no duplicate completion: %+v", completions)
	}
	// R7: the next turn selects the dependent unit, exactly as if the
	// completion had materialized in the historical turn.
	f.controller.Worker = layoutWorker{root: f.root, dir: f.workDir}
	next, err := f.controller.ExecuteTurnWithRepository(ctx, f.request(), f.repo)
	if err != nil || next.ChildObjective != "unit:b" || !next.UnitCompleted {
		t.Fatalf("R7: eligibility must advance to the dependent unit: %+v %v", next, err)
	}
}

// TestMaterializationRefusesChangedConsequence is R2: the exact checkpoint
// must still be contained in the published branch and the span must still
// be the turn's consequence; otherwise materialization refuses.
func TestMaterializationRefusesChangedConsequence(t *testing.T) {
	f := newMaterializationFixture(t, chainBaseline())
	record := f.historicalTurn(t, layoutWorker{root: f.root, dir: f.workDir})
	// The published branch is rewritten past the checkpoint (a force-push
	// of an amended commit): the exact consequence no longer exists there.
	runGitTest(t, f.workDir, "commit", "-q", "--amend", "--no-edit", "-m", "amended: no proposal")
	runGitTest(t, f.workDir, "push", "-q", "--force", "origin", "main")
	if _, err := f.materialize(record.TurnID); !errors.Is(err, ErrConsequenceChanged) {
		t.Fatalf("R2: a checkpoint no longer on the published branch must be refused: %v", err)
	}
	if completions, _ := (Ledger{Store: f.store}).LoadCompletions(context.Background(), f.baseline.ID, f.baseline.Version); len(completions) != 0 {
		t.Fatalf("R2: nothing may be materialized from a changed consequence: %+v", completions)
	}
}

// TestMaterializationRefusesWrongIdentityAndUnqualifiedTurns is R3 and R4.
func TestMaterializationRefusesWrongIdentityAndUnqualifiedTurns(t *testing.T) {
	ctx := context.Background()
	f := newMaterializationFixture(t, chainBaseline())
	// R4: an unpublished (--no-push) turn is not qualified.
	f.controller.Worker = layoutWorker{root: f.root, dir: f.workDir}
	req := f.request()
	req.NoPush = true
	unpublished, err := f.controller.ExecuteTurnWithRepository(ctx, req, legacyRecognizer{f.repo})
	if err != nil || unpublished.CheckpointPublished {
		t.Fatalf("fixture: an unpublished turn: %+v %v", unpublished, err)
	}
	if _, err := f.materialize(unpublished.TurnID); !errors.Is(err, ErrTurnNotQualified) {
		t.Fatalf("R4: an unpublished turn must be refused: %v", err)
	}
	// Publish it so the checkout is authoritative again, then a qualified
	// historical turn whose claim names the wrong unit.
	runGitTest(t, f.workDir, "push", "-q", "origin", "main")
	wrong := f.historicalTurn(t, wrongUnitWorker{layoutWorker{root: f.root, dir: f.workDir}, "unit:b"})
	if _, err := f.materialize(wrong.TurnID); err == nil || errors.Is(err, ErrAlreadyMaterialized) || !strings.Contains(err.Error(), "unit:b") {
		t.Fatalf("R3: a proposal naming a unit other than the selected one must be refused by name: %v", err)
	}
	// R3: unknown turn, wrong generation.
	if _, err := f.materialize("hist-9:turn:9"); err == nil || errors.Is(err, ErrAlreadyMaterialized) {
		t.Fatalf("R3: an unknown turn must be refused: %v", err)
	}
	if _, err := f.controller.MaterializeTurnCompletion(ctx, f.baseline.ID, "7", wrong.TurnID, f.baseline, f.repo, contracts.PrincipalRef{ID: "operator", Kind: "human"}); err == nil {
		t.Fatal("R3: a Goal generation that never recorded the turn must be refused")
	}
	if completions, _ := (Ledger{Store: f.store}).LoadCompletions(ctx, f.baseline.ID, f.baseline.Version); len(completions) != 0 {
		t.Fatalf("no completion may exist after refusals: %+v", completions)
	}
}

type wrongUnitWorker struct {
	layoutWorker
	claim string
}

func (w wrongUnitWorker) Execute(ctx context.Context, request WorkerRequest) (WorkerResult, error) {
	request.ChildObjective = w.claim
	return w.layoutWorker.Execute(ctx, request)
}

type rawMessageWorker struct {
	layoutWorker
	messages []string // one commit per message
}

func (w rawMessageWorker) Execute(_ context.Context, request WorkerRequest) (WorkerResult, error) {
	git := GitRepository{Dir: w.dir, Remote: "origin", Branch: "main"}
	for i, message := range w.messages {
		name := strings.ReplaceAll(request.TurnID, ":", "-") + "-" + string(rune('a'+i)) + ".txt"
		if _, err := git.run(context.Background(), "commit", "-q", "--allow-empty", "-m", message); err != nil {
			return WorkerResult{}, err
		}
		_ = name
	}
	head, err := git.run(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		return WorkerResult{}, err
	}
	return WorkerResult{Outcome: OutcomeContinue, EndHead: strings.TrimSpace(head), CheckpointValid: true}, nil
}

// TestMaterializationRefusesAmbiguousOrConflictingClaims is R5.
func TestMaterializationRefusesAmbiguousOrConflictingClaims(t *testing.T) {
	ctx := context.Background()
	f := newMaterializationFixture(t, chainBaseline())
	// Each message uses the live layout (claim above the Co-Authored-By
	// trailer) so the legacy recognizer sees nothing and the turn qualifies.
	trailer := "\n\nCo-Authored-By: Worker <worker@example.invalid>\n"
	ambiguous := f.historicalTurn(t, rawMessageWorker{layoutWorker{root: f.root, dir: f.workDir}, []string{"work\n\n" + CompletionTrailer + ": unit:a (done, I think)" + trailer}})
	if _, err := f.materialize(ambiguous.TurnID); !errors.Is(err, ErrAmbiguousCompletionClaim) {
		t.Fatalf("R5: an ambiguous claim line must be refused: %v", err)
	}
	conflicting := f.historicalTurn(t, rawMessageWorker{layoutWorker{root: f.root, dir: f.workDir}, []string{"part one\n\n" + CompletionTrailer + ": unit:a" + trailer, "part two\n\n" + CompletionTrailer + ": unit:b" + trailer}})
	if _, err := f.materialize(conflicting.TurnID); err == nil || errors.Is(err, ErrAlreadyMaterialized) {
		t.Fatalf("R5: conflicting units across the span must be refused: %v", err)
	}
	none := f.historicalTurn(t, rawMessageWorker{layoutWorker{root: f.root, dir: f.workDir}, []string{"progress only\n"}})
	if _, err := f.materialize(none.TurnID); !errors.Is(err, ErrNoCompletionProposal) {
		t.Fatalf("a span without a proposal has nothing to materialize: %v", err)
	}
	if completions, _ := (Ledger{Store: f.store}).LoadCompletions(ctx, f.baseline.ID, f.baseline.Version); len(completions) != 0 {
		t.Fatalf("no completion may exist after refusals: %+v", completions)
	}
}

// TestMaterializationDerivesTheGoalCandidateTheTurnWouldHave: when the
// materialized completion completes the generation, the candidate and the
// deterministic evaluation are derived exactly as the original turn would
// have derived them, bound to the historical turn and checkpoint, and the
// next invocation waits for settlement.
func TestMaterializationDerivesTheGoalCandidateTheTurnWouldHave(t *testing.T) {
	ctx := context.Background()
	baseline := chainBaseline()
	baseline.WorkPlan.Candidates = baseline.WorkPlan.Candidates[:1]
	baseline.WorkPlan.Relationships = nil
	baseline.SuccessCriteria = baseline.SuccessCriteria[:1]
	f := newMaterializationFixture(t, baseline)
	record := f.historicalTurn(t, layoutWorker{root: f.root, dir: f.workDir})
	result, err := f.materialize(record.TurnID)
	if err != nil || !result.GoalCandidate || result.GoalEvaluation != string(ResultUnknown) {
		t.Fatalf("completing the last unit must derive the candidate and its deterministic evaluation: %+v %v", result, err)
	}
	goalState, err := (Ledger{Store: f.store, Actor: f.controller.Ledger.Actor}).LoadGoalCompletion(ctx, baseline.ID, baseline.Version)
	if err != nil || goalState.Candidate == nil || goalState.Candidate.TurnID != record.TurnID || goalState.Candidate.FinalHead != record.EndHead || len(goalState.Evaluations) != 1 || goalState.Decision != nil {
		t.Fatalf("candidate and evaluation must bind the historical turn and checkpoint: %v %+v", err, goalState)
	}
	f.controller.Worker = layoutWorker{root: f.root, dir: f.workDir}
	if _, err := f.controller.ExecuteTurnWithRepository(ctx, f.request(), f.repo); !errors.Is(err, ErrGoalCompletionPending) {
		t.Fatalf("the next invocation must wait for settlement: %v", err)
	}
}

// TestMaterializationIsRaceSafe: concurrent materializations of one
// consequence yield exactly one completion.
func TestMaterializationIsRaceSafe(t *testing.T) {
	f := newMaterializationFixture(t, chainBaseline())
	record := f.historicalTurn(t, layoutWorker{root: f.root, dir: f.workDir})
	const n = 6
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = f.materialize(record.TurnID)
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	completions, err := (Ledger{Store: f.store}).LoadCompletions(context.Background(), f.baseline.ID, f.baseline.Version)
	if err != nil || len(completions) != 1 || successes != 1 {
		t.Fatalf("exactly one materialization may win: successes=%d completions=%d err=%v results=%v", successes, len(completions), err, results)
	}
}
