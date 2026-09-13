package workspace

import "testing"

func TestAuthorizeReleaseRejectsSensitiveDestinationMismatch(t *testing.T) {
	d := Destination{ID: "remote-model", Remote: true, AllowedLevels: map[Sensitivity]bool{SensitivityPublic: true}}
	if err := AuthorizeRelease(SensitivitySecret, d); err == nil {
		t.Fatal("secret content must not be released to unauthorized destination")
	}
}

func TestAuthorizeReleaseRequiresPQWhenPolicyDemands(t *testing.T) {
	d := Destination{ID: "remote-model", Remote: true, AllowedLevels: map[Sensitivity]bool{SensitivityConfidential: true}, PQRequired: true, PQAvailable: false}
	if err := AuthorizeRelease(SensitivityConfidential, d); err == nil {
		t.Fatal("required PQ protection must fail closed when unavailable")
	}
}
