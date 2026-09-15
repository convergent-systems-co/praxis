package goalstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func signingPreviewFixture(now time.Time) contracts.SigningPreview {
	return contracts.SigningPreview{
		ID: "publisher-signing-preview:test", Version: "1",
		PackageID: "praxis.package.goals", PackageVersion: "0.1.0", PackageNamespace: "praxis.package",
		ManifestDigest: hexDigest('1'), ArtifactDigest: hexDigest('2'), ExecutableDigest: hexDigest('3'),
		InvocationContractDigest: hexDigest('4'), ExecutableBindingDigest: hexDigest('5'), PluginDefinitionDigest: hexDigest('6'),
		RuntimeID: "praxis.plugin.grpc", RuntimeVersion: "1", RuntimeDigest: hexDigest('7'),
		SourceIdentity: "commit:test/tree:test", BuilderIdentity: "builder:test",
		PublisherPrincipal: contracts.FirstPartyPublisherPrincipal, PublisherGenerationDigest: hexDigest('8'), PublisherGenerationVersion: "1",
		PublicKeyDigest: hexDigest('9'), KeyID: "key:test", Algorithm: "ed25519",
		AuthorityGenerationRef: "authority:test", AuthorityGenerationVersion: "1", AuthorityGenerationDigest: hexDigest('a'),
		AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelSuccessorVersion, AuthorityModelDigest: contracts.AuthorityModelSuccessorDigest(),
		ParentRef: "installation:test", ParentVersion: "1", ParentDigest: hexDigest('b'), AuthorityScope: "package-namespace:praxis.package",
		AuthorityRequestID: "request:test", AuthorityRequestVersion: "1", AuthorityRequestDigest: hexDigest('c'),
		AuthorityDecisionRef: "decision:test", AuthorityDecisionVersion: "1", AuthorityDecisionDigest: hexDigest('d'),
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
}

func hexDigest(c byte) string { return "sha256:" + strings.Repeat(string(c), 64) }

func signingPreviewRepo(t *testing.T, path string) (Repository, *sql.DB) {
	t.Helper()
	db, err := state.OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	store := state.New(db)
	repo := Repository{
		Store:  store,
		Crypto: praxiscrypto.EnvelopeService{Wrapper: &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}},
		KeyRef: "key:test", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential,
	}
	return repo, db
}

func TestSigningPreviewResolvesSemanticDigestAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	now := time.Unix(1700000000, 0).UTC()
	repo, db := signingPreviewRepo(t, path)
	preview := signingPreviewFixture(now)
	digest, err := repo.SaveSigningPreview(context.Background(), preview, now)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	record, err := repo.Store.GetSecureBlob(context.Background(), publisherGovernanceNamespace, preview.ID, preview.Version, now)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if record.ObjectDigest == digest {
		db.Close()
		t.Fatalf("storage integrity digest must remain distinct from semantic preview digest: %s", digest)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, db := signingPreviewRepo(t, path)
	t.Cleanup(func() { db.Close() })
	loaded, err := reopened.LoadSigningPreviewByDigest(context.Background(), digest, now)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != digest || loaded.ID != preview.ID {
		t.Fatalf("reopened preview mismatch: got digest %s id %s", loaded.Digest, loaded.ID)
	}
	if _, err := reopened.LoadSigningPreviewByDigest(context.Background(), hexDigest('f'), now); err == nil {
		t.Fatal("unknown semantic preview digest must fail closed")
	}
}

func TestSigningPreviewRejectsStorageDigestSubstitution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	now := time.Unix(1700000000, 0).UTC()
	repo, db := signingPreviewRepo(t, path)
	t.Cleanup(func() { db.Close() })
	preview := signingPreviewFixture(now)
	digest, err := repo.SaveSigningPreview(context.Background(), preview, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `UPDATE secure_blobs SET object_digest=? WHERE namespace=? AND object_id=? AND object_version=?`, digest, publisherGovernanceNamespace, preview.ID, preview.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadSigningPreviewByDigest(context.Background(), digest, now); err == nil {
		t.Fatal("storage identity substitution must fail closed")
	}
}
