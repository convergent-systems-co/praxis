package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
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
	Activity    *ActivityLog
	// Granted are the capabilities the launch contract actually allows. They
	// are declared by the constructor, never inferred, and the controller
	// refuses dispatch when they do not cover the checkpoint contract.
	Granted []WorkerCapability
}

func (w ProviderCLIWorker) RepositoryResultIsControllerOwned() bool { return true }

// Capabilities reports the launch contract's grants. A nil Granted (an
// adapter constructed without a profile) asserts the full repository
// contract; an explicit empty slice is a refusal.
func (w ProviderCLIWorker) Capabilities() []WorkerCapability {
	if w.Granted == nil {
		return []WorkerCapability{CapabilityEdit, CapabilityValidate, CapabilityStage, CapabilityCommit}
	}
	return append([]WorkerCapability(nil), w.Granted...)
}

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
	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(processCtx, w.Command[0], w.Command[1:]...)
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
	if request.Activity == nil {
		request.Activity = w.Activity
	}
	messageWriter := newProviderMessageWriter(ctx, request, w.ProviderID)
	cmd.Stdout = io.MultiWriter(stdout, messageWriter)
	cmd.Stderr = io.MultiWriter(stderr, messageWriter)
	control := watchInterventions(processCtx, cancel, request)
	if err := cmd.Run(); err != nil {
		messageWriter.Flush()
		if messageWriter.err != nil {
			return WorkerResult{}, fmt.Errorf("persist provider supervision message: %w", messageWriter.err)
		}
		signal := stopIntervention(control)
		if signal == ActivitySuspendRequested {
			return WorkerResult{}, ErrExecutionSuspended
		}
		if signal == ActivityCancelRequested {
			return WorkerResult{}, ErrExecutionCancelled
		}
		if ctx.Err() != nil {
			return WorkerResult{}, fmt.Errorf("provider %s interrupted: %w", w.ProviderID, ctx.Err())
		}
		return WorkerResult{}, fmt.Errorf("provider %s failed: %w: %s", w.ProviderID, err, redactProcessOutput(stderr.String(), os.Environ()))
	}
	messageWriter.Flush()
	if messageWriter.err != nil {
		return WorkerResult{}, fmt.Errorf("persist provider supervision message: %w", messageWriter.err)
	}
	stopIntervention(control)
	if stdout.truncated || stderr.truncated {
		return WorkerResult{}, errors.New("provider transcript exceeds configured output limit")
	}
	// The transcript is intentionally not returned, persisted, or interpreted.
	return WorkerResult{Outcome: OutcomeContinue, ExecutorID: w.ProviderID, CheckpointEvidence: []string{"provider-process:completed"}}, nil
}

type interventionControl struct {
	done   chan struct{}
	signal chan ActivityType
}

func watchInterventions(ctx context.Context, cancel context.CancelFunc, request WorkerRequest) interventionControl {
	control := interventionControl{done: make(chan struct{}), signal: make(chan ActivityType, 1)}
	if request.Activity == nil {
		close(control.done)
		return control
	}
	go func() {
		defer close(control.done)
		cursor := int64(0)
		if existing, err := request.Activity.Load(ctx, request.InvocationID, request.TurnID, 0); err == nil {
			cursor = int64(len(existing))
		}
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				events, err := request.Activity.Load(ctx, request.InvocationID, request.TurnID, cursor)
				if err != nil {
					continue
				}
				for _, event := range events {
					cursor = event.StreamVersion
					if event.Type == ActivitySuspendRequested || event.Type == ActivityCancelRequested {
						select {
						case control.signal <- event.Type:
						default:
						}
						cancel()
						return
					}
				}
			}
		}
	}()
	return control
}

func stopIntervention(control interventionControl) ActivityType {
	select {
	case signal := <-control.signal:
		return signal
	default:
		return ""
	}
}

type providerMessageWriter struct {
	ctx      context.Context
	request  WorkerRequest
	provider string
	buffer   strings.Builder
	err      error
}

