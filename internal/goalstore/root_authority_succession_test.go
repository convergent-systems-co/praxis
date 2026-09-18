package goalstore

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const successionTestBootstrap = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func rootSuccessionFixture(t *testing.T, repo Repository, now time.Time) contracts.AuthorityGeneration {
	t.Helper()
	repo.InstallationDigest = successionTestBootstrap
	root := authorityGenerationFixture(now.Add(-time.Hour))
	if err := repo.SaveAuthorityGeneration(context.Background(), root, now.Add(-time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	return root
}

func acceptedRootSuccession(t *testing.T, repo Repository, predecessor contracts.AuthorityGeneration, now time.Time) (contracts.AuthorityGeneration, contracts.RootAuthoritySuccessionProposal, contracts.RootAuthoritySuccessionReview, contracts.RootAuthoritySuccessionDecision) {
	t.Helper()
	ctx := context.Background()
	proposal, err := contracts.BuildRootAuthoritySuccession(predecessor, successionTestBootstrap, now)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := repo.SaveRootAuthoritySuccessionProposal(ctx, proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	review, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, predecessor.Principal, "test", "REVIEW-ROOT-SUCCESSOR "+proposalDigest, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	successor, decision, err := repo.AcceptRootAuthoritySuccession(ctx, proposalDigest, reviewDigest, successionTestBootstrap, "test", "ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewDigest, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return successor, proposal, review, decision
}

func TestRootAuthoritySuccessionProductionPathAndDurableLineage(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute)
	path := filepath.Join(t.TempDir(), "praxis.db")
	repo, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	predecessor := rootSuccessionFixture(t, repo, now)
	successor, proposal, _, decision := acceptedRootSuccession(t, repo, predecessor, now)

	if successor.Version != "2" || successor.PredecessorDigest != predecessor.Digest || successor.Ref != predecessor.Ref || successor.Scope != predecessor.Scope || successor.Principal != predecessor.Principal || successor.ProvenanceDigest != successionTestBootstrap {
		t.Fatalf("successor lost exact installation/predecessor identity: %+v", successor)
	}
	if successor.ParentRef != "" || successor.DelegatedBy != (contracts.PrincipalRef{}) {
		t.Fatalf("repair-bearing successor is delegable: %+v", successor)
	}
	for _, authority := range contracts.InstallationRepairRootAuthorities {
		if !slices.Contains(successor.Authorities, authority) {
			t.Fatalf("successor lacks %q", authority)
		}
	}
	loadedPredecessor, err := repo.LoadAuthorityGeneration(context.Background(), predecessor.Ref, predecessor.Version, now.Add(3*time.Second))
	if err != nil || loadedPredecessor.Digest != predecessor.Digest {
		t.Fatalf("immutable predecessor unavailable: %+v %v", loadedPredecessor, err)
	}
	if err := repo.ValidateAuthorityGeneration(context.Background(), contracts.AuthorityDecision{AuthorityRef: predecessor.Ref, AuthorityVersion: predecessor.Version, AuthorityGenerationDigest: predecessor.Digest, DecidedBy: predecessor.Principal, GrantedScope: predecessor.Scope}, now.Add(3*time.Second)); err == nil {
		t.Fatal("superseded predecessor remained effective")
	}

	// A newly constructed repository over the same durable database must recover
	// the exact active root and independently validate the complete lineage.
	reopened, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	reopened.InstallationDigest = successionTestBootstrap
	current, err := reopened.LoadCurrentInstallationRoot(context.Background(), successionTestBootstrap, now.Add(4*time.Second))
	if err != nil || current.Digest != successor.Digest {
		t.Fatalf("restart did not recover exact successor: %+v %v", current, err)
	}
	proposalDigest, _ := proposal.Digest()
	reviewDigest := decision.ReviewDigest
	replayed, replayDecision, err := reopened.AcceptRootAuthoritySuccession(context.Background(), proposalDigest, reviewDigest, successionTestBootstrap, "test", "ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewDigest, now.Add(5*time.Second))
	if err != nil || replayed.Digest != successor.Digest {
		t.Fatalf("exact acceptance replay failed: %+v %v", replayed, err)
	}
	if got, _ := replayDecision.Digest(); got == "" {
		t.Fatal("replayed succession decision is invalid")
	}
	if _, err := contracts.BuildRootAuthoritySuccession(current, successionTestBootstrap, now.Add(6*time.Second)); err == nil {
		t.Fatal("a second repair-authority augmentation was proposed")
	}
}

func TestRootAuthoritySuccessionRejectsStaleWrongOrConflictingIdentity(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute)
	ctx := context.Background()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	predecessor := rootSuccessionFixture(t, repo, now)
	proposal, err := contracts.BuildRootAuthoritySuccession(predecessor, successionTestBootstrap, now)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := repo.SaveRootAuthoritySuccessionProposal(ctx, proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := contracts.BuildRootAuthoritySuccession(predecessor, successionTestBootstrap, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRootAuthoritySuccessionProposal(ctx, conflict, now.Add(time.Second)); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("conflicting successor proposal was not rejected: %v", err)
	}
	if _, _, err := repo.SaveRootAuthoritySuccessionReview(ctx, digest, contracts.PrincipalRef{ID: "worker", Kind: "model"}, "test", "REVIEW-ROOT-SUCCESSOR "+digest, now.Add(time.Second)); err == nil {
		t.Fatal("model/worker self-approval was accepted")
	}
	if _, _, err := repo.SaveRootAuthoritySuccessionReview(ctx, digest, predecessor.Principal, "wrong-user", "REVIEW-ROOT-SUCCESSOR "+digest, now.Add(time.Second)); err == nil {
		t.Fatal("wrong authenticated owner was accepted")
	}
	review, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(ctx, digest, predecessor.Principal, "test", "REVIEW-ROOT-SUCCESSOR "+digest, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_ = review
	wrongBootstrap := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, _, err := repo.AcceptRootAuthoritySuccession(ctx, digest, reviewDigest, wrongBootstrap, "test", "ACCEPT-ROOT-SUCCESSOR "+digest+" "+reviewDigest, now.Add(2*time.Second)); err == nil {
		t.Fatal("wrong bootstrap identity was accepted")
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: predecessor.Ref, Version: predecessor.Version, GenerationDigest: predecessor.Digest, InvalidationRef: "test:stale", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: predecessor.Principal, EffectiveAt: now.Add(2 * time.Second), Reason: "test stale predecessor"}
	if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, now.Add(2*time.Second), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.AcceptRootAuthoritySuccession(ctx, digest, reviewDigest, successionTestBootstrap, "test", "ACCEPT-ROOT-SUCCESSOR "+digest+" "+reviewDigest, now.Add(3*time.Second)); err == nil {
		t.Fatal("stale predecessor was accepted")
	}
}

func TestRootAuthoritySuccessionRejectsDelegationMarkers(t *testing.T) {
	now := time.Now().UTC()
	predecessor := authorityGenerationFixture(now.Add(-time.Hour))
	proposal, err := contracts.BuildRootAuthoritySuccession(predecessor, successionTestBootstrap, now)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*contracts.AuthorityGeneration){
		"parent": func(g *contracts.AuthorityGeneration) {
			g.ParentRef, g.ParentVersion, g.ParentDigest = "parent", "1", predecessor.Digest
		},
		"delegated_by": func(g *contracts.AuthorityGeneration) {
			g.DelegatedBy = contracts.PrincipalRef{ID: "worker", Kind: "model"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := proposal
			bad.Successor = proposal.Successor
			mutate(&bad.Successor)
			bad.Successor.Digest, _ = bad.Successor.ComputeDigest()
			if _, err := bad.Digest(); err == nil {
				t.Fatal("delegation marker was accepted in exact successor proposal")
			}
		})
	}
}

func TestInstallationRepairRequestsAreIndependentDurableDecisions(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute)
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	predecessor := rootSuccessionFixture(t, repo, now)
	_, _, _, _ = acceptedRootSuccession(t, repo, predecessor, now)
	owner, _ := contracts.InstallationOwnerPrincipal(successionTestBootstrap)
	expiry := now.Add(time.Hour)

	requests := map[string]contracts.AuthorityRequest{}
	decisions := map[string]contracts.AuthorityDecision{}
	for _, operation := range contracts.InstallationRepairRootAuthorities {
		request, digest, err := repo.SaveInstallationRepairAuthorityRequest(context.Background(), operation, expiry, now.Add(3*time.Second))
		if err != nil {
			t.Fatalf("save %s request: %v", operation, err)
		}
		decision, err := repo.ApproveInstallationRepairAuthorityRequest(context.Background(), digest, owner, "APPROVE-INSTALLATION-REPAIR "+digest, now.Add(4*time.Second))
		if err != nil {
			t.Fatalf("approve %s request: %v", operation, err)
		}
		loaded, err := repo.LoadAuthorityDecision(context.Background(), request.ID, request.Version, now.Add(5*time.Second))
		if err != nil || loaded.RequestDigest != digest {
			t.Fatalf("durable %s decision mismatch: %+v %v", operation, loaded, err)
		}
		requests[operation], decisions[operation] = request, decision
	}
	storage := contracts.GovernedInstallationRepairStorageSchema
	runtime := contracts.GovernedInstallationRepairRuntimeState
	if requests[storage].ID == requests[runtime].ID || decisions[storage].RequestDigest == decisions[runtime].RequestDigest {
		t.Fatal("repair operations collapsed into one request or decision")
	}
	if err := decisions[storage].Validate(requests[runtime], now.Add(5*time.Second)); err == nil {
		t.Fatal("possession of storage-schema decision implied runtime-state authority")
	}
	if err := decisions[runtime].Validate(requests[storage], now.Add(5*time.Second)); err == nil {
		t.Fatal("possession of runtime-state decision implied storage-schema authority")
	}
}

func TestRootSuccessionRawAndGenericPersistenceCannotBypassAdmission(t *testing.T) {
	now := time.Now().UTC()
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	predecessor := authorityGenerationFixture(now.Add(-time.Hour))
	proposal, err := contracts.BuildRootAuthoritySuccession(predecessor, successionTestBootstrap, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(context.Background(), proposal.Successor, now, nil); err == nil {
		t.Fatal("generic typed writer minted repair-bearing root")
	}
	if err := store.PutSecureBlob(context.Background(), state.SecureBlobRecord{Namespace: state.AuthorityGenerationNamespace}); err == nil {
		t.Fatal("raw writer reached reserved authority-generation namespace")
	}
}
