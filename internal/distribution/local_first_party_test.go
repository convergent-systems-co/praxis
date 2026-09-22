package distribution

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// writeLocalFixture writes a correctly-pinned local-first-party package
// version directory and returns its manifest/artifact bytes for the test to
// compare against.
func writeLocalFixture(t *testing.T, root, packageID, version string, artifact []byte, deps []packagecatalog.Dependency, sig *packagecatalog.SignatureEnvelope) (packagecatalog.Manifest, []byte) {
	t.Helper()
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: packageID, Version: version, ContentDigest: sha256Digest(artifact), Dependencies: deps}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, packageID, version)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.tar.gz"), artifact, 0600); err != nil {
		t.Fatal(err)
	}
	identity := localPinnedIdentity{ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "identity.json"), identityBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if sig != nil {
		sigBytes, err := json.Marshal(sig)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "signature.json"), sigBytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return manifest, manifestBytes
}

func TestLocalFirstPartyAcquiresExactQualifiedArtifact(t *testing.T) {
	root := t.TempDir()
	artifact := []byte("exact qualified goals package bytes")
	manifest, manifestBytes := writeLocalFixture(t, root, "praxis.package.goals", "0.1.5", artifact, nil, nil)

	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "praxis.package.goals"}
	release, err := adapter.Resolve(context.Background(), ref, "0.1.5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if release.Ref.Source != SourceLocalFirstParty {
		t.Fatalf("release claims source %q, want %q", release.Ref.Source, SourceLocalFirstParty)
	}
	if string(release.ManifestBytes) != string(manifestBytes) {
		t.Fatalf("manifest bytes were altered in acquisition")
	}
	if release.Manifest.ContentDigest != manifest.ContentDigest {
		t.Fatalf("manifest content digest mismatch: got %s want %s", release.Manifest.ContentDigest, manifest.ContentDigest)
	}
	got, err := adapter.FetchArtifact(context.Background(), release)
	if err != nil {
		t.Fatalf("FetchArtifact: %v", err)
	}
	if string(got) != string(artifact) {
		t.Fatalf("artifact bytes were altered in acquisition")
	}
}

func TestLocalFirstPartyRefusesTamperedArtifact(t *testing.T) {
	root := t.TempDir()
	artifact := []byte("exact qualified goals package bytes")
	writeLocalFixture(t, root, "praxis.package.goals", "0.1.5", artifact, nil, nil)
	// Tamper with the artifact bytes after the pinned identity was written.
	if err := os.WriteFile(filepath.Join(root, "praxis.package.goals", "0.1.5", "artifact.tar.gz"), []byte("tampered bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "praxis.package.goals"}
	if _, err := adapter.Resolve(context.Background(), ref, "0.1.5"); err == nil {
		t.Fatal("Resolve accepted a tampered artifact")
	}
}

func TestLocalFirstPartyRefusesTamperedManifest(t *testing.T) {
	root := t.TempDir()
	artifact := []byte("exact qualified goals package bytes")
	writeLocalFixture(t, root, "praxis.package.goals", "0.1.5", artifact, nil, nil)
	if err := os.WriteFile(filepath.Join(root, "praxis.package.goals", "0.1.5", "manifest.json"), []byte(`{"package_id":"praxis.package.goals","version":"0.1.5"}`), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "praxis.package.goals"}
	if _, err := adapter.Resolve(context.Background(), ref, "0.1.5"); err == nil {
		t.Fatal("Resolve accepted a tampered manifest")
	}
}

func TestLocalFirstPartyRefusesMissingPackage(t *testing.T) {
	adapter := LocalFirstParty{Root: t.TempDir()}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "does.not.exist"}
	if _, err := adapter.Resolve(context.Background(), ref, "1.0.0"); err == nil {
		t.Fatal("Resolve accepted a package with no local fixture")
	}
}

func TestLocalFirstPartyRefusesWrongRequestedIdentity(t *testing.T) {
	root := t.TempDir()
	artifact := []byte("exact qualified goals package bytes")
	// Fixture is pinned and named for version 0.1.5, but placed where a
	// caller might request a different version directory with the same
	// package id -- the manifest's own claimed version must match exactly.
	writeLocalFixture(t, root, "praxis.package.goals", "0.1.5", artifact, nil, nil)
	// Copy the fixture directory under a version label that does not match
	// what the manifest itself claims.
	src := filepath.Join(root, "praxis.package.goals", "0.1.5")
	dst := filepath.Join(root, "praxis.package.goals", "9.9.9")
	if err := os.MkdirAll(dst, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest.json", "artifact.tar.gz", "identity.json"} {
		body, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "praxis.package.goals"}
	if _, err := adapter.Resolve(context.Background(), ref, "9.9.9"); err == nil {
		t.Fatal("Resolve accepted a manifest whose claimed version does not match the requested identity")
	}
}

func TestLocalFirstPartyRefusesUnknownSource(t *testing.T) {
	root := t.TempDir()
	artifact := []byte("exact qualified goals package bytes")
	writeLocalFixture(t, root, "praxis.package.goals", "0.1.5", artifact, nil, nil)
	adapter := LocalFirstParty{Root: root}
	wrongRef := PackageRef{Source: SourceGitHubReleases, Owner: "local", Repo: "praxis.package.goals"}
	if _, err := adapter.Resolve(context.Background(), wrongRef, "0.1.5"); err == nil {
		t.Fatal("Resolve accepted a release ref claiming a different transport's source")
	}
	unknownRef := PackageRef{Source: "some-other-transport", Owner: "local", Repo: "praxis.package.goals"}
	if _, err := adapter.Resolve(context.Background(), unknownRef, "0.1.5"); err == nil {
		t.Fatal("Resolve accepted an entirely unrecognized source")
	}
}

func TestLocalFirstPartyCannotClaimGitHubProvenance(t *testing.T) {
	root := t.TempDir()
	artifact := []byte("exact qualified goals package bytes")
	writeLocalFixture(t, root, "praxis.package.goals", "0.1.5", artifact, nil, nil)
	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "praxis.package.goals"}
	release, err := adapter.Resolve(context.Background(), ref, "0.1.5")
	if err != nil {
		t.Fatal(err)
	}
	if release.Ref.Source == SourceGitHubReleases {
		t.Fatal("a local-first-party release must never carry the GitHub-releases source")
	}
	// goalspublication.RequiresLocalLineage and CheckAcquisition key their
	// GitHub-specific provenance check on Ref.Source == "github-releases"
	// exactly; this release must never satisfy that.
	if release.Ref.Source != SourceLocalFirstParty {
		t.Fatalf("unexpected source %q", release.Ref.Source)
	}
}

