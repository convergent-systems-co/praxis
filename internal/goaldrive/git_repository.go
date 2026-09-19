package goaldrive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// GitRepository is the controller-owned repository adapter for a real
// checkout. It invokes git with explicit argv and never delegates authority
// to worker output or shell text.
type GitRepository struct {
	Dir              string
	Remote           string
	Branch           string
	AllowDetached    bool
	AllowDirtyStart  bool
	DirtyStartDigest string
	// AllowRecoveryStart admits a dirty authoritative checkout only when its
	// consequence fingerprint equals RecoveryDigest, the fingerprint bound to
	// the BLOCKED turn being recovered.
	AllowRecoveryStart bool
	RecoveryDigest     string
}

func (r GitRepository) Location() (string, string) { return r.Dir, r.Branch }

// Fingerprint binds the exact consequence of the checkout: uncommitted
// changes and local commits not yet published to the remote branch.
func (r GitRepository) Fingerprint(ctx context.Context) (string, []string, []string, error) {
	return ConsequenceFingerprint(ctx, r.run, func(path string) ([]byte, error) { return os.ReadFile(filepath.Join(r.Dir, path)) }, "refs/remotes/"+r.Remote+"/"+r.Branch)
}

// CompletionClaims returns the units proposed complete by the commits the
// worker added between the two checkpoints. It reads every full commit
// message of the span and applies the worker-facing contract
// (ParseCompletionProposals), never Git's trailer-block heuristic (#164).
func (r GitRepository) CompletionClaims(ctx context.Context, startHead, endHead string) ([]string, error) {
	if endHead == "" {
		return nil, errors.New("completion claims require the checkpoint HEAD")
	}
	span := endHead
	if startHead != "" {
		span = startHead + ".." + endHead
	}
	output, err := r.run(ctx, "log", "-z", "--format=%B", span)
	if err != nil {
		return nil, fmt.Errorf("read commit messages: %w", err)
	}
	var messages []string
	for _, message := range strings.Split(output, "\x00") {
		if strings.TrimSpace(message) != "" {
			messages = append(messages, message)
		}
	}
	units, err := ParseCompletionClaims(messages)
	if err != nil {
		return nil, fmt.Errorf("read completion proposals in %s: %w", span, err)
	}
	return units, nil
}

// RecoveryStartAllowed reports whether a bound recovery may start from a
// checkout that is dirty or ahead of the remote.
func (r GitRepository) RecoveryStartAllowed() bool { return r.AllowRecoveryStart }

const declaredValidationPath = ".praxis/validate"

// DeclaredValidation reports the repository's own validation entry point.
func (r GitRepository) DeclaredValidation() (string, bool) {
	info, err := os.Stat(filepath.Join(r.Dir, declaredValidationPath))
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", false
	}
	return "./" + declaredValidationPath, true
}

// RunDeclaredValidation executes the declared validation in the checkout
// with the same minimal environment providers receive.
func (r GitRepository) RunDeclaredValidation(ctx context.Context) (string, error) {
	return r.RunDeclaredValidationWith(ctx)
}

// HeadIs fails unless the checkout is exactly at the given commit.
func (r GitRepository) HeadIs(ctx context.Context, head string) error {
	current, err := r.run(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(current) != head {
		return fmt.Errorf("checkout is at %s, not %s", strings.TrimSpace(current), head)
	}
	return nil
}

// RunDeclaredValidationWith executes the declared validation with arguments
// (a bound contract element or "integrated") in the checkout with the same
// minimal environment providers receive.
func (r GitRepository) RunDeclaredValidationWith(ctx context.Context, args ...string) (string, error) {
	command, declared := r.DeclaredValidation()
	if !declared {
		return "", errors.New("repository declares no validation")
	}
	env, err := commandEnvironment(nil, os.Environ())
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(r.Dir, declaredValidationPath), args...)
	cmd.Dir = r.Dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s %s: %w", command, strings.Join(args, " "), err)
	}
	return string(output), nil
}

// VerifyRecoveryConsequence admits the checkout only when its consequence
// fingerprint is exactly the one bound to the recovered turn. It is applied
// at turn start; the post-worker inspection sees the worker's result.
func (r GitRepository) VerifyRecoveryConsequence(ctx context.Context) error {
	fingerprint, _, _, err := r.Fingerprint(ctx)
	if err != nil || r.RecoveryDigest == "" || fingerprint != r.RecoveryDigest {
		return errors.New("checkout does not match the consequence fingerprint bound to the recovered turn")
	}
	return nil
}

func (r GitRepository) DirtyStartAllowed() bool { return r.AllowDirtyStart }

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
		isClean := strings.TrimSpace(clean) == ""
		if !isClean && r.AllowDirtyStart {
			diff, diffErr := r.run(ctx, "diff", "--binary")
			if diffErr != nil || r.DirtyStartDigest == "" || digestBytes([]byte(diff)) != r.DirtyStartDigest {
				return RepositorySnapshot{}, fmt.Errorf("dirty provider workspace migration input does not match its persisted digest")
			}
		}
		return RepositorySnapshot{Clean: isClean, Relation: contracts.RelationEqual, Head: local}, nil
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

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
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
