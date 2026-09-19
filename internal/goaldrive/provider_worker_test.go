package goaldrive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
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

type delayedAppendStore struct {
	eventstore.Store
	delay time.Duration
}

func (s delayedAppendStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error) {
	time.Sleep(s.delay)
	return s.Store.Append(ctx, aggregateID, expectedVersion, events)
}

func TestProviderCLIWorkerDoesNotApplyPipeWaitDelayToDurableTranscriptPersistence(t *testing.T) {
	store := delayedAppendStore{Store: eventstore.NewMemoryStore(), delay: 2200 * time.Millisecond}
	activity := &ActivityLog{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	request := providerWorkerRequest()
	request.InvocationID = "slow-persistence-1"
	request.ProviderID = "codex-subscription"
	request.Activity = activity
	worker := ProviderCLIWorker{ProviderID: request.ProviderID, Command: []string{"/bin/sh", "-c", "printf 'provider completed\\n'"}}

	result, err := worker.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("successful provider output must drain before waiting for durable persistence: %v", err)
	}
	if result.Outcome != OutcomeContinue {
		t.Fatalf("successful provider result changed: %+v", result)
	}
	events, err := activity.Load(context.Background(), request.InvocationID, request.TurnID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != ActivityProviderMessage || events[0].Data["message"] != "provider completed" {
		t.Fatalf("durable provider transcript mismatch: %+v", events)
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
