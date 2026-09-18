package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
)

func fakeCLI(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestProviderCatalogReportsAvailabilityExactlyAsGoalDriveResolves proves the
// public provider catalog answers "which --provider values are valid here"
// from the same facts goal-drive uses: PATH lookup for first-party profiles
// and the environment command worker, without selecting anything.
func TestProviderCatalogReportsAvailabilityExactlyAsGoalDriveResolves(t *testing.T) {
	dir := t.TempDir()
	fakeCLI(t, dir, "claude")
	env := map[string]string{"PATH": dir}
	getenv := func(key string) string { return env[key] }
	t.Setenv("PATH", dir)
	catalog, err := goalDriveProviderCatalog(getenv)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]goalDriveProvider{}
	for _, entry := range catalog {
		byID[entry.ID] = entry
	}
	if !byID["claude"].Available || !byID["claude-subscription"].Available || byID["claude"].Executable != filepath.Join(dir, "claude") {
		t.Fatalf("claude profiles must be available with the PATH executable: %+v", byID["claude"])
	}
	if byID["codex"].Available || !strings.Contains(byID["codex"].Reason, "not on PATH") {
		t.Fatalf("codex must be reported unavailable with a reason: %+v", byID["codex"])
	}
	if _, ok := byID["<any identity>"]; ok {
		t.Fatal("environment worker must not be listed when PRAXIS_GOAL_WORKER_ARGV is unset")
	}
	if got := availableProviderIDs(getenv); strings.Join(got, ",") != "claude,claude-subscription" {
		t.Fatalf("available ids: %v", got)
	}
	// goal-drive's own resolution agrees: unknown and unavailable are
	// distinguishable, and both name the discovery command.
	if _, err := configuredWorker(goaldrive.InvocationRequest{ProviderID: "nonsense", RepositoryPath: dir}, getenv, nil); err == nil || !errors.Is(err, errGoalDriveDispatchDependencies) || !strings.Contains(err.Error(), "not a registered provider") || !strings.Contains(err.Error(), "praxis providers") {
		t.Fatalf("unknown provider: %v", err)
	}
	if _, err := configuredWorker(goaldrive.InvocationRequest{ProviderID: "codex", RepositoryPath: dir}, getenv, nil); err == nil || !strings.Contains(err.Error(), "codex subscription CLI is unavailable") {
		t.Fatalf("unavailable provider must be distinguishable from unknown: %v", err)
	}
	if _, err := configuredWorker(goaldrive.InvocationRequest{ProviderID: "claude", RepositoryPath: dir}, getenv, nil); err != nil {
		t.Fatalf("available provider must resolve: %v", err)
	}
	// The environment command worker accepts any identity and is listed.
	env["PRAXIS_GOAL_WORKER_ARGV"] = `["/bin/true"]`
	catalog, err = goalDriveProviderCatalog(getenv)
	if err != nil || !catalog[0].Available || catalog[0].Kind != "environment command worker (PRAXIS_GOAL_WORKER_ARGV)" || catalog[0].Executable != "/bin/true" {
		t.Fatalf("environment worker must be listed first and available: %v %+v", err, catalog[0])
	}
	if _, err := configuredWorker(goaldrive.InvocationRequest{ProviderID: "local", RepositoryPath: dir}, getenv, nil); err != nil {
		t.Fatalf("environment worker accepts any identity: %v", err)
	}
	env["PRAXIS_GOAL_WORKER_ARGV"] = `not json`
	catalog, _ = goalDriveProviderCatalog(getenv)
	var envWorker goalDriveProvider
	for _, entry := range catalog {
		if entry.ID == "<any identity>" {
			envWorker = entry
		}
	}
	if envWorker.Available || !strings.Contains(envWorker.Reason, "JSON argv") {
		t.Fatalf("malformed environment worker must be reported unavailable: %+v", envWorker)
	}
	var out bytes.Buffer
	if err := runProvidersCommand(nil, getenv, &out); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Available int                 `json:"available"`
		Providers []goalDriveProvider `json:"providers"`
		Note      string              `json:"note"`
	}
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil || listed.Available != 2 || len(listed.Providers) != 5 || !strings.Contains(listed.Note, "explicit --provider") {
		t.Fatalf("praxis providers output: %v %s", err, out.String())
	}
	if err := runProvidersCommand([]string{"extra"}, getenv, &out); err == nil {
		t.Fatal("providers takes no arguments")
	}
	var help bytes.Buffer
	if handled, err := dispatchCLIHelp([]string{"providers", "--help"}, &help); !handled || err != nil || !strings.Contains(help.String(), "usage: praxis providers") {
		t.Fatalf("providers help: %v %v %s", handled, err, help.String())
	}
	if handled, err := dispatchCLIHelp(nil, &help); !handled || err != nil || !strings.Contains(help.String(), "providers,") {
		t.Fatalf("root help must list providers: %v %v", handled, err)
	}
}

// TestDriveTemplateNamesProviderDiscovery proves a drivable generation tells
// the operator how to discover valid providers and which are available now,
// and that goal-drive without a provider points at the same command.
func TestDriveTemplateNamesProviderDiscovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fakeCLI(t, dir, "codex")
	t.Setenv("PATH", dir)
	t.Setenv("PRAXIS_GOAL_WORKER_ARGV", "")
	governed, _, _, work := lifecycleFixture(t, ctx)
	proposalDigest := proposeFixture(t, ctx, governed, work, "drive")
	reviewDigest := reviewBySelector(t, ctx, governed, work, proposalDigest, "acceptable_for_authority_decision")
	requestDigest, err := requestBySelector(t, ctx, governed, work, proposalDigest, reviewDigest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decideInteractive(t, governed, requestDigest, "approve", "DECIDE-APPROVE "+requestDigest); err != nil {
		t.Fatal(err)
	}
	acceptOut, err := executeEmitted(t, ctx, governed, acceptCommand(requestDigest))
	if err != nil {
		t.Fatal(err)
	}
	var accepted map[string]any
	if err := json.Unmarshal(acceptOut, &accepted); err != nil {
		t.Fatal(err)
	}
	if _, err := executeEmitted(t, ctx, governed, emitted(t, accepted, "attach_with")); err != nil {
		t.Fatal(err)
	}
	state := inspectGoal(t, ctx, governed, "goal:pending-surface", "2")
	template := state["drive_template"].(map[string]any)
	if template["providers_with"] != "praxis providers" {
		t.Fatalf("drive template must name provider discovery: %v", template)
	}
	available := template["available_providers"].([]any)
	if len(available) != 2 || available[0] != "codex" || available[1] != "codex-subscription" {
		t.Fatalf("drive template must list the providers available now: %v", available)
	}
	err = runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal:pending-surface", "--goal-version=2", "--invocation-id=inv-1", "--repo=" + dir, "--branch=main"}, governed)
	if err == nil || !strings.Contains(err.Error(), "provider is required") || !strings.Contains(err.Error(), "praxis providers") {
		t.Fatalf("goal-drive without a provider must point at discovery: %v", err)
	}
}
