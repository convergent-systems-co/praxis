//go:build darwin || linux || freebsd || netbsd || openbsd

package goaldrive

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// outputFixture is a plain repository whose declared validator is a real
// process; every case below drives the production collector through
// RunDeclaredValidationWith / RunBoundValidation, never Write directly.
func outputFixture(t *testing.T, script string) (GitRepository, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".praxis", "validate"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return GitRepository{Dir: dir, Remote: "origin", Branch: "main"}, dir
}

func emitScript(body string) string { return "#!/bin/sh\n" + body + "\n" }

// N6: exactly the bound passes; anything above it is a refused result whatever
// the exit status, never a truncated success.
func TestValidationOutputBoundFailsClosedThroughTheRealCollector(t *testing.T) {
	const bound = 1 << 20
	cases := []struct {
		name    string
		script  string
		wantErr bool
		wantLen int
	}{
		{"exactly at the bound on stdout", `head -c 1048576 /dev/zero | tr '\0' a`, false, bound},
		{"exactly at the bound on stderr", `head -c 1048576 /dev/zero | tr '\0' a >&2`, false, bound},
		{"exactly at the bound split across both", `head -c 524288 /dev/zero | tr '\0' a; head -c 524288 /dev/zero | tr '\0' b >&2`, false, bound},
		{"one byte over on stdout", `head -c 1048577 /dev/zero | tr '\0' a`, true, 0},
		{"one byte over on stderr", `head -c 1048577 /dev/zero | tr '\0' a >&2`, true, 0},
		{"one byte over across both", `head -c 524288 /dev/zero | tr '\0' a; head -c 524289 /dev/zero | tr '\0' b >&2`, true, 0},
		{"far over on stdout, exit 0", `head -c 2000000 /dev/zero | tr '\0' a; exit 0`, true, 0},
		{"far over on stderr, exit 0", `head -c 2000000 /dev/zero | tr '\0' a >&2; exit 0`, true, 0},
		{"overflow then nonzero exit", `head -c 2000000 /dev/zero | tr '\0' a; exit 3`, true, 0},
		{"interleaved overflow", `i=0; while [ $i -lt 40 ]; do head -c 65536 /dev/zero | tr '\0' a; head -c 65536 /dev/zero | tr '\0' b >&2; i=$((i+1)); done`, true, 0},
		{"unbounded stream", `exec yes aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := outputFixture(t, emitScript(tc.script))
			start := time.Now()
			type outcome struct {
				output string
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				output, err := repo.RunDeclaredValidationWith(context.Background(), "integrated")
				done <- outcome{output, err}
			}()
			var result outcome
			select {
			case result = <-done:
			case <-time.After(20 * time.Second):
				_ = exec.Command("pkill", "-f", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Run()
				t.Fatal("a producer that overflows the bound was not terminated promptly")
			}
			output, err := result.output, result.err
			_ = time.Since(start)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("output above the bound was treated as a pass (len=%d)", len(output))
				}
				if !strings.Contains(err.Error(), "output exceeds") {
					t.Fatalf("overflow was reported as something else: %v", err)
				}
				if len(output) > bound {
					t.Fatalf("more than the bound was retained: %d", len(output))
				}
				return
			}
			if err != nil || len(output) != tc.wantLen {
				t.Fatalf("output exactly at the bound was refused: len=%d err=%v", len(output), err)
			}
		})
	}
}

// The same collector guards the bound (checkpoint-bound) path safety-bearing
// turns use.
func TestBoundValidationOutputOverflowFailsClosed(t *testing.T) {
	validator := "#!/bin/sh\nif [ \"${1:-integrated}\" = integrated ]; then head -c 2000000 /dev/zero | tr '\\0' a; exit 0; fi\nprintf 'PRAXIS-VALIDATION %s\\n' \"$1\"\n"
	f := newExportFixture(t, validator)
	output, err := f.validate()
	if err == nil || !strings.Contains(err.Error(), "output exceeds") {
		t.Fatalf("overflowing bound validation qualified the checkpoint: %v (len=%d)", err, len(output))
	}
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("descendant process %d outlived the validation", pid)
}

func readPID(t *testing.T, dir string) int {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, ".praxis", "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// Frozen plan §4.3 negative tests: timeout with a surviving child, and a
// descendant that redirected its file descriptors and outlives a validator that
// itself returned success. No descendant may survive any validation.
func TestValidationTerminatesTheWholeProcessTree(t *testing.T) {
	previousTimeout, previousDelay := validationTimeout, validationWaitDelay
	t.Cleanup(func() { validationTimeout, validationWaitDelay = previousTimeout, previousDelay })
	cases := []struct {
		name    string
		script  string
		timeout time.Duration
		wantErr bool
	}{
		{"success with a redirected descendant", `sleep 300 >/dev/null 2>&1 </dev/null & echo $! > "$(dirname "$0")/child.pid"; exit 0`, 30 * time.Second, false},
		{"success with a descendant holding stdout", `sleep 300 & echo $! > "$(dirname "$0")/child.pid"; exit 0`, 30 * time.Second, true},
		{"timeout with a surviving child", `sleep 300 & echo $! > "$(dirname "$0")/child.pid"; sleep 300`, 3 * time.Second, true},
		{"failure with a surviving child", `sleep 300 >/dev/null 2>&1 </dev/null & echo $! > "$(dirname "$0")/child.pid"; exit 4`, 30 * time.Second, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validationTimeout, validationWaitDelay = tc.timeout, 500*time.Millisecond
			repo, dir := outputFixture(t, emitScript(tc.script))
			start := time.Now()
			_, err := repo.RunDeclaredValidationWith(context.Background(), "integrated")
			if time.Since(start) > 15*time.Second {
				t.Fatalf("validation was not bounded in time: %s", time.Since(start))
			}
			if tc.wantErr && err == nil {
				t.Fatal("a validation that left a stuck descendant or timed out was treated as a pass")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("a clean validator was refused: %v", err)
			}
			waitDead(t, readPID(t, dir))
		})
	}
}

// A cancelled caller context stops the whole tree too.
func TestValidationCancellationTerminatesDescendants(t *testing.T) {
	repo, dir := outputFixture(t, emitScript(`sleep 300 & echo $! > "$(dirname "$0")/child.pid"; wait`))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(700 * time.Millisecond)
		cancel()
	}()
	if _, err := repo.RunDeclaredValidationWith(ctx, "integrated"); err == nil {
		t.Fatal("a cancelled validation was treated as a pass")
	} else if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "signal: killed") {
		t.Logf("cancellation surfaced as: %v", err)
	}
	waitDead(t, readPID(t, dir))
}
