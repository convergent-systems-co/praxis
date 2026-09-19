package goaldrive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/scheduler"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type eventFailureStore struct {
	eventstore.Store
	failType string
	failErr  error
}

func (s *eventFailureStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error) {
	for _, event := range events {
		if event.Type == s.failType {
			return nil, s.failErr
		}
	}
	return s.Store.Append(ctx, aggregateID, expectedVersion, events)
}

type phaseRepository struct {
	head      string
	remote    string
	publishes []string
}

func (r *phaseRepository) Snapshot(context.Context) (RepositorySnapshot, error) {
	relation := contracts.RelationEqual
	if r.head != r.remote {
		relation = contracts.RelationLocalAhead
	}
	return RepositorySnapshot{Clean: true, Relation: relation, Head: r.head}, nil
}
func (r *phaseRepository) FastForward(context.Context) error { return nil }
func (r *phaseRepository) PushAndVerify(_ context.Context, head string) error {
	r.publishes = append(r.publishes, head)
	r.remote = head
	return nil
}

type phaseWorker struct {
	repo    *phaseRepository
	after   func()
	started bool
}

func (w *phaseWorker) Execute(context.Context, WorkerRequest) (WorkerResult, error) {
	w.started = true
	w.repo.head = "worker-head"
	if w.after != nil {
		w.after()
	}
	return WorkerResult{}, nil
}
func (*phaseWorker) RepositoryResultIsControllerOwned() bool { return true }
func (*phaseWorker) Capabilities() []WorkerCapability        { return RequiredRepositoryCapabilities() }

func phaseRuntime(store eventstore.Store, repo RepositoryAdapter, worker Worker) (Runtime, *ActivityLog) {
	actor := contracts.PrincipalRef{ID: "controller", Kind: "controller"}
	activity := &ActivityLog{Store: store, Actor: actor}
	return Runtime{Controller: Controller{Ledger: Ledger{Store: store, Actor: actor}, Worker: worker, NoProgressLimit: 3, Activity: activity}, Baselines: supervisionBaselineStore{baseline: divergentRecoveryBaseline()}, Repository: repo, GraphID: "g", GraphVersion: "1", Activity: activity}, activity
}

func phaseInvocation(id string) InvocationRequest {
	return InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:contract"}, GoalVersion: "2", Mode: ModeSupervised, InvocationID: id, ProviderID: "local"}
}

func assertNoSyntheticPreflightTerminal(t *testing.T, activity *ActivityLog, invocation, turn string) {
	t.Helper()
	events, err := activity.Load(context.Background(), invocation, turn, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == ActivityBlockerDetected || event.Type == ActivityExecutionStateChanged {
			t.Fatalf("post-worker failure was mislabeled as preflight: %+v", events)
		}
	}
}

func TestPostWorkerActivityFailureIsNotRecordedAsPreflight(t *testing.T) {
	base := eventstore.NewMemoryStore()
	store := &eventFailureStore{Store: base, failType: string(ActivityActionCompleted), failErr: errors.New("post-worker activity write failed")}
	repo := &phaseRepository{head: "base", remote: "base"}
	worker := &phaseWorker{repo: repo}
	runtime, activity := phaseRuntime(store, repo, worker)
	_, err := runtime.Execute(context.Background(), phaseInvocation("post-worker-activity"))
	if err == nil || !worker.started {
		t.Fatalf("test did not reach the post-worker failure: started=%v err=%v", worker.started, err)
	}
	assertNoSyntheticPreflightTerminal(t, activity, "post-worker-activity", "post-worker-activity:turn:1")
}

func TestPostPushLedgerFailureIsNotRecordedAsPreflight(t *testing.T) {
	base := eventstore.NewMemoryStore()
	store := &eventFailureStore{Store: base, failType: eventType, failErr: errors.New("turn ledger unavailable after push")}
	repo := &phaseRepository{head: "base", remote: "base"}
	worker := &phaseWorker{repo: repo}
	runtime, activity := phaseRuntime(store, repo, worker)
	_, err := runtime.Execute(context.Background(), phaseInvocation("post-push-ledger"))
	if err == nil || !worker.started || strings.Join(repo.publishes, ",") != "worker-head" {
		t.Fatalf("test did not reach post-push ledger failure: started=%v publishes=%v err=%v", worker.started, repo.publishes, err)
	}
	assertNoSyntheticPreflightTerminal(t, activity, "post-push-ledger", "post-push-ledger:turn:1")
}

type losingLeaseStore struct {
	mu    sync.Mutex
	live  bool
	lease scheduler.ResourceLease
}

