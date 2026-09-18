package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/migrations/sqlite"
	"github.com/convergent-systems-co/praxis/packages/develop"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type lifecycleIssuedRoute struct {
	want      inference.DispatchBinding
	candidate inference.DispatchCandidate
}

func (r *lifecycleIssuedRoute) PrepareDispatch(_ context.Context, got inference.DispatchBinding) (inference.DispatchCandidate, error) {
	if got != r.want {
		return inference.DispatchCandidate{}, errors.New("binding mismatch")
	}
	return r.candidate, nil
}

type lifecycleRequestID string

func (r lifecycleRequestID) ResolveIssuedInferenceRequestID(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, agent.ExecutionContext) (string, error) {
	return string(r), nil
}

type lifecycleInferenceExecutor struct{ calls *int }

func (e lifecycleInferenceExecutor) ID() string         { return "executor:local" }
func (e lifecycleInferenceExecutor) ProviderID() string { return "provider:local" }
func (e lifecycleInferenceExecutor) ExecuteRoutedInference(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, agent.ExecutionContext) (agent.InferenceExecution, error) {
	*e.calls++
	return agent.InferenceExecution{Result: kernel.NodeResult{Outcome: "done", Evidence: []string{"artifact:weather"}}}, nil
}

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

	// 13: canonical adoption v1 -> v2 -> v3 -> v6 (ADR-094 succession graph).
	for step, want := range []string{contracts.AuthorityModelSuccessorVersion, contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelRoutingVersion} {
		previewFile := filepath.Join(dir, "model-preview-"+want+".json")
		var previewOut bytes.Buffer
		if err := runAuthorityModelPreview([]string{"--output", previewFile}, fixture.getenv, &previewOut); err != nil {
			t.Fatalf("adoption preview %d failed: %v", step, err)
		}
		var preview struct {
			Adoption      contracts.AuthorityModelAdoption `json:"adoption"`
			PreviewDigest string                           `json:"preview_digest"`
		}
		if err := json.Unmarshal(previewOut.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if preview.Adoption.ToVersion != want {
			t.Fatalf("adoption preview %d offered %s, want %s", step, preview.Adoption.ToVersion, want)
		}
		if err := runAuthorityModelAdoptWithTerminal([]string{"--preview-file", previewFile}, fixture.getenv, strings.NewReader("ADOPT "+preview.PreviewDigest+"\n"), &bytes.Buffer{}, true); err != nil {
			t.Fatalf("adoption to %s failed: %v", want, err)
		}
		var status bytes.Buffer
		if err := runAuthorityModelStatus(nil, fixture.getenv, &status); err != nil || !strings.Contains(status.String(), `"ActiveVersion": "`+want+`"`) {
			t.Fatalf("model-status after adopting %s: %v %s", want, err, status.String())
		}
	}
	if err := runAuthorityModelPreview(nil, fixture.getenv, &bytes.Buffer{}); err == nil {
		t.Fatal("v6 must have no further adoptable successor")
	}

	// 14: issue routing authority under v6 through a delegated routing child.
	writeRepo, writeDB, err := openGovernedRepository(ctx, fixture.getenv)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	routePayload := []byte(`{"authority":"node","target":"weather"}`)
	routeSum := sha256.Sum256(routePayload)
	routeDigest := "sha256:" + hex.EncodeToString(routeSum[:])
	routeScope, _ := contracts.RoutingTargetContributionScope("routing-authority", "1", routeDigest)
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfileRoutingTargetContribution, ParentRef: r1.Ref, ParentVersion: r1.Version, ParentDigest: r1.Digest, DelegatedPrincipal: contracts.PrincipalRef{ID: "controller:routing", Kind: "controller"}, TargetKind: "routing.target-contribution", TargetIdentity: "routing-authority", TargetVersion: "1", TargetDigest: routeDigest, ProposalVersion: "1", ProposalDigest: routeDigest, ReviewVersion: "1", ReviewDigest: routeDigest, RequestedAuthority: contracts.AuthorityRoutingTargetContributionIssue, RequestedOperation: contracts.RoutingIssuanceOperation, RequestedScope: routeScope, ExpiresAt: expires, Reason: "issue exact weather routing targets", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelRoutingVersion, PolicyDigest: contracts.AuthorityModelRoutingDigest()}
	delegateRequest := contracts.AuthorityRequest{ID: "delegate-routing", Version: "1", RequestedAuthority: contracts.AuthorityDelegateCapability, RequestedScope: routeScope, Reason: "least privilege routing issuer", Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	delegateDigest, err := writeRepo.SaveAuthorityRequest(ctx, delegateRequest, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	delegateDecision := contracts.AuthorityDecision{RequestID: delegateRequest.ID, RequestVersion: "1", RequestDigest: delegateDigest, DecisionRef: "decision:delegate-routing", DecisionVersion: "1", DecidedBy: r1.Principal, AuthorityRef: r1.Ref, AuthorityVersion: r1.Version, AuthorityGenerationDigest: r1.Digest, GrantedScope: r1.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelRoutingDigest(), IssuedAt: now, ExpiresAt: &expires, Delegation: &delegation}
	if err := writeRepo.SaveAuthorityDecision(ctx, delegateRequest.ID, "1", delegateDecision, now, &expires); err != nil {
		t.Fatal(err)
	}
	child, err := writeRepo.SaveDelegatedAuthorityGeneration(ctx, delegateRequest.ID, "1", delegateDecision, builtinDelegationPolicy{}, now)
	if err != nil {
		t.Fatalf("routing delegation from R1 under v6 failed: %v", err)
	}
	issueRequest := contracts.AuthorityRequest{ID: "issue-weather-target", Version: "1", BaselineID: "route", BaselineVersion: "1", BaselineDigest: routeDigest, ProposalID: "target", ProposalVersion: "1", ProposalDigest: routeDigest, ReviewRef: "review", ReviewVersion: "1", ReviewDigest: routeDigest, RequestedAuthority: contracts.AuthorityRoutingTargetContributionIssue, RequestedScope: routeScope, Reason: "issue exact weather target", Status: contracts.AuthorityRequestPending}
	issueDigest, err := writeRepo.SaveAuthorityRequest(ctx, issueRequest, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	issueDecision := contracts.AuthorityDecision{RequestID: issueRequest.ID, RequestVersion: "1", RequestDigest: issueDigest, DecisionRef: "decision:issue-weather-target", DecisionVersion: "1", DecidedBy: child.Principal, AuthorityRef: child.Ref, AuthorityVersion: child.Version, AuthorityGenerationDigest: child.Digest, GrantedScope: routeScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelRoutingDigest(), IssuedAt: now, ExpiresAt: &expires}
	if err := writeRepo.SaveAuthorityDecision(ctx, issueRequest.ID, "1", issueDecision, now, &expires); err != nil {
		t.Fatal(err)
	}
	issued, err := writeRepo.SaveRoutingIssuance(ctx, contracts.RoutingIssuance{Version: "1", Kind: contracts.RoutingTargetContribution, Authority: contracts.AuthorityRoutingTargetContributionIssue, Scope: routeScope, RequestID: issueRequest.ID, RequestVersion: "1", DecisionRef: issueDecision.DecisionRef, DecisionVersion: "1", GenerationRef: child.Ref, GenerationVersion: child.Version, GenerationDigest: child.Digest, IssuedBy: child.Principal, Payload: routePayload, PayloadDigest: routeDigest, ExpiresAt: &expires})
	if err != nil {
		t.Fatalf("routing issuance under v6 failed: %v", err)
	}

	// 15: representative weather-app exact-dispatch path on schema 19.
	store := state.New(writeDB)
	graph := develop.Graph()
	node := kernel.NodeDef{ID: "implement", Class: kernel.NodeInference}
	run := &kernel.RunExecution{RunID: "run:weather"}
	execution := agent.ExecutionContext{AgentID: "agent:weather", GenerationID: "7", GoalRef: "goal:weather"}
	binding := inference.DispatchBinding{RequestID: "request:weather", SubjectAgentID: execution.AgentID, AgentGeneration: execution.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: execution.GoalRef}
	candidate := inference.DispatchCandidate{RouteRecordID: "route:weather", RequestID: binding.RequestID, SurfaceID: "surface:local", ExecutorID: "executor:local", ProviderID: "provider:local", WorkContext: "develop:weather-dashboard", TargetScope: "develop:weather-dashboard"}
	routes := &lifecycleIssuedRoute{want: binding, candidate: candidate}
	issuer, err := develop.NewWeatherDispatchIssuer(store, &writeRepo)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := issuer.Issue(ctx, routes, binding, expires)
	if err != nil {
		t.Fatalf("exact dispatch issuance on schema 19 failed: %v", err)
	}
	calls := 0
	routed := agent.IssuedRoutedAgentExecutor{Requests: lifecycleRequestID(binding.RequestID), Routes: routes, Authority: develop.WeatherDispatchAuthority{Store: store}, Executors: map[string]agent.RoutedInferenceExecutor{"executor:local": lifecycleInferenceExecutor{calls: &calls}}}
	result, err := routed.ExecuteAgentNode(ctx, graph, node, run, execution)
	if err != nil || result.Outcome != "done" || calls != 1 {
		t.Fatalf("weather exact dispatch: result=%#v calls=%d err=%v", result, calls, err)
	}
	var effectState string
	if err := writeDB.QueryRowContext(ctx, `SELECT state FROM effects WHERE approval_id=? AND capability_lease_id=?`, grant.ApprovalID, grant.LeaseID).Scan(&effectState); err != nil || effectState != "succeeded" {
		t.Fatalf("dispatch effect state %q %v", effectState, err)
	}
	if _, err := routed.ExecuteAgentNode(ctx, graph, node, run, execution); err == nil || calls != 1 {
		t.Fatalf("replayed exact dispatch must fail closed: calls=%d err=%v", calls, err)
	}
	writeDB.Close()

	// 16: reopen and reconstruct identical authority state.
	repo, readDB, _, err = openGovernedRepositoryReadOnly(ctx, fixture.getenv)
	if err != nil {
		t.Fatal(err)
	}
	if model, err := repo.LoadAuthorityModelState(ctx, time.Now().UTC()); err != nil || model.ActiveVersion != contracts.AuthorityModelRoutingVersion || model.ActiveDigest != contracts.AuthorityModelRoutingDigest() {
		t.Fatalf("v6 did not reconstruct after reopen: %+v %v", model, err)
	}
	if loaded, err := repo.LoadRoutingIssuance(ctx, contracts.RoutingIssuanceRef{ID: issued.ID, Version: issued.Version}, time.Now().UTC()); err != nil || loaded.GenerationDigest != child.Digest {
		t.Fatalf("routing issuance did not reconstruct after reopen: %v", err)
	}
	if _, err := repo.ValidateAuthorityGenerationLineage(ctx, child.Ref, child.Version, child.Digest, fixture.bootstrapDigest, time.Now().UTC()); err != nil {
		t.Fatalf("routing lineage to R1 did not reconstruct: %v", err)
	}
	var grants int
	if err := readDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM exact_dispatch_grants`).Scan(&grants); err != nil || grants != 1 {
		t.Fatalf("exact dispatch grant did not persist: %d %v", grants, err)
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
	// R0, R1, and the one delegated routing child; exactly one supersession.
	if countSecureBlobs(t, ctx, fixture.dbPath, state.AuthorityGenerationNamespace) != 3 || countSecureBlobs(t, ctx, fixture.dbPath, state.AuthorityGenerationInvalidationNamespace) != 1 {
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
