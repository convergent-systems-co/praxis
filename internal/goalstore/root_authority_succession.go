package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// LoadCurrentInstallationRoot resolves the sole active installation root
// under the current root semantics: governance scope bound to the bootstrap
// digest.
func (r Repository) LoadCurrentInstallationRoot(ctx context.Context, bootstrapDigest string, now time.Time) (contracts.AuthorityGeneration, error) {
	return r.loadInstallationRoot(ctx, bootstrapDigest, true, now)
}

// LoadInstallationRootForSchema resolves the sole active installation root
// under the root semantics that were valid when sourceSchema was the latest
// storage schema. A governed migration is authorized by the root that exists
// at its source schema, so it must never require destination-schema
// representation or state to exist first. At schema 11 that admits, in
// addition to the current form, a root enrolled by the original least-scope
// boundary: it is recognisable by its pre-delegation persisted representation
// and verifies against its own persisted bytes. From schema 12 onward only
// the current semantics apply.
func (r Repository) LoadInstallationRootForSchema(ctx context.Context, bootstrapDigest string, sourceSchema int, now time.Time) (contracts.AuthorityGeneration, error) {
	if sourceSchema < 1 {
		return contracts.AuthorityGeneration{}, errors.New("installation root source schema is required")
	}
	return r.loadInstallationRoot(ctx, bootstrapDigest, sourceSchema > contracts.LegacyRootEnrollmentSchema, now)
}

func (r Repository) loadInstallationRoot(ctx context.Context, bootstrapDigest string, requireGovernanceScope bool, now time.Time) (contracts.AuthorityGeneration, error) {
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	scope, err := contracts.InstallationGovernanceScope(bootstrapDigest)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	generations, err := r.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	var active []contracts.AuthorityGeneration
	for _, generation := range generations {
		if generation.Ref != scope || generation.Principal != owner || generation.ParentRef != "" || generation.DelegatedBy != (contracts.PrincipalRef{}) || generation.ProvenanceDigest != bootstrapDigest {
			continue
		}
		if generation.Scope != scope && (requireGovernanceScope || !generation.PreDelegationForm() || generation.Scope == "") {
			continue
		}
		if _, _, err := r.loadWorkPlanBlob(ctx, authorityGenerationInvalidationNamespace, generation.Ref, generation.Version, now); err == nil {
			continue
		} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
			return contracts.AuthorityGeneration{}, err
		}
		active = append(active, generation)
	}
	if len(active) != 1 {
		return contracts.AuthorityGeneration{}, fmt.Errorf("expected exactly one active installation root, found %d", len(active))
	}
	if err := r.validateRootAuthorityLineage(ctx, active[0], bootstrapDigest, now); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	return active[0], nil
}

// LoadHistoricalInstallationRoot resolves the sole historically valid
// enrollment root of an installation that has no current canonical root: the
// exact pre-delegation record enrolled at schema 11 (ADR-090). It is the
// predecessor discovery for historical-root modernization only; it never
// classifies that root as current authority, and it fails closed once a
// current root exists or the historical root has been superseded.
func (r Repository) LoadHistoricalInstallationRoot(ctx context.Context, bootstrapDigest string, now time.Time) (contracts.AuthorityGeneration, error) {
	if _, err := r.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now); err == nil {
		return contracts.AuthorityGeneration{}, errors.New("current installation root is already established; historical-root modernization is not applicable")
	}
	root, err := r.LoadInstallationRootForSchema(ctx, bootstrapDigest, contracts.LegacyRootEnrollmentSchema, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("historical installation root: %w", err)
	}
	if !root.PreDelegationForm() || root.PredecessorRef != "" {
		return contracts.AuthorityGeneration{}, errors.New("installation root is not a historical schema-11 enrollment root")
	}
	return root, nil
}

// closedRootSuccession rebuilds the exact closed proposal for a predecessor
// under the succession kind its representation admits.
func closedRootSuccession(predecessor contracts.AuthorityGeneration, bootstrapDigest string, at time.Time) (contracts.RootAuthoritySuccessionProposal, error) {
	if predecessor.PreDelegationForm() {
		return contracts.BuildHistoricalRootModernization(predecessor, bootstrapDigest, at)
	}
	return contracts.BuildRootAuthoritySuccession(predecessor, bootstrapDigest, at)
}