func (s *losingLeaseStore) DefineSchedulerResource(context.Context, scheduler.ResourceState) error {
	return nil
}
func (s *losingLeaseStore) AcquireSchedulerResourceLeases(_ context.Context, sliceID, attemptID string, requirements []scheduler.ResourceRequirement, now time.Time, expiry *time.Time) ([]scheduler.ResourceLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live = true
	s.lease = scheduler.ResourceLease{ID: "lease", SliceID: sliceID, AttemptID: attemptID, ResourceKey: requirements[0].Key, Capacity: 1, AcquiredAt: now, ExpiresAt: expiry}
	return []scheduler.ResourceLease{s.lease}, nil
}
func (s *losingLeaseStore) ExtendSchedulerResourceLease(context.Context, string, time.Time, time.Time) error {
	return nil
}
func (s *losingLeaseStore) LookupSchedulerResourceLease(context.Context, string) (scheduler.ResourceLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lease, s.live, nil
}
func (s *losingLeaseStore) ReleaseSchedulerResourceLeases(context.Context, []string, time.Time) error {
	return nil
}
func (s *losingLeaseStore) lose() {
	s.mu.Lock()
	s.live = false
	s.mu.Unlock()
}

func TestLeaseLossNeverSynthesizesPreflightTerminal(t *testing.T) {
	base := eventstore.NewMemoryStore()
	store := &eventFailureStore{Store: base, failType: string(ActivityActionCompleted), failErr: errors.New("post-worker activity write failed")}
	repo := &phaseRepository{head: "base", remote: "base"}
	leases := &losingLeaseStore{}
	worker := &phaseWorker{repo: repo, after: leases.lose}
	runtime, activity := phaseRuntime(store, repo, worker)
	runtime.Leases = leases
	runtime.LeaseTTL = time.Hour
	_, err := runtime.Execute(context.Background(), phaseInvocation("lease-loss"))
	if !errors.Is(err, ErrLeaseLost) || !worker.started {
		t.Fatalf("test did not lose the lease after worker execution: started=%v err=%v", worker.started, err)
	}
	assertNoSyntheticPreflightTerminal(t, activity, "lease-loss", "lease-loss:turn:1")
}

type snapshotErrorRepository struct{ err error }

func (r snapshotErrorRepository) Snapshot(context.Context) (RepositorySnapshot, error) {
	return RepositorySnapshot{}, r.err
}
func (snapshotErrorRepository) FastForward(context.Context) error           { return nil }
func (snapshotErrorRepository) PushAndVerify(context.Context, string) error { return nil }

func TestPreflightBlockerActivityIsSanitized(t *testing.T) {
	store := eventstore.NewMemoryStore()
	runtime, activity := phaseRuntime(store, snapshotErrorRepository{err: errors.New("fetch failed token=super-secret")}, &phaseWorker{repo: &phaseRepository{}})
	_, err := runtime.Execute(context.Background(), phaseInvocation("sanitize-preflight"))
	if err == nil {
		t.Fatal("preflight must fail")
	}
	events, loadErr := activity.Load(context.Background(), "sanitize-preflight", "sanitize-preflight:turn:1", 0)
	if loadErr != nil || len(events) != 3 {
		t.Fatalf("load preflight terminal: %+v %v", events, loadErr)
	}
	if strings.Contains(events[1].Data["blocker"], "super-secret") || !strings.Contains(events[1].Data["blocker"], "[REDACTED]") {
		t.Fatalf("preflight blocker bypassed activity sanitization: %+v", events[1])
	}
}

