package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/lifecycle"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var openLifecycleRecoveryRepository = openGovernedRepository

// runLifecycleRecoveryCommand is a reserved control-plane entry point
// (PLAN-016 WU1). It is wired directly in cmd/praxis/main.go's switch,
// before the fallthrough to runDynamicInvocation, so it never depends on
// dynamic invocation resolution, package dynamic dispatch, or the
// invocation_runtime_bindings table this command family exists to recover.
func runLifecycleRecoveryCommand(args []string) error {
	return runLifecycleRecoveryCommandTo(args, os.Getenv, os.Stdout)
}

func runLifecycleRecoveryCommandTo(args []string, getenv func(string) string, out io.Writer) error {
	if len(args) == 0 || args[0] == "" {
		return errors.New("usage: praxis lifecycle-recover status --installation <id> [--db <path>] [--actor-id <id> --actor-kind <kind>]")
	}
	switch args[0] {
	case "status":
		return runLifecycleRecoveryStatus(args[1:], getenv, out)
	case "storage-schema", "runtime-state":
		return runLifecycleRecoveryTransition(args[0], args[1:], getenv, out)
	default:
		return fmt.Errorf("unsupported lifecycle-recover operation %q", args[0])
	}
}

type lifecycleRecoveryArgs struct {
	dbPath                            string
	installation, planID, planVersion string
	storageRequest, storageVersion    string
	runtimeRequest, runtimeVersion    string
	actor                             contracts.PrincipalRef
}

func parseLifecycleRecoveryArgs(args []string, getenv func(string) string) (lifecycleRecoveryArgs, error) {
	out := lifecycleRecoveryArgs{}
	if getenv != nil {
		out.dbPath = getenv("PRAXIS_DB")
		out.actor.ID = getenv("PRAXIS_ACTOR_ID")
		out.actor.Kind = getenv("PRAXIS_ACTOR_KIND")
	}
	out.planVersion, out.storageVersion, out.runtimeVersion = "1", "1", "1"
	for i := 0; i < len(args); i++ {
		key, value, consumed, err := lifecycleRecoveryOption(args, i)
		if err != nil {
			return lifecycleRecoveryArgs{}, err
		}
		i += consumed
		switch key {
		case "db":
			out.dbPath = value
		case "installation":
			out.installation = value
		case "actor-id":
			out.actor.ID = value
		case "actor-kind":
			out.actor.Kind = value
		case "plan":
			out.planID = value
		case "plan-version":
			out.planVersion = value
		case "storage-authority-request":
			out.storageRequest = value
		case "storage-authority-version":
			out.storageVersion = value
		case "runtime-authority-request":
			out.runtimeRequest = value
		case "runtime-authority-version":
			out.runtimeVersion = value
		default:
			return lifecycleRecoveryArgs{}, fmt.Errorf("unknown lifecycle-recover option --%s", key)
		}
	}
	if out.dbPath == "" {
		return lifecycleRecoveryArgs{}, errors.New("Praxis database must be explicit: use --db <path> or PRAXIS_DB")
	}
	if out.installation == "" {
		return lifecycleRecoveryArgs{}, errors.New("lifecycle-recover requires --installation <id>")
	}
	if err := out.actor.Validate(); err != nil {
		return lifecycleRecoveryArgs{}, fmt.Errorf("lifecycle-recover actor: %w", err)
	}
	return out, nil
}

func (a lifecycleRecoveryArgs) validateTransition() error {
	if a.planID == "" || a.planVersion == "" || a.storageRequest == "" || a.storageVersion == "" || a.runtimeRequest == "" || a.runtimeVersion == "" {
		return errors.New("lifecycle-recover transition requires --plan, --storage-authority-request, and --runtime-authority-request identities")
	}
	return nil
}

func lifecycleRecoveryOption(args []string, i int) (key, value string, consumed int, err error) {
	arg := args[i]
	if len(arg) < 3 || arg[0] != '-' || arg[1] != '-' {
		return "", "", 0, fmt.Errorf("unexpected lifecycle-recover argument %q", arg)
	}
	name := arg[2:]
	if i+1 >= len(args) {
		return "", "", 0, fmt.Errorf("--%s requires a value", name)
	}
	return name, args[i+1], 1, nil
}

