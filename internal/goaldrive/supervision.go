package goaldrive

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// ActivityType is the closed v1 supervision vocabulary. Provider and human
// input are observations/control records; they never become controller facts
// merely because they appear in this stream.
type ActivityType string

const (
	ActivityExecutionStarted      ActivityType = "execution.started"
	ActivityExecutionStateChanged ActivityType = "execution.state_changed"
	ActivityWorkSelected          ActivityType = "work.selected"
	ActivityWorkProgress          ActivityType = "work.progress"
	ActivityProviderMessage       ActivityType = "provider.message"
	ActivityActionStarted         ActivityType = "action.started"
	ActivityActionCompleted       ActivityType = "action.completed"
	ActivityActionFailed          ActivityType = "action.failed"
	ActivityValidationStarted     ActivityType = "validation.started"
	ActivityValidationCompleted   ActivityType = "validation.completed"
	ActivityCheckpointCreated     ActivityType = "checkpoint.created"
	ActivityBlockerDetected       ActivityType = "blocker.detected"
	ActivityCapabilityUnsatisfied ActivityType = "capability.unsatisfiable"
	ActivityRecoveryBound         ActivityType = "workspace.recovery_bound"
	// ActivityTurnAllocated is the first durable activity of every turn: it
	// is recorded before the turn identity is announced, so an observer that
	// executes the announced command always finds the turn.
	ActivityTurnAllocated ActivityType = "turn.allocated"
	// ActivityUnitCompleted records that the controller accepted a unit
	// completion proposal and persisted the unit's completion (#158).
	ActivityUnitCompleted ActivityType = "unit.completed"
	// ActivityExecutionInterrupted: the holder recorded a graceful interruption
	// (signal or timeout) before exiting. ActivityExecutionLost: the explicit
	// reconciliation path closed an execution whose process disappeared;
	// its consequence is unknown.
	ActivityExecutionInterrupted ActivityType = "execution.interrupted"
	ActivityExecutionLost        ActivityType = "execution.lost"
	ActivityAuthorityRequired    ActivityType = "authority.required"
	ActivityAuthorityResolved    ActivityType = "authority.resolved"
	ActivityHumanComment         ActivityType = "human.comment"
	ActivityHumanCorrection      ActivityType = "human.correction"
	ActivityHumanConstraint      ActivityType = "human.constraint"
	ActivitySuspendRequested     ActivityType = "execution.suspend_requested"
	ActivitySuspended            ActivityType = "execution.suspended"
	ActivityCancelRequested      ActivityType = "execution.cancel_requested"
	ActivityCancelled            ActivityType = "execution.cancelled"
	ActivityResumed              ActivityType = "execution.resumed"
	// ActivityExecutionEnvelope records, before the provider starts, the
	// non-interactive execution envelope the turn runs under (#170).
	ActivityExecutionEnvelope   ActivityType = "execution.envelope"
	ActivityCompletionClaimed   ActivityType = "completion.claimed"
	ActivityCompletionQualified ActivityType = "completion.qualified"
)

var activityTypes = map[ActivityType]struct{}{
	ActivityExecutionStarted: {}, ActivityExecutionStateChanged: {}, ActivityWorkSelected: {}, ActivityWorkProgress: {}, ActivityProviderMessage: {},
	ActivityActionStarted: {}, ActivityActionCompleted: {}, ActivityActionFailed: {}, ActivityValidationStarted: {}, ActivityValidationCompleted: {},
	ActivityCheckpointCreated: {}, ActivityBlockerDetected: {}, ActivityAuthorityRequired: {}, ActivityAuthorityResolved: {}, ActivityHumanComment: {},
	ActivityHumanCorrection: {}, ActivityHumanConstraint: {}, ActivitySuspendRequested: {}, ActivitySuspended: {}, ActivityCancelRequested: {},
	ActivityCancelled: {}, ActivityResumed: {}, ActivityCompletionClaimed: {}, ActivityCompletionQualified: {},
	ActivityCapabilityUnsatisfied: {}, ActivityRecoveryBound: {}, ActivityTurnAllocated: {}, ActivityUnitCompleted: {}, ActivityExecutionInterrupted: {}, ActivityExecutionLost: {}, ActivityExecutionEnvelope: {},
}

const supervisionAggregateType = "goal_drive_supervision"
const supervisionEventVersion = "1"

var (
	ErrExecutionSuspended = errors.New("Goal-drive execution suspended at a safe boundary")
	ErrExecutionCancelled = errors.New("Goal-drive execution cancelled")
)

