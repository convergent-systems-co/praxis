package goalspublication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	observationResolutionCommandType = "goals-publication-recovery.resolve-observation"
	observationResolutionEventType   = "goals-publication-recovery.observation-resolved"
)

type observationResolutionSnapshot struct {
	RequestID, RequestDigest, EffectID, IntentDigest, AuthorityDigest string
	ExecutionID, Step, Adapter, Contract, Operation                   string
	Attempts                                                          int
	Original, Reconciliation                                          Observation
	OriginalResultDigest                                              string
	ReconciliationEventID, ReconciliationPayloadDigest                string
	EffectCreatedAt                                                   time.Time
}

type canonicalObservationAsset struct {
	ID       int64
	Name     string
	Size     int64
	State    string
	Uploader int64
	Digest   string
}
type canonicalObservation struct {
	RepositoryID, OwnerID, AccountID int64
	Commit, Tree                     string
	ReleaseID                        int64
	Tag, Target, Name, Body          string
	Draft, Prerelease                bool
	Assets                           []canonicalObservationAsset
}

func canonicalObservationValue(o Observation) (canonicalObservation, error) {
	if o.Release == nil {
		return canonicalObservation{}, errors.New("observation release is required")
	}
	if len(o.Release.Assets) != len(o.AssetDigests) {
		return canonicalObservation{}, errors.New("observation asset evidence is incomplete")
	}
	dig := map[string]string{}
	for i, a := range o.Release.Assets {
		if a.Name == "" || dig[a.Name] != "" {
			return canonicalObservation{}, errors.New("observation asset identity is ambiguous")
		}
		dig[a.Name] = o.AssetDigests[i]
	}
	names := []string{"praxis-package.json", "praxis-package.tar.gz", "praxis-package.sig.json"}
	assets := make([]canonicalObservationAsset, 0, len(names))
	for _, name := range names {
		var found *Asset
		for i := range o.Release.Assets {
			if o.Release.Assets[i].Name == name {
				found = &o.Release.Assets[i]
				break
			}
		}
		if found == nil || dig[name] == "" {
			return canonicalObservation{}, errors.New("observation asset inventory is incomplete")
		}
		assets = append(assets, canonicalObservationAsset{found.ID, found.Name, found.Size, found.State, found.Uploader.ID, dig[name]})
	}
	if len(o.Release.Assets) != len(names) {
		return canonicalObservation{}, errors.New("observation asset inventory is unexpected")
	}
	return canonicalObservation{RepositoryID: o.RepositoryID, OwnerID: o.OwnerID, AccountID: o.AccountID, Commit: o.Commit, Tree: o.Tree, ReleaseID: o.Release.ID, Tag: o.Release.Tag, Target: o.Release.Target, Name: o.Release.Name, Body: o.Release.Body, Draft: o.Release.Draft, Prerelease: o.Release.Prerelease, Assets: assets}, nil
}

