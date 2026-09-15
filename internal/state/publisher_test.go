package state

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestPublisherEnrollmentPersistsOnlyWithExactApproval(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor := contracts.PrincipalRef{ID: "owner-1", Kind: "human"}
	generation := contracts.PublisherGeneration{Version: contracts.PublisherGenerationVersion, Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, KeyID: "key:praxis-first-party:1", Algorithm: "ed25519", PublicKeyDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111", PackageNamespace: "praxis.package", Generation: "1", EffectiveAt: now, EnrollmentRef: "owner-enrollment:1", EnrollmentDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222"}
	generationDigest, err := generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	intent := contracts.ActionIntent{Version: "v1", ID: "publisher-enroll-intent", Actor: actor, Operation: "publisher.enroll", Target: generationDigest, Scope: "package:" + generation.PackageNamespace}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,1,1)`, "approval-1", actor.ID, actor.Kind, intentDigest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	got, err := New(db).EnrollPublisherGeneration(ctx, generation, intent, "approval-1", now)
	if err != nil || got != generationDigest {
		t.Fatalf("enrollment failed: %v %s", err, got)
	}
	record, err := New(db).PublisherGeneration(ctx, generationDigest)
	if err != nil || record.State != "active" || record.Generation.Principal.ID != contracts.FirstPartyPublisherPrincipal {
		t.Fatalf("unexpected publisher record: %#v %v", record, err)
	}
	var stored []byte
	if err := db.QueryRowContext(ctx, `SELECT record_json FROM publisher_generations WHERE publisher_generation_digest=?`, generationDigest).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var decoded contracts.PublisherGeneration
	if err := json.Unmarshal(stored, &decoded); err != nil {
		t.Fatal(err)
	}
	storedDigest, err := decoded.Digest()
	if err != nil || storedDigest != generationDigest {
		t.Fatalf("stored publisher record changed: %v %s", err, storedDigest)
	}
}
