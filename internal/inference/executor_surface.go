package inference

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var surfaceDecisionVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
	Contract:       "inference.surface_routing_decision",
	CurrentVersion: "v2",
	Versions: []contracts.ContractVersionDefinition{
		{Version: "v1", Disposition: contracts.VersionUnsupportedPreRelease, Rationale: "v1 accepted caller-asserted eligibility and untrusted authority metadata"},
		{Version: "v2", Disposition: contracts.VersionCurrent},
	},
}, nil)

type SurfaceDecisionOutcome string

const (
	SurfaceSelected             SurfaceDecisionOutcome = "SELECTED"
	SurfaceNoEligible           SurfaceDecisionOutcome = "NO_ELIGIBLE_EXECUTOR"
	SurfaceRequiredUnavailable  SurfaceDecisionOutcome = "REQUIRED_TARGET_UNAVAILABLE"
	SurfaceFallbackProhibited   SurfaceDecisionOutcome = "FALLBACK_PROHIBITED"
	SurfaceAPIUseProhibited     SurfaceDecisionOutcome = "API_USE_PROHIBITED"
	SurfaceTelemetryUnsatisfied SurfaceDecisionOutcome = "TELEMETRY_REQUIREMENT_UNSATISFIED"
)

const (
	ReasonUnavailable       = "availability.unavailable"
	ReasonSecurityDenied    = "security.ineligible"
	ReasonAuthorityDenied   = "authority.ineligible"
	ReasonPolicyDenied      = "policy.ineligible"
	ReasonBudgetExhausted   = "budget.exhausted"
	ReasonQuotaUnavailable  = "quota.unavailable"
	ReasonCapabilityMissing = "capability.required_missing"
	ReasonTransportDenied   = "transport.not_allowed"
	ReasonAPIForbidden      = "api.metered_forbidden"
	ReasonAPIRequired       = "api.metered_required"
	ReasonTelemetryUnknown  = "telemetry.required_unknown"
	ReasonProfileProhibited = "profile.prohibited"
	ReasonPreferredMatch    = "affinity.preferred"
	ReasonAllowedFallback   = "affinity.allowed_fallback"
	ReasonDeterministic     = "affinity.deterministic"
)

// ExecutorSurface is descriptive catalog metadata. Eligibility, policy state,
// availability, and telemetry are issued separately by an authority evaluator.
type ExecutorSurface struct {
	ID               string                   `json:"id"`
	ExecutorID       string                   `json:"executor_id"`
	ProviderID       string                   `json:"provider_id"`
	ProviderMetadata string                   `json:"provider_metadata,omitempty"`
	ModelMetadata    string                   `json:"model_metadata,omitempty"`
	Capabilities     []string                 `json:"capabilities"`
	Profiles         []string                 `json:"profiles"`
	Transport        contracts.TransportClass `json:"transport"`
}

// SurfaceRouteRequest bridges surface selection to the existing governed
// evidence-router request lineage rather than creating a second work identity.
type SurfaceRouteRequest struct {
	ID              string                             `json:"id"`
	RouteRequest    RouteRequest                       `json:"route_request"`
	AgentGeneration string                             `json:"agent_generation"`
	GraphID         string                             `json:"graph_id,omitempty"`
	GraphVersion    string                             `json:"graph_version,omitempty"`
	NodeID          string                             `json:"node_id,omitempty"`
	GoalRef         string                             `json:"goal_ref,omitempty"`
	Target          contracts.EffectiveExecutionTarget `json:"effective_execution_target"`
}

type SurfaceTelemetryEvidence struct {
	Known            bool      `json:"known"`
	Value            string    `json:"value,omitempty"`
	ProvenanceRef    string    `json:"provenance_ref"`
	ProvenanceDigest string    `json:"provenance_digest"`
	ObservedAt       time.Time `json:"observed_at"`
	ValidUntil       time.Time `json:"valid_until"`
}

