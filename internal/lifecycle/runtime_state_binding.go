package lifecycle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrRuntimeStateEvidence                = errors.New("runtime_state reconstruction evidence is incomplete or inconsistent")
	ErrRuntimeStatePluginPending           = errors.New("runtime_state plugin reconstruction is blocked by issue #140")
	ErrRuntimeStateClassificationAmbiguous = errors.New("runtime_state manifest classification is ambiguous pending issue #140")
)

type runtimePopulationMember struct {
	EntryPointID   string `json:"entry_point_id"`
	PackageVersion string `json:"package_version"`
	ContentDigest  string `json:"content_digest"`
}

type runtimeEvidenceIdentity struct {
	EntryPointID         string `json:"entry_point_id"`
	PackageID            string `json:"package_id"`
	PackageVersion       string `json:"package_version"`
	ContentDigest        string `json:"content_digest"`
	ContractDigest       string `json:"contract_digest"`
	ContractBytesDigest  string `json:"contract_bytes_digest"`
	ManifestBytesDigest  string `json:"manifest_bytes_digest"`
	InstalledState       string `json:"installed_state"`
	InstalledAt          string `json:"installed_at"`
	ActivatedAt          string `json:"activated_at"`
	ActivationReceiptIDs string `json:"activation_receipt_ids"`
}

type runtimeProposedBinding struct {
	EntryPointID          string          `json:"entry_point_id"`
	PackageID             string          `json:"package_id"`
	PackageVersion        string          `json:"package_version"`
	ContentDigest         string          `json:"content_digest"`
	ContractDigest        string          `json:"contract_digest"`
	RuntimeID             string          `json:"runtime_id"`
	RuntimeVersion        string          `json:"runtime_version"`
	RuntimeDigest         string          `json:"runtime_digest"`
	RegisteredAt          string          `json:"registered_at"`
	ExecutableBindingJSON json.RawMessage `json:"executable_binding_json"`
	Existing              bool            `json:"-"`
}

// RuntimeStatePreparation is the immutable §13.4 identity bundle. Digest is
// carried as RunRequest.PreconditionDigest; ProposedResultDigest is the
// runtime_state step Target.Digest. The component digests remain available for
// exact mismatch diagnosis and in-transaction revalidation.
type RuntimeStatePreparation struct {
	TargetPreStateDigest         string `json:"target_pre_state_digest"`
	PopulationDigest             string `json:"population_digest"`
	ReconstructionEvidenceDigest string `json:"reconstruction_evidence_digest"`
	ProposedResultDigest         string `json:"proposed_result_digest"`
}

func (p RuntimeStatePreparation) Digest() (string, error) {
	for name, digest := range map[string]string{
		"target pre-state":        p.TargetPreStateDigest,
		"population":              p.PopulationDigest,
		"reconstruction evidence": p.ReconstructionEvidenceDigest,
		"proposed result":         p.ProposedResultDigest,
	} {
		if err := contracts.ValidateSHA256Digest(digest); err != nil {
			return "", fmt.Errorf("runtime_state %s digest: %w", name, err)
		}
	}
	return digestCanonical(p)
}

type runtimeStateDerivation struct {
	Preparation RuntimeStatePreparation
	Proposals   []runtimeProposedBinding
}

type runtimeStateQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// PrepareRuntimeState derives all identities before a plan is approved. It is
// read-only and can run before storage_schema because it reads only stable
// source-side provenance/registry fields and binding key existence, never the
// historical handler_* or canonical runtime_* target columns.
func PrepareRuntimeState(ctx context.Context, db *sql.DB) (RuntimeStatePreparation, error) {
	if db == nil {
		return RuntimeStatePreparation{}, errors.New("runtime_state preparation requires a database")
	}
	derived, err := deriveRuntimeState(ctx, db)
	if err != nil {
		return RuntimeStatePreparation{}, err
	}
	return derived.Preparation, nil
}

type runtimeStateApplyBinding struct {
	planID, planDigest, installationID                  string
	stepID                                              string
	preconditionDigest, snapshotDigest, readinessDigest string
	now                                                 time.Time
	authority                                           *contracts.AuthorityDecision
}

