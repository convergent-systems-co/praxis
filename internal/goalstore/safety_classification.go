package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const goalSafetyClassificationNamespace = "goal_safety_classification"

// ErrSafetyDowngrade is returned when an object attempts to enter, or drive,
// a Goal that the durable record classifies as safety-bearing without carrying
// the safety binding. It is the persistence-side half of downgrade resistance
// (I9): the safety semantics of a Goal generation never depend solely on a
// field inside the plan being admitted.
var ErrSafetyDowngrade = errors.New("Goal is classified safety-bearing; a plan without the safety binding is a downgrade")

// goalSafetyClassification is the authenticated, immutable fact that a Goal
// identity has been put under the safety kernel. It is written, sealed under
// the installation storage key like every other GoalStore record, the first
// time a safety-bearing proposal for that Goal is persisted, and is never
// removed or rewritten. Classification is monotone: once safety-bearing, every
// WorkPlan of every generation of that Goal must carry the same kernel binding.
type goalSafetyClassification struct {
	GoalID        string `json:"goal_id"`
	KernelVersion string `json:"kernel_version"`
}

// GoalSafetyKernel reports the kernel version a Goal identity is classified
// under, if any. A store failure other than "not found" is an error, never
// "unclassified", so a fault cannot downgrade.
//
// Classification is monotone under loss and rollback (I12, I13, I14):
//
//  1. An anchored classification FACT in the governance fact chain, which the
//     forward authority anchor pins outside the store, is authoritative. Erasing
//     the classification row, its proposal, its generations and every other
//     record cannot remove it, and a store whose chain has been truncated or
//     rolled back is refused before this is consulted.
//  2. The sealed classification row.
//  3. Otherwise the classification is DERIVED from every surviving
//     authenticated record that implies the Goal entered the governed safety
//     domain (see goalEvidenceExtractors), so that an unanchored store, or one
//     whose facts were lost in a governed recovery, still cannot be turned
//     legacy by deleting a chosen subset of rows.
func (r Repository) GoalSafetyKernel(ctx context.Context, goalID string) (string, bool, error) {
	if goalID == "" {
		return "", false, errors.New("Goal identity is required to resolve its safety classification")
	}
	if r.anchorEnabled() {
		snap, err := r.governanceSnapshot(ctx)
		if err != nil {
			return "", false, err
		}
		if kernel, ok := snap.View.Classified[goalID]; ok {
			if kernel == "" {
				kernel = contracts.WorkPlanSafetyKernelVersion
			}
			return kernel, true, nil
		}
	}
	payload, _, err := r.loadWorkPlanBlob(ctx, goalSafetyClassificationNamespace, goalID, "1", time.Now().UTC())
	if err != nil {
		if errors.Is(err, state.ErrSecureBlobNotFound) {
			return r.deriveGoalSafetyKernel(ctx, goalID)
		}
		return "", false, fmt.Errorf("resolve Goal safety classification: %w", err)
	}
	var stored goalSafetyClassification
	if err := contracts.UnmarshalExactJSON(payload, &stored, true); err != nil {
		return "", false, fmt.Errorf("decode Goal safety classification: %w", err)
	}
	if stored.GoalID != goalID || stored.KernelVersion == "" {
		return "", false, errors.New("Goal safety classification identity is inconsistent")
	}
	return stored.KernelVersion, true, nil
}

// goalEvidence describes how one kind of authenticated record evidences the
// governed status of a Goal.
type goalEvidence struct {
	// Why names the record's role in the governance lineage.
	Why string
	// Filter narrows the records worth opening for a Goal without decrypting
	// them (nil opens every record of the namespace).
	Filter func(record state.SecureBlobRecord, goalID string) bool
	// Extract returns the binding the record implies for the Goal, or nil.
	Extract func(r Repository, record state.SecureBlobRecord, payload []byte, goalID string) (*contracts.WorkPlanSafetyBinding, error)
}

type evidenceView struct {
	GoalID          string                           `json:"goal_id"`
	Safety          *contracts.WorkPlanSafetyBinding `json:"safety"`
	BaselineID      string                           `json:"baseline_id"`
	CeremonyProfile string                           `json:"ceremony_profile"`
}

type evidenceEnvelope struct {
	evidenceView
	Proposal *evidenceView `json:"proposal"`
	Request  *evidenceView `json:"request"`
	Plan     *evidenceView `json:"plan"`
}

// kernelRequest reports whether an authority request of this Goal is a
// protected (ceremony-profile) request; frozen plan section 9 makes the
// profile mandatory for kernel requests, so its presence is positive evidence
// that the Goal was put under the kernel.
func kernelRequest(v *evidenceView, goalID string) bool {
	return v != nil && v.BaselineID == goalID && v.CeremonyProfile != ""
}

var kernelBinding = &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion}

