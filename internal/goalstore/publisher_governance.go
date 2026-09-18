package goalstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	statepkg "github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const publisherGovernanceNamespace = "publisher_governance"
const authorityModelStateID = "active-authority-model"
const authorityModelActiveProjectionID = "authority-model-current"
const signingPreviewPrefix = "publisher-signing-preview:"
const authorityModelAdoptionPrefix = "authority-model-adoption:"
const authorityModelAdoptionDecisionPrefix = "authority-model-adoption-decision:"
const authorityModelAdoptionSupersessionPrefix = "authority-model-adoption-supersession:"

func (r Repository) SaveSigningPreview(ctx context.Context, preview contracts.SigningPreview, now time.Time) (string, error) {
	d, err := preview.DigestValue()
	if err != nil {
		return "", err
	}
	if preview.Digest != "" && preview.Digest != d {
		return "", errors.New("signing preview digest mismatch")
	}
	preview.Digest = d
	var existing contracts.SigningPreview
	if err := r.loadPublisherGovernance(ctx, preview.ID, preview.Version, now, &existing); err == nil {
		existingDigest, digestErr := existing.DigestValue()
		if digestErr != nil || existingDigest != d {
			return "", errors.New("signing preview attempt identity conflict")
		}
		return d, nil
	} else if !errors.Is(err, statepkg.ErrSecureBlobNotFound) {
		return "", err
	}
	if _, err := r.savePublisherGovernance(ctx, preview.ID, preview.Version, preview, now, &preview.ExpiresAt); err != nil {
		return "", err
	}
	return d, nil
}

func (r Repository) LoadSigningPreviewByDigest(ctx context.Context, wanted string, now time.Time) (contracts.SigningPreview, error) {
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, signingPreviewPrefix) {
			continue
		}
		payload, err := r.decryptGovernanceRecord(ctx, record)
		if err != nil {
			return contracts.SigningPreview{}, err
		}
		var preview contracts.SigningPreview
		if err := json.Unmarshal(payload, &preview); err != nil {
			return contracts.SigningPreview{}, err
		}
		if err := preview.VerifyDigest(); err != nil {
			return contracts.SigningPreview{}, err
		}
		if preview.Digest == wanted {
			return preview, nil
		}
	}
	return contracts.SigningPreview{}, statepkg.ErrSecureBlobNotFound
}

func (r Repository) LoadAuthorityModelState(ctx context.Context, now time.Time) (contracts.AuthorityModelState, error) {
	var modelState contracts.AuthorityModelState
	activeID, activeVersion, err := r.loadAuthorityModelActivePointer(ctx)
	if err == nil {
		err = r.loadPublisherGovernance(ctx, activeID, activeVersion, now, &modelState)
	}
	if err != nil {
		if errors.Is(err, statepkg.ErrSecureBlobNotFound) || errors.Is(err, statepkg.ErrSecureBlobExpired) {
			// The adoption record is the durable journal. If a process stopped
			// after journaling the exact transition but before publishing the
			// active-model pointer, recovery must recognize that committed
			// transition rather than report an ambiguous v1 state.
			var adoption contracts.AuthorityModelAdoption
			if journalErr := r.loadPublisherGovernance(ctx, "authority-model-adoption:v1-to-v2", "1", now, &adoption); journalErr == nil {
				adoptionDigest, digestErr := adoption.Digest()
				if digestErr != nil || adoption.FromModel != contracts.AuthorityModelID || adoption.FromVersion != contracts.AuthorityModelVersion || adoption.FromDigest != contracts.AuthorityModelDigest() || adoption.ToModel != contracts.AuthorityModelID || adoption.ToVersion != contracts.AuthorityModelSuccessorVersion || adoption.ToDigest != contracts.AuthorityModelSuccessorDigest() {
					return contracts.AuthorityModelState{}, errors.New("authority-model adoption journal is invalid")
				}
				return contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: contracts.AuthorityModelSuccessorVersion, ActiveDigest: contracts.AuthorityModelSuccessorDigest(), AdoptionDigest: adoptionDigest, State: "committed-recoverable"}, nil
			}
			if journalErr := r.loadPublisherGovernance(ctx, "authority-model-adoption:v2-to-v3", "1", now, &adoption); journalErr == nil {
				adoptionDigest, digestErr := adoption.Digest()
				if digestErr != nil || adoption.FromModel != contracts.AuthorityModelID || adoption.FromVersion != contracts.AuthorityModelSuccessorVersion || adoption.FromDigest != contracts.AuthorityModelSuccessorDigest() || adoption.ToModel != contracts.AuthorityModelID || adoption.ToVersion != contracts.AuthorityModelDeploymentVersion || adoption.ToDigest != contracts.AuthorityModelDeploymentDigest() {
					return contracts.AuthorityModelState{}, errors.New("authority-model v2-to-v3 adoption journal is invalid")
				}
				return contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: contracts.AuthorityModelDeploymentVersion, ActiveDigest: contracts.AuthorityModelDeploymentDigest(), AdoptionDigest: adoptionDigest, State: "committed-recoverable"}, nil
			}
			return contracts.AuthorityModelState{Version: contracts.AuthorityModelVersion, ActiveModel: contracts.AuthorityModelID, ActiveVersion: contracts.AuthorityModelVersion, ActiveDigest: contracts.AuthorityModelDigest(), State: "implicit-v1"}, nil
		}
		return contracts.AuthorityModelState{}, err
	}
	if modelState.ActiveModel != contracts.AuthorityModelID || modelState.ActiveVersion == "" || modelState.ActiveDigest == "" || modelState.State == "" {
		return contracts.AuthorityModelState{}, errors.New("invalid active authority model state")
	}
	if err := contracts.ValidateAuthorityModel(modelState.ActiveModel, modelState.ActiveVersion, modelState.ActiveDigest); err != nil {
		return contracts.AuthorityModelState{}, err
	}
	return modelState, nil
}

