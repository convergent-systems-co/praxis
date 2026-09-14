package packagecatalog

import (
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestDeploymentRequestBindsExactDependencyFirstClosure(t *testing.T) {
	leaf := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "portable/leaf", Version: "1"}, []byte("leaf"), nil)
	middle := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "research/middle", Version: "2", Dependencies: []Dependency{{PackageID: "portable/leaf", Version: "1", Digest: leaf.Manifest().ContentDigest}}}, []byte("middle"), map[string]VerifiedPackage{"portable/leaf": leaf})
	root := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "delivery/root", Version: "3", Dependencies: []Dependency{{PackageID: "research/middle", Version: "2", Digest: middle.Manifest().ContentDigest}}}, []byte("root"), map[string]VerifiedPackage{"portable/leaf": leaf, "research/middle": middle})

	request, err := NewDeploymentRequest(root, map[string]VerifiedPackage{"research/middle": middle, "portable/leaf": leaf}, contracts.PrincipalRef{ID: "owner", Kind: "user"}, "approval:closure")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, pkg := range request.Packages {
		got = append(got, pkg.Manifest().PackageID)
	}
	want := []string{"portable/leaf", "research/middle", "delivery/root"}
	if len(got) != len(want) {
		t.Fatalf("deployment closure cardinality differs from reachable immutable closure: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("deployment order lost dependency topology: got %v want %v", got, want)
		}
	}

	request.Packages[0], request.Packages[1] = request.Packages[1], request.Packages[0]
	if err := request.Validate(); err == nil {
		t.Fatal("caller-reordered deployment closure crossed the intent boundary")
	}
}

func TestDeploymentRequestRejectsMissingAndUnreachablePackages(t *testing.T) {
	dependency := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "research/dependency", Version: "1"}, []byte("dependency"), nil)
	root := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "research/root", Version: "1", Dependencies: []Dependency{{PackageID: "research/dependency", Version: "1", Digest: dependency.Manifest().ContentDigest}}}, []byte("root"), map[string]VerifiedPackage{"research/dependency": dependency})
	actor := contracts.PrincipalRef{ID: "owner", Kind: "user"}
	if _, err := NewDeploymentRequest(root, nil, actor, "approval"); err == nil {
		t.Fatal("missing verified dependency must fail closed")
	}
	extra := verifiedFixture(t, Manifest{ContractVersion: ManifestContractCurrentVersion(), PackageID: "unreachable/extra", Version: "1"}, []byte("extra"), nil)
	if _, err := NewDeploymentRequest(root, map[string]VerifiedPackage{"research/dependency": dependency, "unreachable/extra": extra}, actor, "approval"); err == nil {
		t.Fatal("unreachable package must not be smuggled into an authorized deployment")
	}
}
