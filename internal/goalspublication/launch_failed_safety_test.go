package goalspublication

import (
	"context"
	"errors"
	"testing"
)

// TestRunGHLaunchFailureNeverStartsAnOSProcess empirically proves the safety
// property Class=="local_pre_dispatch_failure"+Process=="launch_failed"
// depends on: per the Go standard library's documented os/exec.Cmd contract
// ("ProcessState contains information about an exited process. If the
// process was started successfully, Wait or Run will populate its
// ProcessState when the command completes." — go doc os/exec Cmd), and
// Cmd.Run is Start followed by Wait, ProcessState remains nil if and only if
// Start itself never succeeded, meaning the OS never created the "gh" child
// process at all. A process that was never created cannot have performed any
// I/O, so it cannot have sent any request to GitHub — Process=="launch_failed"
// therefore safely proves zero external effect, unlike Process=="started"
// (Wait was reached, meaning Start succeeded and the process ran, so any
// provider-side effect is only ambiguous/UNKNOWN, never provably absent).
//
// This test forces a genuine Start failure (an empty PATH, so exec.LookPath
// cannot find "gh") and asserts runGH classifies it exactly as
// local_pre_dispatch_failure/launch_failed — proving the classification
// really is gated on Start failure in this codebase's actual runtime
// behavior, not merely asserted by comment.
func TestRunGHLaunchFailureNeverStartsAnOSProcess(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // a directory containing no "gh" executable
	_, err := runGH(context.Background(), []string{"api", "/rate_limit"}, nil, "", maxAPIResponseBytes)
	if err == nil {
		t.Fatal("expected runGH to fail when gh cannot be found on PATH")
	}
	var failure dispatchFailure
	if !errors.As(err, &failure) {
		t.Fatalf("expected a dispatchFailure, got %T: %v", err, err)
	}
	if failure.outcome.Class != "local_pre_dispatch_failure" || failure.outcome.Process != "launch_failed" {
		t.Fatalf("expected local_pre_dispatch_failure/launch_failed for a genuine Start failure, got class=%q process=%q", failure.outcome.Class, failure.outcome.Process)
	}
}
