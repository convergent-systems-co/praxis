package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	praxisv1 "github.com/convergent-systems-co/praxis/gen/praxis/v1"
	"github.com/convergent-systems-co/praxis/internal/plugin"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

func TestInspectIsPackageOwnedBehavior(t *testing.T) {
	baseline := goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "inspect", RefinedOutcome: "verified", Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll}
	digest, err := baseline.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	baseline.Digest = digest
	payload, err := json.Marshal(inspectRequest{GoalID: baseline.ID, GoalVersion: baseline.Version, Baseline: baseline})
	if err != nil {
		t.Fatal(err)
	}
	response, err := (server{}).Execute(context.Background(), &praxisv1.ExecuteRequest{Operation: "inspect", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if response.EvidenceClass != "observed" || len(response.Payload) == 0 {
		t.Fatalf("unexpected package result: %#v", response)
	}
}

func TestInspectRejectsInvalidBaseline(t *testing.T) {
	_, err := (server{}).Execute(context.Background(), &praxisv1.ExecuteRequest{Operation: "inspect", Payload: []byte(`{"goal_id":"goal-1","goal_version":"1","baseline":{}}`)})
	if err == nil {
		t.Fatal("invalid baseline must fail")
	}
}

func TestPackageOwnedExecutableServesInspectOverGRPC(t *testing.T) {
	if os.Getenv("PRAXIS_GOALS_PLUGIN_CHILD") == "1" {
		main()
		return
	}
	output := filepath.Join(t.TempDir(), "praxis-goals-plugin")
	command := exec.Command("go", "build", "-trimpath", "-o", output, ".")
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(bytes))
	identity := plugin.InstanceIdentity{InstanceID: "goals-test", PluginID: "praxis.package.goals", PluginVersion: "0.1.0", ArtifactDigest: digest, RuntimeSession: "session-test"}
	manifest := plugin.Manifest{ContractVersion: plugin.ManifestContractCurrentVersion(), ID: identity.PluginID, Version: identity.PluginVersion, ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "praxis-goals-plugin", ExecutableContentID: "goals-runtime", ExecutableContentVersion: "1", Capabilities: []string{"goals.lifecycle.execute"}, ArtifactDigest: digest}
	socketDir, err := os.MkdirTemp("/private/tmp", "g-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	socket := filepath.Join(socketDir, "plugin.sock")
	control := plugin.NewLocalProcessControl()
	if err := control.Start(context.Background(), plugin.LaunchSpec{Provider: plugin.Provider{Manifest: manifest, Identity: identity, State: plugin.StateInstalled, Isolation: plugin.IsolationProfile{}}, Executable: bytes, Entrypoint: manifest.Entrypoint, SocketPath: socket}); err != nil {
		t.Fatal(err)
	}
	defer control.Terminate(context.Background(), identity)
	deadline := time.Now().Add(5 * time.Second)
	var transport *plugin.GRPCTransport
	var lastDialErr error
	for transport == nil && time.Now().Before(deadline) {
		dialContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		candidate, _, dialErr := plugin.DialGRPCPlugin(dialContext, socket, plugin.ProtocolRange{Min: "1", Max: "1"}, manifest, identity, plugin.IsolationProfile{}, "goals-test")
		cancel()
		if dialErr == nil {
			transport = candidate
			break
		}
		lastDialErr = dialErr
		time.Sleep(10 * time.Millisecond)
	}
	if transport == nil {
		t.Fatalf("package-owned Goals executable did not complete handshake: %v", lastDialErr)
	}
	defer transport.Close()
	baseline := goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "inspect", RefinedOutcome: "verified", Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll}
	baseline.Digest, err = baseline.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(inspectRequest{GoalID: baseline.ID, GoalVersion: baseline.Version, Baseline: baseline})
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Call(context.Background(), identity, "inspect", payload)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatal(err)
	}
	if result["operation"] != "inspect" || result["baseline_digest"] != baseline.Digest {
		t.Fatalf("unexpected external plugin result: %s", response)
	}
}

func TestPackageOwnedExecutableTamperingFailsBeforeLaunch(t *testing.T) {
	manifest := plugin.Manifest{ContractVersion: plugin.ManifestContractCurrentVersion(), ID: "praxis.package.goals", Version: "0.1.0", ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "goals", ExecutableContentID: "goals-runtime", ExecutableContentVersion: "1", ArtifactDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	identity := plugin.InstanceIdentity{InstanceID: "tampered", PluginID: manifest.ID, PluginVersion: manifest.Version, ArtifactDigest: manifest.ArtifactDigest, RuntimeSession: "session"}
	err := (plugin.NewLocalProcessControl()).Start(context.Background(), plugin.LaunchSpec{Provider: plugin.Provider{Manifest: manifest, Identity: identity, State: plugin.StateInstalled}, Executable: []byte("tampered"), Entrypoint: manifest.Entrypoint, SocketPath: filepath.Join(t.TempDir(), "unused")})
	if err == nil {
		t.Fatal("tampered executable must fail digest validation before launch")
	}
}
