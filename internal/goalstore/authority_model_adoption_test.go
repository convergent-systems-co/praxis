package goalstore

import (
	"context"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
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

func mustDigest(t *testing.T, adoption contracts.AuthorityModelAdoption) string {
	t.Helper()
	digest, err := adoption.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
