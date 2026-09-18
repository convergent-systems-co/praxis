package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func sealedRecordInNamespace(t *testing.T, namespace string, now time.Time) SecureBlobRecord {
	t.Helper()
	wrapper := &testKeyWrapper{}
	service := praxiscrypto.EnvelopeService{Wrapper: wrapper}
	aad := SecureBlobAAD(namespace, "obj-1", "1", "sha256:obj")
	env, err := service.Seal(context.Background(), "key:test", contracts.CryptoPQRequired, []byte("payload"), aad)
	if err != nil {
		t.Fatal(err)
	}
	return SecureBlobRecord{Namespace: namespace, ObjectID: "obj-1", ObjectVersion: "1", ObjectDigest: "sha256:obj", Sensitivity: SensitivityConfidential, CryptoProfile: contracts.CryptoPQRequired, Envelope: env, CreatedAt: now}
}

func openAuthorityNamespaceTestDB(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db)
}

// TestGenericWritersRejectAuthorityGenerationNamespace proves all four
// generic secure_blobs writers structurally refuse a direct write to the
// reserved AuthorityGenerationNamespace — ADR-088 §10(b)'s bypass-prevention
// closure (PLAN-016 WU4) — using only the record's cleartext Namespace
// field, never inspecting the sealed Envelope.
func TestGenericWritersRejectAuthorityGenerationNamespace(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()
	record := sealedRecordInNamespace(t, AuthorityGenerationNamespace, now)

	if err := store.PutSecureBlob(context.Background(), record); !errors.Is(err, ErrAuthorityGenerationNamespaceReserved) {
		t.Fatalf("PutSecureBlob: expected reserved-namespace rejection, got %v", err)
	}
	if err := store.PutSecureBlobWithLock(context.Background(), record, "lock-ns", "lock-id", "1"); !errors.Is(err, ErrAuthorityGenerationNamespaceReserved) {
		t.Fatalf("PutSecureBlobWithLock: expected reserved-namespace rejection, got %v", err)
	}
	if err := store.PutSecureBlobsWithLock(context.Background(), []SecureBlobRecord{record}, "lock-ns", "lock-id", "1"); !errors.Is(err, ErrAuthorityGenerationNamespaceReserved) {
		t.Fatalf("PutSecureBlobsWithLock: expected reserved-namespace rejection for a batch containing one reserved record, got %v", err)
	}
	if err := store.PutSecureBlobUnlessRevoked(context.Background(), record, "rev-ns", "req-1", "1", "", "", "", "lock-ns", "lock-id", "1"); !errors.Is(err, ErrAuthorityGenerationNamespaceReserved) {
		t.Fatalf("PutSecureBlobUnlessRevoked: expected reserved-namespace rejection, got %v", err)
	}
}

// TestGenericWritersUnaffectedForOtherNamespaces proves the reserved-namespace
// guard does not disturb ordinary writes to any other namespace — the four
// generic writers keep working exactly as before for everything except
// AuthorityGenerationNamespace.
func TestGenericWritersUnaffectedForOtherNamespaces(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()

	if err := store.PutSecureBlob(context.Background(), sealedRecordInNamespaceWithIdentity(t, "goals", "goal-a", "1", now)); err != nil {
		t.Fatalf("PutSecureBlob unexpectedly rejected an unrelated namespace: %v", err)
	}
	lockRecord := sealedRecordInNamespaceWithIdentity(t, "lock-source", "lock-a", "1", now)
	if err := store.PutSecureBlob(context.Background(), lockRecord); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSecureBlobWithLock(context.Background(), sealedRecordInNamespaceWithIdentity(t, "goals", "goal-b", "1", now), "lock-source", "lock-a", "1"); err != nil {
		t.Fatalf("PutSecureBlobWithLock unexpectedly rejected an unrelated namespace: %v", err)
	}
	if err := store.PutSecureBlobsWithLock(context.Background(), []SecureBlobRecord{sealedRecordInNamespaceWithIdentity(t, "goals", "goal-c", "1", now)}, "lock-source", "lock-a", "1"); err != nil {
		t.Fatalf("PutSecureBlobsWithLock unexpectedly rejected an unrelated namespace: %v", err)
	}
	if err := store.PutSecureBlobUnlessRevoked(context.Background(), sealedRecordInNamespaceWithIdentity(t, "goals", "goal-d", "1", now), "revocation-ns", "req-1", "1", "", "", "", "lock-source", "lock-a", "1"); err != nil {
		t.Fatalf("PutSecureBlobUnlessRevoked unexpectedly rejected an unrelated namespace: %v", err)
	}
}

