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
		return errors.New("usage: praxis supervise {observe|comment|correction|constraint|suspend|cancel|resume|reconcile|materialize} ...")
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
	case "reconcile":
		return reconcileLostTurn(parsed, os.Stdout)
	case "materialize":
		return materializeTurnCompletion(parsed, os.Stdout)
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
	first := true
	for {
		events, err := log.Load(ctx, args.invocationID, args.turnID, cursor)
		if err != nil {
			return err
		}
		if first && len(events) == 0 && cursor == 0 {
			// Every turn records turn.allocated before its identity is
			// announced, so an empty history means the selector names no
			// durable turn of this invocation: fail closed (#155).
			return fmt.Errorf("turn %q has no durable activity for invocation %q; discover its turns with `praxis supervise observe --goal-id=%s --goal-version=%s --invocation-id=%s`", args.turnID, args.invocationID, args.goalID, args.goalVersion, args.invocationID)
		}
		first = false
		if err := verifyLineage(events, args); err != nil {
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
		// Follow semantics: replay, stay attached, emit the terminal
		// disposition, then terminate so the stream reaches EOF (#155).
		if terminalActivity(events) {
			return nil
		}
		// A turn whose process stopped renewing its lease is not running.
		// The observer says so explicitly and terminates (#163); the durable
		// disposition is recorded by reconciliation, never inferred here.
		if lost, err := lostTurnNotice(ctx, db, args); err != nil {
			return err
		} else if lost != nil {
			return json.NewEncoder(out).Encode(lost)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// lostTurnNotice reports an observer-side (non-durable) notice when the
// observed turn's admission is unreleased and its lease is no longer live.
func lostTurnNotice(ctx context.Context, db *sql.DB, args supervisionArgs) (map[string]any, error) {
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-supervision-observer", Kind: "cli"}}
	lost, _, err := goaldrive.LostTurns(ctx, ledger, state.New(db), args.goalID, args.goalVersion, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	for _, admission := range lost {
		if admission.TurnID == args.turnID && admission.InvocationID == args.invocationID {
			return map[string]any{"type": "observer.execution_lost", "goal_id": args.goalID, "goal_version": args.goalVersion, "invocation_id": args.invocationID, "turn_id": args.turnID, "pid": admission.PID, "host": admission.Host, "consequence": "unknown", "durable": false, "reconcile_with": reconcileCommand(args.goalID, args.goalVersion, args.invocationID, args.turnID)}, nil
		}
	}
	return nil, nil
}

func reconcileCommand(goalID, version, invocationID, turnID string) string {
	return "praxis supervise reconcile --goal-id=" + goalID + " --goal-version=" + version + " --invocation-id=" + invocationID + " --turn-id=" + turnID
}

// reconcileLostTurn is `praxis supervise reconcile`: the explicit path that
// closes an execution whose process disappeared (#163). It refuses a turn
// whose lease is still live, a turn already released, and a turn that
// predates admission (historical evidence needs an explicit migration).
func reconcileLostTurn(args supervisionArgs, out io.Writer) error {
	ctx := context.Background()
	db, err := openSupervisionDB(ctx, args.db, false)
	if err != nil {
		return err
	}
	defer db.Close()
	store := state.NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}
	controller := goaldrive.Controller{Ledger: goaldrive.Ledger{Store: store, Actor: actor}, Activity: &goaldrive.ActivityLog{Store: store, Actor: actor}}
	by := args.actor
	if by.ID == "" {
		by = contracts.PrincipalRef{ID: "operator", Kind: "human"}
	}
	record, err := controller.ReconcileLostTurn(ctx, state.New(db), args.goalID, args.goalVersion, args.turnID, os.Getenv("PRAXIS_GIT_REMOTE"), by)
	if err != nil {
		return err
	}
	result := map[string]any{"operation": "supervise.reconcile", "turn": record, "consequence": "unknown", "note": "the provider's external consequence is unknown; the checkout was observed at reconciliation and nothing was retried"}
	if record.EndHead != "" && (len(record.ConsequenceFiles) > 0 || len(record.ConsequenceCommits) > 0) {
		result["recover_with"] = "praxis goal-drive --goal-id=" + args.goalID + " --goal-version=" + args.goalVersion + " --mode=supervised --recover-turn=" + record.TurnID + " --provider=<registered provider> --invocation-id=<new durable invocation identity> --repo=<repository path> --branch=<exact branch>"
	}
	return json.NewEncoder(out).Encode(result)
}

// verifyLineage fails closed when a durable event under the selected
// invocation and turn belongs to a different Goal generation than the one
// the operator named.
func verifyLineage(events []goaldrive.ActivityRecord, args supervisionArgs) error {
	for _, event := range events {
		if event.GoalID != args.goalID || event.GoalVersion != args.goalVersion {
			return fmt.Errorf("turn %q of invocation %q belongs to %s/%s, not %s/%s", event.TurnID, event.InvocationID, event.GoalID, event.GoalVersion, args.goalID, args.goalVersion)
		}
	}
	return nil
}

// terminalActivity reports whether a batch carries the turn's terminal
// disposition. The controller ends every turn with execution.state_changed
// (CONTINUE, COMPLETE, NO_PROGRESS, blocked); a turn refused before
// dispatch ends with capability.unsatisfiable; a human intervention ends it
// with cancelled or suspended.
func terminalActivity(events []goaldrive.ActivityRecord) bool {
	for _, event := range events {
		switch event.Type {
		case goaldrive.ActivityExecutionStateChanged, goaldrive.ActivityCapabilityUnsatisfied, goaldrive.ActivityCancelled, goaldrive.ActivitySuspended:
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
	if typ == goaldrive.ActivitySuspendRequested || typ == goaldrive.ActivityCancelRequested {
		events, loadErr := log.Load(ctx, args.invocationID, args.turnID, 0)
		if loadErr != nil {
			return loadErr
		}
		for _, event := range events {
			if event.Type == goaldrive.ActivityTurnAllocated && event.Data["safety_kernel"] == contracts.WorkPlanSafetyKernelVersion {
				return errors.New("unauthenticated suspend/cancel is forbidden for a safety-bearing turn before Gate C authority")
			}
		}
	}
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
			if err := verifyLineage(events, args); err != nil {
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

// materializeTurnCompletion is `praxis supervise materialize`: deterministic
// re-materialization of the UnitCompletion a qualified, published historical
// turn earned but never recorded (#164). It is not settlement: no judgment
// is exercised and no new evidence is created. The repository scope comes
// from the turn's own admission, never from the operator.
func materializeTurnCompletion(args supervisionArgs, out io.Writer) error {
	ctx := context.Background()
	store, db, err := openGovernedRepository(ctx, os.Getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	baseline, err := store.Load(ctx, args.goalID, args.goalVersion, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("load exact Goal generation: %w", err)
	}
	events := state.NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}
	controller := goaldrive.Controller{Ledger: goaldrive.Ledger{Store: events, Actor: actor}, Activity: &goaldrive.ActivityLog{Store: events, Actor: actor}}
	admissions, err := controller.Ledger.LoadAdmissions(ctx, args.goalID, args.goalVersion)
	if err != nil {
		return err
	}
	var admission *goaldrive.TurnAdmission
	for i := range admissions.Admissions {
		if admissions.Admissions[i].TurnID == args.turnID {
			admission = &admissions.Admissions[i]
		}
	}
	if admission == nil {
		return fmt.Errorf("turn %s predates admission for Goal %s/%s; its repository scope is unknown and nothing is materialized", args.turnID, args.goalID, args.goalVersion)
	}
	if admission.InvocationID != args.invocationID {
		return fmt.Errorf("turn %s belongs to invocation %s, not %s", args.turnID, admission.InvocationID, args.invocationID)
	}
	dir, branch := goaldrive.ScopeLocation(admission.Scope)
	remote := os.Getenv("PRAXIS_GIT_REMOTE")
	if remote == "" {
		remote = "origin"
	}
	by := args.actor
	if by.ID == "" {
		by = contracts.PrincipalRef{ID: "operator", Kind: "human"}
	}
	result, err := controller.MaterializeTurnCompletion(ctx, args.goalID, args.goalVersion, args.turnID, baseline, goaldrive.GitRepository{Dir: dir, Remote: remote, Branch: branch}, by)
	if err != nil {
		return err
	}
	output := map[string]any{
		"operation":       "supervise.materialize",
		"semantics":       "deterministic re-materialization of controller state from the exact published consequence of the turn; no judgment exercised, no new evidence created, historical turn unchanged",
		"turn":            result.Turn,
		"completion":      result.Completion,
		"goal_candidate":  result.GoalCandidate,
		"goal_evaluation": result.GoalEvaluation,
		"inspect_with":    "praxis goals-lifecycle --operation=inspect --goal-id=" + args.goalID + " --goal-version=" + args.goalVersion,
	}
	return json.NewEncoder(out).Encode(output)
}
