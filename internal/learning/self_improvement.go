package learning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/conformance"
)

type PlanningGeneration struct {
	ID                             string   `json:"id"`
	ParentID                       string   `json:"parent_id,omitempty"`
	Stages                         []string `json:"stages"`
	RequireOriginalGoalConformance bool     `json:"require_original_goal_conformance"`
	SourceFindingDigest            string   `json:"source_finding_digest,omitempty"`
}

func NewPlanningGeneration(parentID string, stages []string, requireConformance bool, sourceDigest string) (PlanningGeneration, error) {
	if len(stages) == 0 {
		return PlanningGeneration{}, errors.New("generation requires stages")
	}
	copyStages := append([]string(nil), stages...)
	payload := struct {
		ParentID string   `json:"parent_id"`
		Stages   []string `json:"stages"`
		Require  bool     `json:"require"`
		Source   string   `json:"source"`
	}{parentID, copyStages, requireConformance, sourceDigest}
	b, err := json.Marshal(payload)
	if err != nil {
		return PlanningGeneration{}, err
	}
	h := sha256.Sum256(b)
	return PlanningGeneration{ID: "sha256:" + hex.EncodeToString(h[:]), ParentID: parentID, Stages: copyStages, RequireOriginalGoalConformance: requireConformance, SourceFindingDigest: sourceDigest}, nil
}

// ProposePlanningGeneration responds to the systematic failure class (a plan
// proving itself) without inspecting or encoding any particular missing claim.
func ProposePlanningGeneration(candidate Candidate, active PlanningGeneration, frozen conformance.Result) (PlanningGeneration, error) {
	if err := candidate.Validate(); err != nil {
		return PlanningGeneration{}, err
	}
	if err := conformance.VerifyFrozen(frozen); err != nil {
		return PlanningGeneration{}, err
	}
	if candidate.Target != active.ID {
		return PlanningGeneration{}, errors.New("candidate target is not the active generation")
	}
	stages := append([]string(nil), active.Stages...)
	stages = append(stages, "derive-original-intent-denominator", "inventory-independent-evidence", "freeze-blind-conformance", "reconcile-plan-denominator")
	return NewPlanningGeneration(active.ID, stages, true, frozen.Digest)
}

type PlanningReplayScenario struct {
	ID                 string   `json:"id"`
	OriginalClaimIDs   []string `json:"original_claim_ids"`
	PlanClaimIDs       []string `json:"plan_claim_ids"`
	SupportedClaimIDs  []string `json:"supported_claim_ids"`
	SecurityViolations int      `json:"security_violations"`
	PolicyViolations   int      `json:"policy_violations"`
}

type PlanningReplayResult struct {
	ScenarioID         string   `json:"scenario_id"`
	CompletionAllowed  bool     `json:"completion_allowed"`
	DetectedGapIDs     []string `json:"detected_gap_ids"`
	MissedGapIDs       []string `json:"missed_gap_ids"`
	SecurityViolations int      `json:"security_violations"`
	PolicyViolations   int      `json:"policy_violations"`
}

func ReplayPlanning(generation PlanningGeneration, scenario PlanningReplayScenario) (PlanningReplayResult, error) {
	if generation.ID == "" || scenario.ID == "" || len(scenario.OriginalClaimIDs) == 0 {
		return PlanningReplayResult{}, errors.New("generation and scenario with original claims are required")
	}
	supported := set(scenario.SupportedClaimIDs)
	denominator := scenario.PlanClaimIDs
	if generation.RequireOriginalGoalConformance {
		denominator = scenario.OriginalClaimIDs
	}
	detected := difference(denominator, supported)
	missed := difference(difference(scenario.OriginalClaimIDs, supported), setSlice(detected))
	return PlanningReplayResult{ScenarioID: scenario.ID, CompletionAllowed: len(detected) == 0, DetectedGapIDs: detected, MissedGapIDs: missed, SecurityViolations: scenario.SecurityViolations, PolicyViolations: scenario.PolicyViolations}, nil
}

