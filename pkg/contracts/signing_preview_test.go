package contracts

import (
	"testing"
	"time"
)

func validSigningPreview() SigningPreview {
	now := time.Unix(1_700_000_000, 0).UTC()
	return SigningPreview{
		ID: "publisher-signing-preview:test", Version: "1", PackageID: "praxis.package.goals", PackageVersion: "1", PackageNamespace: "praxis.package",
		ManifestDigest: "sha256:" + repeatedHex('1'), ArtifactDigest: "sha256:" + repeatedHex('2'), ExecutableDigest: "sha256:" + repeatedHex('3'),
		InvocationContractDigest: "sha256:" + repeatedHex('4'), ExecutableBindingDigest: "sha256:" + repeatedHex('5'), PluginDefinitionDigest: "sha256:" + repeatedHex('6'), RuntimeID: "praxis.plugin.grpc", RuntimeVersion: "1", RuntimeDigest: "sha256:" + repeatedHex('7'),
		SourceIdentity: "commit:test/tree:test", BuilderIdentity: "builder/v1", PublisherPrincipal: FirstPartyPublisherPrincipal, PublisherGenerationDigest: "sha256:" + repeatedHex('8'), PublisherGenerationVersion: "1", PublicKeyDigest: "sha256:" + repeatedHex('9'), KeyID: "key-1", Algorithm: "ed25519",
		AuthorityGenerationRef: "authority-delegation:test", AuthorityGenerationVersion: "1", AuthorityGenerationDigest: "sha256:" + repeatedHex('a'), AuthorityModel: AuthorityModelID, AuthorityModelVersion: AuthorityModelSuccessorVersion, AuthorityModelDigest: AuthorityModelSuccessorDigest(), ParentRef: "installation-governance:test", ParentVersion: "1", ParentDigest: "sha256:" + repeatedHex('b'), AuthorityScope: "package-namespace:praxis.package",
		AuthorityRequestID: "request:test", AuthorityRequestVersion: "1", AuthorityRequestDigest: "sha256:" + repeatedHex('c'), AuthorityDecisionRef: "decision:test", AuthorityDecisionVersion: "1", AuthorityDecisionDigest: "sha256:" + repeatedHex('d'), ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
}

func repeatedHex(c byte) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = c
	}
	return string(out)
}

func TestSigningPreviewDigestBindsExactAuthority(t *testing.T) {
	p := validSigningPreview()
	digest, err := p.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	p.Digest = digest
	if err := p.VerifyDigest(); err != nil {
		t.Fatal(err)
	}
	p.AuthorityGenerationDigest = "sha256:" + repeatedHex('e')
	if err := p.VerifyDigest(); err == nil {
		t.Fatal("authority substitution must invalidate signing preview")
	}
}
