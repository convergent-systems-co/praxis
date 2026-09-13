package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvidenceFreshnessTracksDigest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil { t.Fatal(err) }
	idx := NewIndex()
	record, _, err := idx.UpdateFile(root, "a.txt", time.Now())
	if err != nil { t.Fatal(err) }
	e := EvidenceRef{ID: "e1", Snapshot: SnapshotRef{WorkspaceID: "w1", Revision: "r1"}, Path: "a.txt", FileDigest: record.Digest, Class: EvidenceExact, Sensitivity: SensitivityInternal, Source: "workspace.search.text", ObservedAt: time.Now()}
	if !e.FreshAgainst(idx) { t.Fatal("evidence should initially be fresh") }
	if err := os.WriteFile(path, []byte("two"), 0o600); err != nil { t.Fatal(err) }
	if _, _, err := idx.UpdateFile(root, "a.txt", time.Now()); err != nil { t.Fatal(err) }
	if e.FreshAgainst(idx) { t.Fatal("old evidence must become stale after content changes") }
}