func TestLocalFirstPartyResolveLockedRefusesWrongDependencyDigest(t *testing.T) {
	root := t.TempDir()
	artifact := []byte("dependency bytes")
	writeLocalFixture(t, root, "shared/graph", "2", artifact, nil, nil)
	adapter := LocalFirstParty{Root: root}
	wrongDigest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	dep := packagecatalog.Dependency{PackageID: "shared/graph", Version: "2", Digest: wrongDigest, SourceKind: SourceLocalFirstParty, SourceRef: "shared/graph"}
	if _, err := adapter.ResolveLocked(context.Background(), dep); err == nil {
		t.Fatal("ResolveLocked accepted a dependency whose digest does not match its immutable lock")
	}
}

func TestLocalFirstPartyResolveLockedRefusesWrongSourceKind(t *testing.T) {
	adapter := LocalFirstParty{Root: t.TempDir()}
	dep := packagecatalog.Dependency{PackageID: "shared/graph", Version: "2", SourceKind: SourceGitHubReleases, SourceRef: "shared/graph"}
	if _, err := adapter.ResolveLocked(context.Background(), dep); err == nil {
		t.Fatal("ResolveLocked accepted a dependency locked to a different transport")
	}
}

func TestLocalFirstPartyCheckUpdateNeverClaimsAnUpdate(t *testing.T) {
	adapter := LocalFirstParty{Root: t.TempDir()}
	release, ok, err := adapter.CheckUpdate(context.Background(), PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "x"}, "1.0.0")
	if err != nil || ok || release.Manifest.PackageID != "" {
		t.Fatalf("CheckUpdate should be a stable no-op for a pinned local package, got (%v, %v, %v)", release, ok, err)
	}
}

// TestLocalTransportDoesNotChangeVerificationOutcome proves requirement 12:
// packagecatalog.VerifyPackage treats an equivalent, identically-signed
// package the same way regardless of whether its bytes were acquired
// through the GitHub or the local-first-party transport -- only the
// recorded SourceKind/SourceRef evidence metadata differs.
func TestLocalTransportDoesNotChangeVerificationOutcome(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("equivalent package material")
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "equivalence/check", Version: "1", ContentDigest: sha256Digest(artifact)}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "catalog-publisher"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	verifiers := []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"catalog-publisher": public}}}
	at := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	githubVerified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope, SourceKind: SourceGitHubReleases, SourceRef: "owner/repo@1", VerifiedAt: at}, verifiers)
	if err != nil {
		t.Fatalf("github-sourced verification: %v", err)
	}
	localVerified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope, SourceKind: SourceLocalFirstParty, SourceRef: "equivalence/check@1", VerifiedAt: at}, verifiers)
	if err != nil {
		t.Fatalf("local-sourced verification: %v", err)
	}
	if githubVerified.Manifest().ContentDigest != localVerified.Manifest().ContentDigest || githubVerified.Evidence().ArtifactDigest != localVerified.Evidence().ArtifactDigest || githubVerified.Evidence().PackageID != localVerified.Evidence().PackageID {
		t.Fatal("transport choice changed the verified package identity")
	}
	if githubVerified.Evidence().SourceKind == localVerified.Evidence().SourceKind {
		t.Fatal("expected the two verifications to record their own distinct transport provenance")
	}
}

