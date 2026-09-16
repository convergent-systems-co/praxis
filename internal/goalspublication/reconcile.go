package goalspublication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Reconcile observes only the original uncertain effect. It is permitted after
// authority expiry/revocation, but cannot dispatch, admit work, or assert success.
// Its result is appended to the existing ledger without overwriting a concurrent
// dispatch result. Matching remote content alone never closes an unknown effect.
func (e Execution) Reconcile(ctx context.Context, requestID string) error {
	if e.Adapter == nil || e.Repository.Store == nil {
		return errors.New("reconciliation dependencies missing")
	}
	key := executionKey(requestID)
	previous := []Observation{}
	for _, step := range steps {
		id := key + ":" + step
		v, err := e.load(ctx, id)
		if err != nil {
			return err
		}
		var p stepPayload
		if err = json.Unmarshal(v.Payload, &p); err != nil {
			return err
		}
		if p.Version != "1" || p.RequestID != requestID || p.Step != step {
			return errors.New("reconciliation payload mismatch")
		}
		if err = contracts.ValidateGoalsPublicationIntent(p.Intent); err != nil {
			return err
		}
		if v.State == string(state.EffectSucceeded) {
			var o Observation
			if err = json.Unmarshal(v.Result, &o); err != nil {
				return err
			}
			if err = validateObservation(p.Intent, step, o, previous); err != nil {
				return err
			}
			previous = append(previous, o)
			continue
		}
		if v.State != string(state.EffectDispatched) && v.State != string(state.EffectUnknown) && v.State != string(state.EffectReconciling) {
			return errors.New("no uncertain dispatch to reconcile")
		}
		observed, remoteErr := e.Adapter.Reconcile(ctx, p.Intent, step, previous)
		// Only inspection evidence, never new authority or a success receipt.
		reason := "matching state does not establish dispatch attribution"
		if remoteErr != nil {
			reason = remoteErr.Error()
		}
		now := e.now()
		eventID := id + ":reconciliation:" + now.Format("20060102T150405.000000000Z")
		payload := mustJSON(struct {
			EffectID, RequestDigest, Outcome, Reason string
			Observation                              Observation
		}{id, hash(v.Payload), "unresolved", reason, observed})
		cmd := state.CommandRecord{ID: eventID, Type: "goals-publication.reconcile", Version: "1", Actor: p.Intent.Actor, Scope: p.Intent.Scope, CorrelationID: key, Payload: payload, CreatedAt: now}
		event := state.EventRecord{ID: eventID, AggregateID: eventID, AggregateType: "goals-publication-reconciliation", AggregateVersion: 1, Type: "goals-publication.reconciled", Version: "1", Actor: p.Intent.Actor, CommandID: eventID, CorrelationID: key, TrustClass: contracts.TrustObserved, Payload: payload, CreatedAt: now}
		if err = e.Repository.Store.CommitTransition(ctx, cmd, 0, event, "", "", nil); err != nil {
			return err
		}
		return fmt.Errorf("publication effect remains unresolved: %s (evidence %s)", reason, eventID)
	}
	return errors.New("no uncertain publication effect")
}
