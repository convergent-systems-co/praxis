package publisher

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestSignRequiresEnrolledGenerationAndPackagePublish(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := state.New(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	generation := contracts.PublisherGeneration{Version: contracts.PublisherGenerationVersion, Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, KeyID: "publisher-key-1", Algorithm: "ed25519", PublicKey: public, PublicKeyDigest: digest(public), PackageNamespace: "praxis.package", Generation: "1", EffectiveAt: now, EnrollmentRef: "owner:enrollment:1", EnrollmentDigest: digest([]byte("enrollment"))}
	generationDigest, err := generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "owner", Kind: "human"}
	enrollment := contracts.ActionIntent{Version: "v1", ID: "enroll-1", Actor: actor, Operation: "publisher.enroll", Target: generationDigest, Scope: "package:praxis.package"}
	enrollmentDigest, err := enrollment.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,1,1)`, "enroll-approval", actor.ID, actor.Kind, enrollmentDigest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnrollPublisherGeneration(ctx, generation, enrollment, "enroll-approval", now); err != nil {
		t.Fatal(err)
	}
	intent := contracts.ActionIntent{Version: "v1", ID: "publish-1", Actor: generation.Principal, Operation: "package.publish", Target: "praxis.package.goals@1", Scope: "package:praxis.package.goals"}
	if _, err := intent.Digest(); err != nil {
		t.Fatal(err)
	}
	ops, _ := json.Marshal([]string{"sign"})
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,version) VALUES(?,?,?,?,?,?,?,1)`, "publish-lease", generation.Principal.ID, generation.Principal.Kind, contracts.PackagePublishCapability, ops, intent.Scope, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "praxis.package.goals", Version: "1", Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentDocumentation, ID: "readme", Version: "1", Digest: digest([]byte("content")), Artifact: "readme"}}}
	built, err := packagecatalog.BuildPackage(packagecatalog.PackageBuildInput{Manifest: manifest, Files: map[string][]byte{"readme": []byte("content")}})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := Sign(ctx, store, praxiscrypto.MemoryPublisherSigner{ID: generation.KeyID, Private: private}, generationDigest, built, "commit:test", "builder:test", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if signed.ProvenanceDigest == "" {
		t.Fatal("provenance digest is required")
	}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}
