package develop

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

type developScript struct { outcomes map[string]string }
func (s developScript) ExecuteNode(_ context.Context, _ kernel.GraphDef, node kernel.NodeDef, _ *kernel.RunExecution) (kernel.NodeResult, error) {
	return kernel.NodeResult{Outcome: s.outcomes[node.ID], Evidence: []string{"ok:" + node.ID}}, nil
}

func TestDevelopFastPathRunsToCompletion(t *testing.T) {
	g := Graph()
	run := &kernel.RunExecution{RunID: "develop-run-1"}
	executor := developScript{outcomes: map[string]string{
		"discover": "ready", "classify": "fast", "prepare": "ready", "implement": "done", "validate": "pass", "review": "pass", "integrate": "done",
	}}
	if err := kernel.Run(context.Background(), g, run, executor); err != nil { t.Fatal(err) }
	if run.State != kernel.RunSucceeded || run.CurrentNode != "complete" {
		t.Fatalf("develop run did not complete: %+v", run)
	}
	for _, evidence := range run.Evidence {
		if evidence == "ok:plan" { t.Fatal("fast path must not execute planning node") }
	}
}
