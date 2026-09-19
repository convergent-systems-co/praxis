package goaldrive

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// The predicates in this file were written RED before any repair (#161):
// #162 (concurrent admission, turn identity, invocation reuse) and #163
// (interrupted or lost execution has no durable disposition). Each test
// states the expected invariant; the helper process below runs one real
// goal-drive turn in a separate OS process so atomicity and process loss
// are exercised across processes, not goroutines.

const helperEnv = "PRAXIS_GOALDRIVE_HELPER"

func raceBaseline() goals.GoalBaseline {
	return goals.GoalBaseline{ID: "goal:race", Version: "2", Digest: "sha256:" + strings.Repeat("e", 64), OriginalIntent: "race", RefinedOutcome: "one turn at a time", Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll,
		WorkPlan: &contracts.WorkPlan{BaselineDigest: "sha256:" + strings.Repeat("d", 64), AuthorityRef: "authority", AuthorityDigest: "sha256:" + strings.Repeat("e", 64), AcceptanceRef: "acceptance", AcceptanceDigest: "sha256:" + strings.Repeat("f", 64), AcceptedBy: contracts.PrincipalRef{ID: "human", Kind: "human"}, ProposalDigest: "sha256:" + strings.Repeat("1", 64),
			Candidates: []contracts.WorkCandidate{{ID: "unit:one", Priority: 1, Sequence: 1, SourceRef: "plan:one", SourceDigest: "sha256:" + strings.Repeat("a", 64), Provenance: contracts.ProvenancePLAN, Requirements: []contracts.RequirementRef{{ID: "req:1", SourceRef: "goal:race/1#success_criteria/1", SourceDigest: "sha256:" + strings.Repeat("1", 64)}}}}}}
}

// raceRuntime builds the runtime a helper process (or the test itself) uses.
// leaseTTL is read from the environment so the RED tests can express the
// expected expiry semantics before the field exists on the runtime.
func raceRuntime(t testing.TB, dbPath, workDir string, worker Worker) (Runtime, *ActivityLog, func()) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	store := state.NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "controller", Kind: "controller"}
	activity := &ActivityLog{Store: store, Actor: actor}
	ttl, _ := time.ParseDuration(os.Getenv("PRAXIS_GOAL_DRIVE_LEASE_TTL"))
	if ttl == 0 {
		ttl = 2 * time.Second
	}
	runtime := Runtime{Controller: Controller{Ledger: Ledger{Store: store, Actor: actor}, Worker: worker, NoProgressLimit: 3, Activity: activity}, Baselines: supervisionBaselineStore{baseline: raceBaseline()}, Repository: GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}, GraphID: "g", GraphVersion: "1", Activity: activity, Leases: state.New(db), LeaseTTL: ttl}
	return runtime, activity, func() { db.Close() }
}

// TestGoalDriveHelperProcess is the re-exec target: it runs one turn with a
// worker that sleeps, optionally leaves an uncommitted file first, then
// commits, and reports the allocated turn and the outcome on stdout.
func TestGoalDriveHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process only")
	}
	dbPath, workDir, invocation := os.Getenv("PRAXIS_HELPER_DB"), os.Getenv("PRAXIS_HELPER_REPO"), os.Getenv("PRAXIS_HELPER_INVOCATION")
	script := "sleep " + os.Getenv("PRAXIS_HELPER_SLEEP") + " && printf 'w\\n' > " + invocation + ".txt && git add -A && git commit -q -m " + invocation + " && head=$(git rev-parse HEAD) && printf '{\"outcome\":\"CONTINUE\",\"end_head\":\"%s\",\"checkpoint_valid\":true}' \"$head\""
	if os.Getenv("PRAXIS_HELPER_DIRTY_FIRST") == "1" {
		script = "printf 'partial\\n' > " + invocation + "-partial.txt && " + script
	}
	worker := CommandWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", script}}
	runtime, _, closeDB := raceRuntime(t, dbPath, workDir, worker)
	defer closeDB()
	runtime.OnTurnAllocated = func(turnID string) { fmt.Fprintf(os.Stdout, "ALLOCATED %s\n", turnID) }
	record, err := runtime.Execute(context.Background(), InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:race"}, GoalVersion: "2", Mode: ModeSupervised, InvocationID: invocation, ProviderID: "local"})
	if err != nil {
		fmt.Fprintf(os.Stdout, "ERROR %s\n", err.Error())
		return
	}
	fmt.Fprintf(os.Stdout, "DONE %s %s progress=%v published=%v\n", record.TurnID, record.Outcome, record.Progress, record.CheckpointPublished)
}