// SuccessionPredecessor resolves the live predecessor a proposal must still
// bind: the current canonical root for ADR-089 repair succession, or the
// historical enrollment root for ADR-090 modernization. Kinds never cross.
func (r Repository) SuccessionPredecessor(ctx context.Context, proposal contracts.RootAuthoritySuccessionProposal, now time.Time) (contracts.AuthorityGeneration, error) {
	switch proposal.Kind {
	case contracts.HistoricalRootModernizationProposalKind:
		return r.LoadHistoricalInstallationRoot(ctx, proposal.BootstrapDigest, now)
	case contracts.RootAuthoritySuccessionProposalKind:
		return r.LoadCurrentInstallationRoot(ctx, proposal.BootstrapDigest, now)
	default:
		return contracts.AuthorityGeneration{}, fmt.Errorf("unknown root-authority succession proposal kind %q", proposal.Kind)
	}
}

func (r Repository) validateRootAuthorityLineage(ctx context.Context, root contracts.AuthorityGeneration, bootstrapDigest string, now time.Time) error {
	if root.PredecessorRef == "" {
		return nil
	}
	predecessor, err := r.LoadAuthorityGeneration(ctx, root.PredecessorRef, root.PredecessorVersion, now)
	if err != nil || predecessor.Digest != root.PredecessorDigest {
		return errors.New("root successor predecessor is unavailable or mismatched")
	}
	proposal, err := closedRootSuccession(predecessor, bootstrapDigest, root.EffectiveAt)
	if err != nil || proposal.Successor.Digest != root.Digest {
		return errors.New("root successor cannot be reconstructed from predecessor")
	}
	proposalDigest, _ := proposal.Digest()
	var storedProposal contracts.RootAuthoritySuccessionProposal
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionProposalNamespace, proposal.ID, proposal.Version, now, &storedProposal); err != nil {
		return err
	}
	if storedDigest, err := storedProposal.Digest(); err != nil || storedDigest != proposalDigest {
		return errors.New("root succession proposal mismatch")
	}
	var review contracts.RootAuthoritySuccessionReview
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionReviewNamespace, "root-authority-succession-review:"+proposalDigest, "1", now, &review); err != nil {
		return err
	}
	reviewDigest, err := review.Digest()
	if err != nil {
		return err
	}
	var decision contracts.RootAuthoritySuccessionDecision
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionDecisionNamespace, "root-authority-succession-decision:"+proposalDigest, "1", now, &decision); err != nil {
		return err
	}
	if _, err := decision.Digest(); err != nil || decision.ReviewDigest != reviewDigest || decision.SuccessorDigest != root.Digest {
		return errors.New("root succession decision mismatch")
	}
	var invalidation contracts.AuthorityGenerationInvalidation
	if err := r.loadRootSuccessionRecord(ctx, authorityGenerationInvalidationNamespace, predecessor.Ref, predecessor.Version, now, &invalidation); err != nil {
		return err
	}
	if err := invalidation.Validate(predecessor); err != nil || invalidation.Kind != "superseded" || invalidation.SupersededBy != root.Version {
		return errors.New("root predecessor supersession mismatch")
	}
	return nil
}

func (r Repository) SaveRootAuthoritySuccessionProposal(ctx context.Context, proposal contracts.RootAuthoritySuccessionProposal, now time.Time) (string, error) {
	digest, err := proposal.Digest()
	if err != nil {
		return "", err
	}
	var existing contracts.RootAuthoritySuccessionProposal
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionProposalNamespace, proposal.ID, proposal.Version, now, &existing); err == nil {
		existingDigest, digestErr := existing.Digest()
		if digestErr != nil || existingDigest != digest {
			return "", errors.New("conflicting root-authority succession proposal")
		}
		return digest, nil
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) {
		return "", err
	}
	body, _ := json.Marshal(proposal)
	if err := r.putWorkPlanBlob(ctx, state.RootAuthoritySuccessionProposalNamespace, proposal.ID, proposal.Version, body, now, nil); err != nil {
		return "", err
	}
	return digest, nil
}

