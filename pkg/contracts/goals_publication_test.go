package contracts

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFrozenGoalsCommitMatchesGitAndHasNoParents(t *testing.T) {
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	nonce := strings.Repeat("cd", 32)
	b, tr, c := GoalsPublicationGitObjects(at, nonce)
	for _, o := range []struct {
		kind string
		data []byte
	}{{"blob", b}, {"tree", tr}, {"commit", c}} {
		cmd := exec.Command("git", "hash-object", "-t", o.kind, "--stdin")
		cmd.Stdin = bytes.NewReader(o.data)
		out, e := cmd.Output()
		if e != nil {
			t.Fatal(e)
		}
		if strings.TrimSpace(string(out)) != goalsGitObject(o.kind, o.data) {
			t.Fatal("prepared Git identity differs from Git")
		}
	}
	if bytes.Contains(c, []byte("\nparent ")) {
		t.Fatal("distribution commit has parent")
	}
	if bytes.Contains(b, []byte(goalsGitObject("commit", c))) {
		t.Fatal("descriptor digest cycle")
	}
}
func TestGoalsModelIsDistinctAndNoHistoricalVersionSubstitution(t *testing.T) {
	for _, v := range []string{AuthorityModelVersion, AuthorityModelSuccessorVersion, AuthorityModelDeploymentVersion} {
		if ValidateAuthorityModel(AuthorityModelID, v, AuthorityModelGoalsPublicationDigest()) == nil {
			t.Fatal("v4 digest accepted under historical version")
		}
	}
	if ValidateAuthorityModel(AuthorityModelID, "v4", AuthorityModelDeploymentDigest()) == nil {
		t.Fatal("v3 digest accepted under v4")
	}
	if e := ValidateAuthorityModel(AuthorityModelID, "v4", AuthorityModelGoalsPublicationDigest()); e != nil {
		t.Fatal(e)
	}
}

func TestV5IsSingleAdditiveSuccessorAndKeepsV1ThroughV4(t *testing.T) {
	for _, model := range []struct{ version, digest string }{{"v1", AuthorityModelDigest()}, {"v2", AuthorityModelSuccessorDigest()}, {"v3", AuthorityModelDeploymentDigest()}, {"v4", AuthorityModelGoalsPublicationDigest()}, {"v5", AuthorityModelGoalsRecoveryDigest()}} {
		if err := ValidateAuthorityModel(AuthorityModelID, model.version, model.digest); err != nil {
			t.Fatalf("%s: %v", model.version, err)
		}
	}
	if AuthorityModelGoalsPublicationDigest() == AuthorityModelGoalsRecoveryDigest() {
		t.Fatal("v5 must have a distinct identity")
	}
	if ValidateAuthorityModel(AuthorityModelID, "v5", AuthorityModelGoalsPublicationDigest()) == nil {
		t.Fatal("v4 bytes must not validate as v5")
	}
}

func TestGoalsRecoveryIntentRejectsStateAndLineageSubstitution(t *testing.T) {
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	in := GoalsRecoveryInput{CreatedAt: at, ExpiresAt: at.Add(time.Hour), Identity: strings.Repeat("a", 64), AccountID: 789, Sizes: [3]int64{12, 34, 56}, PredecessorRequestID: "goals-publication-request:old", PredecessorRequestDigest: "sha256:" + strings.Repeat("1", 64), PredecessorIntentID: "goals-initial-publication:old", PredecessorIntentDigest: "sha256:" + strings.Repeat("2", 64), AbandonmentEventID: "goals-publication-abandoned:old", AbandonmentDigest: "sha256:" + strings.Repeat("3", 64)}
	a, err := NewGoalsPublicationRecoveryIntent(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateGoalsPublicationRecoveryIntent(a); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ActionIntent){"destination": func(x *ActionIntent) { x.Target = "github.com/elsewhere/repo" }, "commit": func(x *ActionIntent) { x.Parameters["commit"] = strings.Repeat("f", 40) }, "unknown": func(x *ActionIntent) { x.Parameters["predecessor_manifest_outcome"] = "failed" }, "release": func(x *ActionIntent) { x.Parameters["release_id"] = "1" }, "effects": func(x *ActionIntent) { x.Parameters["permitted_effects"] += " ,push-refs" }, "predecessor": func(x *ActionIntent) { x.Parameters["abandonment_digest"] = "sha256:" + strings.Repeat("4", 64) }} {
		t.Run(name, func(t *testing.T) {
			bad := a
			bad.Parameters = map[string]string{}
			for k, v := range a.Parameters {
				bad.Parameters[k] = v
			}
			bad.Preconditions = map[string]string{}
			for k, v := range a.Preconditions {
				bad.Preconditions[k] = v
			}
			mutate(&bad)
			if ValidateGoalsPublicationRecoveryIntent(bad) == nil {
				t.Fatal("substituted intent accepted")
			}
		})
	}
}

