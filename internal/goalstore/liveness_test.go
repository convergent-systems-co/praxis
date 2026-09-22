package goalstore

import (
	"context"
	"errors"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// I12 (deletion-monotonicity) at the store boundary. Each case performs a
// keyless SQL DELETE against the SQLite file (no Crypto, no KeyRef) and asserts
// the deletion never widens what a decision or generation permits.

type approvedDecision struct {
	repo       Repository
	store      *state.Store
	request    contracts.AuthorityRequest
	decision   contracts.AuthorityDecision
	generation contracts.AuthorityGeneration
}

func approveUnprotectedAcceptance(t *testing.T) approvedDecision {
	t.Helper()
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	return approveUnprotectedAcceptanceOn(t, repo, store)
}

func approveUnprotectedAcceptanceOn(t *testing.T, repo Repository, store *state.Store) approvedDecision {
	t.Helper()
	ctx := context.Background()
	proposal, _, _ := workPlanProposalFixture()
	proposalDigest, err := repo.SaveWorkPlanProposal(ctx, proposal, "1", time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	review := workPlanReviewFixture(proposal)
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", review, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	request := contracts.AuthorityRequest{ID: "request-live-1", Version: "1", BaselineID: proposal.GoalID, BaselineVersion: proposal.GoalVersion, BaselineDigest: proposal.BaselineDigest, ProposalID: proposal.ID, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewRef: review.ReviewRef, ReviewVersion: "1", ReviewDigest: review.ReviewDigest, RequestedAuthority: "workplan.accept", RequestedScope: "goal:goal-1/proposal-1", Reason: "material decomposition requires explicit authority", Status: contracts.AuthorityRequestPending}
	if _, err := repo.SaveAuthorityRequest(ctx, request, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	generation := contracts.AuthorityGeneration{Ref: "policy:live", Version: "1", Principal: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, Scope: request.RequestedScope, Authorities: []string{contracts.GovernedWorkPlanAccept}, ProvenanceRef: "policy:live", ProvenanceDigest: "sha256:policy-source", State: contracts.AuthorityGenerationActive, EffectiveAt: time.Now().UTC()}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generation, generation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	requestDigest, _ := request.Digest()
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision-live-1", DecisionVersion: "1", DecidedBy: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, AuthorityRef: generation.Ref, AuthorityVersion: generation.Version, AuthorityGenerationDigest: generation.Digest, GrantedScope: request.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: "sha256:operator", IssuedAt: time.Now().UTC()}
	if err := repo.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	return approvedDecision{repo: repo, store: store, request: request, decision: decision, generation: generation}
}

func keylessDelete(t *testing.T, store *state.Store, namespace, id, version string) {
	t.Helper()
	result, err := store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, id, version)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("keyless delete of %s/%s/%s: rows=%d err=%v", namespace, id, version, n, err)
	}
}

func TestRevocationRowDeletionDoesNotRestoreAUnprotectedDecision(t *testing.T) {
	a := approveUnprotectedAcceptance(t)
	ctx := context.Background()
	if _, err := a.repo.LoadAuthorityDecision(ctx, a.request.ID, a.request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("control: an approved decision must load: %v", err)
	}
	digest, _ := a.decision.Digest()
	revocation := contracts.AuthorityRevocation{RequestID: a.request.ID, RequestVersion: a.request.Version, DecisionRef: a.decision.DecisionRef, DecisionVersion: a.decision.DecisionVersion, DecisionDigest: digest, RevocationRef: "revocation-live-1", RevocationVersion: "1", RevokedBy: a.decision.DecidedBy, AuthorityDigest: "sha256:operator-revocation", EffectiveAt: time.Now().UTC(), Reason: "withdrawn"}
	if err := a.repo.SaveAuthorityRevocation(ctx, a.request.ID, a.request.Version, revocation, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	keylessDelete(t, a.store, state.AuthorityRevocationNamespace, a.request.ID, a.request.Version)
	if _, err := a.repo.LoadAuthorityDecision(ctx, a.request.ID, a.request.Version, time.Now().UTC()); !errors.Is(err, ErrAuthorityDecisionNotLive) || !errors.Is(err, ErrAuthorityDecisionRevoked) {
		t.Fatalf("deleting the revocation row restored a revoked decision: %v", err)
	}
	_, _, accepted := workPlanProposalFixture()
	if _, err := a.repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, a.request.ID, a.request.Version, accepted, "acceptance-after-delete", "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("a decision revived by revocation-row deletion was consumed into a new acceptance")
	}
	if err := a.repo.SaveAuthorityDecision(ctx, a.request.ID, a.request.Version, a.decision, time.Now().UTC(), nil); !errors.Is(err, ErrAuthorityDecisionNotLive) {
		t.Fatalf("resubmitting the identical decision resurrected it: %v", err)
	}
	if _, err := a.repo.LoadAuthorityDecisionEvidence(ctx, a.request.ID, a.request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("historical evidence must survive: %v", err)
	}
}

func TestDecisionLivenessRowDeletionFailsClosed(t *testing.T) {
	a := approveUnprotectedAcceptance(t)
	ctx := context.Background()
	keylessDelete(t, a.store, state.AuthorityDecisionLiveNamespace, a.request.ID, a.request.Version)
	if _, err := a.repo.LoadAuthorityDecision(ctx, a.request.ID, a.request.Version, time.Now().UTC()); !errors.Is(err, ErrAuthorityDecisionNotLive) {
		t.Fatalf("a decision without its liveness record granted authority: %v", err)
	}
	_, _, accepted := workPlanProposalFixture()
	if _, err := a.repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, a.request.ID, a.request.Version, accepted, "acceptance-no-live", "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("a decision without liveness was consumed into an acceptance")
	}
}