func (r Repository) LoadRootAuthoritySuccessionProposalByDigest(ctx context.Context, wanted string, now time.Time) (contracts.RootAuthoritySuccessionProposal, error) {
	records, err := r.Store.ListSecureBlobs(ctx, state.RootAuthoritySuccessionProposalNamespace, now)
	if err != nil {
		return contracts.RootAuthoritySuccessionProposal{}, err
	}
	for _, record := range records {
		payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
		if err != nil {
			return contracts.RootAuthoritySuccessionProposal{}, err
		}
		var proposal contracts.RootAuthoritySuccessionProposal
		if json.Unmarshal(payload, &proposal) != nil {
			continue
		}
		digest, digestErr := proposal.Digest()
		if digestErr == nil && digest == wanted {
			return proposal, nil
		}
	}
	return contracts.RootAuthoritySuccessionProposal{}, state.ErrSecureBlobNotFound
}

func (r Repository) SaveRootAuthoritySuccessionReview(ctx context.Context, proposalDigest string, reviewer contracts.PrincipalRef, osUser, confirmation string, now time.Time) (contracts.RootAuthoritySuccessionReview, string, error) {
	proposal, err := r.LoadRootAuthoritySuccessionProposalByDigest(ctx, proposalDigest, now)
	if err != nil {
		return contracts.RootAuthoritySuccessionReview{}, "", err
	}
	if reviewer != proposal.ProposedBy || reviewer.Kind != "human" || osUser == "" || !strings.HasSuffix(proposal.Predecessor.ProvenanceRef, ":os-user:"+osUser) || confirmation != "REVIEW-ROOT-SUCCESSOR "+proposalDigest {
		return contracts.RootAuthoritySuccessionReview{}, "", errors.New("root-authority succession review requires exact authenticated owner confirmation")
	}
	current, err := r.SuccessionPredecessor(ctx, proposal, now)
	if err != nil || current.Digest != proposal.Predecessor.Digest {
		return contracts.RootAuthoritySuccessionReview{}, "", errors.New("root-authority succession predecessor is stale")
	}
	review := contracts.RootAuthoritySuccessionReview{ID: "root-authority-succession-review:" + proposalDigest, Version: "1", Kind: contracts.RootAuthoritySuccessionReviewKind, ProposalID: proposal.ID, ProposalVersion: proposal.Version, ProposalDigest: proposalDigest, ReviewedBy: reviewer, Decision: "approve", Confirmation: confirmation, ReviewedAt: now.UTC()}
	digest, err := review.Digest()
	if err != nil {
		return contracts.RootAuthoritySuccessionReview{}, "", err
	}
	var existing contracts.RootAuthoritySuccessionReview
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionReviewNamespace, review.ID, review.Version, now, &existing); err == nil {
		existingDigest, _ := existing.Digest()
		if existingDigest != digest {
			return contracts.RootAuthoritySuccessionReview{}, "", errors.New("conflicting root-authority succession review")
		}
		return existing, digest, nil
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) {
		return contracts.RootAuthoritySuccessionReview{}, "", err
	}
	body, _ := json.Marshal(review)
	if err := r.putWorkPlanBlob(ctx, state.RootAuthoritySuccessionReviewNamespace, review.ID, review.Version, body, now, nil); err != nil {
		return contracts.RootAuthoritySuccessionReview{}, "", err
	}
	return review, digest, nil
}

