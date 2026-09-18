package goalstore

import (
	"context"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func rootRecordBytes(t *testing.T, repo Repository, ref, version string) (string, []byte) {
	t.Helper()
	var digest string
	var envelope []byte
	if err := repo.Store.DB().QueryRow(`SELECT object_digest, envelope_json FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, state.AuthorityGenerationNamespace, ref, version).Scan(&digest, &envelope); err != nil {
		t.Fatal(err)
	}
	return digest, envelope
}

func countRows(t *testing.T, repo Repository, namespace string) int {
	t.Helper()
	var n int
	if err := repo.Store.DB().QueryRow(`SELECT COUNT(*) FROM secure_blobs WHERE namespace=?`, namespace).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestHistoricalRootModernizationEstablishesSingleCurrentRoot(t *testing.T) {
	ctx := context.Background()
	repo, store := repoFixture(t, praxiscrypto.Capabilities{Classical: true}, contracts.CryptoClassicalCompatible)
	bootstrap := "sha256:" + strings.Repeat("d", 64)
	repo.InstallationDigest = bootstrap
	now := time.Unix(1_800_000_000, 0).UTC()
	r0Digest := persistLegacyEnrollmentRoot(t, repo, bootstrap, "goal:dogfood/baseline/1/proposal/wp-proposal-v1", "fixture", now.Add(-time.Hour))
	r0ObjectDigest, r0Bytes := rootRecordBytes(t, repo, "installation-governance:"+bootstrap, "1")

	// Truthful pre-succession state: no current root, one historical root.
	if _, err := repo.LoadCurrentInstallationRoot(ctx, bootstrap, now); err == nil || !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("historical root must not be classified as current: %v", err)
	}
	r0, err := repo.LoadHistoricalInstallationRoot(ctx, bootstrap, now)
	if err != nil || r0.Digest != r0Digest || !r0.PreDelegationForm() {
		t.Fatalf("historical root discovery failed: %+v %v", r0, err)
	}
	if _, err := contracts.BuildRootAuthoritySuccession(r0, bootstrap, now); err == nil {
		t.Fatal("ADR-089 repair succession must not consume the historical root")
	}

	// Failed succession leaves R0 and the installation unchanged.
	proposal, err := contracts.BuildHistoricalRootModernization(r0, bootstrap, now)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := repo.SaveRootAuthoritySuccessionProposal(ctx, proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, r0.Principal, "someone-else", "REVIEW-ROOT-SUCCESSOR "+proposalDigest, now); err == nil {
		t.Fatal("review must authenticate the enrolling OS user")
	}
	review, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, r0.Principal, "fixture", "REVIEW-ROOT-SUCCESSOR "+proposalDigest, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, again, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, r0.Principal, "fixture", "REVIEW-ROOT-SUCCESSOR "+proposalDigest, now.Add(time.Second)); err != nil || again != reviewDigest {
		t.Fatalf("review replay before acceptance must be idempotent: %v", err)
	}
	forged := contracts.RootAuthoritySuccessionDecision{ID: "root-authority-succession-decision:" + proposalDigest, Version: "1", Kind: contracts.RootAuthoritySuccessionDecisionKind, BootstrapDigest: bootstrap, ProposalID: proposal.ID, ProposalVersion: proposal.Version, ProposalDigest: proposalDigest, ReviewID: review.ID, ReviewVersion: review.Version, ReviewDigest: "sha256:" + strings.Repeat("0", 64), PredecessorRef: r0.Ref, PredecessorVersion: r0.Version, PredecessorDigest: r0.Digest, SuccessorRef: proposal.Successor.Ref, SuccessorVersion: proposal.Successor.Version, SuccessorDigest: proposal.Successor.Digest, DecidedBy: r0.Principal, Decision: "approve", Confirmation: "ACCEPT-ROOT-SUCCESSOR " + proposalDigest + " " + "sha256:" + strings.Repeat("0", 64), DecidedAt: now.Add(2 * time.Second)}
	if err := store.PutRootAuthoritySuccessor(ctx, state.RootAuthoritySuccessionWrite{Proposal: proposal, Review: review, Decision: forged, Crypto: repo.Crypto, KeyRef: repo.KeyRef, Profile: repo.Profile, Sensitivity: repo.Sensitivity, CreatedAt: now.Add(2 * time.Second)}); err == nil {
		t.Fatal("state transition accepted a decision that does not bind the review")
	}
	if countRows(t, repo, state.AuthorityGenerationNamespace) != 1 || countRows(t, repo, state.AuthorityGenerationInvalidationNamespace) != 0 {
		t.Fatal("failed succession mutated authority state")
	}
	if again, err := repo.LoadHistoricalInstallationRoot(ctx, bootstrap, now.Add(3*time.Second)); err != nil || again.Digest != r0Digest {
		t.Fatalf("historical root must remain discoverable after a failed succession: %v", err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, proposal.Successor, now, nil); err == nil {
		t.Fatal("generic writer must not mint a root successor outside the transition")
	}
	if _, err := repo.LoadCurrentInstallationRoot(ctx, bootstrap, now.Add(3*time.Second)); err == nil {
		t.Fatal("current authority must not exist before governed acceptance")
	}

	// Governed acceptance establishes exactly one current root.
	r1, decision, err := repo.AcceptRootAuthoritySuccession(ctx, proposalDigest, reviewDigest, bootstrap, "fixture", "ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewDigest, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(5 * time.Second)
	current, err := repo.LoadCurrentInstallationRoot(ctx, bootstrap, later)
	if err != nil || current.Digest != r1.Digest || current.PredecessorDigest != r0Digest || current.Version != "2" || current.PreDelegationForm() {
		t.Fatalf("current root is not the modernized successor: %+v %v", current, err)
	}
	if decision.PredecessorDigest != r0Digest || decision.SuccessorDigest != r1.Digest || decision.ProposalDigest != proposalDigest {
		t.Fatalf("decision does not bind R0 -> R1: %+v", decision)
	}
	generations, err := repo.ListAuthorityGenerations(ctx, later)
	if err != nil || len(generations) != 2 {
		t.Fatalf("expected R0 and R1 retained: %d %v", len(generations), err)
	}
	afterDigest, afterBytes := rootRecordBytes(t, repo, r0.Ref, r0.Version)
	if afterDigest != r0ObjectDigest || string(afterBytes) != string(r0Bytes) {
		t.Fatal("historical root record was rewritten")
	}
	stored, err := repo.LoadAuthorityGeneration(ctx, r0.Ref, r0.Version, later)
	if err != nil || stored.Digest != r0Digest || !stored.PreDelegationForm() {
		t.Fatalf("historical root must remain verifiable evidence: %v", err)
	}
	if err := repo.ValidateAuthorityGeneration(ctx, contracts.AuthorityDecision{AuthorityRef: r0.Ref, AuthorityVersion: r0.Version, AuthorityGenerationDigest: r0.Digest, DecidedBy: r0.Principal, GrantedScope: r0.Scope}, later); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("superseded historical root must not authorize decisions: %v", err)
	}
	if err := repo.ValidateAuthorityGeneration(ctx, contracts.AuthorityDecision{AuthorityRef: r1.Ref, AuthorityVersion: r1.Version, AuthorityGenerationDigest: r1.Digest, DecidedBy: r1.Principal, GrantedScope: r1.Scope}, later); err != nil {
		t.Fatalf("current root must authorize decisions: %v", err)
	}
	if _, err := repo.LoadHistoricalInstallationRoot(ctx, bootstrap, later); err == nil {
		t.Fatal("historical root discovery must fail closed once current authority exists")
	}

	// Replay and double succession fail closed without a second active root.
	replayed, replayDecision, err := repo.AcceptRootAuthoritySuccession(ctx, proposalDigest, reviewDigest, bootstrap, "fixture", "ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewDigest, later)
	if err != nil || replayed.Digest != r1.Digest || replayDecision.DecidedAt != decision.DecidedAt {
		t.Fatalf("exact replay must return the committed result: %v", err)
	}
	if err := store.PutRootAuthoritySuccessor(ctx, state.RootAuthoritySuccessionWrite{Proposal: proposal, Review: review, Decision: decision, Crypto: repo.Crypto, KeyRef: repo.KeyRef, Profile: repo.Profile, Sensitivity: repo.Sensitivity, CreatedAt: later}); err == nil {
		t.Fatal("re-running the state transition must fail closed")
	}
	if _, _, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, r0.Principal, "fixture", "REVIEW-ROOT-SUCCESSOR "+proposalDigest, later); err == nil {
		t.Fatal("review of a superseded predecessor must fail closed")
	}
	if countRows(t, repo, state.AuthorityGenerationNamespace) != 2 || countRows(t, repo, state.AuthorityGenerationInvalidationNamespace) != 1 {
		t.Fatal("replay created additional authority records")
	}
	if current, err := repo.LoadCurrentInstallationRoot(ctx, bootstrap, later); err != nil || current.Digest != r1.Digest {
		t.Fatalf("exactly one current root expected: %v", err)
	}

	// ADR-089 repair succession continues normally from R1.
	repo.InstallationDigest = bootstrap
	repairProposal, err := contracts.BuildRootAuthoritySuccession(current, bootstrap, later)
	if err != nil {
		t.Fatal(err)
	}
	repairDigest, err := repo.SaveRootAuthoritySuccessionProposal(ctx, repairProposal, later)
	if err != nil {
		t.Fatal(err)
	}
	_, repairReview, err := repo.SaveRootAuthoritySuccessionReview(ctx, repairDigest, current.Principal, "fixture", "REVIEW-ROOT-SUCCESSOR "+repairDigest, later.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	r2, _, err := repo.AcceptRootAuthoritySuccession(ctx, repairDigest, repairReview, bootstrap, "fixture", "ACCEPT-ROOT-SUCCESSOR "+repairDigest+" "+repairReview, later.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	final, err := repo.LoadCurrentInstallationRoot(ctx, bootstrap, later.Add(3*time.Second))
	if err != nil || final.Digest != r2.Digest || final.PredecessorDigest != r1.Digest || !strings.Contains(strings.Join(final.Authorities, ","), contracts.GovernedInstallationRepairStorageSchema) {
		t.Fatalf("ADR-089 succession did not continue from the modernized root: %+v %v", final, err)
	}
}