func (r Repository) AdoptAuthorityModel(ctx context.Context, adoption contracts.AuthorityModelAdoption, bootstrapDigest, osUser, confirmation string, now time.Time) (string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", err
	}
	digest, err := adoption.Digest()
	if err != nil {
		return "", err
	}
	if confirmation != "ADOPT "+digest {
		return "", errors.New("authority-model adoption confirmation does not bind exact preview")
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return "", err
	}
	// Bind the exact current canonical installation root, never the first
	// root-shaped generation: after ADR-089/090 succession the superseded
	// predecessor is retained as evidence and must not authorize adoption.
	canonical, err := r.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now)
	if err != nil {
		return "", err
	}
	root := &canonical
	if root.Principal != owner || root.Ref != adoption.RootRef || root.Version != adoption.RootVersion || root.Digest != adoption.RootDigest || root.AuthorityModelVersion != contracts.AuthorityModelVersion || root.AuthorityModelDigest != contracts.AuthorityModelDigest() || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+osUser) {
		return "", errors.New("authenticated installation root does not match adoption preview")
	}
	current, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return "", err
	}
	if current.ActiveVersion == adoption.ToVersion && current.ActiveDigest == adoption.ToDigest && current.AdoptionDigest == digest {
		return digest, nil
	}
	if current.ActiveVersion == contracts.AuthorityModelSuccessorVersion {
		if adoption.FromVersion == contracts.AuthorityModelSuccessorVersion && adoption.ToVersion == contracts.AuthorityModelDeploymentVersion && adoption.FromDigest == contracts.AuthorityModelSuccessorDigest() && adoption.ToDigest == contracts.AuthorityModelDeploymentDigest() {
			// v2 remains immutable; this is the explicit v2-to-v3 successor transition.
		} else if current.AdoptionDigest == digest {
			return digest, nil
		} else {
			return "", errors.New("authority model v2 is already adopted by a different transition")
		}
	}
	if current.ActiveVersion != adoption.FromVersion || current.ActiveDigest != adoption.FromDigest || !validModelSuccessor(adoption) {
		return "", errors.New("authority-model adoption source or successor mismatch")
	}
	if (adoption.ToVersion == contracts.AuthorityModelGoalsPublicationVersion || adoption.ToVersion == contracts.AuthorityModelGoalsRecoveryVersion) && (bootstrapDigest != contracts.GoalsPublicationBootstrap || root.Digest != contracts.GoalsPublicationRoot) {
		return "", errors.New("Goals publication model is limited to the accepted installation")
	}

	var prior contracts.AuthorityModelAdoption
	priorExists := false
	if err := r.loadPublisherGovernance(ctx, adoption.ID, adoption.Version, now, &prior); err == nil {
		priorDigest, digestErr := prior.Digest()
		if digestErr != nil || priorDigest != digest {
			return "", errors.New("conflicting authority-model adoption already exists")
		}
		priorExists = true
	} else if errors.Is(err, statepkg.ErrSecureBlobNotFound) || errors.Is(err, statepkg.ErrSecureBlobExpired) {
	} else {
		return "", err
	}
	var decision contracts.AuthorityModelAdoptionDecision
	if priorExists {
		decision, err = r.LoadAuthorityModelAdoptionDecision(ctx, digest, now)
		if err != nil {
			return "", errors.New("historical authority-model adoption has no durable owner decision; supersede it before retrying")
		}
	} else {
		decision = contracts.AuthorityModelAdoptionDecision{ID: authorityModelAdoptionDecisionPrefix + digest, Version: "1", Kind: contracts.AuthorityModelAdoptionDecisionKind, BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, RootRef: root.Ref, RootVersion: root.Version, RootDigest: root.Digest, AdoptionID: adoption.ID, AdoptionVersion: adoption.Version, AdoptionDigest: digest, Decision: "approve", Confirmation: confirmation, ProvenanceRef: "authority-model-adoption:" + digest + ":os-user:" + osUser, ProvenanceDigest: bootstrapDigest, DecidedAt: now.UTC()}
	}
	if _, err := decision.Digest(); err != nil {
		return "", err
	}
	active := contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: adoption.ToVersion, ActiveDigest: adoption.ToDigest, AdoptionDigest: digest, State: "committed"}
	activeID := authorityModelStateID + ":" + adoption.ToVersion
	if err := r.saveAuthorityModelTransitionWithLock(ctx, adoption, decision, !priorExists, activeID, "1", active, now); err != nil {
		return "", fmt.Errorf("persist active authority model: %w", err)
	}
	return digest, nil
}

