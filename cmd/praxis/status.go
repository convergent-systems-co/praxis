package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/state"
)

type runStatusOutput struct {
	RunID            string             `json:"run_id"`
	GraphID          string             `json:"graph_id"`
	GraphVersion     string             `json:"graph_version"`
	CurrentNode      string             `json:"current_node"`
	State            kernel.RunState    `json:"state"`
	TransitionCount  int                `json:"transition_count"`
	AggregateVersion int64              `json:"aggregate_version"`
	Evidence         []string           `json:"evidence,omitempty"`
	PendingWait      *kernel.Suspension `json:"pending_wait,omitempty"`
}

func statusCommand(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	if len(args) == 0 || args[0] == "" {
		return errors.New("usage: praxis status <run-id> [--db <path>]")
	}
	runID := args[0]
	dbPath, err := statusDBPath(args[1:], getenv)
	if err != nil {
		return err
	}
	db, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	status, err := (kernel.RunControl{Store: state.NewSQLiteEventStore(db)}).Status(ctx, runID)
	if err != nil {
		return err
	}
	result := runStatusOutput{
		RunID:            status.Run.RunID,
		GraphID:          status.Run.GraphID,
		GraphVersion:     status.Run.GraphVersion,
		CurrentNode:      status.Run.CurrentNode,
		State:            status.Run.State,
		TransitionCount:  status.Run.TransitionCount,
		AggregateVersion: status.AggregateVersion,
		Evidence:         append([]string(nil), status.Run.Evidence...),
		PendingWait:      status.Run.PendingWait,
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(encoded))
	return err
}

func statusDBPath(args []string, getenv func(string) string) (string, error) {
	var path string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--db":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", errors.New("--db requires a path")
			}
			path = args[i+1]
			i++
		case strings.HasPrefix(arg, "--db="):
			path = strings.TrimPrefix(arg, "--db=")
			if path == "" {
				return "", errors.New("--db requires a path")
			}
		default:
			return "", fmt.Errorf("unknown status option %q", arg)
		}
	}
	if path == "" && getenv != nil {
		path = getenv("PRAXIS_DB")
	}
	if path == "" {
		return "", errors.New("Praxis database must be explicit: use --db <path> or PRAXIS_DB")
	}
	return path, nil
}

func runStatus(args []string) error {
	return statusCommand(context.Background(), args, os.Stdout, os.Getenv)
}
