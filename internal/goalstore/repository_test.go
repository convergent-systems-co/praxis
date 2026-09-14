package goalstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type wrapper struct {
	caps praxiscrypto.Capabilities
	key  []byte
}

func (w *wrapper) Capabilities(context.Context, string) (praxiscrypto.Capabilities, error) {
	return w.caps, nil
}
func (w *wrapper) Wrap(_ context.Context, keyRef string, profile contracts.CryptoProfile, key []byte) (praxiscrypto.WrappedKey, error) {
	w.key = append([]byte(nil), key...)
	return praxiscrypto.WrappedKey{Ciphertext: append([]byte(nil), key...), SuiteID: "test", KeyRef: keyRef, KeyVersion: "1", SelectedProfile: profile}, nil
}
func (w *wrapper) Unwrap(_ context.Context, wrapped praxiscrypto.WrappedKey) ([]byte, error) {
	return append([]byte(nil), wrapped.Ciphertext...), nil
}

func repoFixture(t *testing.T, caps praxiscrypto.Capabilities, profile contracts.CryptoProfile) (Repository, *state.Store) {
	t.Helper()
	return repoFixtureAt(t, filepath.Join(t.TempDir(), "praxis.db"), caps, profile)
}

func repoFixtureAt(t *testing.T, path string, caps praxiscrypto.Capabilities, profile contracts.CryptoProfile) (Repository, *state.Store) {
	t.Helper()
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := state.New(db)
	return Repository{Store: store, Crypto: praxiscrypto.EnvelopeService{Wrapper: &wrapper{caps: caps}}, KeyRef: "key:goals", Profile: profile, Sensitivity: state.SensitivityConfidential}, store
}

func goalFixture() goals.GoalBaseline {
	return goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "Help me publish article two", RefinedOutcome: "Produce a publishable second AI-safety article with reusable research context", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationDelegated, Decisions: []goals.Decision{{ID: "d1", Statement: "reuse prior research baseline", Status: goals.DecisionResolved, Recommendation: "reuse", Rationale: "avoids repeated discovery", Reversible: true, AutoAccepted: true}}}
}

