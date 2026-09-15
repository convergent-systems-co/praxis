package goalstore

import (
	"context"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestAuthorityRequestResolvesOnlyByCanonicalDigest(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Unix(1700000000, 0).UTC()
	request := contracts.AuthorityRequest{ID: "authority-request:digest-test", Version: "1", BaselineID: "goal", BaselineVersion: "1", BaselineDigest: "sha256:baseline", ProposalID: "proposal", ProposalVersion: "1", ProposalDigest: "sha256:proposal", ReviewRef: "review", ReviewVersion: "1", ReviewDigest: "sha256:review", RequestedAuthority: "test.authority", RequestedScope: "goal:goal", Reason: "test", Status: contracts.AuthorityRequestPending}
	digest, err := repo.SaveAuthorityRequest(context.Background(), request, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.LoadAuthorityRequestByDigest(context.Background(), digest, now)
	if err != nil || loaded.ID != request.ID {
		t.Fatalf("canonical request did not reload: %+v %v", loaded, err)
	}
	if _, err := repo.LoadAuthorityRequestByDigest(context.Background(), "sha256:0000000000000000000000000000000000000000000000000000000000000000", now); err == nil {
		t.Fatal("unknown request digest must fail closed")
	}
}
