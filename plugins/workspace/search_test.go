package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchTextReturnsExactEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha\nNeedle here\nomega\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := SearchText(root, []string{"a.txt"}, "needle", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Line != 2 || matches[0].Class != EvidenceExact || matches[0].FileDigest == "" {
		t.Fatalf("unexpected matches: %+v", matches)
	}
}

func TestSearchTextHonorsMatchLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x\nx\nx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := SearchText(root, []string{"a.txt"}, "x", SearchOptions{MaxMatches: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected two matches, got %d", len(matches))
	}
}