type GenerationEvaluation struct {
	ID                       string                 `json:"id"`
	ActiveGenerationID       string                 `json:"active_generation_id"`
	CandidateGenerationID    string                 `json:"candidate_generation_id"`
	FrozenFindingDigest      string                 `json:"frozen_finding_digest"`
	ActiveResults            []PlanningReplayResult `json:"active_results"`
	CandidateResults         []PlanningReplayResult `json:"candidate_results"`
	IndependentEvidenceRoots []string               `json:"independent_evidence_roots"`
	ImprovedCoverage         bool                   `json:"improved_coverage"`
	RegressionCount          int                    `json:"regression_count"`
	SecurityViolations       int                    `json:"security_violations"`
	PolicyViolations         int                    `json:"policy_violations"`
	Passed                   bool                   `json:"passed"`
}

func EvaluatePlanningCandidate(active, candidate PlanningGeneration, frozen conformance.Result, scenarios []PlanningReplayScenario, evidence []Evidence) (GenerationEvaluation, error) {
	if candidate.ParentID != active.ID || candidate.ID == active.ID {
		return GenerationEvaluation{}, errors.New("candidate must be a distinct child generation")
	}
	if err := conformance.VerifyFrozen(frozen); err != nil {
		return GenerationEvaluation{}, err
	}
	roots, err := IndependentRoots(evidence)
	if err != nil {
		return GenerationEvaluation{}, err
	}
	if len(scenarios) < 2 {
		return GenerationEvaluation{}, errors.New("original replay and regression corpus are required")
	}
	ev := GenerationEvaluation{ActiveGenerationID: active.ID, CandidateGenerationID: candidate.ID, FrozenFindingDigest: frozen.Digest, IndependentEvidenceRoots: roots}
	activeMissed, candidateMissed := 0, 0
	for _, scenario := range scenarios {
		a, err := ReplayPlanning(active, scenario)
		if err != nil {
			return GenerationEvaluation{}, err
		}
		c, err := ReplayPlanning(candidate, scenario)
		if err != nil {
			return GenerationEvaluation{}, err
		}
		ev.ActiveResults = append(ev.ActiveResults, a)
		ev.CandidateResults = append(ev.CandidateResults, c)
		activeMissed += len(a.MissedGapIDs)
		candidateMissed += len(c.MissedGapIDs)
		if a.CompletionAllowed && !c.CompletionAllowed && len(c.DetectedGapIDs) == 0 {
			ev.RegressionCount++
		}
		if !a.CompletionAllowed && c.CompletionAllowed {
			ev.RegressionCount++
		}
		ev.SecurityViolations += c.SecurityViolations
		ev.PolicyViolations += c.PolicyViolations
	}
	ev.ImprovedCoverage = candidateMissed < activeMissed
	ev.Passed = ev.ImprovedCoverage && ev.RegressionCount == 0 && ev.SecurityViolations == 0 && ev.PolicyViolations == 0 && len(roots) >= 2
	payload := ev
	payload.ID = ""
	b, _ := json.Marshal(payload)
	h := sha256.Sum256(b)
	ev.ID = "sha256:" + hex.EncodeToString(h[:])
	return ev, nil
}

type GovernanceDecision struct {
	AuthorityID  string    `json:"authority_id"`
	Approved     bool      `json:"approved"`
	EvaluationID string    `json:"evaluation_id"`
	DecidedAt    time.Time `json:"decided_at"`
}
type GenerationRegistry struct {
	generations map[string]PlanningGeneration
	activeID    string
	rollbackID  string
	path        string
}