func observationSemanticallyEqual(a, b Observation) error {
	x, err := canonicalObservationValue(a)
	if err != nil {
		return err
	}
	y, err := canonicalObservationValue(b)
	if err != nil {
		return err
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	if string(xb) != string(yb) {
		return errors.New("observations are not semantically equal")
	}
	return nil
}

func resolutionDigest(s observationResolutionSnapshot) (string, error) {
	x := struct {
		RequestID, RequestDigest, EffectID, IntentDigest, AuthorityDigest, ExecutionID, Step, Adapter, Contract, Operation string
		Attempts                                                                                                           int
		Original, Reconciliation                                                                                           canonicalObservation
		ReconciliationEventID, ReconciliationPayloadDigest                                                                 string
		OriginalResultDigest                                                                                               string
		EffectCreatedAt                                                                                                    string
	}{s.RequestID, s.RequestDigest, s.EffectID, s.IntentDigest, s.AuthorityDigest, s.ExecutionID, s.Step, s.Adapter, s.Contract, s.Operation, s.Attempts, mustCanonical(s.Original), mustCanonical(s.Reconciliation), s.ReconciliationEventID, s.ReconciliationPayloadDigest, s.OriginalResultDigest, s.EffectCreatedAt.UTC().Format(time.RFC3339Nano)}
	b, err := json.Marshal(x)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}
func mustCanonical(o Observation) canonicalObservation {
	v, _ := canonicalObservationValue(o)
	return v
}

func (e RecoveryExecution) loadObservationResolution(ctx context.Context, requestID, effectID string) (observationResolutionSnapshot, error) {
	if !strings.HasPrefix(requestID, "goals-publication-recovery-request:") {
		return observationResolutionSnapshot{}, errors.New("observation resolution request is invalid")
	}
	key := recoveryKey(requestID)
	if effectID != key+":verify-draft" {
		return observationResolutionSnapshot{}, errors.New("observation resolution effect is invalid")
	}
	var st string
	var attempts int
	var payload, observed []byte
	var actionDigest, adapter, created string
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT e.state,e.attempts,e.request_payload,e.observed_result,e.action_intent_digest,e.target_adapter,e.created_at FROM effects e JOIN commands c ON c.command_id=e.command_id JOIN events v ON v.event_id=e.effect_id WHERE e.effect_id=? AND e.command_id=e.effect_id AND c.command_type='goals-publication-recovery.step' AND v.event_type='goals-publication-recovery.step-admitted' AND c.payload=e.request_payload AND v.payload=e.request_payload`, effectID).Scan(&st, &attempts, &payload, &observed, &actionDigest, &adapter, &created)
	if err != nil {
		return observationResolutionSnapshot{}, err
	}
	if st != string(state.EffectUnknown) || attempts != 1 || len(observed) == 0 || adapter != "goals-recovery-github" {
		return observationResolutionSnapshot{}, errors.New("observation resolution effect eligibility invalid")
	}
	at, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return observationResolutionSnapshot{}, err
	}
	var sp recoveryStepPayload
	if err := json.Unmarshal(payload, &sp); err != nil {
		return observationResolutionSnapshot{}, err
	}
	if sp.Version != "1" || sp.RequestID != requestID || sp.Step != "verify-draft" || sp.Intent.Parameters["contract"] != contracts.GoalsFailedVerificationContract || sp.Intent.Operation != contracts.GoalsRecoveryOperation {
		return observationResolutionSnapshot{}, errors.New("observation resolution lineage invalid")
	}
	if err := contracts.ValidateGoalsPublicationRecoveryIntent(sp.Intent); err != nil {
		return observationResolutionSnapshot{}, fmt.Errorf("observation resolution intent validation: %w", err)
	}
	intentDigest, err := sp.Intent.Digest()
	if err != nil || intentDigest != actionDigest {
		return observationResolutionSnapshot{}, errors.New("observation resolution intent mismatch")
	}
	authAt, err := e.Repository.LoadGoalsPublicationRecoveryAuthorization(ctx, requestID, at, at)
	if err != nil {
		return observationResolutionSnapshot{}, err
	}
	if !sameRecoveryAuthorization(sp.Authority, authAt) {
		return observationResolutionSnapshot{}, errors.New("observation resolution authority mismatch")
	}
	requestDigest, err := authAt.Request.DigestAt(at)
	if err != nil {
		return observationResolutionSnapshot{}, err
	}
	if at.Before(authAt.Generation.EffectiveAt) || authAt.Generation.ExpiresAt == nil || !at.Before(*authAt.Generation.ExpiresAt) {
		return observationResolutionSnapshot{}, errors.New("original dispatch was outside authority interval")
	}
	current, err := e.current(ctx, requestID)
	if err != nil {
		return observationResolutionSnapshot{}, err
	}
	if current.Generation.Digest != authAt.Generation.Digest || current.Generation.ExpiresAt == nil || !e.now().Before(*current.Generation.ExpiresAt) {
		return observationResolutionSnapshot{}, errors.New("delegated authority is not active")
	}
	var original Observation
	if err := json.Unmarshal(observed, &original); err != nil {
		return observationResolutionSnapshot{}, err
	}
	if err := validateRecoveryObservation(sp.Intent, "verify-draft", original, nil); err != nil {
		return observationResolutionSnapshot{}, err
	}
	var recID, recPayload []byte
	rows, err := e.Repository.Store.DB().QueryContext(ctx, `SELECT v.event_id,v.payload FROM events v JOIN commands c ON c.command_id=v.command_id AND c.payload=v.payload WHERE v.event_type='goals-publication-recovery.reconciled' AND v.event_version='1' AND c.command_type='goals-publication-recovery.reconcile' AND c.command_version='1' AND v.correlation_id=? ORDER BY v.created_at`, key)
	if err != nil {
		return observationResolutionSnapshot{}, err
	}
	for rows.Next() {
		var id string
		var p []byte
		if err := rows.Scan(&id, &p); err != nil {
			rows.Close()
			return observationResolutionSnapshot{}, err
		}
		var x struct {
			EffectID, Outcome, Reason string
			Observation               Observation
		}
		if json.Unmarshal(p, &x) == nil && x.EffectID == effectID {
			recID = append([]byte(id), 0)
			recPayload = p
		}
	}
	rows.Close()
	if len(recID) == 0 {
		return observationResolutionSnapshot{}, errors.New("unresolved reconciliation evidence is required")
	}
	reconciliationEventID := string(recID[:len(recID)-1])
	var rr struct {
		EffectID, Outcome, Reason string
		Observation               Observation
	}
	if err := json.Unmarshal(recPayload, &rr); err != nil || rr.EffectID != effectID || rr.Outcome != "unresolved" {
		return observationResolutionSnapshot{}, errors.New("reconciliation event is not unresolved")
	}
	if err := validateRecoveryObservation(sp.Intent, "verify-draft", rr.Observation, nil); err != nil {
		return observationResolutionSnapshot{}, err
	}
	if err := observationSemanticallyEqual(original, rr.Observation); err != nil {
		return observationResolutionSnapshot{}, err
	}
	// The /4 frontier is exactly one effect; publish and verify-published are unprepared.
	graph, effects, err := e.loadRecoveryFrontier(ctx, requestID)
	if err != nil {
		return observationResolutionSnapshot{}, err
	}
	if len(graph) != 3 || len(effects) != 1 {
		return observationResolutionSnapshot{}, errors.New("observation resolution later effects exist")
	}
	if _, ok := effects["verify-draft"]; !ok {
		return observationResolutionSnapshot{}, errors.New("observation resolution frontier mismatch")
	}
	var abandoned int
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_type='goals-publication-recovery.abandoned' AND correlation_id=?`, key).Scan(&abandoned); err != nil || abandoned != 0 {
		return observationResolutionSnapshot{}, errors.New("observation resolution execution is abandoned")
	}
	var prior int
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_type=? AND correlation_id=?`, observationResolutionEventType, key).Scan(&prior); err != nil {
		return observationResolutionSnapshot{}, err
	}
	if prior > 0 {
		return observationResolutionSnapshot{}, errors.New("observation resolution already exists")
	}
	dig := hash(recPayload)
	return observationResolutionSnapshot{RequestID: requestID, RequestDigest: requestDigest, EffectID: effectID, IntentDigest: intentDigest, AuthorityDigest: current.Generation.Digest, ExecutionID: key, Step: "verify-draft", Adapter: adapter, Contract: sp.Intent.Parameters["contract"], Operation: sp.Intent.Operation, Attempts: attempts, Original: original, Reconciliation: rr.Observation, OriginalResultDigest: hash(observed), ReconciliationEventID: reconciliationEventID, ReconciliationPayloadDigest: dig, EffectCreatedAt: at}, nil
}

// ObservationResolutionChallenge performs all validation without writing or
// contacting the provider and returns the stable confirmation digest.
func (e RecoveryExecution) ObservationResolutionChallenge(ctx context.Context, requestID, effectID string) (string, error) {
	s, err := e.loadObservationResolution(ctx, requestID, effectID)
	if err != nil {
		return "", err
	}
	return resolutionDigest(s)
}

// ResolveObservation atomically appends resolution history and closes the
// UNKNOWN observational effect. It never invokes an adapter.
func (e RecoveryExecution) ResolveObservation(ctx context.Context, requestID, effectID, confirmation string) (string, error) {
	rows, qerr := e.Repository.Store.DB().QueryContext(ctx, `SELECT event_id,payload FROM events WHERE event_type=? AND event_id LIKE ?`, observationResolutionEventType, effectID+":resolution:%")
	if qerr != nil {
		return "", qerr
	}
	for rows.Next() {
		var id string
		var body []byte
		if err := rows.Scan(&id, &body); err != nil {
			rows.Close()
			return "", err
		}
		var p struct{ ResolutionDigest string }
		if err := json.Unmarshal(body, &p); err != nil {
			rows.Close()
			return "", err
		}
		if confirmation != "RESOLVE "+p.ResolutionDigest {
			rows.Close()
			return "", errors.New("observation resolution confirmation conflicts with completed resolution")
		}
		rows.Close()
		return id, nil
	}
	rows.Close()
	s, err := e.loadObservationResolution(ctx, requestID, effectID)
	if err != nil {
		return "", err
	}
	dig, err := resolutionDigest(s)
	if err != nil {
		return "", err
	}
	if confirmation != "RESOLVE "+dig {
		return "", errors.New("observation resolution confirmation mismatch")
	}
	resID := effectID + ":resolution:" + dig
	payload := mustJSON(struct{ Version, RequestID, EffectID, ResolutionDigest, Outcome, ObservationDigest string }{"1", requestID, effectID, dig, "succeeded", s.ReconciliationPayloadDigest})
	at := e.now()
	cmd := state.CommandRecord{ID: resID, Type: observationResolutionCommandType, Version: "1", Actor: contracts.PrincipalRef{ID: "publisher:praxis-first-party", Kind: "publisher"}, Scope: s.EffectID, CorrelationID: s.ExecutionID, Payload: payload, CreatedAt: at}
	ev := state.EventRecord{ID: resID, AggregateID: resID, AggregateType: "goals-publication-recovery-observation-resolution", AggregateVersion: 1, Type: observationResolutionEventType, Version: "1", Actor: cmd.Actor, CommandID: resID, CorrelationID: s.ExecutionID, TrustClass: contracts.TrustObserved, Payload: payload, CreatedAt: at}
	if err := e.Repository.Store.CommitObservationResolution(ctx, cmd, ev, effectID, payload, at); err != nil {
		// Exact replay is idempotent: return the existing immutable resolution.
		var existing []byte
		if qerr := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND event_type=?`, resID, observationResolutionEventType).Scan(&existing); qerr == nil && string(existing) == string(payload) {
			return resID, nil
		}
		return "", err
	}
	return resID, nil
}