// TestLocalUnsignedPackageStillFailsVerification proves the local transport
// adds no trust of its own: an unsigned local-first-party package -- exactly
// the shape of this bootstrap's own Review-10-qualified Goals package before
// any publisher signs it -- is refused by the ordinary verification pipeline
// exactly as an unsigned package from any other transport would be.
func TestLocalUnsignedPackageStillFailsVerification(t *testing.T) {
	artifact := []byte("unsigned local package bytes")
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "praxis.package.goals", Version: "0.1.5", ContentDigest: sha256Digest(artifact)}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, SourceKind: SourceLocalFirstParty, SourceRef: "praxis.package.goals@0.1.5", VerifiedAt: time.Now().UTC()}, nil)
	if err == nil {
		t.Fatal("an unsigned local package must be refused by VerifyPackage, exactly like any other unsigned package")
	}
}

// TestLocalInvalidSignatureStillFailsVerification proves a local package
// signed with a key the verifier does not trust is refused exactly as it
// would be from any other transport.
func TestLocalInvalidSignatureStillFailsVerification(t *testing.T) {
	_, wrongSigner, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trustedPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("locally acquired but untrusted-key-signed bytes")
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "praxis.package.goals", Version: "0.1.5", ContentDigest: sha256Digest(artifact)}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "untrusted-key"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(wrongSigner, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	verifiers := []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"catalog-publisher": trustedPublic}}}
	_, err = packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope, SourceKind: SourceLocalFirstParty, SourceRef: "praxis.package.goals@0.1.5", VerifiedAt: time.Now().UTC()}, verifiers)
	if err == nil {
		t.Fatal("a local package signed by an untrusted key must be refused, exactly like any other transport")
	}
}

// TestLocalFirstPartyRefusesPathTraversalEvenWithMatchingManifestID
// reproduces the exact counterexample independent Review 1 found: the
// identity-binding check alone (manifest.PackageID == ref.Repo) does not
// stop traversal, because packagecatalog.Manifest.Validate places no
// character restriction on PackageID -- a manifest can declare a PackageID
// that is itself the traversal string, matching whatever ref.Repo the
// caller supplies. This proves versionDir's own containment check refuses
// it regardless of what the manifest claims.
func TestLocalFirstPartyRefusesPathTraversalEvenWithMatchingManifestID(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "configured-root")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	// The traversal target sits as a sibling of root, entirely outside it.
	traversal := "../outside-root/victim"
	victimDir := filepath.Join(base, "outside-root", "victim", "1")
	if err := os.MkdirAll(victimDir, 0700); err != nil {
		t.Fatal(err)
	}
	artifact := []byte("bytes planted outside the configured root")
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: traversal, Version: "1", ContentDigest: sha256Digest(artifact)}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victimDir, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victimDir, "artifact.tar.gz"), artifact, 0600); err != nil {
		t.Fatal(err)
	}
	identity := localPinnedIdentity{ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victimDir, "identity.json"), identityBytes, 0600); err != nil {
		t.Fatal(err)
	}
	// filepath.Join(root, traversal, "1") resolves (after Clean) to exactly
	// victimDir -- confirm the test's own geometry before asserting on the
	// adapter, so a future refactor of this test cannot silently stop
	// testing what it claims to.
	if got := filepath.Join(root, traversal, "1"); got != victimDir {
		t.Fatalf("test setup error: filepath.Join(root, traversal, \"1\") = %q, want %q", got, victimDir)
	}
	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: traversal}
	if _, err := adapter.Resolve(context.Background(), ref, "1"); err == nil {
		t.Fatal("Resolve escaped the configured Root via a manifest.PackageID that matches a traversal-shaped ref.Repo")
	}
	if _, err := adapter.Info(context.Background(), ref, "1"); err == nil {
		t.Fatal("Info escaped the configured Root via a manifest.PackageID that matches a traversal-shaped ref.Repo")
	}
	dep := packagecatalog.Dependency{PackageID: traversal, Version: "1", Digest: manifest.ContentDigest, SourceKind: SourceLocalFirstParty, SourceRef: traversal}
	if _, err := adapter.ResolveLocked(context.Background(), dep); err == nil {
		t.Fatal("ResolveLocked escaped the configured Root via a traversal-shaped dependency PackageID")
	}
}