// runLifecycleRecoveryStatus is deliberately read-only: it loads and prints
// the lifecycle_transition journal for one installation directly through
// internal/lifecycle.Journal, never through package/invocation resolution.
// WU5-WU10 extend this command family with governed transition execution;
// this work unit only establishes the reserved, independent entry point.
func runLifecycleRecoveryStatus(args []string, getenv func(string) string, out io.Writer) error {
	parsed, err := parseLifecycleRecoveryArgs(args, getenv)
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := state.OpenSQLiteReadOnly(ctx, parsed.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	journal, err := lifecycle.NewJournal(state.NewSQLiteEventStore(db), parsed.installation, parsed.actor)
	if err != nil {
		return err
	}
	history, err := journal.Load(ctx)
	if err != nil {
		return err
	}
	return printJSONTo(out, history)
}

func runLifecycleRecoveryTransition(operation string, args []string, getenv func(string) string, out io.Writer) error {
	parsed, err := parseLifecycleRecoveryArgs(args, getenv)
	if err != nil {
		return err
	}
	if err := parsed.validateTransition(); err != nil {
		return err
	}
	ctx := context.Background()
	commandEnv := func(key string) string {
		if key == "PRAXIS_DB" {
			return parsed.dbPath
		}
		if getenv != nil {
			return getenv(key)
		}
		return ""
	}
	repo, db, err := openLifecycleRecoveryRepository(ctx, commandEnv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	built, err := loadOrCreateLifecycleRecoveryBuild(ctx, operation, repo, db, parsed, now)
	if err != nil {
		return err
	}
	plan, prepared := built.Plan, built.RuntimePrepared
	journal, err := lifecycle.NewJournal(state.NewSQLiteEventStore(db), parsed.installation, parsed.actor)
	if err != nil {
		return err
	}
	validator := lifecycle.DurableAuthorityValidator{Source: repo, InstallationDigest: repo.InstallationDigest}
	snapshotDigest := built.SnapshotDigest
	switch operation {
	case "storage-schema":
		preconditionDigest, err := built.StoragePrepared.Digest()
		if err != nil {
			return err
		}
		req := lifecycle.RunRequest{Plan: plan, StepID: plan.Steps[0].ID, PreconditionDigest: preconditionDigest, SnapshotDigest: snapshotDigest, Authority: validator, RetryFailedRecoverable: true, Now: now}
		driver := &lifecycle.StorageSchemaDriver{DB: db, Journal: journal, Prepared: built.StoragePrepared, GuardFactory: func(ctx context.Context, decision contracts.AuthorityDecision) (lifecycle.AuthorityGuard, error) {
			return lifecycle.NewTransactionalAuthorityGuard(ctx, repo.Store, decision, time.Now().UTC())
		}}
		if err := lifecycle.RunStep(ctx, journal, req, driver); err != nil {
			return err
		}
	case "runtime-state":
		preconditionDigest, err := prepared.Digest()
		if err != nil {
			return err
		}
		req := lifecycle.RunRequest{Plan: plan, StepID: plan.Steps[1].ID, PreconditionDigest: preconditionDigest, SnapshotDigest: snapshotDigest, Authority: validator, RetryFailedRecoverable: true, Now: now}
		driver := &lifecycle.RuntimeStateDriver{DB: db, Journal: journal, Prepared: prepared, GuardFactory: func(ctx context.Context, decision contracts.AuthorityDecision) (lifecycle.AuthorityGuard, error) {
			return lifecycle.NewTransactionalAuthorityGuard(ctx, repo.Store, decision, time.Now().UTC())
		}}
		if err := lifecycle.RunStep(ctx, journal, req, driver); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported lifecycle recovery transition %q", operation)
	}
	history, err := journal.Load(ctx)
	if err != nil {
		return err
	}
	return printJSONTo(out, history)
}

type lifecycleRecoveryBuild struct {
	Plan            contracts.LifecyclePlan            `json:"plan"`
	StoragePrepared lifecycle.StorageSchemaPreparation `json:"storage_prepared"`
	RuntimePrepared lifecycle.RuntimeStatePreparation  `json:"runtime_prepared"`
	StorageDecision contracts.AuthorityDecision        `json:"storage_decision"`
	RuntimeDecision contracts.AuthorityDecision        `json:"runtime_decision"`
	SnapshotPath    string                             `json:"snapshot_path"`
	SnapshotDigest  string                             `json:"snapshot_digest"`
	RuntimeRefusal  string                             `json:"runtime_refusal,omitempty"`
}

func buildLifecycleRecoveryPlan(ctx context.Context, operation string, repo goalstore.Repository, db *sql.DB, parsed lifecycleRecoveryArgs, now time.Time) (lifecycleRecoveryBuild, error) {
	storageRequest, storageDecision, err := loadLifecycleRecoveryAuthority(ctx, repo, parsed.storageRequest, parsed.storageVersion, contracts.GovernedInstallationRepairStorageSchema, now)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	runtimeRequest, runtimeDecision, err := loadLifecycleRecoveryAuthority(ctx, repo, parsed.runtimeRequest, parsed.runtimeVersion, contracts.GovernedInstallationRepairRuntimeState, now)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	storagePrepared, err := lifecycle.PrepareStorageSchema(ctx, db)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	prepared, runtimeErr := lifecycle.PrepareRuntimeState(ctx, db)
	runtimeRefusal := ""
	if runtimeErr != nil {
		if operation != "storage-schema" {
			return lifecycleRecoveryBuild{}, runtimeErr
		}
		runtimeRefusal = "runtime-evidence-unavailable"
		switch {
		case errors.Is(runtimeErr, lifecycle.ErrRuntimeStatePluginPending):
			runtimeRefusal = "plugin-verification-failed-pending-140"
		case errors.Is(runtimeErr, lifecycle.ErrRuntimeStateClassificationAmbiguous):
			runtimeRefusal = "plugin-classification-ambiguous-pending-140"
		}
		prepared = lifecycle.RuntimeStatePreparation{
			TargetPreStateDigest:         storagePrepared.TargetPreStateDigest,
			PopulationDigest:             digestLifecycleRecoveryMarker("population:" + runtimeRefusal),
			ReconstructionEvidenceDigest: digestLifecycleRecoveryMarker("evidence:" + runtimeRefusal),
			ProposedResultDigest:         digestLifecycleRecoveryMarker("result:" + runtimeRefusal),
		}
	}
	storagePreconditionDigest, err := storagePrepared.Digest()
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	canonicalDigest, err := lifecycle.CanonicalInvocationRuntimeBindingsShape().Digest()
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	storageAuthority, err := lifecycleAuthorityRequirement(storageRequest, storageDecision)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	runtimeAuthority, err := lifecycleAuthorityRequirement(runtimeRequest, runtimeDecision)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	storageStep := contracts.LifecycleTransitionStep{ID: "storage-schema", Sequence: 1, Class: contracts.LifecycleSchema, Current: contracts.LifecycleComponentRef{Class: contracts.LifecycleSchema, ID: lifecycle.InvocationRuntimeBindingsTable, Version: "1", Digest: storagePrepared.SchemaDigest}, Target: contracts.LifecycleComponentRef{Class: contracts.LifecycleSchema, ID: lifecycle.InvocationRuntimeBindingsTable, Version: "2", Digest: canonicalDigest}, Preconditions: []contracts.LifecycleEvidenceRef{{ID: "storage-pre-state", Kind: "live-schema-and-retained-state", Source: lifecycle.InvocationRuntimeBindingsTable, Digest: storagePreconditionDigest}}, Authority: storageAuthority, Effect: contracts.LifecycleAuthorityBound, SnapshotRequired: true, Reversible: true, RecoveryStrategy: "rebuild-or-noop", ReadinessImpact: "schema"}
	runtimeStep := contracts.LifecycleTransitionStep{ID: "runtime-state", Sequence: 2, Class: contracts.LifecycleRuntime, Current: contracts.LifecycleComponentRef{Class: contracts.LifecycleRuntime, ID: "invocation-runtime-state", Version: "1", Digest: prepared.PopulationDigest}, Target: contracts.LifecycleComponentRef{Class: contracts.LifecycleRuntime, ID: "invocation-runtime-state", Version: "2", Digest: prepared.ProposedResultDigest}, Preconditions: []contracts.LifecycleEvidenceRef{{ID: "runtime-reconstruction", Kind: "durable-evidence", Source: "installed-packages/invocation-registry", Digest: prepared.ReconstructionEvidenceDigest}}, Authority: runtimeAuthority, Effect: contracts.LifecycleAuthorityBound, SnapshotRequired: true, Reversible: true, RecoveryStrategy: "reconcile-runtime-bindings", ReadinessImpact: "runtime"}
	readiness := contracts.LifecycleReadiness{CryptoBootstrap: "ready", StateStore: "ready", SchemaCompatibility: "ready", GovernanceRoot: "ready", AuthorityTopology: "ready", PackageRuntimeClosure: "ready", LifecycleRecovery: "ready", Installation: "ready"}
	plan, err := lifecycle.NewRuntimeRecoveryPlan(lifecycle.RuntimeRecoveryPlanSpec{PlanID: parsed.planID, PlanVersion: parsed.planVersion, InstallationID: parsed.installation, CurrentManifestDigest: storagePrepared.SchemaDigest, TargetManifestDigest: prepared.ProposedResultDigest, StorageSchemaStep: storageStep, RuntimeStateStep: runtimeStep, PreservedHistory: []contracts.LifecycleEvidenceRef{{ID: "retained-runtime-history", Kind: "durable-history", Source: lifecycle.InvocationRuntimeBindingsTable, Digest: storagePrepared.RetainedPopulationDigest}}, ExpectedReadiness: readiness})
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	return lifecycleRecoveryBuild{Plan: plan, StoragePrepared: storagePrepared, RuntimePrepared: prepared, StorageDecision: storageDecision, RuntimeDecision: runtimeDecision, RuntimeRefusal: runtimeRefusal}, nil
}

func lifecycleRecoveryEvidencePaths(parsed lifecycleRecoveryArgs) (string, string) {
	sum := sha256.Sum256([]byte(parsed.installation + "\x00" + parsed.planID + "\x00" + parsed.planVersion))
	suffix := hex.EncodeToString(sum[:8])
	base := parsed.dbPath + ".plan016-" + suffix
	return base + ".json", base + ".sqlite.snapshot"
}

func loadOrCreateLifecycleRecoveryBuild(ctx context.Context, operation string, repo goalstore.Repository, db *sql.DB, parsed lifecycleRecoveryArgs, now time.Time) (lifecycleRecoveryBuild, error) {
	evidencePath, snapshotBase := lifecycleRecoveryEvidencePaths(parsed)
	if body, err := os.ReadFile(evidencePath); err == nil {
		var built lifecycleRecoveryBuild
		if err := json.Unmarshal(body, &built); err != nil {
			return lifecycleRecoveryBuild{}, fmt.Errorf("decode lifecycle recovery evidence: %w", err)
		}
		if built.Plan.PlanID != parsed.planID || built.Plan.PlanVersion != parsed.planVersion || built.Plan.InstallationID != parsed.installation || !lifecycleRecoverySnapshotPathBound(snapshotBase, built.SnapshotPath) {
			return lifecycleRecoveryBuild{}, errors.New("lifecycle recovery evidence identity mismatch")
		}
		if err := built.Plan.VerifyDigest(); err != nil {
			return lifecycleRecoveryBuild{}, err
		}
		if err := validateLifecycleRecoveryBuildIdentities(built); err != nil {
			return lifecycleRecoveryBuild{}, err
		}
		actual, err := digestLifecycleRecoveryFile(built.SnapshotPath)
		if err != nil || actual != built.SnapshotDigest {
			return lifecycleRecoveryBuild{}, errors.New("lifecycle recovery snapshot is unavailable or digest-mismatched")
		}
		if err := verifyLifecycleRecoverySnapshot(ctx, built); err != nil {
			return lifecycleRecoveryBuild{}, err
		}
		return built, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return lifecycleRecoveryBuild{}, err
	}
	if operation != "storage-schema" {
		return lifecycleRecoveryBuild{}, errors.New("runtime_state requires the durable pre-mutation recovery plan and snapshot created by storage_schema")
	}
	built, err := buildLifecycleRecoveryPlan(ctx, operation, repo, db, parsed, now)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	if err := os.MkdirAll(filepath.Dir(snapshotBase), 0700); err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(snapshotBase), "."+filepath.Base(snapshotBase)+".staging-*")
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	snapshotPath := filepath.Join(stagingDir, filepath.Base(snapshotBase))
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stagingDir)
		}
	}()
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, snapshotPath); err != nil {
		return lifecycleRecoveryBuild{}, fmt.Errorf("create lifecycle recovery snapshot: %w", err)
	}
	if err := os.Chmod(snapshotPath, 0600); err != nil {
		return lifecycleRecoveryBuild{}, fmt.Errorf("restrict lifecycle recovery snapshot: %w", err)
	}
	built.SnapshotPath = snapshotPath
	built.SnapshotDigest, err = digestLifecycleRecoveryFile(snapshotPath)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	if err := validateLifecycleRecoveryBuildIdentities(built); err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	if err := verifyLifecycleRecoverySnapshot(ctx, built); err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	body, err := json.Marshal(built)
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(evidencePath), filepath.Base(evidencePath)+".*")
	if err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	temporaryEvidencePath := file.Name()
	defer os.Remove(temporaryEvidencePath)
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return lifecycleRecoveryBuild{}, err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return lifecycleRecoveryBuild{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return lifecycleRecoveryBuild{}, err
	}
	if err := file.Close(); err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	// Publishing the hard link is an atomic create-if-absent operation. A
	// crash before it leaves only uniquely named, unbound temporary artifacts
	// and cannot poison a future recovery attempt with a partial evidence file.
	if err := os.Link(temporaryEvidencePath, evidencePath); err != nil {
		return lifecycleRecoveryBuild{}, err
	}
	published = true
	return built, nil
}

