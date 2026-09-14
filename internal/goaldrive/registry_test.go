package goaldrive

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestProviderRegistryRequiresExplicitUniqueProviderIdentity(t *testing.T) {
	registry := NewRegistry()
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeNoProgress}}
	if err := registry.Register("codex-test", worker); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("codex-test"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("codex-test", worker); err == nil {
		t.Fatal("duplicate provider registration must fail")
	}
	if _, err := registry.Resolve("claude-test"); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown provider must fail closed: %v", err)
	}
}

func TestControllerResolvesProviderFromRegistry(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeNoProgress}}
	registry := NewRegistry()
	if err := registry.Register("provider-1", worker); err != nil {
		t.Fatal(err)
	}
	controller := Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Providers: registry, NoProgressLimit: 1}
	request := turnRequest()
	request.ProviderID = "provider-1"
	if _, err := controller.ExecuteTurn(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.TurnID = "turn-2"
	request.ProviderID = "missing"
	if _, err := controller.ExecuteTurn(context.Background(), request); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("missing provider must fail closed: %v", err)
	}
}
