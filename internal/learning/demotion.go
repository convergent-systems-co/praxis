package learning

import (
	"errors"
	"sort"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
)

type DemotionPolicy struct {
	ID                               string   `json:"id"`
	Version                          string   `json:"version"`
	TriggerDiagnoses                 []string `json:"trigger_diagnoses"`
	MinimumIndependentContradictions int      `json:"minimum_independent_contradictions"`
}

func FreezeDemotionPolicy(policy DemotionPolicy) (DemotionPolicy, error) {
	policy.Version = "v1"
	policy.ID = ""
	policy.TriggerDiagnoses = append([]string(nil), policy.TriggerDiagnoses...)
	sort.Strings(policy.TriggerDiagnoses)
	if err := validateDemotionPolicy(policy, false); err != nil {
		return DemotionPolicy{}, err
	}
	digest, err := digestBehaviorValue(policy)
	if err != nil {
		return DemotionPolicy{}, err
	}
	policy.ID = "sha256:" + digest
	return policy, nil
}

func VerifyDemotionPolicy(policy DemotionPolicy) error {
	if err := validateDemotionPolicy(policy, true); err != nil {
		return err
	}
	id := policy.ID
	policy.ID = ""
	digest, err := digestBehaviorValue(policy)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("demotion policy digest mismatch")
	}
	return nil
}

func validateDemotionPolicy(policy DemotionPolicy, requireID bool) error {
	if requireID && policy.ID == "" {
		return errors.New("demotion policy identity is required")
	}
	if policy.Version != "v1" || policy.MinimumIndependentContradictions <= 0 || len(policy.TriggerDiagnoses) == 0 {
		return errors.New("versioned demotion policy, diagnoses, and evidence threshold are required")
	}
	seen := map[string]bool{}
	for _, diagnosis := range policy.TriggerDiagnoses {
		if diagnosis == "" || seen[diagnosis] {
			return errors.New("demotion diagnoses must be non-empty and unique")
		}
		seen[diagnosis] = true
	}
	return nil
}

type DemotionEvaluation struct {
	ID                       string   `json:"id"`
	AnalysisPolicyID         string   `json:"analysis_policy_id"`
	DemotionPolicyID         string   `json:"demotion_policy_id"`
	ReportID                 string   `json:"report_id"`
	ActiveGenerationID       string   `json:"active_generation_id"`
	CandidateGenerationID    string   `json:"candidate_generation_id"`
	InstructionID            string   `json:"instruction_id"`
	BehaviorKey              string   `json:"behavior_key"`
	DiagnosisIDs             []string `json:"diagnosis_ids"`
	MeasurementIDs           []string `json:"measurement_ids"`
	EvidenceIDs              []string `json:"evidence_ids"`
	IndependentEvidenceRoots []string `json:"independent_evidence_roots"`
	Passed                   bool     `json:"passed"`
}

type DemotionRecord struct {
	Analysis   adaptation.AnalysisRecord `json:"analysis"`
	Policy     DemotionPolicy            `json:"policy"`
	Evaluation DemotionEvaluation        `json:"evaluation"`
}

