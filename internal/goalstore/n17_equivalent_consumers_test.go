package goalstore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Equivalent-path search for N17: consumers that select an authority generation
// by its immutable State must apply the same currentness predicate.

func TestRetiredPackageManagerGenerationDoesNotResolveDeploymentAuthority(t *testing.T) {
	repo, root, child, now := packageDeployDelegationFixture(t)
	ctx := context.Background()
	activateAuthorityModelRecordForTest(t, repo, "active-authority-model-v3", contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelDeploymentDigest(), now.Add(time.Millisecond))
	at := now.Add(time.Second)
	if got, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, at); err != nil || got.Digest != child.Digest {
		t.Fatalf("control: a current package-manager generation must resolve: %v", err)
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: child.Ref, Version: child.Version, GenerationDigest: child.Digest, InvalidationRef: "retire-" + child.Ref, InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
	if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, at.Add(time.Second)); err == nil {
		t.Fatal("N17-equivalent: a retired package-manager generation still resolves package.deploy authority")
	}
}

// G7: approving a publisher enrollment requires the CURRENT installation root.
// The check accepted any root-shaped generation, retired or not.
func TestRetiredInstallationRootCannotApproveAPublisherEnrollment(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	adoption, bootstrapDigest := adoptionFixture(t, repo, now)
	if _, err := repo.AdoptAuthorityModel(ctx, adoption, bootstrapDigest, "test", "ADOPT "+mustDigest(t, adoption), now); err != nil {
		t.Fatal(err)
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	generation := contracts.PublisherGeneration{Version: contracts.PublisherGenerationVersion, Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, KeyID: "key:publisher:1", Algorithm: "ed25519", PublicKeyDigest: "sha256:" + strings.Repeat("1", 64), PackageNamespace: "praxis.package", Generation: "1", EffectiveAt: now, EnrollmentRef: "publisher-enrollment-preview:1", EnrollmentDigest: "sha256:" + strings.Repeat("2", 64)}
	generationDigest, err := generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	preview := contracts.PublisherEnrollmentPreview{ID: "publisher-enrollment-preview:" + generationDigest, Version: "1", BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelSuccessorVersion, AuthorityModelDigest: contracts.AuthorityModelSuccessorDigest(), PublisherPrincipal: generation.Principal.ID, KeyID: generation.KeyID, Algorithm: generation.Algorithm, KeyPurpose: "publisher-signing", PublicKeyDigest: generation.PublicKeyDigest, Generation: generation.Generation, Namespace: generation.PackageNamespace, GenerationDigest: generationDigest, GenerationRecord: generation, CreatedAt: now}
	previewDigest, err := preview.Digest()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now)
	if err != nil {
		t.Fatalf("premise: the installation has a current root: %v", err)
	}
	if _, _, err := repo.ApprovePublisherEnrollment(ctx, preview, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+previewDigest, now); err != nil {
		t.Fatalf("control: a current root must approve: %v", err)
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: root.Ref, Version: root.Version, GenerationDigest: root.Digest, InvalidationRef: "retire-root", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
	if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	preview2 := preview
	preview2.CreatedAt = now.Add(time.Second)
	preview2Digest, err := preview2.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.ApprovePublisherEnrollment(ctx, preview2, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+preview2Digest, now.Add(2*time.Second)); err == nil {
		t.Fatal("G7: a retired installation root approved a publisher enrollment")
	}
}

// G3: the operational (package-manager) generation bound by a deployment
// decision must be CURRENT when the approval is derived. Only the issuing root
// was validated; the manager generation was compared field by field.
func TestRetiredPackageManagerGenerationCannotDeriveADeploymentApproval(t *testing.T) {
	retireManagerBeforeTheDecisionIsSaved(t, false)
}

// The same generation retired BEFORE the decision is saved: the decision itself
// is refused, independent of the later approval derivation.
func TestRetiredPackageManagerGenerationCannotBeBoundByADeploymentDecision(t *testing.T) {
	retireManagerBeforeTheDecisionIsSaved(t, true)
}

func retireManagerBeforeTheDecisionIsSaved(t *testing.T, beforeSave bool) {
	t.Helper()
	repo, root, manager, now := packageDeployDelegationFixture(t)
	ctx := context.Background()
	activateAuthorityModelRecordForTest(t, repo, "active-authority-model-v6", contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest(), now)
	verifiedAt := now.Add(time.Second)
	verified := verifiedPackageFixture(t, verifiedAt)
	deployment, err := packagecatalog.NewDeploymentRequest(verified, nil, contracts.PackageManagerPrincipal(), "pending")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := packagecatalog.NewVerificationEvidenceRecord(deployment.Root, deployment.Packages, root.Digest, verifiedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Store.SaveVerificationEvidence(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	deployment.VerificationEvidenceDigest = evidence.ID
	deployment.Intent.Parameters["verification_evidence_digest"] = evidence.ID
	at := now.Add(2 * time.Second)
	request, requestDigest, err := repo.SavePackageDeploymentRequest(ctx, deployment.Intent, root.Digest, deployment.Intent.Parameters["closure_digest"], evidence.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	expires := at.Add(24 * time.Hour)
	if manager.ExpiresAt != nil && manager.ExpiresAt.Before(expires) {
		expires = manager.ExpiresAt.UTC()
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: root.AuthorityModelDigest, IssuedAt: at, ExpiresAt: &expires, OperationalAuthorityRef: manager.Ref, OperationalAuthorityVersion: manager.Version, OperationalAuthorityGenerationDigest: manager.Digest}
	retire := func() {
		invalidation := contracts.AuthorityGenerationInvalidation{Ref: manager.Ref, Version: manager.Version, GenerationDigest: manager.Digest, InvalidationRef: "retire-manager", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
		if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
			t.Fatal(err)
		}
	}
	if beforeSave {
		retire()
		if err := repo.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, at, &expires); err == nil {
			t.Fatal("G3: a deployment decision bound a retired package-manager generation")
		}
		return
	}
	if err := repo.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, at, &expires); err != nil {
		t.Fatal(err)
	}
	retire()
	if _, err := repo.DerivePackageDeploymentApproval(ctx, request.ID, request.Version, deployment.Intent, at.Add(time.Second)); err == nil {
		t.Fatal("G3: an approval was derived from a retired package-manager generation")
	}
}

// A manager generation whose ROOT was retired is not deployment authority even
// though the manager generation's own records are all intact.
func TestPackageManagerGenerationOfARetiredRootDoesNotResolveDeploymentAuthority(t *testing.T) {
	repo, root, child, now := packageDeployDelegationFixture(t)
	ctx := context.Background()
	activateAuthorityModelRecordForTest(t, repo, "active-authority-model-v3", contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelDeploymentDigest(), now.Add(time.Millisecond))
	at := now.Add(time.Second)
	if got, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, at); err != nil || got.Digest != child.Digest {
		t.Fatalf("control: %v", err)
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: root.Ref, Version: root.Version, GenerationDigest: root.Digest, InvalidationRef: "retire-root", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
	if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, at.Add(time.Second)); err == nil {
		t.Fatal("a manager generation whose root was retired still resolves package.deploy authority")
	}
}

// The check both enrollment sites share: only the CURRENT root of THIS owner,
// and, for enrollment, only its enrolled OS user.
func TestCurrentInstallationOwnerCheckRequiresTheCurrentRootOwnerAndOSUser(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	adoption, bootstrapDigest := adoptionFixture(t, repo, now)
	if _, err := repo.AdoptAuthorityModel(ctx, adoption, bootstrapDigest, "test", "ADOPT "+mustDigest(t, adoption), now); err != nil {
		t.Fatal(err)
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	osUser := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	if err := repo.requireCurrentInstallationOwner(ctx, bootstrapDigest, owner, osUser, now); err != nil {
		t.Fatalf("control: the current root's owner and OS user must pass: %v", err)
	}
	if err := repo.requireCurrentInstallationOwner(ctx, bootstrapDigest, owner, "", now); err == nil {
		t.Fatal("approval without an OS user must fail closed")
	}
	if err := repo.requireCurrentInstallationOwner(ctx, bootstrapDigest, owner, "someone-else", now); err == nil {
		t.Fatal("enrollment accepted an OS user that is not the root's enrolled user")
	}
	if err := repo.requireCurrentInstallationOwner(ctx, bootstrapDigest, contracts.PrincipalRef{ID: "installation-owner:other", Kind: "human"}, osUser, now); err == nil {
		t.Fatal("another principal was accepted as the installation owner")
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: root.Ref, Version: root.Version, GenerationDigest: root.Digest, InvalidationRef: "retire-root", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
	if err := repo.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.requireCurrentInstallationOwner(ctx, bootstrapDigest, owner, osUser, now.Add(time.Second)); err == nil {
		t.Fatal("a retired root authenticated its enrolled OS user for enrollment")
	}
	if err := repo.requireCurrentInstallationOwner(ctx, bootstrapDigest, owner, osUser, now.Add(time.Second)); err == nil {
		t.Fatal("a retired root approved as installation owner")
	}
}

// The re-anchor ceremony re-stamps the attested root's liveness. A root that
// carries an invalidation record is never re-stamped, even when no anchored
// retirement fact names it (a pre-anchor-era invalidation). The candidate
// derivation already excludes such a root, so the readmission step is exercised
// directly, after a re-anchor whose readmission was interrupted.
func TestReanchorReadmissionRefusesARootCarryingAnInvalidationRecord(t *testing.T) {
	crashed := func(t *testing.T) (anchored, contracts.AuthorityGeneration, Repository) {
		t.Helper()
		ctx := context.Background()
		a := anchoredFixture(t)
		root := rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
		a.anchor.Restore(successionTestBootstrap, nil)
		r := a.reopen(t)
		plan, err := r.PlanReanchor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:" + strings.Repeat("c", 64)}); err != nil {
			t.Fatal(err)
		}
		// The crash: the re-stamped liveness is lost before readmission completes.
		if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace=?`, state.AuthorityGenerationLiveNamespace); err != nil {
			t.Fatal(err)
		}
		return a, root, a.reopen(t)
	}
	t.Run("control: an interrupted readmission of an intact root completes", func(t *testing.T) {
		a, root, after := crashed(t)
		if err := after.completeRootReadmission(context.Background(), time.Now().UTC()); err != nil {
			t.Fatalf("readmission of an intact root must complete: %v", err)
		}
		if got, err := a.reopen(t).LoadCurrentInstallationRoot(context.Background(), successionTestBootstrap, time.Now().UTC()); err != nil || got.Digest != root.Digest {
			t.Fatalf("the root must be current after readmission: %v", err)
		}
	})
	t.Run("a root carrying an invalidation record is not re-stamped", func(t *testing.T) {
		a, root, after := crashed(t)
		invalidation := contracts.AuthorityGenerationInvalidation{Ref: root.Ref, Version: root.Version, GenerationDigest: root.Digest, InvalidationRef: "pre-anchor-retirement", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test"}
		payload, err := json.Marshal(invalidation)
		if err != nil {
			t.Fatal(err)
		}
		if err := after.putWorkPlanBlob(context.Background(), authorityGenerationInvalidationNamespace, invalidation.Ref, invalidation.Version, payload, time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
		if err := after.completeRootReadmission(context.Background(), time.Now().UTC()); err == nil {
			t.Fatal("readmission re-stamped the liveness of a root carrying an invalidation record")
		}
		if _, err := a.reopen(t).LoadCurrentInstallationRoot(context.Background(), successionTestBootstrap, time.Now().UTC()); err == nil {
			t.Fatal("a root carrying an invalidation record became current again")
		}
	})
}
