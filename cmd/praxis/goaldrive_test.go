package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestDispatchGoalDriveRequiresExactDurableGeneration(t *testing.T) {
	out := normalizedOutput{EntryPointID: "goal-drive", Options: map[string]string{"goal-id": "goal-1", "provider": "local", "invocation-id": "inv-1"}}
	if !errors.Is(dispatchGoalDrive(context.Background(), out, nil), goaldrive.ErrExactGoalVersionRequired) {
		t.Fatal("goal-drive dispatch must require an exact Goal Baseline version")
	}
}

func TestDispatchGoalDriveFailsClosedBeforeExecutionDependencies(t *testing.T) {
	out := normalizedOutput{EntryPointID: "goal-drive", Options: map[string]string{"goal-id": "goal-1", "goal-version": "7", "provider": "local", "invocation-id": "inv-1"}}
	if !errors.Is(dispatchGoalDrive(context.Background(), out, nil), errGoalDriveDispatchDependencies) {
		t.Fatal("CLI must fail closed when runtime provider/key dependencies are unavailable")
	}
}

func TestNativeRuntimeRequiresPersistedBootstrapRecord(t *testing.T) {
	out := normalizedOutput{GraphID: "goal.graph", GraphVersion: "1"}
	invocation := goaldrive.InvocationRequest{ProviderID: "local", RepositoryPath: t.TempDir(), Branch: "main"}
	getenv := func(key string) string {
		if key == "PRAXIS_BOOTSTRAP_RECORD" {
			return filepath.Join(t.TempDir(), "missing.json")
		}
		return ""
	}
	if _, _, err := buildGoalDriveRuntime(context.Background(), out, invocation, getenv); !errors.Is(err, praxiscrypto.ErrBootstrapRecordMissing) {
		t.Fatalf("missing bootstrap metadata must be distinguished: %v", err)
	}
}

func TestNativeRuntimeRejectsUnknownBootstrapProviderBeforeStateOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	record := praxiscrypto.BootstrapRecord{Version: praxiscrypto.BootstrapRecordVersion, ProviderID: "substituted", KeyID: "goal", KeyVersion: "1", KeyMaterialHash: "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000", Owner: "user", Purpose: "goalstore", Profile: contracts.CryptoClassicalCompatible, SecurityLevel: praxiscrypto.SecurityPlatformProtected, Platform: "darwin", Architecture: "arm64", CreatedAt: time.Unix(1, 0).UTC()}
	if err := praxiscrypto.SaveBootstrapRecord(path, record); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "PRAXIS_BOOTSTRAP_RECORD" {
			return path
		}
		return ""
	}
	if _, _, err := buildGoalDriveRuntime(context.Background(), normalizedOutput{}, goaldrive.InvocationRequest{ProviderID: "local"}, getenv); err == nil || !strings.Contains(err.Error(), "unknown cryptographic key provider") {
		t.Fatalf("provider substitution must fail before GoalStore construction: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