func TestGoalsChainedRecoveryIntentBindsPriorRecovery(t *testing.T) {
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	in := GoalsChainedRecoveryInput{GoalsRecoveryInput: GoalsRecoveryInput{CreatedAt: at, ExpiresAt: at.Add(time.Hour), Identity: strings.Repeat("b", 64), AccountID: 789, Sizes: [3]int64{12, 34, 56}, PredecessorRequestID: "goals-publication-request:old", PredecessorRequestDigest: "sha256:" + strings.Repeat("1", 64), PredecessorIntentID: "goals-initial-publication:old", PredecessorIntentDigest: "sha256:" + strings.Repeat("2", 64), AbandonmentEventID: "goals-publication-abandoned:old", AbandonmentDigest: "sha256:" + strings.Repeat("3", 64)}, PriorRecoveryRequestID: "goals-publication-recovery-request:prior", PriorRecoveryRequestDigest: "sha256:" + strings.Repeat("4", 64), PriorRecoveryIntentID: "goals-established-state-publication:prior", PriorRecoveryIntentDigest: "sha256:" + strings.Repeat("5", 64), PriorRecoveryAuthorityDigest: "sha256:" + strings.Repeat("6", 64), PriorRecoveryExecutionID: "goals-publication-recovery:prior", PriorRecoveryAbandonmentEventID: "goals-publication-recovery-abandoned:prior", PriorRecoveryAbandonmentDigest: "sha256:" + strings.Repeat("7", 64), PriorRecoveryManifestEffectID: "goals-publication-recovery:prior:manifest", PriorRecoveryManifestState: "unknown", PriorRecoveryManifestAttempts: 1, PriorRecoveryManifestRequestDigest: "sha256:" + strings.Repeat("8", 64), PriorRecoveryManifestResultDigest: "sha256:" + strings.Repeat("9", 64), PriorRecoveryManifestReconciliationDigest: "sha256:" + strings.Repeat("a", 64)}
	a, err := NewGoalsPublicationChainedRecoveryIntent(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateGoalsPublicationRecoveryIntent(a); err != nil {
		t.Fatal(err)
	}
	a.Parameters["prior_recovery_intent_id"] = ""
	if ValidateGoalsPublicationRecoveryIntent(a) == nil {
		t.Fatal("substituted prior recovery lineage accepted")
	}
}

func TestOrderedRecoveryChainIsDeterministicAndRejectsOrderOrOmission(t *testing.T) {
	base := func(n string) RecoveryGenerationBinding {
		return RecoveryGenerationBinding{RequestID: "goals-publication-recovery-request:" + n, RequestDigest: "sha256:" + strings.Repeat(n, 64)[:64], IntentID: "intent:" + n, IntentDigest: "sha256:" + strings.Repeat("a", 64), AuthorityDigest: "sha256:" + strings.Repeat("b", 64), ExecutionID: "exec:" + n, AbandonmentEventID: "abandon:" + n, AbandonmentDigest: "sha256:" + strings.Repeat("c", 64), ManifestEffectID: "effect:" + n, ManifestState: "unknown", ManifestAttempts: 1, ManifestRequestDigest: "sha256:" + strings.Repeat("d", 64), ManifestResultDigest: "sha256:" + strings.Repeat("e", 64), ManifestReconciliationDigest: "sha256:" + strings.Repeat("f", 64)}
	}
	chain := []RecoveryGenerationBinding{base("1"), base("2")}
	encoded, err := EncodeRecoveryChain(chain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseRecoveryChain(encoded)
	if err != nil || len(got) != 2 {
		t.Fatalf("chain parse failed: %v", err)
	}
	if _, err = ParseRecoveryChain(encoded + ";"); err == nil {
		t.Fatal("omitted/empty generation accepted")
	}
	reversed := mustChainEncode(t, []RecoveryGenerationBinding{chain[1], chain[0]})
	if reversed == encoded {
		t.Fatal("reordered chain kept identical canonical identity")
	}
}

func TestFailedVerificationRecoveryClosesScopeAndPreservesFailedProof(t *testing.T) {
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	d := func(c byte) string { return "sha256:" + strings.Repeat("1", 64) }
	in := GoalsFailedVerificationInput{
		GoalsOrderedRecoveryInput: GoalsOrderedRecoveryInput{GoalsRecoveryInput: GoalsRecoveryInput{CreatedAt: at, ExpiresAt: at.Add(time.Hour), Identity: strings.Repeat("d", 64), AccountID: 789, Sizes: [3]int64{12, 34, 56}, PredecessorRequestID: "goals-publication-request:old", PredecessorRequestDigest: d('1'), PredecessorIntentID: "goals-initial-publication:old", PredecessorIntentDigest: d('2'), AbandonmentEventID: "goals-publication-abandoned:old", AbandonmentDigest: d('3')}},
		FailedRequestID:           "goals-publication-recovery-request:failed", FailedRequestDigest: d('4'), FailedIntentID: "goals-established-state-publication:failed", FailedIntentDigest: d('5'), FailedAuthorityDigest: d('6'), FailedExecutionID: "goals-publication-recovery:failed", FailedManifestEffectID: "goals-publication-recovery:failed:manifest", FailedArchiveEffectID: "goals-publication-recovery:failed:archive", FailedSignatureEffectID: "goals-publication-recovery:failed:signature", FailedVerifyEffectID: "goals-publication-recovery:failed:verify-draft", FailedManifestState: "succeeded", FailedArchiveState: "succeeded", FailedSignatureState: "succeeded", FailedVerifyState: "failed", FailedVerifyAttempts: 1, FailedManifestRequestDigest: d('7'), FailedArchiveRequestDigest: d('8'), FailedSignatureRequestDigest: d('9'), FailedVerifyRequestDigest: d('a'), FailedVerifyResultDigest: d('b'), FailedVerifyReconciliationDigest: d('c'), AssetIDs: [3]string{"1", "2", "3"},
	}
	a, err := NewGoalsPublicationFailedVerificationIntent(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGoalsPublicationRecoveryIntent(a); err != nil {
		t.Fatal(err)
	}
	if a.Parameters["permitted_effects"] != "verify-draft-assets,publish-existing-release,verify-published-release" {
		t.Fatal("upload effects remain authorized")
	}
	a.Parameters["failed_verify_state"] = "succeeded"
	if ValidateGoalsPublicationRecoveryIntent(a) == nil {
		t.Fatal("failed proof substitution accepted")
	}
}

func mustChainEncode(t *testing.T, c []RecoveryGenerationBinding) string {
	t.Helper()
	s, err := EncodeRecoveryChain(c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