func NewGenerationRegistry(active PlanningGeneration) *GenerationRegistry {
	return &GenerationRegistry{generations: map[string]PlanningGeneration{active.ID: active}, activeID: active.ID}
}
func OpenGenerationRegistry(path string, seed PlanningGeneration) (*GenerationRegistry, error) {
	if path == "" {
		return nil, errors.New("generation registry path is required")
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		r := NewGenerationRegistry(seed)
		r.path = path
		if err := r.persist(); err != nil {
			return nil, err
		}
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot generationSnapshot
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return nil, fmt.Errorf("decode generation registry: %w", err)
	}
	if len(snapshot.Generations) == 0 || snapshot.Generations[snapshot.ActiveID].ID == "" {
		return nil, errors.New("generation registry has no valid active generation")
	}
	for id, generation := range snapshot.Generations {
		expected, err := NewPlanningGeneration(generation.ParentID, generation.Stages, generation.RequireOriginalGoalConformance, generation.SourceFindingDigest)
		if err != nil || expected.ID != id || generation.ID != id {
			return nil, fmt.Errorf("generation %s failed immutable identity validation", id)
		}
	}
	return &GenerationRegistry{generations: snapshot.Generations, activeID: snapshot.ActiveID, rollbackID: snapshot.RollbackID, path: path}, nil
}
func (r *GenerationRegistry) Register(g PlanningGeneration) error {
	if g.ID == "" {
		return errors.New("generation id required")
	}
	expected, err := NewPlanningGeneration(g.ParentID, g.Stages, g.RequireOriginalGoalConformance, g.SourceFindingDigest)
	if err != nil || expected.ID != g.ID {
		return errors.New("generation content does not match immutable identity")
	}
	if existing, ok := r.generations[g.ID]; ok {
		existingJSON, _ := json.Marshal(existing)
		generationJSON, _ := json.Marshal(g)
		if string(existingJSON) != string(generationJSON) {
			return errors.New("generation identity collision")
		}
		return nil
	}
	if g.ParentID == "" || r.generations[g.ParentID].ID == "" {
		return errors.New("candidate parent generation is unavailable")
	}
	r.generations[g.ID] = g
	return r.persist()
}
func (r *GenerationRegistry) Promote(candidate PlanningGeneration, evaluation GenerationEvaluation, decision GovernanceDecision, proposerID string) error {
	if decision.AuthorityID == "" || decision.AuthorityID == proposerID {
		return errors.New("candidate cannot promote itself")
	}
	if !decision.Approved || decision.DecidedAt.IsZero() || decision.EvaluationID != evaluation.ID {
		return errors.New("independent governed approval bound to evaluation is required")
	}
	if !evaluation.Passed || evaluation.CandidateGenerationID != candidate.ID || evaluation.ActiveGenerationID != r.activeID {
		return errors.New("candidate evaluation did not pass for current active generation")
	}
	if err := r.Register(candidate); err != nil {
		return err
	}
	r.rollbackID = r.activeID
	r.activeID = candidate.ID
	return r.persist()
}
func (r *GenerationRegistry) Rollback() error {
	if r.rollbackID == "" {
		return errors.New("rollback generation is unavailable")
	}
	current := r.activeID
	r.activeID = r.rollbackID
	r.rollbackID = current
	return r.persist()
}
func (r *GenerationRegistry) Active() PlanningGeneration   { return r.generations[r.activeID] }
func (r *GenerationRegistry) RollbackIdentity() string     { return r.rollbackID }
func (r *GenerationRegistry) HasGeneration(id string) bool { _, ok := r.generations[id]; return ok }

type generationSnapshot struct {
	Generations map[string]PlanningGeneration `json:"generations"`
	ActiveID    string                        `json:"active_id"`
	RollbackID  string                        `json:"rollback_id"`
}

func (r *GenerationRegistry) persist() error {
	if r.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(generationSnapshot{Generations: r.generations, ActiveID: r.activeID, RollbackID: r.rollbackID}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

func set(values []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}
func setSlice(values []string) map[string]bool { return set(values) }
func difference(values []string, exclude map[string]bool) []string {
	var out []string
	for _, v := range values {
		if !exclude[v] {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func (e GenerationEvaluation) Validate() error {
	if e.ID == "" || e.ActiveGenerationID == "" || e.CandidateGenerationID == "" {
		return fmt.Errorf("evaluation identity is incomplete")
	}
	return nil
}
