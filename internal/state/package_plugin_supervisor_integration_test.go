package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/plugin"
)

type packageSupervisorProcess struct{}

func (packageSupervisorProcess) Start(context.Context, plugin.LaunchSpec) error           { return nil }
func (packageSupervisorProcess) Terminate(context.Context, plugin.InstanceIdentity) error { return nil }

func TestActivatedPackageBindsSupervisorLaunchAndFreshRestartHandshake(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "research/mixed", Version: "1", Contents: []packagecatalog.ContentRef{
		{Kind: packagecatalog.ContentPlugin, ID: "research.provider", Version: "1", Artifact: "plugins/provider.json"},
		{Kind: packagecatalog.ContentPluginExecutable, ID: "research.provider.executable", Version: "1", Artifact: "plugins/provider"},
	}}
	manifest = activateFixturePackage(t, ctx, db, New(db), manifest, now)
	resolved, err := New(db).ResolvePlugin(ctx, "research.provider", "1")
	if err != nil {
		t.Fatal(err)
	}
	isolation := plugin.IsolationProfile{Properties: map[plugin.IsolationProperty]plugin.EnforcementState{plugin.IsolationFilesystem: plugin.Enforced}}
	identity := plugin.InstanceIdentity{InstanceID: "research-instance", PluginID: resolved.Manifest.ID, PluginVersion: resolved.Manifest.Version, ArtifactDigest: resolved.Manifest.ArtifactDigest, RuntimeSession: "session-1"}
	spec, err := resolved.LaunchSpec(identity, isolation, filepath.Join(t.TempDir(), "plugin.sock"), nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := plugin.NewRegistry()
	supervisor, err := plugin.NewPersistentSupervisor(ctx, registry, packageSupervisorProcess{}, plugin.SupervisorPolicy{}, New(db))
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.RegisterLaunch(spec); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Start(ctx, identity.InstanceID, now); err != nil {
		t.Fatal(err)
	}
	handshake, err := plugin.ValidateHandshake(plugin.ProtocolRange{Min: "1", Max: "1"}, plugin.Handshake{Manifest: resolved.Manifest, Identity: identity, Isolation: isolation, AdvertisedCapabilities: []string{"workspace.search.text"}, Protocol: plugin.ProtocolRange{Min: "1", Max: "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.MarkReady(identity.InstanceID, handshake); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("workspace.search.text", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted, err := plugin.NewPersistentSupervisor(ctx, plugin.NewRegistry(), packageSupervisorProcess{}, plugin.SupervisorPolicy{}, New(db))
	if err != nil {
		t.Fatal(err)
	}
	if state, ok := restarted.State(identity.InstanceID); !ok || state != plugin.StateStopped {
		t.Fatalf("restart must discard ready runtime state: %q %v", state, ok)
	}
}