func envelopeExtract(r Repository, record state.SecureBlobRecord, payload []byte, goalID string) (*contracts.WorkPlanSafetyBinding, error) {
	var e evidenceEnvelope
	if err := json.Unmarshal(payload, &e); err != nil {
		return nil, err
	}
	var found *contracts.WorkPlanSafetyBinding
	take := func(b *contracts.WorkPlanSafetyBinding) error {
		if b == nil {
			return nil
		}
		if found != nil && found.KernelVersion != b.KernelVersion {
			return fmt.Errorf("%w: Goal %s carries conflicting kernel bindings %q and %q", ErrSafetyDowngrade, goalID, found.KernelVersion, b.KernelVersion)
		}
		found = b
		return nil
	}
	if e.GoalID == goalID {
		if err := take(e.Safety); err != nil {
			return nil, err
		}
	}
	if e.Proposal != nil && e.Proposal.GoalID == goalID {
		if err := take(e.Proposal.Safety); err != nil {
			return nil, err
		}
		if e.Plan != nil {
			if err := take(e.Plan.Safety); err != nil {
				return nil, err
			}
		}
	}
	if kernelRequest(&e.evidenceView, goalID) || kernelRequest(e.Request, goalID) {
		if err := take(kernelBinding); err != nil {
			return nil, err
		}
	}
	return found, nil
}

// goalEvidenceExtractors is the complete registry of record kinds that evidence
// a Goal's governed status. A record kind that is not listed here must be
// listed in nonEvidenceNamespaces with a reason; a test enumerates the
// namespaces the full lifecycle creates and fails on any that is neither, so
// classification cannot silently ignore a new kind of governance evidence.
var goalEvidenceExtractors = map[string]goalEvidence{
	baselineNamespace: {
		Why:    "an attached Goal generation carries the accepted plan and its safety binding",
		Filter: func(record state.SecureBlobRecord, goalID string) bool { return record.ObjectID == goalID },
		Extract: func(r Repository, record state.SecureBlobRecord, payload []byte, goalID string) (*contracts.WorkPlanSafetyBinding, error) {
			baseline, err := r.Load(context.Background(), record.ObjectID, record.ObjectVersion, time.Unix(0, 0).UTC())
			if err != nil {
				return nil, err
			}
			if baseline.WorkPlan == nil {
				return nil, nil
			}
			return baseline.WorkPlan.Safety, nil
		},
	},
	workPlanProposalNamespace:   {Why: "a safety-bearing proposal names its Goal and carries the binding", Extract: envelopeExtract},
	workPlanReviewNamespace:     {Why: "a review embeds the exact proposal it reviewed", Extract: envelopeExtract},
	workPlanAcceptanceNamespace: {Why: "an acceptance embeds the proposal and the accepted plan with their bindings", Extract: envelopeExtract},
	authorityRequestNamespace:   {Why: "a protected request names the Goal generation it governs", Extract: envelopeExtract},
	authorityDecisionNamespace:  {Why: "a decision embeds its protected request", Extract: envelopeExtract},
	completionSealNamespace: {
		Why: "a completion seal exists only for a safety-bearing generation; its object id is goal/version",
		Filter: func(record state.SecureBlobRecord, goalID string) bool {
			return strings.HasPrefix(record.ObjectID, goalID+"/")
		},
		Extract: func(Repository, state.SecureBlobRecord, []byte, string) (*contracts.WorkPlanSafetyBinding, error) {
			return kernelBinding, nil
		},
	},
}

// nonEvidenceNamespaces are governance namespaces that carry no Goal
// classification evidence, each with the reason. Together with
// goalEvidenceExtractors this must cover every namespace the lifecycle creates
// (enforced by a test).
var nonEvidenceNamespaces = map[string]string{
	goalSafetyClassificationNamespace:        "the classification row is read directly",
	state.GovernanceFactNamespace:            "the anchored fact chain is read directly",
	governanceOrphanNamespace:                "preserved provenance, never consulted",
	sessionNamespace:                         "a session is not governance evidence",
	authorityRevocationNamespace:             "a negative record; permission-narrowing only",
	authorityGenerationNamespace:             "an authority generation names no Goal",
	authorityGenerationInvalidationNamespace: "a negative record; permission-narrowing only",
	state.AuthorityDecisionLiveNamespace:     "authority liveness names no Goal",
	state.AuthorityGenerationLiveNamespace:   "authority liveness names no Goal",
	ownerCeremonyNamespace:                   "ceremony evidence names the owner, not a Goal",
	routingIssuanceNamespace:                 "routing authority is not Goal governance",
	providerWorkspaceNamespace:               "a provider workspace is not governance evidence",
	publisherGovernanceNamespace:             "publisher governance is not Goal governance",
}

// GoalEvidenceNamespaces returns the registry keys, for the coverage test.
func GoalEvidenceNamespaces() (evidence, nonEvidence []string) {
	for k := range goalEvidenceExtractors {
		evidence = append(evidence, k)
	}
	for k := range nonEvidenceNamespaces {
		nonEvidence = append(nonEvidence, k)
	}
	return evidence, nonEvidence
}

