package goaldrive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// WorkerCapability names one consequence a worker can bring about under its
// launch contract. A repository turn's checkpoint contract (ADR-070) needs a
// clean tree with a local commit, so the worker must be able to edit, stage,
// and commit; validation is the controller's when the repository declares
// it, and a worker may additionally be allowed to run it.
type WorkerCapability string

const (
	CapabilityEdit     WorkerCapability = "edit"
	CapabilityValidate WorkerCapability = "validate"
	CapabilityStage    WorkerCapability = "stage"
	CapabilityCommit   WorkerCapability = "commit"
)

// ParseCapabilities decodes an operator-declared capability list (a JSON
// array of capability names). Unknown names are refused so a misspelled
// declaration can never widen or silently narrow the worker's authority.
func ParseCapabilities(encoded string) ([]WorkerCapability, error) {
	var names []string
	if err := json.Unmarshal([]byte(encoded), &names); err != nil {
		return nil, fmt.Errorf("capability declaration must be a JSON array of capability names: %w", err)
	}
	known := map[string]WorkerCapability{string(CapabilityEdit): CapabilityEdit, string(CapabilityValidate): CapabilityValidate, string(CapabilityStage): CapabilityStage, string(CapabilityCommit): CapabilityCommit}
	out := make([]WorkerCapability, 0, len(names))
	seen := map[WorkerCapability]struct{}{}
	for _, name := range names {
		capability, ok := known[name]
		if !ok {
			return nil, fmt.Errorf("unknown worker capability %q (known: edit, validate, stage, commit)", name)
		}
		if _, dup := seen[capability]; dup {
			continue
		}
		seen[capability] = struct{}{}
		out = append(out, capability)
	}
	return out, nil
}

// CapabilityDeclaringWorker reports the capabilities its launch contract
// actually grants. The controller refuses to dispatch a turn whose required
// consequences exceed them: an outcome must never be assigned to a worker
// that cannot satisfy it.
type CapabilityDeclaringWorker interface {
	Capabilities() []WorkerCapability
}

// RequiredRepositoryCapabilities are the consequences every repository
// turn's checkpoint contract requires of the worker.
func RequiredRepositoryCapabilities() []WorkerCapability {
	return []WorkerCapability{CapabilityEdit, CapabilityStage, CapabilityCommit}
}

var ErrCapabilityUnsatisfiable = errors.New("worker cannot satisfy the turn's required capabilities")

// CapabilityError is returned before any provider execution when the
// selected worker's declared capabilities do not cover the required
// consequences. It is durable evidence of the class
// "checkpoint-required action unavailable to worker".
type CapabilityError struct {
	ProviderID string
	Required   []WorkerCapability
	Granted    []WorkerCapability
	Missing    []WorkerCapability
}

func (e *CapabilityError) Error() string {
	return fmt.Sprintf("%v: provider %q lacks %s (required %s, granted %s)", ErrCapabilityUnsatisfiable, e.ProviderID, joinCapabilities(e.Missing), joinCapabilities(e.Required), joinCapabilities(e.Granted))
}

func (e *CapabilityError) Unwrap() error { return ErrCapabilityUnsatisfiable }

func joinCapabilities(items []WorkerCapability) string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ",")
}

