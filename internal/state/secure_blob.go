package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrSecureBlobNotFound                    = errors.New("secure blob not found")
	ErrSecureBlobExpired                     = errors.New("secure blob expired")
	ErrAuthorityRevoked                      = errors.New("authority revoked")
	ErrRepairAuthorityRequiresRootSuccession = errors.New("installation-repair authority requires the governed root-succession boundary")

	// ErrAuthorityGenerationNamespaceReserved is returned by the four
	// generic secure_blobs writers when a caller attempts to write a
	// record whose namespace is AuthorityGenerationNamespace. This is a
	// structural rejection forces every present and future production caller
	// through PutAuthorityGeneration, which accepts the typed plaintext and
	// performs semantic admission before it serializes or seals the value.
	ErrAuthorityGenerationNamespaceReserved = errors.New("authority_generation namespace is reserved for typed authority-generation persistence")
)

// AuthorityGenerationNamespace is the reserved secure_blobs namespace for
// durable contracts.AuthorityGeneration records. It is exported so
// internal/goalstore — the only production package that constructs and
// persists AuthorityGeneration values — can reference the exact same
// string this package's writers reserve, rather than each package
// maintaining its own copy that could drift.
const AuthorityGenerationNamespace = "authority_generation"
const RootAuthoritySuccessionProposalNamespace = "root_authority_succession_proposal"
const RootAuthoritySuccessionReviewNamespace = "root_authority_succession_review"
const RootAuthoritySuccessionDecisionNamespace = "root_authority_succession_decision"
const AuthorityGenerationInvalidationNamespace = "authority_generation_invalidation"

// AuthorityRevocationNamespace holds the negative record of a revoked decision.
const AuthorityRevocationNamespace = "authority_revocation"

type Sensitivity string

const (
	SensitivityInternal     Sensitivity = "internal"
	SensitivityConfidential Sensitivity = "confidential"
	SensitivityRestricted   Sensitivity = "restricted"
)

func (s Sensitivity) Validate() error {
	switch s {
	case SensitivityInternal, SensitivityConfidential, SensitivityRestricted:
		return nil
	default:
		return fmt.Errorf("invalid sensitivity %q", s)
	}
}

type SecureBlobRecord struct {
	Namespace     string
	ObjectID      string
	ObjectVersion string
	ObjectDigest  string
	Sensitivity   Sensitivity
	CryptoProfile contracts.CryptoProfile
	Envelope      praxiscrypto.Envelope
	CreatedAt     time.Time
	ExpiresAt     *time.Time
}

func SecureBlobAAD(namespace, objectID, version, digest string) []byte {
	b, _ := json.Marshal(struct {
		Namespace string `json:"namespace"`
		ObjectID  string `json:"object_id"`
		Version   string `json:"version"`
		Digest    string `json:"digest"`
	}{namespace, objectID, version, digest})
	return b
}

func (r SecureBlobRecord) Validate() error {
	if r.Namespace == "" || r.ObjectID == "" || r.ObjectVersion == "" || r.ObjectDigest == "" {
		return errors.New("secure blob namespace, object id, version, and digest are required")
	}
	if err := r.Sensitivity.Validate(); err != nil {
		return err
	}
	if err := r.CryptoProfile.Validate(); err != nil {
		return err
	}
	if r.Envelope.Version == "" || len(r.Envelope.Ciphertext) == 0 {
		return errors.New("secure blob requires encrypted envelope content")
	}
	if r.Envelope.RequestedProfile != r.CryptoProfile {
		return errors.New("secure blob crypto profile differs from envelope request")
	}
	expectedAAD := SecureBlobAAD(r.Namespace, r.ObjectID, r.ObjectVersion, r.ObjectDigest)
	if !bytes.Equal(r.Envelope.AAD, expectedAAD) {
		return errors.New("secure blob envelope is not bound to record identity/digest")
	}
	if r.CreatedAt.IsZero() {
		return errors.New("secure blob creation time is required")
	}
	if r.ExpiresAt != nil && !r.ExpiresAt.After(r.CreatedAt) {
		return errors.New("secure blob expiry must be after creation")
	}
	return nil
}

// PutSecureBlob persists ciphertext-only immutable versioned state. There is no
// plaintext storage path in this table/API. It refuses AuthorityGenerationNamespace
// writes — use the typed PutAuthorityGeneration boundary for those.
func (s *Store) PutSecureBlob(ctx context.Context, record SecureBlobRecord) error {
	if record.Namespace == AuthorityGenerationNamespace {
		return ErrAuthorityGenerationNamespaceReserved
	}
	return s.putSecureBlob(ctx, record)
}

func (s *Store) putSecureBlob(ctx context.Context, record SecureBlobRecord) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil {
		return fmt.Errorf("encode secure blob envelope: %w", err)
	}
	var expires any
	if record.ExpiresAt != nil {
		expires = record.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), expires)
	if err != nil {
		return fmt.Errorf("insert secure blob: %w", err)
	}
	return nil
}

// PutSecureBlobWithLock serializes an immutable write with other authority
// transitions that lock the same existing source record. The lock is held in
// the database transaction, not process memory, and therefore survives
// restart and coordinates multiple processes using the same SQLite store.
// It refuses AuthorityGenerationNamespace writes — use the typed
// PutAuthorityGeneration boundary for those.
func (s *Store) PutSecureBlobWithLock(ctx context.Context, record SecureBlobRecord, lockNamespace, lockID, lockVersion string) error {
	if record.Namespace == AuthorityGenerationNamespace {
		return ErrAuthorityGenerationNamespaceReserved
	}
	return s.putSecureBlobWithLock(ctx, record, lockNamespace, lockID, lockVersion)
}

