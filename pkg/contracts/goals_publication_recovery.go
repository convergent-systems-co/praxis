package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	GoalsRecoveryOperation          = "publish-goals-from-established-state"
	GoalsRecoveryContract           = "goals-established-state-publication/1"
	GoalsChainedRecoveryContract    = "goals-established-state-publication/2"
	GoalsOrderedRecoveryContract    = "goals-established-state-publication/3"
	GoalsFailedVerificationContract = "goals-established-state-publication/4"
	GoalsRecoveryRepositoryID       = "1372388187"
	GoalsRecoveryOwnerID            = "263966243"
	GoalsRecoveryCommit             = "fbdc98828d49cf5ddd515edf91d457b606e89a97"
	GoalsRecoveryTree               = "06476050446988f592cd2064823ff73c5a7a09f0"
	GoalsRecoveryReleaseID          = "389997269"
	GoalsRecoveryNonce              = "7faf8b95afeb3bdd58534b0e1139f01169b4d417cd8d30e3b77a1533ad84aad1"
	GoalsRecoveryAuthorityRoot      = "sha256:7e247747e70c88ad0feb59f485d31d3b2803e9049c22983587ddf61b500c1e47"
	GoalsRecoveryPublisherKey       = "sha256:602527c02d1a5dfa84661699560289895f70fdcd99c4364fbf1703647eea8dd5"
)

type GoalsRecoveryInput struct {
	CreatedAt, ExpiresAt                           time.Time
	Identity                                       string
	AccountID                                      int64
	Sizes                                          [3]int64
	PredecessorRequestID, PredecessorRequestDigest string
	PredecessorIntentID, PredecessorIntentDigest   string
	AbandonmentEventID, AbandonmentDigest          string
}

// GoalsChainedRecoveryInput is the bounded two-generation extension used when
// a recovery successor itself was abandoned. It is deliberately fixed-width:
// it records exactly one prior recovery generation rather than an arbitrary
// recursive event list.
type GoalsChainedRecoveryInput struct {
	GoalsRecoveryInput
	PriorRecoveryRequestID, PriorRecoveryRequestDigest                                                               string
	PriorRecoveryIntentID, PriorRecoveryIntentDigest                                                                 string
	PriorRecoveryAuthorityDigest, PriorRecoveryExecutionID                                                           string
	PriorRecoveryAbandonmentEventID, PriorRecoveryAbandonmentDigest                                                  string
	PriorRecoveryManifestEffectID, PriorRecoveryManifestState                                                        string
	PriorRecoveryManifestAttempts                                                                                    int
	PriorRecoveryManifestRequestDigest, PriorRecoveryManifestResultDigest, PriorRecoveryManifestReconciliationDigest string
}

// RecoveryGenerationBinding is one exact, already-abandoned recovery
// generation in chronological order. The representation is intentionally
// fixed-field and canonical; it is not an arbitrary event/history blob.
type RecoveryGenerationBinding struct {
	RequestID, RequestDigest, IntentID, IntentDigest                          string
	AuthorityDigest, ExecutionID, AbandonmentEventID, AbandonmentDigest       string
	ManifestEffectID, ManifestState                                           string
	ManifestAttempts                                                          int
	ManifestRequestDigest, ManifestResultDigest, ManifestReconciliationDigest string
}

type GoalsOrderedRecoveryInput struct {
	GoalsRecoveryInput
	Chain []RecoveryGenerationBinding
}

type GoalsFailedVerificationInput struct {
	GoalsOrderedRecoveryInput
	FailedRequestID, FailedRequestDigest, FailedIntentID, FailedIntentDigest                                         string
	FailedAuthorityDigest, FailedExecutionID                                                                         string
	FailedHistoricalAuthorityDigest                                                                                  string
	FailedManifestEffectID, FailedArchiveEffectID, FailedSignatureEffectID, FailedVerifyEffectID                     string
	FailedManifestState, FailedArchiveState, FailedSignatureState, FailedVerifyState                                 string
	FailedVerifyAttempts                                                                                             int
	FailedManifestRequestDigest, FailedArchiveRequestDigest, FailedSignatureRequestDigest, FailedVerifyRequestDigest string
	FailedVerifyResultDigest, FailedVerifyReconciliationDigest                                                       string
	AssetIDs                                                                                                         [3]string
}

