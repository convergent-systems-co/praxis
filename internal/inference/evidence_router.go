package inference

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var routingPolicyVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
	Contract:       "inference.routing_policy",
	CurrentVersion: "v1",
	Versions: []contracts.ContractVersionDefinition{
		{Version: "v1", Disposition: contracts.VersionCurrent},
	},
}, nil)

type MetricRule struct {
	ID        string                        `json:"id"`
	Name      string                        `json:"name"`
	Kind      adaptation.MeasureKind        `json:"kind"`
	Unit      string                        `json:"unit,omitempty"`
	Operator  adaptation.ComparisonOperator `json:"operator"`
	Threshold float64                       `json:"threshold"`
}

type RoutingPolicy struct {
	ID                      string       `json:"id"`
	Version                 string       `json:"version"`
	Tier                    Tier         `json:"tier"`
	RequiredMetrics         []MetricRule `json:"required_metrics"`
	ObjectiveMetric         string       `json:"objective_metric"`
	PreferLowerObjective    bool         `json:"prefer_lower_objective"`
	MinimumIndependentRoots int          `json:"minimum_independent_roots"`
}

type RouteCandidate struct {
	ExecutorID     string   `json:"executor_id"`
	ProviderID     string   `json:"provider_id"`
	MeasurementIDs []string `json:"measurement_ids"`
}

type RouteRequest struct {
	ID             string `json:"id"`
	SubjectAgentID string `json:"subject_agent_id"`
	RunID          string `json:"run_id"`
	GoalClass      string `json:"goal_class"`
	Domain         string `json:"domain"`
	BehaviorKey    string `json:"behavior_key"`
	Context        string `json:"context"`
	Tier           Tier   `json:"tier"`
}

func FreezeRouteRequest(request RouteRequest) (RouteRequest, error) {
	request.ID = ""
	if request.SubjectAgentID == "" || request.RunID == "" || request.GoalClass == "" || request.Domain == "" || request.BehaviorKey == "" || request.Context == "" || (request.Tier != D1 && request.Tier != D2) {
		return RouteRequest{}, errors.New("route request requires execution scope and inference tier")
	}
	request.ID = inferenceDigest(request)
	return request, nil
}

type EligibilityEvidence struct {
	ID                    string    `json:"id"`
	RequestID             string    `json:"request_id"`
	ExecutorID            string    `json:"executor_id"`
	ProviderID            string    `json:"provider_id"`
	Tier                  Tier      `json:"tier"`
	AuthorityID           string    `json:"authority_id"`
	CapabilityEvidenceRef string    `json:"capability_evidence_ref"`
	PolicyEvidenceRef     string    `json:"policy_evidence_ref"`
	Authorized            bool      `json:"authorized"`
	CapabilityGranted     bool      `json:"capability_granted"`
	PolicyAllowed         bool      `json:"policy_allowed"`
	Available             bool      `json:"available"`
	EvaluatedAt           time.Time `json:"evaluated_at"`
}

type EligibilityAuthority interface {
	EvaluateRouteEligibility(ctx context.Context, request RouteRequest, candidate RouteCandidate) (EligibilityEvidence, error)
}

type EvidenceDecision struct {
	ID                 string   `json:"id"`
	RequestID          string   `json:"request_id"`
	PolicyID           string   `json:"policy_id"`
	ExecutorID         string   `json:"executor_id"`
	ProviderID         string   `json:"provider_id"`
	Tier               Tier     `json:"tier"`
	EligibilityID      string   `json:"eligibility_id"`
	MeasurementIDs     []string `json:"measurement_ids"`
	ObservationIDs     []string `json:"observation_ids"`
	IndependentRoots   []string `json:"independent_roots"`
	ObjectiveMeasureID string   `json:"objective_measure_id"`
}

