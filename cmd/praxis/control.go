package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	runcontrolsvc "github.com/convergent-systems-co/praxis/internal/runcontrol"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type controlArgs struct {
	runID     string
	dbPath    string
	actor     contracts.PrincipalRef
	waitKind  kernel.WaitKind
	waitRef   string
}

func runControlCommand(operation string, args []string) error {
	parsed, err := parseControlArgs(operation, args, os.Getenv)
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, parsed.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	store := state.New(db)
	control := kernel.RunControl{
		Store:     state.NewSQLiteEventStore(db),
		Actor:     parsed.actor,
		Committer: runcontrolsvc.SQLiteCommitter{State: store},
	}
	commandID, err := randomID("cmd")
	if err != nil {
		return err
	}
	correlationID, err := randomID("corr")
	if err != nil {
		return err
	}
	var status kernel.RunStatus
	switch operation {
	case "cancel":
		status, err = control.Cancel(ctx, parsed.runID, commandID, correlationID)
	case "resume":
		status, err = control.ResumeSignal(ctx, parsed.runID, parsed.waitKind, parsed.waitRef, commandID, correlationID)
	default:
		return fmt.Errorf("unsupported run control operation %q", operation)
	}
	if err != nil {
		return err
	}
	return printJSON(runStatusOutput{
		RunID: status.Run.RunID, GraphID: status.Run.GraphID, GraphVersion: status.Run.GraphVersion,
		CurrentNode: status.Run.CurrentNode, State: status.Run.State,
		TransitionCount: status.Run.TransitionCount, AggregateVersion: status.AggregateVersion,
		Evidence: append([]string(nil), status.Run.Evidence...), PendingWait: status.Run.PendingWait,
	})
}

func parseControlArgs(operation string, args []string, getenv func(string) string) (controlArgs, error) {
	if len(args) == 0 || args[0] == "" {
		return controlArgs{}, fmt.Errorf("usage: praxis %s <run-id> --actor-id <id> --actor-kind <kind> [--db <path>]", operation)
	}
	out := controlArgs{runID: args[0]}
	if getenv != nil {
		out.dbPath = getenv("PRAXIS_DB")
		out.actor.ID = getenv("PRAXIS_ACTOR_ID")
		out.actor.Kind = getenv("PRAXIS_ACTOR_KIND")
	}
	for i := 1; i < len(args); i++ {
		key, value, consumed, err := controlOption(args, i)
		if err != nil {
			return controlArgs{}, err
		}
		i += consumed
		switch key {
		case "db":
			out.dbPath = value
		case "actor-id":
			out.actor.ID = value
		case "actor-kind":
			out.actor.Kind = value
		case "wait-kind":
			out.waitKind = kernel.WaitKind(value)
		case "wait-ref":
			out.waitRef = value
		default:
			return controlArgs{}, fmt.Errorf("unknown %s option --%s", operation, key)
		}
	}
	if out.dbPath == "" {
		return controlArgs{}, errors.New("Praxis database must be explicit: use --db <path> or PRAXIS_DB")
	}
	if err := out.actor.Validate(); err != nil {
		return controlArgs{}, fmt.Errorf("run-control actor: %w", err)
	}
	if operation == "resume" {
		if out.waitRef == "" {
			return controlArgs{}, errors.New("resume requires --wait-ref matching the persisted suspension")
		}
		if err := (kernel.Suspension{Kind: out.waitKind, Ref: out.waitRef}).Validate(); err != nil {
			return controlArgs{}, fmt.Errorf("resume wait: %w", err)
		}
	}
	return out, nil
}

func controlOption(args []string, i int) (key, value string, consumed int, err error) {
	raw := args[i]
	if !strings.HasPrefix(raw, "--") {
		return "", "", 0, fmt.Errorf("unexpected argument %q", raw)
	}
	body := strings.TrimPrefix(raw, "--")
	if parts := strings.SplitN(body, "=", 2); len(parts) == 2 {
		if parts[0] == "" || parts[1] == "" {
			return "", "", 0, fmt.Errorf("invalid option %q", raw)
		}
		return parts[0], parts[1], 0, nil
	}
	if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
		return "", "", 0, fmt.Errorf("option %q requires a value", raw)
	}
	return body, args[i+1], 1, nil
}

func randomID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(b[:]), nil
}