func TestDivergentRecoveryClaimsExcludeGovernorAndMergeTrailers(t *testing.T) {
	ctx := context.Background()
	root, work := contractRepo(t)
	baseHead := strings.TrimSpace(runGitOutput(t, work, "rev-parse", "HEAD"))
	writeFile(t, filepath.Join(work, "retained.txt"), "retained\n")
	runGitTest(t, work, "add", "retained.txt")
	runGitTest(t, work, "commit", "-m", "retained", "-m", "Praxis-Unit-Complete: unit:one")
	retainedHead := strings.TrimSpace(runGitOutput(t, work, "rev-parse", "HEAD"))
	repo := GitRepository{Dir: work, Remote: "origin", Branch: "main"}
	fingerprint, files, commits, err := repo.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repair := filepath.Join(root, "repair")
	runGitTest(t, root, "clone", filepath.Join(root, "remote.git"), repair)
	runGitTest(t, repair, "config", "user.email", "governor@example.invalid")
	runGitTest(t, repair, "config", "user.name", "Governor")
	writeFile(t, filepath.Join(repair, "governor.txt"), "repair\n")
	runGitTest(t, repair, "add", "governor.txt")
	runGitTest(t, repair, "commit", "-m", "governor", "-m", "Praxis-Unit-Complete: governor:foreign")
	runGitTest(t, repair, "push", "origin", "main")
	remoteHead := strings.TrimSpace(runGitOutput(t, repair, "rev-parse", "HEAD"))
	bound := repo
	bound.AllowRecoveryStart, bound.RecoveryDigest = true, fingerprint
	worker := ProviderCLIWorker{ProviderID: "local", Dir: work, Command: []string{"/bin/sh", "-c", "git merge -q --no-edit -m 'reconcile' -m 'Praxis-Unit-Complete: merge:foreign' origin/main"}}
	controller, _ := contractController(t, root, worker)
	req := contractRequest("unit:one")
	req.GoalBaseline.WorkPlan = divergentRecoveryBaseline().WorkPlan
	req.Recovery = &WorkerRecoveryContext{RecoveredTurnID: "blocked:turn:1", Objective: "unit:one", Fingerprint: fingerprint, Files: files, BaseHead: baseHead, Commits: commits, Provenance: RecoveryProvenanceRecorded}
	record, err := controller.ExecuteTurnWithRepository(ctx, req, bound)
	if err != nil || record.CompletionClaim != "unit:one" || !record.CheckpointPublished {
		t.Fatalf("only retained/corrective worker commits may propose completion: record=%+v retained=%s remote=%s err=%v", record, retainedHead, remoteHead, err)
	}
}

type capturingRecoveryWorker struct {
	dir        string
	recoveries []*WorkerRecoveryContext
}

func (w *capturingRecoveryWorker) Execute(_ context.Context, req WorkerRequest) (WorkerResult, error) {
	if req.Context == nil || req.Context.Recovery == nil {
		w.recoveries = append(w.recoveries, nil)
	} else {
		copy := *req.Context.Recovery
		w.recoveries = append(w.recoveries, &copy)
	}
	if len(w.recoveries) == 1 {
		if err := runGitTestWorker(w.dir, "merge", "-q", "--no-edit", "origin/main"); err != nil {
			return WorkerResult{}, err
		}
	} else {
		if err := os.WriteFile(filepath.Join(w.dir, "second.txt"), []byte("second\n"), 0o644); err != nil {
			return WorkerResult{}, err
		}
		if err := runGitTestWorker(w.dir, "add", "second.txt"); err != nil {
			return WorkerResult{}, err
		}
		if err := runGitTestWorker(w.dir, "commit", "-q", "-m", "second turn"); err != nil {
			return WorkerResult{}, err
		}
	}
	return WorkerResult{}, nil
}
func (*capturingRecoveryWorker) RepositoryResultIsControllerOwned() bool { return true }
func (*capturingRecoveryWorker) Capabilities() []WorkerCapability {
	return RequiredRepositoryCapabilities()
}

func runGitTestWorker(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New(string(output) + err.Error())
	}
	return nil
}

func TestContinuousRecoveryUsesPerTurnContextWithoutStaleRemoteHead(t *testing.T) {
	fixture := newDivergentRecoveryFixture(t)
	worker := &capturingRecoveryWorker{dir: fixture.work}
	store := eventstore.NewMemoryStore()
	runtime, _ := phaseRuntime(store, fixture.boundRepository(), worker)
	original := fixture.recovery()
	runtime.Recovery = original
	invocation := phaseInvocation("continuous-recovery")
	invocation.Mode, invocation.MaxTurns = ModeContinuous, 2
	_, err := runtime.Execute(context.Background(), invocation)
	if !errors.Is(err, ErrContinuousTurnLimit) {
		t.Fatalf("two-turn fixture should stop at its explicit limit: %v", err)
	}
	if original.RemoteHead != "" {
		t.Fatalf("runtime mutated shared recovery context: %+v", original)
	}
	if len(worker.recoveries) != 2 || worker.recoveries[0] == nil || worker.recoveries[0].RemoteHead != fixture.remoteHead || worker.recoveries[1] != nil {
		t.Fatalf("recovery context leaked across continuous turns: %+v", worker.recoveries)
	}
}