// SurfaceEligibilityEvidence is composed by core from the existing governed
// route-eligibility lineage. It is not independently issued by a surface
// evaluator selected by the caller.
type SurfaceEligibilityEvidence struct {
	ID                string                              `json:"id"`
	Version           string                              `json:"version"`
	RequestID         string                              `json:"request_id"`
	SurfaceID         string                              `json:"surface_id"`
	SurfaceDigest     string                              `json:"surface_digest"`
	Evaluator         contracts.PrincipalRef              `json:"evaluator"`
	RouteEligibility  EligibilityEvidence                 `json:"route_eligibility"`
	Available         bool                                `json:"available"`
	CapabilityGranted bool                                `json:"capability_granted"`
	SecurityAllowed   bool                                `json:"security_allowed"`
	Authorized        bool                                `json:"authorized"`
	PolicyAllowed     bool                                `json:"policy_allowed"`
	BudgetAllowed     bool                                `json:"budget_allowed"`
	QuotaAllowed      bool                                `json:"quota_allowed"`
	ReasonCodes       []string                            `json:"reason_codes,omitempty"`
	Telemetry         map[string]SurfaceTelemetryEvidence `json:"telemetry,omitempty"`
	EvaluatedAt       time.Time                           `json:"evaluated_at"`
	ValidUntil        time.Time                           `json:"valid_until"`
}

// SurfaceEligibilityComposer is the core-owned bridge from the governed
// EligibilityAuthority lineage to exact concrete-surface eligibility.
type SurfaceEligibilityComposer struct {
	authority EligibilityAuthority
}

func newSurfaceEligibilityComposer(authority EligibilityAuthority) (*SurfaceEligibilityComposer, error) {
	if authority == nil {
		return nil, errors.New("surface eligibility composition requires governed route eligibility authority")
	}
	return &SurfaceEligibilityComposer{authority: authority}, nil
}

type SurfaceEvaluation struct {
	Surface     ExecutorSurface            `json:"surface"`
	Evidence    SurfaceEligibilityEvidence `json:"eligibility_evidence"`
	Eligible    bool                       `json:"eligible"`
	ReasonCodes []string                   `json:"reason_codes,omitempty"`
}

type SurfaceRoutingDecision struct {
	ID                         string                 `json:"id"`
	Version                    string                 `json:"version"`
	Request                    SurfaceRouteRequest    `json:"request"`
	Evaluator                  contracts.PrincipalRef `json:"evaluator"`
	EvaluatorGenerationRef     string                 `json:"evaluator_generation_ref"`
	EvaluatorGenerationVersion string                 `json:"evaluator_generation_version"`
	EvaluatorGenerationDigest  string                 `json:"evaluator_generation_digest"`
	EvaluatorScope             string                 `json:"evaluator_scope"`
	Evaluations                []SurfaceEvaluation    `json:"evaluations"`
	Outcome                    SurfaceDecisionOutcome `json:"outcome"`
	SelectedSurfaceID          string                 `json:"selected_surface_id,omitempty"`
	SelectedProfile            string                 `json:"selected_profile,omitempty"`
	Fallback                   bool                   `json:"fallback"`
	ReasonCodes                []string               `json:"reason_codes"`
	DecidedAt                  time.Time              `json:"decided_at"`
}

func FreezeSurfaceRouteRequest(request SurfaceRouteRequest) (SurfaceRouteRequest, error) {
	request.ID = ""
	frozenRoute, err := FreezeRouteRequest(request.RouteRequest)
	if err != nil || frozenRoute.ID != request.RouteRequest.ID || inferenceDigest(frozenRoute) != inferenceDigest(request.RouteRequest) || request.AgentGeneration == "" {
		return SurfaceRouteRequest{}, errors.New("surface route request requires frozen route lineage and agent generation")
	}
	frozenTarget, err := contracts.FreezeEffectiveExecutionTarget(request.Target)
	if err != nil {
		return SurfaceRouteRequest{}, errors.New("surface route request requires a valid effective execution target")
	}
	request.Target = frozenTarget
	if tier := request.Target.Target.ReasoningTier; tier != "" && tier != string(request.RouteRequest.Tier) {
		return SurfaceRouteRequest{}, errors.New("surface target reasoning tier does not match route request")
	}
	request.ID = inferenceDigest(request)
	return request, nil
}

