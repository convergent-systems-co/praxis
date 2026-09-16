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
