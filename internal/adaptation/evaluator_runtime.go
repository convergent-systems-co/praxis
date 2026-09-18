package adaptation

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var evaluationPlanVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "adaptive.evaluation_plan", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)

type EvaluationPlan struct {
	ID             string         `json:"id"`
	Version        string         `json:"version"`
	SubjectAgentID string         `json:"subject_agent_id"`
	GoalClass      string         `json:"goal_class"`
	Domain         string         `json:"domain"`
	BehaviorKey    string         `json:"behavior_key"`
	Context        string         `json:"context"`
	Evaluators     []EvaluatorRef `json:"evaluators"`
	Policy         AnalysisPolicy `json:"policy"`
}

type SeriesEvaluator interface {
	Identity() EvaluatorRef
	EvaluateSeries(observations []Observation, evaluatedAt time.Time) (Measure, error)
}

func FreezeEvaluationPlan(plan EvaluationPlan) (EvaluationPlan, error) {
	plan.ID, plan.Version = "", evaluationPlanVersions.CurrentVersion()
	plan.Evaluators = append([]EvaluatorRef(nil), plan.Evaluators...)
	sort.Slice(plan.Evaluators, func(i, j int) bool {
		if plan.Evaluators[i].ID != plan.Evaluators[j].ID {
			return plan.Evaluators[i].ID < plan.Evaluators[j].ID
		}
		return plan.Evaluators[i].Version < plan.Evaluators[j].Version
	})
	if plan.SubjectAgentID == "" || plan.GoalClass == "" || plan.Domain == "" || plan.BehaviorKey == "" || plan.Context == "" || len(plan.Evaluators) == 0 || VerifyAnalysisPolicy(plan.Policy) != nil {
		return EvaluationPlan{}, errors.New("evaluation plan requires scope, evaluators, and frozen policy")
	}
	seen := map[string]bool{}
	for _, evaluator := range plan.Evaluators {
		key := evaluator.ID + "\x00" + evaluator.Version
		if evaluator.ID == "" || evaluator.Version == "" || seen[key] {
			return EvaluationPlan{}, errors.New("evaluation plan requires unique versioned evaluator identities")
		}
		seen[key] = true
	}
	digest, err := digestValue(plan)
	if err != nil {
		return EvaluationPlan{}, err
	}
	plan.ID = "sha256:" + digest
	return plan, nil
}

func RunEvaluationPlan(ctx context.Context, ledger *Ledger, plan EvaluationPlan, evaluators []SeriesEvaluator, evaluatedAt time.Time) (AnalysisRecord, []Measurement, error) {
	frozen, err := FreezeEvaluationPlan(plan)
	if err != nil || frozen.ID != plan.ID || evaluatedAt.IsZero() {
		return AnalysisRecord{}, nil, errors.New("longitudinal execution requires frozen plan and evaluation time")
	}
	available := map[string]SeriesEvaluator{}
	for _, evaluator := range evaluators {
		if evaluator == nil {
			return AnalysisRecord{}, nil, errors.New("nil series evaluator")
		}
		identity := evaluator.Identity()
		key := identity.ID + "\x00" + identity.Version
		if identity.ID == "" || identity.Version == "" || available[key] != nil {
			return AnalysisRecord{}, nil, errors.New("series evaluators require unique versioned identities")
		}
		available[key] = evaluator
	}
	all, err := ledger.Observations(ctx, plan.SubjectAgentID)
	if err != nil {
		return AnalysisRecord{}, nil, err
	}
	observations := []Observation{}
	for _, observation := range all {
		if observation.GoalClass == plan.GoalClass && observation.Domain == plan.Domain && observation.BehaviorKey == plan.BehaviorKey && observation.Context == plan.Context && !observation.ObservedAt.After(evaluatedAt) {
			observations = append(observations, observation)
		}
	}
	if len(observations) == 0 {
		return AnalysisRecord{}, nil, errors.New("evaluation plan has no durable in-scope observations")
	}
	observationIDs := map[string]bool{}
	for _, observation := range observations {
		observationIDs[observation.ID] = true
	}
	measurements := []Measurement{}
	for _, required := range plan.Evaluators {
		evaluator := available[required.ID+"\x00"+required.Version]
		if evaluator == nil {
			return AnalysisRecord{}, nil, errors.New("required versioned evaluator is unavailable")
		}
		measure, err := evaluator.EvaluateSeries(append([]Observation(nil), observations...), evaluatedAt.UTC())
		if err != nil {
			return AnalysisRecord{}, nil, err
		}
		if measure.Evaluator == nil || *measure.Evaluator != required || measure.Kind == RawMeasure || measure.MeasuredAt.After(evaluatedAt) {
			return AnalysisRecord{}, nil, errors.New("series evaluator returned semantically mismatched measurement")
		}
		for _, sourceID := range measure.SourceObservationIDs {
			if !observationIDs[sourceID] {
				return AnalysisRecord{}, nil, errors.New("series evaluator cited an unavailable observation")
			}
		}
		measurement, err := FreezeMeasurement(Measurement{SubjectAgentID: plan.SubjectAgentID, GoalClass: plan.GoalClass, Domain: plan.Domain, BehaviorKey: plan.BehaviorKey, Context: plan.Context, Measure: measure})
		if err != nil {
			return AnalysisRecord{}, nil, err
		}
		if err := ledger.RecordMeasurement(ctx, measurement); err != nil {
			return AnalysisRecord{}, nil, err
		}
		measurements = append(measurements, measurement)
	}
	report, err := EvaluateLongitudinal(observations, measurements, plan.Policy, evaluatedAt)
	if err != nil {
		return AnalysisRecord{}, nil, err
	}
	record := AnalysisRecord{Policy: plan.Policy, Report: report}
	if err := ledger.RecordAnalysis(ctx, record); err != nil {
		return AnalysisRecord{}, nil, err
	}
	return record, measurements, nil
}
