package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/migrations/sqlite"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// jsonAfterPrompt drops the interactive confirmation prompt that precedes a
// command's JSON result.
func jsonAfterPrompt(out []byte) []byte {
	if i := bytes.IndexByte(out, '{'); i >= 0 {
		return out[i:]
	}
	return out
}

func schema11RootRecord(t *testing.T, ctx context.Context, dbPath, ref string) (string, []byte) {
	t.Helper()
	db, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var digest string
	var envelope []byte
	if err := db.QueryRowContext(ctx, `SELECT object_digest, envelope_json FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version='1'`, state.AuthorityGenerationNamespace, ref).Scan(&digest, &envelope); err != nil {
		t.Fatal(err)
	}
	return digest, envelope
}

func countSecureBlobs(t *testing.T, ctx context.Context, dbPath, namespace string) int {
	t.Helper()
	db, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=?`, namespace).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestSchema11HistoricalRootReachesCurrentAuthorityLifecycle qualifies the
// complete governed lifecycle of a real schema-11 installation: migration
// preserves the historical enrollment root R0 as unchanged evidence, the
// system truthfully reports that current authority is not yet established,
// ADR-090 modernization succession establishes the single current root R1
// with exact provenance to R0, current-schema consumers resolve R1, the state
// reconstructs identically after reopening, and replay or double succession
// fails closed without a second active root.
func TestSchema11HistoricalRootReachesCurrentAuthorityLifecycle(t *testing.T) {
	ctx := context.Background()
	fixture := schema11InstallationFixture(t, ctx, 11)
	dir := filepath.Dir(fixture.dbPath)
	rootRef := "installation-governance:" + fixture.bootstrapDigest
	r0ObjectDigest, r0Bytes := schema11RootRecord(t, ctx, fixture.dbPath, rootRef)

	// 1-3: schema 11 with R0, preview, full governed migration to 19.
	previewPath := filepath.Join(dir, "migration-preview.json")
	var previewOut bytes.Buffer
	if err := runMigrationPreview([]string{"--output", previewPath}, fixture.getenv, &previewOut); err != nil {
		t.Fatal(err)
	}
	var migration struct {
		Plan sqlite.Plan `json:"plan"`
	}
	if err := json.Unmarshal(previewOut.Bytes(), &migration); err != nil {
		t.Fatal(err)
	}
	if err := runMigrationExecuteWithTerminal([]string{"--preview-file", previewPath}, fixture.getenv, strings.NewReader("MIGRATE "+migration.Plan.PlanDigest+"\n"), &bytes.Buffer{}, true); err != nil {
		t.Fatal(err)
	}
	{
		db, err := state.OpenSQLiteReadOnly(ctx, fixture.dbPath)
		if err != nil {
			t.Fatal(err)
		}
		status, err := sqlite.StatusOf(ctx, db)
		db.Close()
		if err != nil || status.CurrentSchema != 19 || len(status.Pending) != 0 {
			t.Fatalf("migration incomplete: %+v %v", status, err)
		}
	}

	// 4: R0 keeps its original digest, bytes, and identity.
	if digest, body := schema11RootRecord(t, ctx, fixture.dbPath, rootRef); digest != r0ObjectDigest || !bytes.Equal(body, r0Bytes) {
		t.Fatal("migration rewrote the historical root record")
	}

	// 5: current authority is truthfully reported as not established.
	repo, readDB, _, err := openGovernedRepositoryReadOnly(ctx, fixture.getenv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadCurrentInstallationRoot(ctx, fixture.bootstrapDigest, time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("historical root must not be classified as current: %v", err)
	}
	r0, err := repo.LoadHistoricalInstallationRoot(ctx, fixture.bootstrapDigest, time.Now().UTC())
	if err != nil || r0.Digest != fixture.rootDigest || !r0.PreDelegationForm() {
		t.Fatalf("historical root must be discoverable as evidence: %v", err)
	}
	topology, err := inspectAuthorityTopologyWithRepository(ctx, repo, fixture.bootstrapDigest)
	readDB.Close()
	if err == nil || topology.Status != "historical-root-pending-succession" || topology.Root["generation_digest"] != fixture.rootDigest || topology.Root["required_ceremony"] == "" {
		t.Fatalf("doctor must report pending historical-root succession: %+v %v", topology, err)
	}
	if err := runRootAuthoritySuccessionPreview(nil, fixture.getenv, &bytes.Buffer{}); err == nil {
		t.Fatal("ADR-089 preview must not derive a repair successor from a historical root")
	}

	// 6: explicit governed R0 -> R1 modernization succession.
	successionPreview := filepath.Join(dir, "root-successor-preview.json")
	var successionOut bytes.Buffer
	if err := runRootAuthoritySuccessionPreview([]string{"--from-historical-root", "--output", successionPreview}, fixture.getenv, &successionOut); err != nil {
		t.Fatal(err)
	}
	proposal, proposalDigest, err := loadRootSuccessionPreview(successionPreview)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Kind != contracts.HistoricalRootModernizationProposalKind || proposal.HistoricalSchema != 11 || proposal.Predecessor.Digest != fixture.rootDigest || !proposal.Predecessor.PreDelegationForm() {
		t.Fatalf("unexpected modernization proposal: %+v", proposal)
	}
	if err := runRootAuthoritySuccessionProposal([]string{"--preview-file", successionPreview}, fixture.getenv, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := runRootAuthoritySuccessionReviewWithTerminal([]string{"--proposal", proposalDigest}, fixture.getenv, strings.NewReader("REVIEW-ROOT-SUCCESSOR "+proposalDigest+"\n"), &bytes.Buffer{}, false); err == nil {
		t.Fatal("non-interactive review must be refused")
	}
	var reviewOut bytes.Buffer
	if err := runRootAuthoritySuccessionReviewWithTerminal([]string{"--proposal", proposalDigest}, fixture.getenv, strings.NewReader("REVIEW-ROOT-SUCCESSOR "+proposalDigest+"\n"), &reviewOut, true); err != nil {
		t.Fatal(err)
	}
	var reviewed struct {
		ReviewDigest string `json:"review_digest"`
	}
	if err := json.Unmarshal(jsonAfterPrompt(reviewOut.Bytes()), &reviewed); err != nil || reviewed.ReviewDigest == "" {
		t.Fatalf("review digest missing: %v %s", err, reviewOut.String())
	}
	if err := runRootAuthoritySuccessionAcceptWithTerminal([]string{"--proposal", proposalDigest, "--review", reviewed.ReviewDigest}, fixture.getenv, strings.NewReader("wrong\n"), &bytes.Buffer{}, true); err == nil {
		t.Fatal("acceptance without exact confirmation must be refused")
	}
	if _, err := repo.LoadCurrentInstallationRoot(ctx, fixture.bootstrapDigest, time.Now().UTC()); err == nil {
		t.Fatal("refused acceptance must not establish current authority")
	}
	var acceptOut bytes.Buffer
	if err := runRootAuthoritySuccessionAcceptWithTerminal([]string{"--proposal", proposalDigest, "--review", reviewed.ReviewDigest}, fixture.getenv, strings.NewReader("ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewed.ReviewDigest+"\n"), &acceptOut, true); err != nil {
		t.Fatalf("modernization acceptance failed: %v\n%s", err, acceptOut.String())
	}
	var accepted struct {
		Successor contracts.AuthorityGeneration `json:"successor"`
	}
	if err := json.Unmarshal(jsonAfterPrompt(acceptOut.Bytes()), &accepted); err != nil {
		t.Fatal(err)
	}

	// 7-8: exactly one current root, R1 in current semantics with exact R0 provenance.
	repo, readDB, _, err = openGovernedRepositoryReadOnly(ctx, fixture.getenv)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := repo.LoadCurrentInstallationRoot(ctx, fixture.bootstrapDigest, time.Now().UTC())
	if err != nil {
		t.Fatalf("current root not established: %v", err)
	}
	governanceScope, _ := contracts.InstallationGovernanceScope(fixture.bootstrapDigest)
	owner, _ := contracts.InstallationOwnerPrincipal(fixture.bootstrapDigest)
	if r1.Digest != accepted.Successor.Digest || r1.Digest == fixture.rootDigest || r1.Version != "2" || r1.Scope != governanceScope || r1.Ref != governanceScope || r1.Principal != owner || r1.PreDelegationForm() {
		t.Fatalf("R1 is not the current canonical root: %+v", r1)
	}
	if r1.PredecessorRef != rootRef || r1.PredecessorVersion != "1" || r1.PredecessorDigest != fixture.rootDigest || r1.ProvenanceRef != r0.ProvenanceRef || !strings.HasSuffix(r1.ProvenanceRef, ":os-user:"+fixture.osUser) {
		t.Fatalf("R1 lacks exact provenance to R0: %+v", r1)
	}
	if r1.AuthorityModel != contracts.AuthorityModelID || r1.AuthorityModelVersion != contracts.AuthorityModelVersion || !authorityContainsCapability(r1.Capabilities, contracts.AuthorityDelegateCapability) || len(r1.Authorities) != 0 || r1.ParentRef != "" || r1.DelegatedBy != (contracts.PrincipalRef{}) {
		t.Fatalf("R1 lacks current delegation semantics: %+v", r1)
	}
	generations, err := repo.ListAuthorityGenerations(ctx, time.Now().UTC())
	if err != nil || len(generations) != 2 {
		t.Fatalf("expected R0 and R1 retained: %d %v", len(generations), err)
	}

	// 9: R0 remains unmodified historical evidence, superseded not rewritten.
	if digest, body := schema11RootRecord(t, ctx, fixture.dbPath, rootRef); digest != r0ObjectDigest || !bytes.Equal(body, r0Bytes) {
		t.Fatal("succession rewrote the historical root record")
	}
	stored, err := repo.LoadAuthorityGeneration(ctx, rootRef, "1", time.Now().UTC())
	if err != nil || stored.Digest != fixture.rootDigest || !stored.PreDelegationForm() {
		t.Fatalf("R0 must remain verifiable: %v", err)
	}
	if err := repo.ValidateAuthorityGeneration(ctx, contracts.AuthorityDecision{AuthorityRef: rootRef, AuthorityVersion: "1", AuthorityGenerationDigest: fixture.rootDigest, DecidedBy: owner, GrantedScope: r0.Scope}, time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("R0 must be superseded, not current: %v", err)
	}
	if countSecureBlobs(t, ctx, fixture.dbPath, state.AuthorityGenerationInvalidationNamespace) != 1 {
		t.Fatal("expected exactly one supersession record")
	}

	// 10: current-schema consumers resolve R1.
	topology, err = inspectAuthorityTopologyWithRepository(ctx, repo, fixture.bootstrapDigest)
	if err != nil || topology.Status != "qualified" || topology.Root["generation_digest"] != r1.Digest || topology.Root["predecessor_digest"] != fixture.rootDigest {
		t.Fatalf("doctor must qualify the modernized root: %+v %v", topology, err)
	}
	if err := repo.ValidateAuthorityGeneration(ctx, contracts.AuthorityDecision{AuthorityRef: r1.Ref, AuthorityVersion: r1.Version, AuthorityGenerationDigest: r1.Digest, DecidedBy: owner, GrantedScope: r1.Scope}, time.Now().UTC()); err != nil {
		t.Fatalf("package authority validation must accept R1: %v", err)
	}
	var modelOut bytes.Buffer
	if err := runAuthorityModelStatus(nil, fixture.getenv, &modelOut); err != nil || !strings.Contains(modelOut.String(), "implicit-v1") {
		t.Fatalf("authority model-status must resolve: %v", err)
	}
	var repairPreview bytes.Buffer
	if err := runRootAuthoritySuccessionPreview(nil, fixture.getenv, &repairPreview); err != nil {
		t.Fatalf("ADR-089 repair succession must derive from R1: %v", err)
	}
	var repair struct {
		Proposal contracts.RootAuthoritySuccessionProposal `json:"proposal"`
	}
	if err := json.Unmarshal(repairPreview.Bytes(), &repair); err != nil || repair.Proposal.Kind != contracts.RootAuthoritySuccessionProposalKind || repair.Proposal.Predecessor.Digest != r1.Digest {
		t.Fatalf("repair succession preview does not bind R1: %v", err)
	}
	readDB.Close()

	// 11: reopen and reconstruct the same state.
	repo, readDB, _, err = openGovernedRepositoryReadOnly(ctx, fixture.getenv)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := repo.LoadCurrentInstallationRoot(ctx, fixture.bootstrapDigest, time.Now().UTC())
	readDB.Close()
	if err != nil || reopened.Digest != r1.Digest || reopened.PredecessorDigest != fixture.rootDigest {
		t.Fatalf("state did not reconstruct after reopen: %v", err)
	}

	// 12: replay and double succession fail closed without a second active root.
	var replayOut bytes.Buffer
	if err := runRootAuthoritySuccessionAcceptWithTerminal([]string{"--proposal", proposalDigest, "--review", reviewed.ReviewDigest}, fixture.getenv, strings.NewReader("ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewed.ReviewDigest+"\n"), &replayOut, true); err != nil {
		t.Fatalf("exact replay must return the committed result: %v", err)
	}
	if err := runRootAuthoritySuccessionPreview([]string{"--from-historical-root"}, fixture.getenv, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already established") {
		t.Fatalf("second modernization must fail closed: %v", err)
	}
	if err := runRootAuthoritySuccessionProposal([]string{"--preview-file", successionPreview}, fixture.getenv, &bytes.Buffer{}); err == nil {
		t.Fatal("stale modernization preview must not be re-proposed")
	}
	if countSecureBlobs(t, ctx, fixture.dbPath, state.AuthorityGenerationNamespace) != 2 || countSecureBlobs(t, ctx, fixture.dbPath, state.AuthorityGenerationInvalidationNamespace) != 1 {
		t.Fatal("replay created additional authority records")
	}
	repo, readDB, _, err = openGovernedRepositoryReadOnly(ctx, fixture.getenv)
	if err != nil {
		t.Fatal(err)
	}
	defer readDB.Close()
	if final, err := repo.LoadCurrentInstallationRoot(ctx, fixture.bootstrapDigest, time.Now().UTC()); err != nil || final.Digest != r1.Digest {
		t.Fatalf("exactly one current root expected after replay: %v", err)
	}
}
