package clientadapt

import (
	"errors"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Surface describes adapter affordances. It can change presentation, not
// graph identity, option meaning, or authority requirements.
type Surface struct {
	ID          string
	Affordances map[string]bool
}

type Plan struct {
	ClientID        string
	PackageID       string
	GraphID         string
	GraphVersion    string
	EntryPointID    string
	OmittedOptional []string
}

func BuildPlan(contract contracts.InvocationContract, surface Surface) (Plan, error) {
	if err := contract.Validate(); err != nil {
		return Plan{}, err
	}
	if surface.ID == "" {
		return Plan{}, errors.New("client surface identity is required")
	}
	plan := Plan{ClientID: surface.ID, PackageID: contract.PackageID, GraphID: contract.GraphID, GraphVersion: contract.GraphVersion, EntryPointID: contract.EntryPointID}
	for _, capability := range contract.OptionalCapabilities {
		if !surface.Affordances[capability] {
			plan.OmittedOptional = append(plan.OmittedOptional, capability)
		}
	}
	return plan, nil
}
