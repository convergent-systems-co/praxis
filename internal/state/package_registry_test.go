package state

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func fixturePackage(id, version, digest, alias string) packagecatalog.Manifest {
	artifactSum := sha256.Sum256([]byte("package-artifact:" + id + "@" + version))
	digest = "sha256:" + hex.EncodeToString(artifactSum[:])
	return packagecatalog.Manifest{
		PackageID: id, Version: version, ContentDigest: digest,
		Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentGraph, ID: id + ".graph", Version: version, Digest: "sha256:graph-" + version, Artifact: "graphs/default.json"}},
		Invocations: []contracts.InvocationContract{{
			Version: "v1", PackageID: id, PackageVersion: version,
			GraphID: id + ".graph", GraphVersion: version,
			EntryPointID: id + ".default", Aliases: []string{alias},
			Options: []contracts.InvocationOption{{Name: "flag", Type: "bool"}},
		}},
	}
}

func fixtureActivationRequest(t *testing.T, ctx context.Context, db *sql.DB, manifest packagecatalog.Manifest, now time.Time) (packagecatalog.Manifest, packagecatalog.ActivationRequest) {
	t.Helper()
	artifact := []byte("package-artifact:" + manifest.PackageID + "@" + manifest.Version)
	sum := sha256.Sum256(artifact)
	manifest.ContentDigest = "sha256:" + hex.EncodeToString(sum[:])
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
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "fixture-publisher"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	verified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope, SourceKind: "fixture", SourceRef: manifest.PackageID + "@" + manifest.Version, VerifiedAt: now}, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"fixture-publisher": pub}}})
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "fixture-authority", Kind: "user"}
	intent, err := packagecatalog.NewActivationIntent(verified, actor)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	approvalID := "approval:" + manifest.PackageID + ":" + manifest.Version
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, approvalID, actor.ID, actor.Kind, intentDigest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return manifest, packagecatalog.ActivationRequest{Package: verified, Intent: intent, ApprovalID: approvalID}
}

func activateFixturePackage(t *testing.T, ctx context.Context, db *sql.DB, s *Store, manifest packagecatalog.Manifest, now time.Time) packagecatalog.Manifest {
	t.Helper()
	manifest, request := fixtureActivationRequest(t, ctx, db, manifest, now)
	if err := s.ActivatePackage(ctx, request, now); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestPackageActivationPublishesAndRemovesAliasesAndContents(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	m := fixturePackage("example/pkg", "1.0.0", "sha256:one", "example")
	m = activateFixturePackage(t, ctx, db, s, m, time.Now().UTC())
	got, err := s.ResolveInvocationAlias(ctx, "example")
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.PackageID != m.PackageID || got.ContentDigest != m.ContentDigest {
		t.Fatalf("wrong registered package: %#v", got)
	}
	content, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if content.PackageID != m.PackageID || content.Content.Digest != "sha256:graph-1.0.0" {
		t.Fatalf("wrong graph registration: %#v", content)
	}
	if err := s.DeactivatePackage(ctx, m.PackageID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveInvocationAlias(ctx, "example"); err == nil {
		t.Fatal("disabled package alias must disappear")
	}
	if _, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1.0.0"); err == nil {
		t.Fatal("disabled package graph must disappear")
	}
}

func TestAgentOnlyAndMixedPackagesAreFirstClassContents(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	agentOnly := packagecatalog.Manifest{PackageID: "agent/pkg", Version: "1", ContentDigest: "sha256:a", Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentAgentDefinition, ID: "researcher", Version: "1", Digest: "sha256:agent", Artifact: "agents/researcher.json"}}}
	activateFixturePackage(t, ctx, db, s, agentOnly, time.Now().UTC())
	agents, err := s.ActiveContents(ctx, packagecatalog.ContentAgentDefinition)
	if err != nil || len(agents) != 1 || agents[0].Content.ID != "researcher" {
		t.Fatalf("agent contents: %#v err=%v", agents, err)
	}
	mixed := packagecatalog.Manifest{PackageID: "mixed/pkg", Version: "1", ContentDigest: "sha256:m", Contents: []packagecatalog.ContentRef{
		{Kind: packagecatalog.ContentGraph, ID: "mixed.graph", Version: "1", Digest: "sha256:g", Artifact: "graphs/mixed.json"},
		{Kind: packagecatalog.ContentPlugin, ID: "mixed.provider", Version: "1", Digest: "sha256:p", Artifact: "plugins/provider"},
	}}
	activateFixturePackage(t, ctx, db, s, mixed, time.Now().UTC())
	plugins, err := s.ActiveContents(ctx, packagecatalog.ContentPlugin)
	if err != nil || len(plugins) != 1 || plugins[0].Content.ID != "mixed.provider" {
		t.Fatalf("plugin contents: %#v err=%v", plugins, err)
	}
}

