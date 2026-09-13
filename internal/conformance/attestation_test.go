package conformance

import (
	"testing"
	"time"
)

func TestExecutionAttestationMustPassAndRemainImmutable(t *testing.T) {
	a, err := FreezeExecutionAttestation(ExecutionAttestation{Command: []string{"go", "test", "./x"}, WorkingDirectory: ".", StartedAt: time.Date(2026, 9, 13, 1, 0, 0, 0, time.UTC), FinishedAt: time.Date(2026, 9, 13, 1, 0, 1, 0, time.UTC), OutputRef: "run.log", OutputDigest: "sha256:output", SourceDigests: map[string]string{"x_test.go": "sha256:source"}, Observations: []string{"TestX"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutionAttestation(a); err != nil {
		t.Fatal(err)
	}
	a.Observations[0] = "mutated"
	if err := VerifyExecutionAttestation(a); err == nil {
		t.Fatal("mutated attestation verified")
	}
	failed := a
	failed.Observations = []string{"TestX"}
	failed.ExitCode = 1
	failed.Digest = ""
	failed, err = FreezeExecutionAttestation(failed)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutionAttestation(failed); err == nil {
		t.Fatal("failed execution attested support")
	}
}
