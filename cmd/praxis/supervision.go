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
	if args[0] != "observe" && parsed.turnID == "" {
		return errors.New("interventions require the exact --turn-id; discover it with `praxis supervise observe --goal-id=<id> --goal-version=<version> --invocation-id=<invocation>`")
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
	if out.goalID == "" || out.goalVersion == "" || out.invocationID == "" {
		return supervisionArgs{}, errors.New("exact --goal-id, --goal-version, and --invocation-id are required")
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
	if args.turnID == "" {
		return observeInvocation(ctx, log, args, out)
	}
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

// observeInvocation composes observation from an identity the operator
// already holds: it discovers the invocation's durable turns, streams each
// turn's events in order, and with --follow keeps discovering turns that
// start later (continuous mode) until the newest turn reaches a terminal
// activity and no newer turn appears.
func observeInvocation(ctx context.Context, log goaldrive.ActivityLog, args supervisionArgs, out io.Writer) error {
	encoder := json.NewEncoder(out)
	cursors := map[string]int64{}
	known := []string{}
	announce := func(turns []string) error {
		for _, turn := range turns {
			if _, ok := cursors[turn]; ok {
				continue
			}
			cursors[turn] = 0
			known = append(known, turn)
			if err := encoder.Encode(map[string]any{"type": "invocation.turn_discovered", "invocation_id": args.invocationID, "turn_id": turn, "observe_with": "praxis supervise observe --goal-id=" + args.goalID + " --goal-version=" + args.goalVersion + " --invocation-id=" + args.invocationID + " --turn-id=" + turn + " --follow"}); err != nil {
				return err
			}
		}
		return nil
	}
	terminal := map[string]bool{}
	idleAfterTerminal := 0
	for {
		turns, err := log.ListTurns(ctx, args.invocationID)
		if err != nil {
			return err
		}
		knownBefore := len(known)
		if err := announce(turns); err != nil {
			return err
		}
		if len(known) == 0 && !args.follow {
			return encoder.Encode(map[string]any{"type": "invocation.no_turns", "invocation_id": args.invocationID})
		}
		progressed := len(known) != knownBefore
		for _, turn := range known {
			events, err := log.Load(ctx, args.invocationID, turn, cursors[turn])
			if err != nil {
				return err
			}
			for _, event := range events {
				if err := encoder.Encode(event); err != nil {
					return err
				}
				cursors[turn] = event.StreamVersion
			}
			if len(events) > 0 {
				progressed = true
			}
			if terminalActivity(events) {
				terminal[turn] = true
			}
		}
		if !args.follow {
			return nil
		}
		// Stop once the newest turn has reached a terminal activity and no
		// further events or turns appear for a short grace period; a
		// continuous invocation starts its next turn well within it.
		if len(known) > 0 && terminal[known[len(known)-1]] && !progressed {
			idleAfterTerminal++
			if idleAfterTerminal > 10 {
				return nil
			}
		} else {
			idleAfterTerminal = 0
		}
		time.Sleep(150 * time.Millisecond)
	}
}
