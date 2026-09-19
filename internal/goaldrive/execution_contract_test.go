package goaldrive

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func contractRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	remoteDir := filepath.Join(root, "remote.git")
	workDir := filepath.Join(root, "work")
	runGitTest(t, root, "init", "--bare", remoteDir)
	runGitTest(t, root, "init", "--initial-branch=main", workDir)
	runGitTest(t, workDir, "config", "user.email", "contract@example.invalid")
	runGitTest(t, workDir, "config", "user.name", "Contract")
	runGitTest(t, workDir, "remote", "add", "origin", remoteDir)
	writeFile(t, filepath.Join(workDir, "README"), "base\n")
	runGitTest(t, workDir, "add", "README")
	runGitTest(t, workDir, "commit", "-m", "base")
	runGitTest(t, workDir, "push", "-u", "origin", "main")
	return root, workDir
}

func contractController(t *testing.T, root string, worker Worker) (Controller, *ActivityLog) {
	t.Helper()
	db, err := state.OpenSQLite(context.Background(), filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := state.NewSQLiteEventStore(db)
	activity := &ActivityLog{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	return Controller{Ledger: Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, NoProgressLimit: 1, Activity: activity}, activity
}

func contractBaseline() *goals.GoalBaseline {
	return &goals.GoalBaseline{ID: "goal:contract", Version: "2", Digest: "sha256:" + strings.Repeat("a", 64), OriginalIntent: "Build a small real thing", RefinedOutcome: "The thing exists and is tested", Scope: "repository:work branch:main", SuccessCriteria: []string{"it works"}, Constraints: []string{"no secrets"}, NonGoals: []string{"hosting"}, Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll}
}

func contractRequest(objective string) TurnRequest {
	return TurnRequest{GoalID: "goal:contract", GoalVersion: "2", InvocationID: "inv-contract", TurnID: "inv-contract:turn:1", GraphID: "praxis.package.goals.default", GraphVersion: "0.3.0", Mode: ModeSupervised, ProviderID: "local", ChildObjective: objective, GoalBaseline: contractBaseline(),
		WorkCandidates: []contracts.WorkCandidate{{ID: "unit:one", Priority: 1, Sequence: 1, SourceRef: "issue:1", SourceDigest: "sha256:" + strings.Repeat("1", 64), Provenance: contracts.ProvenanceIssue, Requirements: []contracts.RequirementRef{{ID: "req:one", SourceRef: "goal:contract/2#success_criteria/1", SourceDigest: "sha256:" + strings.Repeat("2", 64)}}}}}
}

func activityTypesFor(t *testing.T, log *ActivityLog, invocation, turn string) []string {
	t.Helper()
	events, err := log.Load(context.Background(), invocation, turn, 0)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, event := range events {
		types = append(types, string(event.Type))
	}
	return types
}

// TestDispatchRefusesWorkerWithoutRequiredCapabilities proves the
// pre-dispatch invariant: a worker whose declared capabilities cannot
// satisfy the checkpoint contract is refused before any implementation
// begins, with durable evidence of the class "checkpoint-required action
// unavailable to worker".
func TestDispatchRefusesWorkerWithoutRequiredCapabilities(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	ran := filepath.Join(root, "ran")
	worker := CommandWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "touch " + ran}, Granted: []WorkerCapability{CapabilityEdit}}
	controller, activity := contractController(t, root, worker)
	_, err := controller.ExecuteTurnWithRepository(ctx, contractRequest(""), GitRepository{Dir: workDir, Remote: "origin", Branch: "main"})
	var capErr *CapabilityError
	if err == nil || !errors.As(err, &capErr) || strings.Join(capabilityStrings(capErr.Missing), ",") != "stage,commit" {
		t.Fatalf("dispatch must fail closed on missing capabilities: %v", err)
	}
	if _, statErr := os.Stat(ran); statErr == nil {
		t.Fatal("worker must not run when capabilities are unsatisfiable")
	}
	types := activityTypesFor(t, activity, "inv-contract", "inv-contract:turn:1")
	if len(types) != 1 || types[0] != string(ActivityCapabilityUnsatisfied) {
		t.Fatalf("exactly one durable capability blocker expected before execution: %v", types)
	}
	events, _ := activity.Load(ctx, "inv-contract", "inv-contract:turn:1", 0)
	if events[0].Data["evidence_class"] != "checkpoint-required action unavailable to worker" || events[0].Data["missing"] != "stage,commit" {
		t.Fatalf("blocker evidence: %v", events[0].Data)
	}
	turns, _ := controller.Ledger.Load(ctx, "goal:contract", "2")
	if len(turns) != 0 {
		t.Fatal("no turn record may claim the refused dispatch")
	}
}