// ActivityRecord is the durable, typed projection input for supervision. Data
// is intentionally a small string map rather than arbitrary model JSON.
type ActivityRecord struct {
	ID            string                 `json:"id"`
	Type          ActivityType           `json:"type"`
	GoalID        string                 `json:"goal_id"`
	GoalVersion   string                 `json:"goal_version"`
	InvocationID  string                 `json:"invocation_id"`
	TurnID        string                 `json:"turn_id"`
	ProviderID    string                 `json:"provider_id,omitempty"`
	Actor         contracts.PrincipalRef `json:"actor"`
	Source        string                 `json:"source"`
	Trust         contracts.TrustClass   `json:"trust"`
	Data          map[string]string      `json:"data,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	CausationID   string                 `json:"causation_id,omitempty"`
	Sensitivity   string                 `json:"sensitivity,omitempty"`
	StreamVersion int64                  `json:"stream_version,omitempty"`
}

func activityAggregate(invocationID, turnID string) string {
	return "goal-drive-supervision:" + invocationID + ":" + turnID
}

// ActivityLog stores supervision events in the canonical Praxis event store.
type ActivityLog struct {
	Store eventstore.Store
	Actor contracts.PrincipalRef
}

func (l ActivityLog) Append(ctx context.Context, expectedVersion int64, record ActivityRecord) (ActivityRecord, error) {
	if l.Store == nil {
		return ActivityRecord{}, errors.New("supervision event store is required")
	}
	if err := validateActivity(record); err != nil {
		return ActivityRecord{}, err
	}
	if err := l.Actor.Validate(); err != nil {
		return ActivityRecord{}, fmt.Errorf("supervision actor: %w", err)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.ID == "" {
		record.ID = activityID()
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return ActivityRecord{}, fmt.Errorf("encode supervision activity: %w", err)
	}
	aggregateID := activityAggregate(record.InvocationID, record.TurnID)
	eventActor := record.Actor
	appended, err := l.Store.Append(ctx, aggregateID, expectedVersion, []eventstore.Event{{
		ID: aggregateID + ":" + record.ID, AggregateType: supervisionAggregateType, Type: string(record.Type), Version: supervisionEventVersion,
		Actor: eventActor, CommandID: "supervision:" + record.ID, CorrelationID: aggregateID, CausationID: record.CausationID,
		Trust: record.Trust, Payload: payload, CreatedAt: record.CreatedAt,
	}})
	if err != nil {
		return ActivityRecord{}, err
	}
	record.StreamVersion = appended[0].AggregateVersion
	return record, nil
}

// AppendNext is safe for concurrent controller/CLI writers and retries only
// optimistic version conflicts. It never repeats a successful mutation.
func (l ActivityLog) AppendNext(ctx context.Context, record ActivityRecord) (ActivityRecord, error) {
	for attempt := 0; attempt < 5; attempt++ {
		current, err := l.Load(ctx, record.InvocationID, record.TurnID, 0)
		if err != nil {
			return ActivityRecord{}, err
		}
		appended, err := l.Append(ctx, int64(len(current)), record)
		if !errors.Is(err, eventstore.ErrVersionConflict) {
			return appended, err
		}
	}
	return ActivityRecord{}, eventstore.ErrVersionConflict
}

func (l ActivityLog) Load(ctx context.Context, invocationID, turnID string, afterVersion int64) ([]ActivityRecord, error) {
	if l.Store == nil || invocationID == "" || turnID == "" {
		return nil, errors.New("supervision store and exact execution identity are required")
	}
	events, err := l.Store.LoadAggregate(ctx, activityAggregate(invocationID, turnID), afterVersion)
	if err != nil {
		return nil, err
	}
	result := make([]ActivityRecord, 0, len(events))
	for _, event := range events {
		if _, ok := activityTypes[ActivityType(event.Type)]; !ok || event.Version != supervisionEventVersion || event.Trust == "" {
			return nil, errors.New("unsupported or untrusted supervision event")
		}
		var record ActivityRecord
		if err := json.Unmarshal(event.Payload, &record); err != nil {
			return nil, fmt.Errorf("decode supervision event: %w", err)
		}
		record.StreamVersion = event.AggregateVersion
		if record.Type != ActivityType(event.Type) || record.InvocationID != invocationID || record.TurnID != turnID {
			return nil, errors.New("supervision event payload identity or type mismatch")
		}
		if err := validateActivity(record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

// ListTurns discovers the turn identities that have durable supervision
// activity under one invocation, oldest first. An operator who started an
// invocation holds its identity; this is how they learn the turns it
// allocated without being told each child identity in advance.
func (l ActivityLog) ListTurns(ctx context.Context, invocationID string) ([]string, error) {
	if l.Store == nil || invocationID == "" {
		return nil, errors.New("supervision store and exact invocation identity are required")
	}
	lister, ok := l.Store.(eventstore.AggregateLister)
	if !ok {
		return nil, errors.New("supervision store cannot enumerate invocation turns")
	}
	prefix := activityAggregate(invocationID, "")
	aggregates, err := lister.ListAggregates(ctx, supervisionAggregateType, prefix)
	if err != nil {
		return nil, err
	}
	turns := make([]string, 0, len(aggregates))
	for _, aggregate := range aggregates {
		turns = append(turns, strings.TrimPrefix(aggregate, prefix))
	}
	return turns, nil
}

func (l ActivityLog) Emit(ctx context.Context, typ ActivityType, request WorkerRequest, actor contracts.PrincipalRef, trust contracts.TrustClass, source string, data map[string]string) (ActivityRecord, error) {
	return l.AppendNext(ctx, ActivityRecord{Type: typ, GoalID: request.GoalID, GoalVersion: request.GoalVersion, InvocationID: request.InvocationID, TurnID: request.TurnID, ProviderID: request.ProviderID, Actor: actor, Source: source, Trust: trust, Data: sanitizeActivityData(data)})
}

func validateActivity(record ActivityRecord) error {
	if _, ok := activityTypes[record.Type]; !ok {
		return fmt.Errorf("unsupported supervision activity type %q", record.Type)
	}
	if record.GoalID == "" || record.GoalVersion == "" || record.InvocationID == "" || record.TurnID == "" || record.Source == "" {
		return errors.New("supervision activity requires exact Goal, invocation, turn, and source identity")
	}
	if err := record.Actor.Validate(); err != nil {
		return fmt.Errorf("supervision activity actor: %w", err)
	}
	if record.Trust == "" {
		return errors.New("supervision activity trust is required")
	}
	if record.Type == ActivityProviderMessage && record.Trust != contracts.TrustUntrustedContent {
		return errors.New("provider messages must remain untrusted content")
	}
	if record.Type == ActivityProviderMessage && !strings.HasPrefix(record.Source, "provider:") {
		return errors.New("provider messages require provider provenance")
	}
	if strings.HasPrefix(string(record.Type), "human.") && record.Trust != contracts.TrustUserConfirmed {
		return errors.New("human interventions require user-confirmed trust")
	}
	if strings.HasPrefix(string(record.Type), "human.") && !strings.HasPrefix(record.Source, "human.") {
		return errors.New("human interventions require human provenance")
	}
	if record.Type == ActivitySuspendRequested || record.Type == ActivityCancelRequested {
		if record.Trust != contracts.TrustUserConfirmed || !strings.HasPrefix(record.Source, "human.") {
			return errors.New("execution control requests require human provenance")
		}
		return nil
	}
	if strings.HasPrefix(string(record.Type), "execution.") || strings.HasPrefix(string(record.Type), "work.") || strings.HasPrefix(string(record.Type), "action.") || strings.HasPrefix(string(record.Type), "validation.") || strings.HasPrefix(string(record.Type), "checkpoint.") || strings.HasPrefix(string(record.Type), "blocker.") || strings.HasPrefix(string(record.Type), "authority.") || strings.HasPrefix(string(record.Type), "completion.") {
		if record.Trust != contracts.TrustObserved || record.Source != "praxis.controller" {
			return errors.New("controller supervision facts require controller provenance")
		}
	}
	return nil
}

func activityID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("activity-%d", time.Now().UnixNano())
	}
	return "activity-" + hex.EncodeToString(b[:])
}

var sensitiveActivityPattern = regexp.MustCompile(`(?i)(authorization[ \t]*:[ \t]*bearer[ \t]+|(?:api[_-]?key|token|secret|password|credential)[ \t]*[=:][ \t]*)[^\s,;]+`)

func sanitizeActivityText(value string) string {
	value = sensitiveActivityPattern.ReplaceAllString(value, "$1[REDACTED]")
	if len(value) > 16*1024 {
		value = value[:16*1024] + "…[TRUNCATED]"
	}
	return value
}

func sanitizeActivityData(data map[string]string) map[string]string {
	if len(data) == 0 {
		return nil
	}
	result := make(map[string]string, len(data))
	for key, value := range data {
		result[key] = sanitizeActivityText(value)
	}
	return result
}

func interventionType(typ ActivityType) bool {
	return typ == ActivityHumanComment || typ == ActivityHumanCorrection || typ == ActivityHumanConstraint || typ == ActivitySuspendRequested || typ == ActivityCancelRequested || typ == ActivityResumed
}
