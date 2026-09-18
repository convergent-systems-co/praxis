package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func launchFixtureProvider() Provider {
	identity := InstanceIdentity{InstanceID: "launch-instance", PluginID: "launch", PluginVersion: "1", ArtifactDigest: "placeholder", RuntimeSession: "launch-session"}
	manifest := Manifest{ContractVersion: ManifestContractCurrentVersion(), ID: "launch", Version: "1", ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "fixture", ExecutableContentID: "launch.executable", ExecutableContentVersion: "1", ArtifactDigest: "placeholder"}
	identity.ArtifactDigest = manifest.ArtifactDigest
	return Provider{Manifest: manifest, Identity: identity, State: StateInstalled, Isolation: IsolationProfile{Properties: map[IsolationProperty]EnforcementState{}}}
}

func TestLocalProcessControlBindsIdentityAndReportsExit(t *testing.T) {
	output := filepath.Join(t.TempDir(), "launch-context")
	bytes := []byte("#!/bin/sh\nprintf '%s|%s|%s|%s|%s' \"$PRAXIS_PLUGIN_SOCKET\" \"$PRAXIS_PLUGIN_ID\" \"$PRAXIS_PLUGIN_VERSION\" \"$PRAXIS_PLUGIN_INSTANCE\" \"$PRAXIS_PLUGIN_SESSION\" > \"$PRAXIS_PLUGIN_SOCKET\"\nexit 23\n")
	digest := sha256.Sum256(bytes)
	provider := launchFixtureProvider()
	provider.Manifest.ArtifactDigest = "sha256:" + hex.EncodeToString(digest[:])
	provider.Identity.ArtifactDigest = provider.Manifest.ArtifactDigest
	spec := LaunchSpec{Provider: provider, Executable: bytes, Entrypoint: "fixture", SocketPath: output}
	control := NewLocalProcessControl()
	exited := make(chan error, 1)
	control.SetExitHandler(provider.Identity, func(err error) { exited <- err })
	if err := control.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err == nil {
			t.Fatal("non-zero plugin exit must be reported")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("plugin exit was not observed")
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := output + "|launch|1|launch-instance|launch-session"
	if string(contents) != want {
		t.Fatalf("launch identity mismatch: got %q want %q", contents, want)
	}
}

func TestLocalProcessControlTerminationRequiresExactRuntimeSession(t *testing.T) {
	bytes := []byte("#!/bin/sh\nsleep 5\n")
	digest := sha256.Sum256(bytes)
	provider := launchFixtureProvider()
	provider.Manifest.ArtifactDigest = "sha256:" + hex.EncodeToString(digest[:])
	provider.Identity.ArtifactDigest = provider.Manifest.ArtifactDigest
	spec := LaunchSpec{Provider: provider, Executable: bytes, Entrypoint: "fixture", SocketPath: filepath.Join(t.TempDir(), "unused")}
	control := NewLocalProcessControl()
	if err := control.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	wrongSession := provider.Identity
	wrongSession.RuntimeSession = "different-session"
	if err := control.Terminate(context.Background(), wrongSession); err == nil {
		t.Fatal("termination with a different runtime session must fail closed")
	}
	if err := control.Terminate(context.Background(), provider.Identity); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchSpecRequiresExactVerifiedExecutableBytesAndIsolation(t *testing.T) {
	provider := launchFixtureProvider()
	bytes := []byte("#!/bin/sh\n")
	digest := sha256.Sum256(bytes)
	provider.Manifest.ArtifactDigest = "sha256:" + hex.EncodeToString(digest[:])
	provider.Identity.ArtifactDigest = provider.Manifest.ArtifactDigest
	spec := LaunchSpec{Provider: provider, Executable: bytes, Entrypoint: "fixture", SocketPath: "/tmp/plugin.sock"}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	spec.Executable = []byte("tampered")
	if err := spec.Validate(); err == nil {
		t.Fatal("launch must reject executable bytes that differ from signed digest")
	}
	spec.Executable = bytes
	spec.RequiredIsolation = []IsolationProperty{IsolationNetwork}
	spec.Provider.Isolation = IsolationProfile{Properties: map[IsolationProperty]EnforcementState{IsolationNetwork: Enforced}}
	if err := spec.Validate(); err != nil {
		t.Fatalf("spec validation should preserve required isolation for host policy: %v", err)
	}
	control := NewLocalProcessControl()
	if err := control.Start(context.Background(), spec); err == nil {
		t.Fatal("local launcher must fail closed when required isolation has no enforcer")
	}
}

func TestLocalProcessControlMaterializesOnlyVerifiedBytesAndTerminates(t *testing.T) {
	bytes := []byte("#!/bin/sh\nwhile true; do /bin/sleep 1; done\n")
	digest := sha256.Sum256(bytes)
	provider := launchFixtureProvider()
	provider.Manifest.ArtifactDigest = "sha256:" + hex.EncodeToString(digest[:])
	provider.Identity.ArtifactDigest = provider.Manifest.ArtifactDigest
	spec := LaunchSpec{Provider: provider, Executable: bytes, Entrypoint: "fixture", SocketPath: "/tmp/plugin.sock"}
	control := NewLocalProcessControl()
	if err := control.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Terminate(context.Background(), provider.Identity) })
	deadline := time.Now().Add(time.Second)
	for {
		control.mu.Lock()
		_, running := control.process[processKey(provider.Identity)]
		control.mu.Unlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("materialized plugin did not remain running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := control.Terminate(context.Background(), provider.Identity); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(provider.Manifest.Entrypoint, "/") {
		t.Fatal("fixture entrypoint unexpectedly became an absolute host path")
	}
}
