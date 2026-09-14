package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/convergent-systems-co/praxis/internal/conformance"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "praxis-conformance:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: praxis-conformance blind|qualify|attest [options]")
	}
	switch args[0] {
	case "blind":
		return runBlind(args[1:])
	case "qualify":
		return runQualify(args[1:])
	case "attest":
		return runAttest(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

type stringList []string

func (s *stringList) String() string         { return fmt.Sprint([]string(*s)) }
func (s *stringList) Set(value string) error { *s = append(*s, value); return nil }

func runAttest(args []string) error {
	flags := flag.NewFlagSet("attest", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	out := flags.String("out", "", "attestation path")
	output := flags.String("output", "", "raw command output path")
	var sources, observations stringList
	flags.Var(&sources, "source", "content source to bind; repeatable")
	flags.Var(&observations, "observation", "expected observation label; repeatable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	command := flags.Args()
	if *out == "" || *output == "" || len(sources) == 0 || len(command) == 0 {
		return errors.New("attest requires -out, -output, at least one -source, and a command after --")
	}
	started := time.Now().UTC()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = *root
	raw, runErr := cmd.CombinedOutput()
	finished := time.Now().UTC()
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*output, raw, 0o644); err != nil {
		return err
	}
	sourceDigests := map[string]string{}
	for _, source := range sources {
		d, err := conformance.SourceSetDigest(*root, []string{source})
		if err != nil {
			return err
		}
		sourceDigests[filepath.ToSlash(source)] = d
	}
	outputRel, err := filepath.Rel(*root, *output)
	if err != nil {
		return err
	}
	outputDigest, err := conformance.SourceSetDigest(*root, []string{filepath.ToSlash(outputRel)})
	if err != nil {
		return err
	}
	exitCode := 0
	if runErr != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	attestation, err := conformance.FreezeExecutionAttestation(conformance.ExecutionAttestation{Command: command, WorkingDirectory: ".", StartedAt: started, FinishedAt: finished, ExitCode: exitCode, OutputRef: filepath.ToSlash(outputRel), OutputDigest: outputDigest, SourceDigests: sourceDigests, Observations: observations})
	if err != nil {
		return err
	}
	if err := writeJSON(*out, attestation); err != nil {
		return err
	}
	if runErr != nil {
		return fmt.Errorf("command failed with exit %d; failed attestation preserved", exitCode)
	}
	return nil
}

func runBlind(args []string) error {
	flags := flag.NewFlagSet("blind", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	out := flags.String("out", "", "frozen report path")
	at := flags.String("at", "", "required RFC3339 freeze time")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *out == "" || *at == "" {
		return errors.New("blind requires -out and -at")
	}
	freezeTime, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		return err
	}
	evidence, err := conformance.LoadEvidence(*root, conformance.PraxisEvidenceInventory())
	if err != nil {
		return err
	}
	goalDigest, err := conformance.SourceSetDigest(*root, conformance.OriginalIntentSources)
	if err != nil {
		return err
	}
	result, err := conformance.Evaluate(goalDigest, conformance.PraxisOriginalIntentClaims(), evidence, freezeTime)
	if err != nil {
		return err
	}
	return writeJSON(*out, result)
}

func runQualify(args []string) error {
	flags := flag.NewFlagSet("qualify", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	reportPath := flags.String("report", "", "already-frozen blind report")
	oraclePath := flags.String("oracle", "", "versioned oracle path, or legacy oracle bytes")
	authorityPath := flags.String("authority", "", "authority manifest for legacy oracle bytes")
	out := flags.String("out", "", "qualification score path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *reportPath == "" || *oraclePath == "" || *out == "" {
		return errors.New("qualify requires -report, -oracle, and -out")
	}
	var result conformance.Result
	if err := readJSON(*reportPath, &result); err != nil {
		return err
	}
	oracle, err := conformance.LoadOracle(*root, *oraclePath, *authorityPath)
	if err != nil {
		return err
	}
	score, err := conformance.ScoreQualified(result, oracle)
	if err != nil {
		return err
	}
	oracleDigest, err := conformance.SourceSetDigest(*root, []string{filepath.ToSlash(*oraclePath)})
	if err != nil {
		return err
	}
	authorityDigest := ""
	if *authorityPath != "" {
		authorityDigest, err = conformance.SourceSetDigest(*root, []string{filepath.ToSlash(*authorityPath)})
		if err != nil {
			return err
		}
	}
	return writeJSON(*out, struct {
		FrozenResultDigest string                  `json:"frozen_result_digest"`
		OraclePath         string                  `json:"oracle_path"`
		AuthorityPath      string                  `json:"authority_path,omitempty"`
		OracleDigest       string                  `json:"oracle_digest"`
		AuthorityDigest    string                  `json:"authority_digest,omitempty"`
		OracleID           string                  `json:"oracle_id"`
		OracleGeneration   int                     `json:"oracle_generation"`
		Scope              string                  `json:"scope"`
		ClaimSetDigest     string                  `json:"claim_set_digest"`
		GoalDigest         string                  `json:"goal_digest"`
		Score              conformance.OracleScore `json:"score"`
	}{FrozenResultDigest: result.Digest, OraclePath: filepath.ToSlash(*oraclePath), AuthorityPath: filepath.ToSlash(*authorityPath), OracleDigest: oracleDigest, AuthorityDigest: authorityDigest, OracleID: oracle.OracleID, OracleGeneration: oracle.Generation, Scope: oracle.Scope, ClaimSetDigest: oracle.ClaimSetDigest, GoalDigest: oracle.GoalDigest, Score: score})
}

func readJSON(path string, target any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
