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
	"strings"
)

type ReasoningTier string

const (
	TierD2 ReasoningTier = "D2"
	TierD1 ReasoningTier = "D1"
	TierD0 ReasoningTier = "D0"
)

type AdvisoryInstruction struct {
	ID           string        `json:"id"`
	Text         string        `json:"text"`
	Tier         ReasoningTier `json:"tier"`
	Learnable    bool          `json:"learnable"`
	Active       bool          `json:"active"`
	SupersededBy string        `json:"superseded_by,omitempty"`
}

type DeterministicRule struct {
	ID            string `json:"id"`
	InstructionID string `json:"instruction_id"`
	Mechanism     string `json:"mechanism"`
}

type BehaviorGeneration struct {
	ID                string                `json:"id"`
	ParentID          string                `json:"parent_id,omitempty"`
	Instructions      []AdvisoryInstruction `json:"instructions"`
	Rules             []DeterministicRule   `json:"rules,omitempty"`
	SourceEvidenceIDs []string              `json:"source_evidence_ids,omitempty"`
}

func NewBehaviorGeneration(parentID string, instructions []AdvisoryInstruction, rules []DeterministicRule, evidenceIDs []string) (BehaviorGeneration, error) {
	generation := BehaviorGeneration{
		ParentID:          parentID,
		Instructions:      append([]AdvisoryInstruction(nil), instructions...),
		Rules:             append([]DeterministicRule(nil), rules...),
		SourceEvidenceIDs: append([]string(nil), evidenceIDs...),
	}
	sort.Slice(generation.Instructions, func(i, j int) bool { return generation.Instructions[i].ID < generation.Instructions[j].ID })
	sort.Slice(generation.Rules, func(i, j int) bool { return generation.Rules[i].ID < generation.Rules[j].ID })
	sort.Strings(generation.SourceEvidenceIDs)
	if err := validateBehaviorGeneration(generation); err != nil {
		return BehaviorGeneration{}, err
	}
	id, err := behaviorGenerationID(generation)
	if err != nil {
		return BehaviorGeneration{}, err
	}
	generation.ID = id
	return generation, nil
}

func validateBehaviorGeneration(generation BehaviorGeneration) error {
	if len(generation.Instructions) == 0 {
		return errors.New("behavior generation requires instructions")
	}
	instructions := map[string]AdvisoryInstruction{}
	for _, instruction := range generation.Instructions {
		if instruction.ID == "" || instruction.Text == "" || instruction.Tier == "" {
			return errors.New("instruction identity, text, and tier are required")
		}
		if _, duplicate := instructions[instruction.ID]; duplicate {
			return errors.New("duplicate instruction identity")
		}
		instructions[instruction.ID] = instruction
	}
	rules := map[string]bool{}
	for _, rule := range generation.Rules {
		if rule.ID == "" || rule.InstructionID == "" || !knownMechanism(rule.Mechanism) {
			return errors.New("rule identity, instruction, and known deterministic mechanism are required")
		}
		if rules[rule.ID] || instructions[rule.InstructionID].ID == "" {
			return errors.New("rule is duplicate or refers to an unknown instruction")
		}
		rules[rule.ID] = true
	}
	for _, instruction := range generation.Instructions {
		if instruction.SupersededBy != "" && !rules[instruction.SupersededBy] {
			return errors.New("instruction supersession does not name a generation rule")
		}
		if instruction.Active && instruction.SupersededBy != "" {
			return errors.New("superseded instruction cannot remain active")
		}
	}
	return nil
}

func VerifyBehaviorGeneration(generation BehaviorGeneration) error {
	if generation.ID == "" {
		return errors.New("behavior generation identity is required")
	}
	if err := validateBehaviorGeneration(generation); err != nil {
		return err
	}
	want, err := behaviorGenerationID(generation)
	if err != nil {
		return err
	}
	if generation.ID != want {
		return errors.New("behavior generation content does not match immutable identity")
	}
	return nil
}

func behaviorGenerationID(generation BehaviorGeneration) (string, error) {
	generation.ID = ""
	digest, err := digestBehaviorValue(generation)
	if err != nil {
		return "", err
	}
	return "sha256:" + digest, nil
}

