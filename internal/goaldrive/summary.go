package goaldrive

import "fmt"

// CheckpointDisposition is the controller's human-facing classification of
// checkpoint state. It distinguishes validated progress from publication.
type CheckpointDisposition string

const (
	CheckpointPublished            CheckpointDisposition = "published"
	CheckpointValidatedUnpublished CheckpointDisposition = "validated_unpublished"
	CheckpointNotCreated           CheckpointDisposition = "not_created"
)

// InvocationSummary is a reporting projection, not a second progress
// predicate. Parent Goal advancement remains outside this turn summary.
type InvocationSummary struct {
	Outcome               Outcome               `json:"outcome"`
	Progress              bool                  `json:"progress"`
	Checkpoint            CheckpointDisposition `json:"checkpoint"`
	ParentGoalAdvancement string                `json:"parent_goal_advancement"`
	StartHead             string                `json:"start_head,omitempty"`
	EndHead               string                `json:"end_head,omitempty"`
}

// SummarizeTurn projects controller-owned facts for human-facing output.
func SummarizeTurn(record TurnRecord) InvocationSummary {
	disposition := CheckpointNotCreated
	if record.Progress {
		if record.CheckpointPublished {
			disposition = CheckpointPublished
		} else {
			disposition = CheckpointValidatedUnpublished
		}
	}
	return InvocationSummary{
		Outcome: record.Outcome, Progress: record.Progress, Checkpoint: disposition,
		ParentGoalAdvancement: "not_claimed", StartHead: record.StartHead, EndHead: record.EndHead,
	}
}

// HumanString keeps outcome, progress, publication, and parent Goal
// advancement explicit so verification cannot be mistaken for progress.
func (s InvocationSummary) HumanString() string {
	return fmt.Sprintf("outcome=%s progress=%t checkpoint=%s parent_goal_advancement=%s start=%s end=%s", s.Outcome, s.Progress, s.Checkpoint, s.ParentGoalAdvancement, s.StartHead, s.EndHead)
}
