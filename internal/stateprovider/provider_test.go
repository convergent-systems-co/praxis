package stateprovider

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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

	now := time.Now().UTC()
	artifact := []byte("fixture-package")
	artifactSum := sha256.Sum256(artifact)
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "fixture/pkg", Version: "1.0.0", ContentDigest: "sha256:" + hex.EncodeToString(artifactSum[:])}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestSum := sha256.Sum256(manifestBytes)
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: "sha256:" + hex.EncodeToString(manifestSum[:]), ArtifactDigest: manifest.ContentDigest}
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "publisher"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	verified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope, SourceKind: "fixture", SourceRef: "fixture/pkg@1.0.0", VerifiedAt: now}, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"publisher": pub}}})
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "operator", Kind: "user"}
	intent, err := packagecatalog.NewActivationIntent(verified, actor)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, "package-approval", actor.ID, actor.Kind, intentDigest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := provider.Packages().ActivatePackage(ctx, packagecatalog.ActivationRequest{Package: verified, Intent: intent, ApprovalID: "package-approval"}, now); err != nil {
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
