package packagecatalog

import (
	"slices"
	"testing"
)

func TestEffectiveCapabilitiesIncludesDependencies(t *testing.T) {
	dep := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "dep", Version: "1", ContentDigest: "sha256:dep", Capabilities: []string{"network.read"}}
	root := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "root", Version: "1", ContentDigest: "sha256:root", Capabilities: []string{"workspace.read"}, Dependencies: []Dependency{{PackageID: "dep", Version: "1", Digest: "sha256:dep"}}}
	caps, err := EffectiveCapabilities(root, map[string]Manifest{"dep": dep})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(caps, "workspace.read") || !slices.Contains(caps, "network.read") {
		t.Fatalf("missing transitive capability: %+v", caps)
	}
}

func TestEffectiveCapabilitiesRejectsLockMismatch(t *testing.T) {
	dep := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "dep", Version: "2", ContentDigest: "sha256:new"}
	root := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "root", Version: "1", ContentDigest: "sha256:root", Dependencies: []Dependency{{PackageID: "dep", Version: "1", Digest: "sha256:old"}}}
	if _, err := EffectiveCapabilities(root, map[string]Manifest{"dep": dep}); err == nil {
		t.Fatal("dependency lock mismatch must fail")
	}
}

func TestEffectiveCapabilitiesRejectsCycle(t *testing.T) {
	a := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "a", Version: "1", ContentDigest: "sha256:a", Dependencies: []Dependency{{PackageID: "b", Version: "1", Digest: "sha256:b"}}}
	b := Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "b", Version: "1", ContentDigest: "sha256:b", Dependencies: []Dependency{{PackageID: "a", Version: "1", Digest: "sha256:a"}}}
	if _, err := EffectiveCapabilities(a, map[string]Manifest{"a": a, "b": b}); err == nil {
		t.Fatal("dependency cycle must fail")
	}
}
