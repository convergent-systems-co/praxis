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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// AllowRecoveryStart admits a dirty, local-ahead, or provably divergent
	// authoritative checkout only when its consequence fingerprint equals
	// RecoveryDigest, the fingerprint bound to the recovered turn.
	AllowRecoveryStart bool
	RecoveryDigest     string
}

func (r GitRepository) Location() (string, string) { return r.Dir, r.Branch }

// Fingerprint binds the exact consequence of the checkout: uncommitted
// changes and local commits not yet published to the remote branch.
func (r GitRepository) Fingerprint(ctx context.Context) (string, []string, []string, error) {
	return ConsequenceFingerprint(ctx, r.run, func(path string) ([]byte, error) { return os.ReadFile(filepath.Join(r.Dir, path)) }, "refs/remotes/"+r.Remote+"/"+r.Branch)
}

func (r GitRepository) ConsequenceLineage(ctx context.Context) (string, string, error) {
	remote, err := r.run(ctx, "rev-parse", "--verify", "refs/remotes/"+r.Remote+"/"+r.Branch+"^{commit}")
	if err != nil {
		return "", "", err
	}
	local, err := r.run(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", err
	}
	remote, local = strings.TrimSpace(remote), strings.TrimSpace(local)
	base, err := r.run(ctx, "merge-base", local, remote)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(base), remote, nil
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

func (r GitRepository) ReadCheckpointArtifact(ctx context.Context, checkpoint, sourceRef string) ([]byte, error) {
	if checkpoint == "" || sourceRef == "" || filepath.IsAbs(sourceRef) {
		return nil, errors.New("checkpoint artifact requires a commit and repository-relative source_ref")
	}
	clean := filepath.Clean(sourceRef)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, errors.New("checkpoint artifact source_ref escapes repository")
	}
	if err := r.HeadIs(ctx, checkpoint); err != nil {
		return nil, err
	}
	// Bound the read by the object's own size before buffering its bytes.
	sizeText, err := r.run(ctx, "cat-file", "-s", checkpoint+":"+filepath.ToSlash(clean))
	if err != nil {
		return nil, err
	}
	if size, parseErr := strconv.Atoi(strings.TrimSpace(sizeText)); parseErr != nil || size <= 0 || size > contracts.MaxGovernedArtifactBytes {
		return nil, errors.New("checkpoint artifact is missing or exceeds the byte bound")
	}
	body, err := r.run(ctx, "show", checkpoint+":"+filepath.ToSlash(clean))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > contracts.MaxGovernedArtifactBytes {
		return nil, errors.New("checkpoint artifact is missing or exceeds the byte bound")
	}
	return []byte(body), nil
}

func (r GitRepository) RecoveryCompletionClaims(ctx context.Context, remoteHead, endHead string) ([]string, error) {
	if remoteHead == "" || endHead == "" {
		return nil, errors.New("recovery completion claims require remote and checkpoint HEADs")
	}
	output, err := r.run(ctx, "log", "-z", "--no-merges", "--format=%B", remoteHead+".."+endHead)
	if err != nil {
		return nil, fmt.Errorf("read recovery commit messages: %w", err)
	}
	var messages []string
	for _, message := range strings.Split(output, "\x00") {
		if strings.TrimSpace(message) != "" {
			messages = append(messages, message)
		}
	}
	units, err := ParseCompletionClaims(messages)
	if err != nil {
		return nil, fmt.Errorf("read recovery completion proposals in %s..%s: %w", remoteHead, endHead, err)
	}
	return units, nil
}

// RecoveryStartAllowed reports whether a bound recovery may start from a
// checkout that is dirty, ahead of, or provably diverged from the remote.
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

