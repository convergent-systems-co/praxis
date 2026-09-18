package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func enrollPublisherGenerationFixture(t *testing.T, ctx context.Context, getenv func(string) string) string {
	t.Helper()
	db, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := state.New(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(public)
	generation := contracts.PublisherGeneration{Version: contracts.PublisherGenerationVersion, Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, KeyID: "publisher-key-retention", Algorithm: "ed25519", PublicKey: public, PublicKeyDigest: "sha256:" + hex.EncodeToString(sum[:]), PackageNamespace: "praxis.package", Generation: "1", EffectiveAt: now, EnrollmentRef: "owner:enrollment:1", EnrollmentDigest: "sha256:" + strings.Repeat("e", 64)}
	generationDigest, err := generation.Digest()
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "owner", Kind: "human"}
	enrollment := contracts.ActionIntent{Version: "v1", ID: "enroll-retention", Actor: actor, Operation: "publisher.enroll", Target: generationDigest, Scope: "package:praxis.package"}
	enrollmentDigest, err := enrollment.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,1,1)`, "enroll-approval-retention", actor.ID, actor.Kind, enrollmentDigest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnrollPublisherGeneration(ctx, generation, enrollment, "enroll-approval-retention", now); err != nil {
		t.Fatal(err)
	}
	return generationDigest
}

func publisherAuthorityPreview(t *testing.T, dir, name, generationDigest string) (contracts.PublisherAuthorityProposal, string, string, error) {
	t.Helper()
	previewFile := filepath.Join(dir, "publisher-authority-preview-"+name+".json")
	var out bytes.Buffer
	err := runPublisherAuthorityProposalPreview([]string{"--publisher-generation-digest", generationDigest, "--namespace", "praxis.package", "--expires-at", time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), "--output", previewFile}, &out)
	if err != nil {
		return contracts.PublisherAuthorityProposal{}, "", "", err
	}
	var preview struct {
		Proposal contracts.PublisherAuthorityProposal `json:"proposal"`
		Digest   string                               `json:"proposal_digest"`
	}
	if err := json.Unmarshal(out.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	return preview.Proposal, preview.Digest, previewFile, nil
}

// TestPackagePublishAdmissionFollowsRetainedV2Semantics proves package.publish
// proposal admission is decided by retained v2 semantics: v1 refuses, v2
// admits, and a v6 installation (retains v2 through v3) admits while the
// proposal binds the exact v2 identity rather than the active model.
func TestPackagePublishAdmissionFollowsRetainedV2Semantics(t *testing.T) {
	ctx := context.Background()
	getenv, root := governedInstallationFixture(t, ctx)
	generationDigest := enrollPublisherGenerationFixture(t, ctx, getenv)
	dir := t.TempDir()
	if _, _, _, err := publisherAuthorityPreview(t, dir, "v1", generationDigest); err == nil || !strings.Contains(err.Error(), "retains v2") {
		t.Fatalf("v1 installation must refuse package.publish preview, got %v", err)
	}
	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelSuccessorVersion)
	v2Proposal, _, _, err := publisherAuthorityPreview(t, dir, "v2", generationDigest)
	if err != nil {
		t.Fatalf("v2 installation must admit package.publish preview: %v", err)
	}
	if v2Proposal.AuthorityModelVersion != contracts.AuthorityModelSuccessorVersion || v2Proposal.AuthorityModelDigest != contracts.AuthorityModelSuccessorDigest() || v2Proposal.ParentDigest != root.Digest {
		t.Fatalf("v2 proposal identity: %#v", v2Proposal)
	}
	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelDeploymentVersion)
	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelRoutingVersion)
	v6Proposal, v6Digest, v6File, err := publisherAuthorityPreview(t, dir, "v6", generationDigest)
	if err != nil {
		t.Fatalf("v6 installation must admit package.publish preview (retains v2): %v", err)
	}
	if v6Proposal.AuthorityModelVersion != contracts.AuthorityModelSuccessorVersion || v6Proposal.AuthorityModelDigest != contracts.AuthorityModelSuccessorDigest() {
		t.Fatalf("proposal under v6 must bind the v2 publish identity: %s %s", v6Proposal.AuthorityModelVersion, v6Proposal.AuthorityModelDigest)
	}
	var proposalOut bytes.Buffer
	if err := runPublisherAuthorityProposal([]string{"--preview-file", v6File}, getenv, &proposalOut); err != nil {
		t.Fatalf("package.publish proposal under v6 must persist: %v", err)
	}
	if !strings.Contains(proposalOut.String(), v6Digest) {
		t.Fatalf("persisted proposal digest mismatch: %s", proposalOut.String())
	}
	repo, db, _, err := openGovernedRepositoryReadOnly(ctx, getenv)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	stored, err := repo.LoadPublisherAuthorityProposalByDigest(ctx, v6Digest, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublisherAuthorityProposalCurrent(ctx, repo, state.New(db), stored, now); err != nil {
		t.Fatalf("stored proposal must be current under v6: %v", err)
	}
	forged := stored
	forged.AuthorityModelVersion, forged.AuthorityModelDigest = contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest()
	if err := validatePublisherAuthorityProposalCurrent(ctx, repo, state.New(db), forged, now); err == nil {
		t.Fatal("v6-labelled package.publish proposal must be refused")
	}
}