func TestDivergentRecoveryRejectsConcurrentRemoteAdvanceAndRecordsFence(t *testing.T) {
	ctx := context.Background()
	fixture := newDivergentRecoveryFixture(t)
	worker := ProviderCLIWorker{ProviderID: "local", Dir: fixture.work, Command: []string{"/bin/sh", "-c", "git merge -q --no-edit origin/main && printf 'advance\\n' > " + filepath.Join(fixture.root, "repair", "advance.txt") + " && git -C " + filepath.Join(fixture.root, "repair") + " add advance.txt && git -C " + filepath.Join(fixture.root, "repair") + " commit -q -m concurrent && git -C " + filepath.Join(fixture.root, "repair") + " push -q origin main"}}
	controller, _ := contractController(t, fixture.root, worker)
	req := contractRequest("unit:one")
	req.Recovery = fixture.recovery()
	record, err := controller.ExecuteTurnWithRepository(ctx, req, fixture.boundRepository())
	if err == nil || record.Outcome != OutcomeBlocked || record.CheckpointPublished || !strings.Contains(err.Error(), "authoritative remote advanced") {
		t.Fatalf("concurrent remote advance must fence publication: record=%+v err=%v", record, err)
	}
	encoded, _ := json.Marshal(record)
	if !strings.Contains(string(encoded), `"recovery_remote_head":"`+fixture.remoteHead+`"`) || !strings.Contains(string(encoded), `"consequence_base_head":`) || !strings.Contains(string(encoded), `"consequence_remote_head":`) {
		t.Fatalf("remote fence and new consequence base must be durable: %s", encoded)
	}
	if record.ConsequenceBaseHead == "" || record.ConsequenceRemoteHead == "" || record.ConsequenceBaseHead == record.ConsequenceRemoteHead {
		t.Fatalf("fenced consequence must distinguish its common base from the advanced authority: %+v", record)
	}

	controller.Worker = ProviderCLIWorker{ProviderID: "local", Dir: fixture.work, Command: []string{"git", "merge", "-q", "--no-edit", "origin/main"}}
	retry := contractRequest("unit:one")
	retry.InvocationID, retry.TurnID = "inv-contract-retry", "inv-contract-retry:turn:1"
	retry.Recovery = &WorkerRecoveryContext{
		RecoveredTurnID: record.TurnID,
		Objective:       record.ChildObjective,
		Blocker:         record.Blocker,
		Fingerprint:     record.ConsequenceFingerprint,
		Files:           append([]string(nil), record.ConsequenceFiles...),
		BaseHead:        record.ConsequenceBaseHead,
		RemoteHead:      record.ConsequenceRemoteHead,
		Commits:         append([]string(nil), record.ConsequenceCommits...),
		Provenance:      RecoveryProvenanceRecorded,
	}
	retryRepo := fixture.repository()
	retryRepo.AllowRecoveryStart = true
	retryRepo.RecoveryDigest = record.ConsequenceFingerprint
	retried, retryErr := controller.ExecuteTurnWithRepository(ctx, retry, retryRepo)
	if retryErr != nil || !retried.CheckpointPublished || retried.RecoveryDisposition != "merged" {
		t.Fatalf("fenced consequence must remain exactly recoverable on retry: record=%+v err=%v", retried, retryErr)
	}
}

func TestDivergentRecoveryRejectsUnmergedRetainedLineage(t *testing.T) {
	fixture := newDivergentRecoveryFixture(t)
	worker := ProviderCLIWorker{ProviderID: "local", Dir: fixture.work, Command: []string{"/bin/sh", "-c", "git reset -q --hard origin/main"}}
	controller, _ := contractController(t, fixture.root, worker)
	req := contractRequest("unit:one")
	req.Recovery = fixture.recovery()
	record, err := controller.ExecuteTurnWithRepository(context.Background(), req, fixture.boundRepository())
	if err == nil || record.CheckpointPublished || !strings.Contains(err.Error(), "retained consequence") {
		t.Fatalf("silently dropping the retained lineage must not publish: record=%+v err=%v", record, err)
	}
}

func TestBlockedReconciliationRecordsBaseForChainedRecovery(t *testing.T) {
	fixture := newDivergentRecoveryFixture(t)
	worker := ProviderCLIWorker{ProviderID: "local", Dir: fixture.work, Command: []string{"/bin/sh", "-c", "git merge -q --no-edit origin/main && exit 17"}}
	controller, _ := contractController(t, fixture.root, worker)
	req := contractRequest("unit:one")
	req.Recovery = fixture.recovery()
	record, err := controller.ExecuteTurnWithRepository(context.Background(), req, fixture.boundRepository())
	if err == nil || record.Outcome != OutcomeBlocked || record.ConsequenceFingerprint == "" {
		t.Fatalf("failed reconciliation must retain an exact consequence: record=%+v err=%v", record, err)
	}
	encoded, _ := json.Marshal(record)
	if !strings.Contains(string(encoded), `"consequence_base_head":"`+fixture.remoteHead+`"`) {
		t.Fatalf("chained recovery lost the base used to fingerprint its commits: %s", encoded)
	}
}
