package goals

import (
	"strings"
	"testing"
)

const intakeDocument = `# Goal: Scratch intake

Bring the scratch service to intake readiness by resolving the defects that
most impede governed operation.

## Scope

The scratch repository only.

## Non-goals

- No change outside the scratch repository.
- No rename.

## Constraints

- Every election is recorded with its rationale
  and the evidence it rests on.

## Success criteria

- go test ./... passes on a clean tree.

## Notes

Unrecognized sections stay in the original intent and derive nothing.
`

func TestBaselineFromProseDerivesFieldsDeterministically(t *testing.T) {
	baseline, err := BaselineFromProse("goal:scratch", "1", []byte(intakeDocument))
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ID != "goal:scratch" || baseline.Version != "1" || baseline.PredecessorDigest != "" || baseline.WorkPlan != nil {
		t.Fatalf("intake must yield a root generation with no WorkPlan: %+v", baseline)
	}
	if baseline.OriginalIntent != intakeDocument {
		t.Fatal("original intent must be the document verbatim")
	}
	wantOutcome := "Bring the scratch service to intake readiness by resolving the defects that\nmost impede governed operation."
	if baseline.RefinedOutcome != wantOutcome {
		t.Fatalf("refined outcome = %q, want %q", baseline.RefinedOutcome, wantOutcome)
	}
	if baseline.Scope != "The scratch repository only." {
		t.Fatalf("scope = %q", baseline.Scope)
	}
	if len(baseline.NonGoals) != 2 || baseline.NonGoals[0] != "No change outside the scratch repository." || baseline.NonGoals[1] != "No rename." {
		t.Fatalf("non-goals = %#v", baseline.NonGoals)
	}
	if len(baseline.Constraints) != 1 || baseline.Constraints[0] != "Every election is recorded with its rationale and the evidence it rests on." {
		t.Fatalf("a wrapped bullet must join into one constraint: %#v", baseline.Constraints)
	}
	if len(baseline.SuccessCriteria) != 1 || baseline.SuccessCriteria[0] != "go test ./... passes on a clean tree." {
		t.Fatalf("success criteria = %#v", baseline.SuccessCriteria)
	}
	if baseline.Rigor != RigorStructured || baseline.RecommendationMode != RecommendationReviewAll {
		t.Fatalf("intake defaults must be structured rigor and review-all: %s %s", baseline.Rigor, baseline.RecommendationMode)
	}
	if len(baseline.EvidenceRefs) != 1 || !strings.HasPrefix(baseline.EvidenceRefs[0], "goal-document:sha256:") {
		t.Fatalf("the document digest must be bound as evidence: %#v", baseline.EvidenceRefs)
	}
	if len(baseline.Assumptions) == 0 || !strings.Contains(baseline.Assumptions[0], "not a human-refined") {
		t.Fatalf("derivation must be disclosed as an assumption: %#v", baseline.Assumptions)
	}
	if len(baseline.Decisions) != 0 {
		t.Fatal("intake must not record decisions on anyone's behalf")
	}
	first, err := baseline.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	again, err := BaselineFromProse("goal:scratch", "1", []byte(intakeDocument))
	if err != nil {
		t.Fatal(err)
	}
	second, err := again.ComputeDigest()
	if err != nil || first != second {
		t.Fatalf("derivation must be deterministic: %q vs %q (%v)", first, second, err)
	}
	changed, err := BaselineFromProse("goal:scratch", "1", []byte(intakeDocument+"\nOne more line.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if third, _ := changed.ComputeDigest(); third == first {
		t.Fatal("changed prose must produce a different baseline digest")
	}
}

func TestBaselineFromProseFailsClosed(t *testing.T) {
	cases := map[string]struct {
		id, version, document string
	}{
		"missing goal id":       {"", "1", intakeDocument},
		"missing version":       {"goal:x", "", intakeDocument},
		"empty document":        {"goal:x", "1", "   \n"},
		"title only":            {"goal:x", "1", "# Goal: only a title\n\n## Scope\n\nx\n"},
		"prose in a list":       {"goal:x", "1", "# T\n\nIntent.\n\n## Non-goals\n\nnot a bullet\n"},
		"duplicate section":     {"goal:x", "1", "# T\n\nIntent.\n\n## Scope\n\na\n\n## Scope\n\nb\n"},
		"empty recognized list": {"goal:x", "1", "# T\n\nIntent.\n\n## Constraints\n\n"},
	}
	for name, tc := range cases {
		if _, err := BaselineFromProse(tc.id, tc.version, []byte(tc.document)); err == nil {
			t.Fatalf("%s: expected refusal", name)
		}
	}
	if _, err := BaselineFromProse("goal:x", "1", []byte("No title at all, just intent.\n")); err != nil {
		t.Fatalf("a document without a title heading is still a valid intent: %v", err)
	}
}

func TestGoalContractsAgreeOnIntakeAndExistingGoalOnlyDrive(t *testing.T) {
	drive := GoalDriveInvocation()
	for _, option := range drive.Options {
		if option.Name == "goal" || option.Name == "goal-file" {
			t.Fatalf("goal-drive runs an existing durable Goal only; %q must not be advertised", option.Name)
		}
		if option.Name == "goal-id" && !strings.Contains(option.Description, "goals-lifecycle") {
			t.Fatalf("goal-id must say how a Goal is introduced: %q", option.Description)
		}
	}
	for _, option := range LifecycleInvocation().Options {
		if option.Name == "operation" && !strings.Contains(option.Description, "intake") {
			t.Fatalf("the operation help must list intake: %q", option.Description)
		}
	}
}
