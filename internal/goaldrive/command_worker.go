package goaldrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const defaultWorkerOutputLimit = 1 << 20

// CommandWorker is an explicit-argv provider adapter. It does not invoke a
// shell, interpret worker text, or grant repository authority. The configured
// command must emit exactly one WorkerResult JSON object on stdout.
type CommandWorker struct {
	ProviderID  string
	Command     []string
	Dir         string
	Env         []string
	OutputLimit int
	Activity    *ActivityLog
	// Granted are the operator-declared capabilities of the command worker.
	// nil means the operator asserts the full repository contract; an empty
	// slice is an explicit refusal.
	Granted []WorkerCapability
}

func (w CommandWorker) Capabilities() []WorkerCapability {
	if w.Granted == nil {
		return []WorkerCapability{CapabilityEdit, CapabilityValidate, CapabilityStage, CapabilityCommit}
	}
	return append([]WorkerCapability(nil), w.Granted...)
}

var (
	processCredentialPattern = regexp.MustCompile(`(?i)(authorization[ \t]*:[ \t]*bearer[ \t]+|(?:api[_-]?key|token|secret|password)[ \t]*[=:][ \t]*)[^\s,;]+`)
	safeEnvironmentNames     = map[string]struct{}{
		"HOME": {}, "PATH": {}, "USER": {}, "LOGNAME": {}, "TMPDIR": {}, "TMP": {}, "TEMP": {},
		"LANG": {}, "TERM": {}, "NO_COLOR": {}, "XDG_RUNTIME_DIR": {}, "XDG_CONFIG_HOME": {},
		"XDG_CACHE_HOME": {}, "XDG_DATA_HOME": {}, "XDG_STATE_HOME": {}, "CODEX_HOME": {},
		"CLAUDE_CONFIG_DIR": {}, "SSH_AUTH_SOCK": {}, "GIT_CONFIG_GLOBAL": {}, "SSL_CERT_FILE": {},
	}
)

func (w CommandWorker) Execute(ctx context.Context, request WorkerRequest) (WorkerResult, error) {
	if w.ProviderID == "" {
		return WorkerResult{}, errors.New("worker provider identity is required")
	}
	if len(w.Command) == 0 || w.Command[0] == "" {
		return WorkerResult{}, errors.New("worker command is required")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return WorkerResult{}, fmt.Errorf("encode worker request: %w", err)
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
	cmd.Stdin = bytes.NewReader(payload)
	limit := w.OutputLimit
	if limit <= 0 {
		limit = defaultWorkerOutputLimit
	}
	stdout := &limitedBuffer{limit: limit}
	stderr := &limitedBuffer{limit: limit}
	cmd.Stdout = stdout
	if request.Activity == nil {
		request.Activity = w.Activity
	}
	messageWriter := newProviderMessageWriter(ctx, request, w.ProviderID)
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
		return WorkerResult{}, fmt.Errorf("worker %s failed: %w: %s", w.ProviderID, err, redactProcessOutput(stderr.String(), os.Environ()))
	}
	messageWriter.Flush()
	if messageWriter.err != nil {
		return WorkerResult{}, fmt.Errorf("persist provider supervision message: %w", messageWriter.err)
	}
	stopIntervention(control)
	if stdout.truncated {
		return WorkerResult{}, errors.New("worker result exceeds configured output limit")
	}
	var result WorkerResult
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := decoder.Decode(&result); err != nil {
		return WorkerResult{}, fmt.Errorf("decode worker %s result: %w", w.ProviderID, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return WorkerResult{}, errors.New("worker result must contain exactly one JSON object")
	}
	result.ExecutorID = w.ProviderID
	return result, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func (b *limitedBuffer) String() string { return string(b.Bytes()) }

// commandEnvironment treats a nil Env as a request for the small, non-secret
// runtime environment needed by provider CLIs. Explicit environments remain
// setup-time choices, but credential-shaped names are rejected in either mode.
func commandEnvironment(explicit, inherited []string) ([]string, error) {
	if explicit != nil {
		for _, entry := range explicit {
			if err := validateEnvironmentEntry(entry); err != nil {
				return nil, err
			}
		}
		return append([]string(nil), explicit...), nil
	}
	return sanitizedEnvironment(inherited), nil
}

func sanitizedEnvironment(source []string) []string {
	result := make([]string, 0, len(source))
	for _, entry := range source {
		name, _, ok := strings.Cut(entry, "=")
		if ok && isSafeEnvironmentName(name) {
			result = append(result, entry)
		}
	}
	return result
}

func validateEnvironmentEntry(entry string) error {
	name, _, ok := strings.Cut(entry, "=")
	if !ok || name == "" {
		return errors.New("worker environment entry must be NAME=VALUE")
	}
	if !isSafeEnvironmentName(name) {
		return fmt.Errorf("worker environment variable %q is not permitted; use provider-managed login state or an explicit non-secret runtime variable", name)
	}
	return nil
}

func isSafeEnvironmentName(name string) bool {
	if _, ok := safeEnvironmentNames[name]; ok {
		return true
	}
	return strings.HasPrefix(name, "LC_")
}

func redactProcessOutput(output string, environment []string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && value != "" && !isSafeEnvironmentName(name) {
			output = strings.ReplaceAll(output, value, "[REDACTED]")
		}
	}
	return processCredentialPattern.ReplaceAllString(output, "$1[REDACTED]")
}