func NewGoalsPublicationFailedVerificationIntent(in GoalsFailedVerificationInput) (ActionIntent, error) {
	a, err := NewGoalsPublicationOrderedRecoveryIntent(in.GoalsOrderedRecoveryInput)
	if err != nil {
		return ActionIntent{}, err
	}
	if in.FailedRequestID == "" || in.FailedIntentID == "" || in.FailedAuthorityDigest == "" || in.FailedHistoricalAuthorityDigest == "" || in.FailedExecutionID == "" || in.FailedVerifyAttempts != 1 {
		return ActionIntent{}, errors.New("failed verification lineage missing")
	}
	for _, d := range []string{in.FailedRequestDigest, in.FailedIntentDigest, in.FailedAuthorityDigest, in.FailedHistoricalAuthorityDigest, in.FailedManifestRequestDigest, in.FailedArchiveRequestDigest, in.FailedSignatureRequestDigest, in.FailedVerifyRequestDigest, in.FailedVerifyResultDigest, in.FailedVerifyReconciliationDigest} {
		if !isSHA256Digest(d) {
			return ActionIntent{}, errors.New("failed verification digest invalid")
		}
	}
	for _, s := range []string{in.FailedManifestEffectID, in.FailedArchiveEffectID, in.FailedSignatureEffectID, in.FailedVerifyEffectID} {
		if s == "" {
			return ActionIntent{}, errors.New("failed verification effect binding missing")
		}
	}
	for i, id := range in.AssetIDs {
		if id == "" {
			return ActionIntent{}, fmt.Errorf("asset %d identity missing", i)
		}
	}
	if in.FailedManifestState != "succeeded" || in.FailedArchiveState != "succeeded" || in.FailedSignatureState != "succeeded" || in.FailedVerifyState != "failed" {
		return ActionIntent{}, errors.New("failed verification states invalid")
	}
	a.Parameters["contract"] = GoalsFailedVerificationContract
	a.Parameters["failed_predecessor_request_id"] = in.FailedRequestID
	a.Parameters["failed_predecessor_request_digest"] = in.FailedRequestDigest
	a.Parameters["failed_predecessor_intent_id"] = in.FailedIntentID
	a.Parameters["failed_predecessor_intent_digest"] = in.FailedIntentDigest
	a.Parameters["failed_predecessor_authority_digest"] = in.FailedAuthorityDigest
	a.Parameters["failed_predecessor_historical_authority_digest"] = in.FailedHistoricalAuthorityDigest
	a.Parameters["failed_predecessor_execution_id"] = in.FailedExecutionID
	a.Parameters["failed_manifest_effect_id"] = in.FailedManifestEffectID
	a.Parameters["failed_archive_effect_id"] = in.FailedArchiveEffectID
	a.Parameters["failed_signature_effect_id"] = in.FailedSignatureEffectID
	a.Parameters["failed_verify_effect_id"] = in.FailedVerifyEffectID
	a.Parameters["failed_manifest_state"] = in.FailedManifestState
	a.Parameters["failed_archive_state"] = in.FailedArchiveState
	a.Parameters["failed_signature_state"] = in.FailedSignatureState
	a.Parameters["failed_verify_state"] = in.FailedVerifyState
	a.Parameters["failed_verify_attempts"] = strconv.Itoa(in.FailedVerifyAttempts)
	a.Parameters["failed_manifest_request_digest"] = in.FailedManifestRequestDigest
	a.Parameters["failed_archive_request_digest"] = in.FailedArchiveRequestDigest
	a.Parameters["failed_signature_request_digest"] = in.FailedSignatureRequestDigest
	a.Parameters["failed_verify_request_digest"] = in.FailedVerifyRequestDigest
	a.Parameters["failed_verify_result_digest"] = in.FailedVerifyResultDigest
	a.Parameters["failed_verify_reconciliation_digest"] = in.FailedVerifyReconciliationDigest
	a.Parameters["asset_manifest_id"] = in.AssetIDs[0]
	a.Parameters["asset_archive_id"] = in.AssetIDs[1]
	a.Parameters["asset_signature_id"] = in.AssetIDs[2]
	a.Parameters["asset_inventory"] = "established"
	a.Parameters["asset_count"] = "3"
	for _, k := range []string{"assets", "asset_count", "asset_manifest_id", "asset_archive_id", "asset_signature_id", "asset_manifest_name", "asset_archive_name", "asset_signature_name", "asset_manifest_digest", "asset_archive_digest", "asset_signature_digest"} {
		delete(a.Preconditions, k)
	}
	a.Parameters["asset_manifest_name"] = "praxis-package.json"
	a.Parameters["asset_archive_name"] = "praxis-package.tar.gz"
	a.Parameters["asset_signature_name"] = "praxis-package.sig.json"
	a.Parameters["asset_manifest_digest"] = GoalsPublicationManifest
	a.Parameters["asset_archive_digest"] = GoalsPublicationArchive
	a.Parameters["asset_signature_digest"] = GoalsPublicationSignature
	a.Preconditions["assets"] = "exact-established:3"
	a.Preconditions["asset_count"] = "3"
	a.Preconditions["asset_manifest_id"] = in.AssetIDs[0]
	a.Preconditions["asset_archive_id"] = in.AssetIDs[1]
	a.Preconditions["asset_signature_id"] = in.AssetIDs[2]
	a.Preconditions["asset_manifest_name"] = a.Parameters["asset_manifest_name"]
	a.Preconditions["asset_archive_name"] = a.Parameters["asset_archive_name"]
	a.Preconditions["asset_signature_name"] = a.Parameters["asset_signature_name"]
	a.Preconditions["asset_manifest_digest"] = a.Parameters["asset_manifest_digest"]
	a.Preconditions["asset_archive_digest"] = a.Parameters["asset_archive_digest"]
	a.Preconditions["asset_signature_digest"] = a.Parameters["asset_signature_digest"]
	a.Parameters["permitted_effects"] = "verify-draft-assets,publish-existing-release,verify-published-release"
	return a, nil
}

