package inference

import (
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

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

// TelemetryValue distinguishes unknown telemetry from a known value. Unknown
// data is never synthesized into a zero or another routing input.
type TelemetryValue struct {
	Known bool   `json:"known"`
	Value string `json:"value,omitempty"`
}

// ExecutorSurface describes an invocation surface. Provider and model are
// descriptive metadata only; selection operates on neutral capabilities,
// profiles, transport, and authoritative eligibility evidence.
type ExecutorSurface struct {
	ID                    string                    `json:"id"`
	ProviderMetadata      string                    `json:"provider_metadata,omitempty"`
	ModelMetadata         string                    `json:"model_metadata,omitempty"`
	Capabilities          []string                  `json:"capabilities"`
	Profiles              []string                  `json:"profiles"`
	Transport             contracts.TransportClass  `json:"transport"`
	Available             bool                      `json:"available"`
	SecurityEligible      bool                      `json:"security_eligible"`
	AuthorityEligible     bool                      `json:"authority_eligible"`
	PolicyEligible        bool                      `json:"policy_eligible"`
	CapabilityEvidenceRef string                    `json:"capability_evidence_ref"`
	SecurityEvidenceRef   string                    `json:"security_evidence_ref"`
	AuthorityEvidenceRef  string                    `json:"authority_evidence_ref"`
	PolicyEvidenceRef     string                    `json:"policy_evidence_ref"`
	Telemetry             map[string]TelemetryValue `json:"telemetry,omitempty"`
}

// SurfaceRouteRequest binds persistent agent identity to routing intent. A
// selected surface remains decision evidence and never replaces agent identity.
type SurfaceRouteRequest struct {
	ID              string                             `json:"id"`
	SubjectAgentID  string                             `json:"subject_agent_id"`
	AgentGeneration string                             `json:"agent_generation"`
	RunID           string                             `json:"run_id"`
	Target          contracts.EffectiveExecutionTarget `json:"effective_execution_target"`
}

type SurfaceEvaluation struct {
	SurfaceID             string                    `json:"surface_id"`
	ProviderMetadata      string                    `json:"provider_metadata,omitempty"`
	ModelMetadata         string                    `json:"model_metadata,omitempty"`
	Capabilities          []string                  `json:"capabilities"`
	Profiles              []string                  `json:"profiles"`
	Transport             contracts.TransportClass  `json:"transport"`
	Available             bool                      `json:"available"`
	SecurityEligible      bool                      `json:"security_eligible"`
	AuthorityEligible     bool                      `json:"authority_eligible"`
	PolicyEligible        bool                      `json:"policy_eligible"`
	CapabilityEvidenceRef string                    `json:"capability_evidence_ref"`
	SecurityEvidenceRef   string                    `json:"security_evidence_ref"`
	AuthorityEvidenceRef  string                    `json:"authority_evidence_ref"`
	PolicyEvidenceRef     string                    `json:"policy_evidence_ref"`
	Eligible              bool                      `json:"eligible"`
	ReasonCodes           []string                  `json:"reason_codes,omitempty"`
	Telemetry             map[string]TelemetryValue `json:"required_telemetry,omitempty"`
}

type SurfaceRoutingDecision struct {
	ID                string                 `json:"id"`
	Version           string                 `json:"version"`
	Request           SurfaceRouteRequest    `json:"request"`
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
	if request.SubjectAgentID == "" || request.AgentGeneration == "" || request.RunID == "" {
		return SurfaceRouteRequest{}, errors.New("surface route request requires persistent agent, generation, and run identity")
	}
	if err := request.Target.Target.Validate(); err != nil || !validTargetAuthorities(request.Target.Authorities) {
		return SurfaceRouteRequest{}, errors.New("surface route request requires a valid effective execution target")
	}
	canonicalizeTarget(&request.Target)
	request.ID = inferenceDigest(request)
	return request, nil
}

func SelectExecutorSurface(request SurfaceRouteRequest, surfaces []ExecutorSurface, decidedAt time.Time) (SurfaceRoutingDecision, error) {
	frozen, err := FreezeSurfaceRouteRequest(request)
	if err != nil || frozen.ID != request.ID || inferenceDigest(frozen) != inferenceDigest(request) {
		return SurfaceRoutingDecision{}, errors.New("surface route request is not frozen")
	}
	if decidedAt.IsZero() {
		return SurfaceRoutingDecision{}, errors.New("surface routing decision time is required")
	}
	ordered := append([]ExecutorSurface(nil), surfaces...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	seen := map[string]bool{}
	evaluations := make([]SurfaceEvaluation, 0, len(ordered))
	eligible := make([]ExecutorSurface, 0, len(ordered))
	for _, surface := range ordered {
		if surface.ID == "" || seen[surface.ID] || !validSurfaceTransport(surface.Transport) || surface.CapabilityEvidenceRef == "" || surface.SecurityEvidenceRef == "" || surface.AuthorityEvidenceRef == "" || surface.PolicyEvidenceRef == "" {
			return SurfaceRoutingDecision{}, errors.New("executor surfaces require unique identity and complete eligibility evidence references")
		}
		seen[surface.ID] = true
		evaluation := evaluateSurface(request.Target.Target, surface)
		evaluations = append(evaluations, evaluation)
		if evaluation.Eligible {
			eligible = append(eligible, surface)
		}
	}
	decision := SurfaceRoutingDecision{Version: "v1", Request: request, Evaluations: evaluations, DecidedAt: decidedAt}
	selectSurfaceAffinity(&decision, request.Target.Target, eligible, evaluations)
	decision.ID = inferenceDigest(decision)
	return decision, nil
}

func VerifySurfaceRoutingDecision(decision SurfaceRoutingDecision) error {
	id := decision.ID
	if id == "" || decision.Version != "v1" || decision.DecidedAt.IsZero() {
		return errors.New("surface routing decision identity, version, and time are required")
	}
	frozen, err := FreezeSurfaceRouteRequest(decision.Request)
	if err != nil || frozen.ID != decision.Request.ID || inferenceDigest(frozen) != inferenceDigest(decision.Request) {
		return errors.New("surface routing decision request is not frozen")
	}
	recomputed, err := SelectExecutorSurface(decision.Request, evaluationsAsSurfaces(decision.Evaluations), decision.DecidedAt)
	copy := decision
	copy.ID = ""
	if err != nil || recomputed.ID != id || inferenceDigest(copy) != id || !validDecisionShape(decision) {
		return errors.New("surface routing decision digest or shape mismatch")
	}
	return nil
}

func evaluateSurface(target contracts.ExecutionTarget, surface ExecutorSurface) SurfaceEvaluation {
	e := SurfaceEvaluation{
		SurfaceID: surface.ID, ProviderMetadata: surface.ProviderMetadata, ModelMetadata: surface.ModelMetadata, Transport: surface.Transport,
		Capabilities: append([]string(nil), surface.Capabilities...), Profiles: append([]string(nil), surface.Profiles...),
		Available: surface.Available, SecurityEligible: surface.SecurityEligible, AuthorityEligible: surface.AuthorityEligible, PolicyEligible: surface.PolicyEligible,
		CapabilityEvidenceRef: surface.CapabilityEvidenceRef, SecurityEvidenceRef: surface.SecurityEvidenceRef,
		AuthorityEvidenceRef: surface.AuthorityEvidenceRef, PolicyEvidenceRef: surface.PolicyEvidenceRef,
		Telemetry: map[string]TelemetryValue{},
	}
	sort.Strings(e.Capabilities)
	sort.Strings(e.Profiles)
	add := func(reason string) { e.ReasonCodes = append(e.ReasonCodes, reason) }
	if !surface.Available {
		add(ReasonUnavailable)
	}
	if !surface.SecurityEligible {
		add(ReasonSecurityDenied)
	}
	if !surface.AuthorityEligible {
		add(ReasonAuthorityDenied)
	}
	if !surface.PolicyEligible {
		add(ReasonPolicyDenied)
	}
	if !containsAll(surface.Capabilities, target.RequiredCapabilities) {
		add(ReasonCapabilityMissing)
	}
	if len(target.TransportPolicy) > 0 && !containsTransport(target.TransportPolicy, surface.Transport) {
		add(ReasonTransportDenied)
	}
	if target.APIPolicy == contracts.APIPolicyForbid && surface.Transport == contracts.TransportMeteredAPI {
		add(ReasonAPIForbidden)
	}
	if target.APIPolicy == contracts.APIPolicyRequired && surface.Transport != contracts.TransportMeteredAPI {
		add(ReasonAPIRequired)
	}
	if intersects(surface.Profiles, target.ProhibitedProfiles) {
		add(ReasonProfileProhibited)
	}
	for _, name := range target.TelemetryRequirements {
		value, ok := surface.Telemetry[name]
		if !ok || !value.Known {
			value = TelemetryValue{Known: false}
			add(ReasonTelemetryUnknown + ":" + name)
		}
		e.Telemetry[name] = value
	}
	e.Eligible = len(e.ReasonCodes) == 0
	return e
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
		if intersects(evaluation.Profiles, profiles) {
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

func canonicalizeTarget(target *contracts.EffectiveExecutionTarget) {
	target.Target.RequiredCapabilities = append([]string(nil), target.Target.RequiredCapabilities...)
	target.Target.RequiredProfiles = append([]string(nil), target.Target.RequiredProfiles...)
	target.Target.PreferredProfiles = append([]string(nil), target.Target.PreferredProfiles...)
	target.Target.AllowedFallbackProfiles = append([]string(nil), target.Target.AllowedFallbackProfiles...)
	target.Target.ProhibitedProfiles = append([]string(nil), target.Target.ProhibitedProfiles...)
	target.Target.TelemetryRequirements = append([]string(nil), target.Target.TelemetryRequirements...)
	target.Target.TransportPolicy = append([]contracts.TransportClass(nil), target.Target.TransportPolicy...)
	target.Authorities = append([]contracts.TargetAuthority(nil), target.Authorities...)
	sort.Strings(target.Target.RequiredCapabilities)
	sort.Strings(target.Target.RequiredProfiles)
	sort.Strings(target.Target.PreferredProfiles)
	sort.Strings(target.Target.AllowedFallbackProfiles)
	sort.Strings(target.Target.ProhibitedProfiles)
	sort.Strings(target.Target.TelemetryRequirements)
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

func validDecisionShape(decision SurfaceRoutingDecision) bool {
	if decision.Request.ID == "" || len(decision.ReasonCodes) == 0 {
		return false
	}
	for _, evaluation := range decision.Evaluations {
		if evaluation.SurfaceID == "" || evaluation.CapabilityEvidenceRef == "" || evaluation.SecurityEvidenceRef == "" || evaluation.AuthorityEvidenceRef == "" || evaluation.PolicyEvidenceRef == "" {
			return false
		}
	}
	if decision.Outcome == SurfaceSelected {
		return decision.SelectedSurfaceID != ""
	}
	return decision.SelectedSurfaceID == "" && (decision.Outcome == SurfaceNoEligible || decision.Outcome == SurfaceRequiredUnavailable || decision.Outcome == SurfaceFallbackProhibited || decision.Outcome == SurfaceAPIUseProhibited || decision.Outcome == SurfaceTelemetryUnsatisfied)
}

func evaluationsAsSurfaces(evaluations []SurfaceEvaluation) []ExecutorSurface {
	out := make([]ExecutorSurface, 0, len(evaluations))
	for _, evaluation := range evaluations {
		out = append(out, ExecutorSurface{
			ID: evaluation.SurfaceID, ProviderMetadata: evaluation.ProviderMetadata, ModelMetadata: evaluation.ModelMetadata,
			Capabilities: append([]string(nil), evaluation.Capabilities...), Profiles: append([]string(nil), evaluation.Profiles...), Transport: evaluation.Transport,
			Available: evaluation.Available, SecurityEligible: evaluation.SecurityEligible, AuthorityEligible: evaluation.AuthorityEligible, PolicyEligible: evaluation.PolicyEligible,
			CapabilityEvidenceRef: evaluation.CapabilityEvidenceRef, SecurityEvidenceRef: evaluation.SecurityEvidenceRef,
			AuthorityEvidenceRef: evaluation.AuthorityEvidenceRef, PolicyEvidenceRef: evaluation.PolicyEvidenceRef,
			Telemetry: evaluation.Telemetry,
		})
	}
	return out
}
