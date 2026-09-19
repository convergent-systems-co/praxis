package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
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
	// Envelope is the non-interactive execution envelope the launch
	// contract establishes (#170); nil when undeclared.
	Envelope *WorkerExecutionEnvelope
}

func (w ProviderCLIWorker) RepositoryResultIsControllerOwned() bool { return true }

// ExecutionEnvelope reports the launch contract's execution envelope.
func (w ProviderCLIWorker) ExecutionEnvelope() *WorkerExecutionEnvelope { return w.Envelope }

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
		signal := stopIntervention(control)
		messageWriter.Flush(signal != "" || ctx.Err() != nil)
		if signal == "" {
			signal = stopIntervention(control)
		}
		if signal == ActivitySuspendRequested {
			return WorkerResult{}, providerControlError(ErrExecutionSuspended, messageWriter.err)
		}
		if signal == ActivityCancelRequested {
			return WorkerResult{}, providerControlError(ErrExecutionCancelled, messageWriter.err)
		}
		if ctx.Err() != nil {
			return WorkerResult{}, providerControlError(fmt.Errorf("provider %s interrupted: %w", w.ProviderID, ctx.Err()), messageWriter.err)
		}
		if messageWriter.err != nil {
			return WorkerResult{}, fmt.Errorf("persist provider supervision message: %w", messageWriter.err)
		}
		return WorkerResult{}, fmt.Errorf("provider %s failed: %w: %s", w.ProviderID, err, redactProcessOutput(stderr.String(), os.Environ()))
	}
	messageWriter.Flush(false)
	if signal := stopIntervention(control); signal == ActivitySuspendRequested {
		return WorkerResult{}, providerControlError(ErrExecutionSuspended, messageWriter.err)
	} else if signal == ActivityCancelRequested {
		return WorkerResult{}, providerControlError(ErrExecutionCancelled, messageWriter.err)
	}
	if messageWriter.err != nil {
		return WorkerResult{}, fmt.Errorf("persist provider supervision message: %w", messageWriter.err)
	}
	// These buffers are bounded diagnostic mirrors. They are not returned,
	// persisted, or interpreted. The separate supervision transcript is durably
	// persisted, so mirror truncation cannot invalidate an otherwise successful
	// repository-derived execution.
	return WorkerResult{Outcome: OutcomeContinue, ExecutorID: w.ProviderID, CheckpointEvidence: []string{"provider-process:completed"}}, nil
}

func providerControlError(primary, transcriptErr error) error {
	if transcriptErr == nil {
		return primary
	}
	return fmt.Errorf("%w: %v", primary, transcriptErr)
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
	mu                         sync.Mutex
	cond                       *sync.Cond
	turnCtx                    context.Context
	ctx                        context.Context
	cancel                     context.CancelFunc
	request                    WorkerRequest
	provider                   string
	buffer                     strings.Builder
	queue                      []string
	accepted                   int
	confirmed                  atomic.Int64
	closed                     bool
	done                       chan providerMessagePersistResult
	shutdownTimeout            time.Duration
	interruptedShutdownTimeout time.Duration
	err                        error
}

type providerMessagePersistResult struct {
	confirmed int
	err       error
}

const (
	providerMessageBatchSize                  = 512
	providerMessageShutdownTimeout            = 2 * time.Minute
	providerMessageInterruptedShutdownTimeout = 30 * time.Second
	providerMessageDeadlineGrace              = 30 * time.Second
)

func newProviderMessageWriter(ctx context.Context, request WorkerRequest, provider string) *providerMessageWriter {
	persistCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	w := &providerMessageWriter{
		turnCtx: ctx, ctx: persistCtx, cancel: cancel, request: request, provider: provider,
		done: make(chan providerMessagePersistResult, 1), shutdownTimeout: providerMessageShutdownTimeout,
		interruptedShutdownTimeout: providerMessageInterruptedShutdownTimeout,
	}
	w.cond = sync.NewCond(&w.mu)
	go w.persist()
	return w
}

func (w *providerMessageWriter) setProviderMessageShutdownTimeout(timeout time.Duration) {
	w.shutdownTimeout = timeout
	w.interruptedShutdownTimeout = timeout
}

func (w *providerMessageWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
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
		w.enqueue(line)
	}
	return len(p), nil
}