type ExecutionObservation struct {
	ID               string `json:"id"`
	InstructionID    string `json:"instruction_id"`
	GoalClass        string `json:"goal_class"`
	Context          string `json:"context"`
	Input            string `json:"input"`
	Output           string `json:"output"`
	Successful       bool   `json:"successful"`
	PolicyViolations int    `json:"policy_violations"`
	TrustClass       string `json:"trust_class"`
	CausationRoot    string `json:"causation_root"`
	InferenceTokens  int    `json:"inference_tokens"`
}

// LearnDeterministicCandidate recognizes behavior rather than prose. It tries
// a fixed, reviewable mechanism set against trusted successful observations;
// no observation or model response can supply executable code or authority.
func LearnDeterministicCandidate(active BehaviorGeneration, instructionID string, observations []ExecutionObservation, minimumIndependentRoots int) (BehaviorGeneration, error) {
	if err := VerifyBehaviorGeneration(active); err != nil {
		return BehaviorGeneration{}, err
	}
	if minimumIndependentRoots <= 0 {
		return BehaviorGeneration{}, errors.New("positive independent evidence threshold is required")
	}
	instructionIndex := -1
	for index, instruction := range active.Instructions {
		if instruction.ID == instructionID {
			instructionIndex = index
			if !instruction.Active || !instruction.Learnable {
				return BehaviorGeneration{}, errors.New("instruction is not active and learnable")
			}
		}
	}
	if instructionIndex < 0 {
		return BehaviorGeneration{}, errors.New("instruction is not in the active generation")
	}
	trusted := make([]ExecutionObservation, 0, len(observations))
	roots := map[string]bool{}
	evidenceIDs := []string{}
	for _, observation := range observations {
		if observation.ID == "" || observation.CausationRoot == "" || observation.InstructionID != instructionID || observation.GoalClass == "" || observation.Context == "" || observation.InferenceTokens < 0 {
			return BehaviorGeneration{}, errors.New("observation identity, scope, ancestry, and valid metrics are required")
		}
		if observation.TrustClass != "runtime_verified" && observation.TrustClass != "user_confirmed" {
			continue
		}
		if !observation.Successful || observation.PolicyViolations != 0 {
			continue
		}
		trusted = append(trusted, observation)
		if !roots[observation.CausationRoot] {
			roots[observation.CausationRoot] = true
			evidenceIDs = append(evidenceIDs, observation.ID)
		}
	}
	if len(roots) < minimumIndependentRoots {
		return BehaviorGeneration{}, errors.New("insufficient independent trusted successful observations")
	}
	mechanism := inferMechanism(trusted)
	if mechanism == "" {
		return BehaviorGeneration{}, errors.New("no deterministic representation matches observed behavior")
	}
	ruleSeed := struct {
		Parent        string
		InstructionID string
		Mechanism     string
	}{active.ID, instructionID, mechanism}
	ruleDigest, err := digestBehaviorValue(ruleSeed)
	if err != nil {
		return BehaviorGeneration{}, err
	}
	rule := DeterministicRule{ID: "rule:sha256:" + ruleDigest, InstructionID: instructionID, Mechanism: mechanism}
	instructions := append([]AdvisoryInstruction(nil), active.Instructions...)
	instructions[instructionIndex].Active = false
	instructions[instructionIndex].SupersededBy = rule.ID
	rules := append(append([]DeterministicRule(nil), active.Rules...), rule)
	return NewBehaviorGeneration(active.ID, instructions, rules, evidenceIDs)
}

var deterministicMechanisms = []string{"identity", "trim_space", "lowercase", "uppercase", "trim_lowercase", "trim_uppercase"}

func inferMechanism(observations []ExecutionObservation) string {
	for _, mechanism := range deterministicMechanisms {
		matches := true
		for _, observation := range observations {
			output, err := applyMechanism(mechanism, observation.Input)
			if err != nil || output != observation.Output {
				matches = false
				break
			}
		}
		if matches {
			return mechanism
		}
	}
	return ""
}

