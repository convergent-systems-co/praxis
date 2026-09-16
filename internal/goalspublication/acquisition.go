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
	if !strings.HasPrefix(completion, "goals-initial-publication:") || !strings.HasSuffix(completion, ":completed") {
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
