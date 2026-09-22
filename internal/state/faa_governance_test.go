package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func factRecord(t *testing.T, namespace string, seq uint64) SecureBlobRecord {
	t.Helper()
	service := praxiscrypto.EnvelopeService{Wrapper: &testKeyWrapper{}}
	record, err := SealedLivenessRecord(context.Background(), service, "key:test", contracts.CryptoPQRequired, SensitivityConfidential, AuthorityDecisionLiveNamespace, "x", "1", "sha256:x", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	// Re-seal under the namespace and identity under test.
	aad := SecureBlobAAD(namespace, GovernanceFactID(seq), "1", "sha256:obj")
	env, err := service.Seal(context.Background(), "key:test", contracts.CryptoPQRequired, []byte("payload"), aad)
	if err != nil {
		t.Fatal(err)
	}
	record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, record.Envelope = namespace, GovernanceFactID(seq), "1", "sha256:obj", env
	return record
}

// The fact append undoes its caller's anchor advance when the COMMIT fails, and
// only facts may be appended through it.
func TestAppendGovernanceFactUndoesOnCommitFailureAndAcceptsOnlyFacts(t *testing.T) {
	store := openAuthorityNamespaceTestDB(t)
	ctx := context.Background()

	undone := false
	err := store.AppendGovernanceFact(ctx, func(ctx context.Context, tx *sql.Tx, existing []SecureBlobRecord) (SecureBlobRecord, func(context.Context), error) {
		// End the transaction behind database/sql's back so its COMMIT fails.
		if _, err := tx.ExecContext(ctx, `COMMIT`); err != nil {
			t.Fatal(err)
		}
		return factRecord(t, GovernanceFactNamespace, 0), func(context.Context) { undone = true }, nil
	})
	if err == nil || !undone {
		t.Fatalf("a failed commit must undo the anchor advance (err=%v undone=%v)", err, undone)
	}

	before, _ := store.ListGovernanceFactRecords(ctx) // the raw COMMIT above autocommitted the row; that is an artefact of the injection
	undone = false
	err = store.AppendGovernanceFact(ctx, func(ctx context.Context, tx *sql.Tx, existing []SecureBlobRecord) (SecureBlobRecord, func(context.Context), error) {
		return factRecord(t, "authority_decision", 1), func(context.Context) { undone = true }, nil
	})
	if err == nil || !undone {
		t.Fatalf("a non-fact record must be refused and the advance undone (err=%v undone=%v)", err, undone)
	}
	if rows, _ := store.ListGovernanceFactRecords(ctx); len(rows) != len(before) {
		t.Fatalf("nothing may be appended: %d -> %d", len(before), len(rows))
	}
	// a nil record with a nil error means "already recorded"
	if err := store.AppendGovernanceFact(ctx, func(context.Context, *sql.Tx, []SecureBlobRecord) (SecureBlobRecord, func(context.Context), error) {
		return SecureBlobRecord{}, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	// a builder error is returned and nothing is written
	want := errors.New("refused")
	if err := store.AppendGovernanceFact(ctx, func(context.Context, *sql.Tx, []SecureBlobRecord) (SecureBlobRecord, func(context.Context), error) {
		return SecureBlobRecord{}, nil, want
	}); !errors.Is(err, want) {
		t.Fatalf("%v", err)
	}
}

// Two appenders on SEPARATE connections (separate processes, in production)
// cannot both extend the same chain head: the writer lock the append takes as its
// first statement serialises their builders.
func TestAppendGovernanceFactSerialisesAppendersOnSeparateConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	var stores []*Store
	for i := 0; i < 3; i++ {
		db, err := OpenSQLite(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		stores = append(stores, New(db))
	}
	var mu sync.Mutex
	inside, overlap := 0, 0
	errs := make(chan error, 3)
	for _, store := range stores {
		store := store
		go func() {
			errs <- store.AppendGovernanceFact(ctx, func(ctx context.Context, tx *sql.Tx, existing []SecureBlobRecord) (SecureBlobRecord, func(context.Context), error) {
				mu.Lock()
				inside++
				if inside > 1 {
					overlap++
				}
				mu.Unlock()
				time.Sleep(80 * time.Millisecond)
				rec := factRecord(t, GovernanceFactNamespace, uint64(len(existing)))
				mu.Lock()
				inside--
				mu.Unlock()
				return rec, nil, nil
			})
		}()
	}
	for range stores {
		<-errs
	}
	if overlap != 0 {
		t.Fatalf("appenders on separate connections ran their builders concurrently: %d overlaps", overlap)
	}
	if rows, _ := stores[0].ListGovernanceFactRecords(ctx); len(rows) != 3 {
		t.Fatalf("each serialised appender must extend the chain once: %d", len(rows))
	}
}
