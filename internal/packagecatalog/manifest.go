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

type ContentKind string

const (
	ContentGraph             ContentKind = "graph"
	ContentAgentDefinition   ContentKind = "agent_definition"
	ContentPlugin            ContentKind = "plugin"
	ContentPreference        ContentKind = "preference_contract"
	ContentBehavioralProfile ContentKind = "behavioral_profile"
	ContentTemplate          ContentKind = "template"
	ContentMigration         ContentKind = "migration"
	ContentDocumentation     ContentKind = "documentation"
)

type ContentRef struct {
	Kind          ContentKind `json:"kind"`
	ID            string      `json:"id"`
	Version       string      `json:"version"`
	Digest        string      `json:"digest"`
	Artifact      string      `json:"artifact"`
	Compatibility string      `json:"compatibility,omitempty"`
}

func (c ContentRef) Validate() error {
	if c.Kind == "" || c.ID == "" || c.Version == "" || c.Digest == "" || c.Artifact == "" {
		return errors.New("package content kind, id, version, digest, and artifact are required")
	}
	switch c.Kind {
	case ContentGraph, ContentAgentDefinition, ContentPlugin, ContentPreference, ContentBehavioralProfile, ContentTemplate, ContentMigration, ContentDocumentation:
		return nil
	default:
		return fmt.Errorf("unsupported package content kind %q", c.Kind)
	}
}

type Manifest struct {
	ContractVersion     string                         `json:"contract_version"`
	PackageID           string                         `json:"package_id"`
	Version             string                         `json:"version"`
	ContentDigest       string                         `json:"content_digest"`
	Publisher           string                         `json:"publisher,omitempty"`
	Description         string                         `json:"description,omitempty"`
	Tags                []string                       `json:"tags,omitempty"`
	Dependencies        []Dependency                   `json:"dependencies,omitempty"`
	Capabilities        []string                       `json:"capabilities,omitempty"`
	RequiredEnforcement []string                       `json:"required_enforcement,omitempty"`
	CryptoProfile       contracts.CryptoProfile        `json:"crypto_profile,omitempty"`
	Contents            []ContentRef                   `json:"contents,omitempty"`
	Invocations         []contracts.InvocationContract `json:"invocations,omitempty"`
	UpstreamPackageID   string                         `json:"upstream_package_id,omitempty"`
	UpstreamDigest      string                         `json:"upstream_digest,omitempty"`
}

func (m Manifest) Validate() error {
	if err := requirePackageContractVersion(manifestContractVersions, m.ContractVersion); err != nil {
		return err
	}
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
	seenContent := map[string]struct{}{}
	for _, content := range m.Contents {
		if err := content.Validate(); err != nil {
			return fmt.Errorf("package content: %w", err)
		}
		key := string(content.Kind) + "\x00" + content.ID + "\x00" + content.Version
		if _, ok := seenContent[key]; ok {
			return fmt.Errorf("duplicate package content %s/%s@%s", content.Kind, content.ID, content.Version)
		}
		seenContent[key] = struct{}{}
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

func (m Manifest) ContentsOf(kind ContentKind) []ContentRef {
	out := make([]ContentRef, 0)
	for _, content := range m.Contents {
		if content.Kind == kind {
			out = append(out, content)
		}
	}
	return out
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
