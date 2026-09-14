package plugin

import (
	"errors"
	"testing"
)

func validHandshakeFixture() Handshake {
	manifest := Manifest{
		ContractVersion: ManifestContractCurrentVersion(), ID: "workspace", Version: "1.0.0", ProtocolMin: "1", ProtocolMax: "3", Entrypoint: "plugins/workspace",
		ExecutableContentID: "workspace.executable", ExecutableContentVersion: "1.0.0",
		Capabilities: []string{"workspace.search.text"}, RequiredIsolation: []IsolationProperty{IsolationFilesystem},
		Publisher: "test", ArtifactDigest: "sha256:workspace",
	}
	return Handshake{
		Manifest:               manifest,
		Identity:               InstanceIdentity{InstanceID: "i1", PluginID: "workspace", PluginVersion: "1.0.0", ArtifactDigest: "sha256:workspace", RuntimeSession: "s1"},
		Isolation:              IsolationProfile{Properties: map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}},
		AdvertisedCapabilities: []string{"workspace.search.text"},
		Protocol:               ProtocolRange{Min: "1", Max: "3"},
	}
}

func TestHandshakeAcceptsMatchingRuntime(t *testing.T) {
	result, err := ValidateHandshake(ProtocolRange{Min: "2", Max: "4"}, validHandshakeFixture())
	if err != nil {
		t.Fatal(err)
	}
	if result.NegotiatedProtocol != "3" {
		t.Fatalf("expected protocol 3, got %q", result.NegotiatedProtocol)
	}
	if result.Provider.State != StateStarting {
		t.Fatalf("handshake must not mark provider ready, got %s", result.Provider.State)
	}
}

func TestHandshakeRejectsArtifactIdentityMismatch(t *testing.T) {
	h := validHandshakeFixture()
	h.Identity.ArtifactDigest = "sha256:other"
	if _, err := ValidateHandshake(ProtocolRange{Min: "1", Max: "3"}, h); err == nil {
		t.Fatal("artifact identity mismatch must fail")
	}
}

func TestHandshakeRejectsUndeclaredCapability(t *testing.T) {
	h := validHandshakeFixture()
	h.AdvertisedCapabilities = append(h.AdvertisedCapabilities, "shell.exec")
	if _, err := ValidateHandshake(ProtocolRange{Min: "1", Max: "3"}, h); err == nil {
		t.Fatal("runtime cannot self-advertise undeclared authority")
	}
}

func TestHandshakeRejectsMissingDeclaredCapability(t *testing.T) {
	h := validHandshakeFixture()
	h.AdvertisedCapabilities = nil
	if _, err := ValidateHandshake(ProtocolRange{Min: "1", Max: "3"}, h); err == nil {
		t.Fatal("runtime must advertise declared capability set")
	}
}

func TestHandshakeRejectsProtocolMismatch(t *testing.T) {
	h := validHandshakeFixture()
	h.Protocol = ProtocolRange{Min: "1", Max: "1"}
	_, err := ValidateHandshake(ProtocolRange{Min: "2", Max: "4"}, h)
	if !errors.Is(err, ErrProtocolIncompatible) {
		t.Fatalf("expected protocol incompatibility, got %v", err)
	}
}