// selectExecutorSurfaceAt is a non-authoritative deterministic test seam. The
// public selection boundary always derives time from the core clock.
func selectExecutorSurfaceAt(ctx context.Context, request SurfaceRouteRequest, surfaces []ExecutorSurface, composer *SurfaceEligibilityComposer, decidedAt time.Time) (SurfaceRoutingDecision, error) {
	frozen, err := FreezeSurfaceRouteRequest(request)
	if err != nil || frozen.ID != request.ID || inferenceDigest(frozen) != inferenceDigest(request) {
		return SurfaceRoutingDecision{}, errors.New("surface route request is not frozen")
	}
	if composer == nil || composer.authority == nil || decidedAt.IsZero() {
		return SurfaceRoutingDecision{}, errors.New("surface routing requires governed eligibility composition and decision time")
	}
	ordered := append([]ExecutorSurface(nil), surfaces...)
	for index := range ordered {
		ordered[index], _, err = freezeExecutorSurface(ordered[index])
		if err != nil {
			return SurfaceRoutingDecision{}, err
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	seen := map[string]bool{}
	evaluations := make([]SurfaceEvaluation, 0, len(ordered))
	var lineage surfaceEvaluatorLineage
	for _, surface := range ordered {
		if seen[surface.ID] {
			return SurfaceRoutingDecision{}, errors.New("executor surfaces require unique identity")
		}
		seen[surface.ID] = true
		evidence, currentLineage, err := composer.compose(ctx, request, surface)
		if err != nil {
			return SurfaceRoutingDecision{}, err
		}
		if len(evaluations) == 0 {
			lineage = currentLineage
		} else if currentLineage != lineage {
			return SurfaceRoutingDecision{}, errors.New("surface eligibility evaluations require one governed authority lineage")
		}
		evaluation, err := verifyAndEvaluateSurface(request, surface, evidence, lineage, decidedAt)
		if err != nil {
			return SurfaceRoutingDecision{}, err
		}
		evaluations = append(evaluations, evaluation)
	}
	if len(evaluations) == 0 {
		return SurfaceRoutingDecision{}, errors.New("surface routing requires at least one executor surface")
	}
	return buildSurfaceDecision(request, lineage, evaluations, decidedAt)
}

// RecomputeIssuedSurfaceDecision is the deterministic, non-authoritative
// algorithm seam used by the core routingauthority package after it has loaded
// and verified protected issuance records. It performs no authority lookup.
func RecomputeIssuedSurfaceDecision(ctx context.Context, request SurfaceRouteRequest, surfaces []ExecutorSurface, values []EligibilityEvidence, decidedAt time.Time) (SurfaceRoutingDecision, error) {
	evidence := map[string]EligibilityEvidence{}
	for _, value := range values {
		if value.SurfaceID == "" || evidence[value.SurfaceID].ID != "" {
			return SurfaceRoutingDecision{}, errors.New("issued eligibility requires unique surface identity")
		}
		evidence[value.SurfaceID] = value
	}
	composer, _ := newSurfaceEligibilityComposer(eligibilityEvidenceMap(evidence))
	return selectExecutorSurfaceAt(ctx, request, surfaces, composer, decidedAt)
}

type eligibilityEvidenceMap map[string]EligibilityEvidence

func (m eligibilityEvidenceMap) EvaluateRouteEligibility(_ context.Context, _ RouteRequest, candidate RouteCandidate) (EligibilityEvidence, error) {
	value, ok := m[candidate.SurfaceID]
	if !ok {
		return EligibilityEvidence{}, errors.New("surface lacks issued eligibility")
	}
	return value, nil
}

func VerifySurfaceRoutingDecision(decision SurfaceRoutingDecision) error {
	if _, _, err := surfaceDecisionVersions.Canonicalize(decision.Version, nil); err != nil {
		return err
	}
	lineage := surfaceEvaluatorLineage{
		Evaluator:         decision.Evaluator,
		GenerationRef:     decision.EvaluatorGenerationRef,
		GenerationVersion: decision.EvaluatorGenerationVersion,
		GenerationDigest:  decision.EvaluatorGenerationDigest,
		Scope:             decision.EvaluatorScope,
	}
	if decision.ID == "" || decision.Version != surfaceDecisionVersions.CurrentVersion() || decision.DecidedAt.IsZero() || lineage.validate(decision.Request.ID) != nil {
		return errors.New("surface routing decision v2 identity, evaluator, and time are required")
	}
	frozen, err := FreezeSurfaceRouteRequest(decision.Request)
	if err != nil || frozen.ID != decision.Request.ID || inferenceDigest(frozen) != inferenceDigest(decision.Request) {
		return errors.New("surface routing decision request is not frozen")
	}
	evaluations := make([]SurfaceEvaluation, 0, len(decision.Evaluations))
	for _, persisted := range decision.Evaluations {
		evaluation, err := verifyAndEvaluateSurface(decision.Request, persisted.Surface, persisted.Evidence, lineage, decision.DecidedAt)
		if err != nil {
			return err
		}
		evaluations = append(evaluations, evaluation)
	}
	recomputed, err := buildSurfaceDecision(decision.Request, lineage, evaluations, decision.DecidedAt)
	if err != nil || recomputed.ID != decision.ID || inferenceDigest(recomputed) != inferenceDigest(decision) {
		return errors.New("surface routing decision digest or semantics mismatch")
	}
	return nil
}

type surfaceEvaluatorLineage struct {
	Evaluator         contracts.PrincipalRef
	GenerationRef     string
	GenerationVersion string
	GenerationDigest  string
	Scope             string
}

func (lineage surfaceEvaluatorLineage) validate(requestID string) error {
	if !validSurfaceEvaluator(lineage.Evaluator) || lineage.GenerationRef == "" || lineage.GenerationVersion == "" || lineage.GenerationDigest == "" || lineage.Scope != requestID {
		return errors.New("surface evaluator lineage is incomplete or out of scope")
	}
	return nil
}

func (composer *SurfaceEligibilityComposer) compose(ctx context.Context, request SurfaceRouteRequest, surface ExecutorSurface) (SurfaceEligibilityEvidence, surfaceEvaluatorLineage, error) {
	canonical, digest, err := freezeExecutorSurface(surface)
	if err != nil {
		return SurfaceEligibilityEvidence{}, surfaceEvaluatorLineage{}, err
	}
	candidate := RouteCandidate{ExecutorID: canonical.ExecutorID, ProviderID: canonical.ProviderID, SurfaceID: canonical.ID, SurfaceDigest: digest}
	eligibility, err := composer.authority.EvaluateRouteEligibility(ctx, request.RouteRequest, candidate)
	if err != nil {
		return SurfaceEligibilityEvidence{}, surfaceEvaluatorLineage{}, err
	}
	if err := verifyEligibility(eligibility, request.RouteRequest, candidate); err != nil {
		return SurfaceEligibilityEvidence{}, surfaceEvaluatorLineage{}, err
	}
	return composeSurfaceEligibilityEvidence(request, canonical, digest, eligibility)
}

func composeSurfaceEligibilityEvidence(request SurfaceRouteRequest, canonical ExecutorSurface, digest string, eligibility EligibilityEvidence) (SurfaceEligibilityEvidence, surfaceEvaluatorLineage, error) {
	if err := validateSurfaceEligibilityLineage(eligibility); err != nil {
		return SurfaceEligibilityEvidence{}, surfaceEvaluatorLineage{}, err
	}
	if eligibility.SurfaceRequestID != request.ID {
		return SurfaceEligibilityEvidence{}, surfaceEvaluatorLineage{}, errors.New("governed eligibility does not bind the surface request")
	}
	lineage := surfaceEvaluatorLineage{
		Evaluator:         contracts.PrincipalRef{ID: eligibility.AuthorityID, Kind: "authority"},
		GenerationRef:     eligibility.AuthorityGenerationRef,
		GenerationVersion: eligibility.AuthorityGenerationVersion,
		GenerationDigest:  eligibility.AuthorityGenerationDigest,
		Scope:             eligibility.AuthorityScope,
	}
	if err := lineage.validate(request.ID); err != nil {
		return SurfaceEligibilityEvidence{}, surfaceEvaluatorLineage{}, err
	}
	evidence := SurfaceEligibilityEvidence{
		Version:           surfaceDecisionVersions.CurrentVersion(),
		RequestID:         request.ID,
		SurfaceID:         canonical.ID,
		SurfaceDigest:     digest,
		Evaluator:         lineage.Evaluator,
		RouteEligibility:  eligibility,
		Available:         eligibility.Available,
		CapabilityGranted: eligibility.CapabilityGranted,
		SecurityAllowed:   eligibility.SecurityAllowed,
		Authorized:        eligibility.Authorized,
		PolicyAllowed:     eligibility.PolicyAllowed,
		BudgetAllowed:     eligibility.BudgetAllowed,
		QuotaAllowed:      eligibility.QuotaAllowed,
		Telemetry:         cloneSurfaceTelemetry(eligibility.Telemetry),
		EvaluatedAt:       eligibility.EvaluatedAt,
		ValidUntil:        eligibility.ValidUntil,
	}
	if !evidence.Available {
		evidence.ReasonCodes = append(evidence.ReasonCodes, ReasonUnavailable)
	}
	if !evidence.SecurityAllowed {
		evidence.ReasonCodes = append(evidence.ReasonCodes, ReasonSecurityDenied)
	}
	if !evidence.Authorized {
		evidence.ReasonCodes = append(evidence.ReasonCodes, ReasonAuthorityDenied)
	}
	if !evidence.PolicyAllowed {
		evidence.ReasonCodes = append(evidence.ReasonCodes, ReasonPolicyDenied)
	}
	if !evidence.CapabilityGranted {
		evidence.ReasonCodes = append(evidence.ReasonCodes, ReasonCapabilityMissing)
	}
	if !evidence.BudgetAllowed {
		evidence.ReasonCodes = append(evidence.ReasonCodes, ReasonBudgetExhausted)
	}
	if !evidence.QuotaAllowed {
		evidence.ReasonCodes = append(evidence.ReasonCodes, ReasonQuotaUnavailable)
	}
	evidence.ReasonCodes = uniqueSorted(evidence.ReasonCodes)
	evidence.ID = inferenceDigest(evidence)
	return evidence, lineage, nil
}

func cloneSurfaceTelemetry(values map[string]SurfaceTelemetryEvidence) map[string]SurfaceTelemetryEvidence {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]SurfaceTelemetryEvidence, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}

func verifyAndEvaluateSurface(request SurfaceRouteRequest, surface ExecutorSurface, evidence SurfaceEligibilityEvidence, lineage surfaceEvaluatorLineage, decidedAt time.Time) (SurfaceEvaluation, error) {
	canonical, digest, err := freezeExecutorSurface(surface)
	if err != nil {
		return SurfaceEvaluation{}, err
	}
	if err := lineage.validate(request.ID); err != nil {
		return SurfaceEvaluation{}, err
	}
	candidate := RouteCandidate{ExecutorID: canonical.ExecutorID, ProviderID: canonical.ProviderID, SurfaceID: canonical.ID, SurfaceDigest: digest}
	if err := verifyEligibility(evidence.RouteEligibility, request.RouteRequest, candidate); err != nil {
		return SurfaceEvaluation{}, err
	}
	expected, expectedLineage, err := composeSurfaceEligibilityEvidence(request, canonical, digest, evidence.RouteEligibility)
	if err != nil {
		return SurfaceEvaluation{}, errors.New("surface eligibility evidence is not frozen")
	}
	if expectedLineage != lineage {
		return SurfaceEvaluation{}, errors.New("surface eligibility evidence does not bind the decision authority lineage")
	}
	if expected.ID != evidence.ID || inferenceDigest(expected) != inferenceDigest(evidence) {
		return SurfaceEvaluation{}, errors.New("surface eligibility evidence is not frozen")
	}
	if evidence.RequestID != request.ID || evidence.SurfaceID != canonical.ID || evidence.SurfaceDigest != digest || evidence.Evaluator != lineage.Evaluator {
		return SurfaceEvaluation{}, errors.New("surface eligibility evidence does not bind request, metadata, and evaluator")
	}
	if evidence.EvaluatedAt.After(decidedAt) || !decidedAt.Before(evidence.ValidUntil) {
		return SurfaceEvaluation{}, errors.New("surface eligibility evidence is stale or from the future")
	}
	reasons := append([]string(nil), evidence.ReasonCodes...)
	add := func(reason string) { reasons = append(reasons, reason) }
	if !evidence.Available {
		add(ReasonUnavailable)
	}
	if !evidence.SecurityAllowed {
		add(ReasonSecurityDenied)
	}
	if !evidence.Authorized {
		add(ReasonAuthorityDenied)
	}
	if !evidence.PolicyAllowed {
		add(ReasonPolicyDenied)
	}
	if !evidence.CapabilityGranted || !containsAll(canonical.Capabilities, request.Target.Target.RequiredCapabilities) {
		add(ReasonCapabilityMissing)
	}
	if !evidence.BudgetAllowed {
		add(ReasonBudgetExhausted)
	}
	if !evidence.QuotaAllowed {
		add(ReasonQuotaUnavailable)
	}
	if len(request.Target.Target.TransportPolicy) > 0 && !containsTransport(request.Target.Target.TransportPolicy, canonical.Transport) {
		add(ReasonTransportDenied)
	}
	if request.Target.Target.APIPolicy == contracts.APIPolicyForbid && canonical.Transport == contracts.TransportMeteredAPI {
		add(ReasonAPIForbidden)
	}
	if request.Target.Target.APIPolicy == contracts.APIPolicyRequired && canonical.Transport != contracts.TransportMeteredAPI {
		add(ReasonAPIRequired)
	}
	if intersects(canonical.Profiles, request.Target.Target.ProhibitedProfiles) {
		add(ReasonProfileProhibited)
	}
	for _, name := range request.Target.Target.TelemetryRequirements {
		telemetry, ok := evidence.Telemetry[name]
		if !ok {
			return SurfaceEvaluation{}, errors.New("eligibility authority omitted required telemetry evidence")
		}
		if !decidedAt.Before(telemetry.ValidUntil) {
			return SurfaceEvaluation{}, errors.New("surface telemetry evidence is stale")
		}
		if !telemetry.Known {
			add(ReasonTelemetryUnknown + ":" + name)
		}
	}
	reasons = uniqueSorted(reasons)
	return SurfaceEvaluation{Surface: canonical, Evidence: evidence, Eligible: len(reasons) == 0, ReasonCodes: reasons}, nil
}

func buildSurfaceDecision(request SurfaceRouteRequest, lineage surfaceEvaluatorLineage, evaluations []SurfaceEvaluation, decidedAt time.Time) (SurfaceRoutingDecision, error) {
	if len(evaluations) == 0 {
		return SurfaceRoutingDecision{}, errors.New("surface routing decision requires governed eligibility evidence")
	}
	ordered := append([]SurfaceEvaluation(nil), evaluations...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Surface.ID < ordered[j].Surface.ID })
	seen := map[string]bool{}
	eligible := []ExecutorSurface{}
	for _, evaluation := range ordered {
		if seen[evaluation.Surface.ID] {
			return SurfaceRoutingDecision{}, errors.New("surface evaluations require unique identity")
		}
		seen[evaluation.Surface.ID] = true
		if evaluation.Eligible {
			eligible = append(eligible, evaluation.Surface)
		}
	}
	decision := SurfaceRoutingDecision{Version: surfaceDecisionVersions.CurrentVersion(), Request: request, Evaluator: lineage.Evaluator, EvaluatorGenerationRef: lineage.GenerationRef, EvaluatorGenerationVersion: lineage.GenerationVersion, EvaluatorGenerationDigest: lineage.GenerationDigest, EvaluatorScope: lineage.Scope, Evaluations: ordered, DecidedAt: decidedAt}
	selectSurfaceAffinity(&decision, request.Target.Target, eligible, ordered)
	decision.ID = inferenceDigest(decision)
	return decision, nil
}

