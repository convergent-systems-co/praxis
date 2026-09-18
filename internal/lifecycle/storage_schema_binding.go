package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// AuthorityGuard performs the tx-scoped repair-authority revalidation
// ADR-088 §13.5 requires as the first statement inside the shared Apply
// transaction: it re-checks that the repair authority decision is not
// expired/revoked/superseded, through the caller's own *sql.Tx, so no
// window exists between "authority checked" and "mutation admitted."
//
// RevalidateInTx never decrypts or interprets authority-decision content —
// internal/state has no crypto, by design (ADR-088 §10's writer-surface
// closure keeps it that way). The decision's plaintext content is already
// established earlier, at plan-approval time, by the same mechanism
// RunStep's own step.Authority.Required check already uses; this guard
// only proves that commitment is still fresh at the moment of mutation.
type AuthorityGuard interface {
	RevalidateInTx(ctx context.Context, tx *sql.Tx) error
}

// StorageSchemaDriver implements StepDriver for the ADR-088 §2
// storage_schema transition on invocation_runtime_bindings. Every mutating
// call happens inside one SQLite transaction it owns for the duration of
// Apply, sharing the atomic commit ADR-088 §13.3 requires between the
// domain mutation and the lifecycle outcome journal entry.
type StorageSchemaDriver struct {
	// DB is the authoritative installation database. Preflight queries it
	// directly (no transaction is open yet); Apply opens its own
	// transaction from it.
	DB *sql.DB
	// Journal is this installation's lifecycle_transition journal. Apply
	// uses Journal.LoadInTx/AppendInTx exclusively — never Load/Append —
	// while its own transaction is open, to avoid deadlocking this
	// store's single-connection pool.
	Journal *Journal
	// Guard performs the ADR-088 §13.5 tx-scoped authority revalidation.
	// It is the caller's responsibility to supply one when
	// step.Authority.Required is true; a nil Guard is only appropriate
	// for an unauthenticated test fixture or a step that does not require
	// authority.
	Guard AuthorityGuard
	// GuardFactory binds the durable transaction-time guard only after
	// RunStep has validated current authority. Committed restarts therefore
	// remain readable after authority expiry without constructing a fresh
	// mutation grant.
	GuardFactory func(context.Context, contracts.AuthorityDecision) (AuthorityGuard, error)
	// Prepared is the exact pre-state bundle approved before execution: live
	// schema, complete retained key population, and byte-bearing row contents.
	Prepared StorageSchemaPreparation

	binding *storageSchemaRunBinding

	// The following hooks are qualification instrumentation. Production
	// construction leaves both nil. observeDomainMutation is an explicit SQL
	// write log for the already-canonical path; rebuildCheckpoint permits a
	// deterministic abort after destructive DDL to prove transaction rollback.
	observeDomainMutation func(string)
	rebuildCheckpoint     func(string) error
	applyCheckpoint       func(string) error
}

type StorageSchemaPreparation struct {
	SchemaDigest             string `json:"schema_digest"`
	RetainedPopulationDigest string `json:"retained_population_digest"`
	TargetPreStateDigest     string `json:"target_pre_state_digest"`
}

func (p StorageSchemaPreparation) Digest() (string, error) {
	for name, digest := range map[string]string{"schema": p.SchemaDigest, "retained population": p.RetainedPopulationDigest, "target pre-state": p.TargetPreStateDigest} {
		if err := contracts.ValidateSHA256Digest(digest); err != nil {
			return "", fmt.Errorf("storage_schema %s digest: %w", name, err)
		}
	}
	return digestCanonical(p)
}

type storageRetainedKey struct {
	EntryPointID, PackageVersion, ContentDigest []byte
}

type storageRetainedRow struct {
	EntryPointID, PackageVersion, ContentDigest []byte
	PackageID, ContractDigest                   []byte
	RuntimeID, RuntimeVersion, RuntimeDigest    []byte
	RegisteredAt                                []byte
	ExecutableBindingJSON                       []byte
	ExecutableBindingPresent                    bool
}

