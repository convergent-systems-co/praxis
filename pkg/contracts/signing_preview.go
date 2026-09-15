package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// SigningPreview is the immutable owner-review identity for one package
// signing operation. It binds the bytes, publisher, and exact authority that
// the signer is permitted to consume; the protected signer is never part of
// the authority decision.
type SigningPreview struct {
	ID                         string    `json:"id"`
	Version                    string    `json:"version"`
	PackageID                  string    `json:"package_id"`
	PackageVersion             string    `json:"package_version"`
	PackageNamespace           string    `json:"package_namespace"`
	ManifestDigest             string    `json:"manifest_digest"`
	ArtifactDigest             string    `json:"artifact_digest"`
	ExecutableDigest           string    `json:"executable_digest"`
	InvocationContractDigest   string    `json:"invocation_contract_digest"`
	ExecutableBindingDigest    string    `json:"executable_binding_digest"`
	PluginDefinitionDigest     string    `json:"plugin_definition_digest"`
	RuntimeID                  string    `json:"runtime_id"`
	RuntimeVersion             string    `json:"runtime_version"`
	RuntimeDigest              string    `json:"runtime_digest"`
	SourceIdentity             string    `json:"source_identity"`
	BuilderIdentity            string    `json:"builder_identity"`
	QualificationRef           string    `json:"qualification_ref,omitempty"`
	PublisherPrincipal         string    `json:"publisher_principal"`
	PublisherGenerationDigest  string    `json:"publisher_generation_digest"`
	PublisherGenerationVersion string    `json:"publisher_generation_version"`
	PublicKeyDigest            string    `json:"public_key_digest"`
	KeyID                      string    `json:"key_id"`
	Algorithm                  string    `json:"algorithm"`
	AuthorityGenerationRef     string    `json:"authority_generation_ref"`
	AuthorityGenerationVersion string    `json:"authority_generation_version"`
	AuthorityGenerationDigest  string    `json:"authority_generation_digest"`
	AuthorityModel             string    `json:"authority_model"`
	AuthorityModelVersion      string    `json:"authority_model_version"`
	AuthorityModelDigest       string    `json:"authority_model_digest"`
	ParentRef                  string    `json:"parent_ref"`
	ParentVersion              string    `json:"parent_version"`
	ParentDigest               string    `json:"parent_digest"`
	AuthorityScope             string    `json:"authority_scope"`
	AuthorityRequestID         string    `json:"authority_request_id"`
	AuthorityRequestVersion    string    `json:"authority_request_version"`
	AuthorityRequestDigest     string    `json:"authority_request_digest"`
	AuthorityDecisionRef       string    `json:"authority_decision_ref"`
	AuthorityDecisionVersion   string    `json:"authority_decision_version"`
	AuthorityDecisionDigest    string    `json:"authority_decision_digest"`
	ExpiresAt                  time.Time `json:"expires_at"`
	CreatedAt                  time.Time `json:"created_at"`
	Digest                     string    `json:"digest,omitempty"`
}

func (p SigningPreview) DigestValue() (string, error) {
	if p.ID == "" || p.Version == "" || p.PackageID == "" || p.PackageVersion == "" || p.PackageNamespace == "" || p.ManifestDigest == "" || p.ArtifactDigest == "" || p.ExecutableDigest == "" || p.InvocationContractDigest == "" || p.ExecutableBindingDigest == "" || p.PluginDefinitionDigest == "" || p.RuntimeID == "" || p.RuntimeVersion == "" || p.RuntimeDigest == "" || p.SourceIdentity == "" || p.BuilderIdentity == "" || p.PublisherPrincipal == "" || p.PublisherGenerationDigest == "" || p.PublicKeyDigest == "" || p.KeyID == "" || p.Algorithm == "" || p.AuthorityGenerationRef == "" || p.AuthorityGenerationVersion == "" || p.AuthorityGenerationDigest == "" || p.AuthorityModel != AuthorityModelID || p.AuthorityModelVersion != AuthorityModelSuccessorVersion || p.AuthorityModelDigest != AuthorityModelSuccessorDigest() || p.ParentRef == "" || p.ParentVersion == "" || p.ParentDigest == "" || p.AuthorityScope == "" || p.AuthorityRequestID == "" || p.AuthorityRequestVersion == "" || p.AuthorityRequestDigest == "" || p.AuthorityDecisionRef == "" || p.AuthorityDecisionVersion == "" || p.AuthorityDecisionDigest == "" || p.ExpiresAt.IsZero() || p.CreatedAt.IsZero() {
		return "", errors.New("signing preview is incomplete")
	}
	copy := p
	copy.Digest = ""
	b, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (p SigningPreview) VerifyDigest() error {
	d, err := p.DigestValue()
	if err != nil {
		return err
	}
	if p.Digest != d {
		return errors.New("signing preview digest mismatch")
	}
	return nil
}
