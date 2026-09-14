package distribution

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fixtureLockedAdapter struct {
	releases  map[string]Release
	artifacts map[string][]byte
}

func (f fixtureLockedAdapter) ResolveLocked(_ context.Context, dependency packagecatalog.Dependency) (Release, error) {
	release, ok := f.releases[dependency.SourceRef+"@"+dependency.Version]
	if !ok {
		return Release{}, errors.New("fixture release not found")
	}
	return release, nil
}

func (f fixtureLockedAdapter) FetchArtifact(_ context.Context, release Release) ([]byte, error) {
	body, ok := f.artifacts[release.Manifest.PackageID+"@"+release.Manifest.Version]
	if !ok {
		return nil, errors.New("fixture artifact not found")
	}
	return append([]byte(nil), body...), nil
}

func signedRelease(t *testing.T, private ed25519.PrivateKey, source, ref, id, version string, artifact []byte, capabilities []string, dependencies []packagecatalog.Dependency) Release {
	t.Helper()
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: id, Version: version, ContentDigest: sha256Digest(artifact), Capabilities: capabilities, Dependencies: dependencies}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "catalog-publisher"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	return Release{Ref: PackageRef{Source: source, Owner: "fixture", Repo: ref}, Tag: version, Manifest: manifest, ManifestBytes: manifestBytes, Signature: envelope}
}

func TestResolverVerifiesImmutableTransitiveLocksAcrossCatalogTransports(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	evidenceArtifact := []byte("research evidence package")
	evidence := signedRelease(t, private, "private-catalog", "research-evidence", "research/evidence", "3", evidenceArtifact, []string{"research.read"}, nil)
	sharedArtifact := []byte("shared graph package")
	shared := signedRelease(t, private, "public-catalog", "shared-graph", "shared/graph", "2", sharedArtifact, []string{"graph.execute"}, []packagecatalog.Dependency{{PackageID: "research/evidence", Version: "3", Digest: evidence.Manifest.ContentDigest, SourceKind: "private-catalog", SourceRef: "research-evidence"}})
	toolArtifact := []byte("delivery tool package")
	tool := signedRelease(t, private, "public-catalog", "delivery-tool", "delivery/tool", "5", toolArtifact, []string{"workspace.read"}, nil)
	rootArtifact := []byte("delivery root package")
	root := signedRelease(t, private, "public-catalog", "delivery-root", "delivery/root", "7", rootArtifact, []string{"run.execute"}, []packagecatalog.Dependency{
		{PackageID: "shared/graph", Version: "2", Digest: shared.Manifest.ContentDigest, SourceKind: "public-catalog", SourceRef: "shared-graph"},
		{PackageID: "delivery/tool", Version: "5", Digest: tool.Manifest.ContentDigest, SourceKind: "public-catalog", SourceRef: "delivery-tool"},
	})
	publicCatalog := fixtureLockedAdapter{releases: map[string]Release{"shared-graph@2": shared, "delivery-tool@5": tool}, artifacts: map[string][]byte{"shared/graph@2": sharedArtifact, "delivery/tool@5": toolArtifact}}
	privateCatalog := fixtureLockedAdapter{releases: map[string]Release{"research-evidence@3": evidence}, artifacts: map[string][]byte{"research/evidence@3": evidenceArtifact}}
	resolver := Resolver{Sources: map[string]LockedAdapter{"public-catalog": publicCatalog, "private-catalog": privateCatalog}, Verifiers: []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"catalog-publisher": public}}}, VerifiedAt: time.Date(2026, 9, 13, 21, 0, 0, 0, time.UTC)}

	resolution, err := resolver.Resolve(context.Background(), root, rootArtifact)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := map[string]bool{}
	positions := map[string]int{}
	for index, identity := range resolution.Order {
		gotIDs[identity.PackageID] = identity.VerificationID != "" && identity.ContentDigest != ""
		positions[identity.PackageID] = index
	}
	wantIDs := map[string]bool{"research/evidence": true, "shared/graph": true, "delivery/tool": true, "delivery/root": true}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("resolution did not retain the closed semantic identity set: got=%v want=%v", gotIDs, wantIDs)
	}
	if positions["research/evidence"] >= positions["shared/graph"] || positions["shared/graph"] >= positions["delivery/root"] || positions["delivery/tool"] >= positions["delivery/root"] {
		t.Fatalf("dependency-first order violated: %v", positions)
	}
	wantCapabilities := []string{"graph.execute", "research.read", "run.execute", "workspace.read"}
	if got := resolution.Root.Evidence().EffectiveCapabilities; !reflect.DeepEqual(got, wantCapabilities) {
		t.Fatalf("transitive capability evidence mismatch: got=%v want=%v", got, wantCapabilities)
	}
}

