package conformance

import (
	"errors"
	"runtime"
	"sort"
	"time"
)

type ExecutionAttestation struct {
	Version          string            `json:"version"`
	Command          []string          `json:"command"`
	WorkingDirectory string            `json:"working_directory"`
	StartedAt        time.Time         `json:"started_at"`
	FinishedAt       time.Time         `json:"finished_at"`
	ExitCode         int               `json:"exit_code"`
	OutputRef        string            `json:"output_ref"`
	OutputDigest     string            `json:"output_digest"`
	SourceDigests    map[string]string `json:"source_digests"`
	Observations     []string          `json:"observations"`
	Platform         string            `json:"platform"`
	Digest           string            `json:"digest"`
}

func FreezeExecutionAttestation(a ExecutionAttestation) (ExecutionAttestation, error) {
	if len(a.Command) == 0 || a.WorkingDirectory == "" || a.StartedAt.IsZero() || a.FinishedAt.IsZero() || a.OutputRef == "" || a.OutputDigest == "" || len(a.SourceDigests) == 0 {
		return ExecutionAttestation{}, errors.New("command, directory, times, output, and source digests are required")
	}
	if a.FinishedAt.Before(a.StartedAt) {
		return ExecutionAttestation{}, errors.New("attestation finishes before it starts")
	}
	a.Version = "v1"
	a.Platform = runtime.GOOS + "/" + runtime.GOARCH
	a.Observations = append([]string(nil), a.Observations...)
	sort.Strings(a.Observations)
	a.Digest = ""
	d, err := digest(a)
	if err != nil {
		return ExecutionAttestation{}, err
	}
	a.Digest = "sha256:" + d
	return a, nil
}

func VerifyExecutionAttestation(a ExecutionAttestation) error {
	digestValue := a.Digest
	if digestValue == "" || a.Version != "v1" || a.Platform == "" || len(a.Observations) == 0 {
		return errors.New("attestation is not frozen")
	}
	a.Digest = ""
	d, err := digest(a)
	if err != nil {
		return err
	}
	if digestValue != "sha256:"+d {
		return errors.New("execution attestation digest mismatch")
	}
	if a.ExitCode != 0 {
		return errors.New("execution attestation did not pass")
	}
	return nil
}
