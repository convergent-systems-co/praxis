package eventstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func fixtureEvent(id string) Event {
	return Event{ID: id, AggregateType: "run", Type: "run.transitioned", Version: "v1", Actor: contracts.PrincipalRef{ID: "runtime", Kind: "runtime"}, CommandID: "cmd-1", CorrelationID: "corr-1", CreatedAt: time.Now().UTC()}
}

func TestAppendAssignsMonotonicAggregateAndGlobalVersions(t *testing.T) {
	s := NewMemoryStore()
	appended, err := s.Append(context.Background(), "run-1", 0, []Event{fixtureEvent("e1"), fixtureEvent("e2")})
	if err != nil { t.Fatal(err) }
	if appended[0].Sequence != 1 || appended[1].Sequence != 2 || appended[0].AggregateVersion != 1 || appended[1].AggregateVersion != 2 {
		t.Fatalf("unexpected event numbering: %+v", appended)
	}
}

func TestAppendRejectsStaleExpectedVersion(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Append(context.Background(), "run-1", 0, []Event{fixtureEvent("e1")}); err != nil { t.Fatal(err) }
	if _, err := s.Append(context.Background(), "run-1", 0, []Event{fixtureEvent("e2")}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestReplayLoadsEventsAfterVersion(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Append(context.Background(), "run-1", 0, []Event{fixtureEvent("e1"), fixtureEvent("e2")}); err != nil { t.Fatal(err) }
	events, err := s.LoadAggregate(context.Background(), "run-1", 1)
	if err != nil { t.Fatal(err) }
	if len(events) != 1 || events[0].ID != "e2" { t.Fatalf("unexpected replay: %+v", events) }
}
