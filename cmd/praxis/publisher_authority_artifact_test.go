package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCanonicalPreviewFileCreatesExactArtifactAndRefusesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authority-preview.json")
	payload := []byte("{\"proposal_digest\":\"sha256:test\"}\n")
	if err := writeCanonicalPreviewFile(path, payload); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("artifact changed during write: %q", got)
	}
	if err := writeCanonicalPreviewFile(path, []byte("substituted")); err == nil {
		t.Fatal("existing preview artifact must not be replaced")
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("failed replacement modified artifact: %q", got)
	}
}