func lifecycleRecoverySnapshotPathBound(base, candidate string) bool {
	parent := filepath.Dir(candidate)
	return filepath.Dir(parent) == filepath.Dir(base) &&
		strings.HasPrefix(filepath.Base(parent), "."+filepath.Base(base)+".staging-") &&
		filepath.Base(candidate) == filepath.Base(base)
}

func validateLifecycleRecoveryBuildIdentities(built lifecycleRecoveryBuild) error {
	if len(built.Plan.Steps) != 2 || len(built.Plan.Steps[0].Preconditions) != 1 || len(built.Plan.Steps[1].Preconditions) != 1 || len(built.Plan.PreservedHistory) != 1 {
		return errors.New("lifecycle recovery plan does not have the closed two-step evidence shape")
	}
	storageDigest, err := built.StoragePrepared.Digest()
	if err != nil {
		return err
	}
	_, err = built.RuntimePrepared.Digest()
	if err != nil {
		return err
	}
	storage, runtime := built.Plan.Steps[0], built.Plan.Steps[1]
	if storage.ID != "storage-schema" || storage.Current.Digest != built.StoragePrepared.SchemaDigest || storage.Preconditions[0].Digest != storageDigest || built.Plan.CurrentManifestDigest != built.StoragePrepared.SchemaDigest || built.Plan.PreservedHistory[0].Digest != built.StoragePrepared.RetainedPopulationDigest {
		return errors.New("storage_schema prepared identities do not bind the exact recovery plan")
	}
	if runtime.ID != "runtime-state" || runtime.Current.Digest != built.RuntimePrepared.PopulationDigest || runtime.Target.Digest != built.RuntimePrepared.ProposedResultDigest || runtime.Preconditions[0].Digest != built.RuntimePrepared.ReconstructionEvidenceDigest || built.Plan.TargetManifestDigest != built.RuntimePrepared.ProposedResultDigest || built.RuntimePrepared.TargetPreStateDigest != built.StoragePrepared.TargetPreStateDigest {
		return errors.New("runtime_state prepared identities do not bind the exact recovery plan")
	}
	for _, pair := range []struct {
		step     contracts.LifecycleTransitionStep
		decision contracts.AuthorityDecision
	}{{storage, built.StorageDecision}, {runtime, built.RuntimeDecision}} {
		decisionDigest, err := pair.decision.Digest()
		if err != nil || pair.step.Authority.DecisionDigest != decisionDigest || pair.step.Authority.DecisionRef != pair.decision.DecisionRef || pair.step.Authority.DecisionVersion != pair.decision.DecisionVersion || pair.step.Authority.AuthorityRef != pair.decision.AuthorityRef || pair.step.Authority.AuthorityVersion != pair.decision.AuthorityVersion || pair.step.Authority.AuthorityGenerationDigest != pair.decision.AuthorityGenerationDigest {
			return errors.New("recovery authority decision does not bind the exact plan step")
		}
	}
	return nil
}

