package goaldrive

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/scheduler"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Turn admission (#162) and lost-execution reconciliation (#163).
//
// Admission answers "may this execution begin?": a durable, atomic
// transition on the Goal generation's admission aggregate (event-store
// compare-and-set, correct across OS processes) that allocates a unique
// turn number, refuses a reused invocation identity, and refuses a second
// execution over the same consequence scope while another holds its lease.
//
// The consequence scope is the checkout: canonical repository path and
// branch. Two Goals, or two units of one Goal, in different worktrees are
// independent scopes and are not serialized (ADR-070 leaves that room);
// the same worktree is one scope because its tree and HEAD are shared.
//
// Liveness is a renewable scheduler resource lease (heartbeat). A holder
// that cannot renew has lost its lease and must stop without publishing
// or recording. An unreleased admission whose lease has expired is a LOST
// execution: nothing infers what happened; the explicit reconciliation
// path records the observed consequence as UNKNOWN and makes it
// recoverable.

var (
	ErrScopeLeased          = errors.New("execution scope is already leased by an active turn")
	ErrInvocationReused     = errors.New("invocation identity is single-use")
	ErrLostTurnUnreconciled = errors.New("a lost execution on this scope awaits reconciliation")
	ErrLeaseLost            = errors.New("turn lease lost; this process no longer holds execution authority")
	ErrTurnStillLive        = errors.New("turn lease is still live; nothing to reconcile")
	ErrNotAdmitted          = errors.New("turn was not admitted through the admission aggregate")
)

// LeaseStore is the durable liveness primitive (scheduler resource leases).
type LeaseStore interface {
	DefineSchedulerResource(ctx context.Context, state scheduler.ResourceState) error
	AcquireSchedulerResourceLeases(ctx context.Context, sliceID, attemptID string, requirements []scheduler.ResourceRequirement, now time.Time, expiry *time.Time) ([]scheduler.ResourceLease, error)
	ExtendSchedulerResourceLease(ctx context.Context, leaseID string, expiry, now time.Time) error
	LookupSchedulerResourceLease(ctx context.Context, leaseID string) (scheduler.ResourceLease, bool, error)
	ReleaseSchedulerResourceLeases(ctx context.Context, leaseIDs []string, now time.Time) error
}

// TurnAdmission is the durable record that a turn was admitted.
type TurnAdmission struct {
	GoalID       string        `json:"goal_id"`
	GoalVersion  string        `json:"goal_version"`
	TurnNumber   int           `json:"turn_number"`
	TurnID       string        `json:"turn_id"`
	InvocationID string        `json:"invocation_id"`
	Mode         ExecutionMode `json:"mode"`
	Scope        string        `json:"scope"`
	LeaseID      string        `json:"lease_id,omitempty"`
	Token        string        `json:"token"`
	Host         string        `json:"host,omitempty"`
	PID          int           `json:"pid,omitempty"`
	AdmittedHead string        `json:"admitted_head,omitempty"`
	AdmittedAt   time.Time     `json:"admitted_at"`
	TTL          time.Duration `json:"ttl"`
}

// TurnRelease is the durable end of an admission: the disposition the
// holder recorded, or "reconciled" when the reconciliation path closed a
// lost execution.
type TurnRelease struct {
	TurnID      string    `json:"turn_id"`
	Disposition string    `json:"disposition"`
	ReleasedAt  time.Time `json:"released_at"`
	By          string    `json:"by"`
}

const (
	turnAdmittedEventType = "goal_drive.turn_admitted"
	turnReleasedEventType = "goal_drive.turn_released"
	DefaultLeaseTTL       = 60 * time.Second
)

// AdmissionState is the reconstructed admission history of a generation.
type AdmissionState struct {
	Admissions []TurnAdmission
	Releases   map[string]TurnRelease
	events     int64
}

func admissionAggregate(goalID, version string) string {
	return "goal-drive-admission:" + goalID + ":" + version
}

