package contracts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGoalInputLiteralAndID(t *testing.T) {
	literal, err := ResolveGoalInput("literal goal", "", "")
	if err != nil || literal.Kind != GoalInputLiteral || literal.Text != "literal goal" {
		t.Fatalf("literal input mismatch: %+v %v", literal, err)
	}
	id, err := ResolveGoalInput("", "", "goal-1/2")
	if err != nil || id.Kind != GoalInputID || id.GoalID != "goal-1/2" {
		t.Fatalf("ID input mismatch: %+v %v", id, err)
	}
}

func TestResolveGoalInputFileCapturesProvenanceAndDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GOAL.md")
	if err := os.WriteFile(path, []byte("issue inventory"), 0o600); err != nil {
		t.Fatal(err)
	}
	input, err := ResolveGoalInput("", path, "")
	if err != nil {
		t.Fatal(err)
	}
	if input.Kind != GoalInputFile || input.FilePath != path || input.Text != "issue inventory" || input.ContentDigest != "sha256:2f65222ec1372c046f3ca3614dfc3d1fb74462bb11585a240e149ac2b27f7e60" {
		t.Fatalf("file input did not capture expected provenance/digest: %+v", input)
	}
}

func TestResolveGoalInputRejectsAmbiguousOrMissingForms(t *testing.T) {
	for name, args := range map[string][3]string{
		"missing":          {},
		"literal-and-file": {"goal", "GOAL.md", ""},
		"file-and-id":      {"", "GOAL.md", "goal-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveGoalInput(args[0], args[1], args[2]); err == nil {
				t.Fatal("expected GoalInput validation error")
			}
		})
	}
}