// deriveGoalSafetyKernel re-derives the classification from every surviving
// authenticated record that evidences the Goal's governed status. Any
// undecryptable or mismatched candidate is an error: a fault must not read as
// "never classified". Expired rows are still evidence, so the scan uses the
// epoch as its clock.
func (r Repository) deriveGoalSafetyKernel(ctx context.Context, goalID string) (string, bool, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", false, err
	}
	epoch := time.Unix(0, 0).UTC()
	kernel := ""
	for namespace, evidence := range goalEvidenceExtractors {
		records, err := r.Store.ListSecureBlobs(ctx, namespace, epoch)
		if err != nil {
			return "", false, fmt.Errorf("derive Goal safety classification: %w", err)
		}
		for _, record := range records {
			if evidence.Filter != nil && !evidence.Filter(record, goalID) {
				continue
			}
			// Attribution needs only the sealed fields, so a record the
			// current contract no longer validates (an unrelated historical
			// record) is still evidence for, or irrelevant to, this Goal. A row
			// that cannot be authenticated and decrypted is an error: it might
			// be this Goal's evidence.
			payload, _, err := r.loadWorkPlanBlob(ctx, namespace, record.ObjectID, record.ObjectVersion, epoch)
			if err != nil {
				return "", false, fmt.Errorf("derive Goal safety classification from %s %s/%s: %w", namespace, record.ObjectID, record.ObjectVersion, err)
			}
			binding, err := evidence.Extract(r, record, payload, goalID)
			if err != nil {
				return "", false, fmt.Errorf("derive Goal safety classification from %s %s/%s: %w", namespace, record.ObjectID, record.ObjectVersion, err)
			}
			if binding == nil {
				continue
			}
			if kernel != "" && kernel != binding.KernelVersion {
				return "", false, fmt.Errorf("%w: Goal %s carries conflicting kernel bindings %q and %q", ErrSafetyDowngrade, goalID, kernel, binding.KernelVersion)
			}
			kernel = binding.KernelVersion
		}
	}
	return kernel, kernel != "", nil
}

// GoalSafetyClassified is the boolean form consumed by the Goal-drive
// controller.
func (r Repository) GoalSafetyClassified(ctx context.Context, goalID string) (bool, error) {
	_, classified, err := r.GoalSafetyKernel(ctx, goalID)
	return classified, err
}

// markGoalSafetyBearing records the classification idempotently. It runs
// before the proposal it protects is persisted, so the existence of any
// safety-bearing proposal implies the classification exists.
func (r Repository) markGoalSafetyBearing(ctx context.Context, goalID string, binding *contracts.WorkPlanSafetyBinding, now time.Time) error {
	if binding == nil {
		return nil
	}
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	kernel, classified, err := r.GoalSafetyKernel(ctx, goalID)
	if err != nil {
		return err
	}
	if classified {
		if kernel != binding.KernelVersion {
			return fmt.Errorf("%w: Goal %s is classified under kernel %q, not %q", ErrSafetyDowngrade, goalID, kernel, binding.KernelVersion)
		}
		// Classified by a row or by derivation: make sure the anchored fact
		// exists too (idempotent; restrictive only), so it survives loss of the
		// evidence it was derived from.
		return r.anchorGoalClassified(ctx, goalID, binding.KernelVersion, now)
	}
	// I14: the classification is anchored as a fact BEFORE the row and before the
	// proposal it protects; erasing every row later cannot un-govern the Goal.
	if err := r.anchorGoalClassified(ctx, goalID, binding.KernelVersion, now); err != nil {
		return err
	}
	payload, err := json.Marshal(goalSafetyClassification{GoalID: goalID, KernelVersion: binding.KernelVersion})
	if err != nil {
		return err
	}
	if err := r.putWorkPlanBlob(ctx, goalSafetyClassificationNamespace, goalID, "1", payload, now, nil); err != nil {
		// A concurrent writer of the same immutable fact is not a failure.
		if _, again, readErr := r.GoalSafetyKernel(ctx, goalID); readErr == nil && again {
			return nil
		}
		return fmt.Errorf("persist Goal safety classification: %w", err)
	}
	return nil
}

// requireSafetyConsistent refuses any admission of a plan-bearing object for a
// Goal whose classification and whose own safety binding disagree. binding may
// be nil (a legacy artifact); it is refused when the Goal is classified.
func (r Repository) requireSafetyConsistent(ctx context.Context, goalID string, binding *contracts.WorkPlanSafetyBinding) error {
	kernel, classified, err := r.GoalSafetyKernel(ctx, goalID)
	if err != nil {
		return err
	}
	if !classified {
		return nil
	}
	if binding == nil {
		return fmt.Errorf("%w: Goal %s (kernel %s)", ErrSafetyDowngrade, goalID, kernel)
	}
	if binding.KernelVersion != kernel {
		return fmt.Errorf("%w: Goal %s is classified under kernel %q, not %q", ErrSafetyDowngrade, goalID, kernel, binding.KernelVersion)
	}
	return nil
}
