package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var errGoalDriveDispatchDependencies = errors.New("native goal-drive dispatch requires a registered worker provider and an authoritative GoalStore key provider")

// dispatchGoalDrive is the only CLI entry into Goal-drive. It parses the
// shared invocation contract and fails closed until setup-time providers can
// construct the proven goaldrive.Runtime; contract resolution is not execution.
func dispatchGoalDrive(ctx context.Context, out normalizedOutput, getenv func(string) string) error {
	invocation, err := goaldrive.ParseInvocation(out.Options)
	if err != nil {
		return fmt.Errorf("parse goal-drive invocation: %w", err)
	}
	if invocation.Input.Kind != "goal_id" {
		return goaldrive.ErrGoalExecutionInput
	}
	if invocation.GoalVersion == "" {
		return goaldrive.ErrExactGoalVersionRequired
	}
	runtime, db, err := buildGoalDriveRuntime(ctx, out, invocation, getenv)
	if err != nil {
		return fmt.Errorf("construct native Goal-drive runtime: %w", err)
	}
	defer db.Close()
	record, err := runtime.Execute(ctx, invocation)
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
	workers, err := configuredWorker(invocation, getenv)
	if err != nil {
		return goaldrive.Runtime{}, nil, err
	}
	providers := goaldrive.NewRegistry()
	if err := providers.Register(invocation.ProviderID, workers); err != nil {
		return goaldrive.Runtime{}, nil, err
	}
	if invocation.RepositoryPath == "" || invocation.Branch == "" {
		return goaldrive.Runtime{}, nil, errors.New("Goal-drive repository and exact branch are required")
	}
	remote := getenv("PRAXIS_GIT_REMOTE")
	if remote == "" {
		remote = "origin"
	}
	runtime := goaldrive.Runtime{Controller: goaldrive.Controller{Ledger: goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}, Providers: providers, AuthorityRequests: store, NoProgressLimit: invocation.NoProgressLimit}, Baselines: store, Repository: goaldrive.GitRepository{Dir: invocation.RepositoryPath, Remote: remote, Branch: invocation.Branch}, GraphID: out.GraphID, GraphVersion: out.GraphVersion}
	closeOnError = false
	return runtime, db, nil
}

func configuredWorker(invocation goaldrive.InvocationRequest, getenv func(string) string) (goaldrive.Worker, error) {
	encoded := getenv("PRAXIS_GOAL_WORKER_ARGV")
	if encoded == "" {
		return nil, fmt.Errorf("%w: PRAXIS_GOAL_WORKER_ARGV is required for provider %q", errGoalDriveDispatchDependencies, invocation.ProviderID)
	}
	var argv []string
	if err := json.Unmarshal([]byte(encoded), &argv); err != nil || len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("PRAXIS_GOAL_WORKER_ARGV must be a non-empty JSON argv array")
	}
	return goaldrive.CommandWorker{ProviderID: invocation.ProviderID, Dir: invocation.RepositoryPath, Command: argv}, nil
}
