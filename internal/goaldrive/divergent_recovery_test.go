package goaldrive

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type divergentRecoveryFixture struct {
	root         string
	work         string
	baseHead     string
	retainedHead string
	remoteHead   string
	fingerprint  string
	files        []string
	commits      []string
}

func newDivergentRecoveryFixture(t *testing.T) divergentRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	root, work := contractRepo(t)
	baseHead := strings.TrimSpace(runGitOutput(t, work, "rev-parse", "HEAD"))
	writeFile(t, filepath.Join(work, "retained.txt"), "exact retained consequence\n")
	runGitTest(t, work, "add", "retained.txt")
	runGitTest(t, work, "commit", "-m", "retained consequence")
	retainedHead := strings.TrimSpace(runGitOutput(t, work, "rev-parse", "HEAD"))
	repo := GitRepository{Dir: work, Remote: "origin", Branch: "main"}
	fingerprint, files, commits, err := repo.Fingerprint(ctx)
	if err != nil || len(files) != 0 || len(commits) != 1 || commits[0] != retainedHead {
		t.Fatalf("bind retained consequence: fingerprint=%s files=%v commits=%v err=%v", fingerprint, files, commits, err)
	}

	repair := filepath.Join(root, "repair")
	runGitTest(t, root, "clone", filepath.Join(root, "remote.git"), repair)
	runGitTest(t, repair, "config", "user.email", "governor@example.invalid")
	runGitTest(t, repair, "config", "user.name", "Qualified Governor")
	writeFile(t, filepath.Join(repair, "governor.txt"), "qualified repair\n")
	runGitTest(t, repair, "add", "governor.txt")
	runGitTest(t, repair, "commit", "-m", "qualified governor repair")
	runGitTest(t, repair, "push", "origin", "main")
	remoteHead := strings.TrimSpace(runGitOutput(t, repair, "rev-parse", "HEAD"))
	return divergentRecoveryFixture{root: root, work: work, baseHead: baseHead, retainedHead: retainedHead, remoteHead: remoteHead, fingerprint: fingerprint, files: files, commits: commits}
}

func (f divergentRecoveryFixture) repository() GitRepository {
	return GitRepository{Dir: f.work, Remote: "origin", Branch: "main"}
}

func (f divergentRecoveryFixture) boundRepository() GitRepository {
	repo := f.repository()
	repo.AllowRecoveryStart = true
	repo.RecoveryDigest = f.fingerprint
	return repo
}

func (f divergentRecoveryFixture) recovery() *WorkerRecoveryContext {
	return &WorkerRecoveryContext{
		RecoveredTurnID: "blocked:turn:1",
		Objective:       "unit:one",
		Blocker:         "qualified governor repair required before retry",
		Fingerprint:     f.fingerprint,
		Files:           append([]string(nil), f.files...),
		BaseHead:        f.baseHead,
		Commits:         append([]string(nil), f.commits...),
		Provenance:      RecoveryProvenanceRecorded,
	}
}

func TestPrepareRepositoryAdmitsExactFingerprintBoundDivergence(t *testing.T) {
	fixture := newDivergentRecoveryFixture(t)
	snapshot, err := PrepareRepository(context.Background(), fixture.boundRepository())
	if err != nil {
		t.Fatalf("exact bound divergent consequence must enter governed recovery: %v", err)
	}
	if snapshot.Relation != contracts.RelationDiverged || snapshot.Head != fixture.retainedHead {
		t.Fatalf("preflight must preserve the retained lineage: %+v", snapshot)
	}
}

func TestPrepareRepositoryRejectsUnboundDivergence(t *testing.T) {
	fixture := newDivergentRecoveryFixture(t)
	if _, err := PrepareRepository(context.Background(), fixture.repository()); !errors.Is(err, ErrUnsafeRepository) {
		t.Fatalf("ordinary divergence must remain fail-closed: %v", err)
	}
}

func TestPrepareRepositoryRejectsAlteredBoundDivergentConsequence(t *testing.T) {
	fixture := newDivergentRecoveryFixture(t)
	writeFile(t, filepath.Join(fixture.work, "retained.txt"), "altered after binding\n")
	if _, err := PrepareRepository(context.Background(), fixture.boundRepository()); err == nil || !strings.Contains(err.Error(), "fingerprint bound to the recovered turn") {
		t.Fatalf("altered divergent consequence must fail the exact fingerprint check: %v", err)
	}
}

