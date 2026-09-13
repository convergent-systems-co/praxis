package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
)

// ReplayRun reconstructs durable execution state exclusively from authoritative
// run events. The returned version is the last aggregate version and can be
// assigned directly to EventJournal.ExpectedVersion before resuming execution.
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
			run = &RunExecution{RunID: observation.RunID, GraphID: observation.GraphID, GraphVersion: observation.GraphVersion}
		} else if run.RunID != observation.RunID || run.GraphID != observation.GraphID || run.GraphVersion != observation.GraphVersion {
			return nil, 0, fmt.Errorf("event %d changes run identity", i)
		}

		switch observation.Kind {
		case ObservationRunStarted, ObservationRunResumed:
			run.CurrentNode = observation.NodeID
			run.State = observation.State
			run.TransitionCount = observation.TransitionCount
		case ObservationNodeCompleted:
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
			run.CurrentNode = observation.NodeID
			run.State = observation.State
			run.TransitionCount = observation.TransitionCount
		default:
			return nil, 0, fmt.Errorf("event %d has unknown run observation kind %q", i, observation.Kind)
		}
		lastVersion = event.AggregateVersion
	}
	return run, lastVersion, nil
}
