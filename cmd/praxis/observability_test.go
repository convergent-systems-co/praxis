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

func eventTypes(raw []byte) []string {
	var types []string
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var event map[string]any
		if json.Unmarshal(line, &event) == nil {
			if typ, ok := event["type"].(string); ok {
				types = append(types, typ)
			}
		}
	}
	return types
}

func emitTurn(t *testing.T, ctx context.Context, log goaldrive.ActivityLog, invocationID, turnID string, types []goaldrive.ActivityType, gap time.Duration) {
	t.Helper()
	request := goaldrive.WorkerRequest{GoalID: "goal:observe", GoalVersion: "2", InvocationID: invocationID, TurnID: turnID, ChildObjective: "unit:observe", GraphID: "praxis.package.goals.default", GraphVersion: "0.3.0", ProviderID: "local"}
	actor := contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}
	for _, typ := range types {
		if _, err := log.Emit(ctx, typ, request, actor, contracts.TrustObserved, "praxis.controller", map[string]string{"state": string(typ)}); err != nil {
			t.Errorf("emit %s: %v", typ, err)
			return
		}
		time.Sleep(gap)
	}
}

// TestRunningTurnIsDiscoverableAndObservableByInvocation proves that an
// observer holding only the goal and invocation identities discovers the
// active turn while it is running, follows its durable events to
// completion without restarting, reconstructs the same history in a fresh
// read, and that interventions still demand the exact turn.
func TestRunningTurnIsDiscoverableAndObservableByInvocation(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log := goaldrive.ActivityLog{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	base := supervisionArgs{db: dbPath, goalID: "goal:observe", goalVersion: "2", invocationID: "inv-observe"}

	var before bytes.Buffer
	if err := observeSupervision(base, &before); err != nil || !strings.Contains(before.String(), "invocation.no_turns") {
		t.Fatalf("invocation observe before any turn: %v %s", err, before.String())
	}
	if _, err := parseSupervisionArgs([]string{"--goal-id=goal:observe", "--goal-version=2", "--invocation-id=inv-observe", "--db=" + dbPath}, nil); err != nil {
		t.Fatalf("observe must not require --turn-id: %v", err)
	}
	if err := runSuperviseCommand([]string{"suspend", "--goal-id=goal:observe", "--goal-version=2", "--invocation-id=inv-observe", "--db=" + dbPath}); err == nil || !strings.Contains(err.Error(), "exact --turn-id") {
		t.Fatalf("interventions must require the exact turn: %v", err)
	}

	// A running turn: events arrive over time; the observer starts after the
	// first ones exist and follows until the terminal activity.
	running := []goaldrive.ActivityType{goaldrive.ActivityExecutionStarted, goaldrive.ActivityWorkSelected, goaldrive.ActivityActionStarted, goaldrive.ActivityActionCompleted, goaldrive.ActivityWorkProgress, goaldrive.ActivityCheckpointCreated, goaldrive.ActivityCompletionClaimed, goaldrive.ActivityCompletionQualified, goaldrive.ActivityExecutionStateChanged}
	emitDone := make(chan struct{})
	go func() {
		defer close(emitDone)
		emitTurn(t, ctx, log, "inv-observe", "inv-observe:turn:1", running, 250*time.Millisecond)
	}()
	time.Sleep(700 * time.Millisecond)
	var snapshot bytes.Buffer
	if err := observeSupervision(base, &snapshot); err != nil {
		t.Fatal(err)
	}
	if types := eventTypes(snapshot.Bytes()); len(types) < 2 || types[0] != "invocation.turn_discovered" || strings.Contains(strings.Join(types, ","), "completion.qualified") {
		t.Fatalf("mid-execution observation must discover the turn and show activity before completion: %v", types)
	}
	follow := base
	follow.follow = true
	var followed bytes.Buffer
	if err := observeSupervision(follow, &followed); err != nil {
		t.Fatal(err)
	}
	<-emitDone
	types := eventTypes(followed.Bytes())
	joined := strings.Join(types, ",")
	if !strings.HasPrefix(joined, "invocation.turn_discovered,execution.started,work.selected,action.started") || !strings.Contains(joined, "completion.qualified") {
		t.Fatalf("follow must stream in order to completion without restarting: %v", types)
	}
	var again bytes.Buffer
	if err := observeSupervision(base, &again); err != nil {
		t.Fatal(err)
	}
	if strings.Join(eventTypes(again.Bytes()), ",") != joined {
		t.Fatalf("fresh observation must reconstruct the same history: %v vs %v", eventTypes(again.Bytes()), types)
	}
	exact := base
	exact.turnID = "inv-observe:turn:1"
	var exactOut bytes.Buffer
	if err := observeSupervision(exact, &exactOut); err != nil {
		t.Fatal(err)
	}
	if strings.Join(eventTypes(exactOut.Bytes()), ",") != strings.TrimPrefix(joined, "invocation.turn_discovered,") {
		t.Fatalf("exact-turn observation must match: %v", eventTypes(exactOut.Bytes()))
	}
	turns, err := log.ListTurns(ctx, "inv-observe")
	if err != nil || len(turns) != 1 || turns[0] != "inv-observe:turn:1" {
		t.Fatalf("ListTurns: %v %v", turns, err)
	}
	if turns, err := log.ListTurns(ctx, "inv-other"); err != nil || len(turns) != 0 {
		t.Fatalf("another invocation must not leak turns: %v %v", turns, err)
	}
}

// TestContinuousInvocationObservationFollowsSequentialTurns proves that an
// observer that started at invocation scope before later turns existed
// discovers and streams each of them, and stops after the last terminal.
func TestContinuousInvocationObservationFollowsSequentialTurns(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log := goaldrive.ActivityLog{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	turn := []goaldrive.ActivityType{goaldrive.ActivityExecutionStarted, goaldrive.ActivityWorkSelected, goaldrive.ActivityActionStarted, goaldrive.ActivityActionCompleted, goaldrive.ActivityWorkProgress, goaldrive.ActivityCheckpointCreated, goaldrive.ActivityExecutionStateChanged}
	final := append(append([]goaldrive.ActivityType{}, turn[:6]...), goaldrive.ActivityCompletionClaimed, goaldrive.ActivityCompletionQualified, goaldrive.ActivityExecutionStateChanged)
	go func() {
		emitTurn(t, ctx, log, "inv-continuous", "inv-continuous:turn:1", turn, 120*time.Millisecond)
		emitTurn(t, ctx, log, "inv-continuous", "inv-continuous:turn:2", final, 120*time.Millisecond)
	}()
	time.Sleep(300 * time.Millisecond)
	var followed bytes.Buffer
	if err := observeSupervision(supervisionArgs{db: dbPath, goalID: "goal:observe", goalVersion: "2", invocationID: "inv-continuous", follow: true}, &followed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(followed.String(), "inv-continuous:turn:1") || !strings.Contains(followed.String(), "inv-continuous:turn:2") || !strings.Contains(followed.String(), "completion.qualified") {
		t.Fatalf("observer must discover both sequential turns and their completion: %s", followed.String())
	}
	discovered := 0
	for _, typ := range eventTypes(followed.Bytes()) {
		if typ == "invocation.turn_discovered" {
			discovered++
		}
	}
	if discovered != 2 {
		t.Fatalf("expected two discovered turns: %v", eventTypes(followed.Bytes()))
	}
}

// TestGoalDriveAnnouncesTurnBeforeExecution proves the announcement carries
// the exact turn and both observation commands with full identities.
func TestGoalDriveAnnouncesTurnBeforeExecution(t *testing.T) {
	var out bytes.Buffer
	invocation := goaldrive.InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:weather-app"}, GoalVersion: "2", InvocationID: "weather-app-live-001", ProviderID: "claude-subscription", Mode: goaldrive.ModeSupervised}
	announceGoalDriveTurn(&out, normalizedOutput{}, invocation, "weather-app-live-001:turn:1")
	var announced map[string]any
	if err := json.Unmarshal(out.Bytes(), &announced); err != nil {
		t.Fatal(err)
	}
	if announced["turn_id"] != "weather-app-live-001:turn:1" || announced["provider"] != "claude-subscription" {
		t.Fatalf("announcement: %v", announced)
	}
	if announced["observe_with"] != "praxis supervise observe --goal-id=goal:weather-app --goal-version=2 --invocation-id=weather-app-live-001 --turn-id=weather-app-live-001:turn:1 --follow" {
		t.Fatalf("observe_with: %v", announced["observe_with"])
	}
	if announced["invocation_with"] != "praxis supervise observe --goal-id=goal:weather-app --goal-version=2 --invocation-id=weather-app-live-001 --follow" {
		t.Fatalf("invocation_with: %v", announced["invocation_with"])
	}
}