// TestWorkerReceivesGovernedContext proves the provider prompt carries the
// authoritative Goal, the exact unit with requirement provenance, the
// repository authority, the checkpoint predicates, and the granted and
// forbidden authority, so the worker never has to look elsewhere.
func TestWorkerReceivesGovernedContext(t *testing.T) {
	req := contractRequest("unit:one")
	req.RepositoryPath, req.RepositoryBranch, req.StartHead = "/work", "main", "abc123"
	ctx, err := BuildWorkerContext(req.GoalBaseline, req.WorkCandidates, nil, "unit:one", WorkerRepositoryContext{Path: "/work", Branch: "main", StartHead: "abc123"}, ClaudeSubscriptionCapabilities(), false, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := providerPrompt(WorkerRequest{GoalID: "goal:contract", GoalVersion: "2", TurnID: "t", ChildObjective: "unit:one", GraphID: "g", GraphVersion: "1", StartHead: "abc123", Context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Build a small real thing", "The thing exists and is tested", "it works", "no secrets", "hosting", "Requirement req:one: goal:contract/2#success_criteria/1", "Path: /work", "Branch: main", "clean when you finish", "Granted capabilities: edit,validate,stage,commit", "push or fetch", "create a local Git commit", "Do not push", "./.praxis/validate"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if _, err := BuildWorkerContext(req.GoalBaseline, req.WorkCandidates, nil, "unit:other", WorkerRepositoryContext{}, nil, false, "", nil, nil); err == nil {
		t.Fatal("an objective outside the accepted plan must be refused")
	}
}

// TestDeclaredValidationGatesTheCheckpoint proves controller-owned
// validation: a repository's ./.praxis/validate runs after the provider's
// commit; failure blocks the checkpoint while retaining the local commit as
// evidence, success records the evidence and publishes.
func TestDeclaredValidationGatesTheCheckpoint(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	if err := os.MkdirAll(filepath.Join(workDir, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workDir, ".praxis", "validate"), "#!/bin/sh\ntest -f ok.txt\n")
	if err := os.Chmod(filepath.Join(workDir, ".praxis", "validate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, workDir, "add", ".praxis")
	runGitTest(t, workDir, "commit", "-m", "declare validation")
	runGitTest(t, workDir, "push", "origin", "main")
	failing := ProviderCLIWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "printf 'x\\n' > wrong.txt && git add wrong.txt && git commit -q -m wrong"}}
	controller, activity := contractController(t, root, failing)
	record, err := controller.ExecuteTurnWithRepository(ctx, contractRequest(""), GitRepository{Dir: workDir, Remote: "origin", Branch: "main"})
	if err == nil || record.Outcome != OutcomeBlocked || record.Progress || record.CheckpointPublished || !strings.Contains(record.Blocker, "declared validation failed") {
		t.Fatalf("failed validation must block without a checkpoint: %+v %v", record, err)
	}
	types := strings.Join(activityTypesFor(t, activity, "inv-contract", "inv-contract:turn:1"), ",")
	if !strings.Contains(types, "validation.started,validation.completed,validation.started,validation.completed,blocker.detected") {
		t.Fatalf("declared validation must be a durable stage: %s", types)
	}
	head := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "HEAD"))
	remote := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "origin/main"))
	if head == remote {
		t.Fatal("the failing local commit must be retained as evidence and not published")
	}
	// The retained evidence commit leaves the checkout ahead of the remote:
	// an ordinary turn is refused, and only a bound recovery of that exact
	// consequence may continue.
	plain := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	if _, err := PrepareRepository(ctx, plain); err == nil {
		t.Fatal("an unpublished evidence commit must be refused without a recovery binding")
	}
	fingerprint, files, commits, err := plain.Fingerprint(ctx)
	if err != nil || len(files) != 0 || len(commits) != 1 || commits[0] != head {
		t.Fatalf("fingerprint must name the retained commit: %v %v %v", files, commits, err)
	}
	bound := plain
	bound.AllowRecoveryStart, bound.RecoveryDigest = true, fingerprint
	passing := ProviderCLIWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "printf 'ok\\n' > ok.txt && git add ok.txt && git commit -q -m ok"}}
	controller.Worker = passing
	req := contractRequest("")
	req.InvocationID, req.TurnID = "inv-contract-2", "inv-contract-2:turn:2"
	if record.ConsequenceFingerprint != fingerprint || len(record.ConsequenceCommits) != 1 || record.ConsequenceCommits[0] != head {
		t.Fatalf("the blocked record must carry the consequence it left: %+v", record)
	}
	req.Recovery = &WorkerRecoveryContext{RecoveredTurnID: record.TurnID, Objective: record.ChildObjective, Blocker: record.Blocker, Fingerprint: fingerprint, Commits: commits, Provenance: RecoveryProvenanceRecorded}
	record, err = controller.ExecuteTurnWithRepository(ctx, req, bound)
	if err != nil || !record.Progress || !record.CheckpointPublished || !containsString(record.CheckpointEvidence, "repository:declared-validation-passed") {
		t.Fatalf("recovered evidence plus a fix must checkpoint and publish: %+v %v", record, err)
	}
	if remote := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "origin/main")); remote != record.EndHead || !strings.Contains(runGitOutput(t, workDir, "log", "--oneline", "origin/main"), "wrong") {
		t.Fatalf("publication must carry the evidence commit and the fix: %s", remote)
	}
	// A foreign commit on top of the consequence is not the bound consequence.
	writeFile(t, filepath.Join(workDir, "other.txt"), "x\n")
	runGitTest(t, workDir, "add", "other.txt")
	runGitTest(t, workDir, "commit", "-q", "-m", "foreign")
	if _, err := PrepareRepository(ctx, bound); err == nil {
		t.Fatal("a changed set of unpublished commits must be refused")
	}
}

