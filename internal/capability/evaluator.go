package capability

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrPrincipalMismatch = errors.New("lease principal mismatch")
	ErrCapabilityDenied  = errors.New("capability not granted")
	ErrOperationDenied   = errors.New("operation not granted")
	ErrScopeDenied       = errors.New("scope not granted")
)

// Request is the minimum deterministic information required to evaluate one
// capability use before execution.
type Request struct {
	Principal  contracts.PrincipalRef
	Capability string
	Operation  string
	Scope      string
	Now        time.Time
}

// Evaluate verifies that a lease authorizes the exact principal/capability/
// operation/scope request. Scope inheritance here is deliberately conservative:
// exact match or an explicit hierarchical prefix ending in ':' only.
func Evaluate(lease contracts.CapabilityLease, req Request) error {
	if err := lease.Validate(req.Now); err != nil {
		return err
	}
	if lease.Principal != req.Principal {
		return ErrPrincipalMismatch
	}
	if lease.Capability != req.Capability {
		return ErrCapabilityDenied
	}
	if !slices.Contains(lease.Operations, req.Operation) {
		return ErrOperationDenied
	}
	if !scopeAllows(lease.Scope, req.Scope) {
		return fmt.Errorf("%w: lease=%q request=%q", ErrScopeDenied, lease.Scope, req.Scope)
	}
	return nil
}

func scopeAllows(granted, requested string) bool {
	if granted == requested {
		return true
	}
	if !strings.HasSuffix(granted, ":*") {
		return false
	}
	prefix := strings.TrimSuffix(granted, "*")
	return strings.HasPrefix(requested, prefix)
}