func FreezeRoutingPolicy(policy RoutingPolicy) (RoutingPolicy, error) {
	policy.ID, policy.Version = "", routingPolicyVersions.CurrentVersion()
	policy.RequiredMetrics = append([]MetricRule(nil), policy.RequiredMetrics...)
	sort.Slice(policy.RequiredMetrics, func(i, j int) bool { return policy.RequiredMetrics[i].ID < policy.RequiredMetrics[j].ID })
	if policy.Tier != D1 && policy.Tier != D2 || len(policy.RequiredMetrics) == 0 || policy.ObjectiveMetric == "" || policy.MinimumIndependentRoots <= 0 {
		return RoutingPolicy{}, errors.New("routing policy requires inference tier, metrics, objective, and independent evidence")
	}
	seen, objective := map[string]bool{}, false
	for _, rule := range policy.RequiredMetrics {
		if rule.ID == "" || rule.Name == "" || seen[rule.ID] || (rule.Kind != adaptation.DerivedMeasure && rule.Kind != adaptation.NormalizedScore) {
			return RoutingPolicy{}, errors.New("routing metric rules require unique identity and derived/normalized semantics")
		}
		if math.IsNaN(rule.Threshold) || math.IsInf(rule.Threshold, 0) || !knownMetricOperator(rule.Operator) {
			return RoutingPolicy{}, errors.New("routing metric rule requires finite threshold and known comparison")
		}
		seen[rule.ID] = true
		objective = objective || rule.Name == policy.ObjectiveMetric
	}
	if !objective {
		return RoutingPolicy{}, errors.New("routing objective must be a required metric")
	}
	policy.ID = inferenceDigest(policy)
	return policy, nil
}

func RouteFromEvidence(ctx context.Context, request RouteRequest, policy RoutingPolicy, candidates []RouteCandidate, observations []adaptation.Observation, measurements []adaptation.Measurement, authority EligibilityAuthority) (EvidenceDecision, error) {
	frozenRequest, err := FreezeRouteRequest(request)
	if err != nil || frozenRequest.ID != request.ID || request.Tier != policy.Tier {
		return EvidenceDecision{}, errors.New("route request is not frozen or does not match policy tier")
	}
	frozen, err := FreezeRoutingPolicy(policy)
	if err != nil || frozen.ID != policy.ID {
		return EvidenceDecision{}, errors.New("routing policy is not frozen")
	}
	if authority == nil {
		return EvidenceDecision{}, errors.New("routing requires deterministic eligibility authority")
	}
	obsByID, measureByID := map[string]adaptation.Observation{}, map[string]adaptation.Measurement{}
	for _, observation := range observations {
		if err := adaptation.VerifyObservation(observation); err != nil {
			return EvidenceDecision{}, err
		}
		obsByID[observation.ID] = observation
	}
	for _, measurement := range measurements {
		if err := adaptation.VerifyMeasurement(measurement); err != nil {
			return EvidenceDecision{}, err
		}
		measureByID[measurement.ID] = measurement
	}
	type eligible struct {
		decision  EvidenceDecision
		objective float64
	}
	choices := []eligible{}
	for _, candidate := range candidates {
		eligibility, err := authority.EvaluateRouteEligibility(ctx, request, candidate)
		if err != nil {
			return EvidenceDecision{}, err
		}
		if err := verifyEligibility(eligibility, request, candidate); err != nil {
			return EvidenceDecision{}, err
		}
		if !eligibility.Authorized || !eligibility.CapabilityGranted || !eligibility.PolicyAllowed || !eligibility.Available {
			continue
		}
		selected, sourceIDs, roots := map[string]adaptation.Measurement{}, map[string]bool{}, map[string]bool{}
		valid := true
		for _, id := range candidate.MeasurementIDs {
			measurement, ok := measureByID[id]
			if !ok || measurement.SubjectAgentID != request.SubjectAgentID || measurement.GoalClass != request.GoalClass || measurement.Domain != request.Domain || measurement.BehaviorKey != request.BehaviorKey || measurement.Context != request.Context {
				valid = false
				break
			}
			for _, sourceID := range measurement.Measure.SourceObservationIDs {
				observation, ok := obsByID[sourceID]
				if !ok || observation.ProviderID != candidate.ProviderID {
					valid = false
					break
				}
				sourceIDs[sourceID], roots[observation.CausationRoot] = true, true
			}
			if _, duplicate := selected[measurement.Measure.Name]; duplicate {
				valid = false
				break
			}
			selected[measurement.Measure.Name] = measurement
		}
		if !valid || len(roots) < policy.MinimumIndependentRoots {
			continue
		}
		for _, rule := range policy.RequiredMetrics {
			measurement, ok := selected[rule.Name]
			if !ok || measurement.Measure.Kind != rule.Kind || measurement.Measure.Unit != rule.Unit || !metricCompare(measurement.Measure.Value, rule.Operator, rule.Threshold) {
				valid = false
				break
			}
		}
		objective := selected[policy.ObjectiveMetric]
		if !valid || objective.ID == "" {
			continue
		}
		choices = append(choices, eligible{decision: EvidenceDecision{RequestID: request.ID, PolicyID: policy.ID, ExecutorID: candidate.ExecutorID, ProviderID: candidate.ProviderID, Tier: policy.Tier, EligibilityID: eligibility.ID, MeasurementIDs: sortedInferenceKeys(selected), ObservationIDs: sortedBoolKeys(sourceIDs), IndependentRoots: sortedBoolKeys(roots), ObjectiveMeasureID: objective.ID}, objective: objective.Measure.Value})
	}
	if len(choices) == 0 {
		return EvidenceDecision{}, errors.New("no executor satisfies authoritative eligibility and evidence policy")
	}
	sort.Slice(choices, func(i, j int) bool {
		if choices[i].objective == choices[j].objective {
			return choices[i].decision.ExecutorID < choices[j].decision.ExecutorID
		}
		if policy.PreferLowerObjective {
			return choices[i].objective < choices[j].objective
		}
		return choices[i].objective > choices[j].objective
	})
	decision := choices[0].decision
	decision.ID = inferenceDigest(decision)
	return decision, nil
}