func newProviderMessageWriter(ctx context.Context, request WorkerRequest, provider string) *providerMessageWriter {
	return &providerMessageWriter{ctx: ctx, request: request, provider: provider}
}

func (w *providerMessageWriter) Write(p []byte) (int, error) {
	_, _ = w.buffer.Write(p)
	for {
		value := w.buffer.String()
		idx := strings.IndexByte(value, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(value[:idx])
		w.buffer.Reset()
		_, _ = w.buffer.WriteString(value[idx+1:])
		w.emit(line)
	}
	return len(p), nil
}

func (w *providerMessageWriter) Flush() {
	if strings.TrimSpace(w.buffer.String()) != "" {
		w.emit(strings.TrimSpace(w.buffer.String()))
	}
	w.buffer.Reset()
}

func (w *providerMessageWriter) emit(message string) {
	if w.request.Activity == nil || message == "" {
		return
	}
	message = redactProcessOutput(sanitizeActivityText(message), os.Environ())
	if _, err := w.request.Activity.Emit(w.ctx, ActivityProviderMessage, w.request, contracts.PrincipalRef{ID: w.provider, Kind: "provider"}, contracts.TrustUntrustedContent, "provider:"+w.provider, map[string]string{"message": message, "stream": "user-facing"}); err != nil && w.err == nil {
		w.err = err
	}
}

func providerPrompt(request WorkerRequest) (string, error) {
	if request.GoalID == "" || request.GoalVersion == "" || request.TurnID == "" || request.ChildObjective == "" {
		return "", errors.New("provider request requires exact Goal, turn, and child objective identity")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are a bounded Praxis provider worker executing exactly one accepted work unit.\n\n")
	fmt.Fprintf(&b, "Objective (already selected, do not choose another): %s\nGoal: %s version %s\nTurn: %s\nGraph: %s version %s\nStarting repository HEAD: %s\n", request.ChildObjective, request.GoalID, request.GoalVersion, request.TurnID, request.GraphID, request.GraphVersion, request.StartHead)
	if c := request.Context; c != nil {
		fmt.Fprintf(&b, "\n## Goal (authoritative, Praxis-owned; do not look for it elsewhere)\nGoal digest: %s\nOriginal intent: %s\nRefined outcome: %s\n", c.Goal.Digest, c.Goal.OriginalIntent, c.Goal.RefinedOutcome)
		if c.Goal.Scope != "" {
			fmt.Fprintf(&b, "Scope: %s\n", c.Goal.Scope)
		}
		writeList(&b, "Success criteria", c.Goal.SuccessCriteria)
		writeList(&b, "Constraints", c.Goal.Constraints)
		writeList(&b, "Non-goals", c.Goal.NonGoals)
		writeList(&b, "Validity predicates", c.Goal.ValidityPredicates)
		fmt.Fprintf(&b, "\n## Work unit\nID: %s\nProvenance: %s (%s)\n", c.Unit.ID, c.Unit.Provenance, c.Unit.SourceRef)
		for _, requirement := range c.Unit.Requirements {
			fmt.Fprintf(&b, "Requirement %s: %s\n", requirement.ID, requirement.SourceRef)
		}
		writeList(&b, "Prerequisite units (already accepted as done or not your concern)", c.Unit.Prerequisites)
		writeList(&b, "Units that depend on this one", c.Unit.Dependents)
		fmt.Fprintf(&b, "\n## Repository authority\nPath: %s\nBranch: %s\nStart HEAD: %s\nWork only inside this path.\n", c.Repository.Path, c.Repository.Branch, c.Repository.StartHead)
		if c.Recovery != nil {
			fmt.Fprintf(&b, "\n## Recovered consequence\nTurn %s ended BLOCKED: %s\nThe working tree already contains uncommitted work from that turn (fingerprint %s):\n", c.Recovery.RecoveredTurnID, c.Recovery.Blocker, c.Recovery.Fingerprint)
			for _, file := range c.Recovery.Files {
				fmt.Fprintf(&b, "  - %s\n", file)
			}
			fmt.Fprintf(&b, "Inspect it against this unit. Validate and commit what is correct, fix what is not, and remove what should not exist. Nothing may remain uncommitted.\n")
		}
		writeList(&b, "\n## Checkpoint contract (Praxis verifies every item after you finish)", c.Checkpoint.Predicates)
		fmt.Fprintf(&b, "\n## Authority\nGranted capabilities: %s\n", joinCapabilities(c.Authority.Granted))
		writeList(&b, "Forbidden", c.Authority.Forbidden)
		writeList(&b, "\n## Invariants", c.Invariants)
	}
	fmt.Fprintf(&b, "\nWhen the work is ready: stage the intended files and create a local Git commit with a message naming the unit. Then finish. Do not push, fetch, rewrite refs, alter Praxis durable state, or claim that a checkpoint is valid. Praxis inspects the clean changed repository, runs the declared validation if any, decides progress and checkpoint validity, publishes only through controller policy, and decides the next invocation.\n")
	return b.String(), nil
}

func writeList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	for _, item := range items {
		fmt.Fprintf(b, "  - %s\n", item)
	}
}

// claudeSubscriptionTools is the bounded tool allowlist that makes the
// Claude subscription profile able to satisfy the checkpoint contract:
// repository edits, read-only inspection, staging, local commits, and
// running the repository's own validation toolchain. Push, arbitrary shell,
// and anything outside the repository stay denied.
var claudeSubscriptionTools = []string{
	"Read", "Edit", "Write", "MultiEdit", "Glob", "Grep", "LS",
	"Bash(git status:*)", "Bash(git diff:*)", "Bash(git log:*)", "Bash(git show:*)", "Bash(git add:*)", "Bash(git rm:*)", "Bash(git mv:*)", "Bash(git commit:*)", "Bash(git restore:*)",
	"Bash(./.praxis/validate:*)", "Bash(npm test:*)", "Bash(npm run:*)", "Bash(npm ci:*)", "Bash(npm install:*)", "Bash(node:*)", "Bash(npx:*)",
	"Bash(go test:*)", "Bash(go build:*)", "Bash(go vet:*)", "Bash(gofmt:*)", "Bash(pytest:*)", "Bash(python3:*)", "Bash(python:*)", "Bash(make test:*)", "Bash(make check:*)", "Bash(cargo test:*)", "Bash(cargo build:*)",
	"Bash(ls:*)", "Bash(cat:*)", "Bash(chmod +x:*)", "Bash(mkdir:*)",
}

func claudeSubscriptionCapabilities() []WorkerCapability {
	return []WorkerCapability{CapabilityEdit, CapabilityValidate, CapabilityStage, CapabilityCommit}
}

func codexSubscriptionCapabilities() []WorkerCapability {
	return []WorkerCapability{CapabilityEdit, CapabilityValidate, CapabilityStage, CapabilityCommit}
}

func NewCodexSubscriptionWorker(providerID, dir, model string, activity *ActivityLog) (Worker, error) {
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
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir, Activity: activity, Granted: codexSubscriptionCapabilities()}, nil
}

func NewClaudeSubscriptionWorker(providerID, dir, model string, activity *ActivityLog) (Worker, error) {
	if providerID == "" {
		return nil, errors.New("claude subscription provider identity is required")
	}
	executable, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude subscription CLI is unavailable: %w", err)
	}
	args := []string{"--print", "--output-format", "text", "--no-session-persistence", "--permission-mode", "acceptEdits", "--permission-prompts", "none", "--add-dir", dir, "--allowedTools"}
	args = append(args, claudeSubscriptionTools...)
	if model != "" {
		args = append(args, "--model", model)
	}
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir, Activity: activity, Granted: claudeSubscriptionCapabilities()}, nil
}
