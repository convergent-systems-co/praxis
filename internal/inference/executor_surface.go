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
	Target          contracts.EffectiveExecutionTarget `json:"effective_execution_target"`
}

type SurfaceTelemetryEvidence struct {
	Known         bool      `json:"known"`
	Value         string    `json:"value,omitempty"`
	ProvenanceRef string    `json:"provenance_ref"`
	ObservedAt    time.Time `json:"observed_at"`
	ValidUntil    time.Time `json:"valid_until"`
}

// SurfaceEligibilityEvidence is issued for one exact request and canonical
// surface description. Its digest makes caller mutation detectable.
type SurfaceEligibilityEvidence struct {
	ID                    string                              `json:"id"`
	Version               string                              `json:"version"`
	RequestID             string                              `json:"request_id"`
	SurfaceID             string                              `json:"surface_id"`
	SurfaceDigest         string                              `json:"surface_digest"`
	Evaluator             contracts.PrincipalRef              `json:"evaluator"`
	Available             bool                                `json:"available"`
	CapabilityGranted     bool                                `json:"capability_granted"`
	SecurityAllowed       bool                                `json:"security_allowed"`
	Authorized            bool                                `json:"authorized"`
	PolicyAllowed         bool                                `json:"policy_allowed"`
	ReasonCodes           []string                            `json:"reason_codes,omitempty"`
	CapabilityEvidenceRef string                              `json:"capability_evidence_ref"`
	SecurityEvidenceRef   string                              `json:"security_evidence_ref"`
	AuthorityEvidenceRef  string                              `json:"authority_evidence_ref"`
	PolicyEvidenceRef     string                              `json:"policy_evidence_ref"`
	AvailabilityRef       string                              `json:"availability_evidence_ref"`
	Telemetry             map[string]SurfaceTelemetryEvidence `json:"telemetry,omitempty"`
	EvaluatedAt           time.Time                           `json:"evaluated_at"`
	ValidUntil            time.Time                           `json:"valid_until"`
}

type SurfaceEligibilityAuthority interface {
	Evaluator() contracts.PrincipalRef
	EvaluateSurfaceEligibility(context.Context, SurfaceRouteRequest, ExecutorSurface) (SurfaceEligibilityEvidence, error)
}

type SurfaceEvaluation struct {
	Surface     ExecutorSurface            `json:"surface"`
	Evidence    SurfaceEligibilityEvidence `json:"eligibility_evidence"`
	Eligible    bool                       `json:"eligible"`
	ReasonCodes []string                   `json:"reason_codes,omitempty"`
}

type SurfaceRoutingDecision struct {
	ID                string                 `json:"id"`
	Version           string                 `json:"version"`
	Request           SurfaceRouteRequest    `json:"request"`
	Evaluator         contracts.PrincipalRef `json:"evaluator"`
	Evaluations       []SurfaceEvaluation    `json:"evaluations"`
	Outcome           SurfaceDecisionOutcome `json:"outcome"`
	SelectedSurfaceID string                 `json:"selected_surface_id,omitempty"`
	SelectedProfile   string                 `json:"selected_profile,omitempty"`
	Fallback          bool                   `json:"fallback"`
	ReasonCodes       []string               `json:"reason_codes"`
	DecidedAt         time.Time              `json:"decided_at"`
}

func FreezeSurfaceRouteRequest(request SurfaceRouteRequest) (SurfaceRouteRequest, error) {
	request.ID = ""
	frozenRoute, err := FreezeRouteRequest(request.RouteRequest)
	if err != nil || frozenRoute.ID != request.RouteRequest.ID || inferenceDigest(frozenRoute) != inferenceDigest(request.RouteRequest) || request.AgentGeneration == "" {
		return SurfaceRouteRequest{}, errors.New("surface route request requires frozen route lineage and agent generation")
	}
	if err := request.Target.Target.Validate(); err != nil || !validTargetAuthorities(request.Target.Authorities) {
		return SurfaceRouteRequest{}, errors.New("surface route request requires a valid effective execution target")
	}
	if tier := request.Target.Target.ReasoningTier; tier != "" && tier != string(request.RouteRequest.Tier) {
		return SurfaceRouteRequest{}, errors.New("surface target reasoning tier does not match route request")
	}
	canonicalizeTarget(&request.Target)
	request.ID = inferenceDigest(request)
	return request, nil
}