func knownMechanism(mechanism string) bool {
	for _, known := range deterministicMechanisms {
		if mechanism == known {
			return true
		}
	}
	return false
}

func applyMechanism(mechanism, input string) (string, error) {
	switch mechanism {
	case "identity":
		return input, nil
	case "trim_space":
		return strings.TrimSpace(input), nil
	case "lowercase":
		return strings.ToLower(input), nil
	case "uppercase":
		return strings.ToUpper(input), nil
	case "trim_lowercase":
		return strings.ToLower(strings.TrimSpace(input)), nil
	case "trim_uppercase":
		return strings.ToUpper(strings.TrimSpace(input)), nil
	default:
		return "", errors.New("unknown deterministic mechanism")
	}
}

func ExecuteDeterministic(generation BehaviorGeneration, instructionID, input string) (string, error) {
	if err := VerifyBehaviorGeneration(generation); err != nil {
		return "", err
	}
	for _, rule := range generation.Rules {
		if rule.InstructionID == instructionID {
			return applyMechanism(rule.Mechanism, input)
		}
	}
	return "", errors.New("generation still requires bounded inference for instruction")
}

func ActivePrompt(generation BehaviorGeneration) ([]AdvisoryInstruction, error) {
	if err := VerifyBehaviorGeneration(generation); err != nil {
		return nil, err
	}
	active := []AdvisoryInstruction{}
	for _, instruction := range generation.Instructions {
		if instruction.Active {
			active = append(active, instruction)
		}
	}
	return active, nil
}

type BehaviorReplayCase struct {
	ID                 string `json:"id"`
	Input              string `json:"input"`
	RequiredOutput     string `json:"required_output"`
	ActiveOutput       string `json:"active_output"`
	ActiveTokens       int    `json:"active_tokens"`
	SecurityViolations int    `json:"security_violations"`
	PolicyViolations   int    `json:"policy_violations"`
}

type BehaviorEvaluation struct {
	ID                       string   `json:"id"`
	ActiveGenerationID       string   `json:"active_generation_id"`
	CandidateGenerationID    string   `json:"candidate_generation_id"`
	InstructionID            string   `json:"instruction_id"`
	IndependentEvidenceRoots []string `json:"independent_evidence_roots"`
	RegressionCount          int      `json:"regression_count"`
	SecurityViolations       int      `json:"security_violations"`
	PolicyViolations         int      `json:"policy_violations"`
	ActiveTokens             int      `json:"active_tokens"`
	CandidateTokens          int      `json:"candidate_tokens"`
	Passed                   bool     `json:"passed"`
}

func VerifyBehaviorEvaluation(evaluation BehaviorEvaluation) error {
	if evaluation.ID == "" || evaluation.ActiveGenerationID == "" || evaluation.CandidateGenerationID == "" || evaluation.InstructionID == "" || len(evaluation.IndependentEvidenceRoots) == 0 {
		return errors.New("behavior evaluation identity and evidence are incomplete")
	}
	payload := evaluation
	payload.ID = ""
	digest, err := digestBehaviorValue(payload)
	if err != nil {
		return err
	}
	if evaluation.ID != "sha256:"+digest {
		return errors.New("behavior evaluation digest mismatch")
	}
	return nil
}

