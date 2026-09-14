package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RepositoryDerivedWorker marks a provider adapter whose output is
// conversational/non-authoritative. The controller derives EndHead and
// progress from its repository adapter after the process exits.
type RepositoryDerivedWorker interface {
	Worker
	RepositoryResultIsControllerOwned() bool
}

// ProviderCLIWorker adapts a subscription-backed provider CLI to the
// provider-neutral Worker interface. Provider stdout is a transcript, not a
// WorkerResult protocol. Successful process exit reports only bounded process
// completion; the controller derives repository progress and outcome.
type ProviderCLIWorker struct {
	ProviderID  string
	Command     []string
	Dir         string
	Env         []string
	OutputLimit int
}

func (w ProviderCLIWorker) RepositoryResultIsControllerOwned() bool { return true }

func (w ProviderCLIWorker) Execute(ctx context.Context, request WorkerRequest) (WorkerResult, error) {
	if w.ProviderID == "" {
		return WorkerResult{}, errors.New("provider CLI identity is required")
	}
	if len(w.Command) == 0 || w.Command[0] == "" {
		return WorkerResult{}, errors.New("provider CLI command is required")
	}
	prompt, err := providerPrompt(request)
	if err != nil {
		return WorkerResult{}, err
	}
	cmd := exec.CommandContext(ctx, w.Command[0], w.Command[1:]...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = w.Dir
	env, err := commandEnvironment(w.Env, os.Environ())
	if err != nil {
		return WorkerResult{}, err
	}
	cmd.Env = env
	cmd.Stdin = strings.NewReader(prompt)
	limit := w.OutputLimit
	if limit <= 0 {
		limit = defaultWorkerOutputLimit
	}
	stdout := &limitedBuffer{limit: limit}
	stderr := &limitedBuffer{limit: limit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return WorkerResult{}, fmt.Errorf("provider %s interrupted: %w", w.ProviderID, ctx.Err())
		}
		return WorkerResult{}, fmt.Errorf("provider %s failed: %w: %s", w.ProviderID, err, redactProcessOutput(stderr.String(), os.Environ()))
	}
	if stdout.truncated || stderr.truncated {
		return WorkerResult{}, errors.New("provider transcript exceeds configured output limit")
	}
	// The transcript is intentionally not returned, persisted, or interpreted.
	return WorkerResult{Outcome: OutcomeContinue, ExecutorID: w.ProviderID, CheckpointEvidence: []string{"provider-process:completed"}}, nil
}

func providerPrompt(request WorkerRequest) (string, error) {
	if request.GoalID == "" || request.GoalVersion == "" || request.TurnID == "" || request.ChildObjective == "" {
		return "", errors.New("provider request requires exact Goal, turn, and child objective identity")
	}
	return fmt.Sprintf(`You are a bounded Praxis provider worker.

Work only on this already-selected child objective: %s
Goal: %s version %s
Turn: %s
Graph: %s version %s
Starting repository HEAD: %s

Do not select another objective. Do not decide Goal completion or authority.
Do not push, rewrite remote history, alter Praxis durable state, or claim that
a checkpoint is valid. Make only the bounded repository changes needed for the
selected objective. Praxis will inspect the repository and decide progress,
checkpoint validity, publication, and the next invocation.
`, request.ChildObjective, request.GoalID, request.GoalVersion, request.TurnID, request.GraphID, request.GraphVersion, request.StartHead), nil
}

func NewCodexSubscriptionWorker(providerID, dir, model string) (Worker, error) {
	if providerID == "" {
		return nil, errors.New("codex subscription provider identity is required")
	}
	executable, err := exec.LookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex subscription CLI is unavailable: %w", err)
	}
	args := []string{"exec", "--ephemeral", "--sandbox", "workspace-write", "--skip-git-repo-check", "-C", dir}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "-")
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir}, nil
}

func NewClaudeSubscriptionWorker(providerID, dir, model string) (Worker, error) {
	if providerID == "" {
		return nil, errors.New("claude subscription provider identity is required")
	}
	executable, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude subscription CLI is unavailable: %w", err)
	}
	args := []string{"--print", "--output-format", "text", "--no-session-persistence", "--permission-mode", "acceptEdits", "--permission-prompts", "none", "--add-dir", dir}
	if model != "" {
		args = append(args, "--model", model)
	}
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir}, nil
}
