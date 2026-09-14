package goaldrive

import (
	"errors"
	"fmt"
)

var ErrUnknownProvider = errors.New("unknown Goal-drive provider")

// Registry is setup-time provider authority. Workers cannot register or
// replace themselves during execution.
type Registry struct {
	providers map[string]Worker
}

func NewRegistry() *Registry { return &Registry{providers: map[string]Worker{}} }

func (r *Registry) Register(providerID string, worker Worker) error {
	if r == nil {
		return errors.New("Goal-drive provider registry is required")
	}
	if providerID == "" || worker == nil {
		return errors.New("provider identity and worker are required")
	}
	if _, exists := r.providers[providerID]; exists {
		return fmt.Errorf("provider %q is already registered", providerID)
	}
	r.providers[providerID] = worker
	return nil
}

func (r *Registry) Resolve(providerID string) (Worker, error) {
	if r == nil || providerID == "" {
		return nil, ErrUnknownProvider
	}
	worker, ok := r.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, providerID)
	}
	return worker, nil
}
