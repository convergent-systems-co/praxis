package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProviderWorkspaceState is an immutable lifecycle snapshot. Each transition
// is persisted as a new version; no mutable "current workspace" pointer is
// authoritative.
type ProviderWorkspaceState string

const (
	ProviderWorkspaceCreated          ProviderWorkspaceState = "created"
	ProviderWorkspaceActive           ProviderWorkspaceState = "active"
	ProviderWorkspaceDirtyRecoverable ProviderWorkspaceState = "dirty_recoverable"
	ProviderWorkspaceCheckpointed     ProviderWorkspaceState = "checkpointed"
	ProviderWorkspaceValidated        ProviderWorkspaceState = "validated"
	ProviderWorkspacePublished        ProviderWorkspaceState = "published"
	ProviderWorkspaceBlocked          ProviderWorkspaceState = "blocked"
	ProviderWorkspaceAbandoned        ProviderWorkspaceState = "abandoned"
	ProviderWorkspaceCleaned          ProviderWorkspaceState = "cleaned"
)

// ProviderWorkspaceRecord binds mutable provider files to one exact bounded
// turn. It is metadata only; provider transcripts and secrets do not belong in
// this record.
type ProviderWorkspaceRecord struct {
	WorkspaceID    string                 `json:"workspace_id"`
	Version        string                 `json:"version"`
	Path           string                 `json:"path"`
	Repository     string                 `json:"repository"`
	GoalID         string                 `json:"goal_id"`
	GoalVersion    string                 `json:"goal_version"`
	WorkPlanRef    string                 `json:"work_plan_ref"`
	WorkPlanDigest string                 `json:"work_plan_digest"`
	ChildObjective string                 `json:"child_objective"`
	InvocationID   string                 `json:"invocation_id"`
	TurnID         string                 `json:"turn_id"`
	ProviderID     string                 `json:"provider_id"`
	StartHead      string                 `json:"start_head"`
	EndHead        string                 `json:"end_head,omitempty"`
	State          ProviderWorkspaceState `json:"state"`
	CreatedAt      time.Time              `json:"created_at"`
}

func (r ProviderWorkspaceRecord) Validate() error {
	if r.WorkspaceID == "" || r.Version == "" || r.Path == "" || r.Repository == "" || r.GoalID == "" || r.GoalVersion == "" || r.WorkPlanRef == "" || r.WorkPlanDigest == "" || r.ChildObjective == "" || r.InvocationID == "" || r.TurnID == "" || r.ProviderID == "" || r.StartHead == "" || r.CreatedAt.IsZero() {
		return errors.New("provider workspace identity and turn binding are required")
	}
	if !filepath.IsAbs(r.Path) || !filepath.IsAbs(r.Repository) {
		return errors.New("provider workspace and repository paths must be absolute")
	}
	if _, err := strconv.Atoi(r.Version); err != nil {
		return errors.New("provider workspace version must be numeric")
	}
	switch r.State {
	case ProviderWorkspaceCreated, ProviderWorkspaceActive, ProviderWorkspaceDirtyRecoverable, ProviderWorkspaceCheckpointed, ProviderWorkspaceValidated, ProviderWorkspacePublished, ProviderWorkspaceBlocked, ProviderWorkspaceAbandoned, ProviderWorkspaceCleaned:
	default:
		return fmt.Errorf("unknown provider workspace state %q", r.State)
	}
	if r.EndHead != "" && strings.TrimSpace(r.EndHead) == "" {
		return errors.New("provider workspace end HEAD is invalid")
	}
	return nil
}

func (r ProviderWorkspaceRecord) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (r ProviderWorkspaceRecord) Next(state ProviderWorkspaceState, endHead string, createdAt time.Time) (ProviderWorkspaceRecord, error) {
	version, err := strconv.Atoi(r.Version)
	if err != nil {
		return ProviderWorkspaceRecord{}, err
	}
	r.Version = strconv.Itoa(version + 1)
	r.State, r.EndHead, r.CreatedAt = state, endHead, createdAt.UTC()
	if err := r.Validate(); err != nil {
		return ProviderWorkspaceRecord{}, err
	}
	return r, nil
}