func (r GitRepository) ValidationProfileDigest() (string, error) {
	body, err := os.ReadFile(filepath.Join(r.Dir, declaredValidationPath))
	if err != nil {
		return "", fmt.Errorf("read declared validation profile: %w", err)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
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
// minimal environment providers receive. It is the ADVISORY path used by
// non-safety plans and Goal-level evaluation: it executes the worker checkout
// as it stands and proves nothing about which bytes were validated. A
// safety-bearing turn never uses it; it uses RunBoundValidation.
func (r GitRepository) RunDeclaredValidationWith(ctx context.Context, args ...string) (string, error) {
	if _, declared := r.DeclaredValidation(); !declared {
		return "", errors.New("repository declares no validation")
	}
	return runValidationProcess(ctx, r.Dir, filepath.Join(r.Dir, declaredValidationPath), args)
}

const (
	// validationOutputLimit is one shared budget for the validator's combined
	// stdout and stderr. Output above it is a FAILED validation, never a
	// truncated success.
	validationOutputLimit = 1 << 20
)

var (
	validationTimeout   = 10 * time.Minute
	validationWaitDelay = 5 * time.Second
)

// runValidationProcess is the single collector every validation execution
// goes through. The validator's stdout and stderr share one bounded buffer; on
// overflow the process group is killed and the run fails closed whatever the
// exit status was; the process group is also terminated after the validator
// returns so no descendant outlives a "successful" validation.
func runValidationProcess(ctx context.Context, dir, entrypoint string, args []string) (string, error) {
	command := "./" + declaredValidationPath
	env, err := commandEnvironment(nil, os.Environ())
	if err != nil {
		return "", err
	}
	validationCtx, cancel := context.WithTimeout(ctx, validationTimeout)
	defer cancel()
	collector := &validationOutput{limit: validationOutputLimit, cancel: cancel}
	cmd := exec.CommandContext(validationCtx, entrypoint, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = validationWaitDelay
	configureValidationProcess(cmd)
	cmd.Stdout, cmd.Stderr = collector, collector
	runErr := cmd.Run()
	terminateValidationProcessTree(cmd)
	output, exceeded := collector.result()
	switch {
	case exceeded:
		return output, fmt.Errorf("%s %s: validation output exceeds the %d-byte bound; the result is refused, not truncated", command, strings.Join(args, " "), validationOutputLimit)
	case runErr != nil:
		return output, fmt.Errorf("%s %s: %w", command, strings.Join(args, " "), runErr)
	}
	return output, nil
}

// validationOutput is the bounded collector. It deliberately embeds nothing:
// os/exec copies through io.Copy, which would use a promoted ReadFrom (for
// example bytes.Buffer's) and bypass a size check placed in Write.
type validationOutput struct {
	mu       sync.Mutex
	buffer   []byte
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

func (o *validationOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.exceeded {
		return len(p), nil // keep draining so the child never blocks on a full pipe
	}
	if len(o.buffer)+len(p) > o.limit {
		o.exceeded = true
		o.cancel() // kills the process group; the run then fails closed
		return len(p), nil
	}
	o.buffer = append(o.buffer, p...)
	return len(p), nil
}

func (o *validationOutput) result() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(o.buffer), o.exceeded
}

var (
	objectIDPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	// qualifiedTrees records the tree identity each bound validation ran
	// against so a later binding check can prove it is unchanged.
	qualifiedTrees sync.Map
)

// bindCheckpoint proves, from Git's content-addressed objects only, that the
// checkpoint a validation result is attributed to is one exact commit and tree,
// that the checkout's HEAD (the ref publication reads) still is that commit,
// and that the validator COMMITTED at the checkpoint is executable and hashes
// to the bound profile digest. It never consults the working tree, the index,
// index hints (assume-unchanged, skip-worktree), excludes, or `git status`:
// none of those is evidence that validated bytes equal published bytes.
func (r GitRepository) bindCheckpoint(ctx context.Context, checkpoint, profileDigest string) (string, error) {
	if checkpoint == "" || profileDigest == "" {
		return "", errors.New("validation requires an exact checkpoint and validation-profile digest")
	}
	tree, err := r.resolveCheckpointTree(ctx, checkpoint)
	if err != nil {
		return "", err
	}
	if err := r.HeadIs(ctx, checkpoint); err != nil {
		return "", err
	}
	if branch, err := r.run(ctx, "branch", "--show-current"); err == nil && r.Branch != "" && strings.TrimSpace(branch) == r.Branch {
		local, err := r.run(ctx, "rev-parse", "--verify", "refs/heads/"+r.Branch+"^{commit}")
		if err != nil || strings.TrimSpace(local) != checkpoint {
			return "", fmt.Errorf("local branch %s does not point at the qualified checkpoint %s", r.Branch, checkpoint)
		}
	}
	if recorded, ok := qualifiedTrees.Load(r.Dir + "\x00" + checkpoint); ok && recorded.(string) != tree {
		return "", fmt.Errorf("checkpoint %s no longer has the tree %s that was qualified (now %s)", checkpoint, recorded, tree)
	}
	entry, err := r.run(ctx, "ls-tree", "-z", checkpoint, "--", declaredValidationPath)
	if err != nil {
		return "", fmt.Errorf("read validator committed at checkpoint: %w", err)
	}
	if !strings.HasPrefix(entry, "100755 blob ") {
		return "", errors.New("declared validator is missing, not a regular file, or not executable at the checkpoint")
	}
	committed, err := r.ReadCheckpointArtifact(ctx, checkpoint, declaredValidationPath)
	if err != nil {
		return "", fmt.Errorf("read validator committed at checkpoint: %w", err)
	}
	if digestBytes(committed) != profileDigest {
		return "", errors.New("validator committed at the checkpoint does not match the bound validation profile")
	}
	return tree, nil
}

// resolveCheckpointTree requires the checkpoint to be a full object id naming
// a commit that exists, and returns that commit's tree.
func (r GitRepository) resolveCheckpointTree(ctx context.Context, checkpoint string) (string, error) {
	if !objectIDPattern.MatchString(checkpoint) {
		return "", fmt.Errorf("checkpoint %q is not a full object id", checkpoint)
	}
	commit, err := r.run(ctx, "rev-parse", "--verify", checkpoint+"^{commit}")
	if err != nil || strings.TrimSpace(commit) != checkpoint {
		return "", fmt.Errorf("checkpoint %s is not a commit of this repository", checkpoint)
	}
	tree, err := r.run(ctx, "rev-parse", "--verify", checkpoint+"^{tree}")
	if err != nil {
		return "", fmt.Errorf("resolve tree of checkpoint %s: %w", checkpoint, err)
	}
	return strings.TrimSpace(tree), nil
}

// VerifyValidationBinding re-proves the checkpoint/profile binding without
// executing anything. It is content-based (see bindCheckpoint).
func (r GitRepository) VerifyValidationBinding(ctx context.Context, checkpoint, profileDigest string) error {
	_, err := r.bindCheckpoint(ctx, checkpoint, profileDigest)
	return err
}

// PublishGuard is the pre-publication predicate: immediately before the
// checkpoint is pushed, the exact qualified commit (not merely whatever HEAD
// happens to be) must still be the checkout's HEAD, carry the bound validator,
// and have the tree that was qualified. PushAndVerify pushes that exact object
// id, so a race after this check can only fail the push, never publish a
// different commit.
func (r GitRepository) PublishGuard(ctx context.Context, checkpoint, profileDigest string) error {
	return r.VerifyValidationBinding(ctx, checkpoint, profileDigest)
}

// RunBoundValidation executes one validation operation against an EXACT EXPORT
// of the checkpoint commit made outside the worker-controlled checkout, so the
// bytes validated are the bytes of that commit's tree by construction. The
// worker checkout is only a read-only object source: its working tree, index,
// index hints, excludes, config, hooks and fsmonitor are never consulted and
// never executed. After the run the export is re-proven to still be exactly the
// checkpoint tree by re-hashing its content with a fresh index (so nothing the
// validator did to the export's own index or hints can hide a change).
func (r GitRepository) RunBoundValidation(ctx context.Context, checkpoint, profileDigest string, args ...string) (string, error) {
	tree, err := r.bindCheckpoint(ctx, checkpoint, profileDigest)
	if err != nil {
		return "", fmt.Errorf("validation binding before execution: %w", err)
	}
	qualifiedTrees.Store(r.Dir+"\x00"+checkpoint, tree)
	export, err := r.exportCheckpoint(ctx, checkpoint, tree, profileDigest)
	if err != nil {
		return "", fmt.Errorf("validation export before execution: %w", err)
	}
	defer export.remove()
	output, runErr := runValidationProcess(ctx, export.dir, filepath.Join(export.dir, declaredValidationPath), args)
	if err := export.verify(ctx, checkpoint); err != nil {
		return output, fmt.Errorf("validation binding after execution: %w", err)
	}
	if _, err := r.bindCheckpoint(ctx, checkpoint, profileDigest); err != nil {
		return output, fmt.Errorf("validation binding after execution: %w", err)
	}
	return output, runErr
}

// checkpointExport is a private, exact checkout of one commit.
type checkpointExport struct {
	root string // holds every scratch path; removed as a whole
	dir  string // the export's working tree
	tree string
	env  []string
	repo string
}

// isolatedGitEnvironment gives every export-side git call a clean world: no
// system or global configuration, no prompts, no replace refs, and none of the
// repository-selecting variables a caller may have exported.
func isolatedGitEnvironment(home string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0", "GIT_ATTR_NOSYSTEM=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C",
	}
}

func (e *checkpointExport) git(ctx context.Context, extraEnv []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(append([]string(nil), e.env...), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (r GitRepository) exportCheckpoint(ctx context.Context, checkpoint, tree, profileDigest string) (*checkpointExport, error) {
	source, err := filepath.Abs(r.Dir)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "praxis-bound-validation-")
	if err != nil {
		return nil, err
	}
	export := &checkpointExport{root: root, dir: filepath.Join(root, "checkout"), tree: tree, repo: source}
	ok := false
	defer func() {
		if !ok {
			export.remove()
		}
	}()
	home, empty, template := filepath.Join(root, "home"), filepath.Join(root, "empty"), filepath.Join(root, "template")
	for _, dir := range []string{home, empty, template} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return nil, err
		}
	}
	export.env = isolatedGitEnvironment(home)
	hardening := []string{"-c", "core.hooksPath=" + empty, "-c", "core.fsmonitor=false", "-c", "core.autocrlf=false", "-c", "core.fileMode=true", "-c", "core.symlinks=true"}
	// A local clone copies the object database and refs but not the source's
	// hooks, config, info/exclude, or index; the empty template adds none.
	if _, err := export.git(ctx, nil, append(append([]string{}, hardening...), "clone", "--quiet", "--no-hardlinks", "--no-checkout", "--template="+template, "--", source, export.dir)...); err != nil {
		return nil, fmt.Errorf("export checkpoint: %w", err)
	}
	inExport := append([]string{"-C", export.dir}, hardening...)
	if _, err := export.git(ctx, nil, append(append([]string{}, inExport...), "checkout", "--quiet", "--detach", checkpoint, "--")...); err != nil {
		return nil, fmt.Errorf("check out checkpoint export: %w", err)
	}
	if err := export.verify(ctx, checkpoint); err != nil {
		return nil, err
	}
	info, err := os.Lstat(filepath.Join(export.dir, declaredValidationPath))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, errors.New("exported validator is missing or not an executable regular file")
	}
	body, err := os.ReadFile(filepath.Join(export.dir, declaredValidationPath))
	if err != nil || digestBytes(body) != profileDigest {
		return nil, errors.New("exported validator does not match the bound validation profile")
	}
	ok = true
	return export, nil
}

