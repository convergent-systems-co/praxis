package goalstore

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/publisher"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// countingSigner proves the protected signer was (not) reached.
type countingSigner struct {
	praxiscrypto.MemoryPublisherSigner
	calls int
}

func (s *countingSigner) Sign(ctx context.Context, statement []byte) ([]byte, error) {
	s.calls++
	return s.MemoryPublisherSigner.Sign(ctx, statement)
}

func shaDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// signingFixture enrolls a real publisher generation in a real anchored store,
// delegates package.publish to exactly that generation and key, and builds a package.
func signingFixture(t *testing.T, authorityKeyDigest ...string) (a anchored, root, child contracts.AuthorityGeneration, generationDigest string, signer *countingSigner, built packagecatalog.BuiltPackage) {
	t.Helper()
	ctx := context.Background()
	a = anchoredFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	root = rootSuccessionFixture(t, a.repo, now)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	generation := contracts.PublisherGeneration{Version: contracts.PublisherGenerationVersion, Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, KeyID: "publisher-key-1", Algorithm: "ed25519", PublicKey: public, PublicKeyDigest: shaDigest(public), PackageNamespace: publishNamespace, Generation: "1", EffectiveAt: now, EnrollmentRef: "owner:enrollment:1", EnrollmentDigest: shaDigest([]byte("enrollment"))}
	generationDigest, err = generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "owner", Kind: "human"}
	enrollment := contracts.ActionIntent{Version: "v1", ID: "enroll-1", Actor: actor, Operation: "publisher.enroll", Target: generationDigest, Scope: "package:praxis.package"}
	enrollmentDigest, err := enrollment.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.DB().ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,1,1)`, "enroll-approval", actor.ID, actor.Kind, enrollmentDigest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.EnrollPublisherGeneration(ctx, generation, enrollment, "enroll-approval", now); err != nil {
		t.Fatal(err)
	}
	keyDigest := generation.PublicKeyDigest
	if len(authorityKeyDigest) > 0 {
		keyDigest = authorityKeyDigest[0]
	}
	child = publishDelegationOn(t, a, root, now, generationDigest, keyDigest, generation.Generation)
	input, err := goals.PackageBuildInput([]byte("candidate executable"))
	if err != nil {
		t.Fatal(err)
	}
	built, err = packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	return a, root, child, generationDigest, &countingSigner{MemoryPublisherSigner: praxiscrypto.MemoryPublisherSigner{ID: generation.KeyID, Private: private}}, built
}

func buildPreview(t *testing.T, a anchored, generationDigest string, built packagecatalog.BuiltPackage) (contracts.SigningPreview, error) {
	t.Helper()
	return publisher.BuildSigningPreview(context.Background(), a.store, a.repo, generationDigest, built, "commit:test", "builder:test", "", time.Now().UTC())
}

func TestSigningPathControlSignsUnderCurrentDelegatedAuthority(t *testing.T) {
	a, _, _, generationDigest, signer, built := signingFixture(t)
	preview, err := buildPreview(t, a, generationDigest, built)
	if err != nil {
		t.Fatalf("control: a current delegated generation must build a preview: %v", err)
	}
	if _, err := publisher.SignWithPreview(context.Background(), a.store, a.repo, signer, preview, built, time.Now().UTC()); err != nil || signer.calls != 1 {
		t.Fatalf("control: signing under current authority must reach the signer once: calls=%d err=%v", signer.calls, err)
	}
}

// N17 at BuildSigningPreview: a retired delegated generation builds no preview.
func TestInvalidatedDelegatedPublisherGenerationPreventsBuildSigningPreview(t *testing.T) {
	a, root, child, generationDigest, _, built := signingFixture(t)
	retirePublishChild(t, a, child, root)
	if _, err := buildPreview(t, a, generationDigest, built); err == nil {
		t.Fatal("N17: BuildSigningPreview accepted a retired delegated package.publish generation")
	}
}

// N17 at the final pre-sign revalidation: retired after the preview was built.
func TestInvalidatingTheGenerationAfterPreviewPreventsSignWithPreview(t *testing.T) {
	a, root, child, generationDigest, signer, built := signingFixture(t)
	preview, err := buildPreview(t, a, generationDigest, built)
	if err != nil {
		t.Fatal(err)
	}
	retirePublishChild(t, a, child, root)
	if _, err := publisher.SignWithPreview(context.Background(), a.store, a.repo, signer, preview, built, time.Now().UTC()); err == nil {
		t.Fatal("N17: SignWithPreview signed under a delegated generation retired after the preview")
	}
	if signer.calls != 0 {
		t.Fatalf("the protected signer must never be reached for a retired generation: %d calls", signer.calls)
	}
}

// Deleting the invalidation row after the retirement must not re-authorize signing.
func TestDeletingTheInvalidationRowDoesNotReauthorizeSignWithPreview(t *testing.T) {
	a, root, child, generationDigest, signer, built := signingFixture(t)
	preview, err := buildPreview(t, a, generationDigest, built)
	if err != nil {
		t.Fatal(err)
	}
	retirePublishChild(t, a, child, root)
	keylessDelete(t, a.store, authorityGenerationInvalidationNamespace, child.Ref, child.Version)
	if _, err := publisher.SignWithPreview(context.Background(), a.store, a.repo, signer, preview, built, time.Now().UTC()); err == nil || signer.calls != 0 {
		t.Fatalf("deleting the invalidation row re-authorized signing: calls=%d err=%v", signer.calls, err)
	}
}

// resealed returns preview with one field changed and a VALID digest, so the
// refusal must come from the guard under test and not from the digest check.
func resealed(t *testing.T, preview contracts.SigningPreview, mutate func(p *contracts.SigningPreview)) contracts.SigningPreview {
	t.Helper()
	mutate(&preview)
	digest, err := preview.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	preview.Digest = digest
	return preview
}

// Every pre-sign guard refuses, and the protected signer is never reached.
func TestSignWithPreviewRefusesEachStaleBindingBeforeTheProtectedSigner(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *contracts.SigningPreview)
	}{
		{"manifest bytes changed", func(p *contracts.SigningPreview) { p.ManifestDigest = "sha256:" + strings.Repeat("1", 64) }},
		{"artifact bytes changed", func(p *contracts.SigningPreview) { p.ArtifactDigest = "sha256:" + strings.Repeat("2", 64) }},
		{"publisher principal changed", func(p *contracts.SigningPreview) { p.PublisherPrincipal = "publisher:someone-else" }},
		{"authority scope changed", func(p *contracts.SigningPreview) { p.AuthorityScope = "package-namespace:other" }},
		{"authority generation digest changed", func(p *contracts.SigningPreview) {
			p.AuthorityGenerationDigest = "sha256:" + strings.Repeat("3", 64)
		}},
		{"authority request digest changed", func(p *contracts.SigningPreview) {
			p.AuthorityRequestDigest = "sha256:" + strings.Repeat("4", 64)
		}},
		{"authority decision digest changed", func(p *contracts.SigningPreview) {
			p.AuthorityDecisionDigest = "sha256:" + strings.Repeat("5", 64)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _, generationDigest, signer, built := signingFixture(t)
			preview, err := buildPreview(t, a, generationDigest, built)
			if err != nil {
				t.Fatal(err)
			}
			forged := resealed(t, preview, tc.mutate)
			if _, err := publisher.SignWithPreview(context.Background(), a.store, a.repo, signer, forged, built, time.Now().UTC()); err == nil || signer.calls != 0 {
				t.Fatalf("a stale binding reached the protected signer: calls=%d err=%v", signer.calls, err)
			}
		})
	}
	t.Run("preview digest not resealed", func(t *testing.T) {
		a, _, _, generationDigest, signer, built := signingFixture(t)
		preview, err := buildPreview(t, a, generationDigest, built)
		if err != nil {
			t.Fatal(err)
		}
		preview.SourceIdentity = "tampered:source" // covered only by the preview digest
		if _, err := publisher.SignWithPreview(context.Background(), a.store, a.repo, signer, preview, built, time.Now().UTC()); err == nil || signer.calls != 0 {
			t.Fatalf("a tampered preview reached the protected signer: calls=%d err=%v", signer.calls, err)
		}
	})
	t.Run("the protected signer is not the previewed key", func(t *testing.T) {
		a, _, _, generationDigest, signer, built := signingFixture(t)
		preview, err := buildPreview(t, a, generationDigest, built)
		if err != nil {
			t.Fatal(err)
		}
		wrong := &countingSigner{MemoryPublisherSigner: praxiscrypto.MemoryPublisherSigner{ID: "some-other-key", Private: signer.Private}}
		if _, err := publisher.SignWithPreview(context.Background(), a.store, a.repo, wrong, preview, built, time.Now().UTC()); err == nil || wrong.calls != 0 {
			t.Fatalf("a different signing key reached the protected signer: calls=%d err=%v", wrong.calls, err)
		}
	})
}

// BuildSigningPreview refuses a package outside the publisher's namespace and an
// authority whose key is not the publisher generation's key.
func TestBuildSigningPreviewRefusesForeignNamespaceAndMismatchedAuthorityKey(t *testing.T) {
	t.Run("package outside the publisher namespace", func(t *testing.T) {
		a, _, _, generationDigest, _, built := signingFixture(t)
		foreign := built
		foreign.Manifest.PackageID = "other.namespace.pkg"
		if _, err := buildPreview(t, a, generationDigest, foreign); err == nil {
			t.Fatal("a package outside the publisher's namespace produced a signing preview")
		}
	})
	t.Run("authority key differs from the publisher key", func(t *testing.T) {
		a, _, _, generationDigest, _, built := signingFixture(t, "sha256:"+strings.Repeat("7", 64))
		if _, err := buildPreview(t, a, generationDigest, built); err == nil {
			t.Fatal("an authority delegated to a different key produced a signing preview")
		}
	})
}

// The publisher generation's OWN namespace is enforced independently of the
// delegated authority's: an authority delegated for a wider namespace does not
// let the publisher sign a package outside the namespace it was enrolled for.
func TestBuildSigningPreviewEnforcesThePublisherGenerationsOwnNamespace(t *testing.T) {
	previous := delegatedNamespace
	t.Cleanup(func() { delegatedNamespace = previous })
	delegatedNamespace = "praxis" // wider than the publisher generation's "praxis.package"
	a, _, _, generationDigest, _, built := signingFixture(t)
	if _, err := a.repo.ResolvePackagePublishAuthority(context.Background(), generationDigest, "praxis.other", time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatalf("premise: the wider authority resolves the package: %v", err)
	}
	// A fully internally consistent manifest (package identity matches its own
	// invocation, so it passes Manifest.Validate) for a package inside the widened
	// authority namespace but outside the publisher generation's own namespace.
	input, err := goals.PackageBuildInput([]byte("candidate executable"))
	if err != nil {
		t.Fatal(err)
	}
	input.Manifest.PackageID = "praxis.other"
	for i := range input.Manifest.Invocations {
		input.Manifest.Invocations[i].PackageID = "praxis.other"
	}
	foreign, err := packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildPreview(t, a, generationDigest, foreign); err == nil {
		t.Fatal("a package outside the publisher generation's own namespace produced a signing preview")
	}
	if _, err := buildPreview(t, a, generationDigest, built); err != nil {
		t.Fatalf("control: a package inside both namespaces must build a preview: %v", err)
	}
}

// SignWithPreview refuses to sign once the delegated authority has expired,
// even though the preview itself is otherwise exactly as built.
func TestSignWithPreviewRefusesExpiredAuthorityBeforeSigning(t *testing.T) {
	a, _, _, generationDigest, signer, built := signingFixture(t)
	preview, err := buildPreview(t, a, generationDigest, built)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.SignWithPreview(context.Background(), a.store, a.repo, signer, preview, built, time.Now().UTC()); err != nil {
		t.Fatalf("control: signing before expiry must succeed: %v", err)
	}
	if signer.calls != 1 {
		t.Fatalf("control: the signer must be reached exactly once: %d", signer.calls)
	}
}

// The same, isolated from the resolver's own currentness check: the delegated
// authority's ExpiresAt has passed, but nothing about the generation is retired.
func TestSignWithPreviewRefusesAtTheExactExpiryBoundary(t *testing.T) {
	a, _, child, generationDigest, signer, built := signingFixture(t)
	preview, err := buildPreview(t, a, generationDigest, built)
	if err != nil {
		t.Fatal(err)
	}
	if child.ExpiresAt == nil {
		t.Fatal("premise: the delegated generation must carry an expiry")
	}
	past := child.ExpiresAt.UTC().Add(time.Second)
	_, err = publisher.SignWithPreview(context.Background(), a.store, a.repo, signer, preview, built, past)
	t.Logf("refused with: %v", err)
	if err == nil {
		t.Fatal("SignWithPreview signed past the delegated authority's expiry")
	}
	if signer.calls != 0 {
		t.Fatalf("the protected signer must not be reached past expiry: %d calls", signer.calls)
	}
}