func EncodeRecoveryChain(chain []RecoveryGenerationBinding) (string, error) {
	parts := make([]string, len(chain))
	for i, x := range chain {
		if x.RequestID == "" || x.IntentID == "" || x.AuthorityDigest == "" || x.ExecutionID == "" || x.AbandonmentEventID == "" || x.ManifestEffectID == "" || x.ManifestState != "unknown" || x.ManifestAttempts != 1 {
			return "", errors.New("recovery chain element invalid")
		}
		for _, d := range []string{x.RequestDigest, x.IntentDigest, x.AbandonmentDigest, x.ManifestRequestDigest, x.ManifestResultDigest, x.ManifestReconciliationDigest} {
			if !isSHA256Digest(d) {
				return "", errors.New("recovery chain digest invalid")
			}
		}
		for _, s := range []string{x.RequestID, x.IntentID, x.AuthorityDigest, x.ExecutionID, x.AbandonmentEventID, x.ManifestEffectID} {
			if strings.ContainsAny(s, "|;") {
				return "", errors.New("recovery chain identifier contains delimiter")
			}
		}
		parts[i] = strings.Join([]string{x.RequestID, x.RequestDigest, x.IntentID, x.IntentDigest, x.AuthorityDigest, x.ExecutionID, x.AbandonmentEventID, x.AbandonmentDigest, x.ManifestEffectID, x.ManifestState, strconv.Itoa(x.ManifestAttempts), x.ManifestRequestDigest, x.ManifestResultDigest, x.ManifestReconciliationDigest}, "|")
	}
	return strings.Join(parts, ";"), nil
}

func ParseRecoveryChain(encoded string) ([]RecoveryGenerationBinding, error) {
	if encoded == "" {
		return nil, nil
	}
	rows := strings.Split(encoded, ";")
	out := make([]RecoveryGenerationBinding, len(rows))
	for i, row := range rows {
		f := strings.Split(row, "|")
		if len(f) != 14 {
			return nil, errors.New("recovery chain encoding malformed")
		}
		n, err := strconv.Atoi(f[10])
		if err != nil {
			return nil, errors.New("recovery chain attempts malformed")
		}
		out[i] = RecoveryGenerationBinding{RequestID: f[0], RequestDigest: f[1], IntentID: f[2], IntentDigest: f[3], AuthorityDigest: f[4], ExecutionID: f[5], AbandonmentEventID: f[6], AbandonmentDigest: f[7], ManifestEffectID: f[8], ManifestState: f[9], ManifestAttempts: n, ManifestRequestDigest: f[11], ManifestResultDigest: f[12], ManifestReconciliationDigest: f[13]}
	}
	canonical, err := EncodeRecoveryChain(out)
	if err != nil || canonical != encoded {
		return nil, errors.New("recovery chain is not canonical")
	}
	seen := map[string]bool{}
	for _, x := range out {
		if seen[x.RequestID] || seen[x.ExecutionID] {
			return nil, errors.New("recovery chain contains duplicate generation")
		}
		seen[x.RequestID], seen[x.ExecutionID] = true, true
	}
	return out, nil
}

func NewGoalsPublicationOrderedRecoveryIntent(in GoalsOrderedRecoveryInput) (ActionIntent, error) {
	a, err := NewGoalsPublicationRecoveryIntent(in.GoalsRecoveryInput)
	if err != nil {
		return ActionIntent{}, err
	}
	chain, err := EncodeRecoveryChain(in.Chain)
	if err != nil {
		return ActionIntent{}, err
	}
	a.Parameters["contract"] = GoalsOrderedRecoveryContract
	a.Parameters["recovery_chain_version"] = "1"
	a.Parameters["recovery_chain_count"] = strconv.Itoa(len(in.Chain))
	a.Parameters["recovery_chain"] = chain
	a.Parameters["recovery_chain_digest"] = recoveryChainDigest(chain)
	return a, nil
}

