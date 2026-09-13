package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
)

func ReplayRun(events []eventstore.Event) (*RunExecution, int64, error) {
	if len(events) == 0 {
		return nil, 0, errors.New("run replay requires at least one event")
	}
	var run *RunExecution
	var lastVersion int64
	for i, event := range events {
		if event.AggregateType != "run" || !strings.HasPrefix(event.Type, "run.") {
			return nil, 0, fmt.Errorf("event %d is not a run event", i)
		}
		if event.AggregateVersion != lastVersion+1 {
			return nil, 0, fmt.Errorf("event %d aggregate version gap: expected %d, got %d", i, lastVersion+1, event.AggregateVersion)
		}
		var observation RunObservation
		if err := json.Unmarshal(event.Payload, &observation); err != nil {
			return nil, 0, fmt.Errorf("decode run event %d: %w", i, err)
		}
		if observation.RunID == "" || observation.GraphID == "" || observation.GraphVersion == "" || observation.Kind == "" {
			return nil, 0, fmt.Errorf("event %d contains incomplete run observation", i)
		}
		if event.AggregateID != observation.RunID {
			return nil, 0, fmt.Errorf("event %d aggregate/run id mismatch", i)
		}
		if event.Type != "run."+string(observation.Kind) {
			return nil, 0, fmt.Errorf("event %d type/observation mismatch", i)
		}
		if run == nil {
			run = &RunExecution{RunID: observation.RunID, AgentID: observation.AgentID, GraphID: observation.GraphID, GraphVersion: observation.GraphVersion, AttemptCounts: map[string]int{}}
		} else if run.RunID != observation.RunID || run.GraphID != observation.GraphID || run.GraphVersion != observation.GraphVersion || (observation.AgentID != "" && run.AgentID != observation.AgentID) {
			return nil, 0, fmt.Errorf("event %d changes run identity", i)
		}

		switch observation.Kind {
		case ObservationRunStarted:
			run.CurrentNode = observation.NodeID
			run.State = observation.State
			run.TransitionCount = observation.TransitionCount
		case ObservationRunResumed:
			run.PendingWait = nil
			run.CurrentNode = observation.NodeID
			run.State = observation.State
			run.TransitionCount = observation.TransitionCount
		case ObservationRunStateChanged:
			if observation.State.Terminal() || observation.State == "" {
				return nil, 0, fmt.Errorf("event %d state-change observation has invalid state %q", i, observation.State)
			}
			if observation.Wait != nil {
				if observation.State != RunSuspended {
					return nil, 0, fmt.Errorf("event %d wait reference requires suspended state", i)
				}
				if err := observation.Wait.Validate(); err != nil {
					return nil, 0, fmt.Errorf("event %d invalid wait reference: %w", i, err)
				}
				copyWait := *observation.Wait
				run.PendingWait = &copyWait
			} else if observation.State != RunSuspended {
				run.PendingWait = nil
			}
			run.CurrentNode = observation.NodeID
			run.State = observation.State
			run.TransitionCount = observation.TransitionCount
		case ObservationNodeAttemptFailed:
			if observation.NodeID == "" || observation.Attempt <= 0 || !validFailureClass(observation.FailureClass) {
				return nil, 0, fmt.Errorf("event %d contains invalid failed attempt", i)
			}
			if observation.Attempt <= run.AttemptCounts[observation.NodeID] {
				return nil, 0, fmt.Errorf("event %d attempt did not advance for node %q", i, observation.NodeID)
			}
			run.AttemptCounts[observation.NodeID] = observation.Attempt
		case ObservationNodeCompleted:
			if observation.NodeID == "" || observation.Attempt <= 0 {
				return nil, 0, fmt.Errorf("event %d contains invalid completed attempt", i)
			}
			if observation.Attempt < run.AttemptCounts[observation.NodeID] {
				return nil, 0, fmt.Errorf("event %d completed attempt regressed for node %q", i, observation.NodeID)
			}
			run.AttemptCounts[observation.NodeID] = observation.Attempt
			run.Evidence = append(run.Evidence, observation.Evidence...)
		case ObservationTransitioned:
			if observation.ToNode == "" {
				return nil, 0, fmt.Errorf("event %d transition has no destination", i)
			}
			run.CurrentNode = observation.ToNode
			run.State = observation.State
			run.TransitionCount = observation.TransitionCount
		case ObservationRunTerminal:
			if !observation.State.Terminal() {
				return nil, 0, fmt.Errorf("event %d terminal observation has non-terminal state %q", i, observation.State)
			}
			run.PendingWait = nil
			run.CurrentNode = observation.NodeID
			run.State = observation.State
			run.TransitionCount = observation.TransitionCount
		case ObservationContinuationDecided:
			if observation.Resource == nil || observation.Continuation == nil {
				return nil, 0, fmt.Errorf("event %d continuation observation is incomplete", i)
			}
			if err := observation.Resource.Validate(); err != nil {
				return nil, 0, fmt.Errorf("event %d invalid resource observation: %w", i, err)
			}
			if err := observation.Continuation.Verify(); err != nil {
				return nil, 0, fmt.Errorf("event %d invalid continuation decision: %w", i, err)
			}
			resourceDigest, digestErr := continuationDigest(*observation.Resource)
			if digestErr != nil || observation.Continuation.ObservationDigest != "sha256:"+resourceDigest {
				return nil, 0, fmt.Errorf("event %d continuation does not bind resource observation", i)
			}
			if observation.Resource.RunID != run.RunID || observation.Resource.AgentID != run.AgentID {
				return nil, 0, fmt.Errorf("event %d resource identity mismatch", i)
			}
			if observation.Checkpoint != nil && (observation.Checkpoint.RunID != run.RunID || observation.Checkpoint.AgentID != run.AgentID || observation.Checkpoint.GraphID != run.GraphID || observation.Checkpoint.GraphVersion != run.GraphVersion) {
				return nil, 0, fmt.Errorf("event %d checkpoint identity mismatch", i)
			}
			if observation.Continuation.Action == ContinuationHandoff {
				if observation.State != RunSuspended || observation.Wait == nil || observation.Wait.Kind != WaitHandoff || observation.Wait.Ref != observation.Continuation.HandoffRef || observation.Checkpoint == nil {
					return nil, 0, fmt.Errorf("event %d handoff state is incomplete", i)
				}
				copyWait := *observation.Wait
				run.PendingWait = &copyWait
			}
			run.State = observation.State
			run.CurrentNode = observation.NodeID
			run.TransitionCount = observation.TransitionCount
			run.Evidence = append(run.Evidence, observation.Evidence...)
			run.ResourceObservations = append(run.ResourceObservations, *observation.Resource)
			run.ContinuationHistory = append(run.ContinuationHistory, *observation.Continuation)
			if observation.Checkpoint != nil {
				copyCheckpoint := *observation.Checkpoint
				run.LastCheckpoint = &copyCheckpoint
			}
		default:
			return nil, 0, fmt.Errorf("event %d has unknown run observation kind %q", i, observation.Kind)
		}
		lastVersion = event.AggregateVersion
	}
	return run, lastVersion, nil
}
