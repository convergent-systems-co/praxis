package goaldrive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// GitRepository is the controller-owned repository adapter for a real
// checkout. It invokes git with explicit argv and never delegates authority
// to worker output or shell text.
type GitRepository struct {
	Dir           string
	Remote        string
	Branch        string
	AllowDetached bool
}

func (r GitRepository) validate() error {
	if r.Dir == "" || r.Remote == "" || r.Branch == "" {
		return errors.New("git repository directory, remote, and branch are required")
	}
	return nil
}

func (r GitRepository) Snapshot(ctx context.Context) (RepositorySnapshot, error) {
	if err := r.validate(); err != nil {
		return RepositorySnapshot{}, err
	}
	if _, err := r.run(ctx, "fetch", "--quiet", r.Remote, r.Branch); err != nil {
		return RepositorySnapshot{}, fmt.Errorf("fetch repository remote: %w", err)
	}
	branch, err := r.run(ctx, "branch", "--show-current")
	if err != nil {
		return RepositorySnapshot{}, fmt.Errorf("read repository branch: %w", err)
	}
	if strings.TrimSpace(branch) != r.Branch && !(r.AllowDetached && strings.TrimSpace(branch) == "") {
		return RepositorySnapshot{}, fmt.Errorf("repository branch is %q, want %q", strings.TrimSpace(branch), r.Branch)
	}
	local, err := r.run(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return RepositorySnapshot{}, fmt.Errorf("read repository HEAD: %w", err)
	}
	remoteRef := "refs/remotes/" + r.Remote + "/" + r.Branch
	remote, err := r.run(ctx, "rev-parse", "--verify", remoteRef+"^{commit}")
	if err != nil {
		return RepositorySnapshot{}, fmt.Errorf("read remote HEAD: %w", err)
	}
	local = strings.TrimSpace(local)
	remote = strings.TrimSpace(remote)
	clean, err := r.run(ctx, "status", "--porcelain")
	if err != nil {
		return RepositorySnapshot{}, fmt.Errorf("read repository status: %w", err)
	}
	if local == remote {
		return RepositorySnapshot{Clean: strings.TrimSpace(clean) == "", Relation: contracts.RelationEqual, Head: local}, nil
	}
	localAhead, err := r.isAncestor(ctx, remote, local)
	if err != nil {
		return RepositorySnapshot{}, err
	}
	if localAhead {
		return RepositorySnapshot{Clean: strings.TrimSpace(clean) == "", Relation: contracts.RelationLocalAhead, Head: local}, nil
	}
	remoteAhead, err := r.isAncestor(ctx, local, remote)
	if err != nil {
		return RepositorySnapshot{}, err
	}
	if remoteAhead {
		return RepositorySnapshot{Clean: strings.TrimSpace(clean) == "", Relation: contracts.RelationRemoteAhead, Head: local}, nil
	}
	return RepositorySnapshot{Clean: strings.TrimSpace(clean) == "", Relation: contracts.RelationDiverged, Head: local}, nil
}

func (r GitRepository) FastForward(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}
	if _, err := r.run(ctx, "merge", "--ff-only", r.Remote+"/"+r.Branch); err != nil {
		return fmt.Errorf("fast-forward repository: %w", err)
	}
	return nil
}

func (r GitRepository) PushAndVerify(ctx context.Context, head string) error {
	if err := r.validate(); err != nil {
		return err
	}
	if head == "" {
		return errors.New("checkpoint HEAD is required")
	}
	if _, err := r.run(ctx, "push", r.Remote, "HEAD:refs/heads/"+r.Branch); err != nil {
		return fmt.Errorf("push repository checkpoint: %w", err)
	}
	remote, err := r.run(ctx, "ls-remote", r.Remote, "refs/heads/"+r.Branch)
	if err != nil {
		return fmt.Errorf("verify remote repository checkpoint: %w", err)
	}
	fields := strings.Fields(remote)
	if len(fields) < 1 || fields[0] != head {
		return fmt.Errorf("remote checkpoint is %q, want %q", strings.TrimSpace(remote), head)
	}
	return nil
}

func (r GitRepository) isAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", r.Dir, "merge-base", "--is-ancestor", ancestor, descendant)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("compare repository ancestry: %w", err)
	}
	return true, nil
}

func (r GitRepository) run(ctx context.Context, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", r.Dir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
