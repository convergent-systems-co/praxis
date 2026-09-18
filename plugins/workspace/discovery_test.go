package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverFilesExcludesConfiguredDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "vendor", "dep.go"), []byte("package dep"), 0o600); err != nil { t.Fatal(err) }
	files, err := DiscoverFiles(root, DiscoveryOptions{ExcludeDirs: []string{"vendor"}})
	if err != nil { t.Fatal(err) }
	if len(files) != 1 || files[0] != "main.go" {
		t.Fatalf("unexpected discovered files: %+v", files)
	}
}

func TestDiscoverFilesEnforcesFileLimit(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil { t.Fatal(err) }
	}
	if _, err := DiscoverFiles(root, DiscoveryOptions{MaxFiles: 1}); err == nil {
		t.Fatal("discovery must stop when file limit is exceeded")
	}
}
