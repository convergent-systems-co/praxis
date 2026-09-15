package research

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

type researchScript struct{ seen []string }

func (s *researchScript) ExecuteNode(_ context.Context, graph kernel.GraphDef, node kernel.NodeDef, _ *kernel.RunExecution) (kernel.NodeResult, error) {
	s.seen = append(s.seen, graph.ID+":"+node.ID)
	if graph.ID == "praxis.package.research.default" {
		switch node.ID {
		case "classify":
			return kernel.NodeResult{Outcome: "goals"}, nil
		case "scope":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "discover":
			return kernel.NodeResult{Outcome: "ready", Evidence: []string{"source:1"}}, nil
		case "evaluate":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "synthesize":
			return kernel.NodeResult{Outcome: "ready"}, nil
		case "challenge":
			return kernel.NodeResult{Outcome: "pass"}, nil
		case "evidence":
			return kernel.NodeResult{Outcome: "ephemeral", Evidence: []string{"research:evidence-pack"}}, nil
		}
	}
	if graph.ID == "praxis.package.goals.default" {
		switch node.ID {
		case "intent":
			return kernel.NodeResult{Outcome: "captured"}, nil
		case "frame":
			return kernel.NodeResult{Outcome: "structured"}, nil
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
			return kernel.NodeResult{Outcome: "no_execution"}, nil
		case "baseline":
			return kernel.NodeResult{Outcome: "ephemeral", Evidence: []string{"goal-baseline:research"}}, nil
		}
	}
	return kernel.NodeResult{Outcome: "blocked"}, nil
}

func TestResearchGraphAndInvocationValidate(t *testing.T) {
	if err := Graph().Validate(); err != nil {
		t.Fatalf("research graph invalid: %v", err)
	}
	if err := Invocation().Validate(); err != nil {
		t.Fatalf("research invocation invalid: %v", err)
	}
}

func TestResearchExecutesGoalsAndResearchEvidenceFlow(t *testing.T) {
	delegate := &researchScript{}
	exec, err := NewExecutor(delegate)
	if err != nil {
		t.Fatal(err)
	}
	run := &kernel.RunExecution{RunID: "research-1", State: kernel.RunQueued}
	if err := kernel.Run(context.Background(), Graph(), run, exec); err != nil {
		t.Fatal(err)
	}
	if run.State != kernel.RunSucceeded {
		t.Fatalf("expected success got %s", run.State)
	}
	if len(run.Evidence) != 3 {
		t.Fatalf("expected Goals + source + research evidence, got %v", run.Evidence)
	}
	seenGoals := false
	for _, v := range delegate.seen {
		if v == "praxis.package.goals.default:intent" {
			seenGoals = true
		}
	}
	if !seenGoals {
		t.Fatal("research did not compose Goals")
	}
}

func TestResearchCanReuseExistingBaselineWithoutGoals(t *testing.T) {
	delegate := &researchScript{}
	base := nodeExecutorFunc(func(ctx context.Context, g kernel.GraphDef, n kernel.NodeDef, r *kernel.RunExecution) (kernel.NodeResult, error) {
		if g.ID == "praxis.package.research.default" && n.ID == "classify" {
			delegate.seen = append(delegate.seen, g.ID+":"+n.ID)
			return kernel.NodeResult{Outcome: "baseline"}, nil
		}
		return delegate.ExecuteNode(ctx, g, n, r)
	})
	exec, err := NewExecutor(base)
	if err != nil {
		t.Fatal(err)
	}
	run := &kernel.RunExecution{RunID: "research-reuse", State: kernel.RunQueued}
	if err := kernel.Run(context.Background(), Graph(), run, exec); err != nil {
		t.Fatal(err)
	}
	for _, v := range delegate.seen {
		if v == "praxis.package.goals.default:intent" {
			t.Fatal("valid existing baseline should avoid repeating Goals")
		}
	}
}

type nodeExecutorFunc func(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution) (kernel.NodeResult, error)

func (f nodeExecutorFunc) ExecuteNode(ctx context.Context, g kernel.GraphDef, n kernel.NodeDef, r *kernel.RunExecution) (kernel.NodeResult, error) {
	return f(ctx, g, n, r)
}