func (r Repository) AcceptRootAuthoritySuccession(ctx context.Context, proposalDigest, reviewDigest, bootstrapDigest, osUser, confirmation string, now time.Time) (contracts.AuthorityGeneration, contracts.RootAuthoritySuccessionDecision, error) {
	proposal, err := r.LoadRootAuthoritySuccessionProposalByDigest(ctx, proposalDigest, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, err
	}
	if proposal.BootstrapDigest != bootstrapDigest || confirmation != "ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewDigest || !strings.HasSuffix(proposal.Predecessor.ProvenanceRef, ":os-user:"+osUser) {
		return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, errors.New("root-authority succession acceptance does not authenticate the exact installation owner")
	}
	var review contracts.RootAuthoritySuccessionReview
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionReviewNamespace, "root-authority-succession-review:"+proposalDigest, "1", now, &review); err != nil {
		return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, err
	}
	actualReviewDigest, err := review.Digest()
	if err != nil || actualReviewDigest != reviewDigest {
		return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, errors.New("root-authority succession review digest mismatch")
	}
	current, err := r.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now)
	if err == nil && current.Digest == proposal.Successor.Digest {
		var existing contracts.RootAuthoritySuccessionDecision
		if loadErr := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionDecisionNamespace, "root-authority-succession-decision:"+proposalDigest, "1", now, &existing); loadErr != nil {
			return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, errors.New("root successor exists without its exact durable decision")
		}
		if digest, digestErr := existing.Digest(); digestErr != nil || digest == "" || existing.ReviewDigest != reviewDigest || existing.SuccessorDigest != current.Digest {
			return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, errors.New("root successor durable decision mismatch")
		}
		return current, existing, nil
	}
	predecessor, err := r.SuccessionPredecessor(ctx, proposal, now)
	if err != nil || predecessor.Digest != proposal.Predecessor.Digest {
		return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, errors.New("root-authority succession predecessor is stale")
	}
	decision := contracts.RootAuthoritySuccessionDecision{ID: "root-authority-succession-decision:" + proposalDigest, Version: "1", Kind: contracts.RootAuthoritySuccessionDecisionKind, BootstrapDigest: bootstrapDigest, ProposalID: proposal.ID, ProposalVersion: proposal.Version, ProposalDigest: proposalDigest, ReviewID: review.ID, ReviewVersion: review.Version, ReviewDigest: reviewDigest, PredecessorRef: proposal.Predecessor.Ref, PredecessorVersion: proposal.Predecessor.Version, PredecessorDigest: proposal.Predecessor.Digest, SuccessorRef: proposal.Successor.Ref, SuccessorVersion: proposal.Successor.Version, SuccessorDigest: proposal.Successor.Digest, DecidedBy: proposal.ProposedBy, Decision: "approve", Confirmation: confirmation, DecidedAt: now.UTC()}
	if _, err := decision.Digest(); err != nil {
		return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, err
	}
	err = r.Store.PutRootAuthoritySuccessor(ctx, state.RootAuthoritySuccessionWrite{Proposal: proposal, Review: review, Decision: decision, Crypto: r.Crypto, KeyRef: r.KeyRef, Profile: r.Profile, Sensitivity: r.Sensitivity, CreatedAt: now})
	if err != nil {
		return contracts.AuthorityGeneration{}, contracts.RootAuthoritySuccessionDecision{}, err
	}
	return proposal.Successor, decision, nil
}

func (r Repository) SaveInstallationRepairAuthorityRequest(ctx context.Context, operation string, expiresAt, now time.Time) (contracts.AuthorityRequest, string, error) {
	root, err := r.LoadCurrentInstallationRoot(ctx, r.InstallationDigest, now)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	if !slices.Contains(root.Authorities, operation) {
		return contracts.AuthorityRequest{}, "", errors.New("current root does not possess requested installation-repair authority")
	}
	predecessor, err := r.loadRootPredecessor(ctx, root, now)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	proposal, err := contracts.BuildRootAuthoritySuccession(predecessor, r.InstallationDigest, root.EffectiveAt)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	proposalDigest, _ := proposal.Digest()
	var decision contracts.RootAuthoritySuccessionDecision
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionDecisionNamespace, "root-authority-succession-decision:"+proposalDigest, "1", now, &decision); err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	decisionDigest, err := decision.Digest()
	if err != nil || decision.SuccessorRef != root.Ref || decision.SuccessorVersion != root.Version || decision.SuccessorDigest != root.Digest || decision.BootstrapDigest != r.InstallationDigest || decision.ProposalDigest != proposalDigest {
		if err == nil {
			err = errors.New("current root succession decision does not bind exact installation lineage")
		}
		return contracts.AuthorityRequest{}, "", err
	}
	repair := &contracts.InstallationRepairAuthorityRequest{BootstrapDigest: r.InstallationDigest, RootRef: root.Ref, RootVersion: root.Version, RootDigest: root.Digest, SuccessionDecisionDigest: decisionDigest, Operation: operation, ExpiresAt: expiresAt.UTC()}
	request := contracts.AuthorityRequest{ID: "installation-repair-request:" + operation + ":" + root.Digest + ":" + expiresAt.UTC().Format(time.RFC3339Nano), Version: "1", RequestedAuthority: operation, RequestedScope: root.Scope, Reason: "authorize exact ADR-088 installation repair transition", Status: contracts.AuthorityRequestPending, InstallationDigest: r.InstallationDigest, Repair: repair}
	digest, err := request.DigestAt(now)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	stored, err := r.SaveAuthorityRequest(ctx, request, now, &expiresAt)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	if stored != digest {
		return contracts.AuthorityRequest{}, "", errors.New("installation-repair request persistence digest mismatch")
	}
	return request, digest, nil
}

