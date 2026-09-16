package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
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

func authorityGenerationFixture(now time.Time) contracts.AuthorityGeneration {
	digest := "sha256:" + strings.Repeat("a", 64)
	generation := contracts.AuthorityGeneration{
		Ref:                   "installation-governance:" + digest,
		Version:               "1",
		Principal:             contracts.PrincipalRef{ID: "installation-owner:" + digest, Kind: "human"},
		Scope:                 "installation-governance:" + digest,
		Capabilities:          []string{contracts.AuthorityDelegateCapability},
		ProvenanceRef:         "bootstrap-record:" + digest + ":os-user:test",
		ProvenanceDigest:      digest,
		State:                 contracts.AuthorityGenerationActive,
		EffectiveAt:           now.UTC(),
		AuthorityModel:        contracts.AuthorityModelID,
		AuthorityModelVersion: contracts.AuthorityModelVersion,
		AuthorityModelDigest:  contracts.AuthorityModelDigest(),
	}
	digest, err := generation.ComputeDigest()
	if err != nil {
		panic(err)
	}
	generation.Digest = digest
	return generation
}

func persistAuthorityGenerationPayload(t *testing.T, repo Repository, store *state.Store, generation contracts.AuthorityGeneration, objectDigest string) {
	t.Helper()
	payload, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	if objectDigest == "" {
		objectDigest = payloadDigest(payload)
	}
	aad := state.SecureBlobAAD(authorityGenerationNamespace, generation.Ref, generation.Version, objectDigest)
	envelope, err := repo.Crypto.Seal(context.Background(), repo.KeyRef, repo.Profile, payload, aad)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSecureBlob(context.Background(), state.SecureBlobRecord{
		Namespace:     authorityGenerationNamespace,
		ObjectID:      generation.Ref,
		ObjectVersion: generation.Version,
		ObjectDigest:  objectDigest,
		Sensitivity:   repo.Sensitivity,
		CryptoProfile: repo.Profile,
		Envelope:      envelope,
		CreatedAt:     generation.EffectiveAt,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorityGenerationValidationKeepsGenerationAndObjectDigestsDistinct(t *testing.T) {
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Unix(1700000000, 0).UTC()
	generation := authorityGenerationFixture(now)
	if err := repo.SaveAuthorityGeneration(context.Background(), generation, now, nil); err != nil {
		t.Fatal(err)
	}
	record, err := store.GetSecureBlob(context.Background(), authorityGenerationNamespace, generation.Ref, generation.Version, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.ObjectDigest == generation.Digest {
		t.Fatalf("storage object digest must remain distinct from generation digest: %s", generation.Digest)
	}
	loaded, err := repo.LoadAuthorityGeneration(context.Background(), generation.Ref, generation.Version, now)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != generation.Digest {
		t.Fatalf("generation digest changed on load: got %s want %s", loaded.Digest, generation.Digest)
	}
	listed, err := repo.ListAuthorityGenerations(context.Background(), now)
	if err != nil || len(listed) != 1 || listed[0].Digest != generation.Digest {
		t.Fatalf("valid generation was not listed: generations=%+v err=%v", listed, err)
	}
}

func TestHistoricalGenerationLoaderSeparatesDigestDomains(t *testing.T) {
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Now().UTC()
	generation := authorityGenerationFixture(now.Add(-2 * time.Hour))
	expired := now.Add(-time.Hour)
	generation.ExpiresAt = &expired
	digest, err := generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	generation.Digest = digest
	payload, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	objectDigest := payloadDigest(payload)
	if objectDigest == generation.Digest {
		t.Fatal("fixture must keep secure-blob and generation digests distinct")
	}
	envelope, err := repo.Crypto.Seal(context.Background(), repo.KeyRef, repo.Profile, payload, state.SecureBlobAAD(authorityGenerationNamespace, generation.Ref, generation.Version, objectDigest))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSecureBlob(context.Background(), state.SecureBlobRecord{Namespace: authorityGenerationNamespace, ObjectID: generation.Ref, ObjectVersion: generation.Version, ObjectDigest: objectDigest, Sensitivity: repo.Sensitivity, CryptoProfile: repo.Profile, Envelope: envelope, CreatedAt: generation.EffectiveAt, ExpiresAt: &expired}); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.loadHistoricalGeneration(context.Background(), generation.Ref, generation.Version, generation.Digest)
	if err != nil || loaded.Digest != generation.Digest {
		t.Fatalf("historical generation should validate separate digests: %+v %v", loaded, err)
	}
	if _, err := repo.loadHistoricalGeneration(context.Background(), generation.Ref, generation.Version, "sha256:"+strings.Repeat("b", 64)); err == nil {
		t.Fatal("substituted generation digest accepted")
	}
	if _, err := repo.loadHistoricalGeneration(context.Background(), generation.Ref+"-wrong", generation.Version, generation.Digest); err == nil {
		t.Fatal("wrong generation identity accepted")
	}
	if _, err := repo.loadHistoricalGeneration(context.Background(), generation.Ref, "2", generation.Digest); err == nil {
		t.Fatal("wrong generation version accepted")
	}
	tampered := generation
	tampered.Ref = generation.Ref + "-tampered"
	tamperedPayload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	tamperedEnvelope, err := repo.Crypto.Seal(context.Background(), repo.KeyRef, repo.Profile, tamperedPayload, state.SecureBlobAAD(authorityGenerationNamespace, tampered.Ref, tampered.Version, objectDigest))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSecureBlob(context.Background(), state.SecureBlobRecord{Namespace: authorityGenerationNamespace, ObjectID: tampered.Ref, ObjectVersion: tampered.Version, ObjectDigest: objectDigest, Sensitivity: repo.Sensitivity, CryptoProfile: repo.Profile, Envelope: tamperedEnvelope, CreatedAt: tampered.EffectiveAt, ExpiresAt: &expired}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.loadHistoricalGeneration(context.Background(), tampered.Ref, tampered.Version, tampered.Digest); err == nil || !strings.Contains(err.Error(), "payload digest") {
		t.Fatalf("tampered serialized payload must fail at object-digest layer: %v", err)
	}
}

func TestExpiredHistoricalAuthorityResolvesPersistedParentAndDelegatedChild(t *testing.T) {
	ctx := context.Background()
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	created := time.Now().UTC().Add(-2 * time.Hour)
	now := created.Add(3 * time.Hour)
	bootstrap := contracts.GoalsPublicationBootstrap
	root := contracts.AuthorityGeneration{Ref: "installation-governance:" + bootstrap, Version: "1", Principal: contracts.PrincipalRef{ID: "installation-owner:" + bootstrap, Kind: "human"}, Scope: "installation-governance:" + bootstrap, Capabilities: []string{contracts.AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:test", ProvenanceDigest: bootstrap, State: contracts.AuthorityGenerationActive, EffectiveAt: created}
	root.Digest, _ = root.ComputeDigest()
	childExpiry := created.Add(time.Hour)
	child := contracts.AuthorityGeneration{Ref: "authority-delegation:test-request", Version: "1", Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, Scope: "goals-established-state-publication:test", Authorities: []string{contracts.GovernedPackagePublish}, ProvenanceRef: "authority-decision:test", ProvenanceDigest: "sha256:" + strings.Repeat("d", 64), State: contracts.AuthorityGenerationActive, EffectiveAt: created.Add(10 * time.Minute), ExpiresAt: &childExpiry, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedBy: root.Principal, DelegationRef: "test-request/1", DelegationDigest: "sha256:" + strings.Repeat("e", 64), PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelGoalsRecoveryVersion, PolicyDigest: contracts.AuthorityModelGoalsRecoveryDigest(), AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelGoalsRecoveryVersion, AuthorityModelDigest: contracts.AuthorityModelGoalsRecoveryDigest()}
	child.Digest, _ = child.ComputeDigest()
	persist := func(namespace, id, version string, value any, expires *time.Time) string {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		digest := payloadDigest(payload)
		envelope, err := repo.Crypto.Seal(ctx, repo.KeyRef, repo.Profile, payload, state.SecureBlobAAD(namespace, id, version, digest))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutSecureBlob(ctx, state.SecureBlobRecord{Namespace: namespace, ObjectID: id, ObjectVersion: version, ObjectDigest: digest, Sensitivity: repo.Sensitivity, CryptoProfile: repo.Profile, Envelope: envelope, CreatedAt: created, ExpiresAt: expires}); err != nil {
			t.Fatal(err)
		}
		return digest
	}
	persist(authorityGenerationNamespace, root.Ref, root.Version, root, nil)
	childObjectDigest := persist(authorityGenerationNamespace, child.Ref, child.Version, child, &childExpiry)
	intent := contracts.ActionIntent{Version: "1", ID: "intent:test", Actor: child.Principal, Operation: contracts.GoalsRecoveryOperation, Target: "github-release:389997269", Scope: child.Scope, Parameters: map[string]string{"created_at": created.Format(time.RFC3339Nano), "expires_at": childExpiry.Format(time.RFC3339Nano)}}
	intentDigest, _ := intent.Digest()
	delegation := contracts.DelegationRequest{Profile: contracts.GoalsPublicationRecoveryProfile, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: child.Principal, TargetKind: "action-intent", TargetIdentity: intent.ID, TargetVersion: intent.Version, TargetDigest: intentDigest, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: contracts.GoalsRecoveryOperation, RequestedScope: child.Scope, ProposalVersion: intent.Version, ProposalDigest: intentDigest, ReviewVersion: intent.Version, ReviewDigest: intentDigest, ExpiresAt: childExpiry, Reason: "test", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelGoalsRecoveryVersion, PolicyDigest: contracts.AuthorityModelGoalsRecoveryDigest()}
	request := contracts.AuthorityRequest{ID: "test-request", Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: child.Scope, Reason: "test", Status: contracts.AuthorityRequestPending, Delegation: &delegation, Intent: &intent, IntentDigest: intentDigest, InstallationDigest: contracts.GoalsPublicationRoot}
	requestDigest, _ := request.DigestAt(created)
	delegation.TargetDigest = intentDigest
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision:test", DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: child.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelGoalsRecoveryDigest(), IssuedAt: created.Add(5 * time.Minute), ExpiresAt: &childExpiry, Delegation: &delegation}
	decisionPayload := authorityDecisionRecord{Request: request, Decision: decision}
	requestObjectDigest := persist(authorityRequestNamespace, request.ID, request.Version, request, &childExpiry)
	persist(authorityDecisionNamespace, request.ID, request.Version, decisionPayload, &childExpiry)
	effectIDs := []string{"execution:test:manifest", "execution:test:archive", "execution:test:signature", "execution:test:verify-draft"}
	effectPayload, _ := json.Marshal(struct {
		RequestID string                                `json:"request_id"`
		Authority contracts.PackagePublishAuthorization `json:"authority"`
	}{request.ID, contracts.PackagePublishAuthorization{Generation: child, Request: request, Decision: decision}})
	for _, id := range effectIDs {
		if _, err := store.DB().ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, "test", "1", child.Principal.ID, child.Principal.Kind, child.Scope, "execution:test", effectPayload, "completed", created.Add(20*time.Minute).Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.DB().ExecContext(ctx, `INSERT INTO effects(effect_id,command_id,action_intent_digest,target_adapter,state,attempts,request_payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, id, intentDigest, "test", "succeeded", 1, effectPayload, created.Add(20*time.Minute).Format(time.RFC3339Nano), created.Add(20*time.Minute).Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := repo.LoadExpiredHistoricalAuthorityEvidence(ctx, request.ID, request.Version, requestObjectDigest, contracts.GoalsPublicationRoot, "execution:test", effectIDs, now)
	if err != nil || evidence.GenerationDigest != child.Digest || childObjectDigest == child.Digest {
		t.Fatalf("expected persisted parent/child resolution, got %+v err=%v", evidence, err)
	}
}

func TestAuthorityGenerationValidationRejectsPayloadAndGenerationSubstitution(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	valid := authorityGenerationFixture(now)
	objectSubstitution := "sha256:" + strings.Repeat("b", 64)
	tests := []struct {
		name         string
		generation   contracts.AuthorityGeneration
		objectDigest string
		want         string
	}{
		{name: "corrupted payload", generation: func() contracts.AuthorityGeneration {
			g := valid
			g.Scope = "installation-governance:substituted"
			return g
		}(), objectDigest: func() string { payload, _ := json.Marshal(valid); return payloadDigest(payload) }(), want: "identity or digest mismatch"},
		{name: "corrupted generation digest", generation: func() contracts.AuthorityGeneration {
			g := valid
			g.Digest = "sha256:" + strings.Repeat("c", 64)
			return g
		}(), want: "authority generation digest mismatch"},
		{name: "substituted object digest", generation: valid, objectDigest: objectSubstitution, want: "identity or digest mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
			persistAuthorityGenerationPayload(t, repo, store, tc.generation, tc.objectDigest)
			_, err := repo.LoadAuthorityGeneration(context.Background(), tc.generation.Ref, tc.generation.Version, now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
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

func TestRepositoryConsumesApprovedAuthorityDecisionExactlyOnce(t *testing.T) {
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
	reviewDigest := review.ReviewDigest
	request := contracts.AuthorityRequest{ID: "request-accept-1", Version: "1", BaselineID: proposal.GoalID, BaselineVersion: proposal.GoalVersion, BaselineDigest: proposal.BaselineDigest, ProposalID: proposal.ID, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewRef: review.ReviewRef, ReviewVersion: "1", ReviewDigest: reviewDigest, RequestedAuthority: "workplan.accept", RequestedScope: "goal:goal-1/proposal-1", Reason: "material decomposition requires explicit authority", Status: contracts.AuthorityRequestPending}
	if _, err := repo.SaveAuthorityRequest(ctx, request, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	generation := contracts.AuthorityGeneration{Ref: "policy:goal-acceptance", Version: "7", Principal: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, Scope: request.RequestedScope, ProvenanceRef: "policy:goal-acceptance", ProvenanceDigest: "sha256:policy-source", State: contracts.AuthorityGenerationActive, EffectiveAt: time.Now().UTC()}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generation, generation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	requestDigest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision-accept-1", DecisionVersion: "1", DecidedBy: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, AuthorityRef: "policy:goal-acceptance", AuthorityVersion: "7", AuthorityGenerationDigest: generation.Digest, GrantedScope: request.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: "sha256:operator", IssuedAt: time.Now().UTC()}
	if err := repo.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	acceptedPlan, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, request.ID, request.Version, accepted, "acceptance-authority-1", "1", time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if acceptedPlan.AcceptedBy.ID != decision.DecidedBy.ID || acceptedPlan.AuthorityDigest != decision.AuthorityDigest || acceptedPlan.BaselineDigest != request.BaselineDigest {
		t.Fatalf("decision evidence was not consumed into acceptance: %+v", acceptedPlan)
	}
	replayed, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, request.ID, request.Version, accepted, "acceptance-authority-1", "1", time.Now().UTC(), nil)
	if err != nil || replayed.AcceptanceRef != acceptedPlan.AcceptanceRef {
		t.Fatalf("identical authority-backed acceptance was not idempotent: %+v err=%v", replayed, err)
	}
	decisionDigest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	revocation := contracts.AuthorityRevocation{RequestID: request.ID, RequestVersion: request.Version, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest, RevocationRef: "revocation-1", RevocationVersion: "1", RevokedBy: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, AuthorityDigest: "sha256:operator-revocation", EffectiveAt: time.Now().UTC(), Reason: "authority withdrawn before attachment"}
	if err := repo.SaveAuthorityRevocation(ctx, request.ID, request.Version, revocation, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadAuthorityDecision(ctx, request.ID, request.Version, time.Now().UTC()); !errors.Is(err, ErrAuthorityDecisionRevoked) {
		t.Fatalf("revoked authority remained effective: %v", err)
	}
	if evidence, err := repo.LoadAuthorityDecisionEvidence(ctx, request.ID, request.Version, time.Now().UTC()); err != nil || evidence.DecisionRef != decision.DecisionRef {
		t.Fatalf("historical decision evidence was not retained: %+v err=%v", evidence, err)
	}
	if _, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, request.ID, request.Version, accepted, "acceptance-after-revoke", "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("revoked authority was consumed into a new acceptance")
	}
	rejectedRequest := request
	rejectedRequest.ID = "request-reject-1"
	rejectedRequestDigest, err := rejectedRequest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveAuthorityRequest(ctx, rejectedRequest, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	rejected := decision
	rejected.RequestID = rejectedRequest.ID
	rejected.RequestDigest = rejectedRequestDigest
	rejected.DecisionRef = "decision-reject-1"
	rejected.Outcome = contracts.AuthorityReject
	if err := repo.SaveAuthorityDecision(ctx, rejectedRequest.ID, rejectedRequest.Version, rejected, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, rejectedRequest.ID, rejectedRequest.Version, accepted, "acceptance-rejected", "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("rejected authority decision became executable acceptance")
	}
}

func TestRepositoryAuthorityGenerationInvalidationSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authority.db")
	ctx := context.Background()
	keyWrapper := &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repository{Store: state.New(db), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	now := time.Now().UTC()
	generation := contracts.AuthorityGeneration{Ref: "policy:restart", Version: "4", Principal: contracts.PrincipalRef{ID: "operator-1", Kind: "human"}, Scope: "goal:goal-1/proposal-1", ProvenanceRef: "policy:restart", ProvenanceDigest: "sha256:policy-4", State: contracts.AuthorityGenerationActive, EffectiveAt: now}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generation, now, nil); err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{AuthorityRef: generation.Ref, AuthorityVersion: generation.Version, AuthorityGenerationDigest: generation.Digest, DecidedBy: generation.Principal, GrantedScope: generation.Scope}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: generation.Ref, Version: generation.Version, GenerationDigest: generation.Digest, InvalidationRef: "policy-restart-revoke", InvalidationVersion: "1", Kind: "superseded", SupersededBy: "5", InvalidatedBy: generation.Principal, EffectiveAt: now, Reason: "policy generation advanced"}
	if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, now, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateAuthorityGeneration(ctx, decision, now); err == nil {
		t.Fatal("superseded authority generation remained effective")
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
	if _, err := reopened.LoadAuthorityGeneration(ctx, generation.Ref, generation.Version, now); err != nil {
		t.Fatalf("historical generation was not retained: %v", err)
	}
	if err := reopened.ValidateAuthorityGeneration(ctx, decision, now); err == nil {
		t.Fatal("restart resurrected superseded authority generation")
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
