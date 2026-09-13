package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// EventJournal persists RunObserved observations to the append-only event
// store using optimistic aggregate versions. A journal can resume from a
// known aggregate version after replay/recovery.
type EventJournal struct {
	Store           eventstore.Store
	Actor           contracts.PrincipalRef
	CommandID       string
	CorrelationID   string
	CausationID     string
	Trust           contracts.TrustClass
	ExpectedVersion int64
	Now             func() time.Time
}

func (j *EventJournal) ObserveRun(ctx context.Context, observation RunObservation) error {
	if j == nil || j.Store == nil {
		return errors.New("event journal store is required")
	}
	if err := j.Actor.Validate(); err != nil {
		return fmt.Errorf("journal actor: %w", err)
	}
	if j.CommandID == "" || j.CorrelationID == "" {
		return errors.New("journal command id and correlation id are required")
	}
	if observation.RunID == "" || observation.Kind == "" {
		return errors.New("run observation id and kind are required")
	}
	payload, err := json.Marshal(observation)
	if err != nil {
		return fmt.Errorf("marshal run observation: %w", err)
	}
	now := time.Now().UTC()
	if j.Now != nil {
		now = j.Now().UTC()
	}
	trust := j.Trust
	if trust == "" {
		trust = contracts.TrustObserved
	}
	event := eventstore.Event{
		ID:            fmt.Sprintf("%s:%06d", observation.RunID, j.ExpectedVersion+1),
		AggregateID:   observation.RunID,
		AggregateType: "run",
		Type:          "run." + string(observation.Kind),
		Version:       "1",
		Actor:         j.Actor,
		CommandID:     j.CommandID,
		CorrelationID: j.CorrelationID,
		CausationID:   j.CausationID,
		Trust:         trust,
		Payload:       payload,
		CreatedAt:     now,
	}
	appended, err := j.Store.Append(ctx, observation.RunID, j.ExpectedVersion, []eventstore.Event{event})
	if err != nil {
		return err
	}
	if len(appended) != 1 {
		return errors.New("event journal append returned unexpected event count")
	}
	j.ExpectedVersion = appended[0].AggregateVersion
	return nil
}
