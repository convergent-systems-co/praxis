package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type supervisionArgs struct {
	db, goalID, goalVersion, invocationID, turnID, providerID, text string
	actor                                                           contracts.PrincipalRef
	after                                                           int64
	follow                                                          bool
}

func runSuperviseCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: praxis supervise {observe|comment|correction|constraint|suspend|cancel|resume} ...")
	}
	parsed, err := parseSupervisionArgs(args[1:], os.Getenv)
	if err != nil {
		return err
	}
	switch args[0] {
	case "observe":
		return observeSupervision(parsed, os.Stdout)
	case "comment", "correction", "constraint":
		return appendHumanIntervention(parsed, goaldrive.ActivityType("human."+args[0]))
	case "suspend":
		return appendHumanIntervention(parsed, goaldrive.ActivitySuspendRequested)
	case "cancel":
		return appendHumanIntervention(parsed, goaldrive.ActivityCancelRequested)
	case "resume":
		return appendHumanIntervention(parsed, goaldrive.ActivityHumanCorrection)
	default:
		return fmt.Errorf("unknown supervision operation %q", args[0])
	}
}

func parseSupervisionArgs(args []string, getenv func(string) string) (supervisionArgs, error) {
	out := supervisionArgs{}
	if getenv != nil {
		out.db = getenv("PRAXIS_DB")
		out.actor = contracts.PrincipalRef{ID: getenv("PRAXIS_ACTOR_ID"), Kind: getenv("PRAXIS_ACTOR_KIND")}
	}
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "--") {
			return supervisionArgs{}, fmt.Errorf("unexpected argument %q", args[i])
		}
		key, value, consumed, err := supervisionOption(args, i)
		if err != nil {
			return supervisionArgs{}, err
		}
		i += consumed
		switch key {
		case "db":
			out.db = value
		case "goal-id":
			out.goalID = value
		case "goal-version":
			out.goalVersion = value
		case "invocation-id":
			out.invocationID = value
		case "turn-id":
			out.turnID = value
		case "provider-id":
			out.providerID = value
		case "actor-id":
			out.actor.ID = value
		case "actor-kind":
			out.actor.Kind = value
		case "text":
			out.text = value
		case "after":
			parsed, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil || parsed < 0 {
				return supervisionArgs{}, errors.New("--after must be a non-negative integer")
			}
			out.after = parsed
		case "follow":
			out.follow = true
			if value != "" {
				parsed, parseErr := strconv.ParseBool(value)
				if parseErr != nil {
					return supervisionArgs{}, errors.New("--follow must be boolean")
				}
				out.follow = parsed
			}
		default:
			return supervisionArgs{}, fmt.Errorf("unknown supervision option --%s", key)
		}
	}
	if out.db == "" {
		return supervisionArgs{}, errors.New("Praxis database must be explicit: use --db or PRAXIS_DB")
	}
	if out.goalID == "" || out.goalVersion == "" || out.invocationID == "" || out.turnID == "" {
		return supervisionArgs{}, errors.New("exact --goal-id, --goal-version, --invocation-id, and --turn-id are required")
	}
	return out, nil
}

func supervisionOption(args []string, i int) (string, string, int, error) {
	body := strings.TrimPrefix(args[i], "--")
	if body == "" {
		return "", "", 0, errors.New("invalid supervision option")
	}
	if parts := strings.SplitN(body, "=", 2); len(parts) == 2 {
		return parts[0], parts[1], 0, nil
	}
	if body == "follow" {
		return body, "true", 0, nil
	}
	if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
		return "", "", 0, fmt.Errorf("option %q requires a value", args[i])
	}
	return body, args[i+1], 1, nil
}

func openSupervisionDB(ctx context.Context, path string, readonly bool) (*sql.DB, error) {
	if readonly {
		return state.OpenSQLiteReadOnly(ctx, path)
	}
	return state.OpenSQLite(ctx, path)
}

func observeSupervision(args supervisionArgs, out io.Writer) error {
	ctx := context.Background()
	db, err := openSupervisionDB(ctx, args.db, true)
	if err != nil {
		return err
	}
	defer db.Close()
	log := goaldrive.ActivityLog{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-supervision-observer", Kind: "cli"}}
	cursor := args.after
	for {
		events, err := log.Load(ctx, args.invocationID, args.turnID, cursor)
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := json.NewEncoder(out).Encode(event); err != nil {
				return err
			}
			cursor = event.StreamVersion
		}
		if !args.follow {
			return nil
		}
		if terminalActivity(events) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func terminalActivity(events []goaldrive.ActivityRecord) bool {
	for _, event := range events {
		if event.Type == goaldrive.ActivityCompletionQualified || event.Type == goaldrive.ActivityCancelled || event.Type == goaldrive.ActivitySuspended || event.Type == goaldrive.ActivityBlockerDetected {
			return true
		}
	}
	return false
}

func appendHumanIntervention(args supervisionArgs, typ goaldrive.ActivityType) error {
	if err := args.actor.Validate(); err != nil {
		return fmt.Errorf("supervision actor: %w", err)
	}
	if (typ == goaldrive.ActivityHumanComment || typ == goaldrive.ActivityHumanCorrection || typ == goaldrive.ActivityHumanConstraint) && args.text == "" {
		return errors.New("human intervention requires --text")
	}
	ctx := context.Background()
	db, err := openSupervisionDB(ctx, args.db, false)
	if err != nil {
		return err
	}
	defer db.Close()
	log := goaldrive.ActivityLog{Store: state.NewSQLiteEventStore(db), Actor: args.actor}
	data := map[string]string{}
	if args.text != "" {
		data["text"] = args.text
	}
	if typ == goaldrive.ActivityHumanCorrection && args.text == "" {
		data["operation"] = "resume"
	}
	_, err = log.AppendNext(ctx, goaldrive.ActivityRecord{Type: typ, GoalID: args.goalID, GoalVersion: args.goalVersion, InvocationID: args.invocationID, TurnID: args.turnID, ProviderID: args.providerID, Actor: args.actor, Source: "human.cli", Trust: contracts.TrustUserConfirmed, Data: data})
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"type": typ, "invocation_id": args.invocationID, "turn_id": args.turnID, "accepted": true})
}
