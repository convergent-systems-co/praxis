package goalstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type approvalGateSigner struct{ publicKeyReads int }

func (*approvalGateSigner) KeyID() string     { return "key:publisher:1" }
func (*approvalGateSigner) Algorithm() string { return "ed25519" }
func (s *approvalGateSigner) PublicKey(context.Context) ([]byte, error) {
	s.publicKeyReads++
	return nil, errors.New("protected key must not be read")
}
func (*approvalGateSigner) Sign(context.Context, []byte) ([]byte, error) {
	return nil, errors.New("protected key must not be used")
}

func TestPublisherEnrollmentRejectsLegacyAndStaleRootApprovalsBeforeProtectedKey(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	bootstrap := praxiscrypto.BootstrapRecord{
		Version: praxiscrypto.BootstrapRecordVersion, ProviderID: "fixture", KeyID: "bootstrap:key", KeyVersion: "1",
		KeyMaterialHash: "sha256:" + strings.Repeat("a", 64), Owner: "test", Purpose: "installation-governance",
		Profile: contracts.CryptoPQRequired, SecurityLevel: praxiscrypto.SecurityPlatformProtected,
		Platform: "darwin", Architecture: "arm64", CreatedAt: now,
	}
	bootstrapDigest, err := bootstrap.Digest()
	if err != nil {
		t.Fatal(err)
	}
	repo.InstallationDigest = bootstrapDigest
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	root := authorityGenerationFixture(now)
	root.Principal = owner
	root.Scope, err = contracts.InstallationGovernanceScope(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	root.Ref = root.Scope
	root.ProvenanceRef = "bootstrap-record:" + bootstrapDigest + ":os-user:test"
	root.ProvenanceDigest = bootstrapDigest
	root.Digest, err = root.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, root, now, nil); err != nil {
		t.Fatal(err)
	}
	adoption := contracts.AuthorityModelAdoption{
		ID: "authority-model-adoption:v1-to-v2", Version: "1", FromModel: contracts.AuthorityModelID,
		FromVersion: contracts.AuthorityModelVersion, FromDigest: contracts.AuthorityModelDigest(),
		ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelSuccessorVersion,
		ToDigest: contracts.AuthorityModelSuccessorDigest(), RootRef: root.Ref, RootVersion: root.Version,
		RootDigest: root.Digest, Reason: "adopt authority model", CreatedAt: now,
	}
	if _, err := repo.AdoptAuthorityModel(ctx, adoption, bootstrapDigest, "test", "ADOPT "+mustDigest(t, adoption), now); err != nil {
		t.Fatal(err)
	}
	generation := contracts.PublisherGeneration{
		Version: contracts.PublisherGenerationVersion, Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"},
		KeyID: "key:publisher:1", Algorithm: "ed25519", PublicKeyDigest: "sha256:" + strings.Repeat("1", 64),
		PackageNamespace: "praxis.package", Generation: "1", EffectiveAt: now,
		EnrollmentRef: "publisher-enrollment-preview:1", EnrollmentDigest: "sha256:" + strings.Repeat("2", 64),
	}
	generationDigest, err := generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	preview := contracts.PublisherEnrollmentPreview{
		ID: "publisher-enrollment-preview:" + generationDigest, Version: "1", BootstrapDigest: bootstrapDigest,
		OwnerID: owner.ID, OwnerKind: owner.Kind, AuthorityModel: contracts.AuthorityModelID,
		AuthorityModelVersion: contracts.AuthorityModelSuccessorVersion, AuthorityModelDigest: contracts.AuthorityModelSuccessorDigest(),
		PublisherPrincipal: generation.Principal.ID, KeyID: generation.KeyID, Algorithm: generation.Algorithm,
		KeyPurpose: "publisher-signing", PublicKeyDigest: generation.PublicKeyDigest, Generation: generation.Generation,
		Namespace: generation.PackageNamespace, GenerationDigest: generationDigest, GenerationRecord: generation, CreatedAt: now,
	}
	previewDigest, err := preview.Digest()
	if err != nil {
		t.Fatal(err)
	}
	approval, approvalDigest, err := repo.ApprovePublisherEnrollment(ctx, preview, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+previewDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	legacy := approval
	legacy.Version = "1"
	legacy.ApproverOSUser, legacy.ApprovalRootRef, legacy.ApprovalRootVersion, legacy.ApprovalRootDigest = "", "", "", ""
	legacy.ApprovalRootProvenanceRef, legacy.ApprovalRootProvenanceDigest = "", ""
	legacyDigest, err := legacy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.savePublisherGovernance(ctx, legacy.ID, legacy.Version, legacy, now, nil); err != nil {
		t.Fatal(err)
	}
	signer := &approvalGateSigner{}
	if _, err := repo.EnrollPublisherFromApproval(ctx, legacyDigest, bootstrap, signer, "test", now); err == nil || !strings.Contains(err.Error(), "legacy") {
		t.Fatalf("legacy approval must fail closed: %v", err)
	}
	if signer.publicKeyReads != 0 {
		t.Fatal("legacy approval reached protected key")
	}
	proposal, err := contracts.BuildRootAuthoritySuccession(root, bootstrapDigest, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := repo.SaveRootAuthoritySuccessionProposal(ctx, proposal, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, owner, "test", "REVIEW-ROOT-SUCCESSOR "+proposalDigest, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	successor, _, err := repo.AcceptRootAuthoritySuccession(ctx, proposalDigest, reviewDigest, bootstrapDigest, "test", "ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewDigest, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if successor.Digest == approval.ApprovalRootDigest {
		t.Fatal("root succession did not replace approved root")
	}
	if _, _, err := repo.ApprovePublisherEnrollment(ctx, preview, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+previewDigest, now.Add(4*time.Second)); err == nil {
		t.Fatal("same preview cannot replace immutable stale-root approval")
	}
	fresh := preview
	fresh.CreatedAt = now.Add(5 * time.Second)
	freshDigest, err := fresh.Digest()
	if err != nil || freshDigest == previewDigest {
		t.Fatalf("fresh preview digest required after root succession: %q %v", freshDigest, err)
	}
	freshApproval, _, err := repo.ApprovePublisherEnrollment(ctx, fresh, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+freshDigest, now.Add(5*time.Second))
	if err != nil || freshApproval.ApprovalRootDigest != successor.Digest {
		t.Fatalf("fresh approval must bind successor root: %+v %v", freshApproval, err)
	}
	if _, err := repo.EnrollPublisherFromApproval(ctx, approvalDigest, bootstrap, signer, "test", now.Add(4*time.Second)); err == nil || !strings.Contains(err.Error(), "root is no longer current") {
		t.Fatalf("stale-root approval must fail closed: %v", err)
	}
	if signer.publicKeyReads != 0 {
		t.Fatal("stale-root approval reached protected key")
	}
	if _, err := repo.Store.PublisherGeneration(ctx, generationDigest); err == nil {
		t.Fatal("rejected approval committed canonical publisher generation")
	}
}
