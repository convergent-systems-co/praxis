package goalstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	statepkg "github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func adoptionFixture(t *testing.T, repo Repository, now time.Time) (contracts.AuthorityModelAdoption, string) {
	t.Helper()
	bootstrapDigest := "sha256:" + strings.Repeat("a", 64)
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	root := authorityGenerationFixture(now)
	root.Principal = owner
	root.Scope, err = contracts.InstallationGovernanceScope(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	root.ProvenanceRef = "bootstrap-record:" + bootstrapDigest + ":os-user:test"
	root.ProvenanceDigest = bootstrapDigest
	root.Digest, err = root.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(context.Background(), root, now, nil); err != nil {
		t.Fatal(err)
	}
	adoption := contracts.AuthorityModelAdoption{
		ID: "authority-model-adoption:v1-to-v2", Version: "1",
		FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelVersion, FromDigest: contracts.AuthorityModelDigest(),
		ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelSuccessorVersion, ToDigest: contracts.AuthorityModelSuccessorDigest(),
		RootRef: root.Ref, RootVersion: root.Version, RootDigest: root.Digest,
		Reason: "adopt accepted built-in authority model successor", CreatedAt: now,
	}
	return adoption, bootstrapDigest
}

func TestAdoptAuthorityModelRequiresExactOwnerConfirmationAndDoesNotDelegate(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	adoption, bootstrapDigest := adoptionFixture(t, repo, now)
	digest, err := adoption.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), adoption, bootstrapDigest, "test", "ADOPT sha256:"+strings.Repeat("b", 64), now); err == nil {
		t.Fatal("mismatched owner confirmation must fail closed")
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), adoption, bootstrapDigest, "test", "ADOPT "+digest, now); err != nil {
		t.Fatal(err)
	}
	state, err := repo.LoadAuthorityModelState(context.Background(), now)
	if err != nil || state.ActiveVersion != contracts.AuthorityModelSuccessorVersion || state.AdoptionDigest != digest {
		t.Fatalf("v2 adoption was not recovered: state=%+v err=%v", state, err)
	}
	generations, err := repo.ListAuthorityGenerations(context.Background(), now)
	if err != nil || len(generations) != 1 || generations[0].AuthorityModelVersion != contracts.AuthorityModelVersion {
		t.Fatalf("adoption changed historical root or created downstream authority: generations=%+v err=%v", generations, err)
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), adoption, bootstrapDigest, "test", "ADOPT "+digest, now); err != nil {
		t.Fatalf("identical adoption should be idempotent: %v", err)
	}
}

func TestAdoptAuthorityModelRejectsSubstitutionAndDowngrade(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	adoption, bootstrapDigest := adoptionFixture(t, repo, now)
	digest, err := adoption.Digest()
	if err != nil {
		t.Fatal(err)
	}
	wrong := adoption
	wrong.ToDigest = contracts.AuthorityModelDigest()
	if _, err := repo.AdoptAuthorityModel(context.Background(), wrong, bootstrapDigest, "test", "ADOPT "+mustDigest(t, wrong), now); err == nil {
		t.Fatal("successor digest substitution must fail closed")
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), adoption, "sha256:"+strings.Repeat("c", 64), "test", "ADOPT "+digest, now); err == nil {
		t.Fatal("bootstrap substitution must fail closed")
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), adoption, bootstrapDigest, "other-user", "ADOPT "+digest, now); err == nil {
		t.Fatal("owner substitution must fail closed")
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), adoption, bootstrapDigest, "test", "ADOPT "+digest, now); err != nil {
		t.Fatal(err)
	}
	downgrade := adoption
	downgrade.ToVersion = contracts.AuthorityModelVersion
	downgrade.ToDigest = contracts.AuthorityModelDigest()
	if _, err := repo.AdoptAuthorityModel(context.Background(), downgrade, bootstrapDigest, "test", "ADOPT "+mustDigest(t, downgrade), now); err == nil {
		t.Fatal("downgrade attempt must fail closed")
	}
}

