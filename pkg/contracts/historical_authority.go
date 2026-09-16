package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// HistoricalAuthorityStatus is intentionally separate from the active
// AuthorityGeneration state machine.
type HistoricalAuthorityStatus string

const (
	HistoricalAuthorityExpired HistoricalAuthorityStatus = "expired"
)

// ExpiredHistoricalAuthorityEvidence is a safe, non-executable projection of
// an authority that governed a historical execution. It contains lineage
// identities only; protected authority plaintext is never exposed here.
type ExpiredHistoricalAuthorityEvidence struct {
	ObjectKind                    string                    `json:"object_kind"`
	ObjectID                      string                    `json:"object_id"`
	ObjectVersion                 string                    `json:"object_version"`
	ObjectDigest                  string                    `json:"object_digest"`
	InstallationDigest            string                    `json:"installation_digest"`
	Principal                     PrincipalRef              `json:"principal"`
	RequestDigest                 string                    `json:"request_digest"`
	IntentID                      string                    `json:"intent_id"`
	IntentDigest                  string                    `json:"intent_digest"`
	DecisionRef                   string                    `json:"decision_ref"`
	DecisionVersion               string                    `json:"decision_version"`
	DecisionDigest                string                    `json:"decision_digest"`
	ParentRef                     string                    `json:"parent_ref"`
	ParentVersion                 string                    `json:"parent_version"`
	ParentDigest                  string                    `json:"parent_digest"`
	DelegationRef                 string                    `json:"delegation_ref"`
	DelegationDigest              string                    `json:"delegation_digest"`
	GenerationRef                 string                    `json:"generation_ref"`
	GenerationVersion             string                    `json:"generation_version"`
	GenerationDigest              string                    `json:"generation_digest"`
	ExecutionID                   string                    `json:"execution_id"`
	EffectIDs                     []string                  `json:"effect_ids"`
	EffectiveAt                   time.Time                 `json:"effective_at"`
	ExpiresAt                     time.Time                 `json:"expires_at"`
	HistoricalValidityEstablished bool                      `json:"historical_validity_established"`
	Historical                    bool                      `json:"historical"`
	NonExecutable                 bool                      `json:"non_executable"`
	Status                        HistoricalAuthorityStatus `json:"status"`
	Digest                        string                    `json:"digest"`
}

func (e ExpiredHistoricalAuthorityEvidence) Validate() error {
	if e.ObjectKind != "authority request" || e.ObjectID == "" || e.ObjectVersion == "" || e.InstallationDigest == "" || e.RequestDigest == "" || e.IntentID == "" || e.IntentDigest == "" || e.DecisionRef == "" || e.DecisionVersion == "" || e.DecisionDigest == "" || e.ParentRef == "" || e.ParentVersion == "" || e.ParentDigest == "" || e.GenerationRef == "" || e.GenerationVersion == "" || e.GenerationDigest == "" || e.ExecutionID == "" || len(e.EffectIDs) == 0 || e.EffectiveAt.IsZero() || e.ExpiresAt.IsZero() || e.Status != HistoricalAuthorityExpired || !e.HistoricalValidityEstablished || !e.Historical || !e.NonExecutable {
		return errors.New("expired historical authority evidence is incomplete")
	}
	if err := e.Principal.Validate(); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(e.ObjectDigest); err != nil {
		return err
	}
	for _, d := range []string{e.InstallationDigest, e.RequestDigest, e.IntentDigest, e.DecisionDigest, e.GenerationDigest} {
		if err := ValidateSHA256Digest(d); err != nil {
			return err
		}
	}
	if !e.ExpiresAt.After(e.EffectiveAt) {
		return errors.New("historical authority interval is invalid")
	}
	if e.Digest == "" {
		return errors.New("historical authority projection digest is required")
	}
	return nil
}

func (e ExpiredHistoricalAuthorityEvidence) ComputeDigest() (string, error) {
	x := e
	x.Digest = ""
	b, err := json.Marshal(x)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:]), nil
}

func (e ExpiredHistoricalAuthorityEvidence) VerifyDigest() error {
	d, err := e.ComputeDigest()
	if err != nil {
		return err
	}
	if d != e.Digest {
		return errors.New("historical authority projection digest mismatch")
	}
	return nil
}