func (w *providerMessageWriter) Flush(interrupted ...bool) {
	w.mu.Lock()
	if strings.TrimSpace(w.buffer.String()) != "" {
		w.enqueue(strings.TrimSpace(w.buffer.String()))
	}
	w.buffer.Reset()
	w.closed = true
	w.cond.Broadcast()
	total := w.accepted
	w.mu.Unlock()
	timeout := w.flushTimeout(len(interrupted) > 0 && interrupted[0])
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-w.done:
		w.cancel()
		w.err = providerMessagePersistenceError(result.err, result.confirmed, total)
	case <-timer.C:
		w.cancel()
		confirmed := int(w.confirmed.Load())
		w.err = providerMessagePersistenceError(errors.New("shutdown timeout"), confirmed, total)
	}
}

func (w *providerMessageWriter) flushTimeout(interrupted bool) time.Duration {
	timeout := w.shutdownTimeout
	if interrupted || w.turnCtx.Err() != nil {
		timeout = w.interruptedShutdownTimeout
	} else if deadline, ok := w.turnCtx.Deadline(); ok {
		if remaining := time.Until(deadline) + providerMessageDeadlineGrace; remaining > timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return time.Nanosecond
	}
	return timeout
}

func providerMessagePersistenceError(cause error, confirmed, total int) error {
	if cause == nil {
		return nil
	}
	if confirmed < 0 {
		confirmed = 0
	}
	if confirmed > total {
		confirmed = total
	}
	unconfirmed := total - confirmed
	if unconfirmed == 0 {
		return cause
	}
	return fmt.Errorf("provider transcript persistence incomplete: confirmed=%d total=%d unconfirmed=%d ordinals=%d-%d: %w", confirmed, total, unconfirmed, confirmed+1, total, cause)
}

func (w *providerMessageWriter) enqueue(message string) {
	if message == "" {
		return
	}
	w.queue = append(w.queue, message)
	w.accepted++
	w.cond.Signal()
}

func (w *providerMessageWriter) persist() {
	result := providerMessagePersistResult{}
	defer func() { w.done <- result }()
	version, versionKnown := int64(0), false
	for {
		w.mu.Lock()
		for len(w.queue) == 0 && !w.closed {
			w.cond.Wait()
		}
		if len(w.queue) == 0 && w.closed {
			w.mu.Unlock()
			return
		}
		batchSize := len(w.queue)
		if batchSize > providerMessageBatchSize {
			batchSize = providerMessageBatchSize
		}
		messages := append([]string(nil), w.queue[:batchSize]...)
		for i := range w.queue[:batchSize] {
			w.queue[i] = ""
		}
		w.queue = w.queue[batchSize:]
		w.mu.Unlock()
		if err := w.emitBatch(messages, &version, &versionKnown); err != nil {
			result.err = err
			return
		}
		result.confirmed += len(messages)
		w.confirmed.Store(int64(result.confirmed))
	}
}