func (s *Store) putSecureBlobWithLock(ctx context.Context, record SecureBlobRecord, lockNamespace, lockID, lockVersion string) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if lockNamespace == "" || lockID == "" || lockVersion == "" {
		return errors.New("secure blob lock identity is required")
	}
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil {
		return fmt.Errorf("encode secure blob envelope: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin secure blob transition: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion); err != nil {
		return fmt.Errorf("lock secure blob source: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt)); err != nil {
		return fmt.Errorf("insert locked secure blob: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit secure blob transition: %w", err)
	}
	return nil
}

// PutSecureBlobsWithLock atomically persists a governed transition's related
// immutable records while holding the source-record fence in the database.
// It refuses any batch containing an AuthorityGenerationNamespace record —
// use the typed PutAuthorityGeneration boundary for those.
func (s *Store) PutSecureBlobsWithLock(ctx context.Context, records []SecureBlobRecord, lockNamespace, lockID, lockVersion string) error {
	for _, record := range records {
		if record.Namespace == AuthorityGenerationNamespace {
			return ErrAuthorityGenerationNamespaceReserved
		}
	}
	return s.putSecureBlobsWithLock(ctx, records, lockNamespace, lockID, lockVersion)
}

func (s *Store) putSecureBlobsWithLock(ctx context.Context, records []SecureBlobRecord, lockNamespace, lockID, lockVersion string) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if len(records) == 0 || lockNamespace == "" || lockID == "" || lockVersion == "" {
		return errors.New("secure blob records and lock identity are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin secure blob transition: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion); err != nil {
		return fmt.Errorf("lock secure blob source: %w", err)
	}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
		envelopeJSON, err := json.Marshal(record.Envelope)
		if err != nil {
			return fmt.Errorf("encode secure blob envelope: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt)); err != nil {
			return fmt.Errorf("insert governed secure blob: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit secure blob transition: %w", err)
	}
	return nil
}

func (s *Store) PutSecureBlobsUnlessRevoked(ctx context.Context, records []SecureBlobRecord, revocationNamespace, requestID, requestVersion, generationInvalidationNamespace, generationID, generationVersion, lockNamespace, lockID, lockVersion string) error {
	if s == nil || s.db == nil || len(records) == 0 {
		return errors.New("state store and secure blob records are required")
	}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("secure blob lock source is missing")
	}
	for _, check := range []struct{ namespace, id, version string }{{revocationNamespace, requestID, requestVersion}, {generationInvalidationNamespace, generationID, generationVersion}} {
		revoked, err := revokedOrNotLiveInTx(ctx, tx, check.namespace, check.id, check.version)
		if err != nil {
			return err
		}
		if revoked {
			return ErrAuthorityRevoked
		}
	}
	for _, record := range records {
		envelopeJSON, err := json.Marshal(record.Envelope)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PutSecureBlobUnlessRevoked atomically locks the source authority record,
// checks the immutable revocation namespace, and writes the new record. A
// revoke and this operation therefore have one durable SQLite ordering. It
// refuses to write an AuthorityGenerationNamespace record (revocationNamespace/
// additionalRevocationNamespace may still legitimately reference
// AuthorityGenerationNamespace as a read-only revocation-check target;
// only record.Namespace, the write target, is reserved).
// live are liveness records (SealedLivenessRecord) inserted in the same
// transaction, after the revocation and invalidation checks.
func (s *Store) PutSecureBlobUnlessRevoked(ctx context.Context, record SecureBlobRecord, revocationNamespace, requestID, requestVersion, additionalRevocationNamespace, additionalID, additionalVersion, lockNamespace, lockID, lockVersion string, live ...SecureBlobRecord) error {
	if record.Namespace == AuthorityGenerationNamespace {
		return ErrAuthorityGenerationNamespaceReserved
	}
	for _, item := range live {
		if item.Namespace != AuthorityDecisionLiveNamespace && item.Namespace != AuthorityGenerationLiveNamespace {
			return ErrNotLivenessNamespace
		}
	}
	return s.putSecureBlobUnlessRevoked(ctx, record, revocationNamespace, requestID, requestVersion, additionalRevocationNamespace, additionalID, additionalVersion, lockNamespace, lockID, lockVersion, live...)
}

func (s *Store) putSecureBlobUnlessRevoked(ctx context.Context, record SecureBlobRecord, revocationNamespace, requestID, requestVersion, additionalRevocationNamespace, additionalID, additionalVersion, lockNamespace, lockID, lockVersion string, live ...SecureBlobRecord) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if revocationNamespace == "" || requestID == "" || requestVersion == "" {
		return errors.New("authority revocation lookup identity is required")
	}
	if lockNamespace == "" || lockID == "" || lockVersion == "" {
		return errors.New("secure blob lock identity is required")
	}
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil {
		return fmt.Errorf("encode secure blob envelope: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin authority transition: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion); err != nil {
		return fmt.Errorf("lock authority source: %w", err)
	}
	for _, check := range []struct{ namespace, id, version string }{{revocationNamespace, requestID, requestVersion}, {additionalRevocationNamespace, additionalID, additionalVersion}} {
		namespace, id, version := check.namespace, check.id, check.version
		if namespace == "" {
			continue
		}
		revoked, err := revokedOrNotLiveInTx(ctx, tx, namespace, id, version, live...)
		if err != nil {
			return err
		}
		if revoked {
			return ErrAuthorityRevoked
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt)); err != nil {
		return fmt.Errorf("insert authority-bound secure blob: %w", err)
	}
	for _, item := range live {
		if err := insertSecureBlobTx(ctx, tx, item); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit authority transition: %w", err)
	}
	return nil
}

// AuthorityGenerationWrite is the complete input to the sole production
// authority-generation persistence boundary. RelatedRecords supports the one
// atomic decision+generation transition; it may never contain another reserved
// authority-generation record. Supplying a lock identity selects the fenced
// write form and all three lock fields must be present together.
type AuthorityGenerationWrite struct {
	Generation     contracts.AuthorityGeneration
	Crypto         praxiscrypto.EnvelopeService
	KeyRef         string
	Profile        contracts.CryptoProfile
	Sensitivity    Sensitivity
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	RelatedRecords []SecureBlobRecord
	// MarkLive writes the generation's liveness record in the same transaction
	// as the generation, so no interrupted write can leave a live-looking
	// generation without one, and no replay is ever needed to create it.
	MarkLive bool
	// AdmittedAtSeq stamps the generation's liveness record (see LivenessRecord).
	AdmittedAtSeq uint64
	LockNamespace string
	LockID        string
	LockVersion   string
}

// PutAuthorityGeneration validates the semantic AuthorityGeneration value,
// serializes that exact value, seals those exact bytes, and persists the result.
// No API accepting a caller-constructed sealed record can write the reserved
// namespace. This makes ADR-088's non-delegable repair-authority admission
// boundary structural rather than dependent on today's call graph.
func (s *Store) PutAuthorityGeneration(ctx context.Context, write AuthorityGenerationWrite) error {
	if err := write.Generation.Validate(); err != nil {
		return err
	}
	if err := write.Generation.VerifyDigest(); err != nil {
		return err
	}
	if write.Generation.PreDelegationForm() {
		return errors.New("authority generation must be persisted in the current representation")
	}
	if write.Generation.PredecessorRef != "" {
		return ErrRootSuccessorRequiresSuccession
	}
	if authorityGenerationHasInstallationRepair(write.Generation) {
		return ErrRepairAuthorityRequiresRootSuccession
	}
	if write.KeyRef == "" {
		return errors.New("authority generation key reference is required")
	}
	if err := write.Profile.Validate(); err != nil {
		return err
	}
	if err := write.Sensitivity.Validate(); err != nil {
		return err
	}
	if write.CreatedAt.IsZero() {
		return errors.New("authority generation creation time is required")
	}
	lockFields := 0
	for _, value := range []string{write.LockNamespace, write.LockID, write.LockVersion} {
		if value != "" {
			lockFields++
		}
	}
	if lockFields != 0 && lockFields != 3 {
		return errors.New("authority generation lock identity is incomplete")
	}
	if lockFields == 0 && len(write.RelatedRecords) != 0 {
		return errors.New("related authority records require a lock identity")
	}
	for _, record := range write.RelatedRecords {
		if record.Namespace == AuthorityGenerationNamespace {
			return ErrAuthorityGenerationNamespaceReserved
		}
	}

	payload, err := json.Marshal(write.Generation)
	if err != nil {
		return fmt.Errorf("encode authority generation: %w", err)
	}
	sum := sha256.Sum256(payload)
	objectDigest := "sha256:" + hex.EncodeToString(sum[:])
	aad := SecureBlobAAD(AuthorityGenerationNamespace, write.Generation.Ref, write.Generation.Version, objectDigest)
	envelope, err := write.Crypto.Seal(ctx, write.KeyRef, write.Profile, payload, aad)
	if err != nil {
		return fmt.Errorf("seal authority generation: %w", err)
	}
	record := SecureBlobRecord{
		Namespace:     AuthorityGenerationNamespace,
		ObjectID:      write.Generation.Ref,
		ObjectVersion: write.Generation.Version,
		ObjectDigest:  objectDigest,
		Sensitivity:   write.Sensitivity,
		CryptoProfile: write.Profile,
		Envelope:      envelope,
		CreatedAt:     write.CreatedAt,
		ExpiresAt:     write.ExpiresAt,
	}
	var live []SecureBlobRecord
	if write.MarkLive {
		liveRecord, err := SealedLivenessRecord(ctx, write.Crypto, write.KeyRef, write.Profile, write.Sensitivity, AuthorityGenerationLiveNamespace, write.Generation.Ref, write.Generation.Version, write.Generation.Digest, write.CreatedAt, write.AdmittedAtSeq)
		if err != nil {
			return err
		}
		live = append(live, liveRecord)
	}
	if lockFields == 0 {
		if len(live) == 0 {
			return s.putSecureBlob(ctx, record)
		}
		return s.putSecureBlobsTx(ctx, append(live, record))
	}
	if len(write.RelatedRecords) == 0 && len(live) == 0 {
		return s.putSecureBlobWithLock(ctx, record, write.LockNamespace, write.LockID, write.LockVersion)
	}
	records := append(append(append([]SecureBlobRecord(nil), write.RelatedRecords...), live...), record)
	return s.putSecureBlobsWithLock(ctx, records, write.LockNamespace, write.LockID, write.LockVersion)
}

// ErrRootSuccessorRequiresSuccession closes the generic typed writer to any
// root successor: a generation binding a predecessor is admitted only by
// PutRootAuthoritySuccessor together with its durable proposal, review,
// decision, and predecessor supersession (ADR-089, ADR-090).
var ErrRootSuccessorRequiresSuccession = errors.New("root successor generations are admitted only through governed root succession")

func containsAuthority(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func authorityGenerationHasInstallationRepair(generation contracts.AuthorityGeneration) bool {
	for _, authority := range generation.Authorities {
		if authority == contracts.GovernedInstallationRepairStorageSchema || authority == contracts.GovernedInstallationRepairRuntimeState {
			return true
		}
	}
	return false
}

type RootAuthoritySuccessionWrite struct {
	// AdmittedAtSeq stamps the successor's liveness record.
	AdmittedAtSeq uint64
	Proposal      contracts.RootAuthoritySuccessionProposal
	Review        contracts.RootAuthoritySuccessionReview
	Decision      contracts.RootAuthoritySuccessionDecision
	Crypto        praxiscrypto.EnvelopeService
	KeyRef        string
	Profile       contracts.CryptoProfile
	Sensitivity   Sensitivity
	CreatedAt     time.Time
}

// PutRootAuthoritySuccessor is the sole production persistence boundary for a
// repair-bearing root generation. It reloads the exact durable proposal and
// review, locks and revalidates the predecessor, and atomically persists the
// decision, successor and predecessor supersession.
func (s *Store) PutRootAuthoritySuccessor(ctx context.Context, write RootAuthoritySuccessionWrite) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	proposalDigest, err := write.Proposal.Digest()
	if err != nil {
		return err
	}
	reviewDigest, err := write.Review.Digest()
	if err != nil {
		return err
	}
	if _, err := write.Decision.Digest(); err != nil {
		return err
	}
	if write.Review.ProposalID != write.Proposal.ID || write.Review.ProposalVersion != write.Proposal.Version || write.Review.ProposalDigest != proposalDigest ||
		write.Decision.ProposalID != write.Proposal.ID || write.Decision.ProposalVersion != write.Proposal.Version || write.Decision.ProposalDigest != proposalDigest ||
		write.Decision.ReviewID != write.Review.ID || write.Decision.ReviewVersion != write.Review.Version || write.Decision.ReviewDigest != reviewDigest ||
		write.Decision.PredecessorRef != write.Proposal.Predecessor.Ref || write.Decision.PredecessorVersion != write.Proposal.Predecessor.Version || write.Decision.PredecessorDigest != write.Proposal.Predecessor.Digest ||
		write.Decision.SuccessorRef != write.Proposal.Successor.Ref || write.Decision.SuccessorVersion != write.Proposal.Successor.Version || write.Decision.SuccessorDigest != write.Proposal.Successor.Digest ||
		write.Decision.BootstrapDigest != write.Proposal.BootstrapDigest || write.Review.ReviewedBy != write.Proposal.ProposedBy || write.Decision.DecidedBy != write.Proposal.ProposedBy {
		return errors.New("root-authority succession records do not bind one exact transition")
	}
	if write.Proposal.Successor.ParentRef != "" || write.Proposal.Successor.DelegatedBy != (contracts.PrincipalRef{}) || write.Proposal.Successor.PreDelegationForm() {
		return errors.New("root-authority successor must be a nondelegable current-representation root")
	}
	switch write.Proposal.Kind {
	case contracts.RootAuthoritySuccessionProposalKind:
		if !authorityGenerationHasInstallationRepair(write.Proposal.Successor) || write.Proposal.Predecessor.PreDelegationForm() {
			return errors.New("root-authority successor is not a nondelegable repair-bearing root")
		}
	case contracts.HistoricalRootModernizationProposalKind:
		// ADR-090: the successor establishes current authority only. It must
		// descend from the exact historical enrollment record and must not
		// acquire repair authority; that remains ADR-089's separate ceremony.
		governanceScope, err := contracts.InstallationGovernanceScope(write.Proposal.BootstrapDigest)
		if err != nil {
			return err
		}
		if !write.Proposal.Predecessor.PreDelegationForm() || authorityGenerationHasInstallationRepair(write.Proposal.Successor) || write.Proposal.Successor.Scope != governanceScope || write.Proposal.Successor.Ref != governanceScope || write.Proposal.Successor.AuthorityModel != contracts.AuthorityModelID || write.Proposal.Successor.AuthorityModelVersion != contracts.AuthorityModelVersion || write.Proposal.Successor.AuthorityModelDigest != contracts.AuthorityModelDigest() || !containsAuthority(write.Proposal.Successor.Capabilities, contracts.AuthorityDelegateCapability) {
			return errors.New("historical-root modernization successor is not the closed current canonical root")
		}
	default:
		return fmt.Errorf("unknown root-authority succession proposal kind %q", write.Proposal.Kind)
	}
	if write.KeyRef == "" || write.CreatedAt.IsZero() {
		return errors.New("root-authority succession persistence metadata is incomplete")
	}
	if err := write.Profile.Validate(); err != nil {
		return err
	}
	if err := write.Sensitivity.Validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin root-authority succession: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, AuthorityGenerationNamespace, write.Proposal.Predecessor.Ref, write.Proposal.Predecessor.Version)
	if err != nil {
		return fmt.Errorf("lock root predecessor: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return errors.New("root predecessor is unavailable")
	}
	predecessor, err := loadAuthorityGenerationRecord(ctx, tx, write.Crypto, write.Proposal.Predecessor.Ref, write.Proposal.Predecessor.Version, write.CreatedAt)
	if err != nil || !bytes.Equal(mustJSON(predecessor), mustJSON(write.Proposal.Predecessor)) {
		return errors.New("root predecessor changed or was substituted")
	}
	var invalidated int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, AuthorityGenerationInvalidationNamespace, predecessor.Ref, predecessor.Version).Scan(&invalidated); err != nil {
		return err
	}
	if invalidated != 0 {
		return errors.New("root predecessor is already superseded or revoked")
	}
	if err := verifyGovernanceRecordInTx(ctx, tx, write.Crypto, RootAuthoritySuccessionProposalNamespace, write.Proposal.ID, write.Proposal.Version, write.Proposal, write.CreatedAt); err != nil {
		return fmt.Errorf("verify succession proposal: %w", err)
	}
	if err := verifyGovernanceRecordInTx(ctx, tx, write.Crypto, RootAuthoritySuccessionReviewNamespace, write.Review.ID, write.Review.Version, write.Review, write.CreatedAt); err != nil {
		return fmt.Errorf("verify succession review: %w", err)
	}

	invalidation := contracts.AuthorityGenerationInvalidation{Ref: predecessor.Ref, Version: predecessor.Version, GenerationDigest: predecessor.Digest, InvalidationRef: "root-authority-succession:" + proposalDigest, InvalidationVersion: "1", Kind: "superseded", SupersededBy: write.Proposal.Successor.Version, InvalidatedBy: write.Decision.DecidedBy, EffectiveAt: write.CreatedAt.UTC(), Reason: "governed root-authority succession"}
	if err := invalidation.Validate(predecessor); err != nil {
		return err
	}
	for _, item := range []struct {
		namespace, id, version string
		value                  any
	}{
		{RootAuthoritySuccessionDecisionNamespace, write.Decision.ID, write.Decision.Version, write.Decision},
		{AuthorityGenerationInvalidationNamespace, invalidation.Ref, invalidation.Version, invalidation},
		{AuthorityGenerationNamespace, write.Proposal.Successor.Ref, write.Proposal.Successor.Version, write.Proposal.Successor},
	} {
		record, err := sealedTypedRecord(ctx, write.Crypto, write.KeyRef, write.Profile, write.Sensitivity, item.namespace, item.id, item.version, item.value, write.CreatedAt)
		if err != nil {
			return err
		}
		if err := insertSecureBlobTx(ctx, tx, record); err != nil {
			return err
		}
	}
	// I12: the superseded root is retired and the successor made live in the
	// same transaction that supersedes and admits them.
	if _, err := tx.ExecContext(ctx, `DELETE FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, AuthorityGenerationLiveNamespace, predecessor.Ref, predecessor.Version); err != nil {
		return fmt.Errorf("retire root predecessor liveness: %w", err)
	}
	liveRecord, err := SealedLivenessRecord(ctx, write.Crypto, write.KeyRef, write.Profile, write.Sensitivity, AuthorityGenerationLiveNamespace, write.Proposal.Successor.Ref, write.Proposal.Successor.Version, write.Proposal.Successor.Digest, write.CreatedAt, write.AdmittedAtSeq)
	if err != nil {
		return err
	}
	if err := insertSecureBlobTx(ctx, tx, liveRecord); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit root-authority succession: %w", err)
	}
	return nil
}

func loadAuthorityGenerationRecord(ctx context.Context, q sqlRowQueryer, crypto praxiscrypto.EnvelopeService, ref, version string, now time.Time) (contracts.AuthorityGeneration, error) {
	record, err := getSecureBlob(ctx, q, AuthorityGenerationNamespace, ref, version, now, true)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	payload, err := crypto.Open(ctx, record.Envelope, SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	var generation contracts.AuthorityGeneration
	if err := json.Unmarshal(payload, &generation); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if generation.Ref != ref || generation.Version != version || payloadDigestState(payload) != record.ObjectDigest || generation.VerifyDigest() != nil {
		return contracts.AuthorityGeneration{}, errors.New("authority generation record mismatch")
	}
	return generation, nil
}

func verifyGovernanceRecordInTx(ctx context.Context, q sqlRowQueryer, crypto praxiscrypto.EnvelopeService, namespace, id, version string, value any, now time.Time) error {
	record, err := getSecureBlob(ctx, q, namespace, id, version, now, true)
	if err != nil {
		return err
	}
	payload, err := crypto.Open(ctx, record.Envelope, SecureBlobAAD(namespace, id, version, record.ObjectDigest))
	if err != nil {
		return err
	}
	return compareCanonicalJSON(payload, value)
}

func compareCanonicalJSON(payload []byte, value any) error {
	expected, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(payload, expected) {
		return errors.New("durable governance record differs from exact value")
	}
	return nil
}

func mustJSON(value any) []byte { body, _ := json.Marshal(value); return body }

func payloadDigestState(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sealedTypedRecord(ctx context.Context, crypto praxiscrypto.EnvelopeService, keyRef string, profile contracts.CryptoProfile, sensitivity Sensitivity, namespace, id, version string, value any, createdAt time.Time) (SecureBlobRecord, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return SecureBlobRecord{}, err
	}
	digest := payloadDigestState(payload)
	aad := SecureBlobAAD(namespace, id, version, digest)
	envelope, err := crypto.Seal(ctx, keyRef, profile, payload, aad)
	if err != nil {
		return SecureBlobRecord{}, err
	}
	return SecureBlobRecord{Namespace: namespace, ObjectID: id, ObjectVersion: version, ObjectDigest: digest, Sensitivity: sensitivity, CryptoProfile: profile, Envelope: envelope, CreatedAt: createdAt}, nil
}

func insertSecureBlobTx(ctx context.Context, tx *sql.Tx, record SecureBlobRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt))
	if err != nil {
		return fmt.Errorf("insert root-authority succession record: %w", err)
	}
	return nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *Store) GetSecureBlob(ctx context.Context, namespace, objectID, version string, now time.Time) (SecureBlobRecord, error) {
	if s == nil || s.db == nil {
		return SecureBlobRecord{}, errors.New("state store is required")
	}
	return getSecureBlob(ctx, s.db, namespace, objectID, version, now, true)
}

// GetSecureBlobHistorical returns the immutable encrypted record without
// applying its operational expiry gate. Callers must use a dedicated typed
// historical-evidence contract; this method never opens or authorizes content.
func (s *Store) GetSecureBlobHistorical(ctx context.Context, namespace, objectID, version string) (SecureBlobRecord, error) {
	if s == nil || s.db == nil {
		return SecureBlobRecord{}, errors.New("state store is required")
	}
	return getSecureBlob(ctx, s.db, namespace, objectID, version, time.Time{}, false)
}

// GetSecureBlobInTx reads one secure_blobs record, with the same
// operational-expiry gate GetSecureBlob applies, through a caller-supplied
// transaction rather than a fresh pooled connection. This store's
// connection pool is capped at exactly one connection, so a query against
// the pool while a governed Apply transaction holds that one connection
// open would deadlock, not merely race — a StepDriver revalidating a
// repair authority decision's freshness inside its own transaction
// (ADR-088 §13.5) MUST use this method, never GetSecureBlob, while that
// transaction is open. It never decrypts Envelope; callers with the
// necessary key material do that themselves, exactly as with GetSecureBlob.
func (s *Store) GetSecureBlobInTx(ctx context.Context, tx *sql.Tx, namespace, objectID, version string, now time.Time) (SecureBlobRecord, error) {
	if s == nil || s.db == nil {
		return SecureBlobRecord{}, errors.New("state store is required")
	}
	if tx == nil {
		return SecureBlobRecord{}, errors.New("state store requires an active transaction")
	}
	return getSecureBlob(ctx, tx, namespace, objectID, version, now, true)
}

// IsSecureBlobRevokedInTx checks one revocation namespace/object for a
// revocation record, through a caller-supplied transaction — the tx-scoped
// counterpart to the inline revocation check PutSecureBlobUnlessRevoked
// already performs, exposed separately as a pure read so a governed Apply
// boundary (ADR-088 §13.5) can revalidate revocation state without itself
// performing a write. An empty namespace/id/version is treated as "no
// revocation namespace configured for this check" and reports false, not
// an error, matching PutSecureBlobUnlessRevoked's own optional-second-check
// convention.
func (s *Store) IsSecureBlobRevokedInTx(ctx context.Context, tx *sql.Tx, namespace, id, version string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("state store is required")
	}
	if tx == nil {
		return false, errors.New("state store requires an active transaction")
	}
	if namespace == "" || id == "" || version == "" {
		return false, nil
	}
	return revokedOrNotLiveInTx(ctx, tx, namespace, id, version)
}

// revokedOrNotLiveInTx is the single in-transaction "is this decision or
// generation out of force" predicate (I12). A decision or generation is in
// force only if its negative record is absent AND its sealed liveness record is
// present; absence of the negative record alone is never sufficient, so a
// keyless DELETE of a revocation or invalidation row cannot restore anything.
func revokedOrNotLiveInTx(ctx context.Context, tx *sql.Tx, namespace, id, version string, creating ...SecureBlobRecord) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, id, version).Scan(&count); err != nil {
		return false, fmt.Errorf("check authority revocation: %w", err)
	}
	if count != 0 {
		return true, nil
	}
	liveNamespace := ""
	switch namespace {
	case AuthorityRevocationNamespace:
		liveNamespace = AuthorityDecisionLiveNamespace
	case AuthorityGenerationInvalidationNamespace:
		liveNamespace = AuthorityGenerationLiveNamespace
	default:
		return false, nil
	}
	// A fact whose liveness record is being created by this very transaction
	// is being admitted, not consumed: only its negative record can refuse it.
	for _, record := range creating {
		if record.Namespace == liveNamespace && record.ObjectID == id && record.ObjectVersion == version {
			return false, nil
		}
	}
	present, err := LivenessPresentInTx(ctx, tx, liveNamespace, id, version)
	if err != nil {
		return false, err
	}
	return !present, nil
}

// LivenessPresentInTx reports whether the liveness row of a decision or
// generation exists, inside the caller's transaction.
func LivenessPresentInTx(ctx context.Context, tx *sql.Tx, namespace, id, version string) (bool, error) {
	if namespace != AuthorityDecisionLiveNamespace && namespace != AuthorityGenerationLiveNamespace {
		return false, ErrNotLivenessNamespace
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, id, version).Scan(&count); err != nil {
		return false, fmt.Errorf("check governance liveness: %w", err)
	}
	return count != 0, nil
}

// sqlRowQueryer is satisfied by both *sql.DB and *sql.Tx, letting
// getSecureBlob serve GetSecureBlob/GetSecureBlobHistorical (pool query)
// and GetSecureBlobInTx (caller-owned transaction) from one implementation.
type sqlRowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getSecureBlob(ctx context.Context, q sqlRowQueryer, namespace, objectID, version string, now time.Time, enforceExpiry bool) (SecureBlobRecord, error) {
	if namespace == "" || objectID == "" || version == "" {
		return SecureBlobRecord{}, errors.New("secure blob lookup identity is required")
	}
	var r SecureBlobRecord
	var sensitivity, profile, created string
	var envelopeJSON []byte
	var expires sql.NullString
	err := q.QueryRowContext(ctx, `SELECT object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, objectID, version).Scan(&r.ObjectDigest, &sensitivity, &profile, &envelopeJSON, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return SecureBlobRecord{}, ErrSecureBlobNotFound
	}
	if err != nil {
		return SecureBlobRecord{}, fmt.Errorf("load secure blob: %w", err)
	}
	r.Namespace, r.ObjectID, r.ObjectVersion = namespace, objectID, version
	r.Sensitivity = Sensitivity(sensitivity)
	r.CryptoProfile = contracts.CryptoProfile(profile)
	if err := json.Unmarshal(envelopeJSON, &r.Envelope); err != nil {
		return SecureBlobRecord{}, fmt.Errorf("decode secure blob envelope: %w", err)
	}
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return SecureBlobRecord{}, fmt.Errorf("parse secure blob creation time: %w", err)
	}
	if expires.Valid {
		t, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return SecureBlobRecord{}, fmt.Errorf("parse secure blob expiry: %w", err)
		}
		r.ExpiresAt = &t
		if enforceExpiry && now.IsZero() {
			now = time.Now().UTC()
		}
		if enforceExpiry && !now.Before(t) {
			return SecureBlobRecord{}, ErrSecureBlobExpired
		}
	}
	if err := r.Validate(); err != nil {
		return SecureBlobRecord{}, fmt.Errorf("stored secure blob failed validation: %w", err)
	}
	return r, nil
}

