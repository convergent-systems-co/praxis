// Package goalspublication implements only SPEC-041's fixed Goals transition.
package goalspublication

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/publisher"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var assetNames = []string{"praxis-package.json", "praxis-package.tar.gz", "praxis-package.sig.json"}
var assetDigests = []string{contracts.GoalsPublicationManifest, contracts.GoalsPublicationArchive, contracts.GoalsPublicationSignature}

func hash(b []byte) string { s := sha256.Sum256(b); return fmt.Sprintf("sha256:%x", s) }

type Assets [3][]byte

func ReadAssets(dir string) (Assets, error) {
	var a Assets
	for i, n := range assetNames {
		b, e := os.ReadFile(filepath.Join(dir, n))
		if e != nil {
			return a, e
		}
		a[i] = b
	}
	return a, a.Validate()
}
func (a Assets) Validate() error {
	for i, b := range a {
		if hash(b) != assetDigests[i] {
			return fmt.Errorf("wrong signed Goals asset: %s", assetNames[i])
		}
	}
	return nil
}
func (a Assets) Match(intent contracts.ActionIntent) error {
	if e := a.Validate(); e != nil {
		return e
	}
	for i, k := range []string{"manifest_size", "archive_size", "signature_size"} {
		if fmt.Sprint(len(a[i])) != intent.Parameters[k] {
			return errors.New("asset size differs from authorized intent")
		}
	}
	return nil
}

func (a Assets) MatchRecovery(intent contracts.ActionIntent) error {
	if err := contracts.ValidateGoalsPublicationRecoveryIntent(intent); err != nil {
		return err
	}
	if err := a.Validate(); err != nil {
		return err
	}
	for i, k := range []string{"manifest_size", "archive_size", "signature_size"} {
		if fmt.Sprint(len(a[i])) != intent.Parameters[k] {
			return errors.New("successor asset size differs from exact intent")
		}
	}
	return nil
}

// VerifySigning loads the original receipt and exact historical authorization.
// It never calls a signer or substitutes today's publishing authorization.
func VerifySigning(ctx context.Context, r goalstore.Repository, a Assets, now time.Time) error {
	if e := a.Validate(); e != nil {
		return e
	}
	rec, e := r.Store.PublisherSigningReceipt(ctx, contracts.GoalsPublicationSigningReceipt)
	if e != nil {
		return e
	}
	b, e := json.Marshal(rec["provenance"])
	if e != nil {
		return e
	}
	var p publisher.Provenance
	if e = json.Unmarshal(b, &p); e != nil {
		return e
	}
	pd, e := p.Digest()
	if e != nil || pd != contracts.GoalsPublicationSigningReceipt {
		return errors.New("signing provenance digest mismatch")
	}
	if p.PublisherGenerationDigest != contracts.GoalsPublicationPublisher || p.ManifestDigest != assetDigests[0] || p.ArtifactDigest != assetDigests[1] || p.SignatureEnvelopeDigest != assetDigests[2] || p.PackageID != "praxis.package.goals" || p.PackageVersion != "0.1.0" {
		return errors.New("signing provenance identities mismatch")
	}
	pub, e := r.Store.PublisherGeneration(ctx, contracts.GoalsPublicationPublisher)
	if e != nil {
		return e
	}
	d, e := pub.Generation.Digest()
	if e != nil || d != contracts.GoalsPublicationPublisher || pub.State != "active" {
		return errors.New("publisher is missing, changed or inactive")
	}
	if e = pub.Generation.Validate(); e != nil {
		return e
	}
	if pub.Generation.RevokedAt != nil || pub.Generation.EffectiveAt.After(now) || (pub.Generation.ExpiresAt != nil && !now.Before(*pub.Generation.ExpiresAt)) {
		return errors.New("publisher generation is revoked")
	}
	preview, e := r.LoadSigningPreviewByDigest(ctx, p.SigningPreviewDigest, p.SignedAt)
	if e != nil {
		return e
	}
	auth, e := r.ResolvePackagePublishAuthority(ctx, contracts.GoalsPublicationPublisher, "praxis.package.goals", p.SignedAt)
	if e != nil {
		return e
	}
	// Original requests contain time-sensitive validation. Hash the exact original
	// canonical JSON after the repository's historical resolution has validated it.
	req, e := auth.Request.DigestAt(p.SignedAt)
	if e != nil {
		return e
	}
	dec, e := auth.Decision.Digest()
	if e != nil {
		return e
	}
	if auth.Generation.Digest != p.PackagePublishAuthorityDigest || req != p.PackagePublishRequestDigest || dec != p.PackagePublishDecisionDigest || preview.AuthorityGenerationDigest != auth.Generation.Digest || preview.ManifestDigest != assetDigests[0] || preview.ArtifactDigest != assetDigests[1] {
		return errors.New("signing authority lineage mismatch")
	}
	if e = r.CheckGoalsPublicationInvalidation(ctx, auth.Generation.Ref, auth.Generation.Version, auth.Request.ID, auth.Request.Version, now); e != nil {
		return e
	}
	if e = r.CheckGoalsPublicationInvalidation(ctx, auth.Generation.ParentRef, auth.Generation.ParentVersion, "", "", now); e != nil {
		return e
	}
	var envelope packagecatalog.SignatureEnvelope
	if e = json.Unmarshal(a[2], &envelope); e != nil {
		return e
	}
	return packagecatalog.VerifySignature(envelope, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{pub.Generation.KeyID: pub.Generation.PublicKey}}}, false)
}