func verifyLifecycleRecoverySnapshot(ctx context.Context, built lifecycleRecoveryBuild) error {
	info, err := os.Stat(built.SnapshotPath)
	if err != nil {
		return fmt.Errorf("stat lifecycle recovery snapshot: %w", err)
	}
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("lifecycle recovery snapshot permissions expose protected state")
	}
	db, err := state.OpenSQLiteReadOnly(ctx, built.SnapshotPath)
	if err != nil {
		return fmt.Errorf("open lifecycle recovery snapshot: %w", err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("lifecycle recovery snapshot integrity check: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("lifecycle recovery snapshot integrity check failed: %s", integrity)
	}
	storage, err := lifecycle.PrepareStorageSchema(ctx, db)
	if err != nil || storage != built.StoragePrepared {
		return errors.New("lifecycle recovery snapshot does not contain the exact prepared storage pre-state")
	}
	runtime, runtimeErr := lifecycle.PrepareRuntimeState(ctx, db)
	if built.RuntimeRefusal == "" {
		if runtimeErr != nil || runtime != built.RuntimePrepared {
			return errors.New("lifecycle recovery snapshot does not contain the exact prepared runtime evidence")
		}
	} else if runtimeErr == nil {
		return errors.New("lifecycle recovery snapshot no longer proves the recorded runtime refusal")
	}
	return nil
}

func digestLifecycleRecoveryFile(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func digestLifecycleRecoveryMarker(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func loadLifecycleRecoveryAuthority(ctx context.Context, repo goalstore.Repository, requestID, requestVersion, operation string, now time.Time) (contracts.AuthorityRequest, contracts.AuthorityDecision, error) {
	request, err := repo.LoadAuthorityRequest(ctx, requestID, requestVersion, now)
	if err != nil {
		return contracts.AuthorityRequest{}, contracts.AuthorityDecision{}, err
	}
	if request.RequestedAuthority != operation {
		return contracts.AuthorityRequest{}, contracts.AuthorityDecision{}, fmt.Errorf("authority request %s does not grant %s", requestID, operation)
	}
	decision, err := repo.LoadAuthorityDecision(ctx, requestID, requestVersion, now)
	if err != nil {
		return contracts.AuthorityRequest{}, contracts.AuthorityDecision{}, err
	}
	return request, decision, nil
}

func lifecycleAuthorityRequirement(request contracts.AuthorityRequest, decision contracts.AuthorityDecision) (contracts.LifecycleAuthorityRequirement, error) {
	requestDigest, err := request.DigestAt(decision.IssuedAt)
	if err != nil {
		return contracts.LifecycleAuthorityRequirement{}, err
	}
	decisionDigest, err := decision.Digest()
	if err != nil {
		return contracts.LifecycleAuthorityRequirement{}, err
	}
	return contracts.LifecycleAuthorityRequirement{Required: true, Operation: request.RequestedAuthority, Scope: request.RequestedScope, RequestRef: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest, AuthorityRef: decision.AuthorityRef, AuthorityVersion: decision.AuthorityVersion, AuthorityGenerationDigest: decision.AuthorityGenerationDigest}, nil
}
