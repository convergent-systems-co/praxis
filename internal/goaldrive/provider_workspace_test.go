package goaldrive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestProviderWorkspaceIsolatesDirtyRecoveryAndTerminalCleanup(t *testing.T) {
	ctx := context.Background()
	repository := t.TempDir()
	workspaceRoot := t.TempDir()
	runProviderGitTest(t, repository, "init", "-b", "main")
	runProviderGitTest(t, repository, "config", "user.email", "praxis-test@example.invalid")
	runProviderGitTest(t, repository, "config", "user.name", "Praxis Test")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runProviderGitTest(t, repository, "add", "README.md")
	runProviderGitTest(t, repository, "commit", "-m", "base")
	start, err := runGit(ctx, repository, "rev-parse", "HEAD^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	start = strings.TrimSpace(start)
	manager := ProviderWorkspaceManager{RootDir: workspaceRoot}
	record, err := manager.Create(ctx, ProviderWorkspaceRequest{WorkspaceID: "turn-one", Repository: repository, StartHead: start, GoalID: "goal", GoalVersion: "2", WorkPlanRef: "acceptance", WorkPlanDigest: "sha256:plan", ChildObjective: "child", InvocationID: "invocation", TurnID: "turn", ProviderID: "codex-subscription"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Path != filepath.Join(workspaceRoot, "turn-one") {
		t.Fatalf("workspace escaped manager root: %s", record.Path)
	}
	authoritative, err := runGit(ctx, repository, "status", "--porcelain")
	if err != nil || strings.TrimSpace(authoritative) != "" {
		t.Fatalf("authoritative checkout was changed: %q %v", authoritative, err)
	}

	if err := os.WriteFile(filepath.Join(record.Path, "work.txt"), []byte("provider work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Recover(ctx, record)
	if err != nil || snapshot.Clean || snapshot.Head != start {
		t.Fatalf("dirty provider workspace was not recoverable: %+v %v", snapshot, err)
	}
	if _, err := manager.Cleanup(ctx, record); err == nil {
		t.Fatal("active dirty workspace was cleaned")
	}

	runProviderGitTest(t, record.Path, "config", "user.email", "praxis-test@example.invalid")
	runProviderGitTest(t, record.Path, "config", "user.name", "Praxis Test")
	runProviderGitTest(t, record.Path, "add", "work.txt")
	runProviderGitTest(t, record.Path, "commit", "-m", "bounded child")
	end, err := runGit(ctx, record.Path, "rev-parse", "HEAD^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	end = strings.TrimSpace(end)
	snapshot, err = manager.Recover(ctx, record)
	if err != nil || !snapshot.Clean || snapshot.Head != end || end == start {
		t.Fatalf("committed provider workspace was not recoverable: %+v %v", snapshot, err)
	}
	record, err = record.Next(contracts.ProviderWorkspacePublished, end, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	cleaned, err := manager.Cleanup(ctx, record)
	if err != nil || cleaned.State != "cleaned" {
		t.Fatalf("validated workspace was not safely cleaned: %+v %v", cleaned, err)
	}
	if _, err := os.Stat(record.Path); !os.IsNotExist(err) {
		t.Fatalf("provider worktree remains after terminal cleanup: %v", err)
	}
}

func TestProviderWorkspaceRejectsDirtyAuthoritativeCheckoutAndEscapes(t *testing.T) {
	ctx := context.Background()
	repository := t.TempDir()
	root := t.TempDir()
	runProviderGitTest(t, repository, "init", "-b", "main")
	runProviderGitTest(t, repository, "config", "user.email", "praxis-test@example.invalid")
	runProviderGitTest(t, repository, "config", "user.name", "Praxis Test")
	if err := os.WriteFile(filepath.Join(repository, "dirty.txt"), []byte("human\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runProviderGitTest(t, repository, "add", "dirty.txt")
	runProviderGitTest(t, repository, "commit", "-m", "base")
	start := strings.TrimSpace(runProviderGitTest(t, repository, "rev-parse", "HEAD^{commit}"))
	manager := ProviderWorkspaceManager{RootDir: root}
	request := ProviderWorkspaceRequest{WorkspaceID: "safe", Repository: repository, StartHead: start, GoalID: "goal", GoalVersion: "2", WorkPlanRef: "acceptance", WorkPlanDigest: "sha256:plan", ChildObjective: "child", InvocationID: "invocation", TurnID: "turn", ProviderID: "codex-subscription", RootDir: root}
	if err := os.WriteFile(filepath.Join(repository, "uncommitted.txt"), []byte("human\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(ctx, request); err == nil {
		t.Fatal("dirty authoritative checkout was accepted")
	}
	request.WorkspaceID = "../escape"
	if _, err := manager.Create(ctx, request); err == nil {
		t.Fatal("workspace path escape was accepted")
	}
}

func runProviderGitTest(t *testing.T, dir string, args ...string) string {
	out, err := runGit(context.Background(), dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
