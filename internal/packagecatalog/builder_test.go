package packagecatalog

import (
	"bytes"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func buildInput() PackageBuildInput {
	body := []byte("goals lifecycle contract")
	return PackageBuildInput{
		Manifest: Manifest{
			ContractVersion: ManifestContractCurrentVersion(),
			PackageID:       "praxis.package.goals",
			Version:         "0.1.0",
			Publisher:       "publisher:praxis-first-party",
			CryptoProfile:   contracts.CryptoClassicalCompatible,
			Contents: []ContentRef{{
				Kind: ContentDocumentation, ID: "goals.lifecycle", Version: "1",
				Digest: bytesDigest(body), Artifact: "contracts/goals-lifecycle.json",
			}},
		},
		Files: map[string][]byte{"contracts/goals-lifecycle.json": body},
	}
}

func TestBuildPackageIsDeterministicAndBindsArchive(t *testing.T) {
	a, err := BuildPackage(buildInput())
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildPackage(buildInput())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.ManifestBytes, b.ManifestBytes) || !bytes.Equal(a.ArtifactBytes, b.ArtifactBytes) {
		t.Fatal("same package source must produce identical manifest and archive bytes")
	}
	if a.Manifest.ContentDigest != a.ArtifactDigest || a.ManifestDigest == "" || a.ArtifactDigest == "" {
		t.Fatal("package identities are incomplete or not bound")
	}
	if _, err := VerifyPackage(VerificationInput{ManifestBytes: a.ManifestBytes, ArtifactBytes: a.ArtifactBytes, Signature: SignatureEnvelope{Version: SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: a.ManifestDigest, ArtifactDigest: a.ArtifactDigest}, SourceKind: "builder-test", SourceRef: "builder-test", VerifiedAt: time.Now().UTC()}, nil); err == nil {
		t.Fatal("unsigned package must not verify")
	}
}

func TestBuildPackageRejectsUnmanifestedAndNoncanonicalFiles(t *testing.T) {
	input := buildInput()
	input.Files["extra.txt"] = []byte("not declared")
	if _, err := BuildPackage(input); err == nil {
		t.Fatal("unmanifested package file must fail closed")
	}
	input = buildInput()
	input.Files["../escape"] = []byte("unsafe")
	if _, err := BuildPackage(input); err == nil {
		t.Fatal("noncanonical package file path must fail closed")
	}
}
