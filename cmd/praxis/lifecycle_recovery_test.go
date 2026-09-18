package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestLifecycleRecoverStatusReadsEmptyJournalFromFreshDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	getenv := func(key string) string {
		switch key {
		case "PRAXIS_DB":
			return path
		case "PRAXIS_ACTOR_ID":
			return "agent-1"
		case "PRAXIS_ACTOR_KIND":
			return "agent"
		}
		return ""
	}
	if err := runLifecycleRecoveryCommandTo([]string{"status", "--installation", "install-1"}, getenv, &out); err != nil {
		t.Fatal(err)
	}
	var history []contracts.LifecycleTransitionJournal
	if err := json.Unmarshal(out.Bytes(), &history); err != nil {
		t.Fatalf("decode journal history: %v (output %q)", err, out.String())
	}
	if len(history) != 0 {
		t.Fatalf("expected empty journal for a fresh installation, got %d entries", len(history))
	}
}

// TestLifecycleRecoverWiredBeforeDynamicDispatch proves the reserved
// control-plane case in cmd/praxis/main.go's run() switch is actually
// reached before the fallthrough to runDynamicInvocation (PLAN-016 WU1).
// If the case were missing or misplaced, run() would instead attempt
// dynamic invocation resolution for "lifecycle-recover", which fails
// differently (and does not decode as a LifecycleTransitionJournal array).
func TestLifecycleRecoverWiredBeforeDynamicDispatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PRAXIS_DB", path)
	t.Setenv("PRAXIS_ACTOR_ID", "agent-1")
	t.Setenv("PRAXIS_ACTOR_KIND", "agent")

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	runErr := run([]string{"lifecycle-recover", "status", "--installation", "install-1"})
	_ = write.Close()
	os.Stdout = original
	body, readErr := io.ReadAll(read)
	_ = read.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	var history []contracts.LifecycleTransitionJournal
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("run() output did not decode as lifecycle journal history: %v (output %q)", err, body)
	}
}

// TestLifecycleRecoverIndependentOfInvocationRegistry proves the reserved
// entry point has zero dependency on invocation_runtime_bindings or
// invocation_registry: it succeeds even when both tables are dropped
// entirely, simulating exactly the structural breakage this command family
// exists to recover from (PLAN-016 WU1).
func TestLifecycleRecoverIndependentOfInvocationRegistry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE invocation_runtime_bindings`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE invocation_registry`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	getenv := func(key string) string {
		switch key {
		case "PRAXIS_DB":
			return path
		case "PRAXIS_ACTOR_ID":
			return "agent-1"
		case "PRAXIS_ACTOR_KIND":
			return "agent"
		}
		return ""
	}
	if err := runLifecycleRecoveryCommandTo([]string{"status", "--installation", "install-1"}, getenv, &out); err != nil {
		t.Fatalf("lifecycle-recover status must not depend on invocation_runtime_bindings/invocation_registry: %v", err)
	}
}

func TestLifecycleRecoverStatusRequiresInstallation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "PRAXIS_DB" {
			return path
		}
		return ""
	}
	if err := runLifecycleRecoveryCommandTo([]string{"status", "--actor-id", "agent-1", "--actor-kind", "agent"}, getenv, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for missing --installation")
	}
}
