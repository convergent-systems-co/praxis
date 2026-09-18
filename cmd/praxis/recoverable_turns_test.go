package main

import (
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

// TestRecoverableTurnsNameExactBlockedTurns proves inspect emits a recovery
// template only for BLOCKED turns without checkpoint whose objective has not
// progressed since, and that the template binds the exact turn identity.
func TestRecoverableTurnsNameExactBlockedTurns(t *testing.T) {
	baseline := goals.GoalBaseline{ID: "goal:weather-app", Version: "2"}
	turns := []goaldrive.TurnRecord{
		{TurnID: "live-001:turn:1", InvocationID: "live-001", ChildObjective: "unit:a", Outcome: goaldrive.OutcomeBlocked, EndHead: "0ac6bb8", Blocker: "provider left repository with uncommitted changes"},
		{TurnID: "live-002:turn:2", InvocationID: "live-002", ChildObjective: "unit:b", Outcome: goaldrive.OutcomeBlocked},
		{TurnID: "live-003:turn:3", InvocationID: "live-003", ChildObjective: "unit:b", Outcome: goaldrive.OutcomeContinue, Progress: true},
		{TurnID: "live-004:turn:4", InvocationID: "live-004", ChildObjective: "unit:c", Outcome: goaldrive.OutcomeContinue, Progress: true},
		{TurnID: "live-005:turn:5", InvocationID: "live-005", ChildObjective: "unit:d", Outcome: goaldrive.OutcomeBlocked, Blocker: "worker crashed"},
		// A second block on unit:b after its progress is recoverable: only
		// turns recorded after a blocked turn can supersede it.
		{TurnID: "live-006:turn:6", InvocationID: "live-006", ChildObjective: "unit:b", Outcome: goaldrive.OutcomeBlocked, EndHead: "c778fe5"},
	}
	out := recoverableTurns(baseline, turns)
	if len(out) != 3 || out[0]["turn_id"] != "live-001:turn:1" || out[0]["recoverable"] != true || out[1]["turn_id"] != "live-005:turn:5" || out[1]["recoverable"] != false || out[1]["recover_template"] != nil || out[2]["turn_id"] != "live-006:turn:6" || out[2]["recoverable"] != true {
		t.Fatalf("unprogressed blocked turns are listed in ledger order; supersession counts later turns only: %v", out)
	}
	template := out[0]["recover_template"].(map[string]any)
	command := template["command"].(string)
	for _, want := range []string{"praxis goal-drive", "--goal-id=goal:weather-app", "--goal-version=2", "--mode=supervised", "--recover-turn=live-001:turn:1"} {
		if !strings.Contains(command, want) {
			t.Fatalf("recover template lacks %q: %s", want, command)
		}
	}
	if placeholderPattern.MatchString(command) {
		t.Fatalf("the durable part of the template carries no placeholder: %s", command)
	}
	if supplies := template["operator_supplies"].([]string); len(supplies) != 4 || !strings.HasPrefix(supplies[1], "--invocation-id=") {
		t.Fatalf("operator intent is named, not invented: %v", supplies)
	}
}

// TestEnvironmentWorkerDeclaredCapabilitiesGateDispatch proves the operator
// can declare what the environment worker can actually do, that goal-drive
// resolves the declaration exactly as the providers catalog reports it, and
// that a malformed or unknown declaration fails closed instead of widening.
func TestEnvironmentWorkerDeclaredCapabilitiesGateDispatch(t *testing.T) {
	env := map[string]string{"PRAXIS_GOAL_WORKER_ARGV": `["/bin/true"]`, "PRAXIS_GOAL_WORKER_CAPABILITIES": `["edit"]`}
	getenv := func(key string) string { return env[key] }
	worker, err := configuredWorker(goaldrive.InvocationRequest{ProviderID: "local", RepositoryPath: t.TempDir()}, getenv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := goaldrive.CheckCapabilities(worker, "local", goaldrive.RequiredRepositoryCapabilities()); err == nil || !strings.Contains(err.Error(), "stage") || !strings.Contains(err.Error(), "commit") {
		t.Fatalf("edit-only worker must be refused before dispatch: %v", err)
	}
	catalog, err := goalDriveProviderCatalog(getenv)
	if err != nil || len(catalog) == 0 || strings.Join(catalog[0].Capabilities, ",") != "edit" || strings.Join(catalog[0].Required, ",") != "edit,stage,commit" {
		t.Fatalf("catalog must report the declaration and the requirement: %v %+v", err, catalog[0])
	}
	env["PRAXIS_GOAL_WORKER_CAPABILITIES"] = `["edit","push"]`
	if _, err := configuredWorker(goaldrive.InvocationRequest{ProviderID: "local", RepositoryPath: t.TempDir()}, getenv, nil); err == nil || !strings.Contains(err.Error(), `unknown worker capability "push"`) {
		t.Fatalf("unknown capability must fail closed: %v", err)
	}
	catalog, _ = goalDriveProviderCatalog(getenv)
	for _, entry := range catalog {
		if entry.ID == "<any identity>" && (entry.Available || !strings.Contains(entry.Reason, "PRAXIS_GOAL_WORKER_CAPABILITIES")) {
			t.Fatalf("catalog must report the malformed declaration as unavailable: %+v", entry)
		}
	}
	delete(env, "PRAXIS_GOAL_WORKER_CAPABILITIES")
	worker, _ = configuredWorker(goaldrive.InvocationRequest{ProviderID: "local", RepositoryPath: t.TempDir()}, getenv, nil)
	if err := goaldrive.CheckCapabilities(worker, "local", goaldrive.RequiredRepositoryCapabilities()); err != nil {
		t.Fatalf("undeclared environment worker asserts the full contract: %v", err)
	}
}
