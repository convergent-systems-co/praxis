package packagecatalog

import "testing"

func TestVerifiedPackageCannotSkipInspectionAuthorization(t *testing.T) {
	if CanTransitionInstall(PackageVerified, PackageInstalled) {
		t.Fatal("verification must not imply installation authority")
	}
}

func TestUpdateCapabilityExpansionRequiresReauthorization(t *testing.T) {
	review := ReviewUpdate([]string{"workspace.read"}, []string{"workspace.read", "network.write"}, false, false)
	if !review.RequiresReauthorization || len(review.AddedCapabilities) != 1 || review.AddedCapabilities[0] != "network.write" {
		t.Fatalf("unexpected update review: %+v", review)
	}
}

func TestRemovingCapabilityAloneDoesNotRequireReauthorization(t *testing.T) {
	review := ReviewUpdate([]string{"workspace.read", "network.write"}, []string{"workspace.read"}, false, false)
	if review.RequiresReauthorization {
		t.Fatalf("capability reduction alone should not expand authority: %+v", review)
	}
}
