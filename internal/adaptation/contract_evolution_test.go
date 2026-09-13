package adaptation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type allowContractConfirmation struct{}

func (allowContractConfirmation) AuthorizeEvidenceConfirmation(context.Context, string, string, string) error {
	return nil
}

func canonicalV2Observation(t *testing.T, trust contracts.TrustClass) Observation {
	t.Helper()
	now := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
	observation := Observation{SubjectAgentID: "agent-research", RunID: "run-one", GoalClass: "source-synthesis", Domain: "research", BehaviorKey: "triangulate", Context: "topic-one", CausationRoot: "source-set-one", Trust: trust, ReasoningTier: "D1", ProviderID: "provider-a", Outcome: "complete", PathID: "search-path", RawMeasures: []Measure{{Name: "search_duration", Value: 842, Kind: RawMeasure, Unit: "ms", Provenance: "runtime-clock", MeasuredAt: now}}, Invariants: []InvariantResult{{Class: SecurityInvariant, ControlID: "source-boundary", Passed: true, EvidenceRef: "check:one"}}, ObservedAt: now}
	if trust == contracts.TrustUserConfirmed {
		observation.ConfirmationAuthorityID = "owner"
		observation.ConfirmationEvidenceRef = "approval:observation-one"
	}
	frozen, err := FreezeObservation(observation)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func appendAdaptiveFixture(t *testing.T, store eventstore.Store, subject, eventType, version string, payload any, actor contracts.PrincipalRef, trust contracts.TrustClass) {
	t.Helper()
	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Append(context.Background(), ledgerAggregate(subject), 0, []eventstore.Event{{ID: "event:fixture", AggregateType: "adaptive_behavior", Type: eventType, Version: version, Actor: actor, CommandID: "fixture", CorrelationID: "fixture", Trust: trust, Payload: bytes, CreatedAt: time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAdaptiveV2ContractsRoundTripWithStableIdentity(t *testing.T) {
	ctx := context.Background()
	store := eventstore.NewMemoryStore()
	ledger, err := NewLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	observation := canonicalV2Observation(t, contracts.TrustObserved)
	if observation.ID != "sha256:edf6273df2c8482dab79703115deba7e291284300a1dce94272dc9be5eab0280" {
		t.Fatalf("canonical v2 observation identity changed: %s", observation.ID)
	}
	if err := ledger.Record(ctx, observation); err != nil {
		t.Fatal(err)
	}
	fact, err := FreezeProfileFact(ProfileFact{SubjectAgentID: observation.SubjectAgentID, Dimension: "source-style", Score: Score{Value: 0.75, Range: NumericRange{Minimum: 0, Maximum: 1}}, EvidenceClass: Declared, Confidence: Score{Value: 0.8, Range: NumericRange{Minimum: 0, Maximum: 1}}, Provenance: "research-package", Context: observation.Context, RecordedAt: observation.ObservedAt.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if fact.ID != "sha256:1c3747b2824198ed2e57e880b0dc81bff710e2e55669cba94bb40eb70b990a61" {
		t.Fatalf("canonical v2 profile identity changed: %s", fact.ID)
	}
	if err := ledger.RecordProfileFact(ctx, fact); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	observations, err := restarted.Observations(ctx, observation.SubjectAgentID)
	if err != nil || !reflect.DeepEqual(observations, []Observation{observation}) {
		t.Fatalf("v2 observation did not round-trip: %#v %v", observations, err)
	}
	facts, err := restarted.ProfileHistory(ctx, observation.SubjectAgentID)
	if err != nil || !reflect.DeepEqual(facts, []ProfileFact{fact}) {
		t.Fatalf("v2 profile fact did not round-trip: %#v %v", facts, err)
	}
}

func TestConfirmedObservationRequiresAuthorityAndReplaysActorBinding(t *testing.T) {
	ctx := context.Background()
	observation := canonicalV2Observation(t, contracts.TrustUserConfirmed)
	unauthorized, _ := NewLedger(eventstore.NewMemoryStore())
	if err := unauthorized.Record(ctx, observation); err == nil {
		t.Fatal("caller-selected user_confirmed observation bypassed deterministic authorization")
	}

	store := eventstore.NewMemoryStore()
	authorized, _ := NewLedger(store, allowContractConfirmation{})
	if err := authorized.Record(ctx, observation); err != nil {
		t.Fatal(err)
	}
	restarted, _ := NewLedger(store)
	replayed, err := restarted.Observations(ctx, observation.SubjectAgentID)
	if err != nil || !reflect.DeepEqual(replayed, []Observation{observation}) {
		t.Fatalf("confirmation authority binding did not survive restart: %#v %v", replayed, err)
	}

	badStore := eventstore.NewMemoryStore()
	appendAdaptiveFixture(t, badStore, observation.SubjectAgentID, observationEvent, observationEventContract.CurrentVersion(), observation, contracts.PrincipalRef{ID: observation.SubjectAgentID, Kind: "agent"}, contracts.TrustUserConfirmed)
	badLedger, _ := NewLedger(badStore)
	if _, err := badLedger.Observations(ctx, observation.SubjectAgentID); err == nil || !strings.Contains(err.Error(), "metadata does not bind observation") {
		t.Fatalf("actor mismatch did not fail replay: %v", err)
	}
}

func TestAdaptivePreReleaseAndUnknownVersionsFailClosedDistinctly(t *testing.T) {
	ctx := context.Background()
	for name, registry := range map[string]*contracts.VersionRegistry{"observation": observationEventContract, "profile": profileFactEventContract} {
		definition, ok := registry.Definition("v1")
		if !ok || definition.Disposition != contracts.VersionUnsupportedPreRelease {
			t.Fatalf("%s v1 compatibility is not owned by contract metadata: %#v", name, definition)
		}
	}
	for _, eventType := range []string{observationEvent, profileFactEvent} {
		t.Run(eventType+"-v1", func(t *testing.T) {
			store := eventstore.NewMemoryStore()
			appendAdaptiveFixture(t, store, "agent", eventType, "v1", map[string]string{"version": "v1"}, contracts.PrincipalRef{ID: "agent", Kind: "agent"}, contracts.TrustObserved)
			ledger, _ := NewLedger(store)
			var err error
			if eventType == observationEvent {
				_, err = ledger.Observations(ctx, "agent")
			} else {
				_, err = ledger.ProfileHistory(ctx, "agent")
			}
			if !errors.Is(err, contracts.ErrUnsupportedPreReleaseContractVersion) || !strings.Contains(err.Error(), "durable support begins at v2") {
				t.Fatalf("v1 was not identified as unsupported pre-release contract: %v", err)
			}
		})
	}

	store := eventstore.NewMemoryStore()
	appendAdaptiveFixture(t, store, "agent", observationEvent, "v99", map[string]string{"version": "v99"}, contracts.PrincipalRef{ID: "agent", Kind: "agent"}, contracts.TrustObserved)
	ledger, _ := NewLedger(store)
	if _, err := ledger.Observations(ctx, "agent"); !errors.Is(err, contracts.ErrUnknownContractVersion) || errors.Is(err, contracts.ErrUnsupportedPreReleaseContractVersion) {
		t.Fatalf("unknown future version was not distinguished from pre-release v1: %v", err)
	}
}
