package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/plugin"
)

func TestPluginSupervisorSnapshotSurvivesSQLiteRestartWithoutRuntimeReadiness(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	digest := "sha256:plugin"
	snapshot := plugin.SupervisorSnapshot{
		Provider: plugin.ProviderSnapshot{
			Manifest: plugin.Manifest{ContractVersion: plugin.ManifestContractCurrentVersion(), ID: "research/plugin", Version: "1", ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "plugin", ExecutableContentID: "plugin.exe", ExecutableContentVersion: "1", ArtifactDigest: digest},
			Identity: plugin.InstanceIdentity{InstanceID: "instance-1", PluginID: "research/plugin", PluginVersion: "1", ArtifactDigest: digest, RuntimeSession: "session-1"},
			State:    plugin.StateReady,
		},
		Launch:      plugin.LaunchSnapshot{Executable: []byte("verified"), Entrypoint: "plugin", SocketPath: "/tmp/plugin.sock"},
		LastStartAt: time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC),
	}
	if err := store.SaveSupervisorSnapshot(ctx, snapshot); err != nil {
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
	loaded, err := New(db).LoadSupervisorSnapshots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Provider.Identity != snapshot.Provider.Identity || loaded[0].Provider.State != plugin.StateReady || string(loaded[0].Launch.Executable) != "verified" {
		t.Fatalf("snapshot lost durable launch identity/state: %+v", loaded)
	}
}