func selectSurfaceAffinity(decision *SurfaceRoutingDecision, target contracts.ExecutionTarget, eligible []ExecutorSurface, evaluations []SurfaceEvaluation) {
	if len(eligible) == 0 {
		if len(target.RequiredProfiles) > 0 {
			decision.Outcome, decision.ReasonCodes = SurfaceRequiredUnavailable, []string{string(SurfaceRequiredUnavailable)}
		} else {
			setUnavailableOutcome(decision, evaluations)
		}
		return
	}
	pool := eligible
	if len(target.RequiredProfiles) > 0 {
		pool = matchingSurfaces(eligible, target.RequiredProfiles, true)
		if len(pool) == 0 {
			decision.Outcome, decision.ReasonCodes = SurfaceRequiredUnavailable, []string{string(SurfaceRequiredUnavailable)}
			return
		}
	}
	if len(target.PreferredProfiles) > 0 {
		if preferred, profile := bestProfilePool(pool, target.PreferredProfiles); len(preferred) > 0 {
			chooseSurface(decision, target, preferred, profile, false, ReasonPreferredMatch)
			return
		}
		fallback, profile := bestProfilePool(pool, target.AllowedFallbackProfiles)
		if len(fallback) == 0 {
			if len(target.AllowedFallbackProfiles) == 0 {
				decision.Outcome, decision.ReasonCodes = SurfaceFallbackProhibited, []string{string(SurfaceFallbackProhibited)}
			} else {
				setUnavailableOutcome(decision, matchingEvaluations(evaluations, target.AllowedFallbackProfiles))
			}
			return
		}
		chooseSurface(decision, target, fallback, profile, true, ReasonAllowedFallback)
		return
	}
	chooseSurface(decision, target, pool, "", false, ReasonDeterministic)
}