func TestResolverRejectsLockMismatchCycleAndUnavailableTransport(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifactA, artifactB := []byte("a"), []byte("b")
	a := signedRelease(t, private, "catalog", "a", "pkg/a", "1", artifactA, nil, []packagecatalog.Dependency{{PackageID: "pkg/b", Version: "1", Digest: sha256Digest(artifactB), SourceKind: "catalog", SourceRef: "b"}})
	b := signedRelease(t, private, "catalog", "b", "pkg/b", "1", artifactB, nil, []packagecatalog.Dependency{{PackageID: "pkg/a", Version: "1", Digest: sha256Digest(artifactA), SourceKind: "catalog", SourceRef: "a"}})
	adapter := fixtureLockedAdapter{releases: map[string]Release{"a@1": a, "b@1": b}, artifacts: map[string][]byte{"pkg/a@1": artifactA, "pkg/b@1": artifactB}}
	resolver := Resolver{Sources: map[string]LockedAdapter{"catalog": adapter}, Verifiers: []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"catalog-publisher": public}}}, VerifiedAt: time.Now().UTC()}
	if _, err := resolver.Resolve(context.Background(), a, artifactA); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("dependency cycle did not fail deterministically: %v", err)
	}

	badRoot := signedRelease(t, private, "catalog", "root", "pkg/root", "1", []byte("root"), nil, []packagecatalog.Dependency{{PackageID: "pkg/b", Version: "1", Digest: "sha256:not-the-artifact", SourceKind: "catalog", SourceRef: "b"}})
	if _, err := resolver.Resolve(context.Background(), badRoot, []byte("root")); err == nil || !strings.Contains(err.Error(), "immutable lock") {
		t.Fatalf("digest mismatch did not fail at the lock boundary: %v", err)
	}
	missingTransport := signedRelease(t, private, "catalog", "root", "pkg/root", "1", []byte("root"), nil, []packagecatalog.Dependency{{PackageID: "pkg/b", Version: "1", Digest: b.Manifest.ContentDigest, SourceKind: "absent", SourceRef: "b"}})
	if _, err := resolver.Resolve(context.Background(), missingTransport, []byte("root")); err == nil || !strings.Contains(err.Error(), "unavailable source") {
		t.Fatalf("unavailable dependency transport did not fail closed: %v", err)
	}

	x1Artifact, x2Artifact := []byte("x-one"), []byte("x-two")
	x1 := signedRelease(t, private, "catalog", "x-one", "shared/x", "1", x1Artifact, nil, nil)
	x2 := signedRelease(t, private, "catalog", "x-two", "shared/x", "2", x2Artifact, nil, nil)
	leftArtifact, rightArtifact := []byte("left"), []byte("right")
	left := signedRelease(t, private, "catalog", "left", "branch/left", "1", leftArtifact, nil, []packagecatalog.Dependency{{PackageID: "shared/x", Version: "1", Digest: x1.Manifest.ContentDigest, SourceKind: "catalog", SourceRef: "x-one"}})
	right := signedRelease(t, private, "catalog", "right", "branch/right", "1", rightArtifact, nil, []packagecatalog.Dependency{{PackageID: "shared/x", Version: "2", Digest: x2.Manifest.ContentDigest, SourceKind: "catalog", SourceRef: "x-two"}})
	conflictRootArtifact := []byte("conflict-root")
	conflictRoot := signedRelease(t, private, "catalog", "conflict", "pkg/conflict", "1", conflictRootArtifact, nil, []packagecatalog.Dependency{
		{PackageID: "branch/left", Version: "1", Digest: left.Manifest.ContentDigest, SourceKind: "catalog", SourceRef: "left"},
		{PackageID: "branch/right", Version: "1", Digest: right.Manifest.ContentDigest, SourceKind: "catalog", SourceRef: "right"},
	})
	conflictAdapter := fixtureLockedAdapter{releases: map[string]Release{"left@1": left, "right@1": right, "x-one@1": x1, "x-two@2": x2}, artifacts: map[string][]byte{"branch/left@1": leftArtifact, "branch/right@1": rightArtifact, "shared/x@1": x1Artifact, "shared/x@2": x2Artifact}}
	resolver.Sources["catalog"] = conflictAdapter
	if _, err := resolver.Resolve(context.Background(), conflictRoot, conflictRootArtifact); err == nil || !strings.Contains(err.Error(), "conflicting dependency locks") {
		t.Fatalf("conflicting transitive identities did not fail closed: %v", err)
	}
}