func ProposeInferenceDemotion(active BehaviorGeneration, instructionID, behaviorKey string, observations []adaptation.Observation, measurements []adaptation.Measurement, analysis adaptation.AnalysisRecord, policy DemotionPolicy) (BehaviorGeneration, DemotionRecord, error) {
	if err := VerifyBehaviorGeneration(active); err != nil {
		return BehaviorGeneration{}, DemotionRecord{}, err
	}
	if err := adaptation.VerifyAnalysisRecord(analysis); err != nil {
		return BehaviorGeneration{}, DemotionRecord{}, err
	}
	if err := VerifyDemotionPolicy(policy); err != nil {
		return BehaviorGeneration{}, DemotionRecord{}, err
	}
	if instructionID == "" || behaviorKey == "" {
		return BehaviorGeneration{}, DemotionRecord{}, errors.New("instruction and behavior are required")
	}
	recomputed, err := adaptation.EvaluateLongitudinal(observations, measurements, analysis.Policy, analysis.Report.EvaluatedAt)
	if err != nil || recomputed.ID != analysis.Report.ID {
		return BehaviorGeneration{}, DemotionRecord{}, errors.New("analysis is not derived from supplied evidence and policy")
	}
	report := analysis.Report
	if len(report.InvariantFailures) != 0 {
		return BehaviorGeneration{}, DemotionRecord{}, errors.New("policy/security invariant failure blocks adaptive candidate")
	}

	observationByID := map[string]adaptation.Observation{}
	reportObservationIDs := stringSet(report.ObservationIDs)
	for _, observation := range observations {
		if err := adaptation.VerifyObservation(observation); err != nil {
			return BehaviorGeneration{}, DemotionRecord{}, err
		}
		if observation.SubjectAgentID == report.SubjectAgentID && reportObservationIDs[observation.ID] {
			observationByID[observation.ID] = observation
		}
	}
	measurementByID := map[string]adaptation.Measurement{}
	reportMeasurementIDs := stringSet(report.MeasurementIDs)
	for _, measurement := range measurements {
		if err := adaptation.VerifyMeasurement(measurement); err != nil {
			return BehaviorGeneration{}, DemotionRecord{}, err
		}
		if measurement.SubjectAgentID == report.SubjectAgentID && measurement.BehaviorKey == behaviorKey && reportMeasurementIDs[measurement.ID] {
			measurementByID[measurement.ID] = measurement
		}
	}
	triggerCodes := stringSet(policy.TriggerDiagnoses)
	diagnosisIDs, measurementIDs := []string{}, map[string]bool{}
	evidenceIDs, roots := map[string]bool{}, map[string]bool{}
	for _, diagnosis := range report.Diagnoses {
		if !triggerCodes[diagnosis.Code] {
			continue
		}
		matchedTarget := false
		for _, measurementID := range diagnosis.MeasurementIDs {
			measurement, ok := measurementByID[measurementID]
			if !ok {
				continue
			}
			matchedTarget = true
			measurementIDs[measurementID] = true
			for _, sourceID := range measurement.Measure.SourceObservationIDs {
				observation, ok := observationByID[sourceID]
				if !ok {
					return BehaviorGeneration{}, DemotionRecord{}, errors.New("demotion diagnosis source observation is unavailable")
				}
				evidenceIDs[sourceID] = true
				roots[observation.CausationRoot] = true
			}
		}
		if matchedTarget {
			diagnosisIDs = append(diagnosisIDs, diagnosis.ID)
		}
	}
	if len(diagnosisIDs) == 0 {
		return BehaviorGeneration{}, DemotionRecord{}, errors.New("report contains no configured demotion diagnosis for behavior")
	}
	if len(roots) < policy.MinimumIndependentContradictions {
		return BehaviorGeneration{}, DemotionRecord{}, errors.New("insufficient independent contradiction evidence")
	}

	instructions := append([]AdvisoryInstruction(nil), active.Instructions...)
	foundInstruction, foundRule := false, false
	for index := range instructions {
		if instructions[index].ID != instructionID {
			continue
		}
		if instructions[index].Active || instructions[index].SupersededBy == "" {
			return BehaviorGeneration{}, DemotionRecord{}, errors.New("instruction is not currently replaced by deterministic behavior")
		}
		instructions[index].Active = true
		instructions[index].SupersededBy = ""
		foundInstruction = true
	}
	rules := make([]DeterministicRule, 0, len(active.Rules))
	for _, rule := range active.Rules {
		if rule.InstructionID == instructionID {
			foundRule = true
			continue
		}
		rules = append(rules, rule)
	}
	if !foundInstruction || !foundRule {
		return BehaviorGeneration{}, DemotionRecord{}, errors.New("deterministic behavior for instruction is unavailable")
	}

	evidenceList, measurementList, rootList := sortedKeys(evidenceIDs), sortedKeys(measurementIDs), sortedKeys(roots)
	sort.Strings(diagnosisIDs)
	candidate, err := NewBehaviorGeneration(active.ID, instructions, rules, evidenceList)
	if err != nil {
		return BehaviorGeneration{}, DemotionRecord{}, err
	}
	evaluation := DemotionEvaluation{AnalysisPolicyID: analysis.Policy.ID, DemotionPolicyID: policy.ID, ReportID: report.ID, ActiveGenerationID: active.ID, CandidateGenerationID: candidate.ID, InstructionID: instructionID, BehaviorKey: behaviorKey, DiagnosisIDs: diagnosisIDs, MeasurementIDs: measurementList, EvidenceIDs: evidenceList, IndependentEvidenceRoots: rootList, Passed: true}
	payload := evaluation
	payload.ID = ""
	digest, err := digestBehaviorValue(payload)
	if err != nil {
		return BehaviorGeneration{}, DemotionRecord{}, err
	}
	evaluation.ID = "sha256:" + digest
	return candidate, DemotionRecord{Analysis: analysis, Policy: policy, Evaluation: evaluation}, nil
}

