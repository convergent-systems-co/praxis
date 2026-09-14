package contracts

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
)

// GoalInputKind identifies the authoritative source used to resolve a Goal.
// The source is immutable evidence: changing file contents or literal input
// creates a new Goal generation rather than mutating an existing one.
type GoalInputKind string

const (
	GoalInputLiteral GoalInputKind = "literal"
	GoalInputFile    GoalInputKind = "file"
	GoalInputID      GoalInputKind = "goal_id"
)

// GoalInput is the normalized, provenance-bearing input to a Goals invocation.
// GoalID is intentionally opaque here; the Goal repository owns generation and
// authority validation.
type GoalInput struct {
	Kind          GoalInputKind `json:"kind"`
	Text          string        `json:"text,omitempty"`
	FilePath      string        `json:"file_path,omitempty"`
	ContentDigest string        `json:"content_digest,omitempty"`
	GoalID        string        `json:"goal_id,omitempty"`
}

// ResolveGoalInput enforces the shared mutually-exclusive GoalInput contract.
// File content is captured and digested immediately; the path is provenance,
// not a durable Goal identity.
func ResolveGoalInput(literal, filePath, goalID string) (GoalInput, error) {
	count := 0
	if literal != "" {
		count++
	}
	if filePath != "" {
		count++
	}
	if goalID != "" {
		count++
	}
	if count > 1 {
		return GoalInput{}, errors.New("Goal input forms are mutually exclusive")
	}
	if count == 0 {
		return GoalInput{}, errors.New("one Goal input form is required")
	}
	if literal != "" {
		return GoalInput{Kind: GoalInputLiteral, Text: literal}, nil
	}
	if goalID != "" {
		return GoalInput{Kind: GoalInputID, GoalID: goalID}, nil
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return GoalInput{}, fmt.Errorf("read Goal input file: %w", err)
	}
	digest := sha256.Sum256(content)
	return GoalInput{Kind: GoalInputFile, FilePath: filePath, ContentDigest: fmt.Sprintf("sha256:%x", digest[:]), Text: string(content)}, nil
}
