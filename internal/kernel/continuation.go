package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type ContinuationAction string

const (
	ContinuationContinue   ContinuationAction = "continue"
	ContinuationConstrain  ContinuationAction = "constrain"
	ContinuationCheckpoint ContinuationAction = "checkpoint"
	ContinuationHandoff    ContinuationAction = "handoff"
)

type ResourceRule struct {
	ID        string             `json:"id"`
	Signal    string             `json:"signal"`
	AtOrAbove int64              `json:"at_or_above"`
	Action    ContinuationAction `json:"action"`
}

type ResourceProfile struct {
	ID      string         `json:"id"`
	Version string         `json:"version"`
	Rules   []ResourceRule `json:"rules"`
}

func (p ResourceProfile) Validate() error {
	if p.ID == "" || p.Version == "" || len(p.Rules) == 0 {
		return errors.New("resource profile identity, version, and rules are required")
	}
	seen := map[string]bool{}
	for _, rule := range p.Rules {
		if rule.ID == "" || rule.Signal == "" || rule.AtOrAbove < 0 || continuationStrength(rule.Action) < 0 {
			return errors.New("resource rule identity, signal, non-negative threshold, and known action are required")
		}
		if seen[rule.ID] {
			return errors.New("duplicate resource rule identity")
		}
		seen[rule.ID] = true
	}
	return nil
}

type ResourceObservation struct {
	ID       string           `json:"id"`
	RunID    string           `json:"run_id"`
	AgentID  string           `json:"agent_id,omitempty"`
	Signals  map[string]int64 `json:"signals"`
	Evidence []string         `json:"evidence"`
}

func (o ResourceObservation) Validate() error {
	if o.ID == "" || o.RunID == "" || len(o.Signals) == 0 || len(o.Evidence) == 0 {
		return errors.New("resource observation identity, run, signals, and evidence are required")
	}
	for signal, value := range o.Signals {
		if signal == "" || value < 0 {
			return errors.New("resource observation signals require names and non-negative values")
		}
	}
	return nil
}

type ContinuationDecision struct {
	ID                string             `json:"id"`
	ProfileID         string             `json:"profile_id"`
	ProfileVersion    string             `json:"profile_version"`
	RuleID            string             `json:"rule_id"`
	ObservationDigest string             `json:"observation_digest"`
	Action            ContinuationAction `json:"action"`
	CheckpointRef     string             `json:"checkpoint_ref,omitempty"`
	HandoffRef        string             `json:"handoff_ref,omitempty"`
}

func (d ContinuationDecision) Verify() error {
	if d.ID == "" || d.ProfileID == "" || d.ProfileVersion == "" || d.RuleID == "" || d.ObservationDigest == "" || continuationStrength(d.Action) < 0 {
		return errors.New("continuation decision is incomplete")
	}
	want, err := continuationDecisionID(d)
	if err != nil {
		return err
	}
	if d.ID != want {
		return errors.New("continuation decision digest mismatch")
	}
	if (d.Action == ContinuationCheckpoint || d.Action == ContinuationHandoff) && d.CheckpointRef == "" {
		return errors.New("checkpoint action requires checkpoint reference")
	}
	if d.Action == ContinuationHandoff && d.HandoffRef == "" {
		return errors.New("handoff action requires handoff reference")
	}
	return nil
}

