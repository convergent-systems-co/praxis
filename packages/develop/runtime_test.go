package develop

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

type scriptedExecutor struct {
	seen []string
}

func (s *scriptedExecutor) ExecuteNode(_ context.Context, graph kernel.GraphDef, node kernel.NodeDef, _ *kernel.RunExecution) (kernel.NodeResult, error) {
	s.seen = append(s.seen, graph.ID+":"+node.ID)
	if graph.ID == "praxis.package.develop.default" {
		switch node.ID {
		case "discover":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "classify":
			return kernel.NodeResult{Outcome: "goals"}, nil
		case "materialize":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "plan":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "prepare":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "implement":
			return kernel.NodeResult{Outcome: "done"}, nil
		case "validate":
			return kernel.NodeResult{Outcome: "pass"}, nil
		case "review":
			return kernel.NodeResult{Outcome: "pass"}, nil
		case "integrate":
			return kernel.NodeResult{Outcome: "done"}, nil
		}
	}
	if graph.ID == "praxis.package.goals.default" {
		switch node.ID {
		case "intent":
			return kernel.NodeResult{Outcome: "captured"}, nil
		case "frame":
			return kernel.NodeResult{Outcome: "rigorous"}, nil
		case "discover":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "variance":
			return kernel.NodeResult{Outcome: "decide"}, nil
		case "decide":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "model", "architecture_review":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "specify":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "plan":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "baseline":
			return kernel.NodeResult{Outcome: "stored", Evidence: []string{"goal-baseline:sha256:test"}}, nil
		}
	}
	return kernel.NodeResult{Outcome: "blocked"}, nil
}

func TestDevelopArchitectedPathExecutesGoalsSubgraphEndToEnd(t *testing.T) {
	delegate := &scriptedExecutor{}
	executor, err := NewExecutor(delegate)
	if err != nil {
		t.Fatal(err)
	}
	run := &kernel.RunExecution{RunID: "develop-1", State: kernel.RunQueued}
	if err := kernel.Run(context.Background(), Graph(), run, executor); err != nil {
		t.Fatal(err)
	}
	if run.State != kernel.RunSucceeded {
		t.Fatalf("expected success, got %s", run.State)
	}
	if len(run.Evidence) != 1 || run.Evidence[0] != "goal-baseline:sha256:test" {
		t.Fatalf("child evidence did not reach parent: %v", run.Evidence)
	}
	seenGoals := false
	for _, entry := range delegate.seen {
		if entry == "praxis.package.goals.default:intent" {
			seenGoals = true
			break
		}
	}
	if !seenGoals {
		t.Fatal("develop architected path did not execute Goals graph")
	}
}

func TestDevelopFastPathDoesNotExecuteGoalsSubgraph(t *testing.T) {
	delegate := &scriptedExecutor{}
	// Override the scripted classifier by wrapping only that node.
	base := kernel.NodeExecutor(nodeExecutorFunc(func(ctx context.Context, graph kernel.GraphDef, node kernel.NodeDef, run *kernel.RunExecution) (kernel.NodeResult, error) {
		if graph.ID == "praxis.package.develop.default" && node.ID == "classify" {
			delegate.seen = append(delegate.seen, graph.ID+":"+node.ID)
			return kernel.NodeResult{Outcome: "fast"}, nil
		}
		return delegate.ExecuteNode(ctx, graph, node, run)
	}))
	executor, err := NewExecutor(base)
	if err != nil {
		t.Fatal(err)
	}
	run := &kernel.RunExecution{RunID: "develop-fast", State: kernel.RunQueued}
	if err := kernel.Run(context.Background(), Graph(), run, executor); err != nil {
		t.Fatal(err)
	}
	for _, entry := range delegate.seen {
		if len(entry) >= 21 && entry[:21] == "praxis.package.goals" {
			t.Fatalf("fast path executed Goals: %s", entry)
		}
	}
}

type nodeExecutorFunc func(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution) (kernel.NodeResult, error)

func (f nodeExecutorFunc) ExecuteNode(ctx context.Context, g kernel.GraphDef, n kernel.NodeDef, r *kernel.RunExecution) (kernel.NodeResult, error) {
	return f(ctx, g, n, r)
}