func recoveryChainDigest(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + fmt.Sprintf("%x", h[:])
}

func NewGoalsPublicationChainedRecoveryIntent(in GoalsChainedRecoveryInput) (ActionIntent, error) {
	a, err := NewGoalsPublicationRecoveryIntent(in.GoalsRecoveryInput)
	if err != nil {
		return ActionIntent{}, err
	}
	for _, s := range []string{in.PriorRecoveryRequestID, in.PriorRecoveryIntentID, in.PriorRecoveryAuthorityDigest, in.PriorRecoveryExecutionID, in.PriorRecoveryAbandonmentEventID, in.PriorRecoveryManifestEffectID} {
		if s == "" {
			return ActionIntent{}, errors.New("prior recovery lineage missing")
		}
	}
	for _, d := range []string{in.PriorRecoveryRequestDigest, in.PriorRecoveryIntentDigest, in.PriorRecoveryAbandonmentDigest, in.PriorRecoveryManifestRequestDigest, in.PriorRecoveryManifestResultDigest, in.PriorRecoveryManifestReconciliationDigest} {
		if !isSHA256Digest(d) {
			return ActionIntent{}, errors.New("prior recovery lineage digest invalid")
		}
	}
	if in.PriorRecoveryManifestState != "unknown" || in.PriorRecoveryManifestAttempts != 1 {
		return ActionIntent{}, errors.New("prior recovery manifest state invalid")
	}
	a.Parameters["contract"] = GoalsChainedRecoveryContract
	a.Parameters["prior_recovery_request_id"] = in.PriorRecoveryRequestID
	a.Parameters["prior_recovery_request_digest"] = in.PriorRecoveryRequestDigest
	a.Parameters["prior_recovery_intent_id"] = in.PriorRecoveryIntentID
	a.Parameters["prior_recovery_intent_digest"] = in.PriorRecoveryIntentDigest
	a.Parameters["prior_recovery_authority_digest"] = in.PriorRecoveryAuthorityDigest
	a.Parameters["prior_recovery_execution_id"] = in.PriorRecoveryExecutionID
	a.Parameters["prior_recovery_abandonment_event_id"] = in.PriorRecoveryAbandonmentEventID
	a.Parameters["prior_recovery_abandonment_digest"] = in.PriorRecoveryAbandonmentDigest
	a.Parameters["prior_recovery_manifest_effect_id"] = in.PriorRecoveryManifestEffectID
	a.Parameters["prior_recovery_manifest_state"] = in.PriorRecoveryManifestState
	a.Parameters["prior_recovery_manifest_attempts"] = strconv.Itoa(in.PriorRecoveryManifestAttempts)
	a.Parameters["prior_recovery_manifest_request_digest"] = in.PriorRecoveryManifestRequestDigest
	a.Parameters["prior_recovery_manifest_result_digest"] = in.PriorRecoveryManifestResultDigest
	a.Parameters["prior_recovery_manifest_reconciliation_digest"] = in.PriorRecoveryManifestReconciliationDigest
	return a, nil
}

