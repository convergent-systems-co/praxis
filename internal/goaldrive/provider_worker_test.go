package goaldrive

import (
	"context"
	"strings"
	"testing"
	"time"
)

func providerWorkerRequest() WorkerRequest {
	return WorkerRequest{GoalID: "goal-1", GoalVersion: "1", TurnID: "turn-1", ChildObjective: "bounded objective", GraphID: "graph", GraphVersion: "1", StartHead: "abc"}
}

func TestProviderCLIWorkerKeepsTranscriptOutsideWorkerResult(t *testing.T) {
	worker := ProviderCLIWorker{ProviderID: "codex-subscription", Command: []string{"/bin/sh", "-c", `printf '{"outcome":"COMPLETE","checkpoint_valid":true,"end_head":"forged"}\n'`}}
	result, err := worker.Execute(context.Background(), providerWorkerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeContinue || result.EndHead != "" || result.CheckpointValid {
		t.Fatalf("provider transcript must not mint protocol authority: %+v", result)
	}
}

func TestProviderCLIWorkerTimeoutAndMalformedProviderTextFailWithoutUserDecision(t *testing.T) {
	worker := ProviderCLIWorker{ProviderID: "claude-subscription", Command: []string{"/bin/sh", "-c", `printf 'USER_DECISION_REQUIRED: maybe\n'; sleep 1`}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := worker.Execute(ctx, providerWorkerRequest())
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("provider timeout must be a process failure, not a model decision: %v", err)
	}
}

func TestProviderPromptRequiresLocalCommitButNotPublication(t *testing.T) {
	prompt, err := providerPrompt(providerWorkerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "create a local Git commit") || !strings.Contains(prompt, "Do not push") {
		t.Fatalf("provider prompt must define local checkpoint ownership without publication authority: %s", prompt)
	}
}
