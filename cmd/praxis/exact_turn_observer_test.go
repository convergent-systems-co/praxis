package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// continueTurn is the activity sequence of a turn whose disposition is
// CONTINUE: it ends with execution.state_changed and never carries
// completion.qualified. The live weather recovery turn had this shape (#155).
var continueTurn = []goaldrive.ActivityType{goaldrive.ActivityTurnAllocated, goaldrive.ActivityExecutionStarted, goaldrive.ActivityWorkSelected, goaldrive.ActivityActionStarted, goaldrive.ActivityActionCompleted, goaldrive.ActivityValidationStarted, goaldrive.ActivityValidationCompleted, goaldrive.ActivityWorkProgress, goaldrive.ActivityCheckpointCreated, goaldrive.ActivityExecutionStateChanged}

func observeWithin(t *testing.T, args supervisionArgs, limit time.Duration) ([]byte, error) {
	t.Helper()
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- observeSupervision(args, &out) }()
	select {
	case err := <-done:
		return out.Bytes(), err
	case <-time.After(limit):
		t.Fatalf("observer did not terminate within %s; output so far: %v", limit, eventTypes(out.Bytes()))
		return nil, nil
	}
}

// TestExactTurnFollowReplaysStreamsAndTerminatesOnDisposition is the
// regression test for #155: the exact-turn follower replays durable events
// immediately, streams later ones, emits the terminal disposition, and
// terminates (so a pipeline reading to EOF sees the stream).
func TestExactTurnFollowReplaysStreamsAndTerminatesOnDisposition(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log := goaldrive.ActivityLog{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	exact := supervisionArgs{db: dbPath, goalID: "goal:observe", goalVersion: "2", invocationID: "inv-exact", turnID: "inv-exact:turn:1"}

	// G: unknown turn fails closed, with and without --follow.
	if _, err := observeWithin(t, exact, 5*time.Second); err == nil || !strings.Contains(err.Error(), "no durable activity") {
		t.Fatalf("unknown turn must fail closed: %v", err)
	}
	following := exact
	following.follow = true
	if _, err := observeWithin(t, following, 5*time.Second); err == nil || !strings.Contains(err.Error(), "no durable activity") {
		t.Fatalf("unknown turn must fail closed under --follow: %v", err)
	}

	emitDone := make(chan struct{})
	go func() {
		defer close(emitDone)
		emitTurn(t, ctx, log, "inv-exact", "inv-exact:turn:1", continueTurn, 200*time.Millisecond)
	}()
	time.Sleep(900 * time.Millisecond)
	// A: attach after several events exist; without --follow the durable
	// prefix is emitted immediately and the command returns.
	snapshot, err := observeWithin(t, exact, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if types := eventTypes(snapshot); len(types) < 3 || types[0] != "turn.allocated" || strings.Contains(strings.Join(types, ","), "execution.state_changed") {
		t.Fatalf("mid-execution snapshot must show the durable prefix only: %v", types)
	}
	// B, C: with --follow the historical prefix is replayed, later events
	// stream, the terminal disposition is emitted, and the follower exits.
	start := time.Now()
	followed, err := observeWithin(t, following, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	<-emitDone
	joined := strings.Join(eventTypes(followed), ",")
	want := make([]string, 0, len(continueTurn))
	for _, typ := range continueTurn {
		want = append(want, string(typ))
	}
	if joined != strings.Join(want, ",") {
		t.Fatalf("follow must emit the full ordered history through the disposition: %v", eventTypes(followed))
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("follower must terminate promptly after the terminal disposition")
	}
	// D: attaching after completion reconstructs the full history, and the
	// follower terminates immediately because the disposition is durable.
	again, err := observeWithin(t, following, 5*time.Second)
	if err != nil || strings.Join(eventTypes(again), ",") != joined {
		t.Fatalf("post-completion follow must reconstruct and exit: %v %v", err, eventTypes(again))
	}
	// H: a lineage that does not match the durable events fails closed.
	foreign := exact
	foreign.goalVersion = "3"
	if _, err := observeWithin(t, foreign, 5*time.Second); err == nil || !strings.Contains(err.Error(), "belongs to goal:observe/2") {
		t.Fatalf("mismatched lineage must fail closed: %v", err)
	}
	foreignInvocation := exact
	foreignInvocation.invocationID = "inv-other"
	if _, err := observeWithin(t, foreignInvocation, 5*time.Second); err == nil || !strings.Contains(err.Error(), "no durable activity") {
		t.Fatalf("a turn named under the wrong invocation must fail closed: %v", err)
	}
	// E: invocation scope is the discovery plus union of its turns.
	invocation := supervisionArgs{db: dbPath, goalID: "goal:observe", goalVersion: "2", invocationID: "inv-exact"}
	union, err := observeWithin(t, invocation, 5*time.Second)
	if err != nil || strings.Join(eventTypes(union), ",") != "invocation.turn_discovered,"+joined {
		t.Fatalf("invocation observation must equal discovery plus the turn: %v %v", err, eventTypes(union))
	}
	invocationFollow := invocation
	invocationFollow.follow = true
	if _, err := observeWithin(t, invocationFollow, 10*time.Second); err != nil {
		t.Fatalf("invocation follow must terminate after the CONTINUE disposition: %v", err)
	}
	// I: the literal observe_with string goal-drive emits is executed as is.
	var announced bytes.Buffer
	announceGoalDriveTurn(&announced, normalizedOutput{}, goaldrive.InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:observe"}, GoalVersion: "2", InvocationID: "inv-exact", ProviderID: "local", Mode: goaldrive.ModeSupervised}, "inv-exact:turn:1")
	var announcement map[string]any
	if err := json.Unmarshal(announced.Bytes(), &announcement); err != nil {
		t.Fatal(err)
	}
	literal := announcement["observe_with"].(string)
	fields := strings.Fields(literal)
	if fields[0] != "praxis" || fields[1] != "supervise" {
		t.Fatalf("emitted command is not a praxis supervise invocation: %s", literal)
	}
	t.Setenv("PRAXIS_DB", dbPath)
	// The follower terminates on the durable disposition, so the literal
	// command runs synchronously; a regression here shows as a test timeout.
	var literalErr error
	emitted := captureStdout(t, func() { literalErr = runSuperviseCommand(fields[2:]) })
	if literalErr != nil {
		t.Fatalf("literal observe_with must work: %s: %v", literal, literalErr)
	}
	if strings.Join(eventTypes(emitted), ",") != joined {
		t.Fatalf("literal observe_with must observe exactly that turn: %v", eventTypes(emitted))
	}
}
