package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"

	"github.com/convergent-systems-co/praxis/internal/client"
	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var errGoalDriveDispatchDependencies = errors.New("native goal-drive dispatch requires a registered worker provider and an authoritative GoalStore key provider")

func init() {
	// The CLI only registers a package adapter. Resolution and dispatch remain
	// generic; Goals owns the lifecycle implementation and its contract.
	if err := client.RegisterInvocationHandler("praxis.package.goals", "goal-drive", func(ctx context.Context, in client.ResolvedInvocation, getenv func(string) string) error {
		return dispatchGoalDrive(ctx, normalizedOutput{
			EntryPointID: in.EntryPointID, PackageID: in.PackageID, PackageVersion: in.PackageVersion,
			PackageDigest: in.PackageDigest, GraphID: in.GraphID, GraphVersion: in.GraphVersion,
			Arguments: in.Arguments, Options: in.Options,
		}, getenv)
	}); err != nil {
		panic(err)
	}
}

// dispatchGoalDrive is the only CLI entry into Goal-drive. It parses the
// shared invocation contract and fails closed until setup-time providers can
// construct the proven goaldrive.Runtime; contract resolution is not execution.
func dispatchGoalDrive(ctx context.Context, out normalizedOutput, getenv func(string) string) error {
	if getenv == nil {
		getenv = os.Getenv
	}
	invocation, err := goaldrive.ParseInvocation(out.Options)
	if err != nil {
		if out.Options["provider"] == "" {
			return fmt.Errorf("parse goal-drive invocation: %w (list valid identities with `praxis providers`)", err)
		}
		return fmt.Errorf("parse goal-drive invocation: %w", err)
	}
	if invocation.Input.Kind != "goal_id" {
		return goaldrive.ErrGoalExecutionInput
	}
	if invocation.GoalVersion == "" {
		return goaldrive.ErrExactGoalVersionRequired
	}
	reconcileOnly := getenv("PRAXIS_PROVIDER_WORKSPACE_RECONCILE_ONLY") == "true"
	if reconcileOnly && getenv("PRAXIS_PROVIDER_WORKSPACE_ID") == "" {
		return errors.New("workspace reconciliation requires a provider workspace")
	}
	runtime, db, err := buildGoalDriveRuntime(ctx, out, invocation, getenv)
	if err != nil {
		return fmt.Errorf("construct native Goal-drive runtime: %w", err)
	}
	defer db.Close()
	repositoryAdapter, isGit := runtime.Repository.(goaldrive.GitRepository)
	var record goaldrive.TurnRecord
	if reconcileOnly {
		record, err = loadProviderWorkspaceTurn(ctx, runtime, getenv("PRAXIS_PROVIDER_WORKSPACE_ID"), getenv("PRAXIS_PROVIDER_WORKSPACE_VERSION"), invocation)
		if err != nil {
			return fmt.Errorf("load durable turn for workspace reconciliation: %w", err)
		}
	} else {
		runtime.OnTurnAllocated = func(turnID string) {
			announceGoalDriveTurn(os.Stderr, out, invocation, turnID)
		}
		record, err = runtime.Execute(ctx, invocation)
	}
	if workspaceID := getenv("PRAXIS_PROVIDER_WORKSPACE_ID"); workspaceID != "" {
		ledgerRecord := record
		if err != nil {
			ledgerRecord, err = loadProviderWorkspaceTurn(ctx, runtime, workspaceID, getenv("PRAXIS_PROVIDER_WORKSPACE_VERSION"), invocation)
			if err != nil {
				return fmt.Errorf("recover completed provider turn for workspace reconciliation: %w", err)
			}
		}
		if reconcileErr := reconcileProviderWorkspace(ctx, runtime, workspaceID, getenv("PRAXIS_PROVIDER_WORKSPACE_VERSION"), ledgerRecord); reconcileErr != nil {
			return fmt.Errorf("reconcile provider workspace: %w", reconcileErr)
		}
	}
	if !reconcileOnly && isGit {
		announceBlockedConsequence(ctx, os.Stderr, repositoryAdapter, invocation, record)
	}
	if err != nil {
		return fmt.Errorf("execute supervised Goal-drive turn: %w", err)
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Goal-drive result: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func buildGoalDriveRuntime(ctx context.Context, out normalizedOutput, invocation goaldrive.InvocationRequest, getenv func(string) string) (goaldrive.Runtime, *sql.DB, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	bootstrapPath := getenv("PRAXIS_BOOTSTRAP_RECORD")
	if bootstrapPath == "" {
		return goaldrive.Runtime{}, nil, fmt.Errorf("%w: PRAXIS_BOOTSTRAP_RECORD is required; initialize an explicit provider first", errGoalDriveDispatchDependencies)
	}
	record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath)
	if err != nil {
		return goaldrive.Runtime{}, nil, fmt.Errorf("load bootstrap metadata: %w", err)
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return goaldrive.Runtime{}, nil, fmt.Errorf("construct bootstrap registry: %w", err)
	}
	wrapper, err := registry.Open(ctx, record)
	if err != nil {
		return goaldrive.Runtime{}, nil, fmt.Errorf("open configured bootstrap provider: %w", err)
	}
	keyProviders := praxiscrypto.NewProviderRegistry()
	if err := keyProviders.Register(record.ProviderID, wrapper); err != nil {
		return goaldrive.Runtime{}, nil, err
	}
	service, err := keyProviders.Service(record.ProviderID, praxiscrypto.EnvelopePolicy{})
	if err != nil {
		return goaldrive.Runtime{}, nil, err
	}
	dbPath := getenv("PRAXIS_DB")
	if dbPath == "" {
		return goaldrive.Runtime{}, nil, errors.New("PRAXIS_DB is required for the encrypted GoalStore and durable ledger")
	}
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		return goaldrive.Runtime{}, nil, fmt.Errorf("open authoritative Praxis state: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = db.Close()
		}
	}()
	store := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: record.KeyID, Profile: record.Profile, Sensitivity: state.SensitivityConfidential}
	store.AuthorityGeneration = store
	activityStore := state.NewSQLiteEventStore(db)
	activity := &goaldrive.ActivityLog{Store: activityStore, Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	allowDetached := false
	dirtyStartDigest := ""
	if workspaceID := getenv("PRAXIS_PROVIDER_WORKSPACE_ID"); workspaceID != "" {
		workspaceVersion := getenv("PRAXIS_PROVIDER_WORKSPACE_VERSION")
		if workspaceVersion == "" {
			return goaldrive.Runtime{}, nil, errors.New("provider workspace version is required")
		}
		workspace, err := store.LoadProviderWorkspace(ctx, workspaceID, workspaceVersion, time.Now().UTC())
		if err != nil {
			return goaldrive.Runtime{}, nil, fmt.Errorf("load provider workspace: %w", err)
		}
		if workspace.GoalID != invocation.Input.GoalID || workspace.GoalVersion != invocation.GoalVersion || workspace.InvocationID != invocation.InvocationID || workspace.ProviderID != invocation.ProviderID || filepath.Clean(workspace.Path) != filepath.Clean(invocation.RepositoryPath) {
			return goaldrive.Runtime{}, nil, fmt.Errorf("provider workspace binding does not match exact Goal-drive invocation: workspace goal=%s/%s invocation goal=%s/%s workspace invocation=%s invocation=%s workspace provider=%s invocation provider=%s workspace path=%s invocation path=%s", workspace.GoalID, workspace.GoalVersion, invocation.Input.GoalID, invocation.GoalVersion, workspace.InvocationID, invocation.InvocationID, workspace.ProviderID, invocation.ProviderID, workspace.Path, invocation.RepositoryPath)
		}
		allowDetached = true
		if workspace.MigrationInputDigest != "" {
			dirtyStartDigest = workspace.MigrationInputDigest
		}
	}
	providers := goaldrive.NewRegistry()
	if getenv("PRAXIS_PROVIDER_WORKSPACE_RECONCILE_ONLY") != "true" {
		workers, err := configuredWorker(invocation, getenv, activity)
		if err != nil {
			return goaldrive.Runtime{}, nil, err
		}
		if err := providers.Register(invocation.ProviderID, workers); err != nil {
			return goaldrive.Runtime{}, nil, err
		}
	}
	if invocation.RepositoryPath == "" || invocation.Branch == "" {
		return goaldrive.Runtime{}, nil, errors.New("Goal-drive repository and exact branch are required")
	}
	remote := getenv("PRAXIS_GIT_REMOTE")
	if remote == "" {
		remote = "origin"
	}
	repository := goaldrive.GitRepository{Dir: invocation.RepositoryPath, Remote: remote, Branch: invocation.Branch, AllowDetached: allowDetached, AllowDirtyStart: dirtyStartDigest != "", DirtyStartDigest: dirtyStartDigest}
	ledger := goaldrive.Ledger{Store: activityStore, Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	var recovery *goaldrive.WorkerRecoveryContext
	if invocation.RecoverTurn != "" {
		bound, fingerprint, err := bindRecoveredTurn(ctx, ledger, repository, invocation)
		if err != nil {
			return goaldrive.Runtime{}, nil, err
		}
		repository.AllowRecoveryStart, repository.RecoveryDigest = true, fingerprint
		recovery = bound
	}
	runtime := goaldrive.Runtime{Controller: goaldrive.Controller{Ledger: ledger, Activity: activity, Providers: providers, AuthorityRequests: store, NoProgressLimit: invocation.NoProgressLimit}, Baselines: store, Repository: repository, GraphID: out.GraphID, GraphVersion: out.GraphVersion, Activity: activity, Recovery: recovery}
	closeOnError = false
	return runtime, db, nil
}

func reconcileProviderWorkspace(ctx context.Context, runtime goaldrive.Runtime, workspaceID, workspaceVersion string, turn goaldrive.TurnRecord) error {
	store, ok := runtime.Controller.AuthorityRequests.(goalstore.Repository)
	if !ok {
		return errors.New("provider workspace reconciliation requires the production GoalStore")
	}
	workspace, err := store.LoadProviderWorkspace(ctx, workspaceID, workspaceVersion, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := validateProviderWorkspaceTurn(workspace, turn); err != nil {
		return err
	}
	manager := goaldrive.ProviderWorkspaceManager{RootDir: filepath.Dir(workspace.Path)}
	snapshot, recoverErr := manager.Recover(ctx, workspace)
	if recoverErr != nil {
		return recoverErr
	}
	now := time.Now().UTC()
	state := contracts.ProviderWorkspaceBlocked
	if !snapshot.Clean {
		state = contracts.ProviderWorkspaceDirtyRecoverable
	}
	if turn.Progress && snapshot.Clean && snapshot.Head == turn.EndHead && turn.CheckpointPublished {
		validated, err := workspace.Next(contracts.ProviderWorkspaceValidated, snapshot.Head, now)
		if err != nil {
			return err
		}
		if _, err := store.SaveProviderWorkspace(ctx, validated, now, nil); err != nil {
			return err
		}
		published, err := validated.Next(contracts.ProviderWorkspacePublished, snapshot.Head, now)
		if err != nil {
			return err
		}
		if _, err := store.SaveProviderWorkspace(ctx, published, now, nil); err != nil {
			return err
		}
		cleaned, err := manager.Cleanup(ctx, published)
		if err != nil {
			return err
		}
		_, err = store.SaveProviderWorkspace(ctx, cleaned, now, nil)
		return err
	}
	next, err := workspace.Next(state, snapshot.Head, now)
	if err != nil {
		return err
	}
	_, err = store.SaveProviderWorkspace(ctx, next, now, nil)
	return err
}

func loadProviderWorkspaceTurn(ctx context.Context, runtime goaldrive.Runtime, workspaceID, workspaceVersion string, invocation goaldrive.InvocationRequest) (goaldrive.TurnRecord, error) {
	store, ok := runtime.Controller.AuthorityRequests.(goalstore.Repository)
	if !ok {
		return goaldrive.TurnRecord{}, errors.New("workspace reconciliation requires the production GoalStore")
	}
	workspace, err := store.LoadProviderWorkspace(ctx, workspaceID, workspaceVersion, time.Now().UTC())
	if err != nil {
		return goaldrive.TurnRecord{}, err
	}
	turns, err := runtime.Controller.Ledger.Load(ctx, invocation.Input.GoalID, invocation.GoalVersion)
	if err != nil {
		return goaldrive.TurnRecord{}, err
	}
	for _, turn := range turns {
		if turn.TurnID == workspace.TurnID && turn.InvocationID == invocation.InvocationID {
			return turn, nil
		}
	}
	return goaldrive.TurnRecord{}, fmt.Errorf("workspace reconciliation requires durable turn %s", workspace.TurnID)
}

func validateProviderWorkspaceTurn(workspace contracts.ProviderWorkspaceRecord, turn goaldrive.TurnRecord) error {
	if workspace.GoalID != turn.GoalID || workspace.GoalVersion != turn.GoalVersion || workspace.InvocationID != turn.InvocationID || workspace.TurnID != turn.TurnID || workspace.ProviderID != turn.ExecutorID || workspace.ChildObjective != turn.ChildObjective {
		return fmt.Errorf("provider workspace binding does not match durable Goal-drive turn: workspace goal=%s/%s turn goal=%s/%s workspace invocation=%s turn invocation=%s workspace turn=%s turn=%s workspace provider=%s turn executor=%s workspace child=%s turn child=%s", workspace.GoalID, workspace.GoalVersion, turn.GoalID, turn.GoalVersion, workspace.InvocationID, turn.InvocationID, workspace.TurnID, turn.TurnID, workspace.ProviderID, turn.ExecutorID, workspace.ChildObjective, turn.ChildObjective)
	}
	return nil
}

func configuredWorker(invocation goaldrive.InvocationRequest, getenv func(string) string, activity *goaldrive.ActivityLog) (goaldrive.Worker, error) {
	encoded := getenv("PRAXIS_GOAL_WORKER_ARGV")
	if encoded != "" {
		var argv []string
		if err := json.Unmarshal([]byte(encoded), &argv); err != nil || len(argv) == 0 || argv[0] == "" {
			return nil, errors.New("PRAXIS_GOAL_WORKER_ARGV must be a non-empty JSON argv array")
		}
		worker := goaldrive.CommandWorker{ProviderID: invocation.ProviderID, Dir: invocation.RepositoryPath, Command: argv, Activity: activity}
		if declared := getenv("PRAXIS_GOAL_WORKER_CAPABILITIES"); declared != "" {
			granted, err := goaldrive.ParseCapabilities(declared)
			if err != nil {
				return nil, fmt.Errorf("PRAXIS_GOAL_WORKER_CAPABILITIES: %w", err)
			}
			worker.Granted = granted
		}
		return worker, nil
	}
	switch invocation.ProviderID {
	case "codex", "codex-subscription":
		return goaldrive.NewCodexSubscriptionWorker(invocation.ProviderID, invocation.RepositoryPath, invocation.Model, activity)
	case "claude", "claude-subscription":
		return goaldrive.NewClaudeSubscriptionWorker(invocation.ProviderID, invocation.RepositoryPath, invocation.Model, activity)
	default:
		return nil, fmt.Errorf("%w: provider %q is not a registered provider; list valid identities with `praxis providers`", errGoalDriveDispatchDependencies, invocation.ProviderID)
	}
}

// announceGoalDriveTurn tells the operator the exact turn identity and the
// observation command before control enters the provider worker. It goes
// to stderr so the turn record on stdout stays the command's result.
func announceGoalDriveTurn(w io.Writer, out normalizedOutput, invocation goaldrive.InvocationRequest, turnID string) {
	announcement := map[string]any{
		"type": "goal-drive.turn_allocated", "goal_id": invocation.Input.GoalID, "goal_version": invocation.GoalVersion,
		"invocation_id": invocation.InvocationID, "turn_id": turnID, "provider": invocation.ProviderID, "mode": string(invocation.Mode),
		"observe_with":    "praxis supervise observe --goal-id=" + invocation.Input.GoalID + " --goal-version=" + invocation.GoalVersion + " --invocation-id=" + invocation.InvocationID + " --turn-id=" + turnID + " --follow",
		"invocation_with": "praxis supervise observe --goal-id=" + invocation.Input.GoalID + " --goal-version=" + invocation.GoalVersion + " --invocation-id=" + invocation.InvocationID + " --follow",
	}
	if encoded, err := json.Marshal(announcement); err == nil {
		fmt.Fprintln(w, string(encoded))
	}
}

// bindRecoveredTurn binds the checkout's uncommitted consequence to the
// exact BLOCKED turn named by --recover-turn. The turn must belong to this
// Goal generation, must have ended BLOCKED without a checkpoint, its end
// HEAD must be the checkout's current HEAD, and the checkout must carry a
// consequence: uncommitted changes, unpublished local commits (the evidence
// retained after a failed declared validation), or both. The checkout's
// consequence fingerprint (status, tracked diff, untracked file contents,
// unpublished commits) must equal the one the blocked turn recorded when it
// blocked; a turn that predates recording binds the checkout as found, with
// that provenance durable. The bound fingerprint is enforced by the
// repository adapter at turn start.
func bindRecoveredTurn(ctx context.Context, ledger goaldrive.Ledger, repository goaldrive.GitRepository, invocation goaldrive.InvocationRequest) (*goaldrive.WorkerRecoveryContext, string, error) {
	turns, err := ledger.Load(ctx, invocation.Input.GoalID, invocation.GoalVersion)
	if err != nil {
		return nil, "", fmt.Errorf("load Goal-drive ledger for recovery: %w", err)
	}
	var recovered *goaldrive.TurnRecord
	position := -1
	for i := range turns {
		if turns[i].TurnID == invocation.RecoverTurn {
			recovered, position = &turns[i], i
		}
	}
	if recovered == nil {
		return nil, "", fmt.Errorf("turn %q is not a durable turn of %s/%s", invocation.RecoverTurn, invocation.Input.GoalID, invocation.GoalVersion)
	}
	if recovered.Outcome != goaldrive.OutcomeBlocked || recovered.Progress {
		return nil, "", fmt.Errorf("turn %s is %s, not a BLOCKED turn without checkpoint", recovered.TurnID, recovered.Outcome)
	}
	// Only turns recorded after the blocked one can supersede it.
	for _, later := range turns[position+1:] {
		if later.ChildObjective == recovered.ChildObjective && later.Progress {
			return nil, "", fmt.Errorf("objective %s already progressed in turn %s after the blocked turn; recovery is superseded", recovered.ChildObjective, later.TurnID)
		}
	}
	head, err := gitOutput(ctx, repository.Dir, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return nil, "", fmt.Errorf("read recovery checkout HEAD: %w", err)
	}
	if head != recovered.EndHead {
		return nil, "", fmt.Errorf("checkout HEAD %s is not the blocked turn's HEAD %s; the consequence is not the one recorded", head, recovered.EndHead)
	}
	fingerprint, files, commits, err := repository.Fingerprint(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("fingerprint recovery consequence: %w", err)
	}
	if len(files) == 0 && len(commits) == 0 {
		return nil, "", errors.New("checkout is clean and published; there is no consequence to recover")
	}
	provenance := goaldrive.RecoveryProvenanceRecorded
	if recorded := recovered.ConsequenceFingerprint; recorded != "" {
		if fingerprint != recorded {
			return nil, "", fmt.Errorf("checkout consequence %s is not the consequence %s recorded by turn %s (recorded files %s, commits %s); it was altered after the block and cannot be bound", fingerprint, recorded, recovered.TurnID, strings.Join(recovered.ConsequenceFiles, ","), strings.Join(recovered.ConsequenceCommits, ","))
		}
	} else {
		// The blocked turn predates consequence recording (no fingerprint on
		// its record): the checkout as found now is bound, and that
		// provenance is durable on the recovery turn.
		provenance = goaldrive.RecoveryProvenanceObserved
	}
	return &goaldrive.WorkerRecoveryContext{RecoveredTurnID: recovered.TurnID, Objective: recovered.ChildObjective, Blocker: recovered.Blocker, Fingerprint: fingerprint, Files: files, Commits: commits, Provenance: provenance}, fingerprint, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// announceBlockedConsequence tells the operator, when a turn ends BLOCKED
// with uncommitted work or unpublished commits in the checkout, that the
// consequence is retained
// and names the exact public recovery command. The new invocation identity
// is operator intent and is the only value left to supply.
func announceBlockedConsequence(ctx context.Context, w io.Writer, repository goaldrive.GitRepository, invocation goaldrive.InvocationRequest, record goaldrive.TurnRecord) {
	if record.Outcome != goaldrive.OutcomeBlocked || record.Progress {
		return
	}
	fingerprint, files, commits := record.ConsequenceFingerprint, record.ConsequenceFiles, record.ConsequenceCommits
	if fingerprint == "" {
		var err error
		if fingerprint, files, commits, err = repository.Fingerprint(ctx); err != nil {
			return
		}
	}
	if len(files) == 0 && len(commits) == 0 {
		return
	}
	announcement := map[string]any{
		"type": "goal-drive.turn_blocked_with_consequence", "goal_id": invocation.Input.GoalID, "goal_version": invocation.GoalVersion,
		"invocation_id": invocation.InvocationID, "turn_id": record.TurnID, "child_objective": record.ChildObjective, "end_head": record.EndHead,
		"consequence_fingerprint": fingerprint, "consequence_files": files, "consequence_commits": commits,
		"recover_with":      "praxis goal-drive --goal-id=" + invocation.Input.GoalID + " --goal-version=" + invocation.GoalVersion + " --mode=supervised --provider=" + invocation.ProviderID + " --repo=" + invocation.RepositoryPath + " --branch=" + invocation.Branch + " --recover-turn=" + record.TurnID,
		"operator_supplies": []string{"--invocation-id=<new durable invocation identity>"},
	}
	if encoded, err := json.Marshal(announcement); err == nil {
		fmt.Fprintln(w, string(encoded))
	}
}
