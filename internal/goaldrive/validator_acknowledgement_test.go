package goaldrive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func declareValidator(t *testing.T, workDir, script string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workDir, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workDir, ".praxis", "validate"), script)
	if err := os.Chmod(filepath.Join(workDir, ".praxis", "validate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, workDir, "add", ".praxis")
	runGitTest(t, workDir, "commit", "-q", "-m", "validator")
	runGitTest(t, workDir, "push", "-q", "origin", "main")
	return strings.TrimSpace(runGitOutput(t, workDir, "rev-parse", "HEAD"))
}

func evaluateBindings(t *testing.T, workDir, head string, bindings ...string) map[string]PredicateEvaluation {
	t.Helper()
	baseline := chainBaseline()
	baseline.Constraints = []string{"no secrets"}
	baseline.ValidityPredicates = bindings
	candidate := GoalCompletionCandidate{GoalID: baseline.ID, GoalVersion: baseline.Version, GoalDigest: baseline.Digest, TurnID: "t:turn:1", InvocationID: "t", FinalHead: head, Assessment: GoalCompletionAssessment{AllUnitsComplete: true}}
	evaluation, err := EvaluateDeterministically(context.Background(), baseline, candidate, GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}, contracts.PrincipalRef{ID: "verifier", Kind: "controller"})
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]PredicateEvaluation{}
	for _, item := range evaluation.Items {
		items[item.Ref] = item
	}
	return items
}

func hasEvidence(item PredicateEvaluation, fragment string) bool {
	for _, evidence := range item.Evidence {
		if strings.Contains(evidence, fragment) {
			return true
		}
	}
	return false
}

// TestBoundRefIsUnknownWhenTheValidatorDoesNotAcknowledgeIt is the #168
// contract. Weather II's validator ran the whole suite whatever argument it
// received, so five bound refs were reported SATISFIED by one
// undifferentiated run. A validator that does not acknowledge the exact
// ref it was asked to verify yields UNKNOWN, never SATISFIED; the
// integrated validation keeps its whole-suite semantics.
func TestBoundRefIsUnknownWhenTheValidatorDoesNotAcknowledgeIt(t *testing.T) {
	_, workDir := contractRepo(t)
	head := declareValidator(t, workDir, "#!/bin/sh\n# ignores its argument, like the Weather II validator\nexit 0\n")
	items := evaluateBindings(t, workDir, head, "verify success_criteria/1 with declared-validation", "verify constraint/1 with declared-validation")
	if items["success_criteria/1"].Result != ResultUnknown || items["constraint/1"].Result != ResultUnknown {
		t.Fatalf("RED #168: a validator that never acknowledges the bound ref must yield UNKNOWN, got success_criteria/1=%s constraint/1=%s", items["success_criteria/1"].Result, items["constraint/1"].Result)
	}
	if !hasEvidence(items["success_criteria/1"], "did not acknowledge success_criteria/1") {
		t.Fatalf("the evidence must say why: %v", items["success_criteria/1"].Evidence)
	}
	if items[IntegratedValidator].Result != ResultSatisfied {
		t.Fatalf("integrated validation keeps whole-suite semantics: %s", items[IntegratedValidator].Result)
	}
}

// TestAcknowledgedRefsAreVerifiedByExitStatus proves the positive contract
// and its fail-closed edges: an acknowledged ref is SATISFIED or
// UNSATISFIED by exit status; an acknowledgement naming another ref is
// UNKNOWN; an explicit `unhandled` acknowledgement is UNKNOWN.
func TestAcknowledgedRefsAreVerifiedByExitStatus(t *testing.T) {
	_, workDir := contractRepo(t)
	script := "#!/bin/sh\ncase \"${1:-integrated}\" in\n" +
		"  success_criteria/1) echo 'praxis-verify: success_criteria/1'; exit 0;;\n" +
		"  success_criteria/2) echo 'praxis-verify: success_criteria/2'; echo 'criterion 2 not met'; exit 1;;\n" +
		"  success_criteria/3) echo 'praxis-verify: success_criteria/1'; exit 0;;\n" +
		"  constraint/1) echo 'praxis-verify: constraint/1 unhandled'; exit 0;;\n" +
		"  integrated) exit 0;;\n" +
		"  *) exit 0;;\n" +
		"esac\n"
	head := declareValidator(t, workDir, script)
	items := evaluateBindings(t, workDir, head, "verify success_criteria/1 with declared-validation", "verify success_criteria/2 with declared-validation", "verify success_criteria/3 with declared-validation", "verify constraint/1 with declared-validation")
	if items["success_criteria/1"].Result != ResultSatisfied || !hasEvidence(items["success_criteria/1"], "acknowledged:success_criteria/1") {
		t.Fatalf("an acknowledged passing ref is SATISFIED with the acknowledgement as evidence: %+v", items["success_criteria/1"])
	}
	if items["success_criteria/2"].Result != ResultUnsatisfied {
		t.Fatalf("an acknowledged failing ref is UNSATISFIED: %+v", items["success_criteria/2"])
	}
	if items["success_criteria/3"].Result != ResultUnknown || !hasEvidence(items["success_criteria/3"], "acknowledged success_criteria/1, not success_criteria/3") {
		t.Fatalf("an acknowledgement of another ref is UNKNOWN: %+v", items["success_criteria/3"])
	}
	if items["constraint/1"].Result != ResultUnknown || !hasEvidence(items["constraint/1"], "unhandled") {
		t.Fatalf("an explicit unhandled acknowledgement is UNKNOWN: %+v", items["constraint/1"])
	}
	if items[IntegratedValidator].Result != ResultSatisfied {
		t.Fatalf("integrated: %s", items[IntegratedValidator].Result)
	}
}
