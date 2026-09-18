package state

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/plugin"
)

func TestResolvedPluginLaunchSpecBindsExactVerifiedPackageBytes(t *testing.T) {
	executable := []byte("#!/bin/sh\n")
	digest := digestPackageBytes(executable)
	manifest := plugin.Manifest{
		ContractVersion:          plugin.ManifestContractCurrentVersion(),
		ID:                       "research/plugin",
		Version:                  "1",
		ProtocolMin:              "1",
		ProtocolMax:              "1",
		Entrypoint:               "bin/plugin",
		ExecutableContentID:      "research/plugin.executable",
		ExecutableContentVersion: "1",
		ArtifactDigest:           digest,
	}
	resolved := ResolvedPlugin{
		Manifest: manifest,
		Executable: RegisteredContent{
			PackageID: "research/package", PackageVersion: "1", PackageDigest: "sha256:package",
			Content:       packagecatalog.ContentRef{Kind: packagecatalog.ContentPluginExecutable, ID: manifest.ExecutableContentID, Version: "1", Digest: digest, Artifact: manifest.Entrypoint},
			ArtifactBytes: executable,
		},
	}
	instance := plugin.InstanceIdentity{InstanceID: "research-instance", PluginID: manifest.ID, PluginVersion: manifest.Version, ArtifactDigest: digest, RuntimeSession: "session-1"}
	spec, err := resolved.LaunchSpec(instance, plugin.IsolationProfile{}, "/tmp/praxis-research.sock", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(spec.Executable) != string(executable) || spec.Entrypoint != manifest.Entrypoint || spec.Provider.Identity != instance {
		t.Fatalf("launch spec lost package-bound executable identity: %+v", spec)
	}
	spec.Executable[0] = 'X'
	if string(resolved.Executable.ArtifactBytes) != string(executable) {
		t.Fatal("launch spec must copy verified package bytes rather than aliasing state")
	}
	resolved.Executable.ArtifactBytes = []byte("tampered")
	if _, err := resolved.LaunchSpec(instance, plugin.IsolationProfile{}, "/tmp/praxis-research.sock", nil); err == nil {
		t.Fatal("launch spec must reject package bytes that no longer match the signed digest")
	}
}
