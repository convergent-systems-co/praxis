package contracts

import (
	"errors"
	"fmt"
)

// InvocationRuntimeBinding is the durable bridge between one verified package
// generation, its exact invocation contract, and the runtime adapter that may
// execute it. The binding grants no authority.
type InvocationRuntimeBinding struct {
	PackageID      string `json:"package_id"`
	PackageVersion string `json:"package_version"`
	PackageDigest  string `json:"package_digest"`
	EntryPointID   string `json:"entry_point_id"`
	ContractDigest string `json:"contract_digest"`
	RuntimeID      string `json:"runtime_id"`
	RuntimeVersion string `json:"runtime_version"`
	RuntimeDigest  string `json:"runtime_digest"`
}

func (b InvocationRuntimeBinding) Validate() error {
	if b.PackageID == "" || b.PackageVersion == "" || b.EntryPointID == "" || b.RuntimeID == "" || b.RuntimeVersion == "" {
		return errors.New("invocation runtime binding identity is required")
	}
	for name, value := range map[string]string{
		"package digest": b.PackageDigest, "contract digest": b.ContractDigest, "runtime digest": b.RuntimeDigest,
	} {
		if err := ValidateSHA256Digest(value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}
