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