func PrepareStorageSchema(ctx context.Context, q sqlQueryer) (StorageSchemaPreparation, error) {
	shape, err := IntrospectInvocationRuntimeBindingsShape(ctx, q)
	if err != nil {
		return StorageSchemaPreparation{}, err
	}
	kind := classifyInvocationRuntimeBindingsShape(shape)
	if kind == shapeUnrecognized {
		return StorageSchemaPreparation{}, ErrUnrecognizedSchemaShape
	}
	runtimeColumns := "runtime_id,runtime_version,runtime_digest"
	if kind == shapeHistoricalHandler {
		runtimeColumns = "handler_id,handler_version,handler_digest"
	}
	rows, err := q.QueryContext(ctx, `SELECT entry_point_id,package_version,content_digest,package_id,contract_digest,`+runtimeColumns+`,registered_at,executable_binding_json,executable_binding_json IS NOT NULL FROM invocation_runtime_bindings ORDER BY entry_point_id,package_version,content_digest`)
	if err != nil {
		return StorageSchemaPreparation{}, err
	}
	defer rows.Close()
	var keys []storageRetainedKey
	var contents []storageRetainedRow
	for rows.Next() {
		var row storageRetainedRow
		var executable []byte
		if err := rows.Scan(&row.EntryPointID, &row.PackageVersion, &row.ContentDigest, &row.PackageID, &row.ContractDigest, &row.RuntimeID, &row.RuntimeVersion, &row.RuntimeDigest, &row.RegisteredAt, &executable, &row.ExecutableBindingPresent); err != nil {
			return StorageSchemaPreparation{}, err
		}
		if row.ExecutableBindingPresent {
			row.ExecutableBindingJSON = append([]byte{}, executable...)
		}
		keys = append(keys, storageRetainedKey{append([]byte(nil), row.EntryPointID...), append([]byte(nil), row.PackageVersion...), append([]byte(nil), row.ContentDigest...)})
		contents = append(contents, row)
	}
	if err := rows.Err(); err != nil {
		return StorageSchemaPreparation{}, err
	}
	schemaDigest, err := shape.Digest()
	if err != nil {
		return StorageSchemaPreparation{}, err
	}
	populationDigest, err := digestCanonical(keys)
	if err != nil {
		return StorageSchemaPreparation{}, err
	}
	contentDigest, err := digestCanonical(contents)
	if err != nil {
		return StorageSchemaPreparation{}, err
	}
	return StorageSchemaPreparation{SchemaDigest: schemaDigest, RetainedPopulationDigest: populationDigest, TargetPreStateDigest: contentDigest}, nil
}

type storageSchemaRunBinding struct {
	planID, planDigest, installationID                  string
	preconditionDigest, snapshotDigest, readinessDigest string
	now                                                 time.Time
	authority                                           *contracts.AuthorityDecision
}

// BindRunRequest is called only by RunStep, after plan/digest and authority
// validation. It removes the former parallel-input trust boundary: Apply's
// paired outcome can only be built from this exact invocation.
func (d *StorageSchemaDriver) BindRunRequest(ctx context.Context, req RunRequest, step contracts.LifecycleTransitionStep, authority *contracts.AuthorityDecision) error {
	if req.StepID != step.ID || req.Plan.PlanID == "" || req.Plan.Digest == "" || req.Plan.InstallationID == "" {
		return errors.New("storage_schema run binding lacks exact plan, installation, or step identity")
	}
	if step.Authority.Required && authority == nil {
		return errors.New("storage_schema authority-bound run lacks a validated authority decision")
	}
	if step.Authority.Required && d.Guard == nil && d.GuardFactory != nil {
		guard, err := d.GuardFactory(ctx, *authority)
		if err != nil {
			return fmt.Errorf("bind transaction-time authority guard: %w", err)
		}
		d.Guard = guard
	}
	preparedDigest, err := d.Prepared.Digest()
	if err != nil {
		return err
	}
	if preparedDigest != req.PreconditionDigest || d.Prepared.SchemaDigest != step.Current.Digest {
		return errors.New("storage_schema RunRequest does not bind the exact prepared pre-state")
	}
	var authorityCopy *contracts.AuthorityDecision
	if authority != nil {
		copy := *authority
		authorityCopy = &copy
	}
	d.binding = &storageSchemaRunBinding{
		planID: req.Plan.PlanID, planDigest: req.Plan.Digest, installationID: req.Plan.InstallationID,
		preconditionDigest: req.PreconditionDigest, snapshotDigest: req.SnapshotDigest,
		readinessDigest: req.Plan.TargetManifestDigest, now: req.Now.UTC(), authority: authorityCopy,
	}
	return nil
}