func (w *providerMessageWriter) emitBatch(messages []string, version *int64, versionKnown *bool) error {
	if w.request.Activity == nil || len(messages) == 0 {
		return nil
	}
	records := make([]ActivityRecord, len(messages))
	for i, message := range messages {
		message = redactProcessOutput(sanitizeActivityText(message), os.Environ())
		records[i] = ActivityRecord{
			Type: ActivityProviderMessage, GoalID: w.request.GoalID, GoalVersion: w.request.GoalVersion,
			InvocationID: w.request.InvocationID, TurnID: w.request.TurnID, ProviderID: w.request.ProviderID,
			Actor: contracts.PrincipalRef{ID: w.provider, Kind: "provider"}, Source: "provider:" + w.provider,
			Trust: contracts.TrustUntrustedContent, Data: sanitizeActivityData(map[string]string{"message": message, "stream": "user-facing"}),
		}
	}
	for attempt := 0; attempt < 5; attempt++ {
		if !*versionKnown {
			current, err := w.request.Activity.StreamVersion(w.ctx, w.request.InvocationID, w.request.TurnID)
			if err != nil {
				return err
			}
			*version = current
			*versionKnown = true
		}
		appended, err := w.request.Activity.AppendBatch(w.ctx, *version, records)
		if err == nil {
			*version = appended[len(appended)-1].StreamVersion
			return nil
		}
		if !errors.Is(err, eventstore.ErrVersionConflict) {
			return err
		}
		*versionKnown = false
	}
	return eventstore.ErrVersionConflict
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
			fmt.Fprintf(&b, "\n## Recovered consequence\nTurn %s ended BLOCKED: %s\nThe checkout already carries that turn's work (consequence fingerprint %s, %s).\n", c.Recovery.RecoveredTurnID, c.Recovery.Blocker, c.Recovery.Fingerprint, c.Recovery.Provenance)
			if len(c.Recovery.Files) > 0 {
				fmt.Fprintf(&b, "Uncommitted paths:\n")
				for _, file := range c.Recovery.Files {
					fmt.Fprintf(&b, "  - %s\n", file)
				}
			}
			if len(c.Recovery.Commits) > 0 {
				fmt.Fprintf(&b, "Unpublished local commits retained as evidence (oldest first):\n")
				for _, commit := range c.Recovery.Commits {
					fmt.Fprintf(&b, "  - %s\n", commit)
				}
			}
			fmt.Fprintf(&b, "Inspect it against this unit. Validate and commit what is correct, fix what is not with a further commit, and remove what should not exist. Nothing may remain uncommitted.\n")
		}
		writeList(&b, "\n## Checkpoint contract (Praxis verifies every item after you finish)", c.Checkpoint.Predicates)
		fmt.Fprintf(&b, "\n## Authority\nGranted capabilities: %s\n", joinCapabilities(c.Authority.Granted))
		writeList(&b, "Forbidden", c.Authority.Forbidden)
		if e := c.Authority.Envelope; e != nil {
			fmt.Fprintf(&b, "\n## Execution envelope\nInteractive prompts: %s\n", map[bool]string{true: "available", false: "none; this session is non-interactive"}[e.Interactive])
			if e.ShellPolicy != "" {
				fmt.Fprintf(&b, "Shell policy: %s\n", e.ShellPolicy)
			}
			if e.DenialPolicy != "" {
				fmt.Fprintf(&b, "Outside the envelope: %s\n", e.DenialPolicy)
			}
			writeList(&b, "Allowed tools (exact patterns; anything else is outside the envelope)", e.Tools)
		}
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

// ClaudeSubscriptionEnvelope is the execution envelope of the Claude
// subscription launch: prompts disabled, the exact tool allowlist, one
// allowed command per shell call.
func ClaudeSubscriptionEnvelope() *WorkerExecutionEnvelope {
	return &WorkerExecutionEnvelope{Interactive: false, Tools: append([]string(nil), claudeSubscriptionTools...),
		ShellPolicy:  "one allowed command per shell call, matched against the allowed patterns by its first words; pipes, loops, command lists (&&, ;, ||) and command substitution are outside the envelope even when every part would be allowed alone",
		DenialPolicy: "a call outside the envelope is denied without a prompt and no human can approve it during the turn; plan within the envelope, use the file tools for reading and searching, and never retry a denied call"}
}

// CodexSubscriptionEnvelope is the execution envelope of the Codex
// subscription launch: a non-interactive workspace-write sandbox.
func CodexSubscriptionEnvelope() *WorkerExecutionEnvelope {
	return &WorkerExecutionEnvelope{Interactive: false, Tools: []string{"shell (sandboxed: workspace-write inside the repository, no network)"},
		ShellPolicy:  "shell commands run inside the workspace-write sandbox; writes outside the repository and network access are outside the envelope",
		DenialPolicy: "a call outside the sandbox fails without a prompt and no human can approve it during the turn; plan within the sandbox and never retry a denied call"}
}

// ClaudeSubscriptionCapabilities are the consequences the Claude
// subscription launch contract grants (see claudeSubscriptionTools).
func ClaudeSubscriptionCapabilities() []WorkerCapability {
	return []WorkerCapability{CapabilityEdit, CapabilityValidate, CapabilityStage, CapabilityCommit}
}

// CodexSubscriptionCapabilities are the consequences the Codex
// workspace-write launch grants.
func CodexSubscriptionCapabilities() []WorkerCapability {
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
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir, Activity: activity, Granted: CodexSubscriptionCapabilities(), Envelope: CodexSubscriptionEnvelope()}, nil
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
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir, Activity: activity, Granted: ClaudeSubscriptionCapabilities(), Envelope: ClaudeSubscriptionEnvelope()}, nil
}