func (r Repository) LoadAuthorityModelAdoptionDecision(ctx context.Context, adoptionDigest string, now time.Time) (contracts.AuthorityModelAdoptionDecision, error) {
	if adoptionDigest == "" {
		return contracts.AuthorityModelAdoptionDecision{}, statepkg.ErrSecureBlobNotFound
	}
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil {
		return contracts.AuthorityModelAdoptionDecision{}, err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, authorityModelAdoptionDecisionPrefix) {
			continue
		}
		payload, err := r.decryptGovernanceRecord(ctx, record)
		if err != nil {
			return contracts.AuthorityModelAdoptionDecision{}, err
		}
		var decision contracts.AuthorityModelAdoptionDecision
		if err := json.Unmarshal(payload, &decision); err != nil {
			return contracts.AuthorityModelAdoptionDecision{}, err
		}
		digest, err := decision.Digest()
		if err != nil {
			return contracts.AuthorityModelAdoptionDecision{}, err
		}
		if decision.AdoptionDigest == adoptionDigest {
			_ = digest
			return decision, nil
		}
	}
	return contracts.AuthorityModelAdoptionDecision{}, statepkg.ErrSecureBlobNotFound
}

func (r Repository) LoadAuthorityModelAdoptionByDigest(ctx context.Context, wanted string, now time.Time) (contracts.AuthorityModelAdoption, error) {
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil {
		return contracts.AuthorityModelAdoption{}, err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, authorityModelAdoptionPrefix) {
			continue
		}
		payload, err := r.decryptGovernanceRecord(ctx, record)
		if err != nil {
			return contracts.AuthorityModelAdoption{}, err
		}
		var adoption contracts.AuthorityModelAdoption
		if err := json.Unmarshal(payload, &adoption); err != nil {
			return contracts.AuthorityModelAdoption{}, err
		}
		digest, err := adoption.Digest()
		if err != nil {
			return contracts.AuthorityModelAdoption{}, err
		}
		if digest == wanted {
			return adoption, nil
		}
	}
	return contracts.AuthorityModelAdoption{}, statepkg.ErrSecureBlobNotFound
}

func (r Repository) LoadAuthorityModelAdoptionByID(ctx context.Context, id, version string, now time.Time) (contracts.AuthorityModelAdoption, error) {
	var adoption contracts.AuthorityModelAdoption
	if err := r.loadPublisherGovernance(ctx, id, version, now, &adoption); err != nil {
		return contracts.AuthorityModelAdoption{}, err
	}
	if adoption.ID != id || adoption.Version != version {
		return contracts.AuthorityModelAdoption{}, errors.New("authority-model adoption identity mismatch")
	}
	return adoption, nil
}

func (r Repository) IsAuthorityModelAdoptionSuperseded(ctx context.Context, adoptionID, adoptionVersion string, now time.Time) (bool, error) {
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, authorityModelAdoptionSupersessionPrefix) {
			continue
		}
		payload, err := r.decryptGovernanceRecord(ctx, record)
		if err != nil {
			return false, err
		}
		var supersession contracts.AuthorityModelAdoptionSupersession
		if err := json.Unmarshal(payload, &supersession); err != nil {
			return false, err
		}
		if supersession.AdoptionID == adoptionID && supersession.AdoptionVersion == adoptionVersion {
			return true, nil
		}
	}
	return false, nil
}

