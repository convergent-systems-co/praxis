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
	getenv := func(string) string { return "" }
	if !errors.Is(dispatchGoalDrive(context.Background(), out, getenv), errGoalDriveDispatchDependencies) {
		t.Fatal("CLI must fail closed when runtime provider/key dependencies are unavailable")
	}
}

func TestDispatchGoalDriveDefaultsEnvironmentLookup(t *testing.T) {
	t.Setenv("PRAXIS_BOOTSTRAP_RECORD", "")
	out := normalizedOutput{EntryPointID: "goal-drive", Options: map[string]string{"goal-id": "goal-1", "goal-version": "7", "provider": "local", "invocation-id": "inv-1"}}
	if err := dispatchGoalDrive(context.Background(), out, nil); !errors.Is(err, errGoalDriveDispatchDependencies) {
		t.Fatalf("nil environment lookup must use the process environment and fail closed at dependencies: %v", err)
	}
}

func TestDispatchGoalDriveRecoveryRequiresWorkspace(t *testing.T) {
	out := normalizedOutput{EntryPointID: "goal-drive", Options: map[string]string{"goal-id": "goal-1", "goal-version": "7", "provider": "local", "invocation-id": "inv-1"}}
	getenv := func(key string) string {
		if key == "PRAXIS_PROVIDER_WORKSPACE_RECONCILE_ONLY" {
			return "true"
		}
		return ""
	}
	if err := dispatchGoalDrive(context.Background(), out, getenv); err == nil || !strings.Contains(err.Error(), "workspace reconciliation requires a provider workspace") {
		t.Fatalf("recovery-only dispatch must require a workspace: %v", err)
	}
}

func TestValidateProviderWorkspaceTurnRejectsStaleTurnBinding(t *testing.T) {
	now := time.Now().UTC()
	workspace := contracts.ProviderWorkspaceRecord{
		WorkspaceID: "workspace-1", Version: "1", Path: "/tmp/workspace-1", Repository: "/tmp/repository",
		GoalID: "goal-1", GoalVersion: "7", WorkPlanRef: "plan", WorkPlanDigest: "sha256:plan", ChildObjective: "child-1",
		InvocationID: "inv-1", TurnID: "turn-1", ProviderID: "codex-subscription", StartHead: "head-1", State: contracts.ProviderWorkspaceActive, CreatedAt: now,
	}
	turn := goaldrive.TurnRecord{GoalID: "goal-1", GoalVersion: "7", InvocationID: "inv-1", TurnID: "turn-2", ChildObjective: "child-1", ExecutorID: "codex-subscription"}
	if err := validateProviderWorkspaceTurn(workspace, turn); err == nil {
		t.Fatal("stale workspace turn binding was accepted")
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
