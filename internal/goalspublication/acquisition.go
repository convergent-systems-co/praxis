package goalspublication

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"reflect"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func RequiresLocalLineage(release distribution.Release) bool {
	return release.Ref.String() == contracts.GoalsPublicationRepository || release.Manifest.PackageID == "praxis.package.goals" && release.Manifest.Version == "0.1.0" || release.ManifestDigest == contracts.GoalsPublicationManifest
}

// HoldsLocalLineage reports whether this installation carries a durable Goals
// publication or recovery completion. Only the installation that performed a
// publication can connect an acquisition to it; every other installation
// verifies a first-party release through the ordinary trusted-key signature
// path, which this check never replaces.
func (e Execution) HoldsLocalLineage(ctx context.Context) (bool, error) {
	var n int
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events v JOIN commands c ON c.command_id=v.command_id WHERE v.event_version='1' AND c.payload=v.payload AND ((v.event_type='goals-publication-recovery.completed' AND c.command_type='goals-publication-recovery.complete') OR (v.event_type='goals-publication.completed' AND c.command_type='goals-publication.complete'))`).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CheckAcquisition does not install, approve, or verify a package signature. It
// connects this exact local publication to the bytes about to be passed to the
// existing verifier. Inspection is read-only; event recording is separate.
func (e Execution) CheckAcquisition(ctx context.Context, release distribution.Release, artifact []byte) (string, error) {
	if release.Ref.Source != "github-releases" || release.Ref.String() != contracts.GoalsPublicationRepository || release.Tag != contracts.GoalsPublicationTag {
		return "", errors.New("wrong Goals publication locator")
	}
	var decoded packagecatalog.SignatureEnvelope
	if err := json.Unmarshal(release.SignatureBytes, &decoded); err != nil {
		return "", err
	}
	if !reflect.DeepEqual(decoded, release.Signature) {
		return "", errors.New("signature object differs from exact downloaded bytes")
	}
	a := Assets{release.ManifestBytes, artifact, release.SignatureBytes}
	var err error
	if err = a.Validate(); err != nil {
		return "", err
	}
	var recoveryRows []struct {
		ID      string
		Payload []byte
	}
	rrows, qerr := e.Repository.Store.DB().QueryContext(ctx, `SELECT v.event_id,v.payload FROM events v JOIN commands c ON c.command_id=v.command_id WHERE v.event_type='goals-publication-recovery.completed' AND v.event_version='1' AND c.command_type='goals-publication-recovery.complete' AND c.payload=v.payload`)
	if qerr != nil {
		return "", qerr
	}
	for rrows.Next() {
		var item struct {
			ID      string
			Payload []byte
		}
		if qerr = rrows.Scan(&item.ID, &item.Payload); qerr != nil {
			rrows.Close()
			return "", qerr
		}
		recoveryRows = append(recoveryRows, item)
	}
	qerr = rrows.Err()
	rrows.Close()
	if qerr != nil {
		return "", qerr
	}
	if len(recoveryRows) > 0 {
		if len(recoveryRows) != 1 {
			return "", errors.New("multiple Goals recovery completions are inconsistent")
		}
		return e.checkRecoveryAcquisition(ctx, release, a, recoveryRows[0].ID, recoveryRows[0].Payload)
	}
	rows, err := e.Repository.Store.DB().QueryContext(ctx, `SELECT v.event_id,v.payload FROM events v JOIN commands c ON c.command_id=v.command_id WHERE v.event_type='goals-publication.completed' AND v.event_version='1' AND c.command_type='goals-publication.complete' AND c.payload=v.payload`)
	if err != nil {
		return "", err
	}
	type candidate struct {
		id      string
		payload []byte
	}
	var found []candidate
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.payload); err != nil {
			rows.Close()
			return "", err
		}
		found = append(found, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	if len(found) != 1 {
		return "", errors.New("exact unique local publication completion unavailable")
	}
	c := found[0]
	var completion struct {
		Version, RequestID, SigningProvenance string
		Effects                               []string
	}
	if err = json.Unmarshal(c.payload, &completion); err != nil {
		return "", err
	}
	key := executionKey(completion.RequestID)
	if completion.Version != "1" || completion.SigningProvenance != contracts.GoalsPublicationSigningReceipt || c.id != key+":completed" || len(completion.Effects) != len(steps) {
		return "", errors.New("invalid local publication completion")
	}
	previous := []Observation{}
	var intent contracts.ActionIntent
	for i, step := range steps {
		v, err := e.load(ctx, key+":"+step)
		if err != nil {
			return "", err
		}
		if v.State != string(state.EffectSucceeded) || hash(v.Payload)+"/"+hash(v.Result) != completion.Effects[i] {
			return "", errors.New("publication completion effect evidence mismatch")
		}
		intent, err = e.validateStored(ctx, v, completion.RequestID, step)
		if err != nil {
			return "", err
		}
		var o Observation
		if err = json.Unmarshal(v.Result, &o); err != nil {
			return "", err
		}
		if err = validateObservation(intent, step, o, previous); err != nil {
			return "", err
		}
		previous = append(previous, o)
	}
	if err = a.Match(intent); err != nil {
		return "", err
	}
	if err = VerifySigning(ctx, e.Repository, a, e.now()); err != nil {
		return "", err
	}
	if e.Adapter == nil {
		return "", errors.New("publication remote verifier unavailable")
	}
	if err = e.Adapter.Check(ctx, intent, "verify-published", previous); err != nil {
		return "", err
	}
	return c.id, nil
}

// RecordAcquisitionCheck retains the completion identity in the existing
// command/event ledger. It records only the local check, not package verification
// or deployment success. The same checked bytes remain in the caller's memory.
func (e Execution) RecordAcquisitionCheck(ctx context.Context, completion string) error {
	if (!strings.HasPrefix(completion, "goals-initial-publication:") && !strings.HasPrefix(completion, "goals-publication-recovery:")) || !strings.HasSuffix(completion, ":completed") {
		return errors.New("invalid completion reference")
	}
	at := e.now()
	id := completion + ":acquisition:" + at.Format("20060102T150405.000000000Z")
	b := mustJSON(map[string]string{"completion_event": completion, "manifest": contracts.GoalsPublicationManifest, "archive": contracts.GoalsPublicationArchive, "signature": contracts.GoalsPublicationSignature})
	actor := contracts.PackageManagerPrincipal()
	cmd := state.CommandRecord{ID: id, Type: "goals-publication.acquisition-check", Version: "1", Actor: actor, Scope: contracts.GoalsPublicationPackage, CorrelationID: completion, Payload: b, CreatedAt: at}
	event := state.EventRecord{ID: id, AggregateID: id, AggregateType: "goals-publication-acquisition", AggregateVersion: 1, Type: "goals-publication.acquisition-checked", Version: "1", Actor: actor, CommandID: id, CorrelationID: completion, TrustClass: contracts.TrustObserved, Payload: b, CreatedAt: at}
	return e.Repository.Store.CommitTransition(ctx, cmd, 0, event, "", "", nil)
}

func (e Execution) checkRecoveryAcquisition(ctx context.Context, release distribution.Release, a Assets, completionID string, body []byte) (string, error) {
	var c struct {
		Version, RequestID, IntentID, IntentDigest, AuthorityGeneration, SigningProvenance, PredecessorAbandonment, PredecessorManifestOutcome string
		Effects                                                                                                                                []string
		Final                                                                                                                                  Observation
	}
	if err := json.Unmarshal(body, &c); err != nil {
		return "", err
	}
	key := recoveryKey(c.RequestID)
	if c.Version != "1" || completionID != key+":completed" || c.SigningProvenance != contracts.GoalsPublicationSigningReceipt || c.PredecessorManifestOutcome != "unknown-unresolved" || len(c.Effects) != len(recoverySteps) {
		return "", errors.New("invalid Goals successor completion lineage")
	}
	var intent contracts.ActionIntent
	var prior []Observation
	for i, step := range recoverySteps {
		v, err := (RecoveryExecution{Repository: e.Repository, Now: e.Now}).load(ctx, key+":"+step)
		if err != nil {
			return "", err
		}
		if v.State != string(state.EffectSucceeded) || hash(v.Payload)+"/"+hash(v.Result) != c.Effects[i] {
			return "", errors.New("successor completion effect mismatch")
		}
		var p recoveryStepPayload
		if err = json.Unmarshal(v.Payload, &p); err != nil {
			return "", err
		}
		if p.RequestID != c.RequestID || p.Step != step || p.Version != "1" || p.Intent.ID != c.IntentID || p.Authority.Generation.Digest != c.AuthorityGeneration || p.PredecessorAbandonment != c.PredecessorAbandonment {
			return "", errors.New("successor effect authority/intent lineage mismatch")
		}
		auth, err := e.Repository.LoadGoalsPublicationRecoveryAuthorization(ctx, c.RequestID, v.Created, e.now())
		if err != nil {
			return "", err
		}
		if !sameRecoveryAuthorization(auth, p.Authority) {
			return "", errors.New("successor effect authority is not canonical")
		}
		if i == 0 {
			intent = p.Intent
			if err = a.MatchRecovery(intent); err != nil {
				return "", err
			}
			digest, _ := intent.Digest()
			if digest != c.IntentDigest {
				return "", errors.New("successor completion intent digest mismatch")
			}
			if err = e.Repository.ValidateGoalsPublicationAbandonmentBinding(ctx, intent); err != nil {
				return "", err
			}
		} else if p.Intent.ID != intent.ID {
			return "", errors.New("successor intent changed between effects")
		}
		var o Observation
		if err = json.Unmarshal(v.Result, &o); err != nil {
			return "", err
		}
		if err = validateRecoveryObservation(intent, step, o, prior); err != nil {
			return "", err
		}
		prior = append(prior, o)
	}
	if !sameObservation(lastObservation(prior), c.Final) {
		return "", errors.New("successor final state differs from completion")
	}
	if err := VerifySigning(ctx, e.Repository, a, e.now()); err != nil {
		return "", err
	}
	var check func(context.Context, contracts.ActionIntent, string, []Observation) error
	switch v := e.Adapter.(type) {
	case RecoveryGitHub:
		check = v.Check
	case *RecoveryGitHub:
		if v == nil {
			return "", errors.New("fixed successor read-back verifier unavailable")
		}
		check = v.Check
	case GitHub:
		check = RecoveryGitHub{GitHub: v}.Check
	case *GitHub:
		if v == nil {
			return "", errors.New("fixed successor read-back verifier unavailable")
		}
		check = RecoveryGitHub{GitHub: *v}.Check
	default:
		check = e.Adapter.Check
	}
	if err := check(ctx, intent, "verify-published", prior); err != nil {
		return "", err
	}
	return completionID, nil
}
func lastObservation(v []Observation) Observation {
	if len(v) == 0 {
		return Observation{}
	}
	return v[len(v)-1]
}
func sameObservation(a, b Observation) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
