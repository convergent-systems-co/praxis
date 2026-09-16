package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// SaveGoalsPublicationRequest persists the exact intent inside the existing
// encrypted AuthorityRequest. There is no separate publication ledger.
func (r Repository) SaveGoalsPublicationRequest(ctx context.Context, a contracts.ActionIntent, now time.Time) (contracts.AuthorityRequest, string, error) {
	fail := func(e error) (contracts.AuthorityRequest, string, error) { return contracts.AuthorityRequest{}, "", e }
	if e := contracts.ValidateGoalsPublicationIntent(a); e != nil {
		return fail(e)
	}
	model, e := r.LoadAuthorityModelState(ctx, now)
	if e != nil {
		return fail(e)
	}
	if model.ActiveVersion != contracts.AuthorityModelGoalsPublicationVersion || model.ActiveDigest != contracts.AuthorityModelGoalsPublicationDigest() || model.State != "committed" {
		return fail(errors.New("exact Goals publication model must be explicitly adopted"))
	}
	root, e := r.LoadAuthorityGeneration(ctx, contracts.InstallationGovernanceScopePrefix+contracts.GoalsPublicationBootstrap, "1", now)
	if e != nil {
		return fail(e)
	}
	if e = r.CheckGoalsPublicationInvalidation(ctx, root.Ref, root.Version, "", "", now); e != nil {
		return fail(e)
	}
	pub, e := r.Store.PublisherGeneration(ctx, contracts.GoalsPublicationPublisher)
	if e != nil {
		return fail(e)
	}
	if pub.State != "active" || pub.Generation.Principal != a.Actor || pub.Generation.Generation != "2" {
		return fail(errors.New("exact enrolled publisher unavailable"))
	}
	if e = pub.Generation.Validate(); e != nil {
		return fail(e)
	}
	pd, e := pub.Generation.Digest()
	if e != nil || pd != contracts.GoalsPublicationPublisher {
		return fail(errors.New("publisher generation digest mismatch"))
	}
	digest, _ := a.Digest()
	expiry, _ := time.Parse(time.RFC3339Nano, a.Parameters["expires_at"])
	d := contracts.DelegationRequest{Profile: contracts.GoalsPublicationProfile, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: a.Actor, TargetKind: "action-intent", TargetIdentity: a.ID, TargetVersion: a.Version, TargetDigest: digest, TargetConstraints: []string{a.Target, a.Parameters["repository_id"]}, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: contracts.GoalsPublicationOperation, RequestedScope: a.Scope, ProposalVersion: a.Version, ProposalDigest: digest, ReviewVersion: a.Version, ReviewDigest: digest, ExpiresAt: expiry, Reason: "publish the exact existing signed Goals package", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelGoalsPublicationVersion, PolicyDigest: contracts.AuthorityModelGoalsPublicationDigest(), SubjectKind: "publisher", SubjectID: a.Actor.ID, SubjectVersion: "2", SubjectDigest: pd, SubjectKeyDigest: pub.Generation.PublicKeyDigest}
	if e = contracts.ValidateGoalsPublicationDelegation(root, d, a, now); e != nil {
		return fail(e)
	}
	req := contracts.AuthorityRequest{ID: "goals-publication-request:" + digest, Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: a.Scope, Reason: d.Reason, Status: contracts.AuthorityRequestPending, Delegation: &d, Intent: &a, IntentDigest: digest, InstallationDigest: contracts.GoalsPublicationRoot}
	rd, e := r.SaveAuthorityRequest(ctx, req, now, &expiry)
	if e != nil {
		return fail(e)
	}
	return req, rd, nil
}

// GoalsPublicationPolicy resolves the intent from its protected request, not
// from an arbitrary file or caller-supplied policy closure.
type GoalsPublicationPolicy struct {
	Repository Repository
	Request    contracts.AuthorityRequest
}