// --- Astra Review 2, finding L2: symlink-based containment escapes. ---
//
// Each test below plants a self-consistent, correctly-pinned package
// fixture entirely OUTSIDE a configured Root, then makes it reachable from
// inside Root only through a symlink at one specific point in the chain
// (package directory, version directory, identity.json, manifest.json, or
// artifact.tar.gz), matching Review 2's exact reproduction shape. Every one
// must be refused; a bare identity/digest match is not sufficient evidence
// of containment, since the whole point of this class of test is that the
// lexical path can look contained while the filesystem object is not.

func writeFixtureFilesAt(t *testing.T, dir string, artifact []byte, packageID, version string) {
	t.Helper()
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: packageID, Version: version, ContentDigest: sha256Digest(artifact)}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.tar.gz"), artifact, 0600); err != nil {
		t.Fatal(err)
	}
	identity := localPinnedIdentity{ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "identity.json"), identityBytes, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertLocalAcquisitionRefused(t *testing.T, root, repo, version string) {
	t.Helper()
	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: repo}
	if _, err := adapter.Resolve(context.Background(), ref, version); err == nil {
		t.Fatalf("Resolve(%q,%q) escaped Root through a symlink and must have been refused", repo, version)
	}
	if _, err := adapter.Info(context.Background(), ref, version); err == nil {
		t.Fatalf("Info(%q,%q) escaped Root through a symlink and must have been refused", repo, version)
	}
	dep := packagecatalog.Dependency{PackageID: repo, Version: version, SourceKind: SourceLocalFirstParty, SourceRef: repo}
	if _, err := adapter.ResolveLocked(context.Background(), dep); err == nil {
		t.Fatalf("ResolveLocked(%q,%q) escaped Root through a symlink and must have been refused", repo, version)
	}
}

func TestLocalFirstPartyRefusesPackageDirectorySymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFixtureFilesAt(t, filepath.Join(outside, "alias", "pkg", "1"), []byte("outside bytes via package-dir symlink"), "alias/pkg", "1")
	if err := os.Symlink(filepath.Join(outside, "alias"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	assertLocalAcquisitionRefused(t, root, "alias/pkg", "1")
}

func TestLocalFirstPartyRefusesVersionDirectorySymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFixtureFilesAt(t, filepath.Join(outside, "evil-version"), []byte("outside bytes via version-dir symlink"), "pkg", "1")
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "evil-version"), filepath.Join(root, "pkg", "1")); err != nil {
		t.Fatal(err)
	}
	assertLocalAcquisitionRefused(t, root, "pkg", "1")
}

func TestLocalFirstPartyRefusesIntermediateSymlinkComponent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFixtureFilesAt(t, filepath.Join(outside, "ns-target", "pkg", "1"), []byte("outside bytes via intermediate namespace symlink"), "ns/pkg", "1")
	if err := os.Symlink(filepath.Join(outside, "ns-target"), filepath.Join(root, "ns")); err != nil {
		t.Fatal(err)
	}
	assertLocalAcquisitionRefused(t, root, "ns/pkg", "1")
}