func TestGenerationInvalidationRowDeletionDoesNotRestoreTheGeneration(t *testing.T) {
	a := approveUnprotectedAcceptance(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := a.repo.ValidateAuthorityGeneration(ctx, a.decision, now); err != nil {
		t.Fatalf("control: a live generation must validate: %v", err)
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: a.generation.Ref, Version: a.generation.Version, GenerationDigest: a.generation.Digest, InvalidationRef: "invalidate-live-1", InvalidationVersion: "1", Kind: "superseded", SupersededBy: "2", InvalidatedBy: a.generation.Principal, EffectiveAt: now, Reason: "advanced"}
	if err := a.repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, now, nil); err != nil {
		t.Fatal(err)
	}
	keylessDelete(t, a.store, state.AuthorityGenerationInvalidationNamespace, a.generation.Ref, a.generation.Version)
	if err := a.repo.ValidateAuthorityGeneration(ctx, a.decision, now); err == nil {
		t.Fatal("deleting the invalidation row restored a superseded generation")
	}
	// The in-transaction fence used by every authority-bound write refuses too.
	other := a.request
	other.ID = "request-live-2"
	other.Version = "1"
	if _, err := a.repo.SaveAuthorityRequest(ctx, other, now, nil); err != nil {
		t.Fatal(err)
	}
	otherDigest, _ := other.Digest()
	next := a.decision
	next.RequestID, next.RequestDigest, next.DecisionRef = other.ID, otherDigest, "decision-live-2"
	if err := a.repo.SaveAuthorityDecision(ctx, other.ID, other.Version, next, now, nil); err == nil {
		t.Fatal("a decision was admitted under a generation whose invalidation row was deleted")
	}
}

func TestGenerationLivenessRowDeletionFailsClosed(t *testing.T) {
	a := approveUnprotectedAcceptance(t)
	keylessDelete(t, a.store, state.AuthorityGenerationLiveNamespace, a.generation.Ref, a.generation.Version)
	if err := a.repo.ValidateAuthorityGeneration(context.Background(), a.decision, time.Now().UTC()); !errors.Is(err, ErrLivenessMissing) {
		t.Fatalf("a generation without its liveness record validated: %v", err)
	}
}

func TestDeleteLivenessRecordRefusesEveryOtherNamespace(t *testing.T) {
	a := approveUnprotectedAcceptance(t)
	for _, namespace := range []string{"authority_decision", "authority_generation", "authority_revocation", "goal_baseline", "goal_safety_classification", ""} {
		if err := a.store.DeleteLivenessRecord(context.Background(), namespace, "x", "1"); !errors.Is(err, state.ErrNotLivenessNamespace) {
			t.Fatalf("%q: the retire API deleted outside the liveness namespaces: %v", namespace, err)
		}
	}
}

