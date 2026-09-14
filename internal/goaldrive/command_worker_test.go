package goaldrive

import (
	"context"
	"strings"
	"testing"
)

func TestCommandWorkerUsesExplicitArgvAndBindsProviderIdentity(t *testing.T) {
	worker := CommandWorker{ProviderID: "codex-test", Command: []string{"/usr/bin/printf", `{"outcome":"CONTINUE","end_head":"b","checkpoint_valid":true}`}}
	result, err := worker.Execute(context.Background(), WorkerRequest{GoalID: "goal-1"})
	if err != nil || result.Outcome != OutcomeContinue || result.ExecutorID != "codex-test" || result.EndHead != "b" {
		t.Fatalf("unexpected command worker result: %+v err=%v", result, err)
	}
}

func TestCommandWorkerRejectsMalformedAndOversizedResults(t *testing.T) {
	malformed := CommandWorker{ProviderID: "test", Command: []string{"/usr/bin/printf", "not-json"}}
	if _, err := malformed.Execute(context.Background(), WorkerRequest{}); err == nil {
		t.Fatal("malformed worker output must fail closed")
	}
	oversized := CommandWorker{ProviderID: "test", Command: []string{"/usr/bin/printf", strings.Repeat("x", 32)}, OutputLimit: 8}
	if _, err := oversized.Execute(context.Background(), WorkerRequest{}); err == nil {
		t.Fatal("oversized worker output must fail closed")
	}
}