func TestLocalFirstPartyRefusesIdentityFileSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dir := filepath.Join(root, "pkg", "1")
	writeFixtureFilesAt(t, dir, []byte("legitimate bytes"), "pkg", "1")
	// Plant a different identity.json outside and replace the real one with
	// a symlink to it.
	outsideIdentity := filepath.Join(outside, "identity.json")
	if err := os.WriteFile(outsideIdentity, []byte(`{"manifest_digest":"sha256:00","artifact_digest":"sha256:00"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "identity.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideIdentity, filepath.Join(dir, "identity.json")); err != nil {
		t.Fatal(err)
	}
	assertLocalAcquisitionRefused(t, root, "pkg", "1")
}

func TestLocalFirstPartyRefusesManifestSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dir := filepath.Join(root, "pkg", "1")
	writeFixtureFilesAt(t, dir, []byte("legitimate bytes"), "pkg", "1")
	outsideManifest := filepath.Join(outside, "manifest.json")
	if err := os.WriteFile(outsideManifest, []byte(`{"package_id":"pkg","version":"1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideManifest, filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	assertLocalAcquisitionRefused(t, root, "pkg", "1")
}

func TestLocalFirstPartyRefusesArchiveSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dir := filepath.Join(root, "pkg", "1")
	writeFixtureFilesAt(t, dir, []byte("legitimate bytes"), "pkg", "1")
	outsideArchive := filepath.Join(outside, "artifact.tar.gz")
	if err := os.WriteFile(outsideArchive, []byte("different outside bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "artifact.tar.gz")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideArchive, filepath.Join(dir, "artifact.tar.gz")); err != nil {
		t.Fatal(err)
	}
	assertLocalAcquisitionRefused(t, root, "pkg", "1")
}

// TestLocalFirstPartyAllowsRootItselfToBeASymlink establishes the explicit
// policy for Root itself (as opposed to anything below it): Root is
// operator-supplied configuration, not attacker-influenced input, so it may
// be a symlink to the operator's real package directory. Ordinary
// acquisition of a legitimate package placed entirely beneath the real
// directory that Root points at must still succeed.
func TestLocalFirstPartyAllowsRootItselfToBeASymlink(t *testing.T) {
	real := t.TempDir()
	artifact := []byte("exact qualified goals package bytes")
	writeFixtureFilesAt(t, filepath.Join(real, "praxis.package.goals", "0.1.5"), artifact, "praxis.package.goals", "0.1.5")
	parent := t.TempDir()
	rootLink := filepath.Join(parent, "root-symlink")
	if err := os.Symlink(real, rootLink); err != nil {
		t.Fatal(err)
	}
	adapter := LocalFirstParty{Root: rootLink}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "praxis.package.goals"}
	release, err := adapter.Resolve(context.Background(), ref, "0.1.5")
	if err != nil {
		t.Fatalf("a legitimate package under a symlinked Root must still be acquirable: %v", err)
	}
	got, err := adapter.FetchArtifact(context.Background(), release)
	if err != nil || string(got) != string(artifact) {
		t.Fatalf("FetchArtifact through a symlinked Root: got=(%q,%v)", got, err)
	}
}

// TestLocalFirstPartyResolveRaceCannotObserveExternalBytesAcrossSwaps
// reproduces Astra Review 3's finding L3 shape (a component checked as safe
// is swapped for an escaping symlink before the bytes are actually read)
// against the os.Root-based implementation. Unlike the prior
// check-then-open code, there is no separate check to interleave against:
// every read is a single os.Root call. This test cannot prove a negative
// for all time, but it races many concurrent resolutions against a
// background goroutine continuously alternating a path component between a
// symlink that stays within Root (safe, and must be followed) and one that
// escapes it (must always be refused) and asserts that no successful read,
// across thousands of iterations under concurrent mutation, ever observes
// the external bytes.
func TestLocalFirstPartyResolveRaceCannotObserveExternalBytesAcrossSwaps(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	insideArtifact := []byte("inside-bytes")
	outsideArtifact := []byte("OUTSIDE-BYTES-MUST-NEVER-BE-OBSERVED")
	// A real, permanent directory INSIDE root: a symlink to this is a safe,
	// non-escaping reference os.Root must be willing to follow.
	writeFixtureFilesAt(t, filepath.Join(root, "pivot-real", "pkg", "1"), insideArtifact, "pivot/pkg", "1")
	// A real, permanent directory OUTSIDE root: a symlink to this must
	// always be refused, no matter when it is observed.
	writeFixtureFilesAt(t, filepath.Join(outside, "pivot-outside", "pkg", "1"), outsideArtifact, "pivot/pkg", "1")

	pivot := filepath.Join(root, "pivot")
	safeTarget := filepath.Join(root, "pivot-real")
	escapingTarget := filepath.Join(outside, "pivot-outside")

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		toggle := false
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(pivot)
			if toggle {
				_ = os.Symlink(safeTarget, pivot)
			} else {
				_ = os.Symlink(escapingTarget, pivot)
			}
			toggle = !toggle
		}
	}()

	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "pivot/pkg"}
	const iterations = 4000
	for i := 0; i < iterations; i++ {
		release, err := adapter.Resolve(context.Background(), ref, "1")
		if err != nil {
			continue // pivot mid-swap (removed) or correctly refused -- both fine
		}
		got, err := adapter.FetchArtifact(context.Background(), release)
		if err != nil {
			continue // refused between Resolve and FetchArtifact -- fine, not the failure mode under test
		}
		if string(got) == string(outsideArtifact) {
			close(stop)
			wg.Wait()
			t.Fatalf("iteration %d: FetchArtifact returned external bytes acquired through a racing symlink swap", i)
		}
	}
	close(stop)
	wg.Wait()
}