// Preflight performs complete live schema introspection — never trusting
// schema_meta.value or migration status as evidence of actual shape
// (ADR-088 §2) — and refuses to proceed only when the live shape matches
// neither the historical nor the canonical recognized shape. It
// deliberately does NOT refuse when the live shape is already canonical
// even if the plan's declared Current shape names the historical one:
// that is the legitimate already-canonical case (ADR-088 §2/§6), resolved
// as a no-op inside Apply, not a Preflight downgrade refusal — a
// Preflight refusal here would leave this step permanently unable to
// reach committed, stranding runtime_state.
func (d *StorageSchemaDriver) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	if d.DB == nil {
		return errors.New("storage_schema driver requires a database")
	}
	if step.Authority.Required && d.Guard == nil {
		return errors.New("storage_schema authority-bound step requires an in-transaction authority guard")
	}
	shape, err := IntrospectInvocationRuntimeBindingsShape(ctx, d.DB)
	if err != nil {
		return fmt.Errorf("introspect live schema: %w", err)
	}
	if classifyInvocationRuntimeBindingsShape(shape) == shapeUnrecognized {
		return fmt.Errorf("%w: %w", ErrDowngradeRefused, ErrUnrecognizedSchemaShape)
	}
	return nil
}

// Idempotent reports that retrying this step's Apply is always safe: the
// entire mutation (or no-op observation) and its outcome commit together
// in one transaction, so a resumed attempt either finds nothing changed
// (prior attempt never committed) or never runs again at all (prior
// attempt committed, and RunStep's own current==Committed short-circuit
// prevents a further Apply call before Idempotent is ever consulted).
func (d *StorageSchemaDriver) Idempotent(contracts.LifecycleTransitionStep) bool { return true }

// Apply performs the complete governed storage_schema transition inside
// one SQLite transaction: authority revalidation, live re-introspection
// (the in-transaction predicate revalidation ADR-088 §13.4 requires,
// re-run fresh rather than trusting Preflight's earlier read), the
// structural rebuild (or no-op, for the already-canonical case), the
// driver's own pre-commit result-digest check against step.Target.Digest,
// and the tx-scoped outcome journal append — followed by exactly one
// commit. It reports ApplyResult.OutcomeAlreadyRecorded so RunStep never
// appends a second outcome entry for this attempt (ADR-088 §13.3).
func (d *StorageSchemaDriver) Apply(ctx context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	if d.DB == nil || d.Journal == nil {
		return ApplyResult{}, errors.New("storage_schema driver requires a database and journal")
	}
	if d.binding == nil {
		return ApplyResult{}, errors.New("storage_schema driver is not bound to a validated RunRequest")
	}
	if step.Authority.Required && (d.Guard == nil || d.binding.authority == nil) {
		return ApplyResult{}, errors.New("storage_schema authority-bound Apply requires guard and validated authority")
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("before-transaction"); err != nil {
			return ApplyResult{}, err
		}
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("begin storage_schema apply transaction: %w", err)
	}
	defer tx.Rollback()

	if d.Guard != nil {
		if err := d.Guard.RevalidateInTx(ctx, tx); err != nil {
			return ApplyResult{}, fmt.Errorf("in-transaction authority revalidation: %w", err)
		}
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-guard"); err != nil {
			return ApplyResult{}, err
		}
	}

	livePreparation, err := PrepareStorageSchema(ctx, tx)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("in-transaction storage pre-state revalidation: %w", err)
	}
	if livePreparation != d.Prepared {
		return ApplyResult{}, errors.New("storage_schema prepared pre-state identity drift")
	}
	history, err := d.Journal.LoadInTx(ctx, tx)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("load journal history in transaction: %w", err)
	}
	b := d.binding
	previous, sequence, err := nextStepJournalPosition(history, b.planID, b.planDigest, step.ID)
	if err != nil {
		return ApplyResult{}, err
	}
	for _, candidate := range []struct {
		state    contracts.LifecycleTransitionState
		recovery string
	}{{contracts.LifecycleCommitted, ""}, {contracts.LifecycleReconcileRequired, "resulting-manifest-mismatch"}} {
		entry := NewApplyOutcomeJournalEntry(b.planID, b.planDigest, b.installationID, step.ID, sequence, candidate.state, previous, b.preconditionDigest, b.snapshotDigest, b.readinessDigest, candidate.recovery, b.now, b.authority)
		if _, err := d.Journal.prepareAppend(entry, history); err != nil {
			return ApplyResult{}, fmt.Errorf("prevalidate tx-scoped outcome: %w", err)
		}
	}
	liveShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, tx)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("in-transaction schema revalidation: %w", err)
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-predicate-revalidation"); err != nil {
			return ApplyResult{}, err
		}
	}
	switch classifyInvocationRuntimeBindingsShape(liveShape) {
	case shapeCanonicalRuntime:
		// Already-canonical case: zero mutating SQL. The step still
		// reaches Committed via live-observed evidence below — this is a
		// legal applying->committed transition regardless of whether any
		// row changed (ADR-088 §2/§6).
	case shapeHistoricalHandler:
		if err := rebuildInvocationRuntimeBindingsToCanonical(ctx, tx, d.observeDomainMutation, d.rebuildCheckpoint); err != nil {
			return ApplyResult{}, fmt.Errorf("rebuild invocation_runtime_bindings: %w", err)
		}
	default:
		return ApplyResult{}, fmt.Errorf("in-transaction revalidation found an unrecognized shape: %w", ErrUnrecognizedSchemaShape)
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-mutation"); err != nil {
			return ApplyResult{}, err
		}
	}

	resultingShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, tx)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("observe post-mutation schema: %w", err)
	}
	resultingDigest, err := resultingShape.Digest()
	if err != nil {
		return ApplyResult{}, fmt.Errorf("digest observed schema: %w", err)
	}

	// The result-mismatch check happens here, entirely pre-commit: a
	// mismatch never produces a Committed entry that is later
	// "corrected" — it is never committed at all (ADR-088 §13).
	outcome, recovery := ApplyCommitted, ""
	if resultingDigest != step.Target.Digest {
		outcome, recovery = ApplyReconcileRequired, "resulting-manifest-mismatch"
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-result-check"); err != nil {
			return ApplyResult{}, err
		}
	}

	entry := NewApplyOutcomeJournalEntry(b.planID, b.planDigest, b.installationID, step.ID, sequence, lifecycleStateFor(outcome), previous, b.preconditionDigest, b.snapshotDigest, b.readinessDigest, recovery, b.now, b.authority)
	if err := d.Journal.AppendInTx(ctx, tx, entry); err != nil {
		return ApplyResult{}, fmt.Errorf("append tx-scoped outcome: %w", err)
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-journal-append"); err != nil {
			return ApplyResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ApplyResult{}, fmt.Errorf("commit storage_schema apply transaction: %w", err)
	}
	return ApplyResult{Outcome: outcome, ResultingManifestDigest: resultingDigest, RecoveryAction: recovery, OutcomeAlreadyRecorded: true}, nil
}

