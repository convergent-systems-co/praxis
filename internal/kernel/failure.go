package kernel

import (
	"errors"
	"fmt"
)

type FailureClass string

const (
	FailureValidation           FailureClass = "validation"
	FailureCapabilityUnavailable FailureClass = "capability_unavailable"
	FailureCapabilityDenied     FailureClass = "capability_denied"
	FailureResourceQuota        FailureClass = "resource_quota"
	FailureTimeout              FailureClass = "timeout"
	FailureCancellation         FailureClass = "cancellation"
	FailureExecutor             FailureClass = "executor"
	FailureInferenceInvalid     FailureClass = "inference_invalid"
	FailureExternalEffectUnknown FailureClass = "external_effect_unknown"
	FailurePolicyInvariant      FailureClass = "policy_invariant"
	FailureMigrationVersion     FailureClass = "migration_version"
	FailureInternalRuntime      FailureClass = "internal_runtime"
)

type ExecutionError struct {
	Class FailureClass
	Err   error
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.Class)
	}
	return fmt.Sprintf("%s: %v", e.Class, e.Err)
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func FailureOf(err error) FailureClass {
	if err == nil {
		return ""
	}
	var execution *ExecutionError
	if errors.As(err, &execution) && execution.Class != "" {
		return execution.Class
	}
	return FailureInternalRuntime
}

func validFailureClass(class FailureClass) bool {
	switch class {
	case FailureValidation, FailureCapabilityUnavailable, FailureCapabilityDenied, FailureResourceQuota,
		FailureTimeout, FailureCancellation, FailureExecutor, FailureInferenceInvalid,
		FailureExternalEffectUnknown, FailurePolicyInvariant, FailureMigrationVersion, FailureInternalRuntime:
		return true
	default:
		return false
	}
}
