package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestBindRecoveredTurnRequiresRecordedConsequenceOrDeclaresObservation
// proves the CLI binding admits only the consequence a BLOCKED turn
// recorded, refuses an altered checkout, and, for a turn that predates
// consequence recording (the first live weather turn), binds the checkout
// as found with observed-at-recovery provenance.
func TestBindRecoveredTurnRequiresRecordedConsequenceOrDeclaresObservation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	gitT(t, root, "init", "--bare", remote)
	gitT(t, root, "init", "--initial-branch=main", work)
	gitT(t, work, "config", "user.email", "t@example.invalid")
	gitT(t, work, "config", "user.name", "T")
	gitT(t, work, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(work, "README"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "README")
	gitT(t, work, "commit", "-m", "base")
	gitT(t, work, "push", "-u", "origin", "main")
	head := gitT(t, work, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(work, "left.txt"), []byte("provider left this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := goaldrive.GitRepository{Dir: work, Remote: "origin", Branch: "main"}
	fingerprint, files, _, err := repo.Fingerprint(ctx)
	if err != nil || strings.Join(files, ",") != "left.txt" {
		t.Fatal(err, files)
	}
	ledger := goaldrive.Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	base := goaldrive.TurnRecord{GoalID: "goal:x", GoalVersion: "2", InvocationID: "live-001", Mode: goaldrive.ModeSupervised, ChildObjective: "unit:one", GraphID: "g", GraphVersion: "1", StartHead: head, EndHead: head, Outcome: goaldrive.OutcomeBlocked, Blocker: "provider left repository with uncommitted changes; no checkpoint is valid"}
	legacy := base
	legacy.TurnID = "live-001:turn:1"
	if _, err := ledger.Append(ctx, 0, legacy); err != nil {
		t.Fatal(err)
	}
	recorded := base
	recorded.InvocationID, recorded.TurnID = "live-002", "live-002:turn:2"
	recorded.ConsequenceFingerprint, recorded.ConsequenceFiles = fingerprint, files
	if _, err := ledger.Append(ctx, 1, recorded); err != nil {
		t.Fatal(err)
	}
	invocation := goaldrive.InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:x"}, GoalVersion: "2"}

	invocation.RecoverTurn = "live-002:turn:2"
	bound, digest, err := bindRecoveredTurn(ctx, ledger, repo, invocation)
	if err != nil || digest != fingerprint || bound.Provenance != goaldrive.RecoveryProvenanceRecorded || bound.Objective != "unit:one" {
		t.Fatalf("recorded consequence must bind exactly: %+v %s %v", bound, digest, err)
	}
	invocation.RecoverTurn = "live-001:turn:1"
	bound, digest, err = bindRecoveredTurn(ctx, ledger, repo, invocation)
	if err != nil || digest != fingerprint || bound.Provenance != goaldrive.RecoveryProvenanceObserved {
		t.Fatalf("a turn without a recorded consequence binds the checkout as found, declared as observed: %+v %v", bound, err)
	}
	if err := os.WriteFile(filepath.Join(work, "left.txt"), []byte("altered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	invocation.RecoverTurn = "live-002:turn:2"
	if _, _, err := bindRecoveredTurn(ctx, ledger, repo, invocation); err == nil || !strings.Contains(err.Error(), "altered after the block") {
		t.Fatalf("altered consequence must be refused against the recorded fingerprint: %v", err)
	}
	invocation.RecoverTurn = "live-009:turn:9"
	if _, _, err := bindRecoveredTurn(ctx, ledger, repo, invocation); err == nil || !strings.Contains(err.Error(), "not a durable turn") {
		t.Fatalf("unknown turn must be refused: %v", err)
	}
}

func TestBindRecoveredTurnPreservesRecordedBaseAfterRemoteAdvance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	gitT(t, root, "init", "--bare", remote)
	gitT(t, root, "init", "--initial-branch=main", work)
	gitT(t, work, "config", "user.email", "t@example.invalid")
	gitT(t, work, "config", "user.name", "T")
	gitT(t, work, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(work, "README"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "README")
	gitT(t, work, "commit", "-m", "base")
	gitT(t, work, "push", "-u", "origin", "main")
	baseHead := gitT(t, work, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(work, "retained.txt"), []byte("retained\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "retained.txt")
	gitT(t, work, "commit", "-m", "retained consequence")
	retainedHead := gitT(t, work, "rev-parse", "HEAD")
	repo := goaldrive.GitRepository{Dir: work, Remote: "origin", Branch: "main"}
	fingerprint, files, commits, err := repo.Fingerprint(ctx)
	if err != nil || len(files) != 0 || len(commits) != 1 || commits[0] != retainedHead {
		t.Fatalf("fingerprint retained consequence: %s %v %v %v", fingerprint, files, commits, err)
	}
	ledger := goaldrive.Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	record := goaldrive.TurnRecord{GoalID: "goal:x", GoalVersion: "2", InvocationID: "blocked", Mode: goaldrive.ModeSupervised, TurnID: "blocked:turn:1", ChildObjective: "unit:one", GraphID: "g", GraphVersion: "1", StartHead: baseHead, EndHead: retainedHead, Outcome: goaldrive.OutcomeBlocked, Blocker: "qualified governor repair required", ConsequenceFingerprint: fingerprint, ConsequenceCommits: commits}
	if _, err := ledger.Append(ctx, 0, record); err != nil {
		t.Fatal(err)
	}

	repair := filepath.Join(root, "repair")
	gitT(t, root, "clone", remote, repair)
	gitT(t, repair, "config", "user.email", "governor@example.invalid")
	gitT(t, repair, "config", "user.name", "Governor")
	if err := os.WriteFile(filepath.Join(repair, "governor.txt"), []byte("qualified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repair, "add", "governor.txt")
	gitT(t, repair, "commit", "-m", "qualified governor repair")
	gitT(t, repair, "push", "origin", "main")
	remoteHead := gitT(t, repair, "rev-parse", "HEAD")
	if _, err := repo.Snapshot(ctx); err != nil { // fetch the advanced authority
		t.Fatal(err)
	}

	invocation := goaldrive.InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:x"}, GoalVersion: "2", RecoverTurn: record.TurnID}
	bound, _, err := bindRecoveredTurn(ctx, ledger, repo, invocation)
	if err != nil {
		t.Fatal(err)
	}
	if bound.BaseHead != baseHead {
		t.Fatalf("recovery base drifted to advanced remote: base=%s want=%s remote=%s", bound.BaseHead, baseHead, remoteHead)
	}
}

func TestBindRecoveredTurnUsesRecordedConsequenceBaseForChainedRecovery(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	gitT(t, root, "init", "--bare", remote)
	gitT(t, root, "init", "--initial-branch=main", work)
	gitT(t, work, "config", "user.email", "t@example.invalid")
	gitT(t, work, "config", "user.name", "T")
	gitT(t, work, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(work, "README"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "README")
	gitT(t, work, "commit", "-m", "base")
	gitT(t, work, "push", "-u", "origin", "main")
	if err := os.WriteFile(filepath.Join(work, "retained.txt"), []byte("retained\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "retained.txt")
	gitT(t, work, "commit", "-m", "retained consequence")
	retainedHead := gitT(t, work, "rev-parse", "HEAD")

	repair := filepath.Join(root, "repair")
	gitT(t, root, "clone", remote, repair)
	gitT(t, repair, "config", "user.email", "governor@example.invalid")
	gitT(t, repair, "config", "user.name", "Governor")
	if err := os.WriteFile(filepath.Join(repair, "governor.txt"), []byte("repair\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repair, "add", "governor.txt")
	gitT(t, repair, "commit", "-m", "governor repair")
	gitT(t, repair, "push", "origin", "main")
	remoteHead := gitT(t, repair, "rev-parse", "HEAD")
	gitT(t, work, "fetch", "origin", "main")
	gitT(t, work, "merge", "--no-edit", "origin/main")
	mergedHead := gitT(t, work, "rev-parse", "HEAD")

	repo := goaldrive.GitRepository{Dir: work, Remote: "origin", Branch: "main"}
	fingerprint, files, commits, err := repo.Fingerprint(ctx)
	if err != nil || len(files) != 0 || len(commits) != 2 || commits[len(commits)-1] != mergedHead {
		t.Fatalf("fingerprint chained consequence: %s %v %v %v", fingerprint, files, commits, err)
	}
	ledger := goaldrive.Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	record := goaldrive.TurnRecord{GoalID: "goal:x", GoalVersion: "2", InvocationID: "recovery-1", Mode: goaldrive.ModeSupervised, TurnID: "recovery-1:turn:2", ChildObjective: "unit:one", GraphID: "g", GraphVersion: "1", StartHead: retainedHead, EndHead: mergedHead, Outcome: goaldrive.OutcomeBlocked, Blocker: "validation failed after reconciliation", ConsequenceFingerprint: fingerprint, ConsequenceCommits: commits, ConsequenceBaseHead: remoteHead}
	if _, err := ledger.Append(ctx, 0, record); err != nil {
		t.Fatal(err)
	}
	invocation := goaldrive.InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: "goal:x"}, GoalVersion: "2", RecoverTurn: record.TurnID}
	bound, _, err := bindRecoveredTurn(ctx, ledger, repo, invocation)
	if err != nil {
		t.Fatal(err)
	}
	if bound.BaseHead != remoteHead {
		t.Fatalf("chained recovery base=%s want recorded fingerprint base=%s (prior retained start=%s)", bound.BaseHead, remoteHead, retainedHead)
	}
}
