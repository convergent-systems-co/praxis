package packagecatalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func verifiedFixture(t *testing.T, manifest Manifest, artifact []byte, dependencies map[string]VerifiedPackage) VerifiedPackage {
	t.Helper()
	input, verifier := signedVerificationInput(t, manifest, artifact)
	input.ResolvedDependencies = dependencies
	verified, err := VerifyPackage(input, []SignatureVerifier{verifier})
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

func signedVerificationInput(t *testing.T, manifest Manifest, artifact []byte) (VerificationInput, Ed25519Verifier) {
	t.Helper()
	manifest.ContentDigest = bytesDigest(artifact)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	envelope := SignatureEnvelope{
		Version: SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible,
		ManifestDigest: bytesDigest(manifestBytes), ArtifactDigest: manifest.ContentDigest,
	}
	proof := SignatureProof{Algorithm: SignatureAlgorithmEd25519, KeyID: "publisher-key"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []SignatureProof{proof}
	input := VerificationInput{
		ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope,
		SourceKind: "fixture-catalog", SourceRef: manifest.PackageID + "@" + manifest.Version,
		VerifiedAt: time.Date(2026, 9, 13, 17, 0, 0, 0, time.UTC),
	}
	return input, Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"publisher-key": pub}}
}

func testBundle(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"graphs/research.json", "plugins/provider"} {
		body, ok := files[name]
		if !ok {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestVerifyPackageBindsBytesSignatureDependencyAndCapabilities(t *testing.T) {
	dependency := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "research/sources", Version: "1", Capabilities: []string{"network.read"}}, []byte("research-sources"), nil)
	root := verifiedFixture(t, Manifest{
		ContractVersion: ManifestContractCurrentVersion(),
		PackageID:       "delivery/graph", Version: "2", Publisher: "publisher",
		Capabilities: []string{"workspace.read"}, RequiredEnforcement: []string{"network.egress.filtered"},
		Dependencies: []Dependency{{PackageID: "research/sources", Version: "1", Digest: dependency.Manifest().ContentDigest}},
	}, []byte("delivery-graph"), map[string]VerifiedPackage{"research/sources": dependency})
	if err := root.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, want := root.Evidence().EffectiveCapabilities, []string{"network.read", "workspace.read"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("transitive capability evidence mismatch: got %v want %v", got, want)
	}
	if got := root.Evidence().DependencyEvidenceIDs; len(got) != 1 || got[0] != dependency.Evidence().ID {
		t.Fatalf("dependency verification lineage missing: %v", got)
	}
}

func TestBarePackageCannotMintVerificationAndTamperingFails(t *testing.T) {
	if err := (VerifiedPackage{}).Validate(); err == nil {
		t.Fatal("zero-value package must not mint verification")
	}
	manifest := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "research/pkg", Version: "1", ContentDigest: bytesDigest([]byte("expected"))}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	envelope, verifier := signedEd25519Envelope(t, contracts.CryptoClassicalCompatible)
	envelope.ManifestDigest = bytesDigest(manifestBytes)
	envelope.ArtifactDigest = manifest.ContentDigest
	// The fixture signature was made over different digests. Matching caller
	// fields cannot substitute for a proof over the immutable bytes.
	if _, err := VerifyPackage(VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: []byte("expected"), Signature: envelope, SourceKind: "test", SourceRef: "research/pkg@1", VerifiedAt: time.Now().UTC()}, []SignatureVerifier{verifier}); err == nil {
		t.Fatal("tampered signature binding must fail")
	}
}

func TestVerifiedPackageAccessorsCannotMutateSealedEvidence(t *testing.T) {
	verified := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "research/pkg", Version: "1", Capabilities: []string{"network.read"}}, []byte("research"), nil)
	manifest := verified.Manifest()
	manifest.Capabilities[0] = "network.write"
	evidence := verified.Evidence()
	evidence.EffectiveCapabilities[0] = "filesystem.write"
	signature := verified.Signature()
	signature.Proofs[0].KeyID = "attacker"
	if err := verified.Validate(); err != nil {
		t.Fatalf("returned views mutated sealed package: %v", err)
	}
	if got := verified.Manifest().Capabilities[0]; got != "network.read" {
		t.Fatalf("manifest capability mutated through accessor: %s", got)
	}
	if got := verified.Evidence().EffectiveCapabilities[0]; got != "network.read" {
		t.Fatalf("verification evidence mutated through accessor: %s", got)
	}
	if got := verified.Signature().Proofs[0].KeyID; got != "publisher-key" {
		t.Fatalf("signature mutated through accessor: %s", got)
	}
}

func TestVerifyPackageBindsTypedContentToExactArchiveBytes(t *testing.T) {
	graph := []byte(`{"ID":"research.graph","Version":"1"}`)
	plugin := []byte("executable-provider")
	artifact := testBundle(t, map[string][]byte{"graphs/research.json": graph, "plugins/provider": plugin})
	graphRef := ContentRef{Kind: ContentGraph, ID: "research.graph", Version: "1", Digest: bytesDigest(graph), Artifact: "graphs/research.json"}
	manifest := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "mixed/research", Version: "1", Contents: []ContentRef{
		graphRef,
		{Kind: ContentPlugin, ID: "research.provider", Version: "1", Digest: bytesDigest(plugin), Artifact: "plugins/provider"},
	}}
	verified := verifiedFixture(t, manifest, artifact, nil)
	got, err := verified.ContentBytes(graphRef)
	if err != nil || !bytes.Equal(got, graph) {
		t.Fatalf("verified graph bytes not retained: %q %v", got, err)
	}
	got[0] ^= 0xff
	if err := verified.Validate(); err != nil {
		t.Fatalf("returned content view mutated sealed package: %v", err)
	}

	wrong := manifest
	wrong.Contents = append([]ContentRef(nil), manifest.Contents...)
	for i := range wrong.Contents {
		if wrong.Contents[i].Kind == graphRef.Kind && wrong.Contents[i].ID == graphRef.ID && wrong.Contents[i].Version == graphRef.Version {
			wrong.Contents[i].Digest = bytesDigest([]byte("other graph"))
		}
	}
	input, verifier := signedVerificationInput(t, wrong, artifact)
	if _, err := VerifyPackage(input, []SignatureVerifier{verifier}); err == nil {
		t.Fatal("valid signature over a false content digest must not verify the package")
	}

	missing := testBundle(t, map[string][]byte{"graphs/research.json": graph})
	input, verifier = signedVerificationInput(t, manifest, missing)
	if _, err := VerifyPackage(input, []SignatureVerifier{verifier}); err == nil {
		t.Fatal("signed archive missing a manifest-declared plugin must fail closed")
	}
}

func TestUnverifiedDependencyCannotSatisfyImmutableLock(t *testing.T) {
	manifest := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "root", Version: "1", Dependencies: []Dependency{{PackageID: "dep", Version: "1", Digest: bytesDigest([]byte("dep"))}}}
	manifest.ContentDigest = bytesDigest([]byte("root"))
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	envelope := SignatureEnvelope{Version: SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: bytesDigest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	proof := SignatureProof{Algorithm: SignatureAlgorithmEd25519, KeyID: "publisher"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []SignatureProof{proof}
	_, err = VerifyPackage(VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: []byte("root"), Signature: envelope, ResolvedDependencies: map[string]VerifiedPackage{"dep": {}}, SourceKind: "test", SourceRef: "root@1", VerifiedAt: time.Now().UTC()}, []SignatureVerifier{Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"publisher": pub}}})
	if err == nil {
		t.Fatal("an unverified dependency must not satisfy the dependency lock")
	}
}
