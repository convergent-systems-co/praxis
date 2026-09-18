package plugin

import "fmt"

type EnforcementState string

const (
	Enforced    EnforcementState = "enforced"
	NotEnforced EnforcementState = "not_enforced"
	Unknown     EnforcementState = "unknown"
)

type IsolationProfile struct {
	Properties map[IsolationProperty]EnforcementState
}

func (p IsolationProfile) Satisfies(required []IsolationProperty) error {
	for _, property := range required {
		state, ok := p.Properties[property]
		if !ok || state == Unknown {
			return fmt.Errorf("required isolation property %q is unavailable/unknown", property)
		}
		if state != Enforced {
			return fmt.Errorf("required isolation property %q is not enforced", property)
		}
	}
	return nil
}
