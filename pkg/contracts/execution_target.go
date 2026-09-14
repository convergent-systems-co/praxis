package contracts

import (
	"errors"
	"fmt"
)

type TransportClass string

const (
	TransportSubscriptionCLI TransportClass = "subscription_cli"
	TransportMeteredAPI      TransportClass = "metered_api"
	TransportLocal           TransportClass = "local"
	TransportPlugin          TransportClass = "other_plugin_transport"
)

type APIPolicy string

const (
	APIPolicyForbid    APIPolicy = "forbid"
	APIPolicyAllow     APIPolicy = "allow"
	APIPolicyPreferNot APIPolicy = "prefer_not"
	APIPolicyRequired  APIPolicy = "required"
)

// ExecutionTarget is provider-neutral routing intent. Provider/model names
// belong to executor-surface metadata, not this contract.
type ExecutionTarget struct {
	Version                 string           `json:"version"`
	RequiredCapabilities    []string         `json:"required_capabilities,omitempty"`
	RequiredProfiles        []string         `json:"required_profiles,omitempty"`
	PreferredProfiles       []string         `json:"preferred_profiles,omitempty"`
	AllowedFallbackProfiles []string         `json:"allowed_fallback_profiles,omitempty"`
	ProhibitedProfiles      []string         `json:"prohibited_profiles,omitempty"`
	ReasoningTier           string           `json:"reasoning_tier,omitempty"`
	TransportPolicy         []TransportClass `json:"transport_policy,omitempty"`
	APIPolicy               APIPolicy        `json:"api_policy"`
	BudgetProfile           string           `json:"budget_profile,omitempty"`
	TelemetryRequirements   []string         `json:"telemetry_requirements,omitempty"`
	SourceAuthority         string           `json:"source_authority"`
	Scope                   string           `json:"scope"`
}

var (
	ErrInvalidExecutionTarget = errors.New("invalid execution target")
	ErrTargetConflict         = errors.New("execution target contains conflicting profile constraints")
)

func (t ExecutionTarget) Validate() error {
	if t.Version != "1" || t.SourceAuthority == "" || t.Scope == "" {
		return fmt.Errorf("%w: version, source authority, and scope are required", ErrInvalidExecutionTarget)
	}
	switch t.APIPolicy {
	case APIPolicyForbid, APIPolicyAllow, APIPolicyPreferNot, APIPolicyRequired:
	default:
		return fmt.Errorf("%w: unknown API policy %q", ErrInvalidExecutionTarget, t.APIPolicy)
	}
	for _, transport := range t.TransportPolicy {
		switch transport {
		case TransportSubscriptionCLI, TransportMeteredAPI, TransportLocal, TransportPlugin:
		default:
			return fmt.Errorf("%w: unknown transport %q", ErrInvalidExecutionTarget, transport)
		}
	}
	if duplicate := duplicateValue(t.RequiredProfiles); duplicate != "" {
		return fmt.Errorf("%w: duplicate required profile %q", ErrInvalidExecutionTarget, duplicate)
	}
	if duplicate := duplicateValue(t.ProhibitedProfiles); duplicate != "" {
		return fmt.Errorf("%w: duplicate prohibited profile %q", ErrInvalidExecutionTarget, duplicate)
	}
	for _, required := range t.RequiredProfiles {
		for _, prohibited := range t.ProhibitedProfiles {
			if required == prohibited {
				return fmt.Errorf("%w: %q is both required and prohibited", ErrTargetConflict, required)
			}
		}
	}
	if t.APIPolicy == APIPolicyForbid {
		for _, transport := range t.TransportPolicy {
			if transport == TransportMeteredAPI {
				return fmt.Errorf("%w: metered API transport conflicts with forbid policy", ErrTargetConflict)
			}
		}
	}
	if t.APIPolicy == APIPolicyRequired {
		found := false
		for _, transport := range t.TransportPolicy {
			if transport == TransportMeteredAPI {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: required API policy needs metered API transport", ErrTargetConflict)
		}
	}
	return nil
}

func duplicateValue(values []string) string {
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			return value
		}
		seen[value] = true
	}
	return ""
}
