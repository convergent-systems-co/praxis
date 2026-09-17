package goalspublication

import (
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestAssetRoleIndexMapsFilenameToCanonicalRole proves each real GitHub
// asset filename resolves to its correct canonical role index and that the
// corresponding assetDigests/assetNames entries agree with that role — the
// exact correspondence the original defect silently broke (every filename
// resolved to index 0, so archive and signature were validated against
// manifest's expected digest and size).
func TestAssetRoleIndexMapsFilenameToCanonicalRole(t *testing.T) {
	cases := []struct {
		name       string
		wantIdx    int
		wantDigest string
	}{
		{"praxis-package.json", 0, contracts.GoalsPublicationManifest},
		{"praxis-package.tar.gz", 1, contracts.GoalsPublicationArchive},
		{"praxis-package.sig.json", 2, contracts.GoalsPublicationSignature},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idx, ok := assetRoleIndex(c.name)
			if !ok {
				t.Fatalf("expected %q to resolve, got ok=false", c.name)
			}
			if idx != c.wantIdx {
				t.Fatalf("got idx=%d, want %d", idx, c.wantIdx)
			}
			if assetNames[idx] != c.name {
				t.Fatalf("assetNames[%d]=%q does not round-trip to %q", idx, assetNames[idx], c.name)
			}
			if assetDigests[idx] != c.wantDigest {
				t.Fatalf("assetDigests[%d]=%q, want %q", idx, assetDigests[idx], c.wantDigest)
			}
		})
	}
}

// TestAssetRoleIndexArchiveAndSignatureNeverCollideWithManifest is the
// direct adversarial regression for the actual historical failure mode: the
// archive and signature roles must resolve to a DIFFERENT expected digest
// than the manifest role, so an archive or signature asset can never be
// silently validated against the manifest's expectations the way the
// original map-miss-defaults-to-0 bug caused.
func TestAssetRoleIndexArchiveAndSignatureNeverCollideWithManifest(t *testing.T) {
	manifestIdx, ok := assetRoleIndex("praxis-package.json")
	if !ok {
		t.Fatal("manifest name did not resolve")
	}
	archiveIdx, ok := assetRoleIndex("praxis-package.tar.gz")
	if !ok {
		t.Fatal("archive name did not resolve")
	}
	signatureIdx, ok := assetRoleIndex("praxis-package.sig.json")
	if !ok {
		t.Fatal("signature name did not resolve")
	}
	if archiveIdx == manifestIdx {
		t.Fatal("archive resolved to the same index as manifest")
	}
	if signatureIdx == manifestIdx {
		t.Fatal("signature resolved to the same index as manifest")
	}
	if assetDigests[archiveIdx] == assetDigests[manifestIdx] {
		t.Fatal("archive's expected digest equals manifest's expected digest")
	}
	if assetDigests[signatureIdx] == assetDigests[manifestIdx] {
		t.Fatal("signature's expected digest equals manifest's expected digest")
	}
}

// TestAssetRoleIndexFailsClosedForUnknownName proves an unrecognized asset
// name is rejected explicitly (ok=false) rather than silently defaulting to
// index 0 the way a bare map lookup did.
func TestAssetRoleIndexFailsClosedForUnknownName(t *testing.T) {
	for _, name := range []string{"", "manifest", "archive", "signature", "praxis-package.json.bak", "PRAXIS-PACKAGE.JSON", "../praxis-package.json"} {
		t.Run(name, func(t *testing.T) {
			if idx, ok := assetRoleIndex(name); ok {
				t.Fatalf("expected unrecognized name %q to fail closed, got idx=%d ok=true", name, idx)
			}
		})
	}
}

// TestAssetRoleIndexAllThreeCanonicalAssetsResolveExactlyOnceEach proves the
// three canonical names collectively resolve to exactly the three distinct
// indices 0,1,2 with no duplicate/missing role — the inventory-completeness
// property the original defect (all filenames -> index 0) violated.
func TestAssetRoleIndexAllThreeCanonicalAssetsResolveExactlyOnceEach(t *testing.T) {
	seen := map[int]bool{}
	for _, name := range assetNames {
		idx, ok := assetRoleIndex(name)
		if !ok {
			t.Fatalf("canonical name %q did not resolve", name)
		}
		if seen[idx] {
			t.Fatalf("index %d resolved more than once", idx)
		}
		seen[idx] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected exactly 3 distinct resolved indices, got %d", len(seen))
	}
}
