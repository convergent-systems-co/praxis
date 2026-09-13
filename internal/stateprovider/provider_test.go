package stateprovider

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestSQLiteProviderAdvertisesRequiredLocalSemantics(t *testing.T) {
	ctx := context.Background()
	provider, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()

	required := []Capability{
		EventsAppendOptimistic, EventsReplay, EventsGlobalSequence,
		ProjectionsCheckpointed, ApprovalAtomicConsume, LeaseAtomicConsume,
		EffectsOutboxReconciliation, SecureBlobsImmutableEncrypted,
		PackagesAtomicActivation, RunsDurableReplay, MigrationsVersioned,
		TransactionsMultiRepository,
	}
	if err := provider.Profile().Require(required...); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownProviderCapabilityFailsClosed(t *testing.T) {
	profile := Profile{EventsReplay: Unknown}
	if err := profile.Require(EventsReplay); err == nil {
		t.Fatal("unknown provider capability must fail closed")
	}
}

func TestSQLiteProviderEventSemanticsMatchCanonicalStore(t *testing.T) {
	ctx := context.Background()
	provider, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()

	now := time.Now().UTC()
	proposed := []eventstore.Event{{
		ID: "evt-1", AggregateType: "fixture", Type: "fixture.started", Version: "1",
		Actor: contracts.PrincipalRef{ID: "tester", Kind: "user"}, CommandID: "cmd-1",
		CorrelationID: "corr-1", Payload: []byte(`{}`), CreatedAt: now,
	}}
	appended, err := provider.Events().Append(ctx, "agg-1", 0, proposed)
	if err != nil {
		t.Fatal(err)
	}
	if len(appended) != 1 || appended[0].AggregateVersion != 1 || appended[0].Sequence <= 0 {
		t.Fatalf("unexpected appended event: %#v", appended)
	}
	if _, err := provider.Events().Append(ctx, "agg-1", 0, proposed); err == nil {
		t.Fatal("stale aggregate version must fail")
	}
	loaded, err := provider.Events().LoadAggregate(ctx, "agg-1", 0)
	if err != nil || len(loaded) != 1 || loaded[0].ID != "evt-1" {
		t.Fatalf("replay mismatch loaded=%#v err=%v", loaded, err)
	}
}

func TestSQLiteProviderPackageRegistryIsProviderNeutral(t *testing.T) {
	ctx := context.Background()
	provider, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()

	manifest := packagecatalog.Manifest{PackageID: "fixture/pkg", Version: "1.0.0", ContentDigest: "sha256:fixture"}
	if err := provider.Packages().ActivatePackage(ctx, manifest, "test", "fixture", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	active, err := provider.Packages().ActivePackage(ctx, manifest.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Manifest.PackageID != manifest.PackageID || active.State != "active" {
		t.Fatalf("unexpected active package: %#v", active)
	}
}

func TestEventRuntimeSemanticsSurviveProviderSubstitutionAndMismatchFailsClosed(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC)
	sqlite, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	providers := map[string]Provider{"memory": NewMemory(), "sqlite": sqlite}
	results := map[string][]eventstore.Event{}
	for name, provider := range providers {
		store, err := RequireEvents(provider, EventsAppendOptimistic, EventsReplay, EventsGlobalSequence)
		if err != nil {
			t.Fatalf("%s provider rejected required semantics: %v", name, err)
		}
		proposed := eventstore.Event{ID: "event:portable", AggregateType: "portable_fixture", Type: "portable.recorded", Version: "v1", Actor: contracts.PrincipalRef{ID: "agent:one", Kind: "agent"}, CommandID: "command:portable", CorrelationID: "portable:one", CausationID: "root:one", Trust: contracts.TrustObserved, Payload: []byte(`{"fact":"same"}`), CreatedAt: now}
		if _, err := store.Append(ctx, "portable:one", 0, []eventstore.Event{proposed}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Append(ctx, "portable:one", 0, []eventstore.Event{proposed}); !errors.Is(err, eventstore.ErrVersionConflict) {
			t.Fatalf("%s provider weakened optimistic concurrency: %v", name, err)
		}
		replayed, err := store.LoadAggregate(ctx, "portable:one", 0)
		if err != nil {
			t.Fatal(err)
		}
		for i := range replayed {
			replayed[i].Sequence = 0
		}
		results[name] = replayed
	}
	if !reflect.DeepEqual(results["memory"], results["sqlite"]) {
		t.Fatalf("provider substitution changed canonical event semantics: memory=%#v sqlite=%#v", results["memory"], results["sqlite"])
	}

	deficient := &MemoryProvider{events: eventstore.NewMemoryStore(), profile: Profile{EventsReplay: Enforced, EventsGlobalSequence: Enforced}}
	if _, err := RequireEvents(deficient, EventsAppendOptimistic, EventsReplay, EventsGlobalSequence); err == nil {
		t.Fatal("runtime accepted provider without optimistic append capability")
	}
	loaded, err := deficient.Events().LoadAggregate(ctx, "portable:one", 0)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("capability mismatch mutated deficient provider: %#v %v", loaded, err)
	}
}
