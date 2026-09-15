package goalstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func importFixture(t *testing.T) (Repository, goals.GoalBaseline, []byte) {
	t.Helper()
	repo, _ := repoFixture(t, capabilitiesForTest(), profileForTest())
	b := goalFixture()
	b.ImportSourceRef = "/tmp/canonical-goal.json"
	canonical, err := b.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	// SourceDigest identifies the canonical payload, avoiding circular hashing
	// of the import envelope itself.
	sum := sha256.Sum256(canonical)
	b.ImportSourceDigest = "sha256:" + hex.EncodeToString(sum[:])
	digest, err := b.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	b.Digest = digest
	return repo, b, canonical
}

func capabilitiesForTest() praxiscrypto.Capabilities {
	return praxiscrypto.Capabilities{Classical: true, PQ: true}
}
func profileForTest() contracts.CryptoProfile { return contracts.CryptoClassicalCompatible }

func TestImportBaselineRejectsSourceBinding(t *testing.T) {
	repo, b, _ := importFixture(t)
	ctx := context.Background()
	if _, err := repo.ImportBaseline(ctx, ImportBaselineRequest{Baseline: b, SourceRef: b.ImportSourceRef, Source: []byte("wrong"), CreatedAt: time.Now().UTC()}); !errors.Is(err, ErrBaselineImportSource) {
		t.Fatalf("expected source binding rejection, got %v", err)
	}
}

func TestImportBaselineIsIdempotentAndReloadable(t *testing.T) {
	repo, b, source := importFixture(t)
	ctx := context.Background()
	got, err := repo.ImportBaseline(ctx, ImportBaselineRequest{Baseline: b, SourceRef: b.ImportSourceRef, Source: source, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := repo.ImportBaseline(ctx, ImportBaselineRequest{Baseline: b, SourceRef: b.ImportSourceRef, Source: source, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Digest != got.Digest {
		t.Fatalf("duplicate import changed generation: %s != %s", repeated.Digest, got.Digest)
	}
	loaded, err := repo.Load(ctx, b.ID, b.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != b.Digest || loaded.ImportSourceRef != b.ImportSourceRef {
		t.Fatal("imported baseline did not reload with provenance")
	}
}

func TestImportBaselineRequiresPredecessor(t *testing.T) {
	repo, b, _ := importFixture(t)
	b.Version = "2"
	b.PredecessorDigest = "sha256:missing"
	canonical, _ := b.CanonicalBytes()
	sum := sha256.Sum256(canonical)
	b.ImportSourceDigest = "sha256:" + hex.EncodeToString(sum[:])
	digest, _ := b.ComputeDigest()
	b.Digest = digest
	if _, err := repo.ImportBaseline(context.Background(), ImportBaselineRequest{Baseline: b, SourceRef: b.ImportSourceRef, Source: canonical, CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("import must reject a missing predecessor")
	}
}

func TestImportBaselineRejectsInvalidDigestAndConflictingGeneration(t *testing.T) {
	repo, b, source := importFixture(t)
	invalid := b
	invalid.Digest = "sha256:invalid"
	if _, err := repo.ImportBaseline(context.Background(), ImportBaselineRequest{Baseline: invalid, SourceRef: b.ImportSourceRef, Source: source, CreatedAt: time.Now().UTC()}); !errors.Is(err, goals.ErrBaselineDigestMismatch) {
		t.Fatalf("expected digest rejection, got %v", err)
	}
	if _, err := repo.ImportBaseline(context.Background(), ImportBaselineRequest{Baseline: b, SourceRef: b.ImportSourceRef, Source: source, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	conflict := b
	conflict.RefinedOutcome = "different authoritative content"
	canonical, _ := conflict.CanonicalBytes()
	sum := sha256.Sum256(canonical)
	conflict.ImportSourceDigest = "sha256:" + hex.EncodeToString(sum[:])
	conflict.Digest, _ = conflict.ComputeDigest()
	if _, err := repo.ImportBaseline(context.Background(), ImportBaselineRequest{Baseline: conflict, SourceRef: conflict.ImportSourceRef, Source: canonical, CreatedAt: time.Now().UTC()}); !errors.Is(err, ErrBaselineImportConflict) {
		t.Fatalf("expected same-generation conflict, got %v", err)
	}
}