// N9 store half: with the classification row removed, a Goal stays classified
// while any surviving safety-bearing proposal names it, an unrelated
// historical record does not classify it, and an unreadable candidate fails
// closed rather than reading as "never classified".
func TestClassificationIsDerivedFromSurvivingSafetyBearingProposals(t *testing.T) {
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Now().UTC()
	put := func(id, body string) {
		t.Helper()
		if err := repo.putWorkPlanBlob(ctx, workPlanProposalNamespace, id, "1", []byte(body), now, nil); err != nil {
			t.Fatal(err)
		}
	}
	put("unrelated-legacy", `{"id":"unrelated-legacy","goal_id":"goal-other","obsolete_field":true}`)
	put("unrelated-safety", `{"id":"unrelated-safety","goal_id":"goal-other","safety":{"kernel_version":"`+contracts.WorkPlanSafetyKernelVersion+`"}}`)
	if classified, err := repo.GoalSafetyClassified(ctx, "goal-1"); err != nil || classified {
		t.Fatalf("an unrelated record classified the Goal: %v %v", classified, err)
	}
	put("wp-safety", `{"id":"wp-safety","goal_id":"goal-1","safety":{"kernel_version":"`+contracts.WorkPlanSafetyKernelVersion+`"}}`)
	kernel, classified, err := repo.GoalSafetyKernel(ctx, "goal-1")
	if err != nil || !classified || kernel != contracts.WorkPlanSafetyKernelVersion {
		t.Fatalf("classification was not derived from the surviving safety-bearing proposal: %q %v %v", kernel, classified, err)
	}
	put("wp-conflict", `{"id":"wp-conflict","goal_id":"goal-1","safety":{"kernel_version":"another-kernel/9"}}`)
	if _, _, err := repo.GoalSafetyKernel(ctx, "goal-1"); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("conflicting kernel evidence must fail closed: %v", err)
	}
	keylessDelete(t, store, workPlanProposalNamespace, "wp-conflict", "1")
	if _, err := store.DB().Exec(`UPDATE secure_blobs SET envelope_json=json_set(envelope_json,'$.Ciphertext','AAAA') WHERE namespace=? AND object_id='unrelated-legacy'`, workPlanProposalNamespace); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.GoalSafetyKernel(ctx, "goal-1"); err == nil {
		t.Fatal("an unreadable candidate row must fail closed, not read as unclassified")
	}
}

// I12 at the root: after a governed succession, deleting the successor's
// generation row and the predecessor's supersession record (two keyless
// deletions) must not leave the superseded predecessor as the "sole active
// root". Succession retired the predecessor's liveness record atomically.
func TestRootSuccessionRollbackByRowDeletionDoesNotReviveThePredecessor(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute)
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	predecessor := rootSuccessionFixture(t, repo, now)
	successor, _, _, _ := acceptedRootSuccession(t, repo, predecessor, now)
	ctx := context.Background()
	later := now.Add(10 * time.Second)
	if current, err := repo.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, later); err != nil || current.Digest != successor.Digest {
		t.Fatalf("control: the successor must be the current root: %v", err)
	}
	keylessDelete(t, store, state.AuthorityGenerationNamespace, successor.Ref, successor.Version)
	keylessDelete(t, store, state.AuthorityGenerationInvalidationNamespace, predecessor.Ref, predecessor.Version)
	if current, err := repo.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, later); err == nil {
		t.Fatalf("rolling the root back by row deletion revived %s/%s", current.Ref, current.Version)
	}
}

// Deleting only the current root's liveness record leaves no live root.
func TestRootLivenessRowDeletionLeavesNoCurrentRoot(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute)
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	root := rootSuccessionFixture(t, repo, now)
	ctx := context.Background()
	if _, err := repo.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, now); err != nil {
		t.Fatalf("control: %v", err)
	}
	keylessDelete(t, store, state.AuthorityGenerationLiveNamespace, root.Ref, root.Version)
	if _, err := repo.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, now); !errors.Is(err, ErrLivenessMissing) {
		t.Fatalf("a root without its liveness record was current: %v", err)
	}
}

