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
	return fmt.Sprintf(`You are a bounded Praxis provider worker.

Work only on this already-selected child objective: %s
Goal: %s version %s
Turn: %s
Graph: %s version %s
Starting repository HEAD: %s

Do not select another objective. Do not decide Goal completion or authority.
Do not push, rewrite remote history, alter Praxis durable state, or claim that
a checkpoint is valid. Make only the bounded repository changes needed for the
selected objective, and create a local Git commit when those changes are
ready. Praxis will inspect the clean changed repository, decide progress and
checkpoint validity, publish only through controller policy, and decide the
next invocation.
`, request.ChildObjective, request.GoalID, request.GoalVersion, request.TurnID, request.GraphID, request.GraphVersion, request.StartHead), nil
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
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir, Activity: activity}, nil
}

func NewClaudeSubscriptionWorker(providerID, dir, model string, activity *ActivityLog) (Worker, error) {
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
	return ProviderCLIWorker{ProviderID: providerID, Command: append([]string{executable}, args...), Dir: dir, Activity: activity}, nil
}
