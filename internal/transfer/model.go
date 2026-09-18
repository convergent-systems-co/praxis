// Package transfer implements the governed lifecycle by which reusable,
// generalized knowledge crosses agent boundaries. It deliberately stores
// references rather than source memory content: packages interpret and
// sanitize domain material, while core preserves identity and provenance.
package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type SourceReference struct {
	AgentID       string `json:"agent_id"`
	GenerationID  string `json:"generation_id"`
	MemoryID      string `json:"memory_id"`
	ContentDigest string `json:"content_digest"`
	CausationRoot string `json:"causation_root"`
}

type TransferPolicy struct {
	ID                      string   `json:"id"`
	Version                 string   `json:"version"`
	PackageID               string   `json:"package_id"`
	SourceScope             string   `json:"source_scope"`
	TargetScope             string   `json:"target_scope"`
	GeneralizerID           string   `json:"generalizer_id"`
	GeneralizerVersion      string   `json:"generalizer_version"`
	SanitizerID             string   `json:"sanitizer_id"`
	SanitizerVersion        string   `json:"sanitizer_version"`
	EvaluatorID             string   `json:"evaluator_id"`
	EvaluatorVersion        string   `json:"evaluator_version"`
	RequiredPrivacyControls []string `json:"required_privacy_controls"`
	MinIndependentRoots     int      `json:"min_independent_roots"`
}

func FreezePolicy(policy TransferPolicy) (TransferPolicy, error) {
	policy.Version = transferPolicyVersions.CurrentVersion()
	policy.ID = ""
	policy.RequiredPrivacyControls = canonicalStrings(policy.RequiredPrivacyControls)
	if policy.PackageID == "" || policy.SourceScope == "" || policy.TargetScope == "" || policy.GeneralizerID == "" || policy.GeneralizerVersion == "" || policy.SanitizerID == "" || policy.SanitizerVersion == "" || policy.EvaluatorID == "" || policy.EvaluatorVersion == "" || len(policy.RequiredPrivacyControls) == 0 || policy.MinIndependentRoots < 1 {
		return TransferPolicy{}, errors.New("transfer policy requires package-owned scope, transforms, evaluator, privacy controls, and independent-root threshold")
	}
	policy.ID = digest("transfer-policy", policy)
	return policy, nil
}

func VerifyPolicy(policy TransferPolicy) error {
	id := policy.ID
	frozen, err := FreezePolicy(policy)
	if err != nil {
		return err
	}
	if id == "" || frozen.ID != id {
		return errors.New("transfer policy digest mismatch")
	}
	return nil
}

type Request struct {
	ID           string                 `json:"id"`
	Version      string                 `json:"version"`
	Proposer     contracts.PrincipalRef `json:"proposer"`
	SourceScope  string                 `json:"source_scope"`
	TargetScope  string                 `json:"target_scope"`
	ArtifactKind string                 `json:"artifact_kind"`
	Sources      []SourceReference      `json:"sources"`
	Policy       TransferPolicy         `json:"policy"`
	RequestedAt  time.Time              `json:"requested_at"`
}

func FreezeRequest(request Request) (Request, error) {
	request.Version = transferRequestVersions.CurrentVersion()
	request.ID = ""
	request.Sources = append([]SourceReference(nil), request.Sources...)
	sort.Slice(request.Sources, func(i, j int) bool {
		if request.Sources[i].AgentID != request.Sources[j].AgentID {
			return request.Sources[i].AgentID < request.Sources[j].AgentID
		}
		return request.Sources[i].MemoryID < request.Sources[j].MemoryID
	})
	if err := request.Proposer.Validate(); err != nil {
		return Request{}, err
	}
	if err := VerifyPolicy(request.Policy); err != nil {
		return Request{}, err
	}
	if request.SourceScope == "" || request.TargetScope == "" || request.ArtifactKind == "" || request.RequestedAt.IsZero() || len(request.Sources) == 0 {
		return Request{}, errors.New("transfer request requires scope, artifact kind, policy, sources, proposer, and time")
	}
	if request.SourceScope != request.Policy.SourceScope || request.TargetScope != request.Policy.TargetScope {
		return Request{}, errors.New("transfer request scopes do not match frozen policy")
	}
	seen := map[string]bool{}
	for _, source := range request.Sources {
		if source.AgentID == "" || source.GenerationID == "" || source.MemoryID == "" || source.ContentDigest == "" || source.CausationRoot == "" {
			return Request{}, errors.New("transfer source requires agent, generation, memory, content, and causation identity")
		}
		key := source.AgentID + "\x00" + source.MemoryID
		if seen[key] {
			return Request{}, errors.New("duplicate transfer source")
		}
		seen[key] = true
	}
	request.ID = digest("transfer-request", request)
	return request, nil
}

