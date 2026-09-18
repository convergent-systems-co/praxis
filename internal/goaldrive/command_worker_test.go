package goaldrive

import (
	"context"
	"strings"
	"testing"
)

func TestCommandWorkerUsesExplicitArgvAndBindsProviderIdentity(t *testing.T) {
	worker := CommandWorker{ProviderID: "codex-test", Command: []string{"/usr/bin/printf", `{"outcome":"CONTINUE","end_head":"b","checkpoint_valid":true}`}}
	result, err := worker.Execute(context.Background(), WorkerRequest{GoalID: "goal-1"})
	if err != nil || result.Outcome != OutcomeContinue || result.ExecutorID != "codex-test" || result.EndHead != "b" {
		t.Fatalf("unexpected command worker result: %+v err=%v", result, err)
	}
}

func TestCommandWorkerRejectsMalformedAndOversizedResults(t *testing.T) {
	malformed := CommandWorker{ProviderID: "test", Command: []string{"/usr/bin/printf", "not-json"}}
	if _, err := malformed.Execute(context.Background(), WorkerRequest{}); err == nil {
		t.Fatal("malformed worker output must fail closed")
	}
	oversized := CommandWorker{ProviderID: "test", Command: []string{"/usr/bin/printf", strings.Repeat("x", 32)}, OutputLimit: 8}
	if _, err := oversized.Execute(context.Background(), WorkerRequest{}); err == nil {
		t.Fatal("oversized worker output must fail closed")
	}
}

func TestCommandWorkerUsesSanitizedEnvironmentAndRejectsCredentialOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "should-not-forward")
	worker := CommandWorker{ProviderID: "safe", Command: []string{"/bin/sh", "-c", `if [ -n "${OPENAI_API_KEY:-}" ]; then exit 9; fi; printf '{"outcome":"NO_PROGRESS"}'`}}
	if _, err := worker.Execute(context.Background(), WorkerRequest{}); err != nil {
		t.Fatal(err)
	}
	unsafe := CommandWorker{ProviderID: "unsafe", Env: []string{"OPENAI_API_KEY=secret"}, Command: []string{"/usr/bin/printf", `{"outcome":"NO_PROGRESS"}`}}
	if _, err := unsafe.Execute(context.Background(), WorkerRequest{}); err == nil || !strings.Contains(err.Error(), "not permitted") {
		t.Fatalf("credential-shaped explicit environment must fail closed: %v", err)
	}
}

func TestCommandWorkerRedactsCredentialShapedFailureOutput(t *testing.T) {
	worker := CommandWorker{ProviderID: "redact", Command: []string{"/bin/sh", "-c", `echo 'api_key=super-secret-value' >&2; exit 1`}}
	_, err := worker.Execute(context.Background(), WorkerRequest{})
	if err == nil || strings.Contains(err.Error(), "super-secret-value") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("process credential output was not redacted: %v", err)
	}
}
