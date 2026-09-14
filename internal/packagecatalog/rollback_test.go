package packagecatalog

import (
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestRollbackRequestBindsCurrentAndTargetGenerationClosures(t *testing.T) {
	current := []PackageIdentity{{PackageID: "research/root", Version: "2", ContentDigest: "sha256:root2"}, {PackageID: "shared/evidence", Version: "2", ContentDigest: "sha256:dep2"}}
	targets := []PackageIdentity{{PackageID: "shared/evidence", Version: "1", ContentDigest: "sha256:dep1"}, {PackageID: "research/root", Version: "1", ContentDigest: "sha256:root1"}}
	request, err := NewRollbackRequest("research/root", current, targets, contracts.PrincipalRef{ID: "owner", Kind: "user"}, "approval:rollback")
	if err != nil {
		t.Fatal(err)
	}
	if request.Current[0].PackageID != "research/root" || request.Current[1].PackageID != "shared/evidence" {
		t.Fatalf("current preconditions are not canonically ordered: %+v", request.Current)
	}
	request.Current[0].ContentDigest = "sha256:attacker-selected"
	if err := request.Validate(); err == nil {
		t.Fatal("mutated current generation precondition retained rollback authority")
	}
}