// NewGoalsPublicationRecoveryIntent is closed around the one established
// Goals release and the one predecessor abandonment. It accepts no destination
// or package selector.
func NewGoalsPublicationRecoveryIntent(in GoalsRecoveryInput) (ActionIntent, error) {
	if in.Identity == "" || in.AccountID <= 0 || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.Before(in.CreatedAt.Add(24*time.Hour)) {
		return ActionIntent{}, errors.New("successor identity or bounded expiry invalid")
	}
	for _, d := range []string{in.PredecessorRequestDigest, in.PredecessorIntentDigest, in.AbandonmentDigest} {
		if !isSHA256Digest(d) {
			return ActionIntent{}, errors.New("successor predecessor lineage digest invalid")
		}
	}
	if in.PredecessorRequestID == "" || in.PredecessorIntentID == "" || in.AbandonmentEventID == "" {
		return ActionIntent{}, errors.New("successor predecessor lineage missing")
	}
	for _, s := range in.Sizes {
		if s <= 0 {
			return ActionIntent{}, errors.New("successor asset size invalid")
		}
	}
	actor := PrincipalRef{ID: FirstPartyPublisherPrincipal, Kind: "publisher"}
	expires := in.ExpiresAt.UTC().Format(time.RFC3339Nano)
	created := in.CreatedAt.UTC().Format(time.RFC3339Nano)
	params := map[string]string{
		"contract": GoalsRecoveryContract, "installation_digest": GoalsPublicationRoot, "publisher_generation": GoalsPublicationPublisher,
		"package": GoalsPublicationPackage, "manifest_digest": GoalsPublicationManifest, "archive_digest": GoalsPublicationArchive, "signature_digest": GoalsPublicationSignature, "executable_digest": GoalsPublicationExecutable, "signing_provenance": GoalsPublicationSigningReceipt,
		"repository": GoalsPublicationRepository, "repository_id": GoalsRecoveryRepositoryID, "owner_id": GoalsRecoveryOwnerID, "account_id": strconv.FormatInt(in.AccountID, 10), "commit": GoalsRecoveryCommit, "tree": GoalsRecoveryTree, "branch": "refs/heads/main", "branch_target": GoalsRecoveryCommit, "tag": GoalsPublicationTag, "tag_target": GoalsRecoveryCommit,
		"release_id": GoalsRecoveryReleaseID, "release_name": GoalsPublicationPackage, "release_body": "Exact signed Goals initial publication; intent " + GoalsRecoveryNonce, "draft": "true", "prerelease": "false", "generate_release_notes": "false", "make_latest": "false", "asset_inventory": "empty",
		"manifest_name": "praxis-package.json", "manifest_size": strconv.FormatInt(in.Sizes[0], 10), "archive_name": "praxis-package.tar.gz", "archive_size": strconv.FormatInt(in.Sizes[1], 10), "signature_name": "praxis-package.sig.json", "signature_size": strconv.FormatInt(in.Sizes[2], 10),
		"predecessor_request_id": in.PredecessorRequestID, "predecessor_request_digest": in.PredecessorRequestDigest, "predecessor_intent_id": in.PredecessorIntentID, "predecessor_intent_digest": in.PredecessorIntentDigest, "abandonment_event_id": in.AbandonmentEventID, "abandonment_digest": in.AbandonmentDigest, "predecessor_manifest_outcome": "unknown-unresolved",
		"permitted_effects": "upload-manifest,upload-archive,upload-signature,verify-draft-assets,publish-existing-release,verify-published-release", "created_at": created, "expires_at": expires,
	}
	return ActionIntent{Version: "1", ID: "goals-established-state-publication:" + in.Identity, Actor: actor, Operation: GoalsRecoveryOperation, Target: "github.com/" + GoalsPublicationRepository, Scope: "goals-established-state-publication:" + in.Identity, CryptoProfile: CryptoClassicalCompatible, Parameters: params, Preconditions: map[string]string{"repository_id": GoalsRecoveryRepositoryID, "owner_id": GoalsRecoveryOwnerID, "commit": GoalsRecoveryCommit, "tree": GoalsRecoveryTree, "main": GoalsRecoveryCommit, "tag": GoalsRecoveryCommit, "release_id": GoalsRecoveryReleaseID, "release_draft": "true", "assets": "empty", "predecessor_abandonment": in.AbandonmentDigest}}, nil
}

func ValidateGoalsPublicationRecoveryIntent(a ActionIntent) error {
	if a.Parameters["contract"] == GoalsFailedVerificationContract {
		return ValidateGoalsPublicationFailedVerificationIntent(a)
	}
	if a.Parameters["contract"] == GoalsOrderedRecoveryContract {
		return ValidateGoalsPublicationOrderedRecoveryIntent(a)
	}
	if a.Parameters["contract"] == GoalsChainedRecoveryContract {
		return ValidateGoalsPublicationChainedRecoveryIntent(a)
	}
	return validateGoalsPublicationRecoveryIntentV1(a)
}