func VerifyDemotionEvaluation(evaluation DemotionEvaluation) error {
	if evaluation.ID == "" || evaluation.AnalysisPolicyID == "" || evaluation.DemotionPolicyID == "" || evaluation.ReportID == "" || evaluation.ActiveGenerationID == "" || evaluation.CandidateGenerationID == "" || evaluation.InstructionID == "" || evaluation.BehaviorKey == "" || len(evaluation.DiagnosisIDs) == 0 || len(evaluation.MeasurementIDs) == 0 || len(evaluation.EvidenceIDs) == 0 || len(evaluation.IndependentEvidenceRoots) == 0 || !evaluation.Passed {
		return errors.New("demotion evaluation is incomplete")
	}
	payload := evaluation
	payload.ID = ""
	digest, err := digestBehaviorValue(payload)
	if err != nil {
		return err
	}
	if evaluation.ID != "sha256:"+digest {
		return errors.New("demotion evaluation digest mismatch")
	}
	return nil
}

func VerifyDemotionRecord(record DemotionRecord) error {
	if err := adaptation.VerifyAnalysisRecord(record.Analysis); err != nil {
		return err
	}
	if err := VerifyDemotionPolicy(record.Policy); err != nil {
		return err
	}
	if err := VerifyDemotionEvaluation(record.Evaluation); err != nil {
		return err
	}
	if record.Evaluation.AnalysisPolicyID != record.Analysis.Policy.ID || record.Evaluation.DemotionPolicyID != record.Policy.ID || record.Evaluation.ReportID != record.Analysis.Report.ID {
		return errors.New("demotion evaluation does not bind its analysis and action policies")
	}
	return nil
}

func validateDemotionPair(active, candidate BehaviorGeneration, evaluation DemotionEvaluation) error {
	if candidate.ParentID != active.ID || evaluation.ActiveGenerationID != active.ID || evaluation.CandidateGenerationID != candidate.ID {
		return errors.New("demotion lineage/evaluation mismatch")
	}
	activeInstructions, candidateInstructions := map[string]AdvisoryInstruction{}, map[string]AdvisoryInstruction{}
	for _, instruction := range active.Instructions {
		activeInstructions[instruction.ID] = instruction
	}
	for _, instruction := range candidate.Instructions {
		candidateInstructions[instruction.ID] = instruction
	}
	if len(activeInstructions) != len(candidateInstructions) {
		return errors.New("demotion changed instruction set")
	}
	for id, before := range activeInstructions {
		after, ok := candidateInstructions[id]
		if !ok {
			return errors.New("demotion removed instruction")
		}
		if id == evaluation.InstructionID {
			if before.Active || before.SupersededBy == "" || !after.Active || after.SupersededBy != "" || before.Text != after.Text || before.Tier != after.Tier || before.Learnable != after.Learnable {
				return errors.New("demotion did not restore exact bounded-inference instruction")
			}
		} else if before != after {
			return errors.New("demotion changed unrelated instruction")
		}
	}
	activeRules, candidateRules := map[string]DeterministicRule{}, map[string]DeterministicRule{}
	for _, rule := range active.Rules {
		activeRules[rule.ID] = rule
	}
	for _, rule := range candidate.Rules {
		candidateRules[rule.ID] = rule
	}
	for id, rule := range activeRules {
		if rule.InstructionID == evaluation.InstructionID {
			if _, remains := candidateRules[id]; remains {
				return errors.New("demotion retained contradicted deterministic rule")
			}
			continue
		}
		if candidateRules[id] != rule {
			return errors.New("demotion changed unrelated deterministic rule")
		}
	}
	for id := range candidateRules {
		if _, ok := activeRules[id]; !ok {
			return errors.New("demotion introduced deterministic rule")
		}
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
