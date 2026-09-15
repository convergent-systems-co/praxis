package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrProviderWorkspaceUnsafe   = errors.New("provider workspace is unsafe")
	ErrProviderWorkspaceTerminal = errors.New("provider workspace is not safely terminal for cleanup")
)

type ProviderWorkspaceRequest struct {
	WorkspaceID, RootDir, Repository, StartHead      string
	GoalID, GoalVersion, WorkPlanRef, WorkPlanDigest string
	ChildObjective, InvocationID, TurnID, ProviderID string
	MigrationSource, MigrationInputDigest            string
}

type ProviderWorkspaceSnapshot struct {
	Clean bool
	Head  string
}

// ProviderWorkspaceManager creates isolated Git worktrees from a controller-
// validated exact HEAD. It never grants push authority and never force-removes
// a dirty workspace.
type ProviderWorkspaceManager struct{ RootDir string }

func (m ProviderWorkspaceManager) Create(ctx context.Context, req ProviderWorkspaceRequest) (contracts.ProviderWorkspaceRecord, error) {
	if err := validateWorkspaceRequest(req); err != nil {
		return contracts.ProviderWorkspaceRecord{}, err
	}
	if err := validateRepositoryHead(ctx, req.Repository, req.StartHead); err != nil {
		return contracts.ProviderWorkspaceRecord{}, err
	}
	root := m.RootDir
	if root == "" {
		root = req.RootDir
	}
	if root == "" || (req.RootDir != "" && filepath.Clean(root) != filepath.Clean(req.RootDir)) {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("%w: workspace root is not manager-bound", ErrProviderWorkspaceUnsafe)
	}
	path := filepath.Join(root, req.WorkspaceID)
	if err := validateWorkspacePath(root, path); err != nil {
		return contracts.ProviderWorkspaceRecord{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("%w: workspace path already exists", ErrProviderWorkspaceUnsafe)
	} else if !errors.Is(err, os.ErrNotExist) {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("inspect provider workspace path: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("create provider workspace root: %w", err)
	}
	if _, err := runGit(ctx, req.Repository, "worktree", "add", "--detach", path, req.StartHead); err != nil {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("create provider worktree: %w", err)
	}
	record := contracts.ProviderWorkspaceRecord{WorkspaceID: req.WorkspaceID, Version: "1", Path: path, Repository: req.Repository, GoalID: req.GoalID, GoalVersion: req.GoalVersion, WorkPlanRef: req.WorkPlanRef, WorkPlanDigest: req.WorkPlanDigest, ChildObjective: req.ChildObjective, InvocationID: req.InvocationID, TurnID: req.TurnID, ProviderID: req.ProviderID, StartHead: req.StartHead, MigrationSource: req.MigrationSource, MigrationInputDigest: req.MigrationInputDigest, State: contracts.ProviderWorkspaceActive, CreatedAt: timeNow()}
	if err := record.Validate(); err != nil {
		_ = runGitIgnoreError(ctx, req.Repository, "worktree", "remove", path)
		return contracts.ProviderWorkspaceRecord{}, err
	}
	return record, nil
}

func (m ProviderWorkspaceManager) Recover(ctx context.Context, record contracts.ProviderWorkspaceRecord) (ProviderWorkspaceSnapshot, error) {
	if err := record.Validate(); err != nil {
		return ProviderWorkspaceSnapshot{}, err
	}
	if m.RootDir == "" {
		return ProviderWorkspaceSnapshot{}, fmt.Errorf("%w: workspace manager root is required", ErrProviderWorkspaceUnsafe)
	}
	if err := validateWorkspacePath(m.RootDir, record.Path); err != nil {
		return ProviderWorkspaceSnapshot{}, err
	}
	top, err := runGit(ctx, record.Path, "rev-parse", "--show-toplevel")
	recordedPath, pathErr := filepath.EvalSymlinks(record.Path)
	gitPath, gitPathErr := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil || pathErr != nil || gitPathErr != nil || gitPath != recordedPath {
		return ProviderWorkspaceSnapshot{}, fmt.Errorf("%w: workspace is not the recorded Git worktree", ErrProviderWorkspaceUnsafe)
	}
	common, err := gitCommonDir(ctx, record.Path)
	if err != nil {
		return ProviderWorkspaceSnapshot{}, fmt.Errorf("inspect provider workspace repository: %w", err)
	}
	repositoryCommon, err := gitCommonDir(ctx, record.Repository)
	if err != nil || common != repositoryCommon {
		return ProviderWorkspaceSnapshot{}, fmt.Errorf("%w: workspace belongs to a different repository", ErrProviderWorkspaceUnsafe)
	}
	head, err := runGit(ctx, record.Path, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return ProviderWorkspaceSnapshot{}, fmt.Errorf("read provider workspace HEAD: %w", err)
	}
	status, err := runGit(ctx, record.Path, "status", "--porcelain")
	if err != nil {
		return ProviderWorkspaceSnapshot{}, err
	}
	return ProviderWorkspaceSnapshot{Clean: strings.TrimSpace(status) == "", Head: strings.TrimSpace(head)}, nil
}

func (m ProviderWorkspaceManager) Cleanup(ctx context.Context, record contracts.ProviderWorkspaceRecord) (contracts.ProviderWorkspaceRecord, error) {
	if err := record.Validate(); err != nil {
		return contracts.ProviderWorkspaceRecord{}, err
	}
	if record.State != contracts.ProviderWorkspacePublished && record.State != contracts.ProviderWorkspaceAbandoned {
		return contracts.ProviderWorkspaceRecord{}, ErrProviderWorkspaceTerminal
	}
	snapshot, err := m.Recover(ctx, record)
	if err != nil || !snapshot.Clean {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("%w: workspace must be clean before cleanup", ErrProviderWorkspaceTerminal)
	}
	if _, err := runGit(ctx, record.Repository, "worktree", "remove", record.Path); err != nil {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("cleanup provider worktree: %w", err)
	}
	return record.Next(contracts.ProviderWorkspaceCleaned, record.EndHead, timeNow())
}

func validateWorkspaceRequest(req ProviderWorkspaceRequest) error {
	if req.WorkspaceID == "" || strings.ContainsAny(req.WorkspaceID, `/\\`) || req.Repository == "" || req.StartHead == "" || req.GoalID == "" || req.GoalVersion == "" || req.WorkPlanRef == "" || req.WorkPlanDigest == "" || req.ChildObjective == "" || req.InvocationID == "" || req.TurnID == "" || req.ProviderID == "" {
		return errors.New("provider workspace request is incomplete or unsafe")
	}
	if req.RootDir != "" && !filepath.IsAbs(req.RootDir) {
		return errors.New("provider workspace root must be absolute")
	}
	if !filepath.IsAbs(req.Repository) {
		return errors.New("provider repository must be absolute")
	}
	return nil
}

func validateWorkspacePath(root, path string) error {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("%w: workspace path escapes managed root", ErrProviderWorkspaceUnsafe)
	}
	return nil
}

func validateRepositoryHead(ctx context.Context, repository, want string) error {
	status, err := runGit(ctx, repository, "status", "--porcelain")
	if err != nil || strings.TrimSpace(status) != "" {
		return fmt.Errorf("%w: authoritative repository is not clean", ErrProviderWorkspaceUnsafe)
	}
	head, err := runGit(ctx, repository, "rev-parse", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(head) != want {
		return fmt.Errorf("%w: authoritative repository HEAD does not match start HEAD", ErrProviderWorkspaceUnsafe)
	}
	return nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func runGitIgnoreError(ctx context.Context, dir string, args ...string) error {
	_, err := runGit(ctx, dir, args...)
	return err
}

func gitCommonDir(ctx context.Context, dir string) (string, error) {
	value, err := runGit(ctx, dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if !filepath.IsAbs(value) {
		value = filepath.Join(dir, value)
	}
	value, err = filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(value)
}

var timeNow = func() time.Time { return time.Now().UTC() }
