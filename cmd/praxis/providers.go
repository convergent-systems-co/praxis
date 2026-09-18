package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os/exec"
	"sort"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
)

// goalDriveProvider is one entry of the control-plane provider catalog that
// goal-drive resolves a --provider identity against. Providers are not
// installation state: first-party subscription profiles are compiled into
// the kernel and are available only when their CLI is on PATH, and an
// environment-defined command worker (PRAXIS_GOAL_WORKER_ARGV) accepts any
// identity the operator names. The catalog is the public, read-only answer
// to "which --provider values are valid here"; it never selects one.
type goalDriveProvider struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Available   bool   `json:"available"`
	Reason      string `json:"reason,omitempty"`
	Executable  string `json:"executable,omitempty"`
	Model       string `json:"model"`
	DriveOption string `json:"drive_option"`
	// Capabilities are the consequences the worker's launch contract grants
	// (edit, validate, stage, commit). goal-drive refuses, before any
	// execution, a turn whose checkpoint contract needs more.
	Capabilities []string `json:"capabilities"`
	Required     []string `json:"required_by_repository_turn"`
}

var firstPartyProviderProfiles = []struct {
	id, cli      string
	capabilities []goaldrive.WorkerCapability
}{
	{"claude", "claude", goaldrive.ClaudeSubscriptionCapabilities()}, {"claude-subscription", "claude", goaldrive.ClaudeSubscriptionCapabilities()},
	{"codex", "codex", goaldrive.CodexSubscriptionCapabilities()}, {"codex-subscription", "codex", goaldrive.CodexSubscriptionCapabilities()},
}

func capabilityNames(items []goaldrive.WorkerCapability) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	return out
}

// goalDriveProviderCatalog resolves availability exactly as goal-drive will
// at execution time: the same PATH lookup and the same environment variable.
func goalDriveProviderCatalog(getenv func(string) string) ([]goalDriveProvider, error) {
	var out []goalDriveProvider
	if encoded := getenv("PRAXIS_GOAL_WORKER_ARGV"); encoded != "" {
		var argv []string
		entry := goalDriveProvider{ID: "<any identity>", Kind: "environment command worker (PRAXIS_GOAL_WORKER_ARGV)", Model: "ignored", DriveOption: "--provider=<the identity you choose; recorded as the turn's executor>", Capabilities: capabilityNames(goaldrive.CommandWorker{}.Capabilities()), Required: capabilityNames(goaldrive.RequiredRepositoryCapabilities())}
		if err := json.Unmarshal([]byte(encoded), &argv); err != nil || len(argv) == 0 || argv[0] == "" {
			entry.Reason = "PRAXIS_GOAL_WORKER_ARGV must be a non-empty JSON argv array"
		} else {
			entry.Available, entry.Executable = true, argv[0]
		}
		if declared := getenv("PRAXIS_GOAL_WORKER_CAPABILITIES"); declared != "" {
			granted, err := goaldrive.ParseCapabilities(declared)
			if err != nil {
				entry.Available, entry.Reason = false, "PRAXIS_GOAL_WORKER_CAPABILITIES: "+err.Error()
			} else {
				entry.Capabilities = capabilityNames(granted)
			}
		}
		out = append(out, entry)
	}
	for _, profile := range firstPartyProviderProfiles {
		entry := goalDriveProvider{ID: profile.id, Kind: "first-party subscription profile", Model: "optional --model hint passed to the " + profile.cli + " CLI", DriveOption: "--provider=" + profile.id, Capabilities: capabilityNames(profile.capabilities), Required: capabilityNames(goaldrive.RequiredRepositoryCapabilities())}
		if path, err := exec.LookPath(profile.cli); err != nil {
			entry.Reason = profile.cli + " CLI is not on PATH"
		} else {
			entry.Available, entry.Executable = true, path
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Available && !out[j].Available })
	return out, nil
}

func availableProviderIDs(getenv func(string) string) []string {
	catalog, err := goalDriveProviderCatalog(getenv)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(catalog))
	for _, entry := range catalog {
		if entry.Available {
			ids = append(ids, entry.ID)
		}
	}
	return ids
}

// runProvidersCommand is `praxis providers`: it lists the goal-drive provider
// catalog with availability and the exact --provider option each accepts.
func runProvidersCommand(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("providers", flag.ContinueOnError)
	f.SetOutput(out)
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("usage: praxis providers")
	}
	catalog, err := goalDriveProviderCatalog(getenv)
	if err != nil {
		return err
	}
	available := 0
	for _, entry := range catalog {
		if entry.Available {
			available++
		}
	}
	return printJSONTo(out, map[string]any{"operation": "providers", "available": available, "providers": catalog, "note": "goal-drive requires an explicit --provider; Praxis never selects one implicitly. --model is an optional hint interpreted by the provider CLI."})
}
