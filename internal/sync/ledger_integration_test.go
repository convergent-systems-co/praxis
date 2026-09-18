package sync

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/stateprovider"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type allowImportAuthority struct{}

func (allowImportAuthority) AuthorizeImport(_ context.Context, destination, _ string, _ contracts.PrincipalRef) (contracts.PrincipalRef, error) {
	return contracts.PrincipalRef{ID: "governor:" + destination, Kind: "governance"}, nil
}

func TestCanonicalPortableStateReconcilesConcurrentMachinesAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)
	pathA := filepath.Join(t.TempDir(), "machine-a.db")
	pathB := filepath.Join(t.TempDir(), "machine-b.db")
	providerA, err := stateprovider.OpenSQLite(ctx, pathA)
	if err != nil {
		t.Fatal(err)
	}
	defer providerA.Close()
	providerB, err := stateprovider.OpenSQLite(ctx, pathB)
	if err != nil {
		t.Fatal(err)
	}
	ledgerA, _ := NewLedger(providerA.Events(), "machine-a", allowImportAuthority{})
	ledgerB, _ := NewLedger(providerB.Events(), "machine-b", allowImportAuthority{})

	baseGeneration := freezePortable(t, PortableRecord{Kind: RecordAgentGeneration, EntityID: "agent:shared", Scope: "owner:one", Portability: PortableVersioned, Merge: MergeVersioned, SourceInstallation: "machine-a", SourceAgentID: "agent:shared", GenerationID: "generation:base", SessionID: "session:a1", MutationType: "generation_created", Trust: contracts.TrustPolicy, Sensitivity: PublicState, Payload: json.RawMessage(`{"graph":"research@1"}`), OccurredAt: base})
	if err := ledgerA.RecordLocal(ctx, baseGeneration); err != nil {
		t.Fatal(err)
	}
	initial, err := ledgerA.Export(ctx, 1, []string{baseGeneration.ID}, base.Add(time.Minute), "", "")
	if err != nil {
		t.Fatal(err)
	}
	firstImport, err := ledgerB.Import(ctx, initial)
	if err != nil || !sameStrings(firstImport.AddedIDs, []string{baseGeneration.ID}) || len(firstImport.Conflicts) != 0 {
		t.Fatalf("initial lineage import changed semantics: %#v %v", firstImport, err)
	}
	duplicate, err := ledgerB.Import(ctx, initial)
	if err != nil || duplicate.EnvelopeID != initial.ID || len(duplicate.AddedIDs) != 0 || len(duplicate.Conflicts) != 0 {
		t.Fatalf("duplicate envelope was not idempotent: %#v %v", duplicate, err)
	}

	aHead := freezePortable(t, PortableRecord{Kind: RecordAgentGeneration, EntityID: "agent:shared", Scope: "owner:one", Portability: PortableVersioned, Merge: MergeVersioned, SourceInstallation: "machine-a", SourceAgentID: "agent:shared", GenerationID: "generation:a", SessionID: "session:a2", ParentIDs: []string{baseGeneration.ID}, EvidenceIDs: []string{"evidence:a"}, MutationType: "generation_created", Trust: contracts.TrustDerived, Sensitivity: PublicState, Payload: json.RawMessage(`{"graph":"research@2-a"}`), OccurredAt: base.Add(2 * time.Minute)})
	bHead := freezePortable(t, PortableRecord{Kind: RecordAgentGeneration, EntityID: "agent:shared", Scope: "owner:one", Portability: PortableVersioned, Merge: MergeVersioned, SourceInstallation: "machine-b", SourceAgentID: "agent:shared", GenerationID: "generation:b", SessionID: "session:b1", ParentIDs: []string{baseGeneration.ID}, EvidenceIDs: []string{"evidence:b"}, MutationType: "generation_created", Trust: contracts.TrustDerived, Sensitivity: PublicState, Payload: json.RawMessage(`{"graph":"research@2-b"}`), OccurredAt: base.Add(3 * time.Minute)})
	memoryA := freezePortable(t, PortableRecord{Kind: RecordMemory, EntityID: "memory:shared", Scope: "topic:climate", Portability: PortableMergeable, Merge: MergeCommutative, SourceInstallation: "machine-a", SourceAgentID: "agent:shared", SessionID: "session:a2", EvidenceIDs: []string{"source:a"}, MutationType: "memory_observed", Trust: contracts.TrustObserved, Sensitivity: PublicState, Payload: json.RawMessage(`{"fact":"A"}`), OccurredAt: base.Add(4 * time.Minute)})
	memoryB := freezePortable(t, PortableRecord{Kind: RecordMemory, EntityID: "memory:shared", Scope: "topic:climate", Portability: PortableMergeable, Merge: MergeCommutative, SourceInstallation: "machine-b", SourceAgentID: "agent:shared", SessionID: "session:b1", EvidenceIDs: []string{"source:b"}, MutationType: "memory_observed", Trust: contracts.TrustObserved, Sensitivity: PublicState, Payload: json.RawMessage(`{"fact":"B"}`), OccurredAt: base.Add(5 * time.Minute)})
	preferenceA := freezePortable(t, PortableRecord{Kind: RecordPreference, EntityID: "preference:presentation", Scope: "project:research", Portability: PortableMergeable, Merge: MergeExclusive, SourceInstallation: "machine-a", SourceAgentID: "agent:shared", SessionID: "session:a2", MutationType: "preference_set", Trust: contracts.TrustUserConfirmed, Sensitivity: PublicState, Payload: json.RawMessage(`{"value":"citations"}`), OccurredAt: base.Add(6 * time.Minute)})
	preferenceB := freezePortable(t, PortableRecord{Kind: RecordPreference, EntityID: "preference:presentation", Scope: "household:planning", Portability: PortableMergeable, Merge: MergeExclusive, SourceInstallation: "machine-b", SourceAgentID: "agent:shared", SessionID: "session:b1", MutationType: "preference_set", Trust: contracts.TrustUserConfirmed, Sensitivity: PublicState, Payload: json.RawMessage(`{"value":"concise"}`), OccurredAt: base.Add(7 * time.Minute)})
	policyA := freezePortable(t, PortableRecord{Kind: RecordPolicy, EntityID: "policy:release", Scope: "organization:one", Portability: PortableSingleWriter, Merge: MergeExclusive, SourceInstallation: "machine-a", SessionID: "session:a2", MutationType: "policy_changed", Trust: contracts.TrustPolicy, Sensitivity: PublicState, Payload: json.RawMessage(`{"approval":"two-person"}`), OccurredAt: base.Add(6 * time.Minute)})
	policyB := freezePortable(t, PortableRecord{Kind: RecordPolicy, EntityID: "policy:release", Scope: "organization:one", Portability: PortableSingleWriter, Merge: MergeExclusive, SourceInstallation: "machine-b", SessionID: "session:b1", MutationType: "policy_changed", Trust: contracts.TrustPolicy, Sensitivity: PublicState, Payload: json.RawMessage(`{"approval":"owner"}`), OccurredAt: base.Add(7 * time.Minute)})
	for _, record := range []PortableRecord{aHead, memoryA, preferenceA, policyA} {
		if err := ledgerA.RecordLocal(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range []PortableRecord{bHead, memoryB, preferenceB, policyB} {
		if err := ledgerB.RecordLocal(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	concurrent, err := ledgerA.Export(ctx, 2, []string{aHead.ID, memoryA.ID, preferenceA.ID, policyA.ID}, base.Add(8*time.Minute), "", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := ledgerB.Import(ctx, concurrent)
	if err != nil {
		t.Fatal(err)
	}
	if !sameStrings(result.AddedIDs, []string{aHead.ID, memoryA.ID, preferenceA.ID, policyA.ID}) || len(result.Conflicts) != 2 {
		t.Fatalf("reconciliation did not preserve the supplied concurrent records: %#v", result)
	}
	conflictsByKind := map[RecordKind]Conflict{}
	for _, conflict := range result.Conflicts {
		conflictsByKind[conflict.Kind] = conflict
	}
	generationConflict := conflictsByKind[RecordAgentGeneration]
	if generationConflict.EntityID != baseGeneration.EntityID || !sameStrings(generationConflict.CompetingRecordIDs, []string{aHead.ID, bHead.ID}) || generationConflict.SecuritySignificant {
		t.Fatalf("divergent versioned heads were not represented semantically: %#v", generationConflict)
	}
	policyConflict := conflictsByKind[RecordPolicy]
	if policyConflict.EntityID != policyA.EntityID || !sameStrings(policyConflict.CompetingRecordIDs, []string{policyA.ID, policyB.ID}) || !policyConflict.SecuritySignificant {
		t.Fatalf("single-writer policy conflict was not retained as security-significant: %#v", policyConflict)
	}
	heads, err := ledgerB.Heads(ctx, RecordAgentGeneration, baseGeneration.EntityID, baseGeneration.Scope)
	if !errors.Is(err, ErrUnresolvedConflict) || !recordIDsEqual(heads, []string{aHead.ID, bHead.ID}) {
		t.Fatalf("divergent heads were silently selected: %#v %v", heads, err)
	}
	if memoryHeads, err := ledgerB.Heads(ctx, RecordMemory, memoryA.EntityID, memoryA.Scope); err != nil || !recordIDsEqual(memoryHeads, []string{memoryA.ID, memoryB.ID}) {
		t.Fatalf("commutative learning did not coexist: %#v %v", memoryHeads, err)
	}
	if projectHeads, err := ledgerB.Heads(ctx, RecordPreference, preferenceA.EntityID, preferenceA.Scope); err != nil || !recordIDsEqual(projectHeads, []string{preferenceA.ID}) {
		t.Fatalf("context-separated preference did not retain its scope: %#v %v", projectHeads, err)
	}
	if householdHeads, err := ledgerB.Heads(ctx, RecordPreference, preferenceB.EntityID, preferenceB.Scope); err != nil || !recordIDsEqual(householdHeads, []string{preferenceB.ID}) {
		t.Fatalf("local context preference was overwritten: %#v %v", householdHeads, err)
	}
	if policyHeads, err := ledgerB.Heads(ctx, RecordPolicy, policyA.EntityID, policyA.Scope); !errors.Is(err, ErrUnresolvedConflict) || !recordIDsEqual(policyHeads, []string{policyA.ID, policyB.ID}) {
		t.Fatalf("policy conflict did not fail closed: %#v %v", policyHeads, err)
	}
	sameSequence, err := FreezeEnvelope(StateEnvelope{Source: concurrent.Source, SourceSequence: concurrent.SourceSequence, CreatedAt: base.Add(9 * time.Minute), Records: []PortableRecord{memoryA}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerB.Import(ctx, sameSequence); err == nil {
		t.Fatal("different portable envelope reused an accepted source sequence")
	}

	tombstone := freezePortable(t, PortableRecord{Kind: RecordMemory, EntityID: memoryA.EntityID, Scope: memoryA.Scope, Portability: PortableMergeable, Merge: MergeCommutative, SourceInstallation: "machine-b", SourceAgentID: "agent:shared", SessionID: "session:b2", ParentIDs: []string{memoryA.ID}, SupersedesIDs: []string{memoryA.ID}, EvidenceIDs: []string{"revocation:b"}, MutationType: "tombstone", Trust: contracts.TrustPolicy, Sensitivity: PublicState, Payload: json.RawMessage(`{"reason":"superseded"}`), OccurredAt: base.Add(10 * time.Minute)})
	if err := ledgerB.RecordLocal(ctx, tombstone); err != nil {
		t.Fatal(err)
	}
	staleContents, err := ledgerA.Export(ctx, 3, []string{memoryA.ID}, base.Add(11*time.Minute), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerB.Import(ctx, staleContents); err != nil {
		t.Fatal(err)
	}
	memoryHeads, err := ledgerB.Heads(ctx, RecordMemory, memoryA.EntityID, memoryA.Scope)
	if err != nil || !recordIDsEqual(memoryHeads, []string{memoryB.ID, tombstone.ID}) {
		t.Fatalf("stale export resurrected superseded memory: %#v %v", memoryHeads, err)
	}

	beforeRestart, err := ledgerB.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := providerB.Close(); err != nil {
		t.Fatal(err)
	}
	providerB, err = stateprovider.OpenSQLite(ctx, pathB)
	if err != nil {
		t.Fatal(err)
	}
	defer providerB.Close()
	ledgerB, _ = NewLedger(providerB.Events(), "machine-b", allowImportAuthority{})
	afterRestart, err := ledgerB.State(ctx)
	if err != nil || !reflect.DeepEqual(afterRestart, beforeRestart) {
		t.Fatalf("portable reconciliation changed after restart: before=%#v after=%#v err=%v", beforeRestart, afterRestart, err)
	}
	if _, err := ledgerB.Heads(ctx, RecordAgentGeneration, baseGeneration.EntityID, baseGeneration.Scope); !errors.Is(err, ErrUnresolvedConflict) {
		t.Fatalf("restart hid unresolved generation conflict: %v", err)
	}

	if _, err := FreezeRecord(PortableRecord{Kind: RecordApproval, EntityID: "approval:one", Scope: "run:one", Portability: LocalEphemeral, Merge: MergeExclusive, SourceInstallation: "machine-a", SessionID: "session:a", MutationType: "approval", Trust: contracts.TrustUserConfirmed, Sensitivity: PublicState, Payload: json.RawMessage(`{"remaining_uses":1}`), OccurredAt: base}); err == nil {
		t.Fatal("one-shot approval became portable reusable authority")
	}
	if _, err := FreezeRecord(PortableRecord{Kind: RecordCapabilityLease, EntityID: "lease:one", Scope: "run:one", Portability: LocalEphemeral, Merge: MergeExclusive, SourceInstallation: "machine-a", SessionID: "session:a", MutationType: "lease", Trust: contracts.TrustPolicy, Sensitivity: PublicState, Payload: json.RawMessage(`{"active":true}`), OccurredAt: base}); err == nil {
		t.Fatal("runtime capability lease became portable reusable authority")
	}
}

func freezePortable(t *testing.T, record PortableRecord) PortableRecord {
	t.Helper()
	frozen, err := FreezeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func recordIDsEqual(records []PortableRecord, expected []string) bool {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return sameStrings(ids, expected)
}

func sameStrings(actual, expected []string) bool {
	return reflect.DeepEqual(canonicalStrings(actual), canonicalStrings(expected))
}