func verifyEligibility(e EligibilityEvidence, request RouteRequest, candidate RouteCandidate) error {
	if e.ID == "" || e.RequestID != request.ID || e.ExecutorID != candidate.ExecutorID || e.ProviderID != candidate.ProviderID || e.Tier != request.Tier || e.AuthorityID == "" || e.CapabilityEvidenceRef == "" || e.PolicyEvidenceRef == "" || e.EvaluatedAt.IsZero() {
		return errors.New("eligibility evidence is incomplete or mismatched")
	}
	id := e.ID
	e.ID = ""
	if id != inferenceDigest(e) {
		return errors.New("eligibility evidence digest mismatch")
	}
	return nil
}

func FreezeEligibility(e EligibilityEvidence) (EligibilityEvidence, error) {
	e.ID = ""
	if e.RequestID == "" || e.ExecutorID == "" || e.ProviderID == "" || e.AuthorityID == "" || e.CapabilityEvidenceRef == "" || e.PolicyEvidenceRef == "" || e.EvaluatedAt.IsZero() {
		return EligibilityEvidence{}, errors.New("eligibility evidence is incomplete")
	}
	e.ID = inferenceDigest(e)
	return e, nil
}

func inferenceDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func metricCompare(value float64, operator adaptation.ComparisonOperator, threshold float64) bool {
	switch operator {
	case adaptation.LessThan:
		return value < threshold
	case adaptation.LessThanOrEqual:
		return value <= threshold
	case adaptation.GreaterThan:
		return value > threshold
	case adaptation.GreaterThanOrEqual:
		return value >= threshold
	}
	return false
}
func knownMetricOperator(operator adaptation.ComparisonOperator) bool {
	return operator == adaptation.LessThan || operator == adaptation.LessThanOrEqual || operator == adaptation.GreaterThan || operator == adaptation.GreaterThanOrEqual
}
func sortedBoolKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
func sortedInferenceKeys(values map[string]adaptation.Measurement) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.ID)
	}
	sort.Strings(out)
	return out
}