func setUnavailableOutcome(decision *SurfaceRoutingDecision, evaluations []SurfaceEvaluation) {
	for _, evaluation := range evaluations {
		if contains(evaluation.ReasonCodes, ReasonAPIForbidden) {
			decision.Outcome, decision.ReasonCodes = SurfaceAPIUseProhibited, []string{string(SurfaceAPIUseProhibited)}
			return
		}
	}
	for _, evaluation := range evaluations {
		for _, reason := range evaluation.ReasonCodes {
			if len(reason) >= len(ReasonTelemetryUnknown) && reason[:len(ReasonTelemetryUnknown)] == ReasonTelemetryUnknown {
				decision.Outcome, decision.ReasonCodes = SurfaceTelemetryUnsatisfied, []string{string(SurfaceTelemetryUnsatisfied)}
				return
			}
		}
	}
	decision.Outcome, decision.ReasonCodes = SurfaceNoEligible, []string{string(SurfaceNoEligible)}
}

func matchingEvaluations(evaluations []SurfaceEvaluation, profiles []string) []SurfaceEvaluation {
	out := []SurfaceEvaluation{}
	for _, evaluation := range evaluations {
		if intersects(evaluation.Surface.Profiles, profiles) {
			out = append(out, evaluation)
		}
	}
	return out
}