func EvaluateBehaviorCandidate(active, candidate BehaviorGeneration, instructionID string, observations []ExecutionObservation, cases []BehaviorReplayCase, minimumIndependentRoots int) (BehaviorEvaluation, error) {
	if err := VerifyBehaviorGeneration(active); err != nil {
		return BehaviorEvaluation{}, err
	}
	if err := VerifyBehaviorGeneration(candidate); err != nil {
		return BehaviorEvaluation{}, err
	}
	if candidate.ParentID != active.ID || candidate.ID == active.ID || len(cases) < 2 || minimumIndependentRoots <= 0 {
		return BehaviorEvaluation{}, errors.New("distinct child candidate plus original and regression replay cases are required")
	}
	rootsMap := map[string]bool{}
	for _, observation := range observations {
		if (observation.TrustClass == "runtime_verified" || observation.TrustClass == "user_confirmed") && observation.Successful && observation.PolicyViolations == 0 && observation.CausationRoot != "" {
			rootsMap[observation.CausationRoot] = true
		}
	}
	roots := make([]string, 0, len(rootsMap))
	for root := range rootsMap {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	evaluation := BehaviorEvaluation{ActiveGenerationID: active.ID, CandidateGenerationID: candidate.ID, InstructionID: instructionID, IndependentEvidenceRoots: roots}
	for _, replay := range cases {
		if replay.ID == "" || replay.ActiveTokens < 0 {
			return BehaviorEvaluation{}, errors.New("replay identity and valid metrics are required")
		}
		candidateOutput, err := ExecuteDeterministic(candidate, instructionID, replay.Input)
		if err != nil {
			return BehaviorEvaluation{}, err
		}
		if candidateOutput != replay.RequiredOutput {
			evaluation.RegressionCount++
		}
		evaluation.SecurityViolations += replay.SecurityViolations
		evaluation.PolicyViolations += replay.PolicyViolations
		evaluation.ActiveTokens += replay.ActiveTokens
	}
	evaluation.Passed = len(roots) >= minimumIndependentRoots && evaluation.RegressionCount == 0 && evaluation.SecurityViolations == 0 && evaluation.PolicyViolations == 0 && evaluation.CandidateTokens < evaluation.ActiveTokens
	payload := evaluation
	payload.ID = ""
	digest, err := digestBehaviorValue(payload)
	if err != nil {
		return BehaviorEvaluation{}, err
	}
	evaluation.ID = "sha256:" + digest
	return evaluation, nil
}

type BehaviorRegistry struct {
	generations      map[string]BehaviorGeneration
	states           map[string]CandidateState
	evaluations      map[string]BehaviorEvaluation
	demotions        map[string]DemotionRecord
	inversionReviews map[string]InversionReviewRecord
	activeID         string
	rollbackID       string
	path             string
}

type behaviorSnapshot struct {
	Generations      map[string]BehaviorGeneration    `json:"generations"`
	States           map[string]CandidateState        `json:"states"`
	Evaluations      map[string]BehaviorEvaluation    `json:"evaluations"`
	Demotions        map[string]DemotionRecord        `json:"demotions,omitempty"`
	InversionReviews map[string]InversionReviewRecord `json:"inversion_reviews,omitempty"`
	ActiveID         string                           `json:"active_id"`
	RollbackID       string                           `json:"rollback_id,omitempty"`
}

func OpenBehaviorRegistry(path string, seed BehaviorGeneration) (*BehaviorRegistry, error) {
	if path == "" || seed.ID == "" {
		return nil, errors.New("registry path and seed generation are required")
	}
	bytes, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := VerifyBehaviorGeneration(seed); err != nil {
			return nil, err
		}
		registry := &BehaviorRegistry{generations: map[string]BehaviorGeneration{seed.ID: seed}, states: map[string]CandidateState{seed.ID: CandidateStabilized}, evaluations: map[string]BehaviorEvaluation{}, demotions: map[string]DemotionRecord{}, inversionReviews: map[string]InversionReviewRecord{}, activeID: seed.ID, path: path}
		if err := registry.persist(); err != nil {
			return nil, err
		}
		return registry, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot behaviorSnapshot
	if err := json.Unmarshal(bytes, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Generations[snapshot.ActiveID].ID == "" {
		return nil, errors.New("registry active generation is unavailable")
	}
	for id, generation := range snapshot.Generations {
		if id != generation.ID {
			return nil, errors.New("registry generation key mismatch")
		}
		if err := VerifyBehaviorGeneration(generation); err != nil {
			return nil, err
		}
	}
	for id, evaluation := range snapshot.Evaluations {
		if id != evaluation.ID {
			return nil, errors.New("registry evaluation key mismatch")
		}
		if err := VerifyBehaviorEvaluation(evaluation); err != nil {
			return nil, err
		}
		if snapshot.Generations[evaluation.ActiveGenerationID].ID == "" || snapshot.Generations[evaluation.CandidateGenerationID].ID == "" {
			return nil, errors.New("registry evaluation refers to unavailable generation")
		}
	}
	if snapshot.Demotions == nil {
		snapshot.Demotions = map[string]DemotionRecord{}
	}
	if snapshot.InversionReviews == nil {
		snapshot.InversionReviews = map[string]InversionReviewRecord{}
	}
	for id, record := range snapshot.InversionReviews {
		if id == "" || id != record.CandidateID {
			return nil, errors.New("invalid persisted architecture inversion review")
		}
		if err := validateInversionReviewRecord(record); err != nil {
			return nil, fmt.Errorf("invalid persisted architecture inversion review: %w", err)
		}
		if snapshot.Generations[record.CandidateID].ID == "" {
			return nil, errors.New("persisted architecture inversion review refers to unavailable generation")
		}
	}
	for id, record := range snapshot.Demotions {
		if id != record.Evaluation.ID {
			return nil, errors.New("registry demotion evaluation key mismatch")
		}
		if err := VerifyDemotionRecord(record); err != nil {
			return nil, err
		}
		evaluation := record.Evaluation
		active, candidate := snapshot.Generations[evaluation.ActiveGenerationID], snapshot.Generations[evaluation.CandidateGenerationID]
		if active.ID == "" || candidate.ID == "" {
			return nil, errors.New("registry demotion evaluation refers to unavailable generation")
		}
		if err := validateDemotionPair(active, candidate, evaluation); err != nil {
			return nil, err
		}
	}
	if snapshot.RollbackID != "" && snapshot.Generations[snapshot.RollbackID].ID == "" {
		return nil, errors.New("registry rollback generation is unavailable")
	}
	if state := snapshot.States[snapshot.ActiveID]; state == CandidateRejected || state == CandidateRevoked || state == CandidateDemoted || state == CandidateProposed || state == CandidateEvaluating {
		return nil, errors.New("registry active generation has non-active lifecycle state")
	}
	return &BehaviorRegistry{generations: snapshot.Generations, states: snapshot.States, evaluations: snapshot.Evaluations, demotions: snapshot.Demotions, inversionReviews: snapshot.InversionReviews, activeID: snapshot.ActiveID, rollbackID: snapshot.RollbackID, path: path}, nil
}

// RecordInversionReview persists content-addressed advisory ownership evidence
// beside learning state. It cannot register, evaluate, promote, activate, or
// alter a candidate; blind derivation remains identified by its separate
// digest.
func (r *BehaviorRegistry) RecordInversionReview(record InversionReviewRecord) error {
	if r == nil {
		return errors.New("behavior registry is required")
	}
	if err := validateInversionReviewRecord(record); err != nil {
		return err
	}
	if r.generations[record.CandidateID].ID == "" {
		return errors.New("architecture inversion review candidate generation is unavailable")
	}
	if r.inversionReviews == nil {
		r.inversionReviews = map[string]InversionReviewRecord{}
	}
	if existing, ok := r.inversionReviews[record.CandidateID]; ok {
		existingBytes, _ := json.Marshal(existing)
		recordBytes, _ := json.Marshal(record)
		if string(existingBytes) != string(recordBytes) {
			return errors.New("architecture inversion review identity collision")
		}
		return nil
	}
	r.inversionReviews[record.CandidateID] = record
	return r.persist()
}

func (r *BehaviorRegistry) InversionReview(candidateID string) (InversionReviewRecord, bool) {
	record, ok := r.inversionReviews[candidateID]
	return record, ok
}

func (r *BehaviorRegistry) Register(candidate BehaviorGeneration) error {
	if err := VerifyBehaviorGeneration(candidate); err != nil {
		return err
	}
	if r.generations[candidate.ParentID].ID == "" {
		return errors.New("candidate parent generation is unavailable")
	}
	if existing := r.generations[candidate.ID]; existing.ID != "" {
		existingBytes, _ := json.Marshal(existing)
		candidateBytes, _ := json.Marshal(candidate)
		if string(existingBytes) != string(candidateBytes) {
			return errors.New("candidate generation identity collision")
		}
		return nil
	}
	r.generations[candidate.ID] = candidate
	r.states[candidate.ID] = CandidateProposed
	return r.persist()
}

func (r *BehaviorRegistry) Reject(candidate BehaviorGeneration, evaluation BehaviorEvaluation) error {
	if evaluation.CandidateGenerationID != candidate.ID || evaluation.ID == "" || evaluation.Passed {
		return errors.New("rejection requires a failed evaluation for the candidate")
	}
	if err := VerifyBehaviorEvaluation(evaluation); err != nil {
		return err
	}
	if err := r.Register(candidate); err != nil {
		return err
	}
	r.evaluations[evaluation.ID] = evaluation
	r.states[candidate.ID] = CandidateRejected
	return r.persist()
}

func (r *BehaviorRegistry) Promote(candidate BehaviorGeneration, evaluation BehaviorEvaluation, decision GovernanceDecision, proposerID string) error {
	if decision.AuthorityID == "" || decision.AuthorityID == proposerID {
		return errors.New("candidate cannot promote itself")
	}
	if !decision.Approved || decision.DecidedAt.IsZero() || decision.EvaluationID != evaluation.ID || !evaluation.Passed {
		return errors.New("passing evaluation and independent governed approval are required")
	}
	if evaluation.ActiveGenerationID != r.activeID || evaluation.CandidateGenerationID != candidate.ID || candidate.ParentID != r.activeID {
		return errors.New("evaluation does not bind the active and candidate generations")
	}
	if err := VerifyBehaviorEvaluation(evaluation); err != nil {
		return err
	}
	if err := r.Register(candidate); err != nil {
		return err
	}
	r.evaluations[evaluation.ID] = evaluation
	r.states[candidate.ID] = CandidatePromoted
	r.rollbackID = r.activeID
	r.activeID = candidate.ID
	return r.persist()
}

func (r *BehaviorRegistry) DemoteTo(candidate BehaviorGeneration, record DemotionRecord, decision GovernanceDecision, proposerID string) error {
	evaluation := record.Evaluation
	if decision.AuthorityID == "" || decision.AuthorityID == proposerID {
		return errors.New("candidate cannot demote itself")
	}
	if !decision.Approved || decision.DecidedAt.IsZero() || decision.EvaluationID != evaluation.ID {
		return errors.New("passing demotion evaluation and independent governed approval are required")
	}
	if err := VerifyDemotionRecord(record); err != nil {
		return err
	}
	active := r.generations[r.activeID]
	if err := VerifyBehaviorGeneration(candidate); err != nil {
		return err
	}
	if err := validateDemotionPair(active, candidate, evaluation); err != nil {
		return err
	}
	if err := r.Register(candidate); err != nil {
		return err
	}
	r.demotions[evaluation.ID] = record
	r.states[r.activeID] = CandidateDemoted
	r.states[candidate.ID] = CandidatePromoted
	r.rollbackID = r.activeID
	r.activeID = candidate.ID
	return r.persist()
}

func (r *BehaviorRegistry) Rollback() error {
	if r.rollbackID == "" || r.generations[r.rollbackID].ID == "" {
		return errors.New("rollback generation is unavailable")
	}
	current := r.activeID
	r.activeID = r.rollbackID
	r.rollbackID = current
	if r.states[r.activeID] == CandidateDemoted {
		r.states[r.activeID] = CandidatePromoted
	}
	if r.states[current] == CandidatePromoted {
		r.states[current] = CandidateDemoted
	}
	return r.persist()
}

func (r *BehaviorRegistry) Active() BehaviorGeneration     { return r.generations[r.activeID] }
func (r *BehaviorRegistry) HasGeneration(id string) bool   { return r.generations[id].ID != "" }
func (r *BehaviorRegistry) State(id string) CandidateState { return r.states[id] }

func (r *BehaviorRegistry) persist() error {
	snapshot := behaviorSnapshot{Generations: r.generations, States: r.states, Evaluations: r.evaluations, Demotions: r.demotions, InversionReviews: r.inversionReviews, ActiveID: r.activeID, RollbackID: r.rollbackID}
	bytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	temporary := r.path + ".tmp"
	if err := os.WriteFile(temporary, bytes, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, r.path); err != nil {
		return fmt.Errorf("commit behavior registry: %w", err)
	}
	return nil
}

func digestBehaviorValue(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}
