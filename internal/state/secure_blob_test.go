package state

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type testKeyWrapper struct{ key []byte }

func (w *testKeyWrapper) Capabilities(context.Context, string) (praxiscrypto.Capabilities, error) {
	return praxiscrypto.Capabilities{PQ: true}, nil
}
func (w *testKeyWrapper) Wrap(_ context.Context, keyRef string, profile contracts.CryptoProfile, key []byte) (praxiscrypto.WrappedKey, error) {
	w.key = append([]byte(nil), key...)
	return praxiscrypto.WrappedKey{Ciphertext: []byte("opaque-wrapped-key"), SuiteID: "test-pq-suite", KeyRef: keyRef, KeyVersion: "1", SelectedProfile: profile}, nil
}
func (w *testKeyWrapper) Unwrap(context.Context, praxiscrypto.WrappedKey) ([]byte, error) {
	return append([]byte(nil), w.key...), nil
}

func sealedRecord(t *testing.T, now time.Time, plaintext string) SecureBlobRecord {
	t.Helper()
	wrapper := &testKeyWrapper{}
	service := praxiscrypto.EnvelopeService{Wrapper: wrapper}
	aad := SecureBlobAAD("goals", "goal-1", "1", "sha256:goal")
	env, err := service.Seal(context.Background(), "key:goal", contracts.CryptoPQRequired, []byte(plaintext), aad)
	if err != nil {
		t.Fatal(err)
	}
	return SecureBlobRecord{Namespace: "goals", ObjectID: "goal-1", ObjectVersion: "1", ObjectDigest: "sha256:goal", Sensitivity: SensitivityConfidential, CryptoProfile: contracts.CryptoPQRequired, Envelope: env, CreatedAt: now}
}

func TestSecureBlobRoundTripStoresNoPlaintextColumn(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Now().UTC()
	record := sealedRecord(t, now, "private research hypothesis")
	if err := store.PutSecureBlob(ctx, record); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSecureBlob(ctx, "goals", "goal-1", "1", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ObjectDigest != record.ObjectDigest || len(got.Envelope.Ciphertext) == 0 {
		t.Fatalf("unexpected stored record %+v", got)
	}
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT CAST(envelope_json AS TEXT) FROM secure_blobs WHERE namespace='goals' AND object_id='goal-1' AND object_version='1'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "private research hypothesis") {
		t.Fatal("plaintext leaked into persisted envelope row")
	}
}

func TestSecureBlobRejectsEnvelopeBoundToDifferentIdentity(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := sealedRecord(t, time.Now().UTC(), "x")
	r.ObjectID = "goal-2"
	if err := New(db).PutSecureBlob(ctx, r); err == nil {
		t.Fatal("record/envelope identity mismatch must fail")
	}
}

func TestSecureBlobIsImmutableByVersion(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := sealedRecord(t, time.Now().UTC(), "x")
	store := New(db)
	if err := store.PutSecureBlob(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSecureBlob(ctx, r); err == nil {
		t.Fatal("same baseline version cannot be overwritten")
	}
}

func TestSecureBlobExpiryFailsClosed(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	r := sealedRecord(t, now, "x")
	expiry := now.Add(time.Minute)
	r.ExpiresAt = &expiry
	store := New(db)
	if err := store.PutSecureBlob(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSecureBlob(ctx, "goals", "goal-1", "1", expiry); err != ErrSecureBlobExpired {
		t.Fatalf("expected expiry failure, got %v", err)
	}
}

func TestHistoricalSecureBlobReadPreservesOperationalExpiryGate(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	expiry := now.Add(time.Minute)
	r := sealedRecord(t, now, "historical")
	r.ExpiresAt = &expiry
	store := New(db)
	if err := store.PutSecureBlob(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSecureBlob(ctx, "goals", "goal-1", "1", expiry); err != ErrSecureBlobExpired {
		t.Fatalf("operational read must expire: %v", err)
	}
	historical, err := store.GetSecureBlobHistorical(ctx, "goals", "goal-1", "1")
	if err != nil || historical.ObjectDigest != r.ObjectDigest || historical.ExpiresAt == nil {
		t.Fatalf("historical immutable record unavailable: %+v %v", historical, err)
	}
}
