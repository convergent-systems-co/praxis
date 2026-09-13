package workspace

import (
	"errors"
	"time"
)

type SnapshotRef struct {
	WorkspaceID string
	Revision    string
	DirtyDigest string
	IndexedAt   time.Time
}

type EvidenceRef struct {
	ID           string
	Snapshot     SnapshotRef
	Path         string
	FileDigest   string
	Class        EvidenceClass
	Sensitivity  Sensitivity
	Source       string
	ObservedAt   time.Time
}

func (e EvidenceRef) Validate() error {
	if e.ID == "" || e.Snapshot.WorkspaceID == "" || e.Path == "" || e.FileDigest == "" || e.Source == "" {
		return errors.New("evidence id, workspace, path, digest, and source are required")
	}
	switch e.Class {
	case EvidenceExact, EvidenceStructural, EvidenceSemantic, EvidenceInferred:
	default:
		return errors.New("unknown evidence class")
	}
	return nil
}

// FreshAgainst returns true only if the indexed record still has the exact
// digest from which the evidence was produced.
func (e EvidenceRef) FreshAgainst(index *Index) bool {
	if index == nil || e.Validate() != nil {
		return false
	}
	return !index.Stale(e.Path, e.FileDigest)
}
