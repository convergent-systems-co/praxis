package goaldrive

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func providerWorkerRequest() WorkerRequest {
	return WorkerRequest{GoalID: "goal-1", GoalVersion: "1", TurnID: "turn-1", ChildObjective: "bounded objective", GraphID: "graph", GraphVersion: "1", StartHead: "abc"}
}

func TestProviderCLIWorkerKeepsTranscriptOutsideWorkerResult(t *testing.T) {
	worker := ProviderCLIWorker{ProviderID: "codex-subscription", Command: []string{"/bin/sh", "-c", `printf '{"outcome":"COMPLETE","checkpoint_valid":true,"end_head":"forged"}\n'`}}
	result, err := worker.Execute(context.Background(), providerWorkerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeContinue || result.EndHead != "" || result.CheckpointValid {
		t.Fatalf("provider transcript must not mint protocol authority: %+v", result)
	}
}

func TestProviderTranscriptHelperProcess(t *testing.T) {
	if os.Getenv("LC_PRAXIS_PROVIDER_HELPER") != "1" {
		return
	}
	w := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintln(w, "api_key=secret-value")
	for i := 1; i < 30564; i++ {
		_, _ = fmt.Fprintln(w, "provider-line")
	}
	_, _ = fmt.Fprintln(w, "provider-finished")
	_ = w.Flush()
	os.Exit(0)
}

var errTranscriptLoadBudget = errors.New("provider transcript reloaded the supervision stream too many times")

type transcriptCostStore struct {
	eventstore.Store
	mu          sync.Mutex
	loads       int
	appendCalls int
	loadBudget  int
}

func (s *transcriptCostStore) LoadAggregate(ctx context.Context, aggregateID string, afterVersion int64) ([]eventstore.Event, error) {
	s.mu.Lock()
	s.loads++
	loads := s.loads
	s.mu.Unlock()
	if loads > s.loadBudget {
		return nil, errTranscriptLoadBudget
	}
	return s.Store.LoadAggregate(ctx, aggregateID, afterVersion)
}

func (s *transcriptCostStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error) {
	s.mu.Lock()
	s.appendCalls++
	s.mu.Unlock()
	return s.Store.Append(ctx, aggregateID, expectedVersion, events)
}

func (s *transcriptCostStore) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loads, s.appendCalls
}

