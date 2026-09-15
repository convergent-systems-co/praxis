package contracts

import (
	"testing"
	"time"
)

func validPublisherGeneration() PublisherGeneration {
	now := time.Unix(100, 0).UTC()
	return PublisherGeneration{
		Version: PublisherGenerationVersion, Principal: PrincipalRef{ID: FirstPartyPublisherPrincipal, Kind: "publisher"},
		KeyID: "key:praxis-first-party:1", Algorithm: "ed25519", PublicKeyDigest: "sha256:" + "1" + string(make([]byte, 63)),
		PackageNamespace: "praxis.package", Generation: "1", EffectiveAt: now,
		EnrollmentRef: "enrollment:1", EnrollmentDigest: "sha256:" + "2" + string(make([]byte, 63)),
	}
}

func TestPublisherGenerationSeparatesIdentityScopeAndAuthority(t *testing.T) {
	p := validPublisherGeneration()
	// Construct exact hexadecimal digests without introducing secret material.
	p.PublicKeyDigest = "sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"
	p.EnrollmentDigest = "sha256:" + "2222222222222222222222222222222222222222222222222222222222222222"
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if digest, err := p.Digest(); err != nil || digest == "" {
		t.Fatal("publisher generation must have durable identity")
	}
	if !p.PackageNamespaceAllowed("praxis.package.goals") || p.PackageNamespaceAllowed("other.package") {
		t.Fatal("publisher namespace scope is not enforced")
	}
	if got := CanonicalPublisherCapabilities(); len(got) != 1 || got[0] != PackagePublishCapability {
		t.Fatal("publisher capability scope is not canonical")
	}
}

func TestPublisherGenerationRejectsInstallationPrincipal(t *testing.T) {
	p := validPublisherGeneration()
	p.Principal = PrincipalRef{ID: "installation-owner:one", Kind: "human"}
	if err := p.Validate(); err == nil {
		t.Fatal("installation governance principal must not become publisher")
	}
}
