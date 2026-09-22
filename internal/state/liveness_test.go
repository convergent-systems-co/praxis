package state

import (
	"context"
	"errors"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// I12: the atomic multi-write is not a generic multi-record writer; every
// record after the first must be a liveness record.
func TestPutSecureBlobsAtomicallyAcceptsOnlyLivenessCompanions(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	record := SecureBlobRecord{Namespace: "work_plan_proposal", ObjectID: "a", ObjectVersion: "1"}
	companion := SecureBlobRecord{Namespace: "authority_decision", ObjectID: "b", ObjectVersion: "1"}
	if err := store.PutSecureBlobsAtomically(context.Background(), []SecureBlobRecord{record, companion}); !errors.Is(err, ErrNotLivenessNamespace) {
		t.Fatalf("a non-liveness companion record was accepted: %v", err)
	}
	if err := store.PutSecureBlobsAtomically(context.Background(), []SecureBlobRecord{{Namespace: AuthorityGenerationNamespace, ObjectID: "a", ObjectVersion: "1"}}); !errors.Is(err, ErrAuthorityGenerationNamespaceReserved) {
		t.Fatalf("the reserved generation namespace was writable: %v", err)
	}
}

// I12: liveness sealing and the in-transaction liveness probe refuse every
// namespace that is not a liveness namespace.
func TestLivenessSealingAndProbeRefuseOtherNamespaces(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	ctx := context.Background()
	service := praxiscrypto.EnvelopeService{Wrapper: &testKeyWrapper{}}
	if _, err := SealedLivenessRecord(ctx, service, "key:test", contracts.CryptoPQRequired, SensitivityConfidential, "authority_decision", "a", "1", "sha256:x", time.Now().UTC()); !errors.Is(err, ErrNotLivenessNamespace) {
		t.Fatalf("liveness sealing accepted a non-liveness namespace: %v", err)
	}
	tx, err := store.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := LivenessPresentInTx(ctx, tx, "authority_revocation", "a", "1"); !errors.Is(err, ErrNotLivenessNamespace) {
		t.Fatalf("the liveness probe accepted a non-liveness namespace: %v", err)
	}
}

// I12: the in-transaction fence of every authority-bound write refuses when the
// issuing generation has no liveness record, even though it has no invalidation
// record either. Once the generation is live the same write commits.
func TestUnlessRevokedWriteRequiresGenerationLiveness(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	service := praxiscrypto.EnvelopeService{Wrapper: &testKeyWrapper{}}
	lock := sealedRecordInNamespace(t, "authority_request", now)
	if err := store.PutSecureBlob(ctx, lock); err != nil {
		t.Fatal(err)
	}
	decisionLive, err := SealedLivenessRecord(ctx, service, "key:test", contracts.CryptoPQRequired, SensitivityConfidential, AuthorityDecisionLiveNamespace, "obj-1", "1", "sha256:x", now)
	if err != nil {
		t.Fatal(err)
	}
	record := sealedRecordInNamespace(t, "authority_decision", now)
	put := func() error {
		return store.PutSecureBlobUnlessRevoked(ctx, record, AuthorityRevocationNamespace, "obj-1", "1", AuthorityGenerationInvalidationNamespace, "gen", "1", "authority_request", lock.ObjectID, lock.ObjectVersion, decisionLive)
	}
	if err := put(); !errors.Is(err, ErrAuthorityRevoked) {
		t.Fatalf("a write under a generation without liveness was admitted: %v", err)
	}
	genLive, err := SealedLivenessRecord(ctx, service, "key:test", contracts.CryptoPQRequired, SensitivityConfidential, AuthorityGenerationLiveNamespace, "gen", "1", "sha256:y", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSecureBlob(ctx, genLive); err != nil {
		t.Fatal(err)
	}
	if err := put(); err != nil {
		t.Fatalf("control: a write under a live generation must commit: %v", err)
	}
}