// TestRecoveryPublishesAnUnchangedValidatedConsequence proves that recovery
// does not require a worker to manufacture a correction commit. When the
// exact bound consequence is already correct, a successful worker and the
// repository's declared validation qualify that retained commit for the
// controller-owned checkpoint and publication.
func TestRecoveryPublishesAnUnchangedValidatedConsequence(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	if err := os.MkdirAll(filepath.Join(workDir, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workDir, ".praxis", "validate"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(workDir, ".praxis", "validate"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workDir, "selection.txt"), "already correct\n")
	runGitTest(t, workDir, "add", ".praxis", "selection.txt")
	runGitTest(t, workDir, "commit", "-m", "select MVP", "-m", "Praxis-Unit-Complete: unit:one")

	repo := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	fingerprint, files, commits, err := repo.Fingerprint(ctx)
	if err != nil || len(files) != 0 || len(commits) != 1 {
		t.Fatalf("fingerprint retained commit: %s %v %v %v", fingerprint, files, commits, err)
	}
	bound := repo
	bound.AllowRecoveryStart, bound.RecoveryDigest = true, fingerprint
	worker := ProviderCLIWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "cat >/dev/null"}}
	controller, _ := contractController(t, root, worker)
	req := contractRequest("unit:one")
	req.Recovery = &WorkerRecoveryContext{RecoveredTurnID: "inv-blocked:turn:1", Objective: "unit:one", Blocker: "provider supervision failed after the commit", Fingerprint: fingerprint, Commits: commits, Provenance: RecoveryProvenanceRecorded}
	record, err := controller.ExecuteTurnWithRepository(ctx, req, bound)
	if err != nil || !record.Progress || !record.CheckpointPublished || record.CompletionClaim != "unit:one" || !containsString(record.CheckpointEvidence, "repository:declared-validation-passed") {
		t.Fatalf("unchanged valid recovery consequence must publish and carry its completion claim: %+v %v", record, err)
	}
	if remote := strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "origin/main")); remote != record.EndHead {
		t.Fatalf("qualified retained commit was not published: remote=%s record=%+v", remote, record)
	}
}

