package goalstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// legacyEnrollmentGeneration is the exact AuthorityGeneration wire form of
// commit 5850f27, the `praxis authority bootstrap --scope <least-scope>`
// boundary that enrolled installation roots at storage schema 11 before
// DelegatedBy, Capabilities, and the installation-governance scope existed.
type legacyEnrollmentGeneration struct {
	Ref              string                             `json:"ref"`
	Version          string                             `json:"version"`
	Digest           string                             `json:"digest"`
	Principal        contracts.PrincipalRef             `json:"principal"`
	Scope            string                             `json:"scope"`
	ProvenanceRef    string                             `json:"provenance_ref"`
	ProvenanceDigest string                             `json:"provenance_digest"`
	State            contracts.AuthorityGenerationState `json:"state"`
	EffectiveAt      time.Time                          `json:"effective_at"`
}

// persistLegacyEnrollmentRoot reproduces the historical persistence path of
// commit 5850f27 (Repository.SaveAuthorityGeneration -> putWorkPlanBlob ->
// Store.PutSecureBlob): the nine-field payload, its digest over that exact
// form, the secure-blob AAD, and the raw secure_blobs row. The reserved
// namespace is deliberately bypassed only here, exactly as the historical
// writer preceded that boundary.
func persistLegacyEnrollmentRoot(t *testing.T, repo Repository, bootstrapDigest, scope, osUser string, now time.Time) string {
	t.Helper()
	ctx := context.Background()
	legacy := legacyEnrollmentGeneration{Ref: "installation-governance:" + bootstrapDigest, Version: "1", Principal: contracts.PrincipalRef{ID: "installation-owner:" + bootstrapDigest, Kind: "human"}, Scope: scope, ProvenanceRef: "bootstrap-record:" + bootstrapDigest + ":os-user:" + osUser, ProvenanceDigest: bootstrapDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: now.UTC()}
	unsigned, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(unsigned)
	legacy.Digest = fmt.Sprintf("sha256:%x", sum[:])
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	objectDigest := payloadDigest(payload)
	envelope, err := repo.Crypto.Seal(ctx, repo.KeyRef, repo.Profile, payload, state.SecureBlobAAD(state.AuthorityGenerationNamespace, legacy.Ref, legacy.Version, objectDigest))
	if err != nil {
		t.Fatal(err)
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Store.DB().ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,NULL)`, state.AuthorityGenerationNamespace, legacy.Ref, legacy.Version, objectDigest, string(repo.Sensitivity), string(repo.Profile), envelopeJSON, now.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return legacy.Digest
}

func TestInstallationRootForSchemaAdmitsLegacyEnrollmentOnlyAtSchema11(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{Classical: true}, contracts.CryptoClassicalCompatible)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	bootstrapDigest := "sha256:" + strings.Repeat("b", 64)
	const leastScope = "goal:dogfood/baseline/1/proposal/wp-proposal-v1"
	legacyDigest := persistLegacyEnrollmentRoot(t, repo, bootstrapDigest, leastScope, "fixture", now)

	generations, err := repo.ListAuthorityGenerations(ctx, now)
	if err != nil || len(generations) != 1 || generations[0].Digest != legacyDigest || !generations[0].PreDelegationForm() {
		t.Fatalf("legacy root must list and verify against its persisted bytes: %+v %v", generations, err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generations[0], now, nil); err == nil {
		t.Fatal("a pre-delegation value is read-only evidence and must not be persisted as a new generation")
	}

	root, err := repo.LoadInstallationRootForSchema(ctx, bootstrapDigest, 11, now)
	if err != nil {
		t.Fatalf("schema-11 source must authorize from its legacy enrollment root: %v", err)
	}
	if root.Digest != legacyDigest || root.Scope != leastScope || root.Ref != "installation-governance:"+bootstrapDigest {
		t.Fatalf("unexpected root: %+v", root)
	}
	for _, schema := range []int{12, 15, 18} {
		if _, err := repo.LoadInstallationRootForSchema(ctx, bootstrapDigest, schema, now); err == nil {
			t.Fatalf("schema %d must not admit a least-scope legacy root", schema)
		}
	}
	if _, err := repo.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now); err == nil {
		t.Fatal("current root semantics must not admit a least-scope legacy root")
	}
	if _, err := repo.LoadInstallationRootForSchema(ctx, bootstrapDigest, 0, now); err == nil {
		t.Fatal("source schema is required")
	}
}

func TestInstallationRootForSchema11AdmitsCurrentRootButNotCurrentFormWithLeastScope(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	current, _ := repoFixture(t, praxiscrypto.Capabilities{Classical: true}, contracts.CryptoClassicalCompatible)
	bootstrapDigest := "sha256:" + strings.Repeat("c", 64)
	owner, _ := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	scope, _ := contracts.InstallationGovernanceScope(bootstrapDigest)
	root := contracts.AuthorityGeneration{Ref: scope, Version: "1", Principal: owner, Scope: scope, Capabilities: []string{contracts.AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:" + bootstrapDigest + ":os-user:fixture", ProvenanceDigest: bootstrapDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: now}
	var err error
	if root.Digest, err = root.ComputeDigest(); err != nil {
		t.Fatal(err)
	}
	if err := current.SaveAuthorityGeneration(ctx, root, now, nil); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []int{11, 12, 18} {
		loaded, err := current.LoadInstallationRootForSchema(ctx, bootstrapDigest, schema, now)
		if err != nil || loaded.Digest != root.Digest {
			t.Fatalf("schema %d must admit the current-form governance root: %v", schema, err)
		}
	}

	// A current-form root with a non-governance scope was never produced by
	// any writer; schema 11 must not admit it either.
	mixed, _ := repoFixture(t, praxiscrypto.Capabilities{Classical: true}, contracts.CryptoClassicalCompatible)
	other := root
	other.Scope = "goal:x/baseline/1/proposal/p"
	if other.Digest, err = other.ComputeDigest(); err != nil {
		t.Fatal(err)
	}
	if err := mixed.SaveAuthorityGeneration(ctx, other, now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mixed.LoadInstallationRootForSchema(ctx, bootstrapDigest, 11, now); err == nil {
		t.Fatal("current-form root with a least scope must not be admitted at schema 11")
	}
}
