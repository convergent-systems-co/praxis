package contracts

import "errors"

// RepositoryState is the controller's pre-turn authority classification.
type RepositoryState string

const (
	RepositorySynced      RepositoryState = "SYNCED"
	RepositoryDirty       RepositoryState = "DIRTY"
	RepositoryLocalAhead  RepositoryState = "LOCAL_AHEAD"
	RepositoryRemoteAhead RepositoryState = "REMOTE_AHEAD"
	RepositoryDiverged    RepositoryState = "DIVERGED"
	RepositoryUnknown     RepositoryState = "UNKNOWN"
)

// RepositoryRelation is determined by a VCS adapter; the controller owns the
// policy applied to the relation and must not infer it from worker prose.
type RepositoryRelation string

const (
	RelationEqual       RepositoryRelation = "equal"
	RelationLocalAhead  RepositoryRelation = "local_ahead"
	RelationRemoteAhead RepositoryRelation = "remote_ahead"
	RelationDiverged    RepositoryRelation = "diverged"
	RelationUnknown     RepositoryRelation = "unknown"
)

// ClassifyRepositoryState applies the deterministic pre-turn safety rule.
// Dirtiness always wins: a dirty checkout must not enter another worker turn.
func ClassifyRepositoryState(clean bool, relation RepositoryRelation) RepositoryState {
	if !clean {
		return RepositoryDirty
	}
	switch relation {
	case RelationEqual:
		return RepositorySynced
	case RelationLocalAhead:
		return RepositoryLocalAhead
	case RelationRemoteAhead:
		return RepositoryRemoteAhead
	case RelationDiverged:
		return RepositoryDiverged
	default:
		return RepositoryUnknown
	}
}

// ValidateCheckpointProgress is the controller-owned progress predicate. A
// worker result or CONTINUE status cannot substitute for a clean validated
// checkpoint whose repository identity changed.
func ValidateCheckpointProgress(startHead, endHead string, clean, checkpointValid bool) (bool, error) {
	if startHead == "" || endHead == "" {
		return false, errors.New("start and end repository identities are required")
	}
	if !clean {
		return false, errors.New("checkpoint working tree is not clean")
	}
	if !checkpointValid {
		return false, errors.New("checkpoint validation failed")
	}
	return startHead != endHead, nil
}