// ScopeKey names the consequence scope of a checkout.
func ScopeKey(repositoryPath, branch string) string {
	abs, err := filepath.Abs(repositoryPath)
	if err != nil {
		abs = repositoryPath
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return abs + "|" + branch
}

func scopeRepository(scope string) (string, string) {
	if at := strings.LastIndex(scope, "|"); at >= 0 {
		return scope[:at], scope[at+1:]
	}
	return scope, ""
}

// LoadAdmissions reconstructs the admission aggregate.
func (l Ledger) LoadAdmissions(ctx context.Context, goalID, version string) (AdmissionState, error) {
	if l.Store == nil {
		return AdmissionState{}, errors.New("Goal drive event store is required")
	}
	events, err := l.Store.LoadAggregate(ctx, admissionAggregate(goalID, version), 0)
	if err != nil {
		return AdmissionState{}, err
	}
	state := AdmissionState{Releases: map[string]TurnRelease{}, events: int64(len(events))}
	for _, event := range events {
		switch event.Type {
		case turnAdmittedEventType:
			var admission TurnAdmission
			if err := json.Unmarshal(event.Payload, &admission); err != nil {
				return AdmissionState{}, fmt.Errorf("decode turn admission: %w", err)
			}
			state.Admissions = append(state.Admissions, admission)
		case turnReleasedEventType:
			var release TurnRelease
			if err := json.Unmarshal(event.Payload, &release); err != nil {
				return AdmissionState{}, fmt.Errorf("decode turn release: %w", err)
			}
			state.Releases[release.TurnID] = release
		default:
			return AdmissionState{}, fmt.Errorf("unexpected admission event %q", event.Type)
		}
	}
	return state, nil
}

func (l Ledger) appendAdmission(ctx context.Context, goalID, version string, expected int64, eventType, commandID string, payload any, at time.Time) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	aggregate := admissionAggregate(goalID, version)
	commandID = aggregate + ":" + commandID
	_, err = l.Store.Append(ctx, aggregate, expected, []eventstore.Event{{
		ID: commandID, AggregateType: "goal_drive_admission", Type: eventType, Version: "1",
		Actor: l.Actor, CommandID: commandID, CorrelationID: aggregate, Trust: contracts.TrustObserved,
		Payload: encoded, CreatedAt: at,
	}})
	return err
}

// Liveness reports whether an unreleased admission still holds its lease.
// Without a lease store there is no liveness tracking, and an unreleased
// admission is treated as live (single-process use only).
func Liveness(ctx context.Context, leases LeaseStore, admission TurnAdmission, now time.Time) (live bool, expiresAt time.Time, err error) {
	if leases == nil || admission.LeaseID == "" {
		return true, time.Time{}, nil
	}
	lease, ok, err := leases.LookupSchedulerResourceLease(ctx, admission.LeaseID)
	if err != nil {
		return false, time.Time{}, err
	}
	if !ok || lease.ReleasedAt != nil {
		return false, time.Time{}, nil
	}
	if lease.ExpiresAt != nil && !lease.ExpiresAt.After(now) {
		return false, *lease.ExpiresAt, nil
	}
	if lease.ExpiresAt != nil {
		expiresAt = *lease.ExpiresAt
	}
	return true, expiresAt, nil
}

// TurnLease is the in-process handle of an admitted turn.
type TurnLease struct {
	Admission TurnAdmission
	ledger    Ledger
	leases    LeaseStore
	lost      atomic.Bool
	stop      chan struct{}
	stopOnce  sync.Once
	done      chan struct{}
}

// Held reports whether this process still holds execution authority for the turn.
func (t *TurnLease) Held(ctx context.Context) bool {
	if t == nil {
		return true
	}
	if t.lost.Load() {
		return false
	}
	live, _, err := Liveness(ctx, t.leases, t.Admission, time.Now().UTC())
	if err != nil || !live {
		t.lost.Store(true)
		return false
	}
	return true
}

