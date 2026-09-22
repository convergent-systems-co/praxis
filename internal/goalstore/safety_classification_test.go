package goalstore

import (
	"context"
	"errors"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// I9. Once a Goal identity is classified safety-bearing in authenticated state,
// every admission surface refuses a legacy (binding-less) plan for it, at its
// own layer. The legacy proposal, review and acceptance are persisted BEFORE the
// classification so that each later layer is exercised on its own rather than
// being masked by the proposal layer refusing first.
func TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	source, err := repo.Save(ctx, goalFixture(), time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	proposal, decision, accepted := workPlanProposalFixture()
	proposal.GoalID, proposal.GoalVersion = source.ID, source.Version
	proposal.BaselineDigest, decision.BaselineDigest, accepted.BaselineDigest = source.Digest, source.Digest, source.Digest
	decision.ProposalDigest, err = proposal.Digest()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := repo.SaveWorkPlanProposal(ctx, proposal, "1", now, nil); err != nil {
		t.Fatalf("control: a legacy proposal for an unclassified Goal must persist: %v", err)
	}
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", workPlanReviewFixture(proposal), "1", now, nil); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.SaveAcceptedWorkPlan(ctx, proposal.ID, "1", accepted, decision, "1", now, nil)
	if err != nil {
		t.Fatalf("control: legacy acceptance for an unclassified Goal must persist: %v", err)
	}
	if classified, err := repo.GoalSafetyClassified(ctx, source.ID); err != nil || classified {
		t.Fatalf("a legacy Goal was classified safety-bearing: %v %v", classified, err)
	}

	binding := &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion}
	if err := repo.markGoalSafetyBearing(ctx, source.ID, binding, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.markGoalSafetyBearing(ctx, source.ID, binding, now); err != nil {
		t.Fatalf("classification must be idempotent: %v", err)
	}
	if kernel, ok, err := repo.GoalSafetyKernel(ctx, source.ID); err != nil || !ok || kernel != contracts.WorkPlanSafetyKernelVersion {
		t.Fatalf("classification did not persist: %q %v %v", kernel, ok, err)
	}
	if err := repo.markGoalSafetyBearing(ctx, source.ID, &contracts.WorkPlanSafetyBinding{KernelVersion: "another-kernel/9"}, now); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("a Goal was re-classified under a different kernel: %v", err)
	}

	other, _, _ := workPlanProposalFixture()
	other.ID, other.GoalID, other.GoalVersion, other.BaselineDigest = "proposal-2", source.ID, source.Version, source.Digest
	if _, err := repo.SaveWorkPlanProposal(ctx, other, "1", now, nil); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("proposal layer admitted a legacy proposal: %v", err)
	}
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", workPlanReviewFixture(proposal), "2", now, nil); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("review layer admitted a legacy review: %v", err)
	}
	if _, err := repo.SaveAcceptedWorkPlan(ctx, proposal.ID, "1", accepted, decision, "2", now, nil); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("legacy acceptance layer admitted a legacy plan: %v", err)
	}
	if _, err := repo.AttachAcceptedWorkPlan(ctx, source.ID, source.Version, source.Digest, decision.AcceptanceRef, "1", "2", now, nil); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("attachment layer attached a legacy plan: %v", err)
	}
	legacyGeneration := source
	legacyGeneration.Version, legacyGeneration.Digest, legacyGeneration.PredecessorDigest = "3", "", source.Digest
	legacyGeneration.WorkPlan = &plan
	if _, err := repo.Save(ctx, legacyGeneration, now, nil); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("generic Save gave a classified Goal a legacy plan: %v", err)
	}
	// A plan-less generation is not a plan and may be persisted; it is refused
	// where it would be driven, not here.
	planless := source
	planless.Version, planless.Digest, planless.PredecessorDigest = "4", "", source.Digest
	planless.WorkPlan = nil
	if _, err := repo.Save(ctx, planless, now, nil); err != nil {
		t.Fatalf("a plan-less successor of a classified Goal must remain persistable: %v", err)
	}
}

type passingActivation struct{}

func (passingActivation) Verify(context.Context, contracts.WorkPlanSafetyBinding) error { return nil }