func (r Repository) SaveAuthorityModelAdoptionSupersession(ctx context.Context, supersession contracts.AuthorityModelAdoptionSupersession, now time.Time) (string, error) {
	digest, err := supersession.Digest()
	if err != nil {
		return "", err
	}
	var prior contracts.AuthorityModelAdoptionSupersession
	if err := r.loadPublisherGovernance(ctx, supersession.ID, supersession.Version, now, &prior); err == nil {
		priorDigest, digestErr := prior.Digest()
		if digestErr != nil || priorDigest == "" || !sameSupersessionStableIdentity(prior, supersession) {
			return "", errors.New("conflicting authority-model supersession already exists")
		}
		return digest, nil
	} else if !errors.Is(err, statepkg.ErrSecureBlobNotFound) && !errors.Is(err, statepkg.ErrSecureBlobExpired) {
		return "", err
	}
	adoption, err := r.LoadAuthorityModelAdoptionByDigest(ctx, supersession.AdoptionDigest, now)
	if err != nil {
		return "", err
	}
	if _, err := r.LoadAuthorityModelAdoptionDecision(ctx, supersession.AdoptionDigest, now); err == nil {
		return "", errors.New("committed authority-model adoption cannot be abandoned")
	}
	current, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return "", err
	}
	if current.ActiveVersion != contracts.AuthorityModelSuccessorVersion || current.ActiveDigest != contracts.AuthorityModelSuccessorDigest() {
		return "", errors.New("only an incomplete v2-to-v3 adoption may be abandoned")
	}
	if supersession.AdoptionID != adoption.ID || supersession.AdoptionVersion != adoption.Version || supersession.RootRef != adoption.RootRef || supersession.RootVersion != adoption.RootVersion || supersession.RootDigest != adoption.RootDigest {
		return "", errors.New("supersession does not bind exact adoption")
	}
	b, err := json.Marshal(supersession)
	if err != nil {
		return "", err
	}
	_ = b
	if err := r.savePublisherGovernanceWithLock(ctx, supersession.ID, supersession.Version, supersession, now, authorityGenerationNamespace, adoption.RootRef, adoption.RootVersion); err != nil {
		return "", err
	}
	return digest, nil
}

func sameSupersessionStableIdentity(existing, replay contracts.AuthorityModelAdoptionSupersession) bool {
	existing.DecidedAt = time.Time{}
	replay.DecidedAt = time.Time{}
	left, leftErr := json.Marshal(existing)
	right, rightErr := json.Marshal(replay)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func (r Repository) AbandonAuthorityModelAdoption(ctx context.Context, adoptionDigest, bootstrapDigest, osUser, confirmation string, now time.Time) (string, error) {
	adoption, err := r.LoadAuthorityModelAdoptionByDigest(ctx, adoptionDigest, now)
	if err != nil {
		return "", err
	}
	if confirmation != "ABANDON "+adoptionDigest {
		return "", errors.New("authority-model abandonment confirmation does not bind exact adoption")
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return "", err
	}
	// Bind the exact current canonical installation root, never the first
	// root-shaped generation: after ADR-089/090 succession the superseded
	// predecessor is retained as evidence and must not authorize adoption.
	canonical, err := r.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now)
	if err != nil {
		return "", err
	}
	root := &canonical
	if root.Principal != owner || root.Ref != adoption.RootRef || root.Version != adoption.RootVersion || root.Digest != adoption.RootDigest || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+osUser) {
		return "", errors.New("authenticated installation root does not match adoption")
	}
	current, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return "", err
	}
	if current.ActiveVersion != contracts.AuthorityModelSuccessorVersion || current.ActiveDigest != contracts.AuthorityModelSuccessorDigest() {
		return "", errors.New("only an incomplete v2-to-v3 adoption may be abandoned")
	}
	if _, err := r.LoadAuthorityModelAdoptionDecision(ctx, adoptionDigest, now); err == nil {
		return "", errors.New("committed authority-model adoption cannot be abandoned")
	}
	supersession := contracts.AuthorityModelAdoptionSupersession{ID: authorityModelAdoptionSupersessionPrefix + adoptionDigest, Version: "1", Kind: contracts.AuthorityModelAdoptionSupersessionKind, BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, RootRef: root.Ref, RootVersion: root.Version, RootDigest: root.Digest, AdoptionID: adoption.ID, AdoptionVersion: adoption.Version, AdoptionDigest: adoptionDigest, Decision: "abandon", Reason: "historical adoption lacks durable owner decision", Confirmation: confirmation, ProvenanceRef: "authority-model-adoption-abandon:" + adoptionDigest + ":os-user:" + osUser, ProvenanceDigest: bootstrapDigest, DecidedAt: now.UTC()}
	return r.SaveAuthorityModelAdoptionSupersession(ctx, supersession, now)
}

func (r Repository) loadAuthorityModelActivePointer(ctx context.Context) (string, string, error) {
	var namespace, id, version string
	err := r.Store.DB().QueryRowContext(ctx, `SELECT object_namespace,object_id,object_version FROM authority_model_active WHERE singleton_id=?`, authorityModelActiveProjectionID).Scan(&namespace, &id, &version)
	if err != nil {
		return "", "", fmt.Errorf("load active authority model pointer: %w", err)
	}
	if namespace != publisherGovernanceNamespace || id == "" || version == "" {
		return "", "", errors.New("active authority model pointer is invalid")
	}
	return id, version, nil
}