func (p GoalsPublicationPolicy) ContainDelegation(parent contracts.AuthorityGeneration, d contracts.DelegationRequest, now time.Time) error {
	ctx := context.Background()
	model, e := p.Repository.LoadAuthorityModelState(ctx, now)
	if e != nil {
		return e
	}
	if model.ActiveVersion != contracts.AuthorityModelGoalsPublicationVersion || model.ActiveDigest != contracts.AuthorityModelGoalsPublicationDigest() || model.State != "committed" {
		return errors.New("publication successor is not active")
	}
	req, e := p.Repository.LoadAuthorityRequest(ctx, p.Request.ID, p.Request.Version, now)
	if e != nil {
		return e
	}
	if req.Intent == nil || req.Delegation == nil {
		return errors.New("publication request missing protected intent")
	}
	id, err := req.Intent.Digest()
	if err != nil || req.RequestedAuthority != contracts.GovernedPackagePublish || req.ID != "goals-publication-request:"+id || req.IntentDigest != id || req.InstallationDigest != contracts.GoalsPublicationRoot || req.RequestedScope != req.Intent.Scope || req.Status != contracts.AuthorityRequestPending {
		return errors.New("not the exact canonical publication request")
	}
	left, _ := json.Marshal(d)
	right, _ := json.Marshal(req.Delegation)
	if string(left) != string(right) {
		return errors.New("publication delegation substitution")
	}
	if e = p.Repository.CheckGoalsPublicationInvalidation(ctx, parent.Ref, parent.Version, "", "", now); e != nil {
		return e
	}
	return contracts.ValidateGoalsPublicationDelegation(parent, d, *req.Intent, now)
}

// CheckGoalsPublicationInvalidation checks current fences independently of
// historical authority expiry. This is scoped to the publication consumer.
func (r Repository) CheckGoalsPublicationInvalidation(ctx context.Context, ref, version, requestID, requestVersion string, now time.Time) error {
	if ref != "" {
		_, _, e := r.loadWorkPlanBlob(ctx, authorityGenerationInvalidationNamespace, ref, version, now)
		if e == nil {
			return errors.New("publication authority generation is invalidated")
		}
		if !errors.Is(e, state.ErrSecureBlobNotFound) {
			return e
		}
	}
	if requestID != "" {
		_, e := r.LoadAuthorityRevocation(ctx, requestID, requestVersion, now)
		if e == nil {
			return errors.New("publication authority decision is revoked")
		}
		if !errors.Is(e, state.ErrSecureBlobNotFound) {
			return e
		}
	}
	return nil
}

