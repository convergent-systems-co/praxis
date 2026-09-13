package plugin

import "testing"

func TestInstanceIdentityMustMatchManifest(t *testing.T) {
	m := Manifest{ID: "workspace", Version: "1.0.0", ProtocolMin: "v1", ProtocolMax: "v1", Entrypoint: "workspace-plugin", ArtifactDigest: "sha256:abc", Capabilities: []string{"workspace.search.text"}}
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
	m := Manifest{ID: "p", Version: "1", ProtocolMin: "v1", ProtocolMax: "v1", Entrypoint: "p", ArtifactDigest: "sha256:x", Capabilities: []string{"a", "a"}}
	if err := m.Validate(); err == nil {
		t.Fatal("duplicate capabilities must be rejected")
	}
}
