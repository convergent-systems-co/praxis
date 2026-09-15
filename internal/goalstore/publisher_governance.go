package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	d, e := review.Digest()
	if e != nil {
		return "", e
	}
	if _, e = r.savePublisherGovernance(ctx, review.ID, review.Version, review, now, nil); e != nil {
		return "", e
	}
	return d, nil
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
