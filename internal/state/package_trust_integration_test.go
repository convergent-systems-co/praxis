package state

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type trustCatalogAdapter struct {
	releases  map[string]distribution.Release
	artifacts map[string][]byte
}

func (a trustCatalogAdapter) ResolveLocked(_ context.Context, dependency packagecatalog.Dependency) (distribution.Release, error) {
	release, ok := a.releases[dependency.SourceRef]
	if !ok {
		return distribution.Release{}, errors.New("locked release is unavailable")
	}
	return release, nil
}

func (a trustCatalogAdapter) FetchArtifact(_ context.Context, release distribution.Release) ([]byte, error) {
	body, ok := a.artifacts[release.Manifest.PackageID]
	if !ok {
		return nil, errors.New("release artifact is unavailable")
	}
	return append([]byte(nil), body...), nil
}

func signedTrustRelease(t *testing.T, private ed25519.PrivateKey, ref distribution.PackageRef, manifest packagecatalog.Manifest, artifact []byte) distribution.Release {
	t.Helper()
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	envelope := packagecatalog.SignatureEnvelope{
		Version:        packagecatalog.SignatureEnvelopeCurrentVersion(),
		Profile:        contracts.CryptoClassicalCompatible,
		ManifestDigest: digestPackageBytes(manifestBytes),
		ArtifactDigest: digestPackageBytes(artifact),
	}
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "catalog-publisher"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	return distribution.Release{Ref: ref, Tag: manifest.Version, Manifest: manifest, ManifestBytes: manifestBytes, ManifestDigest: envelope.ManifestDigest, Signature: envelope}
}

func TestDistributedPackageTrustBindsCatalogResolutionReviewAndLocalAuthorityAcrossRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	dependencyManifest := fixturePackage("research/evidence", "1", "", "evidence")
	dependencyManifest.Publisher = "research-catalog"
	dependencyManifest.Capabilities = []string{"evidence.read"}
	dependencyArtifact := fixturePackageArtifact(t, &dependencyManifest)
	dependencyRef := distribution.PackageRef{Source: "organization-catalog", Owner: "research", Repo: "evidence"}
	dependencyRelease := signedTrustRelease(t, private, dependencyRef, dependencyManifest, dependencyArtifact)

	rootManifest := fixturePackage("delivery/verified", "2", "", "deliver")
	rootManifest.Publisher = "delivery-catalog"
	rootManifest.Capabilities = []string{"run.execute"}
	rootManifest.RequiredEnforcement = []string{"policy"}
	rootManifest.Dependencies = []packagecatalog.Dependency{{PackageID: dependencyManifest.PackageID, Version: dependencyManifest.Version, Digest: dependencyManifest.ContentDigest, SourceKind: dependencyRef.Source, SourceRef: dependencyRef.String()}}
	rootArtifact := fixturePackageArtifact(t, &rootManifest)
	rootRef := distribution.PackageRef{Source: "public-catalog", Owner: "delivery", Repo: "verified"}
	rootRelease := signedTrustRelease(t, private, rootRef, rootManifest, rootArtifact)

	adapter := trustCatalogAdapter{releases: map[string]distribution.Release{dependencyRef.String(): dependencyRelease}, artifacts: map[string][]byte{dependencyManifest.PackageID: dependencyArtifact}}
	resolution, err := (distribution.Resolver{
		Sources:    map[string]distribution.LockedAdapter{dependencyRef.Source: adapter},
		Verifiers:  []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"catalog-publisher": public}}},
		VerifiedAt: now,
	}).Resolve(ctx, rootRelease, rootArtifact)
	if err != nil {
		t.Fatal(err)
	}
	wantCapabilities := []string{"evidence.read", "run.execute"}
	rootEvidence := resolution.Root.Evidence()
	if !reflect.DeepEqual(rootEvidence.EffectiveCapabilities, wantCapabilities) || rootEvidence.Publisher != rootManifest.Publisher || rootEvidence.SourceKind != rootRef.Source || rootEvidence.SourceRef != rootRef.String() || !reflect.DeepEqual(rootEvidence.SignerKeyIDs, []string{"catalog-publisher"}) {
		t.Fatalf("resolution lost immutable provenance/signature/capability evidence: %+v", rootEvidence)
	}
	review := packagecatalog.ReviewUpdate(nil, rootEvidence.EffectiveCapabilities, len(rootEvidence.RequiredEnforcement) != 0, false)
	if !review.RequiresReauthorization || !reflect.DeepEqual(review.AddedCapabilities, wantCapabilities) || !review.EnforcementChanged {
		t.Fatalf("transitive capability/enforcement review was not explicit: %+v", review)
	}

	actor := contracts.PrincipalRef{ID: "local-package-governor", Kind: "user"}
	request, err := packagecatalog.NewDeploymentRequest(resolution.Root, resolution.Dependencies, actor, "approval:distributed-trust")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	if err := store.DeployPackages(ctx, request, now.Add(time.Minute)); err == nil {
		t.Fatal("catalog discovery, signatures, and review minted local activation authority")
	}
	if _, err := store.ActivePackage(ctx, rootManifest.PackageID); err == nil {
		t.Fatal("denied distributed package became active")
	}
	intentDigest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, request.ApprovalID, actor.ID, actor.Kind, intentDigest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := store.DeployPackages(ctx, request, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	replayed := New(db)
	for _, identity := range []packagecatalog.Manifest{dependencyManifest, rootManifest} {
		active, err := replayed.ActivePackage(ctx, identity.PackageID)
		if err != nil || active.Manifest.ContentDigest != identity.ContentDigest {
			t.Fatalf("verified dependency closure did not survive restart: package=%s active=%+v err=%v", identity.PackageID, active, err)
		}
	}
	receipts, err := replayed.PackageActivationReceipts(ctx, rootManifest.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("one local authority consumption must retain one root activation receipt, got %d", len(receipts))
	}
	receipt := receipts[0]
	if receipt.Authority != actor || receipt.ApprovalID != request.ApprovalID || receipt.Verification.ID != rootEvidence.ID || receipt.Verification.SourceRef != rootRef.String() || !reflect.DeepEqual(receipt.Verification.EffectiveCapabilities, wantCapabilities) {
		t.Fatalf("restart lost explicit local trust or distributed verification lineage: %+v", receipt)
	}
}