// TestRecoveryBindsExactConsequence proves that a dirty checkout is admitted
// only when its consequence fingerprint matches the bound recovery, that the
// worker is told what it recovers, and that a committing worker turns the
// recovered consequence into a checkpoint.
func TestRecoveryBindsExactConsequence(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	writeFile(t, filepath.Join(workDir, "left.txt"), "provider left this\n")
	repo := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	if _, err := PrepareRepository(ctx, repo); err == nil {
		t.Fatal("a dirty checkout must be refused without a binding")
	}
	fingerprint, files, _, err := repo.Fingerprint(ctx)
	if err != nil || len(files) != 1 || files[0] != "left.txt" {
		t.Fatalf("fingerprint: %v %v %v", fingerprint, files, err)
	}
	wrong := repo
	wrong.AllowRecoveryStart, wrong.RecoveryDigest = true, "sha256:"+strings.Repeat("0", 64)
	if _, err := PrepareRepository(ctx, wrong); err == nil {
		t.Fatal("a mismatched fingerprint must be refused")
	}
	writeFile(t, filepath.Join(workDir, "left.txt"), "provider left this, then someone changed it\n")
	bound := repo
	bound.AllowRecoveryStart, bound.RecoveryDigest = true, fingerprint
	if _, err := PrepareRepository(ctx, bound); err == nil {
		t.Fatal("altered consequence must be refused")
	}
	writeFile(t, filepath.Join(workDir, "left.txt"), "provider left this\n")
	if _, err := PrepareRepository(ctx, bound); err != nil {
		t.Fatalf("exact consequence must be admitted: %v", err)
	}
	recovery := &WorkerRecoveryContext{RecoveredTurnID: "inv-blocked:turn:1", Objective: "unit:one", Blocker: "provider left repository with uncommitted changes; no checkpoint is valid", Fingerprint: fingerprint, Files: files, Provenance: RecoveryProvenanceRecorded}
	seen := filepath.Join(root, "prompt.txt")
	worker := ProviderCLIWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "cat > " + seen + " && git add left.txt && git commit -q -m recovered"}}
	controller, activity := contractController(t, root, worker)
	req := contractRequest("unit:one")
	req.Recovery = recovery
	record, err := controller.ExecuteTurnWithRepository(ctx, req, bound)
	if err != nil || !record.Progress || !record.CheckpointPublished {
		t.Fatalf("recovered consequence must become a checkpoint: %+v %v", record, err)
	}
	prompt, _ := os.ReadFile(seen)
	if !strings.Contains(string(prompt), "Recovered consequence") || !strings.Contains(string(prompt), "left.txt") || !strings.Contains(string(prompt), fingerprint) {
		t.Fatalf("worker must be told what it recovers: %s", prompt)
	}
	types := strings.Join(activityTypesFor(t, activity, "inv-contract", "inv-contract:turn:1"), ",")
	if !strings.HasPrefix(types, "execution.started,workspace.recovery_bound,") {
		t.Fatalf("recovery binding must be durable before work: %s", types)
	}
}

// TestClaudeSubscriptionLaunchGrantsCheckpointCapabilities proves the
// launch contract declares and grants exactly the bounded tools the
// checkpoint contract needs and nothing that publishes.
func TestClaudeSubscriptionLaunchGrantsCheckpointCapabilities(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "claude"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(dir, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	worker, err := NewClaudeSubscriptionWorker("claude-subscription", dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckCapabilities(worker, "claude-subscription", RequiredRepositoryCapabilities()); err != nil {
		t.Fatalf("claude profile must satisfy the checkpoint contract: %v", err)
	}
	args := strings.Join(worker.(ProviderCLIWorker).Command, " ")
	for _, want := range []string{"--permission-mode acceptEdits", "--permission-prompts none", "--allowedTools", "Bash(git add:*)", "Bash(git commit:*)", "Bash(npm test:*)", "Bash(./.praxis/validate:*)"} {
		if !strings.Contains(args, want) {
			t.Fatalf("launch contract lacks %q: %s", want, args)
		}
	}
	for _, forbidden := range []string{"Bash(git push", "dangerously-skip-permissions", "bypassPermissions"} {
		if strings.Contains(args, forbidden) {
			t.Fatalf("launch contract must not grant %q", forbidden)
		}
	}
}

func capabilityStrings(items []WorkerCapability) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	return out
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return string(output)
}

// TestBlockedTurnRecordsTheConsequenceItLeft proves a dirty-leaving worker's
// BLOCKED record binds the exact uncommitted consequence at block time, so a
// later alteration of the checkout cannot pass as the recorded consequence.
func TestBlockedTurnRecordsTheConsequenceItLeft(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	worker := ProviderCLIWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "printf 'left\\n' > left.txt"}}
	controller, _ := contractController(t, root, worker)
	record, err := controller.ExecuteTurnWithRepository(ctx, contractRequest(""), GitRepository{Dir: workDir, Remote: "origin", Branch: "main"})
	if err == nil || record.Outcome != OutcomeBlocked || record.ConsequenceFingerprint == "" || strings.Join(record.ConsequenceFiles, ",") != "left.txt" {
		t.Fatalf("blocked record must carry the consequence: %+v %v", record, err)
	}
	repo := GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}
	fingerprint, _, _, err := repo.Fingerprint(ctx)
	if err != nil || fingerprint != record.ConsequenceFingerprint {
		t.Fatalf("recorded fingerprint must be the checkout's: %s vs %s %v", fingerprint, record.ConsequenceFingerprint, err)
	}
	writeFile(t, filepath.Join(workDir, "left.txt"), "altered\n")
	altered, _, _, _ := repo.Fingerprint(ctx)
	if altered == record.ConsequenceFingerprint {
		t.Fatal("an altered consequence must not match the recorded fingerprint")
	}
}