type RuntimeStateDriver struct {
	DB           *sql.DB
	Journal      *Journal
	Guard        AuthorityGuard
	GuardFactory func(context.Context, contracts.AuthorityDecision) (AuthorityGuard, error)
	Prerequisite RuntimeStatePrerequisite
	Prepared     RuntimeStatePreparation

	binding            *runtimeStateApplyBinding
	mutationCheckpoint func(string) error
	applyCheckpoint    func(string) error
}

func (d *RuntimeStateDriver) BindRunRequest(ctx context.Context, req RunRequest, step contracts.LifecycleTransitionStep, authority *contracts.AuthorityDecision) error {
	if err := d.Prerequisite.BindRunRequest(ctx, req, step, authority); err != nil {
		return err
	}
	preparedDigest, err := d.Prepared.Digest()
	if err != nil {
		return err
	}
	if preparedDigest != req.PreconditionDigest {
		return errors.New("runtime_state RunRequest precondition digest does not bind the supplied preparation")
	}
	if d.Prepared.ProposedResultDigest != step.Target.Digest {
		return errors.New("runtime_state target digest does not bind the prepared proposed result")
	}
	if step.Authority.Required && authority == nil {
		return errors.New("runtime_state authority-bound run lacks a validated authority decision")
	}
	if step.Authority.Required && d.Guard == nil && d.GuardFactory != nil {
		guard, err := d.GuardFactory(ctx, *authority)
		if err != nil {
			return fmt.Errorf("bind transaction-time authority guard: %w", err)
		}
		d.Guard = guard
	}
	var authorityCopy *contracts.AuthorityDecision
	if authority != nil {
		copy := *authority
		authorityCopy = &copy
	}
	d.binding = &runtimeStateApplyBinding{
		planID: req.Plan.PlanID, planDigest: req.Plan.Digest, installationID: req.Plan.InstallationID, stepID: step.ID,
		preconditionDigest: req.PreconditionDigest, snapshotDigest: req.SnapshotDigest, readinessDigest: req.Plan.TargetManifestDigest,
		now: req.Now.UTC(), authority: authorityCopy,
	}
	return nil
}

func (d *RuntimeStateDriver) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	if d.DB == nil || d.Journal == nil {
		return errors.New("runtime_state driver requires database and journal")
	}
	if step.Authority.Required && d.Guard == nil {
		return errors.New("runtime_state authority-bound step requires an in-transaction authority guard")
	}
	d.Prerequisite.Journal = d.Journal
	return d.Prerequisite.Preflight(ctx, step)
}

func (*RuntimeStateDriver) Idempotent(contracts.LifecycleTransitionStep) bool { return true }