func TestBoundDivergentRecoveryPublishesRemoteLineageAndRecordsRetry(t *testing.T) {
	ctx := context.Background()
	fixture := newDivergentRecoveryFixture(t)
	promptPath := filepath.Join(fixture.root, "worker-prompt.txt")
	worker := ProviderCLIWorker{ProviderID: "local", Dir: fixture.work, Command: []string{"/bin/sh", "-c", "cat > " + promptPath + " && git merge -q --no-edit origin/main"}}
	controller, _ := contractController(t, fixture.root, worker)
	req := contractRequest("unit:one")
	req.Recovery = fixture.recovery()

	record, err := controller.ExecuteTurnWithRepository(ctx, req, fixture.boundRepository())
	if err != nil || !record.Progress || !record.CheckpointPublished {
		t.Fatalf("governed reconciliation must publish: record=%+v err=%v", record, err)
	}
	if record.RetryOf != req.Recovery.RecoveredTurnID {
		t.Fatalf("published recovery must retain retry lineage: %+v", record)
	}
	for _, ancestor := range []string{fixture.retainedHead, fixture.remoteHead} {
		if err := runGitAncestor(fixture.work, ancestor, record.EndHead); err != nil {
			t.Fatalf("published checkpoint must preserve lineage %s: %v", ancestor, err)
		}
	}
	if remote := strings.TrimSpace(runGitOutput(t, fixture.work, "rev-parse", "origin/main")); remote != record.EndHead {
		t.Fatalf("controller did not publish the exact verified checkpoint: remote=%s record=%+v", remote, record)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{fixture.baseHead, fixture.remoteHead, fixture.retainedHead} {
		if !strings.Contains(string(prompt), identity) {
			t.Fatalf("worker recovery context omits lineage %s:\n%s", identity, prompt)
		}
	}
	turns, err := controller.Ledger.Load(ctx, req.GoalID, req.GoalVersion)
	if err != nil || len(turns) != 1 || turns[0].RetryOf != req.Recovery.RecoveredTurnID || turns[0].EndHead != record.EndHead {
		t.Fatalf("published retry lineage must be durable: turns=%+v err=%v", turns, err)
	}
}

func TestDivergentRecoveryPreflightFailureIsDurablyTerminalAndRetryable(t *testing.T) {
	ctx := context.Background()
	fixture := newDivergentRecoveryFixture(t)
	store := eventstore.NewMemoryStore()
	actor := contracts.PrincipalRef{ID: "controller", Kind: "controller"}
	activity := &ActivityLog{Store: store, Actor: actor}
	baseline := divergentRecoveryBaseline()
	worker := ProviderCLIWorker{ProviderID: "local", Dir: fixture.work, Command: []string{"/bin/sh", "-c", "git merge -q --no-edit origin/main"}}
	bad := fixture.boundRepository()
	bad.RecoveryDigest = "sha256:" + strings.Repeat("0", 64)
	runtime := Runtime{Controller: Controller{Ledger: Ledger{Store: store, Actor: actor}, Worker: worker, NoProgressLimit: 1, Activity: activity}, Baselines: supervisionBaselineStore{baseline: baseline}, Repository: bad, GraphID: "g", GraphVersion: "1", Activity: activity, Recovery: fixture.recovery()}
	invocation := InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: baseline.ID}, GoalVersion: baseline.Version, Mode: ModeSupervised, InvocationID: "recovery-preflight-1", ProviderID: "local"}
	record, err := runtime.Execute(ctx, invocation)
	if err == nil || record.Outcome != "" {
		t.Fatalf("mismatched preflight must stop before worker execution: record=%+v err=%v", record, err)
	}
	events, loadErr := activity.Load(ctx, invocation.InvocationID, "recovery-preflight-1:turn:1", 0)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(events) != 3 || events[0].Type != ActivityTurnAllocated || events[1].Type != ActivityBlockerDetected || events[2].Type != ActivityExecutionStateChanged {
		t.Fatalf("allocated preflight failure needs an exact durable terminal sequence: %+v", events)
	}
	if events[1].Data["stage"] != "repository-preflight" || !strings.Contains(events[1].Data["blocker"], "fingerprint bound to the recovered turn") || events[2].Data["state"] != "blocked" || events[2].Data["retry_of"] != fixture.recovery().RecoveredTurnID {
		t.Fatalf("durable preflight blocker is not actionable and exact: %+v", events)
	}

	// A fresh runtime and invocation can retry the same original turn after
	// the preflight condition is corrected; no altered or synthetic
	// consequence is adopted by the failed attempt.
	retry := Runtime{Controller: runtime.Controller, Baselines: runtime.Baselines, Repository: fixture.boundRepository(), GraphID: "g", GraphVersion: "1", Activity: activity, Recovery: fixture.recovery()}
	invocation.InvocationID = "recovery-preflight-2"
	record, err = retry.Execute(ctx, invocation)
	if err != nil || !record.CheckpointPublished || record.RetryOf != fixture.recovery().RecoveredTurnID {
		t.Fatalf("fresh exact retry must reconcile and publish: record=%+v err=%v", record, err)
	}
}

func divergentRecoveryBaseline() goals.GoalBaseline {
	baseline := *contractBaseline()
	baseline.WorkPlan = &contracts.WorkPlan{
		BaselineDigest:   baseline.Digest,
		AuthorityRef:     "authority:test",
		AuthorityDigest:  "sha256:" + strings.Repeat("3", 64),
		AcceptanceRef:    "acceptance:test",
		AcceptanceDigest: "sha256:" + strings.Repeat("4", 64),
		AcceptedBy:       contracts.PrincipalRef{ID: "human", Kind: "human"},
		ProposalDigest:   "sha256:" + strings.Repeat("5", 64),
		Candidates:       contractRequest("unit:one").WorkCandidates,
	}
	return baseline
}

func runGitAncestor(dir, ancestor, descendant string) error {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	return cmd.Run()
}
