package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
)

func TestNativeGoalDriveDoesNotReadPackageRegistryBeforeBootstrap(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "bootstrap.json")
	getenv := func(key string) string {
		if key == "PRAXIS_BOOTSTRAP_RECORD" {
			return missing
		}
		if key == "PRAXIS_DB" {
			return filepath.Join(t.TempDir(), "does-not-exist.db")
		}
		return ""
	}
	err := runNativeGoalDriveInvocation(context.Background(), []string{"--goal-id=goal", "--goal-version=1", "--provider=local", "--invocation-id=first"}, getenv)
	if !errors.Is(err, praxiscrypto.ErrBootstrapRecordMissing) {
		t.Fatalf("first-run dispatch must reach bootstrap state before registry SQLite: %v", err)
	}
}

func TestStateInitRequiresBootstrapBeforeCreatingSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "praxis.db")
	getenv := func(key string) string {
		if key == "PRAXIS_DB" {
			return dbPath
		}
		return ""
	}
	if err := runStateInit(nil, getenv); err == nil {
		t.Fatal("state initialization must require bootstrap authority")
	}
}