func (r Repository) ApproveInstallationRepairAuthorityRequest(ctx context.Context, requestDigest string, owner contracts.PrincipalRef, confirmation string, now time.Time) (contracts.AuthorityDecision, error) {
	request, err := r.LoadAuthorityRequestByDigest(ctx, requestDigest, now)
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	if request.Repair == nil || confirmation != "APPROVE-INSTALLATION-REPAIR "+requestDigest {
		return contracts.AuthorityDecision{}, errors.New("installation-repair approval does not bind exact request")
	}
	root, err := r.LoadCurrentInstallationRoot(ctx, r.InstallationDigest, now)
	if err != nil || root.Ref != request.Repair.RootRef || root.Version != request.Repair.RootVersion || root.Digest != request.Repair.RootDigest || root.Principal != owner || !slices.Contains(root.Authorities, request.RequestedAuthority) {
		return contracts.AuthorityDecision{}, errors.New("installation-repair request root is stale or unauthorized")
	}
	predecessor, err := r.loadRootPredecessor(ctx, root, now)
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	proposal, err := contracts.BuildRootAuthoritySuccession(predecessor, r.InstallationDigest, root.EffectiveAt)
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	proposalDigest, err := proposal.Digest()
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	var succession contracts.RootAuthoritySuccessionDecision
	if err := r.loadRootSuccessionRecord(ctx, state.RootAuthoritySuccessionDecisionNamespace, "root-authority-succession-decision:"+proposalDigest, "1", now, &succession); err != nil {
		return contracts.AuthorityDecision{}, err
	}
	successionDigest, err := succession.Digest()
	if err != nil || successionDigest != request.Repair.SuccessionDecisionDigest || succession.SuccessorDigest != root.Digest {
		return contracts.AuthorityDecision{}, errors.New("installation-repair request succession decision is unavailable or mismatched")
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: owner, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: request.Repair.SuccessionDecisionDigest, IssuedAt: now.UTC(), ExpiresAt: &request.Repair.ExpiresAt}
	if err := r.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, now, &request.Repair.ExpiresAt); err != nil {
		return contracts.AuthorityDecision{}, err
	}
	return decision, nil
}

func (r Repository) loadRootPredecessor(ctx context.Context, root contracts.AuthorityGeneration, now time.Time) (contracts.AuthorityGeneration, error) {
	if root.PredecessorRef == "" || root.PredecessorVersion == "" || root.PredecessorDigest == "" {
		return contracts.AuthorityGeneration{}, errors.New("current root has no governed predecessor lineage")
	}
	predecessor, err := r.LoadAuthorityGeneration(ctx, root.PredecessorRef, root.PredecessorVersion, now)
	if err != nil || predecessor.Digest != root.PredecessorDigest {
		return contracts.AuthorityGeneration{}, errors.New("current root predecessor is unavailable or mismatched")
	}
	return predecessor, nil
}

func (r Repository) loadRootSuccessionRecord(ctx context.Context, namespace, id, version string, now time.Time, out any) error {
	payload, record, err := r.loadWorkPlanBlob(ctx, namespace, id, version, now)
	if err != nil {
		return err
	}
	if record.ObjectDigest == "" || json.Unmarshal(payload, out) != nil {
		return errors.New("invalid root-authority succession record")
	}
	return nil
}
