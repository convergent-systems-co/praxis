package sync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestRuntimeAuthorityIsNotReusableOnImport(t *testing.T) {
	for _, kind := range []RecordKind{RecordApproval, RecordCapabilityLease, RecordClientSession, RecordResourceLease} {
		if ReusableAuthorityOnImport(kind) {
			t.Fatalf("%s must not become reusable authority after import", kind)
		}
	}
}

func TestAgentGenerationIsPortableVersioned(t *testing.T) {
	class, err := Classify(RecordAgentGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if class != PortableVersioned {
		t.Fatalf("unexpected class %s", class)
	}
}

func TestPortableImportReplayRejectsAuthorityActorMismatch(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 5, 0, 0, 0, time.UTC)
	record, err := FreezeRecord(PortableRecord{Kind: RecordMemory, EntityID: "memory:one", Scope: "topic:one", Portability: PortableMergeable, Merge: MergeCommutative, SourceInstallation: "machine-a", SessionID: "session-a", MutationType: "observed", Trust: contracts.TrustObserved, Sensitivity: PublicState, Payload: json.RawMessage(`{"fact":"one"}`), OccurredAt: now})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := FreezeEnvelope(StateEnvelope{Source: contracts.PrincipalRef{ID: "machine-a", Kind: "installation"}, SourceSequence: 1, CreatedAt: now, Records: []PortableRecord{record}})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := reconcile(State{SourceSequences: map[string]int64{}}, envelope)
	authority := contracts.PrincipalRef{ID: "governor:machine-b", Kind: "governance"}
	payload, _ := json.Marshal(importPayload{Envelope: envelope, Result: result, Authority: authority})
	store := eventstore.NewMemoryStore()
	_, err = store.Append(ctx, portableAggregate("machine-b"), 0, []eventstore.Event{{ID: "event:import:" + envelope.ID, AggregateType: "portable_state", Type: portableEnvelopeImported, Version: portableEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: "attacker", Kind: "governance"}, CommandID: "portable-import:" + envelope.ID, CorrelationID: envelope.ID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: now}})
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := NewLedger(store, "machine-b", allowImportAuthority{})
	if _, err := ledger.State(ctx); err == nil {
		t.Fatal("portable replay accepted an actor not bound to the import authority")
	}
	future := envelope
	future.Version = "v99"
	if err := VerifyEnvelope(future); !errors.Is(err, contracts.ErrUnknownContractVersion) {
		t.Fatalf("unknown portable envelope version did not fail through contract metadata: %v", err)
	}
}