// CheckCapabilities compares a worker's declared capabilities with the
// required set. A worker that declares nothing is treated as declaring the
// full set only when it does not implement CapabilityDeclaringWorker (a
// legacy adapter); an explicit empty declaration is a refusal.
func CheckCapabilities(worker Worker, providerID string, required []WorkerCapability) error {
	declaring, ok := worker.(CapabilityDeclaringWorker)
	if !ok {
		return nil
	}
	granted := declaring.Capabilities()
	have := map[WorkerCapability]struct{}{}
	for _, capability := range granted {
		have[capability] = struct{}{}
	}
	var missing []WorkerCapability
	for _, capability := range required {
		if _, ok := have[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &CapabilityError{ProviderID: providerID, Required: required, Granted: granted, Missing: missing}
}

// WorkerContext is the authoritative context Praxis hands the worker: the
// exact Goal generation, the accepted work unit and its provenance, the
// repository authority, the checkpoint contract, and the authority
// boundaries. The worker must never have to discover these by reading
// Praxis state or files outside the repository.
type WorkerContext struct {
	Goal        WorkerGoalContext       `json:"goal"`
	Unit        WorkerUnitContext       `json:"unit"`
	Repository  WorkerRepositoryContext `json:"repository"`
	Checkpoint  WorkerCheckpointContext `json:"checkpoint"`
	Authority   WorkerAuthorityContext  `json:"authority"`
	Recovery    *WorkerRecoveryContext  `json:"recovery,omitempty"`
	Invariants  []string                `json:"invariants"`
	Constraints []string                `json:"constraints,omitempty"`
}

type WorkerGoalContext struct {
	ID                 string   `json:"id"`
	Version            string   `json:"version"`
	Digest             string   `json:"digest"`
	OriginalIntent     string   `json:"original_intent"`
	RefinedOutcome     string   `json:"refined_outcome"`
	Scope              string   `json:"scope,omitempty"`
	SuccessCriteria    []string `json:"success_criteria,omitempty"`
	Constraints        []string `json:"constraints,omitempty"`
	NonGoals           []string `json:"non_goals,omitempty"`
	ValidityPredicates []string `json:"validity_predicates,omitempty"`
}

type WorkerUnitContext struct {
	ID            string                     `json:"id"`
	Requirements  []contracts.RequirementRef `json:"requirements"`
	Prerequisites []string                   `json:"prerequisites,omitempty"`
	Dependents    []string                   `json:"dependents,omitempty"`
	Provenance    string                     `json:"provenance"`
	SourceRef     string                     `json:"source_ref"`
}

type WorkerRepositoryContext struct {
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	StartHead string `json:"start_head"`
}

type WorkerCheckpointContext struct {
	Predicates         []string `json:"predicates"`
	DeclaredValidation string   `json:"declared_validation,omitempty"`
	ValidationDeclared bool     `json:"validation_declared"`
}

type WorkerAuthorityContext struct {
	Granted   []WorkerCapability `json:"granted"`
	Forbidden []string           `json:"forbidden"`
	// Envelope is the non-interactive execution envelope the worker actually
	// runs under (#170); nil when the launch contract does not declare one.
	Envelope *WorkerExecutionEnvelope `json:"envelope,omitempty"`
}

// WorkerExecutionEnvelope is the usable non-interactive execution envelope
// of a worker launch: whether prompts exist, the exact tool patterns the
// launch allows, how shell calls are judged, and what happens to a call
// outside the envelope. Weather II workers ran under a fixed allowlist with
// prompts disabled and discovered the denials mid-turn (turns 3, 4, 6, 7),
// re-running work each time; the envelope is declared by the launch
// contract, rendered to the worker, and recorded durably at dispatch.
type WorkerExecutionEnvelope struct {
	Interactive  bool     `json:"interactive"`
	Tools        []string `json:"tools,omitempty"`
	ShellPolicy  string   `json:"shell_policy,omitempty"`
	DenialPolicy string   `json:"denial_policy,omitempty"`
}

// EnvelopeDeclaringWorker reports the execution envelope its launch
// contract establishes.
type EnvelopeDeclaringWorker interface {
	ExecutionEnvelope() *WorkerExecutionEnvelope
}

// WorkerRecoveryContext describes uncommitted consequence bound from an
// earlier BLOCKED turn of the same objective that this turn must validate
// and commit, correct, or deliberately discard.
type WorkerRecoveryContext struct {
	RecoveredTurnID string   `json:"recovered_turn_id"`
	Objective       string   `json:"objective"`
	Blocker         string   `json:"blocker"`
	Fingerprint     string   `json:"fingerprint"`
	Files           []string `json:"files"`
	// Commits are the turn's local commits not yet published to the remote
	// (for example the evidence commit retained after a failed declared
	// validation), oldest first.
	Commits []string `json:"commits,omitempty"`
	// Provenance says how the fingerprint was bound: "recorded" when the
	// blocked turn recorded it at block time (the consequence admitted is
	// exactly the one recorded), or "observed-at-recovery" for a turn that
	// predates consequence recording, where the checkout as found is bound.
	Provenance string `json:"provenance"`
}

const (
	RecoveryProvenanceRecorded = "recorded"
	RecoveryProvenanceObserved = "observed-at-recovery"
)

// BuildWorkerContext derives the worker context from the exact Goal
// generation and the selected candidate. It fails closed when the candidate
// is not part of the accepted plan.
func BuildWorkerContext(baseline *goals.GoalBaseline, candidates []contracts.WorkCandidate, relationships []contracts.WorkRelationship, objective string, repository WorkerRepositoryContext, granted []WorkerCapability, validationDeclared bool, declaredValidation string, recovery *WorkerRecoveryContext, envelope *WorkerExecutionEnvelope) (*WorkerContext, error) {
	if baseline == nil {
		return nil, errors.New("worker context requires the exact Goal generation")
	}
	var unit *contracts.WorkCandidate
	for i := range candidates {
		if candidates[i].ID == objective {
			unit = &candidates[i]
			break
		}
	}
	if unit == nil {
		return nil, fmt.Errorf("objective %q is not a candidate of the accepted WorkPlan", objective)
	}
	unitContext := WorkerUnitContext{ID: unit.ID, Requirements: append([]contracts.RequirementRef(nil), unit.Requirements...), Provenance: string(unit.Provenance), SourceRef: unit.SourceRef}
	for _, relationship := range relationships {
		if relationship.Dependent == unit.ID {
			unitContext.Prerequisites = append(unitContext.Prerequisites, relationship.Prerequisite)
		}
		if relationship.Prerequisite == unit.ID {
			unitContext.Dependents = append(unitContext.Dependents, relationship.Dependent)
		}
	}
	sort.Strings(unitContext.Prerequisites)
	sort.Strings(unitContext.Dependents)
	predicates := []string{
		"the repository working tree is clean when you finish (every intended change staged and committed, nothing left untracked or modified)",
		"exactly the bounded changes for this unit are committed locally on branch " + repository.Branch + " on top of " + repository.StartHead,
	}
	if validationDeclared {
		predicates = append(predicates, "the repository's declared validation ("+declaredValidation+") passes; Praxis runs it after your commit and refuses the checkpoint if it fails; when Praxis later invokes it with a contract reference argument (success_criteria/<n>, constraint/<n>, validity_predicates/<n>) it must print `"+ValidationAcknowledgement+" <ref>` and exit non-zero if that element does not hold, or print `"+ValidationAcknowledgement+" <ref> unhandled` when it does not verify that element; an unacknowledged run never counts as verification")
	} else {
		predicates = append(predicates, "if you introduce a test or validation toolchain, also declare it as an executable ./.praxis/validate script so Praxis can run it on every later checkpoint; invoked with no argument or `integrated` it runs the whole validation; invoked with a contract reference (success_criteria/<n>, constraint/<n>, validity_predicates/<n>) it must print `"+ValidationAcknowledgement+" <ref>` and exit non-zero if that element does not hold, or print `"+ValidationAcknowledgement+" <ref> unhandled`; an unacknowledged run never counts as verification of a bound element")
	}
	predicates = append(predicates, "when the committed work fully satisfies every requirement of this unit, add the line `"+CompletionTrailer+": "+objective+"` on its own line in the message of the final commit (any paragraph after the subject line; nothing else on that line); Praxis verifies the published checkpoint and its declared validation and only then records the unit complete, which makes dependent units eligible; omit the line when more turns are needed on this unit")
	if recovery != nil {
		predicates = append(predicates, "the recovered uncommitted consequence from turn "+recovery.RecoveredTurnID+" is validated and committed, corrected, or deliberately removed; none of it may remain uncommitted")
	}
	return &WorkerContext{
		Goal:       WorkerGoalContext{ID: baseline.ID, Version: baseline.Version, Digest: baseline.Digest, OriginalIntent: baseline.OriginalIntent, RefinedOutcome: baseline.RefinedOutcome, Scope: baseline.Scope, SuccessCriteria: append([]string(nil), baseline.SuccessCriteria...), Constraints: append([]string(nil), baseline.Constraints...), NonGoals: append([]string(nil), baseline.NonGoals...), ValidityPredicates: append([]string(nil), baseline.ValidityPredicates...)},
		Unit:       unitContext,
		Repository: repository,
		Checkpoint: WorkerCheckpointContext{Predicates: predicates, DeclaredValidation: declaredValidation, ValidationDeclared: validationDeclared},
		Authority:  WorkerAuthorityContext{Granted: granted, Forbidden: []string{"push or fetch", "rewrite or force-update any ref", "change branch", "modify Praxis durable state", "read or write outside the repository", "claim that a checkpoint is valid or that the Goal is complete", "select another objective"}, Envelope: envelope},
		Recovery:   recovery,
		Invariants: []string{"Praxis, not the provider, decides progress, checkpoint validity, publication, and the next invocation", "a worker receives the minimum authority its accepted unit needs and cannot expand it", "uncommitted work is evidence, never progress"},
	}, nil
}

// ConsequenceFingerprint binds the exact uncommitted state of a working
// tree: its porcelain status, the tracked diff, and the content of every
// untracked file. Two trees with the same fingerprint carry the same
// consequence.
func ConsequenceFingerprint(ctx context.Context, run func(context.Context, ...string) (string, error), readFile func(string) ([]byte, error), upstream string) (string, []string, []string, error) {
	status, err := run(ctx, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", nil, nil, err
	}
	diff, err := run(ctx, "diff", "--binary", "HEAD")
	if err != nil {
		return "", nil, nil, err
	}
	hasher := sha256.New()
	hasher.Write([]byte("status\n" + status + "\ndiff\n" + diff + "\n"))
	var files []string
	for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		files = append(files, path)
		if strings.HasPrefix(line, "??") {
			body, err := readFile(path)
			if err != nil {
				return "", nil, nil, fmt.Errorf("read untracked %s: %w", path, err)
			}
			sum := sha256.Sum256(body)
			hasher.Write([]byte("untracked\n" + path + "\n" + hex.EncodeToString(sum[:]) + "\n"))
		}
	}
	sort.Strings(files)
	var commits []string
	if upstream != "" {
		ahead, err := run(ctx, "rev-list", "--reverse", upstream+"..HEAD")
		if err != nil {
			return "", nil, nil, fmt.Errorf("list unpublished commits: %w", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(ahead), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				commits = append(commits, line)
				hasher.Write([]byte("commit\n" + line + "\n"))
			}
		}
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), files, commits, nil
}
