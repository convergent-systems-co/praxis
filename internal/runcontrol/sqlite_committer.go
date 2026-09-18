package runcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// SQLiteCommitter is the durable run-control authority boundary. The state
// store validates and consumes the matching capability lease in the same
// transaction that advances the run aggregate and appends the observation.
type SQLiteCommitter struct {
	State *state.Store
	Now   func() time.Time
}

func (c SQLiteCommitter) CommitRunControl(ctx context.Context, actor contracts.PrincipalRef, operation kernel.RunControlOperation, expectedVersion int64, observation kernel.RunObservation, commandID, correlationID string) (int64, error) {
	if c.State == nil {
		return expectedVersion, errors.New("run control state store is required")
	}
	if err := actor.Validate(); err != nil {
		return expectedVersion, err
	}
	if observation.RunID == "" || observation.GraphID == "" || observation.GraphVersion == "" || observation.Kind == "" {
		return expectedVersion, errors.New("run control observation identity is incomplete")
	}
	if commandID == "" || correlationID == "" {
		return expectedVersion, errors.New("run control command id and correlation id are required")
	}
	switch operation {
	case kernel.RunControlCancel:
		if observation.Kind != kernel.ObservationRunTerminal || observation.State != kernel.RunCancelled {
			return expectedVersion, errors.New("cancel operation requires cancelled terminal observation")
		}
	case kernel.RunControlResume:
		if observation.Kind != kernel.ObservationRunResumed || (observation.State != kernel.RunRunning && observation.State != kernel.RunRunnable) {
			return expectedVersion, errors.New("resume operation requires running or runnable resumed observation")
		}
	default:
		return expectedVersion, fmt.Errorf("unsupported run control operation %q", operation)
	}
	payload, err := json.Marshal(observation)
	if err != nil {
		return expectedVersion, fmt.Errorf("marshal run control observation: %w", err)
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	scope := "run:" + observation.RunID
	cmd := state.CommandRecord{
		ID: commandID, Type: "run.control." + string(operation), Version: "1", Actor: actor,
		Scope: scope, CorrelationID: correlationID, Payload: payload, CreatedAt: now,
	}
	event := state.EventRecord{
		ID: fmt.Sprintf("%s:%06d", observation.RunID, expectedVersion+1), AggregateID: observation.RunID,
		AggregateType: "run", AggregateVersion: expectedVersion + 1, Type: "run." + string(observation.Kind), Version: "1",
		Actor: actor, CommandID: commandID, CorrelationID: correlationID, TrustClass: contracts.TrustObserved, Payload: payload, CreatedAt: now,
	}
	_, err = c.State.CommitTransitionAuthorizedLease(ctx, cmd, expectedVersion, event, capability.Request{
		Principal: actor, Capability: kernel.RunControlCapability, Operation: string(operation), Scope: scope,
	})
	if err != nil {
		return expectedVersion, err
	}
	return expectedVersion + 1, nil
}

var _ kernel.RunControlCommitter = SQLiteCommitter{}
