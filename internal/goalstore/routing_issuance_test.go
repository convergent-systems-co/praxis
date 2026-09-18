package goalstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type builtinTestPolicy struct{}

func (builtinTestPolicy) ContainDelegation(parent contracts.AuthorityGeneration, request contracts.DelegationRequest, now time.Time) error {
	return containBuiltinDelegation(parent, request, now)
}

// activateAuthorityModelForTest publishes an active model state directly.
// The governed v5->v6 adoption edge is exercised separately by
// TestAdoptAuthorityModelV5ToV6; this fixture only positions the state.
func activateAuthorityModelForTest(t *testing.T, repo Repository, version, digest string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	id := "active-authority-model-test-" + version
	if err := repo.insertGovernanceRecordTx(ctx, tx, id, "1", contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: version, ActiveDigest: digest, State: "committed"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE authority_model_active SET object_namespace=?,object_id=?,object_version=?,updated_at=? WHERE singleton_id=?`, publisherGovernanceNamespace, id, "1", now.UTC().Format(time.RFC3339Nano), authorityModelActiveProjectionID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func routingIssuanceFixture(t *testing.T) (Repository, contracts.AuthorityGeneration, contracts.AuthorityGeneration, contracts.RoutingIssuance, time.Time, time.Time) {
	t.Helper()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Now().UTC()
	bootstrap := "sha256:" + strings.Repeat("a", 64)
	repo.InstallationDigest = bootstrap
	root := authorityGenerationFixture(now.Add(-time.Minute))
	if err := repo.SaveAuthorityGeneration(ctx, root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour)
	payload := []byte(`{"authority":"node","target":"exact"}`)
	sum := sha256.Sum256(payload)
	payloadDigest := "sha256:" + hex.EncodeToString(sum[:])
	scope, _ := contracts.RoutingTargetContributionScope("routing-authority", "1", payloadDigest)
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfileRoutingTargetContribution, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: contracts.PrincipalRef{ID: "controller:routing", Kind: "controller"}, TargetKind: "routing.target-contribution", TargetIdentity: "routing-authority", TargetVersion: "1", TargetDigest: payloadDigest, ProposalVersion: "1", ProposalDigest: payloadDigest, ReviewVersion: "1", ReviewDigest: payloadDigest, RequestedAuthority: contracts.AuthorityRoutingTargetContributionIssue, RequestedOperation: contracts.RoutingIssuanceOperation, RequestedScope: scope, ExpiresAt: expires, Reason: "issue exact routing targets", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelRoutingVersion, PolicyDigest: contracts.AuthorityModelRoutingDigest()}
	delegateRequest := contracts.AuthorityRequest{ID: "delegate-routing", Version: "1", RequestedAuthority: contracts.AuthorityDelegateCapability, RequestedScope: scope, Reason: "least privilege routing issuer", Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	delegateDigest, err := repo.SaveAuthorityRequest(ctx, delegateRequest, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	delegateDecision := contracts.AuthorityDecision{RequestID: delegateRequest.ID, RequestVersion: "1", RequestDigest: delegateDigest, DecisionRef: "decision:delegate-routing", DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelRoutingDigest(), IssuedAt: now, ExpiresAt: &expires, Delegation: &delegation}
	if err := repo.SaveAuthorityDecision(ctx, delegateRequest.ID, "1", delegateDecision, now, &expires); err != nil {
		t.Fatal(err)
	}
	child, err := repo.SaveDelegatedAuthorityGeneration(ctx, delegateRequest.ID, "1", delegateDecision, builtinTestPolicy{}, now)
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.AuthorityRequest{ID: "issue-target", Version: "1", BaselineID: "route", BaselineVersion: "1", BaselineDigest: payloadDigest, ProposalID: "target", ProposalVersion: "1", ProposalDigest: payloadDigest, ReviewRef: "review", ReviewVersion: "1", ReviewDigest: payloadDigest, RequestedAuthority: contracts.AuthorityRoutingTargetContributionIssue, RequestedScope: scope, Reason: "issue exact target", Status: contracts.AuthorityRequestPending}
	requestDigest, err := repo.SaveAuthorityRequest(ctx, request, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: "1", RequestDigest: requestDigest, DecisionRef: "decision:issue-target", DecisionVersion: "1", DecidedBy: child.Principal, AuthorityRef: child.Ref, AuthorityVersion: child.Version, AuthorityGenerationDigest: child.Digest, GrantedScope: scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelRoutingDigest(), IssuedAt: now, ExpiresAt: &expires}
	if err := repo.SaveAuthorityDecision(ctx, request.ID, "1", decision, now, &expires); err != nil {
		t.Fatal(err)
	}
	draft := contracts.RoutingIssuance{Version: "1", Kind: contracts.RoutingTargetContribution, Authority: contracts.AuthorityRoutingTargetContributionIssue, Scope: scope, RequestID: request.ID, RequestVersion: "1", DecisionRef: decision.DecisionRef, DecisionVersion: "1", GenerationRef: child.Ref, GenerationVersion: child.Version, GenerationDigest: child.Digest, IssuedBy: child.Principal, Payload: payload, PayloadDigest: payloadDigest, ExpiresAt: &expires}
	return repo, root, child, draft, now, expires
}

func TestRoutingIssuanceRequiresAdoptedV6AndExactDelegatedAuthority(t *testing.T) {
	repo, root, child, draft, now, expires := routingIssuanceFixture(t)
	ctx := context.Background()
	if _, err := repo.SaveRoutingIssuance(ctx, draft); err == nil {
		t.Fatal("routing issuance succeeded without an adopted v6 model")
	}
	for i, model := range []struct{ version, digest string }{
		{contracts.AuthorityModelSuccessorVersion, contracts.AuthorityModelSuccessorDigest()},
		{contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelDeploymentDigest()},
		{contracts.AuthorityModelGoalsPublicationVersion, contracts.AuthorityModelGoalsPublicationDigest()},
		{contracts.AuthorityModelGoalsRecoveryVersion, contracts.AuthorityModelGoalsRecoveryDigest()},
	} {
		activateAuthorityModelForTest(t, repo, model.version, model.digest, now.Add(time.Duration(i)*time.Millisecond))
		if _, err := repo.SaveRoutingIssuance(ctx, draft); err == nil {
			t.Fatalf("routing issuance succeeded under %s: routing authority exists only under adopted v6", model.version)
		}
	}
	activateAuthorityModelForTest(t, repo, contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest(), now.Add(time.Second))
	forgedAuthority := draft
	forgedAuthority.Authority = contracts.AuthorityRoutingSurfaceEligibilityIssue
	if _, err := repo.SaveRoutingIssuance(ctx, forgedAuthority); err == nil {
		t.Fatal("forged routing authority issued")
	}
	rootIssued := draft
	rootIssued.GenerationRef, rootIssued.GenerationVersion, rootIssued.GenerationDigest, rootIssued.IssuedBy = root.Ref, root.Version, root.Digest, root.Principal
	if _, err := repo.SaveRoutingIssuance(ctx, rootIssued); err == nil {
		t.Fatal("installation root issued a route directly")
	}
	issued, err := repo.SaveRoutingIssuance(ctx, draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadRoutingIssuance(ctx, contracts.RoutingIssuanceRef{ID: issued.ID, Version: issued.Version}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadRoutingIssuance(ctx, contracts.RoutingIssuanceRef{ID: issued.ID, Version: issued.Version}, expires); err == nil {
		t.Fatal("expired issuance loaded")
	}
	forged := issued
	forged.ID = "sha256:" + strings.Repeat("f", 64)
	if _, err := repo.LoadRoutingIssuance(ctx, contracts.RoutingIssuanceRef{ID: forged.ID, Version: forged.Version}, time.Now().UTC()); err == nil {
		t.Fatal("unissued payload loaded")
	}
	if _, err := repo.ValidateAuthorityGenerationLineage(ctx, child.Ref, child.Version, child.Digest, repo.InstallationDigest, time.Now().UTC()); err != nil {
		t.Fatalf("delegated routing lineage must validate to the current root: %v", err)
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: child.Ref, Version: child.Version, GenerationDigest: child.Digest, InvalidationRef: "revoke-routing", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test revocation"}
	if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadRoutingIssuance(ctx, contracts.RoutingIssuanceRef{ID: issued.ID, Version: issued.Version}, time.Now().UTC()); err == nil {
		t.Fatal("revoked issuance loaded")
	}
}

func v6AdoptionFixture(t *testing.T, repo Repository, now time.Time) (contracts.AuthorityModelAdoption, string, string) {
	t.Helper()
	adoption, bootstrapDigest := adoptionFixture(t, repo, now)
	v6 := adoption
	v6.ID, v6.FromVersion, v6.FromDigest, v6.ToVersion, v6.ToDigest = "authority-model-adoption:v3-to-v6:test", contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelDeploymentDigest(), contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest()
	digest, err := v6.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return v6, digest, bootstrapDigest
}

// TestAdoptAuthorityModelV3ToV6IsTheOnlyRoutingAdoptionEdge proves the
// succession graph of ADR-094: v6 is adoptable from active v3 only. The
// installation-scoped Goals branch (v4, v5) is neither an ancestor nor a
// predecessor of v6, and v1/v2 must still pass through v3.
func TestAdoptAuthorityModelV3ToV6IsTheOnlyRoutingAdoptionEdge(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	ctx := context.Background()
	for _, from := range []struct {
		name, version, digest string
	}{
		{"v2", contracts.AuthorityModelSuccessorVersion, contracts.AuthorityModelSuccessorDigest()},
		{"v4", contracts.AuthorityModelGoalsPublicationVersion, contracts.AuthorityModelGoalsPublicationDigest()},
		{"v5", contracts.AuthorityModelGoalsRecoveryVersion, contracts.AuthorityModelGoalsRecoveryDigest()},
	} {
		repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
		v6, digest, bootstrapDigest := v6AdoptionFixture(t, repo, now)
		if _, err := repo.AdoptAuthorityModel(ctx, v6, bootstrapDigest, "test", "ADOPT "+digest, now); err == nil {
			t.Fatal("v6 adopted from implicit v1")
		}
		activateAuthorityModelForTest(t, repo, from.version, from.digest, now)
		if _, err := repo.AdoptAuthorityModel(ctx, v6, bootstrapDigest, "test", "ADOPT "+digest, now.Add(time.Second)); err == nil {
			t.Fatalf("v6 adopted from %s", from.name)
		}
		// A forged adoption record claiming the branch model as predecessor
		// is refused by the successor predicate itself.
		forged := v6
		forged.FromVersion, forged.FromDigest = from.version, from.digest
		forgedDigest, _ := forged.Digest()
		if _, err := repo.AdoptAuthorityModel(ctx, forged, bootstrapDigest, "test", "ADOPT "+forgedDigest, now.Add(time.Second)); err == nil {
			t.Fatalf("v6 adopted through a forged %s predecessor", from.name)
		}
		if state, err := repo.LoadAuthorityModelState(ctx, now.Add(2*time.Second)); err != nil || state.ActiveVersion != from.version {
			t.Fatalf("refused adoption must leave %s active: %+v %v", from.name, state, err)
		}
	}

	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	v6, digest, bootstrapDigest := v6AdoptionFixture(t, repo, now)
	activateAuthorityModelForTest(t, repo, contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelDeploymentDigest(), now)
	// The Goals branch stays limited to its exact installation.
	v4 := v6
	v4.ID, v4.ToVersion, v4.ToDigest = "authority-model-adoption:v3-to-v4:test", contracts.AuthorityModelGoalsPublicationVersion, contracts.AuthorityModelGoalsPublicationDigest()
	v4Digest, _ := v4.Digest()
	if _, err := repo.AdoptAuthorityModel(ctx, v4, bootstrapDigest, "test", "ADOPT "+v4Digest, now.Add(time.Second)); err == nil {
		t.Fatal("v4 adopted outside the exact Goals installation")
	}
	if _, err := repo.AdoptAuthorityModel(ctx, v6, bootstrapDigest, "test", "ADOPT "+digest, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, err := repo.LoadAuthorityModelState(ctx, now.Add(3*time.Second))
	if err != nil || state.ActiveVersion != contracts.AuthorityModelRoutingVersion || state.ActiveDigest != contracts.AuthorityModelRoutingDigest() {
		t.Fatalf("v6 not active: %+v %v", state, err)
	}
}

// TestPersistedV5InstallationReopensUnchanged proves the topology decision
// leaves an installation on the Goals branch exactly where it was.
func TestPersistedV5InstallationReopensUnchanged(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	repo, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	v6, digest, bootstrapDigest := v6AdoptionFixture(t, repo, now)
	activateAuthorityModelForTest(t, repo, contracts.AuthorityModelGoalsRecoveryVersion, contracts.AuthorityModelGoalsRecoveryDigest(), now)
	before, err := repo.LoadAuthorityModelState(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	reopened, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	reopened.Crypto = repo.Crypto
	after, err := reopened.LoadAuthorityModelState(ctx, now.Add(2*time.Second))
	if err != nil || after != before || after.ActiveVersion != contracts.AuthorityModelGoalsRecoveryVersion || after.ActiveDigest != "sha256:fb63093de7b4f271d8b8ffb7229572d7887d5f6af7dd89191ef5283eb90fb53c" {
		t.Fatalf("persisted v5 state changed on reopen: %+v %+v %v", before, after, err)
	}
	if err := contracts.ValidateAuthorityModel(after.ActiveModel, after.ActiveVersion, after.ActiveDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.AdoptAuthorityModel(ctx, v6, bootstrapDigest, "test", "ADOPT "+digest, now.Add(3*time.Second)); err == nil {
		t.Fatal("a v5 installation must not adopt v6 at this time")
	}
}
