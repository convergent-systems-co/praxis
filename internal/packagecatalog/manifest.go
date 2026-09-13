package packagecatalog

import (
	"errors"
	"fmt"
	"sort"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type Dependency struct {
	PackageID string `json:"package_id"`
	Version   string `json:"version"`
	Digest    string `json:"digest"`
}

type Manifest struct {
	PackageID           string                         `json:"package_id"`
	Version             string                         `json:"version"`
	ContentDigest       string                         `json:"content_digest"`
	Publisher           string                         `json:"publisher,omitempty"`
	Dependencies        []Dependency                   `json:"dependencies,omitempty"`
	Capabilities        []string                       `json:"capabilities,omitempty"`
	RequiredEnforcement []string                       `json:"required_enforcement,omitempty"`
	CryptoProfile       contracts.CryptoProfile        `json:"crypto_profile,omitempty"`
	Invocations         []contracts.InvocationContract `json:"invocations,omitempty"`
	UpstreamPackageID   string                         `json:"upstream_package_id,omitempty"`
	UpstreamDigest      string                         `json:"upstream_digest,omitempty"`
}

func (m Manifest) Validate() error {
	if m.PackageID == "" || m.Version == "" || m.ContentDigest == "" {
		return errors.New("package id, version, and content digest are required")
	}
	if m.CryptoProfile != "" {
		if err := m.CryptoProfile.Validate(); err != nil {
			return err
		}
	}
	seenDeps := map[string]struct{}{}
	for _, d := range m.Dependencies {
		if d.PackageID == "" || d.Version == "" || d.Digest == "" {
			return errors.New("dependency id, version, and digest are required")
		}
		if _, ok := seenDeps[d.PackageID]; ok {
			return fmt.Errorf("duplicate dependency %q", d.PackageID)
		}
		seenDeps[d.PackageID] = struct{}{}
	}
	seenEntries := map[string]struct{}{}
	seenAliases := map[string]struct{}{}
	for _, inv := range m.Invocations {
		if err := inv.Validate(); err != nil {
			return fmt.Errorf("invocation contract: %w", err)
		}
		if inv.PackageID != m.PackageID || inv.PackageVersion != m.Version {
			return fmt.Errorf("invocation %q package identity does not match manifest", inv.EntryPointID)
		}
		if _, ok := seenEntries[inv.EntryPointID]; ok {
			return fmt.Errorf("duplicate invocation entry point %q", inv.EntryPointID)
		}
		seenEntries[inv.EntryPointID] = struct{}{}
		for _, alias := range inv.Aliases {
			if _, ok := seenAliases[alias]; ok {
				return fmt.Errorf("duplicate invocation alias %q in package", alias)
			}
			seenAliases[alias] = struct{}{}
		}
	}
	return nil
}

// EffectiveCapabilities computes the deterministic transitive capability request.
// It does not grant any of these capabilities.
func EffectiveCapabilities(root Manifest, dependencies map[string]Manifest) ([]string, error) {
	if err := root.Validate(); err != nil {
		return nil, err
	}
	capSet := map[string]struct{}{}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var walk func(Manifest) error
	walk = func(m Manifest) error {
		if visiting[m.PackageID] {
			return fmt.Errorf("dependency cycle at %q", m.PackageID)
		}
		if visited[m.PackageID] {
			return nil
		}
		visiting[m.PackageID] = true
		for _, capability := range m.Capabilities {
			if capability == "" {
				return errors.New("empty capability request")
			}
			capSet[capability] = struct{}{}
		}
		for _, dep := range m.Dependencies {
			resolved, ok := dependencies[dep.PackageID]
			if !ok {
				return fmt.Errorf("dependency %q not resolved", dep.PackageID)
			}
			if resolved.Version != dep.Version || resolved.ContentDigest != dep.Digest {
				return fmt.Errorf("dependency lock mismatch for %q", dep.PackageID)
			}
			if err := walk(resolved); err != nil {
				return err
			}
		}
		visiting[m.PackageID] = false
		visited[m.PackageID] = true
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	caps := make([]string, 0, len(capSet))
	for capability := range capSet {
		caps = append(caps, capability)
	}
	sort.Strings(caps)
	return caps, nil
}