func VerifyRequest(request Request) error {
	id := request.ID
	frozen, err := FreezeRequest(request)
	if err != nil {
		return err
	}
	if id == "" || frozen.ID != id {
		return errors.New("transfer request digest mismatch")
	}
	return nil
}

type Artifact struct {
	ID                       string            `json:"id"`
	Version                  string            `json:"version"`
	RequestID                string            `json:"request_id"`
	Kind                     string            `json:"kind"`
	Scope                    string            `json:"scope"`
	ContentRef               string            `json:"content_ref"`
	ContentDigest            string            `json:"content_digest"`
	SourceRefs               []SourceReference `json:"source_refs"`
	GeneralizerID            string            `json:"generalizer_id"`
	GeneralizerVersion       string            `json:"generalizer_version"`
	SanitizerID              string            `json:"sanitizer_id"`
	SanitizerVersion         string            `json:"sanitizer_version"`
	SanitizationEvidenceRefs []string          `json:"sanitization_evidence_refs"`
	CreatedAt                time.Time         `json:"created_at"`
}

type InvariantResult struct {
	Class       string `json:"class"`
	ControlID   string `json:"control_id"`
	Passed      bool   `json:"passed"`
	EvidenceRef string `json:"evidence_ref"`
}

type Evaluation struct {
	ID               string            `json:"id"`
	Version          string            `json:"version"`
	ArtifactID       string            `json:"artifact_id"`
	EvaluatorID      string            `json:"evaluator_id"`
	EvaluatorVersion string            `json:"evaluator_version"`
	IndependentRoots []string          `json:"independent_roots"`
	Invariants       []InvariantResult `json:"invariants"`
	Accepted         bool              `json:"accepted"`
	EvaluatedAt      time.Time         `json:"evaluated_at"`
}

type Publication struct {
	ID           string                 `json:"id"`
	Version      string                 `json:"version"`
	ArtifactID   string                 `json:"artifact_id"`
	EvaluationID string                 `json:"evaluation_id"`
	Scope        string                 `json:"scope"`
	Authority    contracts.PrincipalRef `json:"authority"`
	AuthorityRef string                 `json:"authority_ref"`
	PublishedAt  time.Time              `json:"published_at"`
}

type Adoption struct {
	ID                   string                 `json:"id"`
	Version              string                 `json:"version"`
	PublicationID        string                 `json:"publication_id"`
	ArtifactID           string                 `json:"artifact_id"`
	TargetAgentID        string                 `json:"target_agent_id"`
	TargetGenerationID   string                 `json:"target_generation_id"`
	ReceivingRecordID    string                 `json:"receiving_record_id"`
	SourceAgentIDs       []string               `json:"source_agent_ids"`
	SourceGenerationIDs  []string               `json:"source_generation_ids"`
	SourceMemoryIDs      []string               `json:"source_memory_ids"`
	TransferMechanismRef string                 `json:"transfer_mechanism_ref"`
	Trust                contracts.TrustClass   `json:"trust"`
	Authority            contracts.PrincipalRef `json:"authority"`
	AuthorityRef         string                 `json:"authority_ref"`
	AdoptedAt            time.Time              `json:"adopted_at"`
}

func canonicalStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	if len(out) == 0 {
		return out
	}
	n := 1
	for i := 1; i < len(out); i++ {
		if out[i] != out[n-1] {
			out[n] = out[i]
			n++
		}
	}
	return out[:n]
}

func digest(prefix string, value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return prefix + ":sha256:" + hex.EncodeToString(sum[:])
}