func TestPackageCannotShadowCoreCommand(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	activateFixturePackageExpectError(t, ctx, db, New(db), fixturePackage("bad/pkg", "1", "sha256:x", "status"), time.Now().UTC(), "reserved core command")
}

func TestPackageAliasCollisionFailsWithoutChangingExistingOwner(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	now := time.Now().UTC()
	activateFixturePackage(t, ctx, db, s, fixturePackage("a/pkg", "1", "sha256:a", "shared"), now)
	activateFixturePackageExpectError(t, ctx, db, s, fixturePackage("b/pkg", "1", "sha256:b", "shared"), now, "alias collision")
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval:b/pkg:1'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM package_activation_receipts WHERE package_id='b/pkg'`, 0)
	got, err := s.ResolveInvocationAlias(ctx, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.PackageID != "a/pkg" {
		t.Fatalf("existing alias owner changed: %s", got.Contract.PackageID)
	}
}

func TestPackageUpdateAtomicallyReplacesAliasAndContentSurface(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	now := time.Now().UTC()
	activateFixturePackage(t, ctx, db, s, fixturePackage("example/pkg", "1", "sha256:1", "old"), now)
	v2 := activateFixturePackage(t, ctx, db, s, fixturePackage("example/pkg", "2", "sha256:2", "new"), now.Add(time.Second))
	if _, err := s.ResolveInvocationAlias(ctx, "old"); err == nil {
		t.Fatal("old alias must not remain active")
	}
	if _, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1"); err == nil {
		t.Fatal("old graph generation must not remain active")
	}
	got, err := s.ResolveInvocationAlias(ctx, "new")
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.PackageVersion != "2" || got.ContentDigest != v2.ContentDigest {
		t.Fatalf("new generation not active: %#v", got)
	}
}

func TestPackageActivationRequiresExactPersistedAuthorityAndIsAtomic(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC)
	manifest, request := fixtureActivationRequest(t, ctx, db, fixturePackage("research/pkg", "1", "", "research"), now)
	request.Intent.Actor = contracts.PrincipalRef{ID: "caller-selected", Kind: "user"}
	if err := store.ActivatePackage(ctx, request, now); err == nil {
		t.Fatal("caller-selected authority must not activate a package")
	}
	if _, err := store.ActivePackage(ctx, manifest.PackageID); err == nil {
		t.Fatal("denied activation must not publish an active package")
	}
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval:research/pkg:1'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM package_activation_receipts WHERE package_id='research/pkg'`, 0)
}

func TestPackageVerificationAuthorityAndRegistrationsSurviveRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	now := time.Date(2026, 9, 13, 18, 30, 0, 0, time.UTC)
	manifest, request := fixtureActivationRequest(t, ctx, db, fixturePackage("delivery/pkg", "1", "", "deliver"), now)
	wantVerificationID := request.Package.Evidence().ID
	wantIntentDigest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ActivatePackage(ctx, request, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	replayed := New(restarted)
	receipts, err := replayed.PackageActivationReceipts(ctx, manifest.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("one consumed approval must produce one durable activation receipt, got %d", len(receipts))
	}
	receipt := receipts[0]
	if receipt.Verification.ID != wantVerificationID || receipt.IntentDigest != wantIntentDigest || receipt.ApprovalID != request.ApprovalID || receipt.Authority != request.Intent.Actor {
		t.Fatalf("restart lost verification/authority lineage: %#v", receipt)
	}
	resolved, err := replayed.ResolveInvocationAlias(ctx, "deliver")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Contract.PackageID != manifest.PackageID || resolved.ContentDigest != manifest.ContentDigest {
		t.Fatalf("restart changed active client-visible generation: %#v", resolved)
	}
}

func activateFixturePackageExpectError(t *testing.T, ctx context.Context, db *sql.DB, s *Store, manifest packagecatalog.Manifest, now time.Time, label string) {
	t.Helper()
	_, request := fixtureActivationRequest(t, ctx, db, manifest, now)
	if err := s.ActivatePackage(ctx, request, now); err == nil {
		t.Fatalf("%s must fail", label)
	}
}
