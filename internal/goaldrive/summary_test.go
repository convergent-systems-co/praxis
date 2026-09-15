package goaldrive

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestSummarizeTurnDistinguishesProgressPublicationAndGoalAdvancement(t *testing.T) {
	progressed := TurnRecord{Outcome: OutcomeComplete, Progress: true, StartHead: "a", EndHead: "b"}
	progressed.CheckpointPublished = true
	published := SummarizeTurn(progressed)
	if published.Checkpoint != CheckpointPublished || published.ParentGoalAdvancement != "not_claimed" {
		t.Fatalf("published summary overclaims or loses checkpoint state: %+v", published)
	}
	if got := published.HumanString(); got != "outcome=COMPLETE progress=true checkpoint=published parent_goal_advancement=not_claimed start=a end=b" {
		t.Fatalf("unexpected published summary: %s", got)
	}

	noProgress := TurnRecord{Outcome: OutcomeNoProgress, StartHead: "a", EndHead: "a", CheckpointEvidence: []string{"verification:passed"}}
	summary := SummarizeTurn(noProgress)
	if summary.Checkpoint != CheckpointNotCreated || summary.Progress || summary.ParentGoalAdvancement != "not_claimed" {
		t.Fatalf("NO_PROGRESS summary must not claim checkpoint or Goal advancement: %+v", summary)
	}
	if got := summary.HumanString(); got != "outcome=NO_PROGRESS progress=false checkpoint=not_created parent_goal_advancement=not_claimed start=a end=a" {
		t.Fatalf("unexpected no-progress summary: %s", got)
	}
}

func TestSummarizeTurnReportsValidatedLocalCheckpointWithoutPublication(t *testing.T) {
	record := TurnRecord{Outcome: OutcomeContinue, Progress: true, StartHead: "a", EndHead: "b"}
	summary := SummarizeTurn(record)
	if summary.Checkpoint != CheckpointValidatedUnpublished {
		t.Fatalf("validated local progress without publication was misreported: %+v", summary)
	}
}

func TestRepositoryInvocationSummaryDoesNotClaimUnchangedCheckpoint(t *testing.T) {
	controller := controllerFixture(&fakeWorker{result: WorkerResult{Outcome: OutcomeContinue, EndHead: "a", CheckpointValid: true}})
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}}}
	record, err := controller.ExecuteTurnWithRepository(context.Background(), turnRequest(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if record.CheckpointPublished {
		t.Fatal("unchanged invocation must not persist publication authority")
	}
	summary := SummarizeTurn(record)
	if summary.Outcome != OutcomeNoProgress || summary.Progress || summary.Checkpoint != CheckpointNotCreated || summary.ParentGoalAdvancement != "not_claimed" {
		t.Fatalf("unchanged repository invocation was overclaimed: record=%+v summary=%+v", record, summary)
	}
}

func TestLedgerRejectsPublicationWithoutProgress(t *testing.T) {
	record := fixtureTurn()
	record.Outcome = OutcomeNoProgress
	record.Progress = false
	record.CheckpointPublished = true
	ledger := Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	if _, err := ledger.Append(context.Background(), 0, record); err == nil {
		t.Fatal("publication without validated progress must fail closed")
	}
}
