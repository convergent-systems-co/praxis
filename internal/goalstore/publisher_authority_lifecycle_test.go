package goalstore

import (
	"context"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestPublisherAuthorityProposalAndReviewBindExactLineage(t *testing.T) {
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
	proposal := contracts.PublisherAuthorityProposal{ID: "publisher-authority-proposal:test", Version: "1", Kind: contracts.PublisherAuthorityProposalKind, BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelSuccessorVersion, AuthorityModelDigest: contracts.AuthorityModelSuccessorDigest(), PublisherGenerationDigest: "sha256:" + strings.Repeat("1", 64), PublisherPrincipal: contracts.FirstPartyPublisherPrincipal, PublicKeyDigest: "sha256:" + strings.Repeat("2", 64), Namespace: "praxis.package", ParentRef: "installation-governance:root", ParentVersion: "1", ParentDigest: "sha256:" + strings.Repeat("3", 64), Capability: contracts.GovernedPackagePublish, Scope: "package-namespace:praxis.package", Reason: "test", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	proposalDigest, err := repo.SavePublisherAuthorityProposal(context.Background(), proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.LoadPublisherAuthorityProposalByDigest(context.Background(), proposalDigest, now)
	if err != nil || loaded.Namespace != proposal.Namespace {
		t.Fatalf("proposal reload failed: %+v %v", loaded, err)
	}
	review := contracts.PublisherAuthorityReview{ID: "publisher-authority-review:" + proposalDigest, Version: "1", Kind: contracts.PublisherAuthorityReviewKind, ProposalID: proposal.ID, ProposalVersion: proposal.Version, ProposalDigest: proposalDigest, ReviewedBy: owner.ID, ReviewedKind: owner.Kind, Decision: "approve", Namespace: proposal.Namespace, PublisherGenerationDigest: proposal.PublisherGenerationDigest, ReviewedAt: now.Add(time.Minute)}
	reviewDigest, err := repo.SavePublisherAuthorityReview(context.Background(), review, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadPublisherAuthorityReviewByDigest(context.Background(), reviewDigest, now); err != nil {
		t.Fatal(err)
	}
	wrong := review
	wrong.ProposalDigest = "sha256:" + strings.Repeat("4", 64)
	if _, err := repo.SavePublisherAuthorityReview(context.Background(), wrong, now); err == nil {
		t.Fatal("review substitution must fail closed")
	}
}
