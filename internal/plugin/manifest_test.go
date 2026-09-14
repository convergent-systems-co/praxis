package plugin

import "testing"

func TestInstanceIdentityMustMatchManifest(t *testing.T) {
	m := Manifest{ContractVersion: ManifestContractCurrentVersion(), ID: "workspace", Version: "1.0.0", ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "plugins/workspace", ExecutableContentID: "workspace.executable", ExecutableContentVersion: "1.0.0", ArtifactDigest: "sha256:abc", Capabilities: []string{"workspace.search.text"}}
	i := InstanceIdentity{InstanceID: "instance-1", PluginID: "workspace", PluginVersion: "1.0.0", ArtifactDigest: "sha256:def", RuntimeSession: "session-1"}
	if err := i.ValidateAgainst(m); err == nil {
		t.Fatal("instance with different artifact digest must be rejected")
	}
	i.ArtifactDigest = m.ArtifactDigest
	if err := i.ValidateAgainst(m); err != nil {
		t.Fatalf("matching instance rejected: %v", err)
	}
}

func TestManifestRejectsDuplicateCapabilities(t *testing.T) {
	m := Manifest{ContractVersion: ManifestContractCurrentVersion(), ID: "p", Version: "1", ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "plugins/p", ExecutableContentID: "p.executable", ExecutableContentVersion: "1", ArtifactDigest: "sha256:x", Capabilities: []string{"a", "a"}}
	if err := m.Validate(); err == nil {
		t.Fatal("duplicate capabilities must be rejected")
	}
}

func TestManifestVersionPolicyRejectsPreReleaseAndUnknownContracts(t *testing.T) {
	m := Manifest{ContractVersion: "v1", ID: "p", Version: "1", ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "plugins/p", ExecutableContentID: "p.executable", ExecutableContentVersion: "1", ArtifactDigest: "sha256:x"}
	if err := m.Validate(); err == nil {
		t.Fatal("unsupported pre-release plugin manifest crossed the durable boundary")
	}
	m.ContractVersion = "v99"
	if err := m.Validate(); err == nil {
		t.Fatal("unknown future plugin manifest crossed the durable boundary")
	}
}