// heartbeat renews the lease until stopped; on the first failed renewal the
// lease is lost and onLost cancels the turn.
func (t *TurnLease) heartbeat(onLost func()) {
	defer close(t.done)
	if t.leases == nil || t.Admission.LeaseID == "" {
		<-t.stop
		return
	}
	interval := t.Admission.TTL / 3
	if interval < 200*time.Millisecond {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			now := time.Now().UTC()
			if err := t.leases.ExtendSchedulerResourceLease(context.Background(), t.Admission.LeaseID, now.Add(t.Admission.TTL), now); err != nil {
				t.lost.Store(true)
				onLost()
				return
			}
		}
	}
}

func (t *TurnLease) stopHeartbeat() {
	t.stopOnce.Do(func() { close(t.stop) })
	<-t.done
}

// Release records the holder's disposition and releases the lease. A lost
// lease is never released by the loser: reconciliation owns that.
func (t *TurnLease) Release(ctx context.Context, disposition string) error {
	t.stopHeartbeat()
	if t.lost.Load() {
		return ErrLeaseLost
	}
	now := time.Now().UTC()
	state, err := t.ledger.LoadAdmissions(ctx, t.Admission.GoalID, t.Admission.GoalVersion)
	if err != nil {
		return err
	}
	if _, released := state.Releases[t.Admission.TurnID]; released {
		return nil
	}
	if err := t.ledger.appendAdmission(ctx, t.Admission.GoalID, t.Admission.GoalVersion, state.events, turnReleasedEventType, "released:"+t.Admission.TurnID, TurnRelease{TurnID: t.Admission.TurnID, Disposition: disposition, ReleasedAt: now, By: "holder"}, now); err != nil {
		return err
	}
	if t.leases != nil && t.Admission.LeaseID != "" {
		return t.leases.ReleaseSchedulerResourceLeases(ctx, []string{t.Admission.LeaseID}, now)
	}
	return nil
}

func randomToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf[:])
}

// Admit performs the atomic admission transition. continuing names the
// invocation identities this process already admitted (continuous mode
// runs several turns under one invocation in one process).
func Admit(ctx context.Context, ledger Ledger, leases LeaseStore, goalID, version, invocationID string, mode ExecutionMode, scope, admittedHead string, ttl time.Duration, continuing map[string]bool) (*TurnLease, error) {
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	host, _ := os.Hostname()
	for attempt := 0; attempt < 8; attempt++ {
		now := time.Now().UTC()
		state, err := ledger.LoadAdmissions(ctx, goalID, version)
		if err != nil {
			return nil, err
		}
		turns, err := ledger.Load(ctx, goalID, version)
		if err != nil {
			return nil, err
		}
		next := len(turns) + 1
		for _, admission := range state.Admissions {
			if admission.TurnNumber >= next {
				next = admission.TurnNumber + 1
			}
			if admission.InvocationID == invocationID && !continuing[invocationID] {
				return nil, fmt.Errorf("%w: %q already admitted turn %s", ErrInvocationReused, invocationID, admission.TurnID)
			}
			if _, released := state.Releases[admission.TurnID]; released || admission.Scope != scope {
				continue
			}
			live, expiresAt, err := Liveness(ctx, leases, admission, now)
			if err != nil {
				return nil, err
			}
			if live {
				return nil, fmt.Errorf("%w: turn %s (invocation %s, pid %d on %s) holds the lease on %s until %s", ErrScopeLeased, admission.TurnID, admission.InvocationID, admission.PID, admission.Host, scope, expiresAt.UTC().Format(time.RFC3339))
			}
			return nil, fmt.Errorf("%w: turn %s (invocation %s) lost its lease at %s; reconcile it first: praxis supervise reconcile --goal-id=%s --goal-version=%s --invocation-id=%s --turn-id=%s", ErrLostTurnUnreconciled, admission.TurnID, admission.InvocationID, expiresAt.UTC().Format(time.RFC3339), goalID, version, admission.InvocationID, admission.TurnID)
		}
		turnID := invocationID + ":turn:" + strconv.Itoa(next)
		admission := TurnAdmission{GoalID: goalID, GoalVersion: version, TurnNumber: next, TurnID: turnID, InvocationID: invocationID, Mode: mode, Scope: scope, Token: randomToken(), Host: host, PID: os.Getpid(), AdmittedHead: admittedHead, AdmittedAt: now, TTL: ttl}
		if leases != nil {
			resource := "goal-drive-scope:" + scope
			if err := leases.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: resource, Capacity: 1, Exclusive: true}); err != nil {
				return nil, fmt.Errorf("define scope resource: %w", err)
			}
			expiry := now.Add(ttl)
			acquired, err := leases.AcquireSchedulerResourceLeases(ctx, goalID+"/"+version, turnID, []scheduler.ResourceRequirement{{Key: resource, Capacity: 1, Exclusive: true}}, now, &expiry)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrScopeLeased, err)
			}
			admission.LeaseID = acquired[0].ID
		}
		err = ledger.appendAdmission(ctx, goalID, version, state.events, turnAdmittedEventType, "admitted:"+turnID, admission, now)
		if err == nil {
			lease := &TurnLease{Admission: admission, ledger: ledger, leases: leases, stop: make(chan struct{}), done: make(chan struct{})}
			return lease, nil
		}
		if leases != nil && admission.LeaseID != "" {
			_ = leases.ReleaseSchedulerResourceLeases(ctx, []string{admission.LeaseID}, time.Now().UTC())
		}
		if !errors.Is(err, eventstore.ErrVersionConflict) {
			return nil, err
		}
	}
	return nil, errors.New("turn admission contended beyond retry limit")
}

