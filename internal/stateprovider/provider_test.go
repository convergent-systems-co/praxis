package stateprovider

import (
	"context"
	"path/filepath"
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
		CorrelationID: "corr-1", CreatedAt: now,
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
