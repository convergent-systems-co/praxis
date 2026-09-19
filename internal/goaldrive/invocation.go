package goaldrive

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type InvocationRequest struct {
	Input           contracts.GoalInput
	GoalVersion     string
	Mode            ExecutionMode
	InvocationID    string
	ProviderID      string
	Model           string
	RepositoryPath  string
	Branch          string
	MaxTurns        int
	RecoverTurn     string
	TurnTimeout     time.Duration
	NoProgressLimit int
	LedgerPath      string
	NoPush          bool
	RequireClean    bool
}

// ParseInvocation normalizes the registered Goal-drive option map. It is a
// parser only: it does not create a Goal, authorize a provider, or invoke a
// worker.
func ParseInvocation(options map[string]string) (InvocationRequest, error) {
	if options["goal"] != "" || options["goal-file"] != "" {
		return InvocationRequest{}, ErrGoalInputNotDriveable
	}
	if options["goal-id"] == "" {
		return InvocationRequest{}, errors.New("Goal-drive requires an existing durable Goal: --goal-id and --goal-version")
	}
	input, err := contracts.ResolveGoalInput("", "", options["goal-id"])
	if err != nil {
		return InvocationRequest{}, err
	}
	provider := options["provider"]
	if provider == "" {
		return InvocationRequest{}, errors.New("Goal-drive provider is required")
	}
	mode := ExecutionMode(options["mode"])
	if mode == "" {
		mode = ModeSupervised
	}
	if mode != ModeSupervised && mode != ModeContinuous {
		return InvocationRequest{}, fmt.Errorf("mode must be supervised or continuous: %q", mode)
	}
	invocationID := options["invocation-id"]
	if invocationID == "" {
		return InvocationRequest{}, errors.New("Goal-drive invocation-id is required")
	}
	out := InvocationRequest{Input: input, GoalVersion: options["goal-version"], Mode: mode, InvocationID: invocationID, ProviderID: provider, Model: options["model"], RepositoryPath: options["repo"], Branch: options["branch"], LedgerPath: options["ledger"], RequireClean: true, RecoverTurn: options["recover-turn"]}
	if value := options["max-turns"]; value != "" {
		out.MaxTurns, err = positiveInt("max-turns", value)
		if err != nil {
			return InvocationRequest{}, err
		}
	}
	if value := options["turn-timeout"]; value != "" {
		out.TurnTimeout, err = time.ParseDuration(value)
		if err != nil || out.TurnTimeout <= 0 {
			return InvocationRequest{}, fmt.Errorf("turn-timeout must be a positive duration: %q", value)
		}
	}
	if value := options["no-progress-limit"]; value != "" {
		out.NoProgressLimit, err = positiveInt("no-progress-limit", value)
		if err != nil {
			return InvocationRequest{}, err
		}
	}
	if value := options["no-push"]; value != "" {
		out.NoPush, err = strconv.ParseBool(value)
		if err != nil {
			return InvocationRequest{}, fmt.Errorf("no-push must be boolean: %q", value)
		}
	}
	if value := options["require-clean"]; value != "" {
		out.RequireClean, err = strconv.ParseBool(value)
		if err != nil {
			return InvocationRequest{}, fmt.Errorf("require-clean must be boolean: %q", value)
		}
	}
	return out, nil
}

func positiveInt(name, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer: %q", name, value)
	}
	return parsed, nil
}
