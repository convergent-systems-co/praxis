package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	FirstPartyPublisherPrincipal = "publisher:praxis-first-party"
	PackagePublishCapability     = "package.publish"
	PublisherGenerationVersion   = "v1"
)

// PublisherGeneration is the non-secret, durable identity of one authorized
// package publisher generation. Private signing material is deliberately not
// represented here; it belongs to an external protected signing backend.
type PublisherGeneration struct {
	Version          string       `json:"version"`
	Principal        PrincipalRef `json:"principal"`
	KeyID            string       `json:"key_id"`
	Algorithm        string       `json:"algorithm"`
	PublicKey        []byte       `json:"public_key,omitempty"`
	PublicKeyDigest  string       `json:"public_key_digest"`
	PackageNamespace string       `json:"package_namespace"`
	Generation       string       `json:"generation"`
	Predecessor      string       `json:"predecessor,omitempty"`
	EffectiveAt      time.Time    `json:"effective_at"`
	ExpiresAt        *time.Time   `json:"expires_at,omitempty"`
	RevokedAt        *time.Time   `json:"revoked_at,omitempty"`
	EnrollmentRef    string       `json:"enrollment_ref"`
	EnrollmentDigest string       `json:"enrollment_digest"`
}

func (p PublisherGeneration) Validate() error {
	if p.Version != PublisherGenerationVersion || p.KeyID == "" || p.Algorithm == "" || p.Generation == "" || p.PackageNamespace == "" || p.EnrollmentRef == "" || p.EnrollmentDigest == "" || p.EffectiveAt.IsZero() {
		return errors.New("publisher generation identity, scope, enrollment, and effective time are required")
	}
	if p.Principal.ID != FirstPartyPublisherPrincipal || p.Principal.Kind != "publisher" {
		return errors.New("publisher generation principal is not the approved first-party publisher")
	}
	if len(p.PublicKey) != 0 && digestBytes(p.PublicKey) != p.PublicKeyDigest {
		return errors.New("publisher public key does not match its digest")
	}
	if !strings.HasPrefix(p.PublicKeyDigest, "sha256:") || len(p.PublicKeyDigest) != len("sha256:")+64 || !strings.HasPrefix(p.EnrollmentDigest, "sha256:") || len(p.EnrollmentDigest) != len("sha256:")+64 {
		return errors.New("publisher generation digests must be sha256")
	}
	if p.ExpiresAt != nil && !p.ExpiresAt.After(p.EffectiveAt) {
		return errors.New("publisher generation expiry must be after effective time")
	}
	if p.RevokedAt != nil && p.RevokedAt.Before(p.EffectiveAt) {
		return errors.New("publisher generation revocation cannot precede effective time")
	}
	return nil
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (p PublisherGeneration) Digest() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// PackageNamespaceAllowed is deliberately exact-prefix scoped. A publisher
// generation cannot publish outside its enrolled namespace.
func (p PublisherGeneration) PackageNamespaceAllowed(packageID string) bool {
	if p.Validate() != nil || packageID == "" {
		return false
	}
	return packageID == p.PackageNamespace || strings.HasPrefix(packageID, p.PackageNamespace+"/") || strings.HasPrefix(packageID, p.PackageNamespace+".")
}

func CanonicalPublisherCapabilities() []string {
	return []string{PackagePublishCapability}
}

func CanonicalPublisherStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
