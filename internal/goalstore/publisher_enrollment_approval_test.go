package goalstore

import (
	"context"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestApprovePublisherEnrollmentBindsPreviewAndOwner(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	adoption, bootstrapDigest := adoptionFixture(t, repo, now)
	if _, err := repo.AdoptAuthorityModel(context.Background(), adoption, bootstrapDigest, "test", "ADOPT "+mustDigest(t, adoption), now); err != nil {
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
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, owner.ID, "APPROVE-PUBLISHER wrong", now); err == nil {
		t.Fatal("confirmation mismatch must fail closed")
	}
	approval, approvalDigest, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, owner.ID, "APPROVE-PUBLISHER "+previewDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	if approval.PreviewDigest != previewDigest || approval.GenerationTemplateDigest != generationDigest || approval.ApproverID != owner.ID {
		t.Fatalf("approval lost exact binding: %+v", approval)
	}
	loaded, err := repo.LoadPublisherEnrollmentApproval(context.Background(), previewDigest, now)
	if err != nil || loaded.PreviewDigest != previewDigest {
		t.Fatalf("approval was not recoverable: loaded=%+v err=%v", loaded, err)
	}
	loadedByDigest, err := repo.LoadPublisherEnrollmentApprovalByDigest(context.Background(), approvalDigest, now)
	if err != nil || loadedByDigest.PreviewDigest != previewDigest {
		t.Fatalf("approval digest lookup was not recoverable: loaded=%+v err=%v", loadedByDigest, err)
	}
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, "installation-owner:other", "APPROVE-PUBLISHER "+previewDigest, now); err == nil {
		t.Fatal("owner substitution must fail closed")
	}
}
