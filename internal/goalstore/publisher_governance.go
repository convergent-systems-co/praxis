package goalstore

import (
	"context"
	"crypto/sha256"
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

func (r Repository) LoadAuthorityModelState(ctx context.Context, now time.Time) (contracts.AuthorityModelState, error) {
	var modelState contracts.AuthorityModelState
	err := r.loadPublisherGovernance(ctx, authorityModelStateID, "1", now, &modelState)
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
	gens, err := r.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return "", err
	}
	var root *contracts.AuthorityGeneration
	for i := range gens {
		if gens[i].ParentRef == "" && gens[i].Principal == owner {
			root = &gens[i]
			break
		}
	}
	if root == nil || root.Ref != adoption.RootRef || root.Version != adoption.RootVersion || root.Digest != adoption.RootDigest || root.AuthorityModelVersion != contracts.AuthorityModelVersion || root.AuthorityModelDigest != contracts.AuthorityModelDigest() || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+osUser) {
		return "", errors.New("authenticated installation root does not match adoption preview")
	}
	current, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return "", err
	}
	if current.ActiveVersion == contracts.AuthorityModelSuccessorVersion {
		if current.AdoptionDigest == digest {
			return digest, nil
		}
		return "", errors.New("authority model v2 is already adopted by a different transition")
	}
	if current.ActiveVersion != contracts.AuthorityModelVersion || current.ActiveDigest != contracts.AuthorityModelDigest() || adoption.FromVersion != contracts.AuthorityModelVersion || adoption.FromDigest != contracts.AuthorityModelDigest() || adoption.ToVersion != contracts.AuthorityModelSuccessorVersion || adoption.ToDigest != contracts.AuthorityModelSuccessorDigest() {
		return "", errors.New("authority-model adoption source or successor mismatch")
	}
	var prior contracts.AuthorityModelAdoption
	if err := r.loadPublisherGovernance(ctx, adoption.ID, adoption.Version, now, &prior); err == nil {
		priorDigest, digestErr := prior.Digest()
		if digestErr != nil || priorDigest != digest {
			return "", errors.New("conflicting authority-model adoption already exists")
		}
	} else if errors.Is(err, statepkg.ErrSecureBlobNotFound) || errors.Is(err, statepkg.ErrSecureBlobExpired) {
		if err := r.savePublisherGovernanceWithLock(ctx, adoption.ID, adoption.Version, adoption, now, authorityGenerationNamespace, adoption.RootRef, adoption.RootVersion); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	active := contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: contracts.AuthorityModelSuccessorVersion, ActiveDigest: contracts.AuthorityModelSuccessorDigest(), AdoptionDigest: digest, State: "committed"}
	if _, err := r.savePublisherGovernance(ctx, authorityModelStateID, "1", active, now, nil); err != nil {
		return "", fmt.Errorf("persist active authority model: %w", err)
	}
	return digest, nil
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
	if adoption.ToVersion != contracts.AuthorityModelSuccessorVersion || adoption.ToDigest != contracts.AuthorityModelSuccessorDigest() {
		return "", errors.New("adoption does not bind authority-model v2")
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
	if err != nil || model.ActiveVersion != contracts.AuthorityModelSuccessorVersion || model.ActiveDigest != contracts.AuthorityModelSuccessorDigest() {
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
	return r.Crypto.Open(ctx, record.Envelope, record.Envelope.AAD)
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
	if err != nil || model.ActiveVersion != contracts.AuthorityModelSuccessorVersion || model.ActiveDigest != contracts.AuthorityModelSuccessorDigest() {
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