// I11 store half: a completion is sealed under the installation storage key
// only for a safety-bearing generation under verified activation, and only the
// exact sealed bytes resolve.
func TestCompletionSealsAuthenticateExactBytesOnly(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Now().UTC()
	safe := source(t, repo)
	safe.WorkPlan = &contracts.WorkPlan{Safety: &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion, ActivationManifestDigest: "sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"}}
	payload := []byte(`{"unit_id":"one"}`)

	if _, err := repo.SealCompletion(ctx, source(t, repo), payload, now); err == nil {
		t.Fatal("a legacy generation sealed a completion")
	}
	if _, err := repo.SealCompletion(ctx, safe, payload, now); !errors.Is(err, ErrSafetyActivationRequired) {
		t.Fatalf("sealing without a verified activation must fail closed: %v", err)
	}
	repo.SafetyActivation = passingActivation{}
	digest, err := repo.SealCompletion(ctx, safe, payload, now)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := repo.SealCompletion(ctx, safe, payload, now); err != nil || again != digest {
		t.Fatalf("sealing must be idempotent: %v %v", again, err)
	}
	got, err := repo.LoadSealedCompletion(ctx, safe, digest, now)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("sealed bytes did not resolve: %q %v", got, err)
	}
	if _, err := repo.LoadSealedCompletion(ctx, safe, "sha256:"+"0000000000000000000000000000000000000000000000000000000000000000", now); !errors.Is(err, contracts.ErrCompletionUnauthenticated) {
		t.Fatalf("an unsealed digest resolved: %v", err)
	}
	// A seal is scoped to its Goal generation.
	other := safe
	other.Version = "9"
	if _, err := repo.LoadSealedCompletion(ctx, other, digest, now); !errors.Is(err, contracts.ErrCompletionUnauthenticated) {
		t.Fatalf("a seal crossed Goal generations: %v", err)
	}
}

func source(t *testing.T, repo Repository) goals.GoalBaseline {
	t.Helper()
	return goalFixture()
}

// The authority-backed acceptance bridge is its own admission layer: with the
// legacy proposal, review, request and human decision all persisted BEFORE the
// Goal is classified, only the bridge's own classification check can refuse the
// acceptance afterwards.
func TestSafetyClassificationRefusesTheAuthorityBackedAcceptanceBridgeOnItsOwn(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	proposal, _, accepted := workPlanProposalFixture()
	proposalDigest, err := repo.SaveWorkPlanProposal(ctx, proposal, "1", time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	review := workPlanReviewFixture(proposal)
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", review, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	request := contracts.AuthorityRequest{ID: "request-accept-1", Version: "1", BaselineID: proposal.GoalID, BaselineVersion: proposal.GoalVersion, BaselineDigest: proposal.BaselineDigest, ProposalID: proposal.ID, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewRef: review.ReviewRef, ReviewVersion: "1", ReviewDigest: review.ReviewDigest, RequestedAuthority: "workplan.accept", RequestedScope: "goal:goal-1/proposal-1", Reason: "material decomposition requires explicit authority", Status: contracts.AuthorityRequestPending}
	if _, err := repo.SaveAuthorityRequest(ctx, request, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	generation := contracts.AuthorityGeneration{Ref: "policy:goal-acceptance", Version: "7", Principal: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, Scope: request.RequestedScope, Authorities: []string{contracts.GovernedWorkPlanAccept}, ProvenanceRef: "policy:goal-acceptance", ProvenanceDigest: "sha256:policy-source", State: contracts.AuthorityGenerationActive, EffectiveAt: time.Now().UTC()}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generation, generation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	requestDigest, _ := request.Digest()
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision-accept-1", DecisionVersion: "1", DecidedBy: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, AuthorityRef: "policy:goal-acceptance", AuthorityVersion: "7", AuthorityGenerationDigest: generation.Digest, GrantedScope: request.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: "sha256:operator", IssuedAt: time.Now().UTC()}
	if err := repo.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.markGoalSafetyBearing(ctx, proposal.GoalID, &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, request.ID, request.Version, accepted, "acceptance-authority-1", "1", time.Now().UTC(), nil); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("the acceptance bridge admitted a legacy plan for a classified Goal: %v", err)
	}
}

// The consistency predicate refuses a binding for another kernel version, not
// only a missing binding.
func TestSafetyConsistencyRefusesAnotherKernelVersion(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	if err := repo.requireSafetyConsistent(ctx, "goal-1", nil); err != nil {
		t.Fatalf("control: an unclassified Goal must accept a legacy artifact: %v", err)
	}
	if err := repo.markGoalSafetyBearing(ctx, "goal-1", &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := repo.requireSafetyConsistent(ctx, "goal-1", &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion}); err != nil {
		t.Fatalf("control: the classified kernel must be accepted: %v", err)
	}
	if err := repo.requireSafetyConsistent(ctx, "goal-1", &contracts.WorkPlanSafetyBinding{KernelVersion: "another-kernel/9"}); !errors.Is(err, ErrSafetyDowngrade) {
		t.Fatalf("a binding for a different kernel version was consistent: %v", err)
	}
}

// A seal row whose bytes are not the bytes its key names (a swapped payload
// under an existing key) never authenticates anything.
func TestSealedCompletionWhoseBytesDoNotMatchItsKeyIsRefused(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Now().UTC()
	safe := source(t, repo)
	safe.WorkPlan = &contracts.WorkPlan{Safety: &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion}}
	genuine := []byte(`{"unit_id":"one"}`)
	digest := payloadDigest(genuine)
	swapped := []byte(`{"unit_id":"swapped"}`)
	if err := repo.putWorkPlanBlob(ctx, completionSealNamespace, completionSealID(safe), digest, swapped, now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadSealedCompletion(ctx, safe, digest, now); !errors.Is(err, contracts.ErrCompletionUnauthenticated) {
		t.Fatalf("a seal whose bytes differ from its key authenticated a completion: %v", err)
	}
}