// LostTurns lists unreleased admissions whose lease is no longer live.
func LostTurns(ctx context.Context, ledger Ledger, leases LeaseStore, goalID, version string, now time.Time) ([]TurnAdmission, []TurnAdmission, error) {
	state, err := ledger.LoadAdmissions(ctx, goalID, version)
	if err != nil {
		return nil, nil, err
	}
	var lost, active []TurnAdmission
	for _, admission := range state.Admissions {
		if _, released := state.Releases[admission.TurnID]; released {
			continue
		}
		live, _, err := Liveness(ctx, leases, admission, now)
		if err != nil {
			return nil, nil, err
		}
		if live {
			active = append(active, admission)
		} else {
			lost = append(lost, admission)
		}
	}
	return lost, active, nil
}

// ReconcileLostTurn closes a lost execution explicitly: it requires an
// unreleased admission whose lease is no longer live, observes the scope's
// checkout (HEAD, uncommitted paths, unpublished commits), records a
// BLOCKED turn with UNKNOWN provider consequence and the fingerprint bound
// for recovery, emits the terminal activities on the turn's stream, and
// releases the admission with disposition "reconciled". Nothing is retried
// and no effect is inferred.
func (c Controller) ReconcileLostTurn(ctx context.Context, leases LeaseStore, goalID, version, turnID string, remote string, by contracts.PrincipalRef) (TurnRecord, error) {
	now := time.Now().UTC()
	state, err := c.Ledger.LoadAdmissions(ctx, goalID, version)
	if err != nil {
		return TurnRecord{}, err
	}
	var admission *TurnAdmission
	for i := range state.Admissions {
		if state.Admissions[i].TurnID == turnID {
			admission = &state.Admissions[i]
		}
	}
	if admission == nil {
		return TurnRecord{}, fmt.Errorf("%w: %s (turns that predate admission are historical evidence and need an explicit migration)", ErrNotAdmitted, turnID)
	}
	if release, released := state.Releases[turnID]; released {
		return TurnRecord{}, fmt.Errorf("turn %s was already released (%s at %s)", turnID, release.Disposition, release.ReleasedAt.UTC().Format(time.RFC3339))
	}
	live, expiresAt, err := Liveness(ctx, leases, *admission, now)
	if err != nil {
		return TurnRecord{}, err
	}
	if live {
		return TurnRecord{}, fmt.Errorf("%w: turn %s lease expires %s", ErrTurnStillLive, turnID, expiresAt.UTC().Format(time.RFC3339))
	}
	objective := "unselected"
	var startHead string
	if c.Activity != nil {
		events, err := c.Activity.Load(ctx, admission.InvocationID, turnID, 0)
		if err != nil {
			return TurnRecord{}, fmt.Errorf("load lost turn activity: %w", err)
		}
		for _, event := range events {
			if event.Type == ActivityWorkSelected && event.Data["objective"] != "" {
				objective = event.Data["objective"]
			}
		}
	}
	startHead = admission.AdmittedHead
	dir, branch := scopeRepository(admission.Scope)
	if remote == "" {
		remote = "origin"
	}
	repo := GitRepository{Dir: dir, Remote: remote, Branch: branch}
	record := TurnRecord{GoalID: goalID, GoalVersion: version, InvocationID: admission.InvocationID, Mode: admission.Mode, TurnID: turnID, ChildObjective: objective, GraphID: "praxis.package.goals.default", GraphVersion: "0.3.0", StartHead: startHead, Outcome: OutcomeBlocked}
	observed, headErr := repo.run(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if headErr == nil {
		record.EndHead = strings.TrimSpace(observed)
	}
	lostAt := expiresAt.UTC().Format(time.RFC3339)
	if expiresAt.IsZero() {
		lostAt = "unknown"
	}
	record.Blocker = "execution lost: the process (pid " + strconv.Itoa(admission.PID) + " on " + admission.Host + ") stopped renewing its lease at " + lostAt + "; provider consequence unknown, checkout observed at reconciliation"
	if headErr == nil {
		recordConsequence(ctx, repo, &record)
	}
	req := TurnRequest{GoalID: goalID, GoalVersion: version, InvocationID: admission.InvocationID, TurnID: turnID, ChildObjective: objective, GraphID: record.GraphID, GraphVersion: record.GraphVersion, Mode: admission.Mode, ProviderID: "reconciliation"}
	if err := c.emit(ctx, ActivityExecutionLost, req, map[string]string{"lost_at": lostAt, "pid": strconv.Itoa(admission.PID), "host": admission.Host, "consequence": "unknown", "reconciled_by": by.ID}); err != nil {
		return TurnRecord{}, err
	}
	if err := c.recordTurn(ctx, &record); err != nil {
		return TurnRecord{}, fmt.Errorf("record lost turn: %w", err)
	}
	if err := c.emitTurnOutcome(ctx, req, record, errors.New(record.Blocker)); err != nil {
		return TurnRecord{}, err
	}
	state, err = c.Ledger.LoadAdmissions(ctx, goalID, version)
	if err != nil {
		return TurnRecord{}, err
	}
	if err := c.Ledger.appendAdmission(ctx, goalID, version, state.events, turnReleasedEventType, "released:"+turnID, TurnRelease{TurnID: turnID, Disposition: "reconciled", ReleasedAt: now, By: by.ID}, now); err != nil {
		return TurnRecord{}, err
	}
	if leases != nil && admission.LeaseID != "" {
		_ = leases.ReleaseSchedulerResourceLeases(ctx, []string{admission.LeaseID}, now)
	}
	return record, nil
}

// recordTurn appends a turn record at the ledger's current version, read
// at append time rather than at turn start, so independent scopes that
// finish in any order do not conflict.
func (c Controller) recordTurn(ctx context.Context, record *TurnRecord) error {
	turns, err := c.Ledger.Load(ctx, record.GoalID, record.GoalVersion)
	if err != nil {
		return err
	}
	_, err = c.Ledger.Record(ctx, int64(len(turns)), record)
	return err
}
