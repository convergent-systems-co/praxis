package goalstore

import (
	"context"
	"encoding/json"
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
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER wrong", now); err == nil {
		t.Fatal("confirmation mismatch must fail closed")
	}
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, owner.ID, "", "APPROVE-PUBLISHER "+previewDigest, now); err == nil {
		t.Fatal("missing OS user must fail closed")
	}
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, owner.ID, "someone-else", "APPROVE-PUBLISHER "+previewDigest, now); err == nil {
		t.Fatal("OS user mismatched with current installation root must fail closed")
	}
	approval, approvalDigest, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+previewDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	if approval.PreviewDigest != previewDigest || approval.GenerationTemplateDigest != generationDigest || approval.ApproverID != owner.ID {
		t.Fatalf("approval lost exact binding: %+v", approval)
	}
	root, err := repo.LoadCurrentInstallationRoot(context.Background(), bootstrapDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	if approval.Version != contracts.PublisherEnrollmentApprovalVersion || approval.ApproverOSUser != "test" || approval.ApprovalRootDigest != root.Digest || approval.ApprovalRootRef != root.Ref || approval.ApprovalRootVersion != root.Version || approval.ApprovalRootProvenanceRef != root.ProvenanceRef || approval.ApprovalRootProvenanceDigest != root.ProvenanceDigest {
		t.Fatalf("version 2 approval lost current root and OS-user provenance: %+v", approval)
	}
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+previewDigest, now); err == nil {
		t.Fatal("same-preview version 2 approval must not overwrite its immutable record")
	}
	loaded, err := repo.LoadPublisherEnrollmentApproval(context.Background(), previewDigest, now)
	if err != nil || loaded.PreviewDigest != previewDigest {
		t.Fatalf("approval was not recoverable: loaded=%+v err=%v", loaded, err)
	}
	loadedByDigest, err := repo.LoadPublisherEnrollmentApprovalByDigest(context.Background(), approvalDigest, now)
	if err != nil || loadedByDigest.PreviewDigest != previewDigest {
		t.Fatalf("approval digest lookup was not recoverable: loaded=%+v err=%v", loadedByDigest, err)
	}
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), preview, bootstrapDigest, "installation-owner:other", "test", "APPROVE-PUBLISHER "+previewDigest, now); err == nil {
		t.Fatal("owner substitution must fail closed")
	}
	legacy := approval
	legacy.Version = "1"
	legacy.ApproverOSUser = ""
	legacy.ApprovalRootRef = ""
	legacy.ApprovalRootVersion = ""
	legacy.ApprovalRootDigest = ""
	legacy.ApprovalRootProvenanceRef = ""
	legacy.ApprovalRootProvenanceDigest = ""
	legacyDigest, err := legacy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if legacyDigest != "sha256:18efde6c856cb36bf474abfca54a861929e6419d9eb4ab66f0db7f98f457120c" {
		t.Fatalf("legacy approval digest changed: %s", legacyDigest)
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacyJSON), "ApproverOSUser") || strings.Contains(string(legacyJSON), "ApprovalRoot") {
		t.Fatalf("version 2 fields changed the historical version 1 payload: %s", legacyJSON)
	}
	if _, err := repo.SavePublisherEnrollmentApproval(context.Background(), legacy, now); err == nil {
		t.Fatal("new writes of legacy approvals must fail")
	}
	if _, err := repo.savePublisherGovernance(context.Background(), legacy.ID, legacy.Version, legacy, now, nil); err != nil {
		t.Fatal(err)
	}
	if historical, err := repo.LoadPublisherEnrollmentApprovalByDigest(context.Background(), legacyDigest, now); err != nil || historical.Version != "1" {
		t.Fatalf("legacy approval must remain historically readable: approval=%+v err=%v", historical, err)
	}
	if preferred, err := repo.LoadPublisherEnrollmentApproval(context.Background(), previewDigest, now); err != nil || preferred.Version != contracts.PublisherEnrollmentApprovalVersion {
		t.Fatalf("inspection must prefer version 2: approval=%+v err=%v", preferred, err)
	}
	fresh := preview
	fresh.CreatedAt = now.Add(time.Second)
	freshDigest, err := fresh.Digest()
	if err != nil || freshDigest == previewDigest {
		t.Fatalf("fresh preview must have a new digest: %q %v", freshDigest, err)
	}
	direct := approval
	direct.ID = "publisher-enrollment-approval:" + freshDigest
	direct.PreviewDigest = freshDigest
	direct.IssuedAt = now.Add(time.Second)
	if _, err := direct.Digest(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SavePublisherEnrollmentApproval(context.Background(), direct, now.Add(time.Second)); err == nil {
		t.Fatal("direct version 2 approval write bypassed guarded owner approval")
	}
	if _, err := repo.LoadPublisherEnrollmentApproval(context.Background(), freshDigest, now.Add(time.Second)); err == nil {
		t.Fatal("direct version 2 approval write persisted an authorizing record")
	}
	if _, _, err := repo.ApprovePublisherEnrollment(context.Background(), fresh, bootstrapDigest, owner.ID, "test", "APPROVE-PUBLISHER "+freshDigest, now.Add(time.Second)); err != nil {
		t.Fatalf("fresh preview permits a new immutable approval: %v", err)
	}
	legacyOnlyPreview := preview
	legacyOnlyPreview.CreatedAt = now.Add(2 * time.Second)
	legacyOnlyDigest, err := legacyOnlyPreview.Digest()
	if err != nil {
		t.Fatal(err)
	}
	legacyOnly := legacy
	legacyOnly.ID = "publisher-enrollment-approval:" + legacyOnlyDigest
	legacyOnly.PreviewDigest = legacyOnlyDigest
	if _, err := repo.savePublisherGovernance(context.Background(), legacyOnly.ID, legacyOnly.Version, legacyOnly, now.Add(2*time.Second), nil); err != nil {
		t.Fatal(err)
	}
	if historical, err := repo.LoadPublisherEnrollmentApproval(context.Background(), legacyOnlyDigest, now.Add(2*time.Second)); err != nil || historical.Version != "1" {
		t.Fatalf("inspection must fall back to a historical version 1 approval: %+v %v", historical, err)
	}
}