type helperRun struct {
	cmd    *exec.Cmd
	lines  chan string
	done   chan error
	turnID string
}

func prepareDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := state.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}

func hasLine(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func lineWith(lines []string, prefix string) string {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func startHelper(t *testing.T, dbPath, workDir, invocation, sleep string, dirtyFirst bool) *helperRun {
	t.Helper()
	prepareDB(t, dbPath)
	cmd := exec.Command(os.Args[0], "-test.run=^TestGoalDriveHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), helperEnv+"=1", "PRAXIS_HELPER_DB="+dbPath, "PRAXIS_HELPER_REPO="+workDir, "PRAXIS_HELPER_INVOCATION="+invocation, "PRAXIS_HELPER_SLEEP="+sleep, "PRAXIS_HELPER_DIRTY_FIRST="+map[bool]string{true: "1", false: "0"}[dirtyFirst], "PRAXIS_GOAL_DRIVE_LEASE_TTL=2s")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	run := &helperRun{cmd: cmd, lines: make(chan string, 64), done: make(chan error, 1)}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			run.lines <- scanner.Text()
		}
		close(run.lines)
	}()
	go func() { run.done <- cmd.Wait() }()
	return run
}

// waitFor returns the first line with the prefix, or "" after the deadline.
func (h *helperRun) waitFor(prefix string, limit time.Duration) string {
	deadline := time.After(limit)
	for {
		select {
		case line, ok := <-h.lines:
			if !ok {
				return ""
			}
			if strings.HasPrefix(line, prefix) {
				return line
			}
		case <-deadline:
			return ""
		}
	}
}

func (h *helperRun) finish(limit time.Duration) []string {
	var out []string
	deadline := time.After(limit)
	for {
		select {
		case line, ok := <-h.lines:
			if !ok {
				<-h.done
				return out
			}
			out = append(out, line)
		case <-deadline:
			_ = h.cmd.Process.Kill()
			return append(out, "TIMEOUT")
		}
	}
}

func turnNumber(turnID string) string { return turnID[strings.LastIndex(turnID, ":")+1:] }

func countMarkers(t *testing.T, workDir, suffix string) int {
	entries, err := filepath.Glob(filepath.Join(workDir, "*"+suffix))
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// A1 + A2 + A4: two OS processes race to start a turn on the same checkout.
// Expected: exactly one is admitted (the other fails closed before its
// worker runs, naming the conflict), and the admitted turn numbers are
// unique across the race.
func TestRED_A1_A2_A4_ConcurrentStartsOnOneCheckoutAdmitExactlyOne(t *testing.T) {
	root, workDir := contractRepo(t)
	dbPath := filepath.Join(root, "praxis.db")
	first := startHelper(t, dbPath, workDir, "race-a", "3", false)
	second := startHelper(t, dbPath, workDir, "race-b", "3", false)
	linesA := first.finish(40 * time.Second)
	linesB := second.finish(40 * time.Second)
	all := append(append([]string{}, linesA...), linesB...)
	var allocated []string
	admittedWorkers := countMarkers(t, workDir, ".txt")
	errorsNamingConflict := 0
	for _, line := range all {
		if strings.HasPrefix(line, "ALLOCATED ") {
			allocated = append(allocated, strings.TrimPrefix(line, "ALLOCATED "))
		}
		if strings.HasPrefix(line, "ERROR ") && (strings.Contains(line, "lease") || strings.Contains(line, "already active")) {
			errorsNamingConflict++
		}
	}
	if admittedWorkers != 1 || errorsNamingConflict != 1 {
		t.Fatalf("A1/A4 RED: exactly one process may be admitted and the other must fail closed before its worker runs, naming the active lease; worker executions=%d conflict errors=%d\n%s", admittedWorkers, errorsNamingConflict, strings.Join(all, "\n"))
	}
	if len(allocated) == 2 && turnNumber(allocated[0]) == turnNumber(allocated[1]) {
		t.Fatalf("A2 RED: both processes were allocated the same durable turn number: %v", allocated)
	}
}

// A3: an invocation identity is single-use.
func TestRED_A3_InvocationIdentityIsSingleUse(t *testing.T) {
	root, workDir := contractRepo(t)
	dbPath := filepath.Join(root, "praxis.db")
	first := startHelper(t, dbPath, workDir, "reuse-me", "0", false)
	if lines := first.finish(30 * time.Second); !hasLine(lines, "DONE") {
		t.Fatalf("first use must succeed: %v", lines)
	}
	again := startHelper(t, dbPath, workDir, "reuse-me", "0", false)
	lines := again.finish(30 * time.Second)
	last := lineWith(lines, "ERROR")
	if last == "" || !strings.Contains(last, "reuse-me") || !strings.Contains(last, "single-use") {
		t.Fatalf("A3 RED: reusing invocation id must fail closed as single-use; got: %v", lines)
	}
}

// A5 (guard, expected green before and after): two independent checkouts of
// the same Goal generation are not serialized by the checkout lease.
func TestGuard_A5_IndependentCheckoutsAreNotSerialized(t *testing.T) {
	root, workDir := contractRepo(t)
	otherDir := filepath.Join(root, "other")
	runGitTest(t, root, "clone", "-q", filepath.Join(root, "remote.git"), otherDir)
	runGitTest(t, otherDir, "config", "user.email", "contract@example.invalid")
	runGitTest(t, otherDir, "config", "user.name", "Contract")
	dbPath := filepath.Join(root, "praxis.db")
	first := startHelper(t, dbPath, workDir, "scope-a", "2", false)
	second := startHelper(t, dbPath, otherDir, "scope-b", "2", false)
	linesA, linesB := first.finish(40*time.Second), second.finish(40*time.Second)
	admitted := 0
	for _, lines := range [][]string{linesA, linesB} {
		for _, line := range lines {
			if strings.HasPrefix(line, "ALLOCATED ") {
				admitted++
			}
		}
	}
	if admitted != 2 {
		t.Fatalf("A5: independent checkouts must both be admitted: %v %v", linesA, linesB)
	}
}

// B1: graceful interruption (the signal handler cancels the context) after
// execution.started leaves a durable BLOCKED disposition on the ledger and a
// terminal state change on the stream, with the blocker naming interruption.
func TestRED_B1_GracefulInterruptionRecordsDisposition(t *testing.T) {
	root, workDir := contractRepo(t)
	worker := CommandWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "printf 'partial\\n' > partial.txt && sleep 20"}}
	runtime, activity, closeDB := raceRuntime(t, filepath.Join(root, "praxis.db"), workDir, worker)
	defer closeDB()
	ctx, cancel := context.WithCancel(context.Background())
	runtime.OnTurnAllocated = func(string) { go func() { time.Sleep(1500 * time.Millisecond); cancel() }() }
	record, err := runtime.Execute(ctx, InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:race"}, GoalVersion: "2", Mode: ModeSupervised, InvocationID: "int-1", ProviderID: "local"})
	turns, loadErr := runtime.Controller.Ledger.Load(context.Background(), "goal:race", "2")
	if loadErr != nil || len(turns) != 1 || turns[0].Outcome != OutcomeBlocked || !strings.Contains(turns[0].Blocker, "interrupt") || turns[0].ConsequenceFingerprint == "" {
		t.Fatalf("B1 RED: interruption must leave a durable BLOCKED record naming the interruption with the consequence fingerprinted; turns=%+v exec err=%v record=%+v", turns, err, record)
	}
	types := strings.Join(activityTypesFor(t, activity, "int-1", turns[0].TurnID), ",")
	if !strings.HasSuffix(types, "execution.state_changed") {
		t.Fatalf("B1 RED: the stream must end in a terminal state change: %s", types)
	}
}

// B2 + B3 + B4 + B5 + B6: abrupt process loss (SIGKILL) after execution
// started. Expected: the observer's follow terminates once the lease has
// expired; a fresh process finds the orphaned turn, refuses to start a
// new turn on that checkout until it is reconciled (naming the exact turn
// and the reconciliation command), does not retry anything; and after
// reconciliation the ledger holds a BLOCKED record for the orphan with
// UNKNOWN consequence and the dirty consequence fingerprinted.
func TestRED_B2_B6_ProcessLossBecomesExplicitReconcilableState(t *testing.T) {
	root, workDir := contractRepo(t)
	dbPath := filepath.Join(root, "praxis.db")
	lost := startHelper(t, dbPath, workDir, "lost-1", "30", true)
	allocated := lost.waitFor("ALLOCATED ", 20*time.Second)
	if allocated == "" {
		t.Fatal("helper did not allocate a turn")
	}
	turnID := strings.TrimPrefix(allocated, "ALLOCATED ")
	time.Sleep(1500 * time.Millisecond) // let it start the provider and write the partial file
	if err := syscall.Kill(-lost.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	<-lost.done
	// B3: an observer following the turn must transition to an explicit
	// non-running state once the lease is dead (the CLI observer consults
	// the same liveness primitive), rather than waiting forever.
	db, err := state.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	observerLedger := Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "observer", Kind: "cli"}}
	followDone := make(chan bool, 1)
	go func() {
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			lost, _, _ := LostTurns(context.Background(), observerLedger, state.New(db), "goal:race", "2", time.Now().UTC())
			for _, admission := range lost {
				if admission.TurnID == turnID {
					followDone <- true
					return
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		followDone <- false
	}()
	// B2/B6: a fresh process must refuse to start on the orphan's checkout
	// and must name the orphan and the reconciliation path; it must not retry.
	fresh, _, closeFresh := raceRuntime(t, dbPath, workDir, CommandWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "exit 0"}})
	defer closeFresh()
	time.Sleep(3 * time.Second) // beyond the 2s lease TTL the helper was given
	_, err = fresh.Execute(context.Background(), InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:race"}, GoalVersion: "2", Mode: ModeSupervised, InvocationID: "after-loss", ProviderID: "local"})
	if err == nil || !strings.Contains(err.Error(), turnID) || !strings.Contains(err.Error(), "reconcile") {
		t.Fatalf("B2/B6 RED: a fresh start on the orphaned checkout must fail closed naming the lost turn %s and the reconciliation path; got: %v", turnID, err)
	}
	if !<-followDone {
		t.Fatalf("B3 RED: the orphan's stream must reach an explicit non-running state after the lease expires")
	}
	// B4/B5: reconciliation is explicit (an operator or authorized path
	// concludes the loss); afterwards the orphan is a BLOCKED record with
	// UNKNOWN consequence and the partial file fingerprinted, a fresh
	// process reconstructs that state, and the checkout is recoverable.
	if _, err := fresh.Controller.ReconcileLostTurn(context.Background(), fresh.Leases, "goal:race", "2", turnID, "origin", contracts.PrincipalRef{ID: "operator", Kind: "human"}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, err := fresh.Controller.ReconcileLostTurn(context.Background(), fresh.Leases, "goal:race", "2", turnID, "origin", contracts.PrincipalRef{ID: "operator", Kind: "human"}); err == nil {
		t.Fatal("reconciliation must not repeat")
	}
	turns, loadErr := (Ledger{Store: state.NewSQLiteEventStore(db)}).Load(context.Background(), "goal:race", "2")
	if loadErr != nil || len(turns) != 1 || turns[0].TurnID != turnID || turns[0].Outcome != OutcomeBlocked || !strings.Contains(strings.ToLower(turns[0].Blocker), "unknown") || len(turns[0].ConsequenceFiles) == 0 {
		t.Fatalf("B4/B5 RED: the lost turn must be reconciled into a BLOCKED record with UNKNOWN consequence and the partial file fingerprinted: %+v %v", turns, loadErr)
	}
}

// B7: a stale process whose lease expired while it was stopped cannot
// publish after authority moved on.
func TestRED_B7_StaleProcessCannotPublishAfterLeaseLoss(t *testing.T) {
	root, workDir := contractRepo(t)
	dbPath := filepath.Join(root, "praxis.db")
	stale := startHelper(t, dbPath, workDir, "stale-1", "8", false)
	if stale.waitFor("ALLOCATED ", 20*time.Second) == "" {
		t.Fatal("helper did not allocate a turn")
	}
	time.Sleep(500 * time.Millisecond)
	if err := syscall.Kill(-stale.cmd.Process.Pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second) // lease TTL 2s elapses while it is stopped
	before := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "origin/main"))
	_ = syscall.Kill(-stale.cmd.Process.Pid, syscall.SIGCONT)
	lines := stale.finish(40 * time.Second)
	after := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "origin/main"))
	if after != before || hasLine(lines, "DONE") || !strings.Contains(lineWith(lines, "ERROR"), "lease") {
		t.Fatalf("B7 RED: a process whose lease expired must not publish; remote moved %s -> %s, helper said: %v", before, after, lines)
	}
	if hasLine(lines, "TIMEOUT") {
		t.Fatalf("B7: helper did not finish: %v", lines)
	}
}

var _ = json.Marshal