func EvaluateResourceObservation(profile ResourceProfile, observation ResourceObservation) (ContinuationDecision, error) {
	if err := profile.Validate(); err != nil {
		return ContinuationDecision{}, err
	}
	if err := observation.Validate(); err != nil {
		return ContinuationDecision{}, err
	}
	rules := append([]ResourceRule(nil), profile.Rules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	selected := ResourceRule{ID: "default-continue", Action: ContinuationContinue}
	for _, rule := range rules {
		value, exists := observation.Signals[rule.Signal]
		if !exists || value < rule.AtOrAbove {
			continue
		}
		if continuationStrength(rule.Action) > continuationStrength(selected.Action) {
			selected = rule
		}
	}
	observationDigest, err := continuationDigest(observation)
	if err != nil {
		return ContinuationDecision{}, err
	}
	decision := ContinuationDecision{ProfileID: profile.ID, ProfileVersion: profile.Version, RuleID: selected.ID, ObservationDigest: "sha256:" + observationDigest, Action: selected.Action}
	if selected.Action == ContinuationCheckpoint || selected.Action == ContinuationHandoff {
		checkpointDigest, digestErr := continuationDigest(struct {
			ProfileID         string
			ProfileVersion    string
			RuleID            string
			ObservationDigest string
			Action            ContinuationAction
		}{profile.ID, profile.Version, selected.ID, decision.ObservationDigest, selected.Action})
		if digestErr != nil {
			return ContinuationDecision{}, digestErr
		}
		decision.CheckpointRef = "checkpoint:sha256:" + checkpointDigest
	}
	if selected.Action == ContinuationHandoff {
		decision.HandoffRef = "handoff:" + strings.TrimPrefix(decision.CheckpointRef, "checkpoint:")
	}
	decision.ID, err = continuationDecisionID(decision)
	if err != nil {
		return ContinuationDecision{}, err
	}
	return decision, nil
}

type ContinuationAuthorizer interface {
	AuthorizeContinuation(context.Context, contracts.PrincipalRef, RunExecution, ContinuationDecision) error
}

type ContinuationController struct {
	Authorizer ContinuationAuthorizer
}

func (c ContinuationController) Apply(ctx context.Context, run *RunExecution, profile ResourceProfile, observation ResourceObservation, journal *EventJournal) (ContinuationDecision, error) {
	if run == nil || journal == nil || c.Authorizer == nil {
		return ContinuationDecision{}, errors.New("run, journal, and continuation authorizer are required")
	}
	if run.State.Terminal() {
		return ContinuationDecision{}, errors.New("terminal run cannot receive continuation decision")
	}
	if run.State == RunSuspended || run.PendingWait != nil {
		return ContinuationDecision{}, errors.New("suspended run must resolve its existing wait before another continuation decision")
	}
	if observation.RunID != run.RunID || observation.AgentID != run.AgentID {
		return ContinuationDecision{}, errors.New("resource observation run/agent identity mismatch")
	}
	decision, err := EvaluateResourceObservation(profile, observation)
	if err != nil {
		return ContinuationDecision{}, err
	}
	if err := c.Authorizer.AuthorizeContinuation(ctx, journal.Actor, *run, decision); err != nil {
		return ContinuationDecision{}, fmt.Errorf("continuation denied: %w", err)
	}

	next := *run
	next.Evidence = append([]string(nil), run.Evidence...)
	next.ResourceObservations = append([]ResourceObservation(nil), run.ResourceObservations...)
	next.ContinuationHistory = append([]ContinuationDecision(nil), run.ContinuationHistory...)
	next.ResourceObservations = append(next.ResourceObservations, observation)
	next.ContinuationHistory = append(next.ContinuationHistory, decision)
	next.Evidence = append(next.Evidence, observation.Evidence...)
	var decisionCheckpoint *Checkpoint
	if decision.Action == ContinuationCheckpoint || decision.Action == ContinuationHandoff {
		checkpoint := Checkpoint{RunID: next.RunID, AgentID: next.AgentID, GraphID: next.GraphID, GraphVersion: next.GraphVersion, CurrentNode: next.CurrentNode, RunState: next.State, TransitionCount: next.TransitionCount, LastEventSequence: journal.LastEventSequence, CompletedEvidence: append([]string(nil), next.Evidence...)}
		next.LastCheckpoint = &checkpoint
		decisionCheckpoint = &checkpoint
	}
	if decision.Action == ContinuationHandoff {
		next.State = RunSuspended
		next.PendingWait = &Suspension{Kind: WaitHandoff, Ref: decision.HandoffRef}
		if next.LastCheckpoint != nil {
			next.LastCheckpoint.RunState = RunSuspended
		}
	}
	observationEvent := baseObservation(&next, ObservationContinuationDecided, next.CurrentNode)
	observationEvent.Resource = &observation
	observationEvent.Continuation = &decision
	observationEvent.Checkpoint = decisionCheckpoint
	observationEvent.Evidence = append([]string(nil), observation.Evidence...)
	observationEvent.Wait = next.PendingWait
	if err := journal.ObserveRun(ctx, observationEvent); err != nil {
		return ContinuationDecision{}, err
	}
	*run = next
	return decision, nil
}

func continuationStrength(action ContinuationAction) int {
	switch action {
	case ContinuationContinue:
		return 0
	case ContinuationConstrain:
		return 1
	case ContinuationCheckpoint:
		return 2
	case ContinuationHandoff:
		return 3
	default:
		return -1
	}
}

func continuationDecisionID(decision ContinuationDecision) (string, error) {
	decision.ID = ""
	digest, err := continuationDigest(decision)
	if err != nil {
		return "", err
	}
	return "decision:sha256:" + digest, nil
}

func continuationDigest(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}