func chooseSurface(decision *SurfaceRoutingDecision, target contracts.ExecutionTarget, pool []ExecutorSurface, profile string, fallback bool, reason string) {
	sort.Slice(pool, func(i, j int) bool {
		if target.APIPolicy == contracts.APIPolicyPreferNot && (pool[i].Transport == contracts.TransportMeteredAPI) != (pool[j].Transport == contracts.TransportMeteredAPI) {
			return pool[i].Transport != contracts.TransportMeteredAPI
		}
		return pool[i].ID < pool[j].ID
	})
	decision.Outcome, decision.SelectedSurfaceID, decision.SelectedProfile, decision.Fallback = SurfaceSelected, pool[0].ID, profile, fallback
	decision.ReasonCodes = []string{reason}
}

func matchingSurfaces(surfaces []ExecutorSurface, profiles []string, all bool) []ExecutorSurface {
	out := []ExecutorSurface{}
	for _, surface := range surfaces {
		match := intersects(surface.Profiles, profiles)
		if all {
			match = containsAll(surface.Profiles, profiles)
		}
		if match {
			out = append(out, surface)
		}
	}
	return out
}
func bestProfilePool(surfaces []ExecutorSurface, profiles []string) ([]ExecutorSurface, string) {
	for _, profile := range profiles {
		if pool := matchingSurfaces(surfaces, []string{profile}, false); len(pool) > 0 {
			return pool, profile
		}
	}
	return nil, ""
}
func containsAll(values, required []string) bool {
	for _, requiredValue := range required {
		if !contains(values, requiredValue) {
			return false
		}
	}
	return true
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func intersects(left, right []string) bool {
	for _, value := range left {
		if contains(right, value) {
			return true
		}
	}
	return false
}
func containsTransport(values []contracts.TransportClass, wanted contracts.TransportClass) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func freezeExecutorSurface(surface ExecutorSurface) (ExecutorSurface, string, error) {
	surface.Capabilities = uniqueSorted(surface.Capabilities)
	surface.Profiles = uniqueSorted(surface.Profiles)
	if surface.ID == "" || surface.ExecutorID == "" || surface.ProviderID == "" || !validSurfaceTransport(surface.Transport) || contains(surface.Capabilities, "") || contains(surface.Profiles, "") {
		return ExecutorSurface{}, "", errors.New("executor surface metadata is incomplete")
	}
	return surface, inferenceDigest(surface), nil
}

func uniqueSorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	if len(out) < 2 {
		return out
	}
	write := 1
	for read := 1; read < len(out); read++ {
		if out[read] != out[write-1] {
			out[write], write = out[read], write+1
		}
	}
	return out[:write]
}

func validSurfaceTransport(transport contracts.TransportClass) bool {
	switch transport {
	case contracts.TransportSubscriptionCLI, contracts.TransportMeteredAPI, contracts.TransportLocal, contracts.TransportPlugin:
		return true
	default:
		return false
	}
}

func validSurfaceEvaluator(evaluator contracts.PrincipalRef) bool {
	return evaluator.Validate() == nil && evaluator.Kind == "authority"
}
