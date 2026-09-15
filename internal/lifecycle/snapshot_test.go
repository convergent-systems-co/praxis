package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func artifactDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func testSnapshotManifest(t *testing.T, embedded, external []byte) contracts.LifecycleSnapshotManifest {
	t.Helper()
	entries := []contracts.LifecycleSnapshotEntry{{Component: contracts.LifecycleComponentRef{Class: contracts.LifecycleBinary, ID: "binary", Version: "1", Digest: artifactDigest([]byte("binary-component"))}, Artifact: contracts.LifecycleArtifactRef{ID: "binary-artifact", Version: "1", Digest: artifactDigest(embedded), MediaType: "application/octet-stream", Size: int64(len(embedded)), ProvenanceDigest: artifactDigest([]byte("binary-provenance")), Retention: "required", AvailabilityEvidence: "verified"}}}
	if external != nil {
		entries = append(entries, contracts.LifecycleSnapshotEntry{Component: contracts.LifecycleComponentRef{Class: contracts.LifecycleSchema, ID: "schema", Version: "1", Digest: artifactDigest([]byte("schema-component"))}, Artifact: contracts.LifecycleArtifactRef{ID: "schema-artifact", Version: "1", Digest: artifactDigest(external), MediaType: "application/octet-stream", Size: int64(len(external)), Provider: "fixture-store", Reference: "fixture://schema", ProvenanceDigest: artifactDigest([]byte("schema-provenance")), Retention: "required", AvailabilityEvidence: "verified"}})
	}
	manifest := contracts.LifecycleSnapshotManifest{SnapshotID: "snapshot-1", Version: "1", InstallationID: "installation-1", InstallationManifestDigest: artifactDigest([]byte("installation")), RequiredClasses: []contracts.LifecycleComponentClass{contracts.LifecycleBinary, contracts.LifecycleSchema}, Entries: entries, CapturedAt: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)}
	digest, err := manifest.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Digest = digest
	return manifest
}

type testResolver struct{ data []byte }

func (r testResolver) ResolveArtifact(_ context.Context, _ contracts.LifecycleArtifactRef) ([]byte, error) {
	return r.data, nil
}

func TestSnapshotExportIsCanonicalAndImportIsFenced(t *testing.T) {
	embedded, external := []byte("binary"), []byte("schema")
	manifest := testSnapshotManifest(t, embedded, external)
	req := SnapshotExportRequest{Manifest: manifest, Embedded: []SnapshotPayload{{ArtifactID: "binary-artifact", Bytes: embedded}}, External: testResolver{data: external}}
	first, digest, err := ExportSnapshot(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, digest2, err := ExportSnapshot(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || digest != digest2 {
		t.Fatal("snapshot export was not deterministic")
	}
	imported, err := ImportSnapshot(context.Background(), first, testResolver{data: external})
	if err != nil {
		t.Fatal(err)
	}
	if imported.Executable || imported.AuthorityState != "historical_only" {
		t.Fatalf("snapshot was not fenced: %+v", imported)
	}
}

func TestSnapshotRejectsMissingOrForgedExternalArtifact(t *testing.T) {
	embedded, external := []byte("binary"), []byte("schema")
	manifest := testSnapshotManifest(t, embedded, external)
	base := SnapshotExportRequest{Manifest: manifest, Embedded: []SnapshotPayload{{ArtifactID: "binary-artifact", Bytes: embedded}}}
	if _, _, err := ExportSnapshot(context.Background(), base); err == nil {
		t.Fatal("missing external verifier accepted")
	}
	base.External = testResolver{data: []byte("forged")}
	if _, _, err := ExportSnapshot(context.Background(), base); err == nil {
		t.Fatal("forged external artifact accepted")
	}
}

func TestSnapshotRejectsEmbeddedMismatchAndTampering(t *testing.T) {
	embedded, external := []byte("binary"), []byte("schema")
	manifest := testSnapshotManifest(t, embedded, external)
	bad := SnapshotExportRequest{Manifest: manifest, Embedded: []SnapshotPayload{{ArtifactID: "binary-artifact", Bytes: []byte("forged")}}, External: testResolver{data: external}}
	if _, _, err := ExportSnapshot(context.Background(), bad); err == nil {
		t.Fatal("forged embedded artifact accepted")
	}
	good := SnapshotExportRequest{Manifest: manifest, Embedded: []SnapshotPayload{{ArtifactID: "binary-artifact", Bytes: embedded}}, External: testResolver{data: external}}
	bytes, _, err := ExportSnapshot(context.Background(), good)
	if err != nil {
		t.Fatal(err)
	}
	bytes[len(bytes)-2] ^= 1
	if _, err := ImportSnapshot(context.Background(), bytes, testResolver{data: external}); err == nil {
		t.Fatal("tampered snapshot accepted")
	}
}
