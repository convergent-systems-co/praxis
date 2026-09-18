package projection

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestRunProjectionRebuildsFromAuthoritativeEvents(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &kernel.EventJournal{
		Store:         store,
		Actor:         contracts.PrincipalRef{ID: "agent-1", Kind: "agent"},
		CommandID:     "cmd-1",
		CorrelationID: "corr-1",
	}
	graph := kernel.GraphDef{
		ID: "g", Version: "1", EntryNode: "work", MaxTransitions: 2,
		Nodes: []kernel.NodeDef{
			{ID: "work", Class: kernel.NodeDeterministic},
			{ID: "done", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded},
		},
		Transitions: []kernel.TransitionDef{{From: "work", Outcome: "done", To: "done"}},
	}
	run := &kernel.RunExecution{RunID: "run-1"}
	executor := projectionExecutor{outcomes: map[string]string{"work": "done"}}
	if err := kernel.RunObserved(context.Background(), graph, run, executor, journal); err != nil {
		t.Fatal(err)
	}

	views := NewRunViews()
	runner := Runner{
		Events:  store,
		Handler: views,
		Checkpoint: Checkpoint{
			Name:        "runs",
			Version:     "1",
			Consistency: StrongCheckpointed,
		},
		BatchSize: 2,
	}
	if err := runner.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	view, ok := views.Get("run-1")
	if !ok {
		t.Fatal("expected run view")
	}
	if view.State != kernel.RunSucceeded || view.CurrentNode != "done" || view.TransitionCount != 1 || view.EvidenceCount != 1 {
		t.Fatalf("unexpected run view: %+v", view)
	}
	if view.LastSequence != 4 || view.LastVersion != 4 || runner.Checkpoint.LastSequence != 4 {
		t.Fatalf("unexpected projection positions: view=%+v checkpoint=%+v", view, runner.Checkpoint)
	}
}

type projectionExecutor struct{ outcomes map[string]string }

func (p projectionExecutor) ExecuteNode(_ context.Context, _ kernel.GraphDef, node kernel.NodeDef, _ *kernel.RunExecution) (kernel.NodeResult, error) {
	return kernel.NodeResult{Outcome: p.outcomes[node.ID], Evidence: []string{"evidence:" + node.ID}}, nil
}