// ValidateGoalsPublicationFailedVerificationIntent closes the narrowly scoped
// successor created after a terminal local draft-verification failure. The
// earlier upload effects are historical bindings and cannot be re-authorized.
func ValidateGoalsPublicationFailedVerificationIntent(a ActionIntent) error {
	p := a.Parameters
	if p["contract"] != GoalsFailedVerificationContract || p["permitted_effects"] != "verify-draft-assets,publish-existing-release,verify-published-release" {
		return errors.New("failed-verification contract is not closed")
	}
	if err := ValidateFailedVerificationAssetPreconditions(a); err != nil {
		return err
	}
	created, e1 := time.Parse(time.RFC3339Nano, p["created_at"])
	expiry, e2 := time.Parse(time.RFC3339Nano, p["expires_at"])
	if e1 != nil || e2 != nil {
		return errors.New("successor intent timestamps malformed")
	}
	base, err := NewGoalsPublicationOrderedRecoveryIntent(GoalsOrderedRecoveryInput{GoalsRecoveryInput: GoalsRecoveryInput{CreatedAt: created, ExpiresAt: expiry, Identity: stringsTrimPrefix(a.ID, "goals-established-state-publication:"), AccountID: parseInt(p["account_id"]), Sizes: [3]int64{parseInt(p["manifest_size"]), parseInt(p["archive_size"]), parseInt(p["signature_size"])}, PredecessorRequestID: p["predecessor_request_id"], PredecessorRequestDigest: p["predecessor_request_digest"], PredecessorIntentID: p["predecessor_intent_id"], PredecessorIntentDigest: p["predecessor_intent_digest"], AbandonmentEventID: p["abandonment_event_id"], AbandonmentDigest: p["abandonment_digest"]}, Chain: mustParseChain(p["recovery_chain"])})
	if err != nil {
		return err
	}
	for k, v := range p {
		if strings.HasPrefix(k, "failed_") || strings.HasPrefix(k, "asset_") {
			base.Parameters[k] = v
		}
	}
	base.Parameters["contract"] = GoalsFailedVerificationContract
	base.Parameters["permitted_effects"] = p["permitted_effects"]
	for k, v := range a.Preconditions {
		base.Preconditions[k] = v
	}
	x, _ := json.Marshal(base)
	y, _ := json.Marshal(a)
	if !reflect.DeepEqual(x, y) {
		return errors.New("failed-verification intent differs from closed contract")
	}
	if p["failed_manifest_state"] != "succeeded" || p["failed_archive_state"] != "succeeded" || p["failed_signature_state"] != "succeeded" || p["failed_verify_state"] != "failed" || parseInt(p["failed_verify_attempts"]) != 1 {
		return errors.New("failed verification state invalid")
	}
	for _, k := range []string{"failed_predecessor_request_digest", "failed_predecessor_intent_digest", "failed_predecessor_authority_digest", "failed_predecessor_historical_authority_digest", "failed_manifest_request_digest", "failed_archive_request_digest", "failed_signature_request_digest", "failed_verify_request_digest", "failed_verify_result_digest", "failed_verify_reconciliation_digest"} {
		if !isSHA256Digest(p[k]) {
			return errors.New("failed verification digest invalid")
		}
	}
	for _, k := range []string{"failed_predecessor_request_id", "failed_predecessor_intent_id", "failed_predecessor_authority_digest", "failed_predecessor_execution_id", "failed_manifest_effect_id", "failed_archive_effect_id", "failed_signature_effect_id", "failed_verify_effect_id", "asset_manifest_id", "asset_archive_id", "asset_signature_id"} {
		if p[k] == "" {
			return errors.New("failed verification binding missing")
		}
	}
	return nil
}

// ValidateFailedVerificationAssetPreconditions enforces the single canonical
// current-asset representation in both intent parameters and durable
// preconditions.
func ValidateFailedVerificationAssetPreconditions(a ActionIntent) error {
	p, q := a.Parameters, a.Preconditions
	if p["asset_inventory"] != "established" || p["asset_count"] != "3" || q["assets"] != "exact-established:3" || q["asset_count"] != "3" {
		return errors.New("failed-verification asset inventory precondition invalid")
	}
	keys := []string{"manifest_id", "archive_id", "signature_id"}
	names := []string{"praxis-package.json", "praxis-package.tar.gz", "praxis-package.sig.json"}
	digests := []string{GoalsPublicationManifest, GoalsPublicationArchive, GoalsPublicationSignature}
	for i, k := range keys {
		pk, qk := "asset_"+k, "asset_"+k
		if p[pk] == "" || p[pk] != q[qk] {
			return errors.New("failed-verification asset ID precondition mismatch")
		}
		nk := "asset_" + strings.TrimSuffix(k, "_id") + "_name"
		dk := "asset_" + strings.TrimSuffix(k, "_id") + "_digest"
		if p[nk] != names[i] || q[nk] != names[i] || p[dk] != digests[i] || q[dk] != digests[i] {
			return errors.New("failed-verification asset precondition mismatch")
		}
	}
	return nil
}

func mustParseChain(s string) []RecoveryGenerationBinding { c, _ := ParseRecoveryChain(s); return c }