func FreezeSurfaceEligibilityEvidence(evidence SurfaceEligibilityEvidence) (SurfaceEligibilityEvidence, error) {
	evidence.ID, evidence.Version = "", "v2"
	evidence.ReasonCodes = uniqueSorted(evidence.ReasonCodes)
	if evidence.RequestID == "" || evidence.SurfaceID == "" || evidence.SurfaceDigest == "" || !validSurfaceEvaluator(evidence.Evaluator) || evidence.CapabilityEvidenceRef == "" || evidence.SecurityEvidenceRef == "" || evidence.AuthorityEvidenceRef == "" || evidence.PolicyEvidenceRef == "" || evidence.AvailabilityRef == "" || evidence.EvaluatedAt.IsZero() || !evidence.ValidUntil.After(evidence.EvaluatedAt) {
		return SurfaceEligibilityEvidence{}, errors.New("surface eligibility evidence is incomplete")
	}
	if (!evidence.Available || !evidence.CapabilityGranted || !evidence.SecurityAllowed || !evidence.Authorized || !evidence.PolicyAllowed) && len(evidence.ReasonCodes) == 0 {
		return SurfaceEligibilityEvidence{}, errors.New("denied surface eligibility requires reasons")
	}
	for name, telemetry := range evidence.Telemetry {
		if name == "" || telemetry.ProvenanceRef == "" || telemetry.ObservedAt.IsZero() || !telemetry.ValidUntil.After(telemetry.ObservedAt) || telemetry.ObservedAt.After(evidence.EvaluatedAt) {
			return SurfaceEligibilityEvidence{}, errors.New("surface telemetry evidence is incomplete")
		}
		if telemetry.Known == (telemetry.Value == "") {
			return SurfaceEligibilityEvidence{}, errors.New("surface telemetry known state and value disagree")
		}
	}
	evidence.ID = inferenceDigest(evidence)
	return evidence, nil
}