// rebuildInvocationRuntimeBindingsToCanonical performs the standard SQLite
// table-rebuild sequence: SQLite cannot alter an existing table's FOREIGN
// KEY clause in place, so the FK's ON DELETE CASCADE addition (alongside
// the three-column rename) requires create/copy/drop/rename, not a bare
// ALTER TABLE. Every row is copied unconditionally — active and inactive
// alike, with no WHERE clause of any kind — and the row count is verified
// to match exactly before the historical-shape table is ever dropped,
// so a partial or lossy copy can never proceed to the irreversible step.
func rebuildInvocationRuntimeBindingsToCanonical(ctx context.Context, tx *sql.Tx, observe func(string), checkpoint func(string) error) error {
	const stagingTable = "invocation_runtime_bindings_storage_schema_rebuild"
	execMutation := func(stage, statement string) error {
		if observe != nil {
			observe(statement)
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
		if checkpoint != nil {
			return checkpoint(stage)
		}
		return nil
	}
	if err := execMutation("staging-cleared", `DROP TABLE IF EXISTS `+stagingTable); err != nil {
		return fmt.Errorf("clear rebuild staging table: %w", err)
	}
	createDDL := `CREATE TABLE ` + stagingTable + ` (
		entry_point_id TEXT NOT NULL,
		package_version TEXT NOT NULL,
		content_digest TEXT NOT NULL,
		package_id TEXT NOT NULL,
		contract_digest TEXT NOT NULL,
		runtime_id TEXT NOT NULL,
		runtime_version TEXT NOT NULL,
		runtime_digest TEXT NOT NULL,
		registered_at TEXT NOT NULL,
		executable_binding_json BLOB,
		PRIMARY KEY (entry_point_id, package_version, content_digest),
		FOREIGN KEY (entry_point_id, package_version, content_digest)
		  REFERENCES invocation_registry(entry_point_id, package_version, content_digest)
		  ON DELETE CASCADE
	)`
	if err := execMutation("staging-created", createDDL); err != nil {
		return fmt.Errorf("create rebuild staging table: %w", err)
	}
	copyDML := `INSERT INTO ` + stagingTable + `(entry_point_id,package_version,content_digest,package_id,contract_digest,runtime_id,runtime_version,runtime_digest,registered_at,executable_binding_json)
		SELECT entry_point_id,package_version,content_digest,package_id,contract_digest,handler_id,handler_version,handler_digest,registered_at,executable_binding_json
		FROM invocation_runtime_bindings`
	if err := execMutation("rows-copied", copyDML); err != nil {
		return fmt.Errorf("copy every retained row unconditionally: %w", err)
	}
	var oldCount, newCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invocation_runtime_bindings`).Scan(&oldCount); err != nil {
		return fmt.Errorf("count historical-shape rows: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+stagingTable).Scan(&newCount); err != nil {
		return fmt.Errorf("count rebuilt rows: %w", err)
	}
	if oldCount != newCount {
		return fmt.Errorf("rebuild row count mismatch: %d retained rows became %d rows — refusing to proceed", oldCount, newCount)
	}
	if err := execMutation("original-dropped", `DROP TABLE invocation_runtime_bindings`); err != nil {
		return fmt.Errorf("drop historical-shape table: %w", err)
	}
	if err := execMutation("staging-renamed", `ALTER TABLE `+stagingTable+` RENAME TO invocation_runtime_bindings`); err != nil {
		return fmt.Errorf("rename rebuilt table into place: %w", err)
	}
	if err := execMutation("index-created", `CREATE INDEX `+invocationRuntimeBindingsIndex+` ON invocation_runtime_bindings(package_id, content_digest)`); err != nil {
		return fmt.Errorf("recreate package index: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check(invocation_runtime_bindings)`)
	if err != nil {
		return fmt.Errorf("run foreign key check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("rebuilt invocation_runtime_bindings failed foreign_key_check — refusing to commit")
	}
	return rows.Err()
}

// rebuildInvocationRuntimeBindingsToHistorical is the concrete reverse of the
// chosen Reversible=true strategy. It is intentionally a narrow structural
// primitive: a separately governed rollback step may call it, but this forward
// driver never silently invokes it. Like the forward rebuild it copies every
// retained row without interpreting content.
func rebuildInvocationRuntimeBindingsToHistorical(ctx context.Context, tx *sql.Tx) error {
	const stagingTable = "invocation_runtime_bindings_storage_schema_rollback"
	statements := []struct {
		name string
		sql  string
	}{
		{"clear rollback staging table", `DROP TABLE IF EXISTS ` + stagingTable},
		{"create historical rollback table", `CREATE TABLE ` + stagingTable + ` (
			entry_point_id TEXT NOT NULL,
			package_version TEXT NOT NULL,
			content_digest TEXT NOT NULL,
			package_id TEXT NOT NULL,
			contract_digest TEXT NOT NULL,
			handler_id TEXT NOT NULL,
			handler_version TEXT NOT NULL,
			handler_digest TEXT NOT NULL,
			registered_at TEXT NOT NULL,
			executable_binding_json BLOB,
			PRIMARY KEY (entry_point_id, package_version, content_digest),
			FOREIGN KEY (entry_point_id, package_version, content_digest)
			  REFERENCES invocation_registry(entry_point_id, package_version, content_digest)
		)`},
		{"copy every retained row into rollback table", `INSERT INTO ` + stagingTable + `(entry_point_id,package_version,content_digest,package_id,contract_digest,handler_id,handler_version,handler_digest,registered_at,executable_binding_json)
			SELECT entry_point_id,package_version,content_digest,package_id,contract_digest,runtime_id,runtime_version,runtime_digest,registered_at,executable_binding_json
			FROM invocation_runtime_bindings`},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.sql); err != nil {
			return fmt.Errorf("%s: %w", statement.name, err)
		}
	}
	var oldCount, newCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invocation_runtime_bindings`).Scan(&oldCount); err != nil {
		return fmt.Errorf("count canonical rows for rollback: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+stagingTable).Scan(&newCount); err != nil {
		return fmt.Errorf("count historical rollback rows: %w", err)
	}
	if oldCount != newCount {
		return fmt.Errorf("rollback row count mismatch: %d retained rows became %d rows", oldCount, newCount)
	}
	for _, statement := range []struct {
		name string
		sql  string
	}{
		{"drop canonical table", `DROP TABLE invocation_runtime_bindings`},
		{"rename historical rollback table", `ALTER TABLE ` + stagingTable + ` RENAME TO invocation_runtime_bindings`},
		{"recreate historical package index", `CREATE INDEX ` + invocationRuntimeBindingsIndex + ` ON invocation_runtime_bindings(package_id, content_digest)`},
	} {
		if _, err := tx.ExecContext(ctx, statement.sql); err != nil {
			return fmt.Errorf("%s: %w", statement.name, err)
		}
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check(invocation_runtime_bindings)`)
	if err != nil {
		return fmt.Errorf("run rollback foreign key check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("rolled-back invocation_runtime_bindings failed foreign_key_check")
	}
	return rows.Err()
}