// HasSecureBlobDigest checks immutable lineage without opening encrypted
// content. Import uses it to require a predecessor generation.
func (s *Store) HasSecureBlobDigest(ctx context.Context, namespace, objectID, digest string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("state store is required")
	}
	if namespace == "" || objectID == "" || digest == "" {
		return false, errors.New("secure blob lineage identity is required")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_digest=?`, namespace, objectID, digest).Scan(&count); err != nil {
		return false, fmt.Errorf("check secure blob lineage: %w", err)
	}
	return count == 1, nil
}

// ListSecureBlobs returns immutable records for a namespace in stable identity
// order. Callers still decrypt and validate each record through their owning
// contract; this method exposes no plaintext.
func (s *Store) ListSecureBlobs(ctx context.Context, namespace string, now time.Time) ([]SecureBlobRecord, error) {
	if namespace == "" {
		return nil, errors.New("secure blob namespace is required")
	}
	return s.ListSecureBlobsInNamespaces(ctx, []string{namespace}, now)
}

// ListSecureBlobsInNamespaces lists every record of the given namespaces with
// one statement, so callers that must relate records across namespaces (for
// example generations and their invalidations) observe a single consistent
// snapshot rather than two statements that a concurrent commit may split.
func (s *Store) ListSecureBlobsInNamespaces(ctx context.Context, namespaces []string, now time.Time) ([]SecureBlobRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	return listSecureBlobs(ctx, s.db, namespaces, now)
}

func listSecureBlobs(ctx context.Context, q sqlQueryer, namespaces []string, now time.Time) ([]SecureBlobRecord, error) {
	if len(namespaces) == 0 {
		return nil, errors.New("secure blob namespace is required")
	}
	args := make([]any, 0, len(namespaces))
	marks := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		if namespace == "" {
			return nil, errors.New("secure blob namespace is required")
		}
		args = append(args, namespace)
		marks = append(marks, "?")
	}
	rows, err := q.QueryContext(ctx, `SELECT namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at FROM secure_blobs WHERE namespace IN (`+strings.Join(marks, ",")+`) ORDER BY namespace,object_id,object_version`, args...)
	if err != nil {
		return nil, fmt.Errorf("list secure blobs: %w", err)
	}
	defer rows.Close()
	var records []SecureBlobRecord
	for rows.Next() {
		var r SecureBlobRecord
		var sensitivity, profile, created string
		var envelopeJSON []byte
		var expires sql.NullString
		if err := rows.Scan(&r.Namespace, &r.ObjectID, &r.ObjectVersion, &r.ObjectDigest, &sensitivity, &profile, &envelopeJSON, &created, &expires); err != nil {
			return nil, fmt.Errorf("scan secure blob: %w", err)
		}
		r.Sensitivity, r.CryptoProfile = Sensitivity(sensitivity), contracts.CryptoProfile(profile)
		if err := json.Unmarshal(envelopeJSON, &r.Envelope); err != nil {
			return nil, fmt.Errorf("decode secure blob envelope: %w", err)
		}
		r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse secure blob creation time: %w", err)
		}
		if expires.Valid {
			t, parseErr := time.Parse(time.RFC3339Nano, expires.String)
			if parseErr != nil {
				return nil, fmt.Errorf("parse secure blob expiry: %w", parseErr)
			}
			r.ExpiresAt = &t
		}
		if r.ExpiresAt != nil && (now.IsZero() || !now.Before(*r.ExpiresAt)) {
			continue
		}
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("listed secure blob failed validation: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// Liveness namespaces hold POSITIVE, sealed records whose presence is REQUIRED
// for a governance fact to grant anything (I12, deletion-monotonicity). A
// revocation or invalidation retires the record by deleting it. Because a
// consequence-permitting predicate demands presence, a writer of the database
// file who can only DELETE rows can never widen what is permitted: deleting the
// revocation record leaves the decision without its liveness record, and
// deleting the liveness record refuses. Restoring authority would require
// inserting a previously copied sealed row (a replay), which is outside the
// delete-only capability the accepted trust model denies.
const (
	AuthorityDecisionLiveNamespace   = "authority_decision_live"
	AuthorityGenerationLiveNamespace = "authority_generation_live"
)

// ErrNotLivenessNamespace refuses deletion outside the liveness namespaces: the
// rest of secure_blobs stays immutable.
var ErrNotLivenessNamespace = errors.New("only liveness records may be retired")

// DeleteLivenessRecord retires one liveness record. It is idempotent (a record
// that is already absent is retired), and it refuses every other namespace.
func (s *Store) DeleteLivenessRecord(ctx context.Context, namespace, objectID, objectVersion string) error {
	if namespace != AuthorityDecisionLiveNamespace && namespace != AuthorityGenerationLiveNamespace {
		return ErrNotLivenessNamespace
	}
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if objectID == "" || objectVersion == "" {
		return errors.New("liveness record identity is required")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, objectID, objectVersion); err != nil {
		return fmt.Errorf("retire liveness record: %w", err)
	}
	return nil
}

// LivenessRecord is the sealed payload of a liveness row: it names the exact
// governance fact (identity and digest) it keeps alive.
type LivenessRecord struct {
	Namespace string `json:"namespace"`
	ID        string `json:"id"`
	Version   string `json:"version"`
	Digest    string `json:"digest"`
	// AdmittedAtSeq is the Forward Authority Anchor sequence at admission (0
	// when the installation is unanchored). A governed re-anchor at sequence S
	// voids every admission stamped below S, so restoring an older store never
	// makes an older admission current again (I13).
	AdmittedAtSeq uint64 `json:"admitted_at_seq,omitempty"`
}

// SealedLivenessRecord seals a liveness row for atomic insertion together with
// the governance record it keeps alive. It refuses any other namespace.
func SealedLivenessRecord(ctx context.Context, crypto praxiscrypto.EnvelopeService, keyRef string, profile contracts.CryptoProfile, sensitivity Sensitivity, namespace, id, version, digest string, createdAt time.Time, admittedAtSeq ...uint64) (SecureBlobRecord, error) {
	if namespace != AuthorityDecisionLiveNamespace && namespace != AuthorityGenerationLiveNamespace {
		return SecureBlobRecord{}, ErrNotLivenessNamespace
	}
	if id == "" || version == "" || digest == "" {
		return SecureBlobRecord{}, errors.New("liveness identity and digest are required")
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return sealedTypedRecord(ctx, crypto, keyRef, profile, sensitivity, namespace, id, version, LivenessRecord{Namespace: namespace, ID: id, Version: version, Digest: digest, AdmittedAtSeq: firstSeq(admittedAtSeq)}, createdAt)
}

func (s *Store) putSecureBlobsTx(ctx context.Context, records []SecureBlobRecord) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin secure blob transition: %w", err)
	}
	defer tx.Rollback()
	for _, record := range records {
		if err := insertSecureBlobTx(ctx, tx, record); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit secure blob transition: %w", err)
	}
	return nil
}

// PutSecureBlobsAtomically writes a record together with its liveness records
// in one transaction. Every record other than the first must be a liveness
// record, so this is not a generic multi-write; the reserved authority
// generation namespace is refused.
func (s *Store) PutSecureBlobsAtomically(ctx context.Context, records []SecureBlobRecord) error {
	if len(records) == 0 {
		return errors.New("secure blob records are required")
	}
	if records[0].Namespace == AuthorityGenerationNamespace {
		return ErrAuthorityGenerationNamespaceReserved
	}
	for _, record := range records[1:] {
		if record.Namespace != AuthorityDecisionLiveNamespace && record.Namespace != AuthorityGenerationLiveNamespace {
			return ErrNotLivenessNamespace
		}
	}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	return s.putSecureBlobsTx(ctx, records)
}

func firstSeq(v []uint64) uint64 {
	if len(v) == 0 {
		return 0
	}
	return v[0]
}

// GovernanceFactNamespace holds the Forward Authority Anchor's fact chain: one
// authenticated row per governance transition whose loss or replay would broaden
// permission. Object id is the zero-padded sequence number.
const GovernanceFactNamespace = "governance_fact"

// GovernanceFactID is the object id of the fact with the given sequence.
func GovernanceFactID(seq uint64) string { return fmt.Sprintf("%020d", seq) }

// ListGovernanceFactRecords returns every fact row in one statement.
func (s *Store) ListGovernanceFactRecords(ctx context.Context) ([]SecureBlobRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	return listSecureBlobs(ctx, s.db, []string{GovernanceFactNamespace}, time.Unix(0, 0).UTC())
}

// ListGovernanceFactRecordsTx is ListGovernanceFactRecords inside the caller's
// transaction, so a check made inside a governed write sees the same chain the
// write will extend.
func ListGovernanceFactRecordsTx(ctx context.Context, tx *sql.Tx) ([]SecureBlobRecord, error) {
	if tx == nil {
		return nil, errors.New("a transaction is required")
	}
	return listSecureBlobs(ctx, tx, []string{GovernanceFactNamespace}, time.Unix(0, 0).UTC())
}

// GovernanceFactBuilder builds the next fact row from the chain as it stands
// inside the write lock. It authenticates and verifies that chain against the
// anchor, seals the new fact and advances the anchor, then returns the row and
// an undo that reverts the anchor if the database commit fails. Returning a nil
// record with a nil error means the transition is already recorded and nothing
// is written.
type GovernanceFactBuilder func(ctx context.Context, tx *sql.Tx, existing []SecureBlobRecord) (SecureBlobRecord, func(context.Context), error)

// AppendGovernanceFact runs build inside a write transaction (the SQLite writer
// lock serialises every appender in every process), inserts the row build
// returns and commits. The anchor is advanced by build BEFORE the commit, so a
// crash between the two leaves the anchor ahead of the store: the store then
// refuses to be consumed until a governed re-anchor, which is the fail-closed
// direction. The reverse order would let a crash leave a store that has lost a
// retirement the anchor never learned about.
func (s *Store) AppendGovernanceFact(ctx context.Context, build GovernanceFactBuilder) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin governance fact transition: %w", err)
	}
	defer tx.Rollback()
	// A write statement that matches nothing still takes the writer lock.
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=''`, GovernanceFactNamespace); err != nil {
		return fmt.Errorf("lock governance fact chain: %w", err)
	}
	existing, err := ListGovernanceFactRecordsTx(ctx, tx)
	if err != nil {
		return err
	}
	record, undo, err := build(ctx, tx, existing)
	if err != nil {
		return err
	}
	if record.Namespace == "" {
		return nil
	}
	if record.Namespace != GovernanceFactNamespace {
		if undo != nil {
			undo(ctx)
		}
		return errors.New("only governance facts may be appended through the fact chain")
	}
	if err := insertSecureBlobTx(ctx, tx, record); err != nil {
		if undo != nil {
			undo(ctx)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		if undo != nil {
			undo(ctx)
		}
		return fmt.Errorf("commit governance fact: %w", err)
	}
	return nil
}

