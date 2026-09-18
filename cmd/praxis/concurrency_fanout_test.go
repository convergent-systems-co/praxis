package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/agent"
	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/scheduler"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/migrations/sqlite"
	"github.com/convergent-systems-co/praxis/packages/develop"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// The fan-out harness re-executes this test binary as independent OS
// processes (real SQLite file locking, one connection per process) against an
// isolated scratch installation. Roles are selected through the environment.
const (
	fanoutRoleEnv    = "PRAXIS_FANOUT_ROLE"
	fanoutFixtureEnv = "PRAXIS_FANOUT_FIXTURE"
	fanoutKeyRef     = "fixture-fanout-key"
)

type fanoutFixture struct {
	DBPath, LegacyDBPath, BootstrapDigest, OSUser, LeaseLog string
	ProposalDigest, ReviewDigest                            string
	Adoption                                                contracts.AuthorityModelAdoption
}

type fanoutReport struct {
	Role      string         `json:"role"`
	PID       int            `json:"pid"`
	OK        int            `json:"ok"`
	Denied    int            `json:"denied"`
	Busy      int            `json:"busy"`
	Errors    []string       `json:"errors"`
	MaxOpMs   int64          `json:"max_op_ms"`
	Committed string         `json:"committed,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

func fanoutRepository(ctx context.Context, dbPath, bootstrapDigest string, governed bool) (goalstore.Repository, *sql.DB, error) {
	var db *sql.DB
	var err error
	if governed {
		db, err = state.OpenSQLite(ctx, dbPath)
	} else {
		db, err = state.OpenSQLiteForGovernedMigration(ctx, dbPath)
	}
	if err != nil {
		return goalstore.Repository{}, nil, err
	}
	return goalstore.Repository{Store: state.New(db), Crypto: praxiscrypto.EnvelopeService{Wrapper: recoveryTestWrapper{}}, KeyRef: fanoutKeyRef, Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: bootstrapDigest}, db, nil
}

func isBusy(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "locked") || strings.Contains(err.Error(), "busy"))
}

func (r *fanoutReport) note(err error) {
	if err == nil {
		r.OK++
		return
	}
	if isBusy(err) {
		r.Busy++
	}
	if len(r.Errors) < 8 {
		r.Errors = append(r.Errors, err.Error())
	}
}

func timed(r *fanoutReport, f func() error) error {
	start := time.Now()
	err := f()
	if ms := time.Since(start).Milliseconds(); ms > r.MaxOpMs {
		r.MaxOpMs = ms
	}
	return err
}

func fanoutDispatchInputs(requestID string) (inference.DispatchBinding, inference.DispatchCandidate, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, agent.ExecutionContext) {
	graph := develop.Graph()
	node := kernel.NodeDef{ID: "implement", Class: kernel.NodeInference}
	run := &kernel.RunExecution{RunID: "run:" + requestID}
	execution := agent.ExecutionContext{AgentID: "agent:weather", GenerationID: "7", GoalRef: "goal:weather"}
	binding := inference.DispatchBinding{RequestID: requestID, SubjectAgentID: execution.AgentID, AgentGeneration: execution.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: execution.GoalRef}
	candidate := inference.DispatchCandidate{RouteRecordID: "route:" + requestID, RequestID: requestID, SurfaceID: "surface:local", ExecutorID: "executor:local", ProviderID: "provider:local", WorkContext: "develop:weather-dashboard", TargetScope: "develop:weather-dashboard"}
	return binding, candidate, graph, node, run, execution
}

// TestFanoutHelperProcess is the worker body. It is skipped unless the
// parent harness selected a role through the environment.
func TestFanoutHelperProcess(t *testing.T) {
	role := os.Getenv(fanoutRoleEnv)
	if role == "" {
		t.Skip("worker role only")
	}
	var fx fanoutFixture
	if payload, err := os.ReadFile(os.Getenv(fanoutFixtureEnv)); err != nil || json.Unmarshal(payload, &fx) != nil {
		t.Fatalf("fixture unavailable: %v", err)
	}
	ctx := context.Background()
	report := fanoutReport{Role: role, PID: os.Getpid(), Extra: map[string]any{}}
	arg := ""
	if i := strings.Index(role, ":"); i >= 0 {
		role, arg = role[:i], role[i+1:]
	}
	governed := role != "legacy-writer" && role != "migrator"
	dbPath := fx.DBPath
	if role == "legacy-writer" || role == "migrator" {
		dbPath = fx.LegacyDBPath
	}
	repo, db, err := fanoutRepository(ctx, dbPath, fx.BootstrapDigest, governed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	store := repo.Store
	now := func() time.Time { return time.Now().UTC() }
	switch role {
	case "reader":
		for i := 0; i < 150; i++ {
			report.note(timed(&report, func() error {
				if _, err := repo.LoadCurrentInstallationRoot(ctx, fx.BootstrapDigest, now()); err != nil {
					return err
				}
				if _, err := sqlite.StatusOf(ctx, db); err != nil {
					return err
				}
				_, err := repo.LoadAuthorityModelState(ctx, now())
				return err
			}))
		}
	case "repo-writer", "legacy-writer":
		key := "repo:" + arg
		if err := store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: key, Capacity: 1, Exclusive: true}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 40; i++ {
			seq := i
			report.note(timed(&report, func() error {
				expires := now().Add(time.Hour)
				request := contracts.AuthorityRequest{ID: fmt.Sprintf("%s:request:%d", key, seq), Version: "1", BaselineID: "baseline:" + arg, BaselineVersion: "1", BaselineDigest: fixtureDigest('1'), ProposalID: "proposal:" + arg, ProposalVersion: "1", ProposalDigest: fixtureDigest('2'), ReviewRef: "review:" + arg, ReviewVersion: "1", ReviewDigest: fixtureDigest('3'), RequestedAuthority: contracts.GovernedWorkPlanAccept, RequestedScope: "repo:" + arg, Reason: "fanout unrelated governed state", Status: contracts.AuthorityRequestPending}
				if _, err := repo.SaveAuthorityRequest(ctx, request, now(), &expires); err != nil {
					return err
				}
				leases, err := store.AcquireSchedulerResourceLeases(ctx, "slice:"+arg, fmt.Sprintf("attempt:%d", seq), []scheduler.ResourceRequirement{{Key: key, Capacity: 1, Exclusive: true}}, now(), nil)
				if err != nil {
					return err
				}
				return store.ReleaseSchedulerResourceLeases(ctx, []string{leases[0].ID}, now())
			}))
		}
	case "lease-contender":
		log, err := os.OpenFile(fx.LeaseLog, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		for i := 0; i < 25; i++ {
			expiry := now().Add(2 * time.Second)
			leases, err := store.AcquireSchedulerResourceLeases(ctx, "slice:gpu", fmt.Sprintf("attempt:%d:%d", os.Getpid(), i), []scheduler.ResourceRequirement{{Key: "gpu", Capacity: 1, Exclusive: true}}, now(), &expiry)
			if errors.Is(err, state.ErrResourceUnavailable) {
				report.Denied++
				time.Sleep(3 * time.Millisecond)
				continue
			}
			if err != nil {
				report.note(err)
				continue
			}
			acquired := time.Now().UnixNano()
			time.Sleep(8 * time.Millisecond)
			released := time.Now().UnixNano()
			if err := store.ReleaseSchedulerResourceLeases(ctx, []string{leases[0].ID}, now()); err != nil {
				report.note(err)
				continue
			}
			fmt.Fprintf(log, "%d %d %d\n", os.Getpid(), acquired, released)
			report.OK++
		}
	case "dispatch-contender":
		binding, candidate, graph, node, run, execution := fanoutDispatchInputs("request:contended")
		routes := &lifecycleIssuedRoute{want: binding, candidate: candidate}
		issuer, err := develop.NewWeatherDispatchIssuer(store, &repo)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := issuer.Issue(ctx, routes, binding, now().Add(time.Hour)); err != nil {
			report.Extra["issue_error"] = err.Error()
		} else {
			report.Extra["issued"] = true
		}
		calls := 0
		routed := agent.IssuedRoutedAgentExecutor{Requests: lifecycleRequestID(binding.RequestID), Routes: routes, Authority: develop.WeatherDispatchAuthority{Store: store}, Executors: map[string]agent.RoutedInferenceExecutor{"executor:local": lifecycleInferenceExecutor{calls: &calls}}}
		result, err := routed.ExecuteAgentNode(ctx, graph, node, run, execution)
		report.Extra["executor_calls"] = calls
		if err != nil {
			report.Denied++
			report.Extra["dispatch_error"] = err.Error()
		} else {
			report.OK++
			report.Extra["outcome"] = result.Outcome
		}
	case "adoption-contender":
		digest, _ := fx.Adoption.Digest()
		committed, err := repo.AdoptAuthorityModel(ctx, fx.Adoption, fx.BootstrapDigest, fx.OSUser, "ADOPT "+digest, now())
		report.Committed = committed
		report.note(err)
	case "succession-contender":
		successor, _, err := repo.AcceptRootAuthoritySuccession(ctx, fx.ProposalDigest, fx.ReviewDigest, fx.BootstrapDigest, fx.OSUser, "ACCEPT-ROOT-SUCCESSOR "+fx.ProposalDigest+" "+fx.ReviewDigest, now())
		report.Committed = successor.Digest
		report.note(err)
	case "crash-claim":
		expiry := now().Add(30 * time.Second)
		if _, err := store.AcquireSchedulerResourceLeases(ctx, "slice:crash", "attempt:crash", []scheduler.ResourceRequirement{{Key: "crash-gpu", Capacity: 1, Exclusive: true}}, now(), &expiry); err != nil {
			t.Fatal(err)
		}
		os.Exit(137) // holding the lease, no release, no report
	case "crash-effect":
		binding, candidate, _, _, _, _ := fanoutDispatchInputs("request:crash")
		routes := &lifecycleIssuedRoute{want: binding, candidate: candidate}
		issuer, err := develop.NewWeatherDispatchIssuer(store, &repo)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := issuer.Issue(ctx, routes, binding, now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		authority := develop.WeatherDispatchAuthority{Store: store}
		authorization, err := authority.AuthorizeIssuedDispatch(ctx, binding, candidate, now())
		if err != nil {
			t.Fatal(err)
		}
		if err := authority.RecordDispatchAttempt(ctx, authorization.EffectID, now()); err != nil {
			t.Fatal(err)
		}
		os.Exit(137) // effect dispatched, outcome never recorded
	case "migrator":
		plan, err := sqlite.NewPlan(ctx, db, fx.BootstrapDigest, "installation-owner:"+fx.BootstrapDigest, "installation-governance:"+fx.BootstrapDigest, fixtureDigest('9'), now())
		if err != nil {
			t.Fatal(err)
		}
		journal, err := sqlite.ApplyPlan(ctx, db, plan, dbPath+".migration-"+arg+".snapshot", "installation-owner:"+fx.BootstrapDigest, now())
		if err != nil {
			report.Denied++
			report.Extra["error"] = err.Error()
		} else {
			report.OK++
			report.Extra["journal"] = journal.State
		}
	default:
		t.Fatalf("unknown role %q", role)
	}
	payload, _ := json.Marshal(report)
	fmt.Printf("FANOUT:%s\n", payload)
}

func spawnFanout(t *testing.T, fixturePath string, roles []string) map[string][]fanoutReport {
	t.Helper()
	var mu sync.Mutex
	var wg sync.WaitGroup
	reports := map[string][]fanoutReport{}
	for _, role := range roles {
		wg.Add(1)
		go func(role string) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestFanoutHelperProcess$", "-test.v")
			cmd.Env = append(os.Environ(), fanoutRoleEnv+"="+role, fanoutFixtureEnv+"="+fixturePath)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			err := cmd.Run()
			base := role
			if i := strings.Index(role, ":"); i >= 0 {
				base = role[:i]
			}
			var report fanoutReport
			for _, line := range strings.Split(out.String(), "\n") {
				if strings.HasPrefix(line, "FANOUT:") {
					_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "FANOUT:")), &report)
				}
			}
			if err != nil && !strings.HasPrefix(base, "crash-") {
				report.Errors = append(report.Errors, "process: "+err.Error()+": "+out.String())
			}
			report.Role = role
			mu.Lock()
			reports[base] = append(reports[base], report)
			mu.Unlock()
		}(role)
	}
	wg.Wait()
	return reports
}

func fanoutInstallation(t *testing.T, ctx context.Context, dir string, schema int) (string, string, goalstore.Repository, *sql.DB, contracts.AuthorityGeneration) {
	t.Helper()
	dbPath := filepath.Join(dir, fmt.Sprintf("praxis-%d.db", schema))
	if schema == 0 {
		db, err := state.OpenSQLite(ctx, dbPath)
		if err != nil {
			t.Fatal(err)
		}
		db.Close()
	} else {
		historicalDatabaseAt(t, ctx, dbPath, schema)
	}
	record := praxiscrypto.BootstrapRecord{Version: praxiscrypto.BootstrapRecordVersion, ProviderID: "fixture", KeyID: "fixture-key", KeyVersion: "1", KeyMaterialHash: fixtureDigest('4'), Owner: "fixture", Purpose: "fanout qualification", Profile: contracts.CryptoClassicalCompatible, SecurityLevel: praxiscrypto.SecurityPortableUserControlled, Platform: "test", Architecture: "test", CreatedAt: time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)}
	bootstrapDigest, err := record.Digest()
	if err != nil {
		t.Fatal(err)
	}
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	repo, db, err := fanoutRepository(ctx, dbPath, bootstrapDigest, schema == 0)
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	scope, _ := contracts.InstallationGovernanceScope(bootstrapDigest)
	root := contracts.AuthorityGeneration{Ref: scope, Version: "1", Principal: owner, Scope: scope, Capabilities: []string{contracts.AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:" + bootstrapDigest + ":os-user:" + current.Username, ProvenanceDigest: bootstrapDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: time.Now().UTC().Add(-time.Hour), AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelVersion, AuthorityModelDigest: contracts.AuthorityModelDigest()}
	root.Digest, _ = root.ComputeDigest()
	if err := repo.SaveAuthorityGeneration(ctx, root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	return dbPath, bootstrapDigest, repo, db, root
}

func TestConcurrentInstallationFanoutAcrossProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("process fan-out is skipped in short mode")
	}
	ctx := context.Background()
	dir := t.TempDir()
	dbPath, bootstrapDigest, repo, db, root := fanoutInstallation(t, ctx, dir, 0)
	legacyPath, _, legacyRepo, legacyDB, _ := fanoutInstallation(t, ctx, dir, 18)
	legacyDB.Close()
	_ = legacyRepo
	current, _ := user.Current()
	now := time.Now().UTC()
	if err := repo.Store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: "gpu", Capacity: 1, Exclusive: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: "crash-gpu", Capacity: 1, Exclusive: true}); err != nil {
		t.Fatal(err)
	}
	proposal, err := contracts.BuildRootAuthoritySuccession(root, bootstrapDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := repo.SaveRootAuthoritySuccessionProposal(ctx, proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	_, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, root.Principal, current.Username, "REVIEW-ROOT-SUCCESSOR "+proposalDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	adoption := contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v1-to-v2", Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelVersion, FromDigest: contracts.AuthorityModelDigest(), ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelSuccessorVersion, ToDigest: contracts.AuthorityModelSuccessorDigest(), RootRef: root.Ref, RootVersion: root.Version, RootDigest: root.Digest, Reason: "fanout adoption contention", CreatedAt: now}
	db.Close()
	leaseLog := filepath.Join(dir, "lease.log")
	if err := os.WriteFile(leaseLog, nil, 0600); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(dir, "fixture.json")
	payload, _ := json.Marshal(fanoutFixture{DBPath: dbPath, LegacyDBPath: legacyPath, BootstrapDigest: bootstrapDigest, OSUser: current.Username, LeaseLog: leaseLog, ProposalDigest: proposalDigest, ReviewDigest: reviewDigest, Adoption: adoption})
	if err := os.WriteFile(fixturePath, payload, 0600); err != nil {
		t.Fatal(err)
	}

	// Phase 1: 14 concurrent processes — readers, three repositories writing
	// unrelated governed state, four lease contenders, three dispatch
	// contenders on one intent, two adoption contenders, two succession
	// contenders, one crash after claim, one crash after effect.
	roles := []string{"reader", "reader", "repo-writer:weather", "repo-writer:praxis", "repo-writer:japetella", "lease-contender", "lease-contender", "lease-contender", "lease-contender", "dispatch-contender", "dispatch-contender", "dispatch-contender", "succession-contender", "succession-contender", "crash-claim", "crash-effect"}
	reports := spawnFanout(t, fixturePath, roles)
	busy, maxOp := 0, int64(0)
	for base, list := range reports {
		for _, r := range list {
			busy += r.Busy
			if r.MaxOpMs > maxOp {
				maxOp = r.MaxOpMs
			}
			if base == "reader" || base == "repo-writer" {
				if len(r.Errors) != 0 {
					t.Fatalf("%s reported errors: %v", r.Role, r.Errors)
				}
			}
		}
	}
	t.Logf("phase 1: %d processes, busy errors=%d, max single operation=%dms", len(roles), busy, maxOp)
	if busy != 0 {
		t.Fatalf("busy or locked errors surfaced to callers: %d", busy)
	}
	for _, r := range reports["reader"] {
		if r.OK != 150 {
			t.Fatalf("reader completed %d/150 iterations: %v", r.OK, r.Errors)
		}
	}
	for _, r := range reports["repo-writer"] {
		if r.OK != 40 {
			t.Fatalf("%s completed %d/40 iterations: %v", r.Role, r.OK, r.Errors)
		}
	}

	// G: lease holders never overlap.
	logBytes, _ := os.ReadFile(leaseLog)
	type hold struct{ pid, acquired, released int64 }
	var holds []hold
	for _, line := range strings.Split(strings.TrimSpace(string(logBytes)), "\n") {
		if line == "" {
			continue
		}
		var h hold
		fields := strings.Fields(line)
		h.pid, _ = strconv.ParseInt(fields[0], 10, 64)
		h.acquired, _ = strconv.ParseInt(fields[1], 10, 64)
		h.released, _ = strconv.ParseInt(fields[2], 10, 64)
		holds = append(holds, h)
	}
	sort.Slice(holds, func(i, j int) bool { return holds[i].acquired < holds[j].acquired })
	for i := 1; i < len(holds); i++ {
		if holds[i].acquired < holds[i-1].released {
			t.Fatalf("exclusive lease held concurrently by pid %d and %d", holds[i-1].pid, holds[i].pid)
		}
	}
	denied := 0
	for _, r := range reports["lease-contender"] {
		denied += r.Denied
	}
	t.Logf("lease contention: %d exclusive holds across 4 processes, %d deterministic denials, no overlap", len(holds), denied)
	if len(holds) == 0 || denied == 0 {
		t.Fatal("lease contention did not exercise both grant and denial paths")
	}

	// D/H: exactly one process issued and executed the contended dispatch.
	issued, executed, denials := 0, 0, 0
	for _, r := range reports["dispatch-contender"] {
		if r.Extra["issued"] == true {
			issued++
		}
		if r.OK == 1 {
			executed++
		}
		if r.Denied == 1 {
			denials++
		}
	}
	if issued != 1 || executed != 1 || denials != 2 {
		t.Fatalf("contended dispatch: issued=%d executed=%d denied=%d %+v", issued, executed, denials, reports["dispatch-contender"])
	}

	// Verification from a fresh connection (L: restart/recovery).
	repo, db, err = fanoutRepository(ctx, dbPath, bootstrapDigest, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var effects, succeeded int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), SUM(state='succeeded') FROM effects WHERE effect_id LIKE '%' AND action_intent_digest IN (SELECT intent_digest FROM exact_dispatch_grants)`).Scan(&effects, &succeeded); err != nil {
		t.Fatal(err)
	}
	var grants int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM exact_dispatch_grants`).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 2 || effects != 2 || succeeded != 1 {
		t.Fatalf("expected two grants (contended, crash) and one succeeded effect: grants=%d effects=%d succeeded=%d", grants, effects, succeeded)
	}

	// F: succession — both contenders report the same successor; one root.
	successorDigests := map[string]bool{}
	for _, r := range reports["succession-contender"] {
		if len(r.Errors) != 0 {
			t.Fatalf("succession contender failed: %v", r.Errors)
		}
		successorDigests[r.Committed] = true
	}
	currentRoot, err := repo.LoadCurrentInstallationRoot(ctx, bootstrapDigest, time.Now().UTC())
	if err != nil || currentRoot.Version != "2" || len(successorDigests) != 1 || !successorDigests[currentRoot.Digest] {
		t.Fatalf("succession contention: root=%+v digests=%v err=%v", currentRoot, successorDigests, err)
	}
	var invalidations, generations int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=?`, state.AuthorityGenerationInvalidationNamespace).Scan(&invalidations)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=?`, state.AuthorityGenerationNamespace).Scan(&generations)
	if invalidations != 1 || generations != 2 {
		t.Fatalf("succession records: invalidations=%d generations=%d", invalidations, generations)
	}

	// F: adoption contention against the established current root R1 (a
	// preview bound to the superseded R0 is refused as stale, so the phase
	// runs after succession): exactly one commit; the other contender
	// receives the same committed digest (idempotent) or a truthful refusal.
	adoption.RootRef, adoption.RootVersion, adoption.RootDigest = currentRoot.Ref, currentRoot.Version, currentRoot.Digest
	payload, _ = json.Marshal(fanoutFixture{DBPath: dbPath, LegacyDBPath: legacyPath, BootstrapDigest: bootstrapDigest, OSUser: current.Username, LeaseLog: leaseLog, ProposalDigest: proposalDigest, ReviewDigest: reviewDigest, Adoption: adoption})
	if err := os.WriteFile(fixturePath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	db.Close()
	adoptionReports := spawnFanout(t, fixturePath, []string{"adoption-contender", "adoption-contender", "reader", "reader"})
	reports["adoption-contender"] = adoptionReports["adoption-contender"]
	for _, r := range adoptionReports["reader"] {
		if len(r.Errors) != 0 || r.Busy != 0 {
			t.Fatalf("reader during adoption contention: %v", r.Errors)
		}
	}
	repo, db, err = fanoutRepository(ctx, dbPath, bootstrapDigest, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	adoptionDigest, _ := adoption.Digest()
	commits := 0
	for _, r := range reports["adoption-contender"] {
		if r.Committed == adoptionDigest && len(r.Errors) == 0 {
			commits++
		} else if len(r.Errors) == 0 {
			t.Fatalf("adoption contender returned unexpected digest %q", r.Committed)
		}
	}
	if commits == 0 {
		t.Fatalf("no adoption contender committed: %+v", reports["adoption-contender"])
	}
	if modelState, err := repo.LoadAuthorityModelState(ctx, time.Now().UTC()); err != nil || modelState.ActiveVersion != contracts.AuthorityModelSuccessorVersion || modelState.AdoptionDigest != adoptionDigest {
		t.Fatalf("adoption state after contention: %+v %v", modelState, err)
	}
	var adoptionRecords int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace='publisher_governance' AND object_id='authority-model-adoption:v1-to-v2'`).Scan(&adoptionRecords); err != nil || adoptionRecords != 1 {
		t.Fatalf("adoption records=%d %v", adoptionRecords, err)
	}

	// I: crash after claim — the lease stays held until its expiry, and the
	// acquisition path reaps it once expired (evaluated at the caller's
	// clock, so no wall-clock sleep is needed).
	if _, err := repo.Store.AcquireSchedulerResourceLeases(ctx, "slice:after-crash", "attempt:1", []scheduler.ResourceRequirement{{Key: "crash-gpu", Capacity: 1, Exclusive: true}}, time.Now().UTC(), nil); !errors.Is(err, state.ErrResourceUnavailable) {
		t.Fatalf("crashed holder's lease must remain held before expiry: %v", err)
	}
	if _, err := repo.Store.AcquireSchedulerResourceLeases(ctx, "slice:after-crash", "attempt:2", []scheduler.ResourceRequirement{{Key: "crash-gpu", Capacity: 1, Exclusive: true}}, time.Now().UTC().Add(31*time.Second), nil); err != nil {
		t.Fatalf("expired lease of crashed holder must be reaped on acquisition: %v", err)
	}

	// J: crash after effect dispatch — effect is truthfully non-final, the
	// consumed authority cannot be replayed, and recovery lists it.
	var crashState string
	var attempts int
	if err := db.QueryRowContext(ctx, `SELECT e.state, e.attempts FROM effects e JOIN exact_dispatch_grants g ON g.intent_digest=e.action_intent_digest WHERE e.state<>'succeeded'`).Scan(&crashState, &attempts); err != nil || crashState != "dispatched" || attempts != 1 {
		t.Fatalf("crashed dispatch effect state=%q attempts=%d err=%v", crashState, attempts, err)
	}
	binding, candidate, _, _, _, _ := fanoutDispatchInputs("request:crash")
	if _, err := (develop.WeatherDispatchAuthority{Store: repo.Store}).AuthorizeIssuedDispatch(ctx, binding, candidate, time.Now().UTC()); err == nil {
		t.Fatal("consumed exact dispatch authority was replayed after the crash")
	}
	recoverable, err := repo.Store.RecoverableEffects(ctx)
	if err != nil || len(recoverable) != 1 || recoverable[0].State != state.EffectDispatched {
		t.Fatalf("recoverable effects after crash: %+v %v", recoverable, err)
	}

	// K: migration attempted while other clients are active on the schema-18
	// installation: two migrators race, one commits, the other fails closed,
	// and the already-open legacy writers keep completing.
	migration := spawnFanout(t, fixturePath, []string{"migrator:a", "migrator:b", "legacy-writer:weather", "legacy-writer:praxis", "legacy-writer:japetella"})
	committed, refused := 0, 0
	for _, r := range migration["migrator"] {
		if r.OK == 1 {
			committed++
		} else {
			refused++
			if msg, _ := r.Extra["error"].(string); !strings.Contains(msg, "in progress by another process") && !strings.Contains(msg, "already current") && !strings.Contains(msg, "ledger still reports") {
				t.Fatalf("losing migrator must be refused by the maintenance lease or a fail-closed plan check, got: %v", r.Extra["error"])
			}
			t.Logf("refused migrator %s: %v", r.Role, r.Extra["error"])
		}
	}
	for _, r := range migration["legacy-writer"] {
		if r.OK != 40 || r.Busy != 0 {
			t.Fatalf("%s during migration: ok=%d busy=%d errors=%v", r.Role, r.OK, r.Busy, r.Errors)
		}
	}
	if committed != 1 || refused != 1 {
		t.Fatalf("concurrent migrators: committed=%d refused=%d %+v", committed, refused, migration["migrator"])
	}
	legacyDB, err = state.OpenSQLiteReadOnly(ctx, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer legacyDB.Close()
	status, err := sqlite.StatusOf(ctx, legacyDB)
	if err != nil || status.CurrentSchema != 19 || len(status.Pending) != 0 {
		t.Fatalf("legacy installation after concurrent migration: %+v %v", status, err)
	}
	var ledger, distinct int
	legacyDB.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT substr(name,1,4)) FROM praxis_schema_migrations`).Scan(&ledger, &distinct)
	if ledger != 19 || distinct != 19 {
		t.Fatalf("migration ledger after race: rows=%d distinct=%d", ledger, distinct)
	}
	journal, err := sqlite.ReadJournalLatest(ctx, legacyDB)
	if err != nil || journal.State != "committed" || journal.Holder != "" || journal.LeaseExpiresAt != nil {
		t.Fatalf("migration journal after race must be committed with the lease released: %+v %v", journal, err)
	}
	lease, err := sqlite.ReadMaintenanceLease(ctx, legacyDB, bootstrapDigest)
	if err != nil || lease.State != "released" || lease.Holder != "" {
		t.Fatalf("maintenance lease after race must be released: %+v %v", lease, err)
	}
}
