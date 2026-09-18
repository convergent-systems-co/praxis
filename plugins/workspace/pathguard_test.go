package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAuthorizedPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveAuthorizedPath(root, "escape"); !errors.Is(err, ErrWorkspaceEscape) {
		t.Fatalf("expected workspace escape rejection, got %v", err)
	}
}

func TestResolveAuthorizedPathAllowsContainedFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "ok.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveAuthorizedPath(root, "ok.txt")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatal(err)
	}
	want, err = filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("expected canonical path %s got %s", want, resolved)
	}
}