func ValidateGoalsPublicationOrderedRecoveryIntent(a ActionIntent) error {
	p := a.Parameters
	created, e1 := time.Parse(time.RFC3339Nano, p["created_at"])
	expiry, e2 := time.Parse(time.RFC3339Nano, p["expires_at"])
	if e1 != nil || e2 != nil {
		return errors.New("successor intent timestamps malformed")
	}
	base, err := NewGoalsPublicationRecoveryIntent(GoalsRecoveryInput{CreatedAt: created, ExpiresAt: expiry, Identity: stringsTrimPrefix(a.ID, "goals-established-state-publication:"), AccountID: parseInt(p["account_id"]), Sizes: [3]int64{parseInt(p["manifest_size"]), parseInt(p["archive_size"]), parseInt(p["signature_size"])}, PredecessorRequestID: p["predecessor_request_id"], PredecessorRequestDigest: p["predecessor_request_digest"], PredecessorIntentID: p["predecessor_intent_id"], PredecessorIntentDigest: p["predecessor_intent_digest"], AbandonmentEventID: p["abandonment_event_id"], AbandonmentDigest: p["abandonment_digest"]})
	if err != nil {
		return err
	}
	chain, err := ParseRecoveryChain(p["recovery_chain"])
	if err != nil {
		return err
	}
	if p["recovery_chain_version"] != "1" || p["recovery_chain_count"] != strconv.Itoa(len(chain)) || p["recovery_chain_digest"] != recoveryChainDigest(p["recovery_chain"]) {
		return errors.New("recovery chain identity mismatch")
	}
	base.Parameters["contract"] = GoalsOrderedRecoveryContract
	base.Parameters["recovery_chain_version"] = "1"
	base.Parameters["recovery_chain_count"] = strconv.Itoa(len(chain))
	base.Parameters["recovery_chain"] = p["recovery_chain"]
	base.Parameters["recovery_chain_digest"] = p["recovery_chain_digest"]
	if _, err := EncodeRecoveryChain(chain); err != nil {
		return err
	}
	x, _ := json.Marshal(base)
	y, _ := json.Marshal(a)
	if !reflect.DeepEqual(x, y) {
		return errors.New("ordered recovery intent differs from closed contract")
	}
	return nil
}