func TestRepositoryEncryptedRoundTrip(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Now().UTC()
	saved, err := repo.Save(context.Background(), goalFixture(), now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Digest == "" {
		t.Fatal("saved baseline must have digest")
	}
	loaded, err := repo.Load(context.Background(), saved.ID, saved.Version, now)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != saved.Digest || loaded.OriginalIntent != saved.OriginalIntent {
		t.Fatalf("round trip mismatch saved=%+v loaded=%+v", saved, loaded)
	}
}

func TestRepositoryPQRequiredFailsWithoutPQProvider(t *testing.T) {
	repo, store := repoFixture(t, praxiscrypto.Capabilities{Classical: true}, contracts.CryptoPQRequired)
	_, err := repo.Save(context.Background(), goalFixture(), time.Now().UTC(), nil)
	if err == nil {
		t.Fatal("pq-required Goal Baseline must not persist through classical-only wrapper")
	}
	if _, err := store.GetSecureBlob(context.Background(), baselineNamespace, "goal-1", "1", time.Now().UTC()); err != state.ErrSecureBlobNotFound {
		t.Fatalf("failed encryption must not leave persisted record, got %v", err)
	}
}

func TestRepositoryRejectsBaselineDigestMutationBeforePersistence(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	b := goalFixture()
	d, _ := b.ComputeDigest()
	b.Digest = d
	b.RefinedOutcome = "changed after digest"
	if _, err := repo.Save(context.Background(), b, time.Now().UTC(), nil); err == nil {
		t.Fatal("mutated baseline must not persist")
	}
}

func TestRepositoryHonorsRetentionExpiry(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	saved, err := repo.Save(context.Background(), goalFixture(), now, &expiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Load(context.Background(), saved.ID, saved.Version, expiry); err != state.ErrSecureBlobExpired {
		t.Fatalf("expected expired baseline, got %v", err)
	}
}

func TestRepositorySessionCheckpointSurvivesRestartAndResumes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	ctx := context.Background()
	keyWrapper := &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := state.New(db)
	repo := Repository{Store: store, Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	session, err := goals.NewSession("session-1", "Help me make a reliable research plan")
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"captured", "rigorous", "ready", "calibrate", "review_all"} {
		if err := session.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveSession(context.Background(), session, "5", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}

	// Close the provider and reopen the same durable database, as a process
	// restart would. The key wrapper is retained only as the test's simulated
	// external key service; plaintext session state is not retained in memory.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedDB, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopened := Repository{Store: state.New(reopenedDB), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	resumed, err := reopened.LoadSession(ctx, session.ID, "5", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Stage != goals.StageDecide || resumed.StageOutcomes[goals.StageCalibrate] != "review_all" {
		t.Fatalf("checkpoint did not preserve resumable responsibility state: %+v", resumed)
	}
	if err := resumed.Advance("ready"); err != nil {
		t.Fatal(err)
	}
}

func TestRepositorySessionCheckpointVersionsAreImmutable(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	session, err := goals.NewSession("session-immutable", "goal")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSession(context.Background(), session, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSession(context.Background(), session, "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("reusing a checkpoint version must not replace the immutable snapshot")
	}
}

func workPlanProposalFixture() (contracts.WorkPlanProposal, contracts.WorkPlanAcceptance, contracts.WorkPlan) {
	proposal := contracts.WorkPlanProposal{
		ID: "proposal-1", GoalID: "goal-1", GoalVersion: "1", BaselineDigest: "sha256:baseline",
		ProposedBy: contracts.PrincipalRef{ID: "planner-model", Kind: "model"}, ProposerGeneration: "planner-generation-1",
		Candidates: []contracts.WorkCandidate{{ID: "unit-1", SourceRef: "model:proposal", SourceDigest: "sha256:model", Provenance: contracts.ProvenanceModelProposal, Requirements: []contracts.RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}},
	}
	digest, _ := proposal.Digest()
	decision := contracts.WorkPlanAcceptance{
		ProposalDigest: digest, BaselineDigest: proposal.BaselineDigest,
		AuthorityRef: "docs/PLAN/003-post-release-roadmap.md#unit-1", AuthorityDigest: "sha256:authority", AuthorityScope: "goal:goal-1",
		AcceptanceRef: "acceptance-1", AcceptanceDigest: "sha256:acceptance", AcceptedBy: contracts.PrincipalRef{ID: "human-reviewer", Kind: "human"},
		ReviewRef: "review-1", ReviewVersion: "1", ReviewDigest: "sha256:review", Mode: "human",
	}
	accepted := contracts.WorkPlan{BaselineDigest: proposal.BaselineDigest, Candidates: []contracts.WorkCandidate{{ID: "unit-1", SourceRef: "docs/PLAN/003-post-release-roadmap.md#unit-1", SourceDigest: "sha256:authority", Provenance: contracts.ProvenancePLAN, Requirements: proposal.Candidates[0].Requirements}}}
	return proposal, decision, accepted
}

func workPlanReviewFixture(proposal contracts.WorkPlanProposal) contracts.WorkPlanProposalReview {
	digest, _ := proposal.Digest()
	return contracts.WorkPlanProposalReview{ProposalDigest: digest, BaselineDigest: proposal.BaselineDigest, ReviewRef: "review-1", ReviewDigest: "sha256:review", ReviewedBy: contracts.PrincipalRef{ID: "reviewer", Kind: "agent"}, ReviewerGeneration: "reviewer-generation-1", Status: contracts.ReviewAcceptableForAuthority, CoveredRequirements: []string{"req-1"}}
}

func TestRepositoryWorkPlanAcceptanceSurvivesRestartAndRejectsDuplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	ctx := context.Background()
	keyWrapper := &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repository{Store: state.New(db), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	proposal, decision, accepted := workPlanProposalFixture()
	proposalDigest, err := repo.SaveWorkPlanProposal(ctx, proposal, "1", time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if proposalDigest == "" {
		t.Fatal("proposal digest must be persisted")
	}
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", workPlanReviewFixture(proposal), "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedDB, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopened := Repository{Store: state.New(reopenedDB), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	plan, err := reopened.SaveAcceptedWorkPlan(ctx, proposal.ID, "1", accepted, decision, "1", time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.LoadAcceptedWorkPlan(ctx, decision.AcceptanceRef, "1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProposalDigest != proposalDigest || loaded.AcceptanceRef != decision.AcceptanceRef {
		t.Fatalf("accepted WorkPlan lost durable bindings: saved=%+v loaded=%+v", plan, loaded)
	}
	if _, err := reopened.SaveAcceptedWorkPlan(ctx, proposal.ID, "1", accepted, decision, "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("accepted WorkPlan version was overwritten")
	}
}

func TestRepositoryWorkPlanAcceptanceRejectsMissingProposal(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	proposal, decision, accepted := workPlanProposalFixture()
	if _, err := repo.SaveAcceptedWorkPlan(context.Background(), proposal.ID, "1", accepted, decision, "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("acceptance without a durable proposal was authorized")
	}
}

func TestRepositoryWorkPlanReviewBindsProposalAndSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	ctx := context.Background()
	keyWrapper := &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repository{Store: state.New(db), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	proposal, _, _ := workPlanProposalFixture()
	if _, err := repo.SaveWorkPlanProposal(ctx, proposal, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	digest, err := proposal.Digest()
	if err != nil {
		t.Fatal(err)
	}
	review := contracts.WorkPlanProposalReview{ProposalDigest: digest, BaselineDigest: proposal.BaselineDigest, ReviewRef: "review-1", ReviewDigest: "sha256:review", ReviewedBy: contracts.PrincipalRef{ID: "reviewer", Kind: "agent"}, ReviewerGeneration: "reviewer-generation-1", ReviewerProvider: "same-provider-is-allowed", Status: contracts.ReviewAcceptableForAuthority, CoveredRequirements: []string{"req-1"}}
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", review, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedDB, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopened := Repository{Store: state.New(reopenedDB), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	loaded, err := reopened.LoadWorkPlanReview(ctx, review.ReviewRef, "1", time.Now().UTC())
	if err != nil || loaded.ProposalDigest != digest || loaded.Status != contracts.ReviewAcceptableForAuthority {
		t.Fatalf("review did not survive restart: %+v err=%v", loaded, err)
	}
	if _, err := reopened.LoadAcceptedWorkPlan(ctx, "missing-acceptance", "1", time.Now().UTC()); err == nil {
		t.Fatal("review unexpectedly created acceptance authority")
	}
}

func TestRepositoryAcceptanceRejectsNonAcceptableReview(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	proposal, decision, accepted := workPlanProposalFixture()
	if _, err := repo.SaveWorkPlanProposal(ctx, proposal, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	review := workPlanReviewFixture(proposal)
	review.Status = contracts.ReviewRevisionRequired
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", review, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveAcceptedWorkPlan(ctx, proposal.ID, "1", accepted, decision, "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("revision-required review became acceptance authority")
	}
}

func TestRepositoryAuthorityRequestDecisionIsBoundRestartReadableAndSingleUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	ctx := context.Background()
	keyWrapper := &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repository{Store: state.New(db), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	request := contracts.AuthorityRequest{ID: "authority-request-1", Version: "1", BaselineID: "goal-1", BaselineVersion: "1", BaselineDigest: "sha256:baseline", ProposalID: "proposal-1", ProposalVersion: "1", ProposalDigest: "sha256:proposal", ReviewRef: "review-1", ReviewVersion: "1", ReviewDigest: "sha256:review", RequestedAuthority: "workplan.accept", RequestedScope: "goal:goal-1/proposal-1", Reason: "policy authority is not configured for this material decomposition", AffectedWork: []string{"unit-1"}, TransitivelyBlocked: []string{"unit-2"}, UnrelatedRunnableWork: []string{"unit-3"}, Recommendation: "approve one proposal", Alternatives: []string{"reject", "revise"}, Status: contracts.AuthorityRequestPending}
	if _, err := repo.SaveAuthorityRequest(ctx, request, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.PendingAuthorityRequests(ctx, request.BaselineID, request.BaselineVersion, time.Now().UTC())
	if err != nil || len(pending) != 1 || pending[0].ID != request.ID {
		t.Fatalf("pending request was not discoverable: %+v err=%v", pending, err)
	}
	requestDigest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision-1", DecisionVersion: "1", DecidedBy: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, GrantedScope: request.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: "sha256:operator-authority", IssuedAt: now}
	if err := repo.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, now, nil); err != nil {
		t.Fatal(err)
	}
	pending, err = repo.PendingAuthorityRequests(ctx, request.BaselineID, request.BaselineVersion, time.Now().UTC())
	if err != nil || len(pending) != 0 {
		t.Fatalf("resolved request remained pending: %+v err=%v", pending, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedDB, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopened := Repository{Store: state.New(reopenedDB), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	loaded, err := reopened.LoadAuthorityDecision(ctx, request.ID, request.Version, time.Now().UTC())
	if err != nil || loaded.DecisionRef != decision.DecisionRef || loaded.GrantedScope != request.RequestedScope {
		t.Fatalf("decision did not survive restart: %+v err=%v", loaded, err)
	}
	if err := reopened.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, now, nil); err != nil {
		t.Fatalf("identical authority decision was not idempotent: %v", err)
	}
	overScoped := decision
	overScoped.DecisionRef = "decision-2"
	overScoped.GrantedScope = "org:all"
	if err := reopened.SaveAuthorityDecision(ctx, request.ID, request.Version, overScoped, now, nil); err == nil {
		t.Fatal("over-scoped decision was accepted")
	}
	forged := decision
	forged.DecisionRef = "decision-3"
	forged.DecidedBy = contracts.PrincipalRef{ID: "model", Kind: "model"}
	if err := reopened.SaveAuthorityDecision(ctx, request.ID, request.Version, forged, now, nil); err == nil {
		t.Fatal("model conversational output became authority")
	}
}

func TestRepositoryAttachAcceptedWorkPlanCreatesBoundSuccessor(t *testing.T) {
	repo, db := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	source, err := repo.Save(ctx, goalFixture(), time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	proposal, decision, accepted := workPlanProposalFixture()
	proposal.BaselineDigest = source.Digest
	decision.BaselineDigest = source.Digest
	decision.ProposalDigest, err = proposal.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveWorkPlanProposal(ctx, proposal, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveWorkPlanReview(ctx, proposal.ID, "1", workPlanReviewFixture(proposal), "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	accepted.BaselineDigest = source.Digest
	if _, err := repo.SaveAcceptedWorkPlan(ctx, proposal.ID, "1", accepted, decision, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	successor, err := repo.AttachAcceptedWorkPlan(ctx, source.ID, source.Version, source.Digest, decision.AcceptanceRef, "1", "2", time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if successor.PredecessorDigest != source.Digest || successor.WorkPlan == nil || successor.WorkPlan.BaselineDigest != source.Digest || successor.OriginalIntent != source.OriginalIntent {
		t.Fatalf("successor lost immutable lineage or binding: source=%+v successor=%+v", source, successor)
	}
	unchanged, err := repo.Load(ctx, source.ID, source.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.WorkPlan != nil || unchanged.Digest != source.Digest {
		t.Fatalf("predecessor was mutated: %+v", unchanged)
	}
	if _, err := repo.AttachAcceptedWorkPlan(ctx, source.ID, source.Version, "sha256:wrong", decision.AcceptanceRef, "1", "3", time.Now().UTC(), nil); err == nil {
		t.Fatal("stale source baseline was attached")
	}
	if _, err := repo.AttachAcceptedWorkPlan(ctx, source.ID, source.Version, source.Digest, decision.AcceptanceRef, "1", "2", time.Now().UTC(), nil); err == nil {
		t.Fatal("competing successor generation replaced an immutable baseline")
	}
	_ = db
}