// verify proves the export is exactly the checkpoint: HEAD and tree are the
// qualified ones, no index entry carries a hint flag, and the tree computed by
// hashing the export's CONTENT into a fresh index in a fresh, unconfigured
// repository equals the checkpoint tree. That last check does not depend on the
// export's own index, hints, excludes, attributes or config, all of which the
// validator can modify.
func (e *checkpointExport) verify(ctx context.Context, checkpoint string) error {
	hardening := []string{"-C", e.dir, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + filepath.Join(e.root, "empty")}
	head, err := e.git(ctx, nil, append(append([]string{}, hardening...), "rev-parse", "--verify", "HEAD^{commit}")...)
	if err != nil || strings.TrimSpace(head) != checkpoint {
		return fmt.Errorf("validation export is not at the checkpoint %s", checkpoint)
	}
	tree, err := e.git(ctx, nil, append(append([]string{}, hardening...), "rev-parse", "--verify", "HEAD^{tree}")...)
	if err != nil || strings.TrimSpace(tree) != e.tree {
		return fmt.Errorf("validation export tree is not the qualified tree %s", e.tree)
	}
	listing, err := e.git(ctx, nil, append(append([]string{}, hardening...), "ls-files", "-v", "-z")...)
	if err != nil {
		return err
	}
	for _, entry := range strings.Split(listing, "\x00") {
		if entry != "" && !strings.HasPrefix(entry, "H ") {
			return fmt.Errorf("validation export index carries a hint or state flag on %q", entry)
		}
	}
	status, err := e.git(ctx, nil, append(append([]string{}, hardening...), "status", "--porcelain", "--untracked-files=all")...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return errors.New("validation export was modified during validation")
	}
	// `git status` honours .git/info/exclude, which the validator can rewrite;
	// listing untracked paths WITHOUT any exclude standard cannot be hidden.
	untracked, err := e.git(ctx, nil, append(append([]string{}, hardening...), "ls-files", "--others", "-z")...)
	if err != nil {
		return err
	}
	if untracked != "" {
		return errors.New("validation export gained untracked or excluded files during validation")
	}
	return e.verifyContent(ctx)
}