func authorityGenerationPersistenceFixture(now time.Time) contracts.AuthorityGeneration {
	generation := contracts.AuthorityGeneration{
		Ref: "installation-governance:root", Version: "1",
		Principal: contracts.PrincipalRef{ID: "installation-owner:root", Kind: "human"},
		Scope:     "installation-governance:root", ProvenanceRef: "bootstrap:test",
		ProvenanceDigest: "sha256:" + strings.Repeat("a", 64),
		State:            contracts.AuthorityGenerationActive, EffectiveAt: now,
		Capabilities: []string{contracts.AuthorityDelegateCapability},
	}
	generation.Digest, _ = generation.ComputeDigest()
	return generation
}

func TestPutAuthorityGenerationRejectsRepairBearingRootOutsideSuccession(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()
	generation := authorityGenerationPersistenceFixture(now)
	generation.Authorities = []string{contracts.GovernedInstallationRepairStorageSchema}
	generation.Digest, _ = generation.ComputeDigest()
	if err := store.PutAuthorityGeneration(context.Background(), authorityGenerationWriteFixture(generation, now)); !errors.Is(err, ErrRepairAuthorityRequiresRootSuccession) {
		t.Fatalf("repair-bearing root bypass error = %v", err)
	}
}

func authorityGenerationWriteFixture(generation contracts.AuthorityGeneration, now time.Time) AuthorityGenerationWrite {
	return AuthorityGenerationWrite{
		Generation: generation, Crypto: praxiscrypto.EnvelopeService{Wrapper: &testKeyWrapper{}},
		KeyRef: "key:test", Profile: contracts.CryptoPQRequired,
		Sensitivity: SensitivityConfidential, CreatedAt: now,
	}
}

func TestPutAuthorityGenerationPersistsExactValidatedTypedValue(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()
	generation := authorityGenerationPersistenceFixture(now)
	write := authorityGenerationWriteFixture(generation, now)
	if err := store.PutAuthorityGeneration(context.Background(), write); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetSecureBlob(context.Background(), AuthorityGenerationNamespace, generation.Ref, generation.Version, now)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := write.Crypto.Open(context.Background(), loaded.Envelope, SecureBlobAAD(loaded.Namespace, loaded.ObjectID, loaded.ObjectVersion, loaded.ObjectDigest))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, want) {
		t.Fatalf("persisted plaintext differs from the exact validated value:\n got %s\nwant %s", payload, want)
	}
}

func TestPutAuthorityGenerationRejectsDelegatedRepairAuthority(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()

	withParent := authorityGenerationPersistenceFixture(now)
	withParent.Authorities = []string{contracts.GovernedInstallationRepairStorageSchema}
	withParent.ParentRef = "authority:parent"
	withParent.ParentVersion = "1"
	withParent.ParentDigest = "sha256:" + strings.Repeat("b", 64)
	withParent.DelegatedBy = contracts.PrincipalRef{ID: "principal:parent", Kind: "human"}
	withParent.DelegationRef = "delegation:1"
	withParent.DelegationDigest = "sha256:" + strings.Repeat("c", 64)
	withParent.PolicyRef = "policy:test"
	withParent.PolicyVersion = "1"
	withParent.PolicyDigest = "sha256:" + strings.Repeat("d", 64)
	withParent.Digest, _ = withParent.ComputeDigest()
	if err := store.PutAuthorityGeneration(context.Background(), authorityGenerationWriteFixture(withParent, now)); err == nil {
		t.Fatal("typed admission accepted repair authority with a non-empty ParentRef")
	}

	withDelegatedBy := authorityGenerationPersistenceFixture(now)
	withDelegatedBy.Authorities = []string{contracts.GovernedInstallationRepairRuntimeState}
	withDelegatedBy.DelegatedBy = contracts.PrincipalRef{ID: "principal:delegate", Kind: "human"}
	withDelegatedBy.Digest, _ = withDelegatedBy.ComputeDigest()
	if err := store.PutAuthorityGeneration(context.Background(), authorityGenerationWriteFixture(withDelegatedBy, now)); err == nil {
		t.Fatal("typed admission accepted repair authority with DelegatedBy but no ParentRef")
	}
}