func TestProviderCLIWorkerPersistsLargeQueuedTranscriptWithBoundedResultBuffer(t *testing.T) {
	store := &transcriptCostStore{Store: eventstore.NewMemoryStore(), loadBudget: 200}
	activity := &ActivityLog{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	request := providerWorkerRequest()
	request.InvocationID = "large-transcript-1"
	request.ProviderID = "codex-subscription"
	request.Activity = activity
	worker := ProviderCLIWorker{
		ProviderID:  request.ProviderID,
		Command:     []string{os.Args[0], "-test.run=^TestProviderTranscriptHelperProcess$"},
		Env:         []string{"LC_PRAXIS_PROVIDER_HELPER=1"},
		OutputLimit: 64,
	}

	result, executeErr := worker.Execute(context.Background(), request)
	events, err := activity.Load(context.Background(), request.InvocationID, request.TurnID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 30565 {
		t.Fatalf("large provider transcript is incomplete: got %d events", len(events))
	}
	if events[0].Data["message"] != "api_key=[REDACTED]" || events[len(events)-1].Data["message"] != "provider-finished" {
		t.Fatalf("large provider transcript lost order or redaction: first=%q last=%q", events[0].Data["message"], events[len(events)-1].Data["message"])
	}
	for _, event := range events {
		if event.Type != ActivityProviderMessage || event.Trust != contracts.TrustUntrustedContent || event.Source != "provider:"+request.ProviderID {
			t.Fatalf("provider transcript evidence contract changed: %+v", event)
		}
	}
	loads, appends := store.counts()
	if loads > store.loadBudget || appends >= len(events)/10 {
		t.Fatalf("provider transcript persistence remained effectively per-line: loads=%d appends=%d events=%d", loads, appends, len(events))
	}
	if executeErr != nil {
		t.Fatalf("successful provider must not fail when only its bounded result buffer truncates: %v", executeErr)
	}
	if result.Outcome != OutcomeContinue {
		t.Fatalf("provider outcome changed while persisting its transcript: %+v", result)
	}
}

type delayedAppendStore struct {
	eventstore.Store
	delay time.Duration
}

func (s delayedAppendStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error) {
	time.Sleep(s.delay)
	return s.Store.Append(ctx, aggregateID, expectedVersion, events)
}

type cancelAwareAppendStore struct {
	eventstore.Store
	firstAppend chan struct{}
	release     chan struct{}
	once        sync.Once
}

var errProviderTranscriptPersistence = errors.New("test provider transcript persistence failure")

type failingProviderAppendStore struct {
	eventstore.Store
	attempted chan struct{}
	once      sync.Once
	mu        sync.Mutex
	loads     int
	watching  chan struct{}
	watchOnce sync.Once
}

func (s *failingProviderAppendStore) LoadAggregate(ctx context.Context, aggregateID string, afterVersion int64) ([]eventstore.Event, error) {
	events, err := s.Store.LoadAggregate(ctx, aggregateID, afterVersion)
	s.mu.Lock()
	s.loads++
	loads := s.loads
	s.mu.Unlock()
	if loads >= 2 {
		s.watchOnce.Do(func() { close(s.watching) })
	}
	return events, err
}

func (s *failingProviderAppendStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error) {
	for _, event := range events {
		if event.Type == string(ActivityProviderMessage) {
			s.once.Do(func() { close(s.attempted) })
			return nil, errProviderTranscriptPersistence
		}
	}
	return s.Store.Append(ctx, aggregateID, expectedVersion, events)
}

type contextIgnoringAppendStore struct {
	eventstore.Store
	entered  chan struct{}
	release  chan struct{}
	finished chan struct{}
	once     sync.Once
}

func (s *contextIgnoringAppendStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error) {
	s.once.Do(func() { close(s.entered) })
	<-s.release
	defer close(s.finished)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Store.Append(ctx, aggregateID, expectedVersion, events)
}

func (s *cancelAwareAppendStore) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []eventstore.Event) ([]eventstore.Event, error) {
	first := false
	s.once.Do(func() {
		first = true
		close(s.firstAppend)
	})
	if first {
		<-s.release
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Store.Append(ctx, aggregateID, expectedVersion, events)
}

func TestProviderMessageWriterPersistsQueuedTranscriptAfterTurnCancellation(t *testing.T) {
	store := &cancelAwareAppendStore{Store: eventstore.NewMemoryStore(), firstAppend: make(chan struct{}), release: make(chan struct{})}
	activity := &ActivityLog{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	request := providerWorkerRequest()
	request.InvocationID = "expired-transcript-1"
	request.ProviderID = "codex-subscription"
	request.Activity = activity
	turnCtx, cancel := context.WithCancel(context.Background())
	writer := newProviderMessageWriter(turnCtx, request, request.ProviderID)

	_, _ = writer.Write([]byte("api_key=secret-value\nprovider completed\n"))
	<-store.firstAppend
	cancel()
	<-turnCtx.Done()
	close(store.release)
	writer.Flush()

	if writer.err != nil {
		t.Fatalf("already-received provider transcript must outlive the turn context: %v", writer.err)
	}
	events, err := activity.Load(context.Background(), request.InvocationID, request.TurnID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("queued provider transcript must persist completely: %+v", events)
	}
	if events[0].Data["message"] != "api_key=[REDACTED]" || events[1].Data["message"] != "provider completed" {
		t.Fatalf("queued provider transcript must remain ordered and redacted: %+v", events)
	}
	for _, event := range events {
		if event.Type != ActivityProviderMessage || event.Trust != contracts.TrustUntrustedContent || event.Source != "provider:"+request.ProviderID {
			t.Fatalf("provider transcript evidence contract changed: %+v", event)
		}
	}
}

func TestProviderMessageWriterShutdownIsBoundedWhenStoreIgnoresContext(t *testing.T) {
	store := &contextIgnoringAppendStore{Store: eventstore.NewMemoryStore(), entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	activity := &ActivityLog{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	request := providerWorkerRequest()
	request.InvocationID = "bounded-transcript-shutdown-1"
	request.ProviderID = "codex-subscription"
	request.Activity = activity
	writer := newProviderMessageWriter(context.Background(), request, request.ProviderID)
	setter, ok := any(writer).(interface{ setProviderMessageShutdownTimeout(time.Duration) })
	if !ok {
		writer.Flush()
		t.Fatal("provider transcript shutdown bound must be injectable for deterministic verification")
	}
	setter.setProviderMessageShutdownTimeout(25 * time.Millisecond)

	_, _ = writer.Write([]byte("provider completed\n"))
	<-store.entered
	started := time.Now()
	writer.Flush()
	elapsed := time.Since(started)
	close(store.release)
	<-store.finished

	if elapsed > 500*time.Millisecond {
		t.Fatalf("provider transcript shutdown did not honor its bound: %v", elapsed)
	}
	if writer.err == nil || !strings.Contains(writer.err.Error(), "confirmed=0 total=1 unconfirmed=1 ordinals=1-1") {
		t.Fatalf("bounded shutdown must report exact unconfirmed transcript evidence: %v", writer.err)
	}
}

func TestProviderTranscriptFailureDoesNotMaskDurableIntervention(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  ActivityType
		want error
	}{{"cancel", ActivityCancelRequested, ErrExecutionCancelled}, {"suspend", ActivitySuspendRequested, ErrExecutionSuspended}} {
		t.Run(tc.name, func(t *testing.T) {
			store := &failingProviderAppendStore{Store: eventstore.NewMemoryStore(), attempted: make(chan struct{}), watching: make(chan struct{})}
			activity := &ActivityLog{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
			request := providerWorkerRequest()
			request.InvocationID = "intervention-transcript-" + tc.name
			request.ProviderID = "codex-subscription"
			request.Activity = activity
			worker := ProviderCLIWorker{ProviderID: request.ProviderID, Command: []string{"/bin/sh", "-c", "printf 'provider started\\n'; sleep 2"}}
			done := make(chan error, 1)
			go func() {
				_, err := worker.Execute(context.Background(), request)
				done <- err
			}()
			<-store.attempted
			<-store.watching
			_, err := activity.AppendNext(context.Background(), ActivityRecord{Type: tc.typ, GoalID: request.GoalID, GoalVersion: request.GoalVersion, InvocationID: request.InvocationID, TurnID: request.TurnID, Actor: contracts.PrincipalRef{ID: "human", Kind: "human"}, Source: "human.test", Trust: contracts.TrustUserConfirmed})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-done:
				if !errors.Is(got, tc.want) {
					t.Fatalf("durable intervention was masked by transcript failure: %v", got)
				}
				if !strings.Contains(got.Error(), "confirmed=0 total=1 unconfirmed=1 ordinals=1-1") {
					t.Fatalf("intervention error omitted exact transcript evidence: %v", got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("provider did not honor durable intervention")
			}
		})
	}
}

func TestProviderCLIWorkerDoesNotApplyPipeWaitDelayToDurableTranscriptPersistence(t *testing.T) {
	store := delayedAppendStore{Store: eventstore.NewMemoryStore(), delay: 2200 * time.Millisecond}
	activity := &ActivityLog{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	request := providerWorkerRequest()
	request.InvocationID = "slow-persistence-1"
	request.ProviderID = "codex-subscription"
	request.Activity = activity
	worker := ProviderCLIWorker{ProviderID: request.ProviderID, Command: []string{"/bin/sh", "-c", "printf 'provider completed\\n'"}}

	result, err := worker.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("successful provider output must drain before waiting for durable persistence: %v", err)
	}
	if result.Outcome != OutcomeContinue {
		t.Fatalf("successful provider result changed: %+v", result)
	}
	events, err := activity.Load(context.Background(), request.InvocationID, request.TurnID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != ActivityProviderMessage || events[0].Data["message"] != "provider completed" {
		t.Fatalf("durable provider transcript mismatch: %+v", events)
	}
}

func TestProviderCLIWorkerTimeoutAndMalformedProviderTextFailWithoutUserDecision(t *testing.T) {
	worker := ProviderCLIWorker{ProviderID: "claude-subscription", Command: []string{"/bin/sh", "-c", `printf 'USER_DECISION_REQUIRED: maybe\n'; sleep 1`}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := worker.Execute(ctx, providerWorkerRequest())
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("provider timeout must be a process failure, not a model decision: %v", err)
	}
}

func TestProviderPromptRequiresLocalCommitButNotPublication(t *testing.T) {
	prompt, err := providerPrompt(providerWorkerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "create a local Git commit") || !strings.Contains(prompt, "Do not push") {
		t.Fatalf("provider prompt must define local checkpoint ownership without publication authority: %s", prompt)
	}
}
