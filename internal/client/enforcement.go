package client

import "fmt"

type EnforcementState string

const (
	ClientEnforced    EnforcementState = "enforced"
	ClientNotEnforced EnforcementState = "not_enforced"
	ClientUnknown     EnforcementState = "unknown"
)

type EnforcementProfile struct {
	ClientID   string
	Properties map[string]EnforcementState
	BypassPaths []string
}

func (p EnforcementProfile) Require(properties []string, requireExclusiveMediation bool) error {
	if p.ClientID == "" {
		return fmt.Errorf("client id is required")
	}
	for _, property := range properties {
		state, ok := p.Properties[property]
		if !ok || state == ClientUnknown {
			return fmt.Errorf("required client enforcement %q is unknown/unavailable", property)
		}
		if state != ClientEnforced {
			return fmt.Errorf("required client enforcement %q is not enforced", property)
		}
	}
	if requireExclusiveMediation && len(p.BypassPaths) > 0 {
		return fmt.Errorf("client has unmediated bypass paths: %v", p.BypassPaths)
	}
	return nil
}