// LoadGoalsPublicationAuthorization returns original authority at an explicit
// execution instant, while always applying current invalidation fences. It
// neither renews an expired grant nor grants permission to execute in the past.
func (r Repository) LoadGoalsPublicationAuthorization(ctx context.Context, requestID string, at, current time.Time) (contracts.PackagePublishAuthorization, error) {
	fail := func(e error) (contracts.PackagePublishAuthorization, error) {
		return contracts.PackagePublishAuthorization{}, e
	}
	payload, _, e := r.loadWorkPlanBlob(ctx, authorityRequestNamespace, requestID, "1", at)
	if e != nil {
		return fail(e)
	}
	var req contracts.AuthorityRequest
	if e = json.Unmarshal(payload, &req); e != nil {
		return fail(e)
	}
	if req.ID != requestID || req.Version != "1" || req.Delegation == nil || req.Intent == nil || req.Delegation.Profile != contracts.GoalsPublicationProfile {
		return fail(errors.New("not a protected Goals publication request"))
	}
	d := req.Delegation
	root, e := r.LoadAuthorityGeneration(ctx, d.ParentRef, d.ParentVersion, at)
	if e != nil {
		return fail(e)
	}
	if e = contracts.ValidateGoalsPublicationDelegation(root, *d, *req.Intent, at); e != nil {
		return fail(e)
	}
	id, _ := req.Intent.Digest()
	if req.IntentDigest != id || req.ID != "goals-publication-request:"+id || req.InstallationDigest != contracts.GoalsPublicationRoot || req.RequestedAuthority != contracts.GovernedPackagePublish || req.RequestedScope != req.Intent.Scope {
		return fail(errors.New("publication request identity mismatch"))
	}
	decisionPayload, _, e := r.loadWorkPlanBlob(ctx, authorityDecisionNamespace, req.ID, req.Version, at)
	if e != nil {
		return fail(e)
	}
	var stored authorityDecisionRecord
	if e = json.Unmarshal(decisionPayload, &stored); e != nil {
		return fail(e)
	}
	original, _ := json.Marshal(req)
	inside, _ := json.Marshal(stored.Request)
	if string(original) != string(inside) {
		return fail(errors.New("publication decision request mismatch"))
	}
	decision := stored.Decision
	if e = decision.Validate(req, at); e != nil {
		return fail(e)
	}
	rd, e := req.DigestAt(at)
	if e != nil {
		return fail(e)
	}
	if decision.RequestID != req.ID || decision.RequestVersion != req.Version || decision.RequestDigest != rd || decision.Outcome != contracts.AuthorityApprove || decision.DecidedBy != root.Principal || decision.AuthorityRef != root.Ref || decision.AuthorityVersion != root.Version || decision.AuthorityGenerationDigest != root.Digest || decision.GrantedScope != req.RequestedScope || decision.AuthorityDigest != contracts.AuthorityModelGoalsPublicationDigest() || decision.IssuedAt.After(at) || decision.ExpiresAt == nil || !at.Before(*decision.ExpiresAt) || decision.Delegation == nil {
		return fail(errors.New("publication decision does not authorize this execution"))
	}
	left, _ := json.Marshal(decision.Delegation)
	right, _ := json.Marshal(d)
	if string(left) != string(right) {
		return fail(errors.New("publication decision delegation mismatch"))
	}
	gen, e := r.LoadAuthorityGeneration(ctx, "authority-delegation:"+req.ID, "1", at)
	if e != nil {
		return fail(e)
	}
	want := contracts.AuthorityGeneration{Ref: "authority-delegation:" + req.ID, Version: "1", Principal: d.DelegatedPrincipal, Scope: d.RequestedScope, Authorities: []string{d.RequestedAuthority}, DelegationProfile: d.Profile, SubjectKind: d.SubjectKind, SubjectID: d.SubjectID, SubjectVersion: d.SubjectVersion, SubjectDigest: d.SubjectDigest, SubjectKeyDigest: d.SubjectKeyDigest, EffectiveAt: decision.IssuedAt.UTC(), ExpiresAt: &d.ExpiresAt, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedBy: decision.DecidedBy, DelegationRef: req.ID + "/" + req.Version, DelegationDigest: rd, PolicyRef: d.PolicyRef, PolicyVersion: d.PolicyVersion, PolicyDigest: d.PolicyDigest, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: d.PolicyVersion, AuthorityModelDigest: d.PolicyDigest, State: contracts.AuthorityGenerationActive, ProvenanceRef: "authority-decision:" + decision.DecisionRef + ":" + decision.DecisionVersion, ProvenanceDigest: decision.AuthorityDigest}
	want.Digest, e = want.ComputeDigest()
	if e != nil || !reflect.DeepEqual(gen, want) || gen.ExpiresAt == nil || !at.Before(*gen.ExpiresAt) || !decision.ExpiresAt.Equal(d.ExpiresAt) {
		return fail(errors.New("publication generation binding mismatch"))
	}
	for _, v := range []contracts.AuthorityGeneration{root, gen} {
		if e = r.CheckGoalsPublicationInvalidation(ctx, v.Ref, v.Version, req.ID, req.Version, current); e != nil {
			return fail(e)
		}
	}
	if decision.DecisionRef != "authority-decision:"+req.ID || decision.DecisionVersion != "1" {
		return fail(fmt.Errorf("invalid publication decision identity"))
	}
	return contracts.PackagePublishAuthorization{Generation: gen, Request: req, Decision: decision}, nil
}
