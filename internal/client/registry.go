package client

import (
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type Registry struct {
	byAlias map[string]contracts.InvocationContract
}

func NewRegistry(all []contracts.InvocationContract) (*Registry, error) {
	r := &Registry{byAlias: map[string]contracts.InvocationContract{}}
	for _, contract := range all {
		if err := contract.Validate(); err != nil {
			return nil, err
		}
		for _, alias := range contract.Aliases {
			if _, exists := r.byAlias[alias]; exists {
				return nil, fmt.Errorf("duplicate invocation alias %q", alias)
			}
			r.byAlias[alias] = contract
		}
	}
	return r, nil
}

func (r *Registry) Resolve(inv Invocation) (contracts.InvocationContract, map[string]string, error) {
	if r == nil {
		return contracts.InvocationContract{}, nil, errors.New("nil invocation registry")
	}
	contract, ok := r.byAlias[inv.EntryPoint]
	if !ok {
		return contracts.InvocationContract{}, nil, fmt.Errorf("unknown Praxis entry point %q", inv.EntryPoint)
	}
	values := map[string]string{}
	known := map[string]contracts.InvocationOption{}
	for _, option := range contract.Options {
		known[option.Name] = option
		if option.Default != "" {
			values[option.Name] = option.Default
		}
	}
	for name, value := range inv.Options {
		if _, ok := known[name]; !ok {
			return contracts.InvocationContract{}, nil, fmt.Errorf("unknown option %q for %s", name, inv.EntryPoint)
		}
		values[name] = value
	}
	for _, option := range contract.Options {
		if option.Required && values[option.Name] == "" {
			return contracts.InvocationContract{}, nil, fmt.Errorf("required option %q missing", option.Name)
		}
	}
	return contract, values, nil
}