func TestGetSecureBlobInTxMatchesGetSecureBlobAndRequiresActiveTransaction(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()
	record := sealedRecordInNamespace(t, "goals", now)
	if err := store.PutSecureBlob(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSecureBlobInTx(context.Background(), nil, record.Namespace, record.ObjectID, record.ObjectVersion, now); err == nil {
		t.Fatal("expected error for nil transaction")
	}

	tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	inTx, err := store.GetSecureBlobInTx(context.Background(), tx, record.Namespace, record.ObjectID, record.ObjectVersion, now)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	// The transaction must be closed before querying through the pool
	// again — this store's pool is capped at exactly one connection, so
	// holding tx open while also issuing a pooled query would deadlock,
	// not merely race (this is precisely the hazard GetSecureBlobInTx
	// exists to let callers avoid by staying inside their own tx).
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	outside, err := store.GetSecureBlob(context.Background(), record.Namespace, record.ObjectID, record.ObjectVersion, now)
	if err != nil {
		t.Fatal(err)
	}
	if inTx.ObjectDigest != outside.ObjectDigest {
		t.Fatalf("GetSecureBlobInTx does not match GetSecureBlob: %+v vs %+v", inTx, outside)
	}
}

func TestGetSecureBlobInTxEnforcesExpiryLikeGetSecureBlob(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()
	record := sealedRecordInNamespace(t, "goals", now.Add(-time.Hour))
	expired := now.Add(-time.Minute)
	record.ExpiresAt = &expired
	if err := store.PutSecureBlob(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := store.GetSecureBlobInTx(context.Background(), tx, record.Namespace, record.ObjectID, record.ObjectVersion, now); !errors.Is(err, ErrSecureBlobExpired) {
		t.Fatalf("expected ErrSecureBlobExpired inside the transaction, got %v", err)
	}
}

func TestIsSecureBlobRevokedInTx(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	now := time.Now().UTC()

	tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.IsSecureBlobRevokedInTx(context.Background(), nil, "revocation", "req-1", "1"); err == nil {
		t.Fatal("expected error for nil transaction")
	}
	revoked, err := store.IsSecureBlobRevokedInTx(context.Background(), tx, "revocation", "req-1", "1")
	if err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Fatal("no revocation record exists yet; expected false")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if err := store.PutSecureBlob(context.Background(), sealedRecordInNamespaceWithIdentity(t, "revocation", "req-1", "1", now)); err != nil {
		t.Fatal(err)
	}
	tx2, err := store.db.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback()
	revoked2, err := store.IsSecureBlobRevokedInTx(context.Background(), tx2, "revocation", "req-1", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !revoked2 {
		t.Fatal("expected revocation record to be observed inside the transaction")
	}

	// An empty check namespace/id/version (the "no additional check
	// configured" convention PutSecureBlobUnlessRevoked already uses)
	// reports false, not an error.
	revoked3, err := store.IsSecureBlobRevokedInTx(context.Background(), tx2, "", "", "")
	if err != nil || revoked3 {
		t.Fatalf("expected no-op check to report false with no error: %v %v", revoked3, err)
	}
}

func sealedRecordInNamespaceWithIdentity(t *testing.T, namespace, id, version string, now time.Time) SecureBlobRecord {
	t.Helper()
	wrapper := &testKeyWrapper{}
	service := praxiscrypto.EnvelopeService{Wrapper: wrapper}
	aad := SecureBlobAAD(namespace, id, version, "sha256:obj")
	env, err := service.Seal(context.Background(), "key:test", contracts.CryptoPQRequired, []byte("payload"), aad)
	if err != nil {
		t.Fatal(err)
	}
	return SecureBlobRecord{Namespace: namespace, ObjectID: id, ObjectVersion: version, ObjectDigest: "sha256:obj", Sensitivity: SensitivityConfidential, CryptoProfile: contracts.CryptoPQRequired, Envelope: env, CreatedAt: now}
}