func SelectExecutorSurface(ctx context.Context, request SurfaceRouteRequest, surfaces []ExecutorSurface, authority SurfaceEligibilityAuthority, decidedAt time.Time) (SurfaceRoutingDecision, error) {
	frozen, err := FreezeSurfaceRouteRequest(request)
	if err != nil || frozen.ID != request.ID || inferenceDigest(frozen) != inferenceDigest(request) {
		return SurfaceRoutingDecision{}, errors.New("surface route request is not frozen")
	}
	if authority == nil || !validSurfaceEvaluator(authority.Evaluator()) || decidedAt.IsZero() {
		return SurfaceRoutingDecision{}, errors.New("surface routing requires evaluator authority and decision time")
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
	for _, surface := range ordered {
		if seen[surface.ID] {
			return SurfaceRoutingDecision{}, errors.New("executor surfaces require unique identity")
		}
		seen[surface.ID] = true
		evidence, err := authority.EvaluateSurfaceEligibility(ctx, request, surface)
		if err != nil {
			return SurfaceRoutingDecision{}, err
		}
		evaluation, err := verifyAndEvaluateSurface(request, surface, evidence, authority.Evaluator(), decidedAt)
		if err != nil {
			return SurfaceRoutingDecision{}, err
		}
		evaluations = append(evaluations, evaluation)
	}
	return buildSurfaceDecision(request, authority.Evaluator(), evaluations, decidedAt)
}

func VerifySurfaceRoutingDecision(decision SurfaceRoutingDecision) error {
	if _, _, err := surfaceDecisionVersions.Canonicalize(decision.Version, nil); err != nil {
		return err
	}
	if decision.ID == "" || decision.Version != surfaceDecisionVersions.CurrentVersion() || decision.DecidedAt.IsZero() || !validSurfaceEvaluator(decision.Evaluator) {
		return errors.New("surface routing decision v2 identity, evaluator, and time are required")
	}
	frozen, err := FreezeSurfaceRouteRequest(decision.Request)
	if err != nil || frozen.ID != decision.Request.ID || inferenceDigest(frozen) != inferenceDigest(decision.Request) {
		return errors.New("surface routing decision request is not frozen")
	}
	evaluations := make([]SurfaceEvaluation, 0, len(decision.Evaluations))
	for _, persisted := range decision.Evaluations {
		evaluation, err := verifyAndEvaluateSurface(decision.Request, persisted.Surface, persisted.Evidence, decision.Evaluator, decision.DecidedAt)
		if err != nil {
			return err
		}
		evaluations = append(evaluations, evaluation)
	}
	recomputed, err := buildSurfaceDecision(decision.Request, decision.Evaluator, evaluations, decision.DecidedAt)
	if err != nil || recomputed.ID != decision.ID || inferenceDigest(recomputed) != inferenceDigest(decision) {
		return errors.New("surface routing decision digest or semantics mismatch")
	}
	return nil
}

func verifyAndEvaluateSurface(request SurfaceRouteRequest, surface ExecutorSurface, evidence SurfaceEligibilityEvidence, evaluator contracts.PrincipalRef, decidedAt time.Time) (SurfaceEvaluation, error) {
	canonical, digest, err := freezeExecutorSurface(surface)
	if err != nil {
		return SurfaceEvaluation{}, err
	}
	frozenEvidence, err := FreezeSurfaceEligibilityEvidence(evidence)
	if err != nil || frozenEvidence.ID != evidence.ID || inferenceDigest(frozenEvidence) != inferenceDigest(evidence) {
		return SurfaceEvaluation{}, errors.New("surface eligibility evidence is not frozen")
	}
	if evidence.RequestID != request.ID || evidence.SurfaceID != canonical.ID || evidence.SurfaceDigest != digest || evidence.Evaluator != evaluator {
		return SurfaceEvaluation{}, errors.New("surface eligibility evidence does not bind request, metadata, and evaluator")
	}
	if evidence.EvaluatedAt.After(decidedAt) || evidence.ValidUntil.Before(decidedAt) {
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
		if telemetry.ValidUntil.Before(decidedAt) {
			return SurfaceEvaluation{}, errors.New("surface telemetry evidence is stale")
		}
		if !telemetry.Known {
			add(ReasonTelemetryUnknown + ":" + name)
		}
	}
	reasons = uniqueSorted(reasons)
	return SurfaceEvaluation{Surface: canonical, Evidence: evidence, Eligible: len(reasons) == 0, ReasonCodes: reasons}, nil
}

func buildSurfaceDecision(request SurfaceRouteRequest, evaluator contracts.PrincipalRef, evaluations []SurfaceEvaluation, decidedAt time.Time) (SurfaceRoutingDecision, error) {
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
	decision := SurfaceRoutingDecision{Version: surfaceDecisionVersions.CurrentVersion(), Request: request, Evaluator: evaluator, Evaluations: ordered, DecidedAt: decidedAt}
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

func canonicalizeTarget(target *contracts.EffectiveExecutionTarget) {
	target.Target.RequiredCapabilities = uniqueSorted(target.Target.RequiredCapabilities)
	target.Target.RequiredProfiles = uniqueSorted(target.Target.RequiredProfiles)
	target.Target.PreferredProfiles = uniqueSorted(target.Target.PreferredProfiles)
	target.Target.AllowedFallbackProfiles = uniqueSorted(target.Target.AllowedFallbackProfiles)
	target.Target.ProhibitedProfiles = uniqueSorted(target.Target.ProhibitedProfiles)
	target.Target.TelemetryRequirements = uniqueSorted(target.Target.TelemetryRequirements)
	target.Target.TransportPolicy = append([]contracts.TransportClass(nil), target.Target.TransportPolicy...)
	target.Authorities = append([]contracts.TargetAuthority(nil), target.Authorities...)
	sort.Slice(target.Target.TransportPolicy, func(i, j int) bool { return target.Target.TransportPolicy[i] < target.Target.TransportPolicy[j] })
	sort.Slice(target.Authorities, func(i, j int) bool { return target.Authorities[i] < target.Authorities[j] })
}

func validTargetAuthorities(authorities []contracts.TargetAuthority) bool {
	if len(authorities) == 0 {
		return false
	}
	seen := map[contracts.TargetAuthority]bool{}
	for _, authority := range authorities {
		if seen[authority] {
			return false
		}
		seen[authority] = true
		switch authority {
		case contracts.AuthorityPlatformSecurity, contracts.AuthorityOrganization, contracts.AuthorityOperator, contracts.AuthorityPackage, contracts.AuthorityAgent, contracts.AuthorityGraph, contracts.AuthorityNode, contracts.AuthorityLearned:
		default:
			return false
		}
	}
	return true
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