func (d *RuntimeStateDriver) Apply(ctx context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	if d.binding == nil || d.Guard == nil || d.binding.authority == nil {
		return ApplyResult{}, errors.New("runtime_state Apply lacks exact request or authority binding")
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("before-transaction"); err != nil {
			return ApplyResult{}, err
		}
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("begin runtime_state transaction: %w", err)
	}
	defer tx.Rollback()

	// ADR-088 §13.5: this is deliberately the first operation inside the
	// governed transaction.
	if err := d.Guard.RevalidateInTx(ctx, tx); err != nil {
		return ApplyResult{}, fmt.Errorf("in-transaction authority revalidation: %w", err)
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-guard"); err != nil {
			return ApplyResult{}, err
		}
	}
	current, err := deriveRuntimeState(ctx, tx)
	if err != nil {
		if errors.Is(err, ErrRuntimeStatePluginPending) {
			return d.recordOutcome(ctx, tx, step, ApplyFailedRecoverable, "plugin-verification-failed-pending-140", "")
		}
		if errors.Is(err, ErrRuntimeStateClassificationAmbiguous) {
			return d.recordOutcome(ctx, tx, step, ApplyReconcileRequired, "plugin-classification-ambiguous-pending-140", "")
		}
		return d.recordOutcome(ctx, tx, step, ApplyFailedRecoverable, "runtime-state-evidence-invalid", "")
	}
	if current.Preparation != d.Prepared {
		return d.recordOutcome(ctx, tx, step, ApplyFailedRecoverable, "runtime-state-prepared-identity-drift", "")
	}
	history, err := d.Journal.LoadInTx(ctx, tx)
	if err != nil {
		return ApplyResult{}, err
	}
	b := d.binding
	previous, sequence, err := nextStepJournalPosition(history, b.planID, b.planDigest, step.ID)
	if err != nil {
		return ApplyResult{}, err
	}
	for _, candidate := range []struct {
		state    contracts.LifecycleTransitionState
		recovery string
	}{{contracts.LifecycleCommitted, ""}, {contracts.LifecycleReconcileRequired, "runtime-state-result-mismatch"}} {
		entry := NewApplyOutcomeJournalEntry(b.planID, b.planDigest, b.installationID, step.ID, sequence, candidate.state, previous, b.preconditionDigest, b.snapshotDigest, b.readinessDigest, candidate.recovery, b.now, b.authority)
		if _, err := d.Journal.prepareAppend(entry, history); err != nil {
			return ApplyResult{}, fmt.Errorf("prevalidate tx-scoped outcome: %w", err)
		}
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-predicate-revalidation"); err != nil {
			return ApplyResult{}, err
		}
	}

	for _, proposal := range current.Proposals {
		if proposal.Existing {
			_, err = tx.ExecContext(ctx, `UPDATE invocation_runtime_bindings SET package_id=?,contract_digest=?,runtime_id=?,runtime_version=?,runtime_digest=?,executable_binding_json=NULL WHERE entry_point_id=? AND package_version=? AND content_digest=?`, proposal.PackageID, proposal.ContractDigest, proposal.RuntimeID, proposal.RuntimeVersion, proposal.RuntimeDigest, proposal.EntryPointID, proposal.PackageVersion, proposal.ContentDigest)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO invocation_runtime_bindings(entry_point_id,package_version,content_digest,package_id,contract_digest,runtime_id,runtime_version,runtime_digest,registered_at,executable_binding_json) VALUES(?,?,?,?,?,?,?,?,?,NULL)`, proposal.EntryPointID, proposal.PackageVersion, proposal.ContentDigest, proposal.PackageID, proposal.ContractDigest, proposal.RuntimeID, proposal.RuntimeVersion, proposal.RuntimeDigest, proposal.RegisteredAt)
		}
		if err != nil {
			return ApplyResult{}, fmt.Errorf("write runtime_state proposal: %w", err)
		}
		if d.mutationCheckpoint != nil {
			if err := d.mutationCheckpoint(proposal.EntryPointID); err != nil {
				return ApplyResult{}, err
			}
		}
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-mutation"); err != nil {
			return ApplyResult{}, err
		}
	}
	observedDigest, err := observeActiveRuntimeResultDigest(ctx, tx)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("observe runtime_state result: %w", err)
	}
	if observedDigest != d.Prepared.ProposedResultDigest || observedDigest != step.Target.Digest {
		return d.recordOutcome(ctx, tx, step, ApplyReconcileRequired, "runtime-state-result-mismatch", observedDigest)
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-result-check"); err != nil {
			return ApplyResult{}, err
		}
	}
	return d.recordOutcome(ctx, tx, step, ApplyCommitted, "", observedDigest)
}

func (d *RuntimeStateDriver) recordOutcome(ctx context.Context, tx *sql.Tx, step contracts.LifecycleTransitionStep, outcome ApplyOutcome, recovery, resultDigest string) (ApplyResult, error) {
	history, err := d.Journal.LoadInTx(ctx, tx)
	if err != nil {
		return ApplyResult{}, err
	}
	b := d.binding
	previous, sequence, err := nextStepJournalPosition(history, b.planID, b.planDigest, step.ID)
	if err != nil {
		return ApplyResult{}, err
	}
	entry := NewApplyOutcomeJournalEntry(b.planID, b.planDigest, b.installationID, step.ID, sequence, lifecycleStateFor(outcome), previous, b.preconditionDigest, b.snapshotDigest, b.readinessDigest, recovery, b.now, b.authority)
	if err := d.Journal.AppendInTx(ctx, tx, entry); err != nil {
		return ApplyResult{}, err
	}
	if d.applyCheckpoint != nil {
		if err := d.applyCheckpoint("after-journal-append"); err != nil {
			return ApplyResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Outcome: outcome, ResultingManifestDigest: resultDigest, RecoveryAction: recovery, OutcomeAlreadyRecorded: true}, nil
}

func deriveRuntimeState(ctx context.Context, q runtimeStateQueryer) (runtimeStateDerivation, error) {
	storagePreparation, err := PrepareStorageSchema(ctx, q)
	if err != nil {
		return runtimeStateDerivation{}, fmt.Errorf("derive runtime target pre-state: %w", err)
	}
	population, err := loadRuntimePopulation(ctx, q)
	if err != nil {
		return runtimeStateDerivation{}, err
	}
	rows, err := q.QueryContext(ctx, `SELECT ir.entry_point_id,ir.package_id,ir.package_version,ir.content_digest,ir.contract_json,ir.contract_digest,ir.registered_at,ip.manifest_json,ip.state,ip.installed_at,ip.activated_at,
			COALESCE((SELECT group_concat(activation_id, char(0)) FROM (SELECT activation_id FROM package_activation_receipts par WHERE par.package_id=ir.package_id AND par.package_version=ir.package_version AND par.content_digest=ir.content_digest ORDER BY activation_id)),''),
			EXISTS(SELECT 1 FROM invocation_runtime_bindings rb WHERE rb.entry_point_id=ir.entry_point_id AND rb.package_version=ir.package_version AND rb.content_digest=ir.content_digest),
			(SELECT rb.package_id FROM invocation_runtime_bindings rb WHERE rb.entry_point_id=ir.entry_point_id AND rb.package_version=ir.package_version AND rb.content_digest=ir.content_digest)
		FROM invocation_registry ir LEFT JOIN installed_packages ip ON ip.package_id=ir.package_id AND ip.package_version=ir.package_version AND ip.content_digest=ir.content_digest AND ip.state<>'removed'
		WHERE ir.active=1 ORDER BY ir.entry_point_id,ir.package_version,ir.content_digest`)
	if err != nil {
		return runtimeStateDerivation{}, err
	}
	defer rows.Close()
	var evidence []runtimeEvidenceIdentity
	var proposals []runtimeProposedBinding
	for rows.Next() {
		var p runtimeProposedBinding
		var contractJSON, manifestJSON []byte
		var installedState, installedAt, activatedAt sql.NullString
		var receiptIDs string
		var exists bool
		var existingPackageID sql.NullString
		if err := rows.Scan(&p.EntryPointID, &p.PackageID, &p.PackageVersion, &p.ContentDigest, &contractJSON, &p.ContractDigest, &p.RegisteredAt, &manifestJSON, &installedState, &installedAt, &activatedAt, &receiptIDs, &exists, &existingPackageID); err != nil {
			return runtimeStateDerivation{}, err
		}
		if exists && (!existingPackageID.Valid || existingPackageID.String != p.PackageID) {
			return runtimeStateDerivation{}, fmt.Errorf("%w: target binding package identity mismatch for %s", ErrRuntimeStateEvidence, p.EntryPointID)
		}
		if len(manifestJSON) == 0 {
			return runtimeStateDerivation{}, fmt.Errorf("%w: durable manifest missing for %s", ErrRuntimeStateClassificationAmbiguous, p.EntryPointID)
		}
		if !installedState.Valid || !installedAt.Valid || (!activatedAt.Valid && receiptIDs == "") {
			return runtimeStateDerivation{}, fmt.Errorf("%w: missing installation/activation provenance for %s", ErrRuntimeStateEvidence, p.EntryPointID)
		}
		var manifest packagecatalog.Manifest
		if err := json.Unmarshal(manifestJSON, &manifest); err != nil || manifest.Validate() != nil || manifest.PackageID != p.PackageID || manifest.Version != p.PackageVersion || manifest.ContentDigest != p.ContentDigest {
			return runtimeStateDerivation{}, fmt.Errorf("%w: durable manifest invalid or mismatched for %s", ErrRuntimeStateClassificationAmbiguous, p.EntryPointID)
		}
		if _, pluginBacked := manifest.ExecutableBinding(p.EntryPointID); pluginBacked {
			return runtimeStateDerivation{}, fmt.Errorf("%w: %s", ErrRuntimeStatePluginPending, p.EntryPointID)
		}
		contract, err := contracts.DecodeInvocationContract(contractJSON)
		if err != nil || contract.PackageID != p.PackageID || contract.PackageVersion != p.PackageVersion || contract.EntryPointID != p.EntryPointID {
			return runtimeStateDerivation{}, fmt.Errorf("%w: contract identity mismatch for %s", ErrRuntimeStateEvidence, p.EntryPointID)
		}
		contractBytesDigest := state.DigestPackageBytes(contractJSON)
		if contractBytesDigest != p.ContractDigest {
			return runtimeStateDerivation{}, fmt.Errorf("%w: contract digest mismatch for %s", ErrRuntimeStateEvidence, p.EntryPointID)
		}
		evidence = append(evidence, runtimeEvidenceIdentity{EntryPointID: p.EntryPointID, PackageID: p.PackageID, PackageVersion: p.PackageVersion, ContentDigest: p.ContentDigest, ContractDigest: p.ContractDigest, ContractBytesDigest: contractBytesDigest, ManifestBytesDigest: state.DigestPackageBytes(manifestJSON), InstalledState: installedState.String, InstalledAt: installedAt.String, ActivatedAt: activatedAt.String, ActivationReceiptIDs: receiptIDs})
		p.RuntimeID = "client-adapter:" + contract.PackageID + ":" + contract.EntryPointID
		p.RuntimeVersion = contract.Version
		p.RuntimeDigest = contractBytesDigest
		p.Existing = exists
		proposals = append(proposals, p)
	}
	if err := rows.Err(); err != nil {
		return runtimeStateDerivation{}, err
	}
	populationDigest, err := digestCanonical(population)
	if err != nil {
		return runtimeStateDerivation{}, err
	}
	evidenceDigest, err := digestCanonical(evidence)
	if err != nil {
		return runtimeStateDerivation{}, err
	}
	proposedDigest, err := digestCanonical(proposals)
	if err != nil {
		return runtimeStateDerivation{}, err
	}
	return runtimeStateDerivation{Preparation: RuntimeStatePreparation{TargetPreStateDigest: storagePreparation.TargetPreStateDigest, PopulationDigest: populationDigest, ReconstructionEvidenceDigest: evidenceDigest, ProposedResultDigest: proposedDigest}, Proposals: proposals}, nil
}

func loadRuntimePopulation(ctx context.Context, q runtimeStateQueryer) ([]runtimePopulationMember, error) {
	rows, err := q.QueryContext(ctx, `SELECT entry_point_id,package_version,content_digest FROM invocation_registry WHERE active=1 ORDER BY entry_point_id,package_version,content_digest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []runtimePopulationMember
	for rows.Next() {
		var member runtimePopulationMember
		if err := rows.Scan(&member.EntryPointID, &member.PackageVersion, &member.ContentDigest); err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

func observeActiveRuntimeResultDigest(ctx context.Context, q runtimeStateQueryer) (string, error) {
	rows, err := q.QueryContext(ctx, `SELECT rb.entry_point_id,rb.package_id,rb.package_version,rb.content_digest,rb.contract_digest,rb.runtime_id,rb.runtime_version,rb.runtime_digest,rb.registered_at,rb.executable_binding_json
		FROM invocation_registry ir JOIN invocation_runtime_bindings rb ON rb.entry_point_id=ir.entry_point_id AND rb.package_version=ir.package_version AND rb.content_digest=ir.content_digest
		WHERE ir.active=1 ORDER BY rb.entry_point_id,rb.package_version,rb.content_digest`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var observed []runtimeProposedBinding
	for rows.Next() {
		var p runtimeProposedBinding
		var executableBindingJSON []byte
		if err := rows.Scan(&p.EntryPointID, &p.PackageID, &p.PackageVersion, &p.ContentDigest, &p.ContractDigest, &p.RuntimeID, &p.RuntimeVersion, &p.RuntimeDigest, &p.RegisteredAt, &executableBindingJSON); err != nil {
			return "", err
		}
		p.ExecutableBindingJSON = append(json.RawMessage(nil), executableBindingJSON...)
		observed = append(observed, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return digestCanonical(observed)
}

func digestCanonical(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
