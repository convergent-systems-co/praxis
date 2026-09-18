package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexDetectsContentChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx := NewIndex()
	first, changed, err := idx.UpdateFile(root, "a.txt", time.Now())
	if err != nil || !changed {
		t.Fatalf("initial index failed changed=%v err=%v", changed, err)
	}
	if err := os.WriteFile(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, changed, err := idx.UpdateFile(root, "a.txt", time.Now())
	if err != nil || !changed {
		t.Fatalf("changed file not detected changed=%v err=%v", changed, err)
	}
	if first.Digest == second.Digest {
		t.Fatal("content change must change digest")
	}
	if !idx.Stale("a.txt", first.Digest) {
		t.Fatal("old digest must be stale")
	}
}

func TestIndexUnchangedFileReportsNoChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx := NewIndex()
	if _, _, err := idx.UpdateFile(root, "a.txt", time.Now()); err != nil {
		t.Fatal(err)
	}
	_, changed, err := idx.UpdateFile(root, "a.txt", time.Now())
	if err != nil || changed {
		t.Fatalf("unchanged file should not invalidate index changed=%v err=%v", changed, err)
	}
}