// I12, lineage walk: a delegated child whose liveness record is gone is not in
// force even though its parent chain and the current root are intact and the
// child has no invalidation record.
func TestLineageWalkRefusesAChildWithoutItsLivenessRecord(t *testing.T) {
	repo, _, child, _, _, _ := routingIssuanceFixture(t)
	ctx := context.Background()
	if _, err := repo.ValidateAuthorityGenerationLineage(ctx, child.Ref, child.Version, child.Digest, repo.InstallationDigest, time.Now().UTC()); err != nil {
		t.Fatalf("control: %v", err)
	}
	keylessDelete(t, repo.Store, state.AuthorityGenerationLiveNamespace, child.Ref, child.Version)
	if _, err := repo.ValidateAuthorityGenerationLineage(ctx, child.Ref, child.Version, child.Digest, repo.InstallationDigest, time.Now().UTC()); !errors.Is(err, ErrLivenessMissing) {
		t.Fatalf("a child without liveness validated: %v", err)
	}
}

// I12, publication consumers: current-fence checks require positive liveness.
func TestPublicationFencesRequireLivenessRecords(t *testing.T) {
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Now().UTC()
	seal := func(namespace, id string) {
		t.Helper()
		record, err := state.SealedLivenessRecord(ctx, repo.Crypto, repo.KeyRef, repo.Profile, repo.Sensitivity, namespace, id, "1", "sha256:x", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutSecureBlob(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CheckGoalsPublicationInvalidation(ctx, "gen", "1", "req", "1", now); err == nil {
		t.Fatal("a fence with no liveness records passed")
	}
	seal(state.AuthorityGenerationLiveNamespace, "gen")
	if err := repo.CheckGoalsPublicationInvalidation(ctx, "gen", "1", "req", "1", now); err == nil {
		t.Fatal("a fence without the decision liveness record passed")
	}
	seal(state.AuthorityDecisionLiveNamespace, "req")
	if err := repo.CheckGoalsPublicationInvalidation(ctx, "gen", "1", "req", "1", now); err != nil {
		t.Fatalf("control: live generation and decision must pass: %v", err)
	}
	keylessDelete(t, store, state.AuthorityGenerationLiveNamespace, "gen", "1")
	if err := repo.CheckGoalsPublicationInvalidation(ctx, "gen", "1", "", "", now); err == nil {
		t.Fatal("a generation without liveness passed the publication fence")
	}
}

// I12, recovery admission: the in-transaction re-check requires the decision
// liveness record and the liveness record of every generation it rests on.
func TestRecoveryAdmissionRecheckRequiresLivenessRecords(t *testing.T) {
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	seal := func(namespace, id string) {
		t.Helper()
		record, err := state.SealedLivenessRecord(ctx, repo.Crypto, repo.KeyRef, repo.Profile, repo.Sensitivity, namespace, id, "1", "sha256:x", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutSecureBlob(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	auth := contracts.PackagePublishAuthorization{}
	auth.Request.ID, auth.Request.Version = "req", "1"
	auth.Decision.ExpiresAt = &expires
	auth.Generation.Ref, auth.Generation.Version, auth.Generation.ExpiresAt = "child", "1", &expires
	auth.Generation.ParentRef, auth.Generation.ParentVersion = "parent", "1"
	check := func() error {
		tx, err := store.DB().BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return repo.RevalidateGoalsPublicationRecoveryAuthorizationTx(ctx, tx, auth, now)
	}
	if err := check(); err == nil {
		t.Fatal("re-check with no liveness records passed")
	}
	seal(state.AuthorityDecisionLiveNamespace, "req")
	if err := check(); err == nil {
		t.Fatal("re-check without generation liveness passed")
	}
	seal(state.AuthorityGenerationLiveNamespace, "child")
	if err := check(); err == nil {
		t.Fatal("re-check without the parent generation liveness passed")
	}
	seal(state.AuthorityGenerationLiveNamespace, "parent")
	if err := check(); err != nil {
		t.Fatalf("control: fully live authorization must pass: %v", err)
	}
	keylessDelete(t, store, state.AuthorityDecisionLiveNamespace, "req", "1")
	if err := check(); err == nil {
		t.Fatal("re-check after decision liveness deletion passed")
	}
}
