package goaldrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
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
}

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
	cmd := exec.CommandContext(ctx, w.Command[0], w.Command[1:]...)
	cmd.Dir = w.Dir
	cmd.Env = append([]string(nil), w.Env...)
	cmd.Stdin = bytes.NewReader(payload)
	limit := w.OutputLimit
	if limit <= 0 {
		limit = defaultWorkerOutputLimit
	}
	stdout := &limitedBuffer{limit: limit}
	stderr := &limitedBuffer{limit: limit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return WorkerResult{}, fmt.Errorf("worker %s failed: %w: %s", w.ProviderID, err, stderr.String())
	}
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