func (r Repository) saveAuthorityModelTransitionWithLock(ctx context.Context, adoption contracts.AuthorityModelAdoption, decision contracts.AuthorityModelAdoptionDecision, saveAdoption bool, activeID, activeVersion string, active contracts.AuthorityModelState, now time.Time) error {
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	if activeID == "" || activeVersion == "" {
		return errors.New("active authority model identity is required")
	}
	tx, err := r.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin authority-model transition: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, authorityGenerationNamespace, adoption.RootRef, adoption.RootVersion); err != nil {
		return fmt.Errorf("lock authority-model root: %w", err)
	}
	if saveAdoption {
		if err := r.insertGovernanceRecordTx(ctx, tx, adoption.ID, adoption.Version, adoption, now); err != nil {
			return err
		}
	}
	if err := r.insertGovernanceRecordUnlessExactTx(ctx, tx, decision.ID, decision.Version, decision, now); err != nil {
		return err
	}
	if err := r.insertGovernanceRecordUnlessExactTx(ctx, tx, activeID, activeVersion, active, now); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE authority_model_active SET object_namespace=?,object_id=?,object_version=?,updated_at=? WHERE singleton_id=?`, publisherGovernanceNamespace, activeID, activeVersion, now.UTC().Format(time.RFC3339Nano), authorityModelActiveProjectionID)
	if err != nil {
		return fmt.Errorf("update active authority model pointer: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("active authority model pointer is unavailable")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit authority-model transition: %w", err)
	}
	return nil
}

func (r Repository) insertGovernanceRecordTx(ctx context.Context, tx *sql.Tx, id, version string, value any, created time.Time) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	digest := payloadDigest(b)
	aad := statepkg.SecureBlobAAD(publisherGovernanceNamespace, id, version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, b, aad)
	if err != nil {
		return err
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, publisherGovernanceNamespace, id, version, digest, string(r.Sensitivity), string(r.Profile), envelopeJSON, created.UTC().Format(time.RFC3339Nano), nil)
	if err != nil {
		return fmt.Errorf("insert authority-model record: %w", err)
	}
	return nil
}

func (r Repository) insertGovernanceRecordUnlessExactTx(ctx context.Context, tx *sql.Tx, id, version string, value any, created time.Time) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	digest := payloadDigest(b)
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT object_digest FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, publisherGovernanceNamespace, id, version).Scan(&existing)
	if err == nil {
		if existing != digest {
			return errors.New("active authority model record identity conflict")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect active authority model record: %w", err)
	}
	return r.insertGovernanceRecordTx(ctx, tx, id, version, value, created)
}

func (r Repository) savePublisherGovernance(ctx context.Context, id, version string, value any, created time.Time, expires *time.Time) (string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", err
	}
	b, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	if err := r.putWorkPlanBlob(ctx, publisherGovernanceNamespace, id, version, b, created, expires); err != nil {
		return "", err
	}
	return "", nil
}

func (r Repository) savePublisherGovernanceWithLock(ctx context.Context, id, version string, value any, created time.Time, lockNamespace, lockID, lockVersion string) error {
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	digest := payloadDigest(b)
	aad := statepkg.SecureBlobAAD(publisherGovernanceNamespace, id, version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, b, aad)
	if err != nil {
		return err
	}
	return r.Store.PutSecureBlobWithLock(ctx, statepkg.SecureBlobRecord{Namespace: publisherGovernanceNamespace, ObjectID: id, ObjectVersion: version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: created}, lockNamespace, lockID, lockVersion)
}
func (r Repository) loadPublisherGovernance(ctx context.Context, id, version string, now time.Time, value any) error {
	b, _, e := r.loadWorkPlanBlob(ctx, publisherGovernanceNamespace, id, version, now)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, value)
}

func (r Repository) SaveAuthorityModelAdoption(ctx context.Context, adoption contracts.AuthorityModelAdoption, now time.Time) (string, error) {
	d, e := adoption.Digest()
	if e != nil {
		return "", e
	}
	if !validModelSuccessor(adoption) {
		return "", errors.New("adoption does not bind a supported successor")
	}
	if adoption.ToVersion == contracts.AuthorityModelSuccessorVersion && adoption.ToDigest != contracts.AuthorityModelSuccessorDigest() {
		return "", errors.New("adoption does not bind authority-model v2")
	}
	if adoption.ToVersion == contracts.AuthorityModelDeploymentVersion && adoption.ToDigest != contracts.AuthorityModelDeploymentDigest() {
		return "", errors.New("adoption does not bind authority-model v3")
	}
	if _, e = r.savePublisherGovernance(ctx, adoption.ID, adoption.Version, adoption, now, nil); e != nil {
		return "", e
	}
	return d, nil
}
func (r Repository) SavePublisherAuthorityProposal(ctx context.Context, p contracts.PublisherAuthorityProposal, now time.Time) (string, error) {
	d, e := p.Digest()
	if e != nil {
		return "", e
	}
	if _, e = r.savePublisherGovernance(ctx, p.ID, p.Version, p, now, nil); e != nil {
		return "", e
	}
	return d, nil
}
func (r Repository) SavePublisherAuthorityReview(ctx context.Context, review contracts.PublisherAuthorityReview, now time.Time) (string, error) {
	p := contracts.PublisherAuthorityProposal{}
	if e := r.loadPublisherGovernance(ctx, review.ProposalID, review.ProposalVersion, now, &p); e != nil {
		return "", e
	}
	pd, e := p.Digest()
	if e != nil || pd != review.ProposalDigest {
		return "", errors.New("review does not bind exact proposal")
	}
	if review.Namespace != p.Namespace || review.PublisherGenerationDigest != p.PublisherGenerationDigest {
		return "", errors.New("review subject does not bind exact proposal")
	}
	d, e := review.Digest()
	if e != nil {
		return "", e
	}
	if _, e = r.savePublisherGovernance(ctx, review.ID, review.Version, review, now, nil); e != nil {
		return "", e
	}
	return d, nil
}

func (r Repository) LoadPublisherAuthorityProposalByDigest(ctx context.Context, wanted string, now time.Time) (contracts.PublisherAuthorityProposal, error) {
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil {
		return contracts.PublisherAuthorityProposal{}, err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, "publisher-authority-proposal:") {
			continue
		}
		payload, err := r.decryptGovernanceRecord(ctx, record)
		if err != nil {
			return contracts.PublisherAuthorityProposal{}, err
		}
		var proposal contracts.PublisherAuthorityProposal
		if err := json.Unmarshal(payload, &proposal); err != nil {
			return contracts.PublisherAuthorityProposal{}, err
		}
		digest, err := proposal.Digest()
		if err != nil {
			return contracts.PublisherAuthorityProposal{}, err
		}
		if digest == wanted {
			return proposal, nil
		}
	}
	return contracts.PublisherAuthorityProposal{}, statepkg.ErrSecureBlobNotFound
}

func (r Repository) LoadPublisherAuthorityReviewByDigest(ctx context.Context, wanted string, now time.Time) (contracts.PublisherAuthorityReview, error) {
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil {
		return contracts.PublisherAuthorityReview{}, err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, "publisher-authority-review:") {
			continue
		}
		payload, err := r.decryptGovernanceRecord(ctx, record)
		if err != nil {
			return contracts.PublisherAuthorityReview{}, err
		}
		var review contracts.PublisherAuthorityReview
		if err := json.Unmarshal(payload, &review); err != nil {
			return contracts.PublisherAuthorityReview{}, err
		}
		digest, err := review.Digest()
		if err != nil {
			return contracts.PublisherAuthorityReview{}, err
		}
		if digest == wanted {
			return review, nil
		}
	}
	return contracts.PublisherAuthorityReview{}, statepkg.ErrSecureBlobNotFound
}
func (r Repository) SavePublisherEnrollmentApproval(ctx context.Context, a contracts.PublisherEnrollmentApproval, now time.Time) (string, error) {
	d, e := a.Digest()
	if e != nil {
		return "", e
	}
	if _, e = r.savePublisherGovernance(ctx, a.ID, a.Version, a, now, nil); e != nil {
		return "", e
	}
	return d, nil
}

func (r Repository) LoadPublisherEnrollmentApproval(ctx context.Context, previewDigest string, now time.Time) (contracts.PublisherEnrollmentApproval, error) {
	if previewDigest == "" {
		return contracts.PublisherEnrollmentApproval{}, errors.New("publisher enrollment preview digest is required")
	}
	var approval contracts.PublisherEnrollmentApproval
	if err := r.loadPublisherGovernance(ctx, "publisher-enrollment-approval:"+previewDigest, "1", now, &approval); err != nil {
		return approval, err
	}
	if _, err := approval.Digest(); err != nil || approval.PreviewDigest != previewDigest {
		return contracts.PublisherEnrollmentApproval{}, errors.New("publisher enrollment approval digest mismatch")
	}
	return approval, nil
}

// LoadPublisherEnrollmentApprovalByDigest resolves the canonical approval
// identity without trusting its storage key. This is the production consumer
// lookup used after the owner has reviewed the emitted approval digest.
func (r Repository) LoadPublisherEnrollmentApprovalByDigest(ctx context.Context, approvalDigest string, now time.Time) (contracts.PublisherEnrollmentApproval, error) {
	if approvalDigest == "" {
		return contracts.PublisherEnrollmentApproval{}, errors.New("publisher enrollment approval digest is required")
	}
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil {
		return contracts.PublisherEnrollmentApproval{}, err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, "publisher-enrollment-approval:") {
			continue
		}
		payload, err := r.decryptGovernanceRecord(ctx, record)
		if err != nil {
			return contracts.PublisherEnrollmentApproval{}, err
		}
		var approval contracts.PublisherEnrollmentApproval
		if err := json.Unmarshal(payload, &approval); err != nil {
			return contracts.PublisherEnrollmentApproval{}, err
		}
		digest, err := approval.Digest()
		if err != nil {
			return contracts.PublisherEnrollmentApproval{}, err
		}
		if digest == approvalDigest {
			return approval, nil
		}
	}
	return contracts.PublisherEnrollmentApproval{}, statepkg.ErrSecureBlobNotFound
}

// EnrollPublisherFromApproval is the single repository-owned enrollment
// boundary. It resolves the encrypted approval, revalidates installation
// ownership/model/root/key state, then commits generation and provenance in
// one state transaction.
func (r Repository) EnrollPublisherFromApproval(ctx context.Context, approvalDigest string, bootstrap praxiscrypto.BootstrapRecord, signer praxiscrypto.PublisherSigner, osUser string, now time.Time) (contracts.PublisherGeneration, error) {
	if signer == nil || now.IsZero() {
		return contracts.PublisherGeneration{}, errors.New("publisher signer and enrollment time are required")
	}
	bootstrapDigest, err := bootstrap.Digest()
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	approval, err := r.LoadPublisherEnrollmentApprovalByDigest(ctx, approvalDigest, now)
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	if approval.BootstrapDigest != bootstrapDigest || approval.OwnerID != owner.ID || approval.OwnerKind != owner.Kind || approval.ApproverID != owner.ID || approval.ApproverKind != owner.Kind || approval.AuthorityModel != contracts.AuthorityModelID || approval.AuthorityModelVersion != contracts.AuthorityModelSuccessorVersion || approval.AuthorityModelDigest != contracts.AuthorityModelSuccessorDigest() {
		return contracts.PublisherGeneration{}, errors.New("publisher enrollment approval is not bound to the current installation owner/model")
	}
	generation := approval.GenerationRecord
	if err := approval.ValidateForGeneration(generation, owner); err != nil {
		return contracts.PublisherGeneration{}, err
	}
	if generation.Principal.ID != contracts.FirstPartyPublisherPrincipal {
		return contracts.PublisherGeneration{}, errors.New("publisher enrollment generation is not first-party")
	}
	model, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil || !contracts.AuthorityModelStateRetains(model, contracts.AuthorityModelSuccessorVersion) {
		return contracts.PublisherGeneration{}, errors.New("publisher enrollment requires currently adopted authority-model v2")
	}
	gens, err := r.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	rootFound := false
	for _, root := range gens {
		if root.ParentRef == "" && root.Principal == owner && strings.HasSuffix(root.ProvenanceRef, ":os-user:"+osUser) {
			rootFound = true
			break
		}
	}
	if !rootFound {
		return contracts.PublisherGeneration{}, errors.New("authenticated installation root is unavailable")
	}
	publicKey, err := signer.PublicKey(ctx)
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	keySum := sha256.Sum256(publicKey)
	keyDigest := "sha256:" + hex.EncodeToString(keySum[:])
	if signer.KeyID() != generation.KeyID || signer.Algorithm() != generation.Algorithm || keyDigest != generation.PublicKeyDigest {
		return contracts.PublisherGeneration{}, errors.New("protected publisher key does not match approved generation")
	}
	if _, err := r.Store.CommitCanonicalPublisherEnrollment(ctx, generation, owner, approval.ID, approvalDigest, approval.PreviewDigest, now.UTC()); err != nil {
		return contracts.PublisherGeneration{}, err
	}
	return generation, nil
}

func (r Repository) decryptGovernanceRecord(ctx context.Context, record statepkg.SecureBlobRecord) ([]byte, error) {
	payload, err := r.Crypto.Open(ctx, record.Envelope, record.Envelope.AAD)
	if err != nil {
		return nil, err
	}
	// Secure-blob ObjectDigest is the payload-integrity identity, not the
	// semantic identity carried by the decoded governance object. Keep this
	// check at the shared decrypt boundary so every publisher-governance
	// consumer validates storage integrity independently of its contract
	// digest.
	if payloadDigest(payload) != record.ObjectDigest {
		return nil, errors.New("publisher governance payload integrity mismatch")
	}
	return payload, nil
}

func (r Repository) ApprovePublisherEnrollment(ctx context.Context, preview contracts.PublisherEnrollmentPreview, bootstrapDigest, ownerID, confirmation string, now time.Time) (contracts.PublisherEnrollmentApproval, string, error) {
	previewDigest, err := preview.Digest()
	if err != nil {
		return contracts.PublisherEnrollmentApproval{}, "", err
	}
	if confirmation != "APPROVE-PUBLISHER "+previewDigest {
		return contracts.PublisherEnrollmentApproval{}, "", errors.New("publisher enrollment confirmation does not bind exact preview")
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil || bootstrapDigest != preview.BootstrapDigest || owner.ID != preview.OwnerID || owner.ID != ownerID || owner.Kind != preview.OwnerKind {
		return contracts.PublisherEnrollmentApproval{}, "", errors.New("publisher enrollment owner does not match preview")
	}
	if preview.AuthorityModel != contracts.AuthorityModelID || preview.AuthorityModelVersion != contracts.AuthorityModelSuccessorVersion || preview.AuthorityModelDigest != contracts.AuthorityModelSuccessorDigest() || preview.PublisherPrincipal != contracts.FirstPartyPublisherPrincipal {
		return contracts.PublisherEnrollmentApproval{}, "", errors.New("publisher enrollment preview is not bound to the active accepted authority model")
	}
	model, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil || !contracts.AuthorityModelStateRetains(model, contracts.AuthorityModelSuccessorVersion) {
		return contracts.PublisherEnrollmentApproval{}, "", errors.New("publisher enrollment requires currently adopted authority-model v2")
	}
	gens, err := r.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return contracts.PublisherEnrollmentApproval{}, "", err
	}
	rootFound := false
	for _, generation := range gens {
		if generation.ParentRef == "" && generation.Principal == owner {
			rootFound = true
			break
		}
	}
	if !rootFound {
		return contracts.PublisherEnrollmentApproval{}, "", errors.New("installation governance root is unavailable")
	}
	approval := contracts.PublisherEnrollmentApproval{ID: "publisher-enrollment-approval:" + previewDigest, Version: "1", Kind: "publisher-enrollment-approval", PreviewDigest: previewDigest, BootstrapDigest: preview.BootstrapDigest, OwnerID: preview.OwnerID, OwnerKind: preview.OwnerKind, AuthorityModel: preview.AuthorityModel, AuthorityModelVersion: preview.AuthorityModelVersion, AuthorityModelDigest: preview.AuthorityModelDigest, GenerationTemplateDigest: preview.GenerationDigest, PublisherPrincipal: preview.PublisherPrincipal, PublicKeyDigest: preview.PublicKeyDigest, KeyID: preview.KeyID, Algorithm: preview.Algorithm, Namespace: preview.Namespace, Generation: preview.Generation, Predecessor: preview.Predecessor, GenerationRecord: preview.GenerationRecord, ApproverID: owner.ID, ApproverKind: owner.Kind, IssuedAt: now.UTC()}
	approvalDigest, err := approval.Digest()
	if err != nil {
		return contracts.PublisherEnrollmentApproval{}, "", err
	}
	if _, err := r.savePublisherGovernance(ctx, approval.ID, approval.Version, approval, now, nil); err != nil {
		return contracts.PublisherEnrollmentApproval{}, "", err
	}
	return approval, approvalDigest, nil
}

// Closed successor edges: adding v4 does not reinterpret earlier models.
func validModelSuccessor(a contracts.AuthorityModelAdoption) bool {
	if a.FromModel != contracts.AuthorityModelID || a.ToModel != contracts.AuthorityModelID {
		return false
	}
	switch a.FromVersion {
	case contracts.AuthorityModelVersion:
		return a.FromDigest == contracts.AuthorityModelDigest() && a.ToVersion == contracts.AuthorityModelSuccessorVersion && a.ToDigest == contracts.AuthorityModelSuccessorDigest()
	case contracts.AuthorityModelSuccessorVersion:
		return a.FromDigest == contracts.AuthorityModelSuccessorDigest() && a.ToVersion == contracts.AuthorityModelDeploymentVersion && a.ToDigest == contracts.AuthorityModelDeploymentDigest()
	case contracts.AuthorityModelDeploymentVersion:
		// v3 has two successors in the succession graph (ADR-094): the
		// installation-scoped Goals branch v4 (further limited to the exact
		// Goals installation by AdoptAuthorityModel) and the global routing
		// model v6. v5 is a terminal leaf of the Goals branch.
		if a.FromDigest != contracts.AuthorityModelDeploymentDigest() {
			return false
		}
		return (a.ToVersion == contracts.AuthorityModelGoalsPublicationVersion && a.ToDigest == contracts.AuthorityModelGoalsPublicationDigest()) || (a.ToVersion == contracts.AuthorityModelRoutingVersion && a.ToDigest == contracts.AuthorityModelRoutingDigest())
	case contracts.AuthorityModelGoalsPublicationVersion:
		return a.FromDigest == contracts.AuthorityModelGoalsPublicationDigest() && a.ToVersion == contracts.AuthorityModelGoalsRecoveryVersion && a.ToDigest == contracts.AuthorityModelGoalsRecoveryDigest()
	}
	return false
}
