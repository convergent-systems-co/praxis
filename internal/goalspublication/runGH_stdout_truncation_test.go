package goalspublication

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// TestRunGHReturnsCompletePayloadWithinBound proves functional stdout is no
// longer silently truncated (the original generation-5 defect: a
// boundedCapture{limit: 8192} corrupted any asset over 8192 bytes) while
// still being bounded rather than fully unbounded (the independent-review
// finding: a fully unbounded buffer is a resource-exhaustion surface for
// provider-controlled or unexpectedly large output). A payload that fits
// within the caller-supplied maxStdoutBytes must be returned complete and
// byte-exact.
func TestRunGHReturnsCompletePayloadWithinBound(t *testing.T) {
	dir := t.TempDir()
	const payloadSize = 50000
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	writeFakeGH(t, dir, payload)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := runGH(context.Background(), []string{"api", "whatever"}, nil, "", payloadSize)
	if err != nil {
		t.Fatalf("unexpected runGH error: %v", err)
	}
	if len(got) != payloadSize {
		t.Fatalf("stdout was truncated: got %d bytes, want %d", len(got), payloadSize)
	}
	if sha256.Sum256(got) != sha256.Sum256(payload) {
		t.Fatal("returned payload does not match what the fake gh process wrote to stdout")
	}
}

// TestRunGHFailsClosedWhenOutputExceedsBound proves the fail-closed half of
// the invariant: functional stdout larger than the caller-supplied bound
// must be rejected outright, never silently truncated and never accepted
// unbounded. This is the direct adversarial counterpart to a provider (or a
// compromised/misbehaving one) returning more data than the trusted size
// contract expects — e.g. readAsset's expectedSize from already-verified
// GitHub release metadata.
func TestRunGHFailsClosedWhenOutputExceedsBound(t *testing.T) {
	dir := t.TempDir()
	const bound = 100
	oversized := make([]byte, bound+1)
	writeFakeGH(t, dir, oversized)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := runGH(context.Background(), []string{"api", "whatever"}, nil, "", bound)
	if err == nil {
		t.Fatalf("expected runGH to fail closed on output exceeding the bound, got %d bytes with no error", len(got))
	}
}

// TestReadAssetRejectsExpectedSizeAtOrBelowZero proves readAsset refuses to
// download without a trusted positive size bound — it must never fall back
// to an implicit unbounded read.
func TestReadAssetRejectsExpectedSizeAtOrBelowZero(t *testing.T) {
	for _, size := range []int64{0, -1} {
		if _, err := readAsset(context.Background(), 12345, size); err == nil {
			t.Fatalf("expected readAsset to reject expectedSize=%d", size)
		}
	}
}

// writeFakeGH installs a "gh" executable on dir that writes exactly payload
// to stdout and exits 0, regardless of arguments.
func writeFakeGH(t *testing.T, dir string, payload []byte) {
	t.Helper()
	dataPath := filepath.Join(dir, "gh.payload")
	if err := os.WriteFile(dataPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat \"" + dataPath + "\"\nexit 0\n"
	ghPath := filepath.Join(dir, "gh")
	if err := os.WriteFile(ghPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}