func TestAdoptAuthorityModelV2ToV3PreservesHistoryAndIsReplaySafe(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	v1ToV2, bootstrapDigest := adoptionFixture(t, repo, now)
	if _, err := repo.AdoptAuthorityModel(context.Background(), v1ToV2, bootstrapDigest, "test", "ADOPT "+mustDigest(t, v1ToV2), now); err != nil {
		t.Fatal(err)
	}
	root, err := repo.ListAuthorityGenerations(context.Background(), now)
	if err != nil || len(root) != 1 {
		t.Fatalf("root lineage unavailable: generations=%+v err=%v", root, err)
	}
	v2ToV3 := contracts.AuthorityModelAdoption{
		ID: "authority-model-adoption:v2-to-v3", Version: "1",
		FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelSuccessorVersion, FromDigest: contracts.AuthorityModelSuccessorDigest(),
		ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelDeploymentVersion, ToDigest: contracts.AuthorityModelDeploymentDigest(),
		RootRef: root[0].Ref, RootVersion: root[0].Version, RootDigest: root[0].Digest,
		Reason: "adopt accepted built-in package-deployment authority model", CreatedAt: now,
	}
	digest := mustDigest(t, v2ToV3)
	if _, err := repo.AdoptAuthorityModel(context.Background(), v2ToV3, bootstrapDigest, "test", "ADOPT "+digest, now); err != nil {
		t.Fatal(err)
	}
	active, err := repo.LoadAuthorityModelState(context.Background(), now)
	if err != nil || active.ActiveVersion != contracts.AuthorityModelDeploymentVersion || active.ActiveDigest != contracts.AuthorityModelDeploymentDigest() || active.AdoptionDigest != digest {
		t.Fatalf("v3 is not the active exact successor: state=%+v err=%v", active, err)
	}
	var historicalV2 contracts.AuthorityModelState
	if err := repo.loadPublisherGovernance(context.Background(), authorityModelStateID+":"+contracts.AuthorityModelSuccessorVersion, "1", now, &historicalV2); err != nil {
		t.Fatal(err)
	}
	if historicalV2.ActiveVersion != contracts.AuthorityModelSuccessorVersion {
		t.Fatalf("v2 historical secure record changed: %+v", historicalV2)
	}
	var journal contracts.AuthorityModelAdoption
	if err := repo.loadPublisherGovernance(context.Background(), v2ToV3.ID, v2ToV3.Version, now, &journal); err != nil {
		t.Fatal(err)
	}
	if got := mustDigest(t, journal); got != digest {
		t.Fatalf("v3 adoption journal digest=%s want=%s", got, digest)
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), v2ToV3, bootstrapDigest, "test", "ADOPT "+digest, now); err != nil {
		t.Fatalf("exact v3 replay must be idempotent: %v", err)
	}
	if _, err := store.GetSecureBlob(context.Background(), publisherGovernanceNamespace, authorityModelStateID+":"+contracts.AuthorityModelDeploymentVersion, "1", now); err != nil {
		t.Fatalf("v3 active secure record unavailable: %v", err)
	}
}

func TestAdoptAuthorityModelV2ToV3FailureRollsBackJournalAndPointer(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	v1ToV2, bootstrapDigest := adoptionFixture(t, repo, now)
	if _, err := repo.AdoptAuthorityModel(context.Background(), v1ToV2, bootstrapDigest, "test", "ADOPT "+mustDigest(t, v1ToV2), now); err != nil {
		t.Fatal(err)
	}
	root, err := repo.ListAuthorityGenerations(context.Background(), now)
	if err != nil || len(root) != 1 {
		t.Fatalf("root lineage unavailable: generations=%+v err=%v", root, err)
	}
	v2ToV3 := contracts.AuthorityModelAdoption{
		ID: "authority-model-adoption:v2-to-v3", Version: "1",
		FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelSuccessorVersion, FromDigest: contracts.AuthorityModelSuccessorDigest(),
		ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelDeploymentVersion, ToDigest: contracts.AuthorityModelDeploymentDigest(),
		RootRef: root[0].Ref, RootVersion: root[0].Version, RootDigest: root[0].Digest,
		Reason: "adopt accepted built-in package-deployment authority model", CreatedAt: now,
	}
	bad := contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: "v3", ActiveDigest: "sha256:" + strings.Repeat("0", 64), AdoptionDigest: "sha256:" + strings.Repeat("1", 64), State: "committed"}
	if _, err := repo.savePublisherGovernance(context.Background(), authorityModelStateID+":"+contracts.AuthorityModelDeploymentVersion, "1", bad, now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), v2ToV3, bootstrapDigest, "test", "ADOPT "+mustDigest(t, v2ToV3), now); err == nil {
		t.Fatal("conflicting successor record must fail")
	}
	active, err := repo.LoadAuthorityModelState(context.Background(), now)
	if err != nil || active.ActiveVersion != contracts.AuthorityModelSuccessorVersion {
		t.Fatalf("failed transition changed active model: state=%+v err=%v", active, err)
	}
	var journal contracts.AuthorityModelAdoption
	if err := repo.loadPublisherGovernance(context.Background(), v2ToV3.ID, v2ToV3.Version, now, &journal); !errors.Is(err, statepkg.ErrSecureBlobNotFound) {
		t.Fatalf("failed transition left adoption journal: %v", err)
	}
}

