package contracts

import (
	"errors"
	"reflect"
	"testing"
)

func TestVersionRegistryOwnsCompatibilityAndMigrationSemantics(t *testing.T) {
	called := 0
	policy := ContractVersionPolicy{Contract: "example.record", CurrentVersion: "v4", Versions: []ContractVersionDefinition{
		{Version: "v4", Disposition: VersionCurrent},
		{Version: "v3", Disposition: VersionSupportedHistorical},
		{Version: "v2", Disposition: VersionMigratable, MigrationID: "example-v2-to-v4/v1", MigrationTarget: "v4"},
		{Version: "v1-beta", Disposition: VersionUnsupportedPreRelease, Rationale: "never released"},
		{Version: "v1", Disposition: VersionRevokedUnsafe, Rationale: "authorization semantics are unsafe"},
	}}
	upcasters := map[string]VersionUpcaster{"example-v2-to-v4/v1": func(payload []byte) ([]byte, error) {
		called++
		return append([]byte("v4:"), payload...), nil
	}}
	registry, err := NewVersionRegistry(policy, upcasters)
	if err != nil {
		t.Fatal(err)
	}

	current := []byte("current")
	canonical, migration, err := registry.Canonicalize("v4", current)
	if err != nil || migration != nil || !reflect.DeepEqual(canonical, current) {
		t.Fatalf("current version was not readable: %q %#v %v", canonical, migration, err)
	}
	historical, migration, err := registry.Canonicalize("v3", []byte("historical"))
	if err != nil || migration != nil || string(historical) != "historical" {
		t.Fatalf("supported historical version was not readable as-is: %q %#v %v", historical, migration, err)
	}
	migrated, migration, err := registry.Canonicalize("v2", []byte("old"))
	if err != nil || called != 1 || string(migrated) != "v4:old" || migration == nil || migration.UpcasterID != "example-v2-to-v4/v1" || migration.FromVersion != "v2" || migration.ToVersion != "v4" || migration.PolicyDigest != registry.PolicyDigest() || migration.InputDigest == migration.OutputDigest {
		t.Fatalf("versioned migration was not invoked and traced: %q %#v %v", migrated, migration, err)
	}
	migratedAgain, migrationAgain, err := registry.Canonicalize("v2", []byte("old"))
	if err != nil || called != 2 || !reflect.DeepEqual(migratedAgain, migrated) || !reflect.DeepEqual(migrationAgain, migration) {
		t.Fatalf("upcaster was not deterministic: %q %#v %v", migratedAgain, migrationAgain, err)
	}
	restarted, err := NewVersionRegistry(policy, upcasters)
	if err != nil || restarted.PolicyDigest() != registry.PolicyDigest() {
		t.Fatalf("contract policy changed after registry reconstruction: %v", err)
	}
	replayed, replayMigration, err := restarted.Canonicalize("v2", []byte("old"))
	if err != nil || !reflect.DeepEqual(replayed, migrated) || !reflect.DeepEqual(replayMigration, migration) {
		t.Fatalf("migration replay changed after registry reconstruction: %q %#v %v", replayed, replayMigration, err)
	}
	if _, _, err := registry.Canonicalize("v1-beta", nil); !errors.Is(err, ErrUnsupportedPreReleaseContractVersion) {
		t.Fatalf("pre-release version did not fail intentionally: %v", err)
	}
	if _, _, err := registry.Canonicalize("v1", nil); !errors.Is(err, ErrRevokedContractVersion) {
		t.Fatalf("revoked version did not fail closed: %v", err)
	}
	if _, _, err := registry.Canonicalize("v99", nil); !errors.Is(err, ErrUnknownContractVersion) {
		t.Fatalf("unknown future version did not fail closed: %v", err)
	}
	if _, ok := registry.Definition("v99"); ok {
		t.Fatal("registry inferred compatibility from version ordering")
	}
	policyCopy := registry.Policy()
	policyCopy.Versions[0].Disposition = VersionCurrent
	if definition, _ := registry.Definition("v1"); definition.Disposition != VersionRevokedUnsafe {
		t.Fatal("fixture expected v1 to remain revoked")
	}
}