func (e *checkpointExport) verifyContent(ctx context.Context) error {
	scratchGit := filepath.Join(e.root, "verify.git")
	index := filepath.Join(e.root, "verify.index")
	_ = os.RemoveAll(scratchGit)
	_ = os.Remove(index)
	if _, err := e.git(ctx, nil, "init", "--quiet", "--bare", "--template="+filepath.Join(e.root, "template"), scratchGit); err != nil {
		return fmt.Errorf("prepare content verification: %w", err)
	}
	env := []string{"GIT_DIR=" + scratchGit, "GIT_WORK_TREE=" + e.dir, "GIT_INDEX_FILE=" + index, "GIT_ALTERNATE_OBJECT_DIRECTORIES=" + filepath.Join(e.dir, ".git", "objects")}
	hardening := []string{"-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + filepath.Join(e.root, "empty"), "-c", "core.autocrlf=false", "-c", "core.fileMode=true", "-c", "core.symlinks=true", "-c", "core.excludesFile=" + os.DevNull}
	for _, step := range [][]string{{"read-tree", e.tree}, {"add", "--all", "--force", "--", "."}} {
		if _, err := e.git(ctx, env, append(append([]string{}, hardening...), step...)...); err != nil {
			return fmt.Errorf("re-hash validation export content: %w", err)
		}
	}
	rehashed, err := e.git(ctx, env, append(append([]string{}, hardening...), "write-tree")...)
	if err != nil {
		return fmt.Errorf("re-hash validation export content: %w", err)
	}
	if strings.TrimSpace(rehashed) != e.tree {
		return fmt.Errorf("validation export content is %s, not the qualified tree %s", strings.TrimSpace(rehashed), e.tree)
	}
	return nil
}

// remove deletes the export, always, even if the validator made parts of it
// unwritable.
func (e *checkpointExport) remove() {
	if e == nil || e.root == "" {
		return
	}
	if err := os.RemoveAll(e.root); err == nil {
		return
	}
	_ = filepath.WalkDir(e.root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(e.root)
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

// VerifyDivergedRecovery proves the two lineages share a base and that the
// fetched authority advanced beyond it. The exact local consequence is
// independently fenced by VerifyRecoveryConsequence.
func (r GitRepository) VerifyDivergedRecovery(ctx context.Context) (string, string, error) {
	local, err := r.run(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", err
	}
	remoteRef := "refs/remotes/" + r.Remote + "/" + r.Branch
	remote, err := r.run(ctx, "rev-parse", "--verify", remoteRef+"^{commit}")
	if err != nil {
		return "", "", err
	}
	base, err := r.run(ctx, "merge-base", strings.TrimSpace(local), strings.TrimSpace(remote))
	if err != nil {
		return "", "", fmt.Errorf("find divergent recovery base: %w", err)
	}
	local, remote, base = strings.TrimSpace(local), strings.TrimSpace(remote), strings.TrimSpace(base)
	if base == "" || base == local || base == remote {
		return "", "", errors.New("authoritative remote did not advance from the retained consequence base")
	}
	return remote, base, nil
}

// VerifyCheckpointLineage fences publication to the exact authority observed
// at recovery preflight. A concurrent remote advance requires a fresh turn;
// the controller never asks the worker to guess or overwrite it.
func (r GitRepository) VerifyCheckpointLineage(ctx context.Context, remoteHead, retainedHead, checkpointHead string) (string, error) {
	if remoteHead == "" || retainedHead == "" || checkpointHead == "" {
		return "", errors.New("remote, retained, and checkpoint HEADs are required for lineage verification")
	}
	if _, err := r.run(ctx, "fetch", "--quiet", r.Remote, r.Branch); err != nil {
		return "", fmt.Errorf("fetch checkpoint authority: %w", err)
	}
	remoteRef := "refs/remotes/" + r.Remote + "/" + r.Branch
	current, err := r.run(ctx, "rev-parse", "--verify", remoteRef+"^{commit}")
	if err != nil {
		return "", err
	}
	if current = strings.TrimSpace(current); current != remoteHead {
		return "", fmt.Errorf("authoritative remote advanced from %s to %s during recovery", remoteHead, current)
	}
	spans, err := r.isAncestor(ctx, remoteHead, checkpointHead)
	if err != nil {
		return "", err
	}
	if !spans {
		return "", fmt.Errorf("recovered checkpoint %s does not contain authoritative remote %s", checkpointHead, remoteHead)
	}
	retained, err := r.isAncestor(ctx, retainedHead, checkpointHead)
	if err != nil {
		return "", err
	}
	if retained {
		return "merged", nil
	}
	if checkpointHead == remoteHead {
		return "", errors.New("recovered checkpoint silently drops the retained consequence without a worker-authored replacement")
	}
	replacements, err := r.run(ctx, "rev-list", "--no-merges", remoteHead+".."+checkpointHead)
	if err != nil {
		return "", fmt.Errorf("inspect recovery replacement lineage: %w", err)
	}
	if strings.TrimSpace(replacements) == "" {
		return "", errors.New("recovered checkpoint neither contains the retained consequence nor records a worker-authored replacement")
	}
	return "replaced", nil
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

// PushAndVerify publishes exactly the qualified commit. It pushes the object id
// (not "HEAD", not a branch name) to the branch ref, so anything that moves
// after qualification can only make the push fail, never publish a different
// commit, and then verifies the remote ref is exactly that commit. Worker
// hooks and fsmonitor configuration are not run by the controller's git.
func (r GitRepository) PushAndVerify(ctx context.Context, head string) error {
	if err := r.validate(); err != nil {
		return err
	}
	if head == "" {
		return errors.New("checkpoint HEAD is required")
	}
	if _, err := r.resolveCheckpointTree(ctx, head); err != nil {
		return fmt.Errorf("publication refused: %w", err)
	}
	if err := r.HeadIs(ctx, head); err != nil {
		return fmt.Errorf("publication refused: %w", err)
	}
	if _, err := r.run(ctx, "push", r.Remote, head+":refs/heads/"+r.Branch); err != nil {
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

// CheckpointPublished verifies that the exact consequence of a turn is
// still what the remote branch publishes: endHead is contained in the
// fetched remote branch and startHead (when known) is an ancestor of it.
func (r GitRepository) CheckpointPublished(ctx context.Context, startHead, endHead string) error {
	if err := r.validate(); err != nil {
		return err
	}
	if endHead == "" {
		return errors.New("checkpoint HEAD is required")
	}
	if _, err := r.run(ctx, "fetch", "--quiet", r.Remote, r.Branch); err != nil {
		return fmt.Errorf("fetch published branch: %w", err)
	}
	remoteRef := "refs/remotes/" + r.Remote + "/" + r.Branch
	contained, err := r.isAncestor(ctx, endHead, remoteRef)
	if err != nil {
		return err
	}
	if !contained {
		return fmt.Errorf("checkpoint %s is not contained in %s/%s", endHead, r.Remote, r.Branch)
	}
	if startHead != "" {
		spans, err := r.isAncestor(ctx, startHead, endHead)
		if err != nil {
			return err
		}
		if !spans {
			return fmt.Errorf("start head %s is not an ancestor of checkpoint %s", startHead, endHead)
		}
	}
	return nil
}

func (r GitRepository) isAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", append(controllerGitPrefix(r.Dir), "merge-base", "--is-ancestor", ancestor, descendant)...)
	cmd.Env = controllerGitEnvironment()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("compare repository ancestry: %w", err)
	}
	return true, nil
}

// controllerGitPrefix hardens every git the controller runs against the
// worker-controlled checkout: the worker's core.fsmonitor and hooks must never
// be executed with the controller's authority, and replace refs must never
// change which object an id names.
func controllerGitPrefix(dir string) []string {
	return []string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + os.DevNull}
}

func controllerGitEnvironment() []string {
	return append(os.Environ(), "GIT_NO_REPLACE_OBJECTS=1")
}

func (r GitRepository) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append(controllerGitPrefix(r.Dir), args...)...)
	cmd.Env = controllerGitEnvironment()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