func TestHistoricalUndecidedV3AdoptionMustBeAbandonedBeforeNewAttempt(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	v1ToV2, bootstrapDigest := adoptionFixture(t, repo, now)
	if _, err := repo.AdoptAuthorityModel(context.Background(), v1ToV2, bootstrapDigest, "test", "ADOPT "+mustDigest(t, v1ToV2), now); err != nil {
		t.Fatal(err)
	}
	gens, err := repo.ListAuthorityGenerations(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	stale := contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v2-to-v3", Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelSuccessorVersion, FromDigest: contracts.AuthorityModelSuccessorDigest(), ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelDeploymentVersion, ToDigest: contracts.AuthorityModelDeploymentDigest(), RootRef: gens[0].Ref, RootVersion: gens[0].Version, RootDigest: gens[0].Digest, Reason: "historical interrupted attempt", CreatedAt: now}
	staleDigest := mustDigest(t, stale)
	if _, err := repo.SaveAuthorityModelAdoption(context.Background(), stale, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AdoptAuthorityModel(context.Background(), stale, bootstrapDigest, "test", "ADOPT "+staleDigest, now); err == nil {
		t.Fatal("undecided historical adoption must not activate")
	}
	if _, err := repo.AbandonAuthorityModelAdoption(context.Background(), staleDigest, bootstrapDigest, "test", "ABANDON "+staleDigest, now); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.AbandonAuthorityModelAdoption(context.Background(), staleDigest, bootstrapDigest, "test", "ABANDON "+staleDigest, now); err != nil || got == "" {
		t.Fatalf("exact supersession replay must be idempotent: digest=%q err=%v", got, err)
	}
	if got, err := repo.AbandonAuthorityModelAdoption(context.Background(), staleDigest, bootstrapDigest, "test", "ABANDON "+staleDigest, now.Add(time.Hour)); err != nil || got == "" {
		t.Fatalf("replay after time advanced must resolve original supersession: digest=%q err=%v", got, err)
	}
	newAttempt := stale
	newAttempt.ID = "authority-model-adoption:v2-to-v3:attempt-2"
	newDigest := mustDigest(t, newAttempt)
	if _, err := repo.AdoptAuthorityModel(context.Background(), newAttempt, bootstrapDigest, "test", "ADOPT "+newDigest, now); err != nil {
		t.Fatal(err)
	}
	state, err := repo.LoadAuthorityModelState(context.Background(), now)
	if err != nil || state.ActiveVersion != contracts.AuthorityModelDeploymentVersion || state.AdoptionDigest != newDigest {
		t.Fatalf("new attempt did not become active: state=%+v err=%v", state, err)
	}
	var supersession contracts.AuthorityModelAdoptionSupersession
	if err := repo.loadPublisherGovernance(context.Background(), "authority-model-adoption-supersession:"+staleDigest, "1", now, &supersession); err != nil {
		t.Fatal(err)
	}
}

func mustDigest(t *testing.T, adoption contracts.AuthorityModelAdoption) string {
	t.Helper()
	digest, err := adoption.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
