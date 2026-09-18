package eventstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var ErrVersionConflict = errors.New("event store aggregate version conflict")

type Event struct {
	Sequence         int64
	ID               string
	AggregateID      string
	AggregateType    string
	AggregateVersion int64
	Type             string
	Version          string
	Actor            contracts.PrincipalRef
	CommandID        string
	CorrelationID    string
	CausationID      string
	Trust            contracts.TrustClass
	Payload          []byte
	CreatedAt        time.Time
}

type Store interface {
	Append(ctx context.Context, aggregateID string, expectedVersion int64, events []Event) ([]Event, error)
	LoadAggregate(ctx context.Context, aggregateID string, afterVersion int64) ([]Event, error)
	ReadFrom(ctx context.Context, afterSequence int64, limit int) ([]Event, error)
}

type MemoryStore struct {
	mu       sync.RWMutex
	sequence int64
	versions map[string]int64
	events   []Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{versions: map[string]int64{}}
}

func (s *MemoryStore) Append(_ context.Context, aggregateID string, expectedVersion int64, proposed []Event) ([]Event, error) {
	if aggregateID == "" || len(proposed) == 0 {
		return nil, errors.New("aggregate id and events are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.versions[aggregateID] != expectedVersion {
		return nil, ErrVersionConflict
	}
	appended := make([]Event, len(proposed))
	for i, event := range proposed {
		if event.ID == "" || event.Type == "" || event.Version == "" || event.CommandID == "" || event.CorrelationID == "" {
			return nil, fmt.Errorf("event %d missing required identity/metadata", i)
		}
		if event.AggregateID != "" && event.AggregateID != aggregateID {
			return nil, errors.New("event aggregate id mismatch")
		}
		event.AggregateID = aggregateID
		event.AggregateVersion = expectedVersion + int64(i) + 1
		s.sequence++
		event.Sequence = s.sequence
		appended[i] = event
	}
	s.events = append(s.events, appended...)
	s.versions[aggregateID] = expectedVersion + int64(len(appended))
	return append([]Event(nil), appended...), nil
}

func (s *MemoryStore) LoadAggregate(_ context.Context, aggregateID string, afterVersion int64) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Event, 0)
	for _, event := range s.events {
		if event.AggregateID == aggregateID && event.AggregateVersion > afterVersion {
			out = append(out, event)
		}
	}
	return out, nil
}

func (s *MemoryStore) ReadFrom(_ context.Context, afterSequence int64, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, errors.New("positive read limit required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Event, 0, limit)
	for _, event := range s.events {
		if event.Sequence <= afterSequence {
			continue
		}
		out = append(out, event)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
