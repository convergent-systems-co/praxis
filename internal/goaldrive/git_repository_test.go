package goaldrive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestExecuteTurnWithGitRepositoryPersistsAndPublishesOneSelectedUnit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remoteDir := filepath.Join(root, "remote.git")
	workDir := filepath.Join(root, "work")
	runGitTest(t, root, "init", "--bare", remoteDir)
	runGitTest(t, root, "init", "--initial-branch=main", workDir)
	runGitTest(t, workDir, "config", "user.email", "dogfood@example.invalid")
	runGitTest(t, workDir, "config", "user.name", "Praxis Dogfood")
	runGitTest(t, workDir, "remote", "add", "origin", remoteDir)
	writeFile(t, filepath.Join(workDir, "README"), "base\n")
	runGitTest(t, workDir, "add", "README")
	runGitTest(t, workDir, "commit", "-m", "base")
	runGitTest(t, workDir, "push", "-u", "origin", "main")

	db, err := state.OpenSQLite(ctx, filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	ledger := Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	worker := CommandWorker{ProviderID: "local-test-worker", Dir: workDir, Command: []string{"/bin/sh", "-c", "printf 'bounded\\n' > unit.txt && git add unit.txt && git commit -m bounded >/dev/null && head=$(git rev-parse HEAD) && printf '{\"outcome\":\"COMPLETE\",\"end_head\":\"%s\",\"checkpoint_valid\":true}' \"$head\""}}
	controller := Controller{Ledger: ledger, Worker: worker, NoProgressLimit: 1}
	record, err := controller.ExecuteTurnWithRepository(ctx, TurnRequest{
		GoalID: "dogfood-praxis-issues-96-plus", GoalVersion: "2", InvocationID: "git-integration-1", TurnID: "turn-1",
		GraphID: "praxis.package.goals.default", GraphVersion: "0.2.0", Mode: ModeSupervised, Repository: contracts.RepositorySynced,
		WorkCandidates: []contracts.WorkCandidate{{ID: "bounded-unit", Priority: 1, Sequence: 1, SourceRef: "docs/PLAN/003-post-release-roadmap.md#103", SourceDigest: "sha256:roadmap", Provenance: contracts.ProvenancePLAN}},
	}, GitRepository{Dir: workDir, Remote: "origin", Branch: "main"})
	if err != nil || record.ChildObjective != "bounded-unit" || record.Outcome != OutcomeComplete || !record.Progress || !record.CheckpointPublished {
		db.Close()
		t.Fatalf("production-backed bounded turn mismatch: record=%+v err=%v", record, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.OpenSQLite(ctx, filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	resumed := Ledger{Store: state.NewSQLiteEventStore(reopened), Actor: ledger.Actor}
	turns, err := resumed.Load(ctx, record.GoalID, record.GoalVersion)
	if err != nil || len(turns) != 1 || turns[0].ChildObjective != "bounded-unit" || !turns[0].CheckpointPublished {
		t.Fatalf("checkpoint did not survive SQLite restart: turns=%+v err=%v", turns, err)
	}
	snapshot, err := (GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}).Snapshot(ctx)
	if err != nil || snapshot.Relation != contracts.RelationEqual || snapshot.Head != record.EndHead || !snapshot.Clean {
		t.Fatalf("published repository checkpoint mismatch: snapshot=%+v record=%+v err=%v", snapshot, record, err)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