// ReplaceLivenessRecord re-stamps a liveness record in one transaction (delete
// then insert). It is used only by governed re-anchoring to re-admit the
// installation root.
func (s *Store) ReplaceLivenessRecord(ctx context.Context, record SecureBlobRecord) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if record.Namespace != AuthorityDecisionLiveNamespace && record.Namespace != AuthorityGenerationLiveNamespace {
		return ErrNotLivenessNamespace
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, record.Namespace, record.ObjectID, record.ObjectVersion); err != nil {
		return err
	}
	if err := insertSecureBlobTx(ctx, tx, record); err != nil {
		return err
	}
	return tx.Commit()
}

// ListSecureBlobsInNamespacesTx is ListSecureBlobsInNamespaces inside the
// caller's transaction (the store uses a single connection, so a governed write
// must read through its own transaction).
func ListSecureBlobsInNamespacesTx(ctx context.Context, tx *sql.Tx, namespaces []string, now time.Time) ([]SecureBlobRecord, error) {
	if tx == nil {
		return nil, errors.New("a transaction is required")
	}
	return listSecureBlobs(ctx, tx, namespaces, now)
}

// DeleteSecureBlobInTx and InsertSecureBlobInTx let a governed re-anchor move
// orphaned fact rows out of the chain in the same transaction that bridges it.
func DeleteSecureBlobInTx(ctx context.Context, tx *sql.Tx, namespace, id, version string) error {
	if namespace != GovernanceFactNamespace {
		return errors.New("only governance fact rows may be moved by a re-anchor")
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, id, version)
	return err
}

func InsertSecureBlobInTx(ctx context.Context, tx *sql.Tx, record SecureBlobRecord) error {
	if record.Namespace != "governance_fact_orphan" {
		return errors.New("only orphaned governance facts may be inserted by a re-anchor")
	}
	return insertSecureBlobTx(ctx, tx, record)
}

// WithWriteLock runs fn inside a write transaction (the store opens every
// transaction with the writer lock, so this waits for every in-flight writer of
// every process) and rolls it back. Its use is a barrier: inside it no write is
// in flight, so a comparison of the fact chain with the anchor made through tx
// cannot be confused by a concurrent append.
func (s *Store) WithWriteLock(ctx context.Context, fn func(tx *sql.Tx) error) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("take the governance write lock: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=''`, GovernanceFactNamespace); err != nil {
		return fmt.Errorf("take the governance write lock: %w", err)
	}
	return fn(tx)
}