func parseInt(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

func validateGoalsPublicationRecoveryIntentV1(a ActionIntent) error {
	p := a.Parameters
	parse := func(k string) int64 { n, _ := strconv.ParseInt(p[k], 10, 64); return n }
	created, e1 := time.Parse(time.RFC3339Nano, p["created_at"])
	expiry, e2 := time.Parse(time.RFC3339Nano, p["expires_at"])
	if e1 != nil || e2 != nil {
		return errors.New("successor intent timestamps malformed")
	}
	want, err := NewGoalsPublicationRecoveryIntent(GoalsRecoveryInput{CreatedAt: created, ExpiresAt: expiry, Identity: stringsTrimPrefix(a.ID, "goals-established-state-publication:"), AccountID: parse("account_id"), Sizes: [3]int64{parse("manifest_size"), parse("archive_size"), parse("signature_size")}, PredecessorRequestID: p["predecessor_request_id"], PredecessorRequestDigest: p["predecessor_request_digest"], PredecessorIntentID: p["predecessor_intent_id"], PredecessorIntentDigest: p["predecessor_intent_digest"], AbandonmentEventID: p["abandonment_event_id"], AbandonmentDigest: p["abandonment_digest"]})
	if err != nil {
		return err
	}
	x, _ := json.Marshal(want)
	y, _ := json.Marshal(a)
	if !reflect.DeepEqual(x, y) {
		return fmt.Errorf("successor intent differs from its closed established-state contract")
	}
	return nil
}

func ValidateGoalsPublicationChainedRecoveryIntent(a ActionIntent) error {
	p := a.Parameters
	parse := func(k string) int64 { n, _ := strconv.ParseInt(p[k], 10, 64); return n }
	created, e1 := time.Parse(time.RFC3339Nano, p["created_at"])
	expiry, e2 := time.Parse(time.RFC3339Nano, p["expires_at"])
	if e1 != nil || e2 != nil {
		return errors.New("successor intent timestamps malformed")
	}
	base, err := NewGoalsPublicationRecoveryIntent(GoalsRecoveryInput{CreatedAt: created, ExpiresAt: expiry, Identity: stringsTrimPrefix(a.ID, "goals-established-state-publication:"), AccountID: parse("account_id"), Sizes: [3]int64{parse("manifest_size"), parse("archive_size"), parse("signature_size")}, PredecessorRequestID: p["predecessor_request_id"], PredecessorRequestDigest: p["predecessor_request_digest"], PredecessorIntentID: p["predecessor_intent_id"], PredecessorIntentDigest: p["predecessor_intent_digest"], AbandonmentEventID: p["abandonment_event_id"], AbandonmentDigest: p["abandonment_digest"]})
	if err != nil {
		return err
	}
	base.Parameters["contract"] = GoalsChainedRecoveryContract
	for _, k := range []string{"prior_recovery_request_id", "prior_recovery_request_digest", "prior_recovery_intent_id", "prior_recovery_intent_digest", "prior_recovery_authority_digest", "prior_recovery_execution_id", "prior_recovery_abandonment_event_id", "prior_recovery_abandonment_digest", "prior_recovery_manifest_effect_id", "prior_recovery_manifest_state", "prior_recovery_manifest_attempts", "prior_recovery_manifest_request_digest", "prior_recovery_manifest_result_digest", "prior_recovery_manifest_reconciliation_digest"} {
		base.Parameters[k] = p[k]
	}
	if p["prior_recovery_manifest_state"] != "unknown" || parse("prior_recovery_manifest_attempts") != 1 {
		return errors.New("prior recovery manifest state invalid")
	}
	for _, k := range []string{"prior_recovery_request_digest", "prior_recovery_intent_digest", "prior_recovery_abandonment_digest", "prior_recovery_manifest_request_digest", "prior_recovery_manifest_result_digest", "prior_recovery_manifest_reconciliation_digest"} {
		if !isSHA256Digest(p[k]) {
			return errors.New("prior recovery lineage digest invalid")
		}
	}
	for _, k := range []string{"prior_recovery_request_id", "prior_recovery_intent_id", "prior_recovery_authority_digest", "prior_recovery_execution_id", "prior_recovery_abandonment_event_id", "prior_recovery_manifest_effect_id"} {
		if p[k] == "" {
			return errors.New("prior recovery lineage missing")
		}
	}
	x, _ := json.Marshal(base)
	y, _ := json.Marshal(a)
	if !reflect.DeepEqual(x, y) {
		return errors.New("chained successor intent differs from closed contract")
	}
	return nil
}

func ValidateGoalsPublicationRecoveryDelegation(parent AuthorityGeneration, d DelegationRequest, a ActionIntent, now time.Time) error {
	if err := ValidateGoalsPublicationRecoveryIntent(a); err != nil {
		return err
	}
	digest, err := a.Digest()
	if err != nil {
		return err
	}
	expiry, _ := time.Parse(time.RFC3339Nano, a.Parameters["expires_at"])
	owner, _ := InstallationOwnerPrincipal(GoalsPublicationBootstrap)
	if parent.Digest != GoalsPublicationRoot || parent.ParentRef != "" || parent.Version != "1" || parent.Principal != owner || parent.ProvenanceDigest != GoalsPublicationBootstrap || parent.Scope != InstallationGovernanceScopePrefix+GoalsPublicationBootstrap || parent.AuthorityModel != AuthorityModelID || parent.AuthorityModelVersion != AuthorityModelVersion || parent.AuthorityModelDigest != AuthorityModelDigest() || !containsString(parent.Capabilities, AuthorityDelegateCapability) {
		return errors.New("successor parent is not the exact installation governance root")
	}
	if d.Profile != GoalsPublicationRecoveryProfile || d.ParentRef != parent.Ref || d.ParentVersion != parent.Version || d.ParentDigest != parent.Digest || d.DelegatedPrincipal != a.Actor || d.RequestedAuthority != GovernedPackagePublish || d.RequestedOperation != GoalsRecoveryOperation || d.RequestedScope != a.Scope || len(d.RequestedCapabilities) != 0 || len(d.RequestedOperations) != 0 || d.TargetKind != "action-intent" || d.TargetIdentity != a.ID || d.TargetVersion != a.Version || d.TargetDigest != digest || !reflect.DeepEqual(d.TargetConstraints, []string{a.Target, a.Parameters["repository_id"]}) || d.ProposalVersion != a.Version || d.ProposalDigest != digest || d.ReviewVersion != a.Version || d.ReviewDigest != digest || !d.ExpiresAt.Equal(expiry) || d.PolicyRef != AuthorityModelID || d.PolicyVersion != AuthorityModelGoalsRecoveryVersion || d.PolicyDigest != AuthorityModelGoalsRecoveryDigest() || d.SubjectKind != "publisher" || d.SubjectID != FirstPartyPublisherPrincipal || d.SubjectVersion != "2" || d.SubjectDigest != GoalsPublicationPublisher || d.SubjectKeyDigest != GoalsRecoveryPublisherKey || d.Reason != "publish the exact signed Goals assets to the established draft release" || !now.Before(expiry) {
		return errors.New("delegation is outside the exact v5 Goals successor profile")
	}
	return nil
}

func stringsTrimPrefix(s, p string) string {
	if len(s) >= len(p) && s[:len(p)] == p {
		return s[len(p):]
	}
	return ""
}
