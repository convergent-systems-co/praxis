package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

type VersionDisposition string

const (
	VersionCurrent               VersionDisposition = "current"
	VersionSupportedHistorical   VersionDisposition = "supported_historical"
	VersionMigratable            VersionDisposition = "migratable"
	VersionUnsupportedPreRelease VersionDisposition = "unsupported_pre_release"
	VersionRevokedUnsafe         VersionDisposition = "revoked_unsafe"
)

var (
	ErrUnsupportedPreReleaseContractVersion = errors.New("unsupported pre-release contract version")
	ErrRevokedContractVersion               = errors.New("revoked contract version")
	ErrUnknownContractVersion               = errors.New("unknown contract version")
	ErrContractMigrationUnavailable         = errors.New("contract migration is unavailable")
	ErrContractPolicyAlreadyRegistered      = errors.New("contract version policy is already registered")
)

type ContractVersionDefinition struct {
	Version         string             `json:"version"`
	Disposition     VersionDisposition `json:"disposition"`
	MigrationID     string             `json:"migration_id,omitempty"`
	MigrationTarget string             `json:"migration_target,omitempty"`
	Rationale       string             `json:"rationale,omitempty"`
}

type ContractVersionPolicy struct {
	Contract       string                      `json:"contract"`
	CurrentVersion string                      `json:"current_version"`
	Versions       []ContractVersionDefinition `json:"versions"`
}

type VersionUpcaster func(payload []byte) ([]byte, error)

type MigrationRecord struct {
	Contract     string `json:"contract"`
	FromVersion  string `json:"from_version"`
	ToVersion    string `json:"to_version"`
	UpcasterID   string `json:"upcaster_id"`
	PolicyDigest string `json:"policy_digest"`
	InputDigest  string `json:"input_digest"`
	OutputDigest string `json:"output_digest"`
}

type VersionCompatibilityError struct {
	Contract    string
	Version     string
	Disposition VersionDisposition
	Rationale   string
}

func (e *VersionCompatibilityError) Error() string {
	switch e.Disposition {
	case VersionUnsupportedPreRelease:
		return fmt.Sprintf("contract %s version %s is an unsupported pre-release contract: %s", e.Contract, e.Version, e.Rationale)
	case VersionRevokedUnsafe:
		return fmt.Sprintf("contract %s version %s is revoked and unsafe: %s", e.Contract, e.Version, e.Rationale)
	default:
		return fmt.Sprintf("contract %s has unknown version %s", e.Contract, e.Version)
	}
}

func (e *VersionCompatibilityError) Unwrap() error {
	switch e.Disposition {
	case VersionUnsupportedPreRelease:
		return ErrUnsupportedPreReleaseContractVersion
	case VersionRevokedUnsafe:
		return ErrRevokedContractVersion
	default:
		return ErrUnknownContractVersion
	}
}

type VersionRegistry struct {
	policy       ContractVersionPolicy
	policyDigest string
	versions     map[string]ContractVersionDefinition
	upcasters    map[string]VersionUpcaster
}

// VersionCatalog owns one authoritative VersionRegistry per contract identity
// within a contract family/runtime context. Duplicate declarations fail closed,
// including identical duplicates, so initialization never hides distributed
// ownership behind apparently idempotent registration.
type VersionCatalog struct {
	mu         sync.RWMutex
	registries map[string]*VersionRegistry
}

func NewVersionCatalog() *VersionCatalog {
	return &VersionCatalog{registries: map[string]*VersionRegistry{}}
}

func (c *VersionCatalog) Register(policy ContractVersionPolicy, upcasters map[string]VersionUpcaster) (*VersionRegistry, error) {
	if c == nil {
		return nil, errors.New("contract version catalog is required")
	}
	registry, err := NewVersionRegistry(policy, upcasters)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.registries[policy.Contract]; existing != nil {
		return nil, fmt.Errorf("%w: %s already owns policy %s", ErrContractPolicyAlreadyRegistered, policy.Contract, existing.PolicyDigest())
	}
	c.registries[policy.Contract] = registry
	return registry, nil
}

func (c *VersionCatalog) MustRegister(policy ContractVersionPolicy, upcasters map[string]VersionUpcaster) *VersionRegistry {
	registry, err := c.Register(policy, upcasters)
	if err != nil {
		panic(err)
	}
	return registry
}

func (c *VersionCatalog) Lookup(contract string) (*VersionRegistry, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	registry, ok := c.registries[contract]
	return registry, ok
}

func NewVersionRegistry(policy ContractVersionPolicy, upcasters map[string]VersionUpcaster) (*VersionRegistry, error) {
	if policy.Contract == "" || policy.CurrentVersion == "" || len(policy.Versions) == 0 {
		return nil, errors.New("contract name, current version, and version definitions are required")
	}
	policy.Versions = append([]ContractVersionDefinition(nil), policy.Versions...)
	sort.Slice(policy.Versions, func(i, j int) bool { return policy.Versions[i].Version < policy.Versions[j].Version })
	registry := &VersionRegistry{policy: policy, versions: map[string]ContractVersionDefinition{}, upcasters: map[string]VersionUpcaster{}}
	for id, upcaster := range upcasters {
		if id == "" || upcaster == nil {
			return nil, errors.New("versioned upcaster identity and implementation are required")
		}
		registry.upcasters[id] = upcaster
	}
	currentCount := 0
	for _, definition := range policy.Versions {
		if definition.Version == "" || registry.versions[definition.Version].Version != "" {
			return nil, errors.New("contract versions must be non-empty and unique")
		}
		switch definition.Disposition {
		case VersionCurrent:
			currentCount++
			if definition.Version != policy.CurrentVersion || definition.MigrationID != "" || definition.MigrationTarget != "" {
				return nil, errors.New("current contract version metadata is inconsistent")
			}
		case VersionSupportedHistorical:
			if definition.Version == policy.CurrentVersion || definition.MigrationID != "" || definition.MigrationTarget != "" {
				return nil, errors.New("supported historical version metadata is inconsistent")
			}
		case VersionMigratable:
			if definition.MigrationID == "" || definition.MigrationTarget != policy.CurrentVersion || registry.upcasters[definition.MigrationID] == nil {
				return nil, errors.New("migratable version requires a registered upcaster targeting the current contract")
			}
		case VersionUnsupportedPreRelease, VersionRevokedUnsafe:
			if definition.Rationale == "" || definition.MigrationID != "" || definition.MigrationTarget != "" {
				return nil, errors.New("rejected contract version requires rationale and cannot declare migration")
			}
		default:
			return nil, errors.New("unknown contract version disposition")
		}
		registry.versions[definition.Version] = definition
	}
	if currentCount != 1 {
		return nil, errors.New("contract requires exactly one current version definition")
	}
	encodedPolicy, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	registry.policyDigest = versionPayloadDigest(encodedPolicy)
	return registry, nil
}

func MustVersionRegistry(policy ContractVersionPolicy, upcasters map[string]VersionUpcaster) *VersionRegistry {
	registry, err := NewVersionRegistry(policy, upcasters)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r *VersionRegistry) Policy() ContractVersionPolicy {
	policy := r.policy
	policy.Versions = append([]ContractVersionDefinition(nil), r.policy.Versions...)
	return policy
}

func (r *VersionRegistry) CurrentVersion() string { return r.policy.CurrentVersion }

func (r *VersionRegistry) PolicyDigest() string { return r.policyDigest }

func (r *VersionRegistry) Definition(version string) (ContractVersionDefinition, bool) {
	definition, ok := r.versions[version]
	return definition, ok
}

func (r *VersionRegistry) Canonicalize(version string, payload []byte) ([]byte, *MigrationRecord, error) {
	definition, ok := r.versions[version]
	if !ok {
		return nil, nil, &VersionCompatibilityError{Contract: r.policy.Contract, Version: version}
	}
	switch definition.Disposition {
	case VersionCurrent, VersionSupportedHistorical:
		return append([]byte(nil), payload...), nil, nil
	case VersionUnsupportedPreRelease, VersionRevokedUnsafe:
		return nil, nil, &VersionCompatibilityError{Contract: r.policy.Contract, Version: version, Disposition: definition.Disposition, Rationale: definition.Rationale}
	case VersionMigratable:
		upcaster := r.upcasters[definition.MigrationID]
		if upcaster == nil {
			return nil, nil, fmt.Errorf("%w: %s", ErrContractMigrationUnavailable, definition.MigrationID)
		}
		canonical, err := upcaster(append([]byte(nil), payload...))
		if err != nil {
			return nil, nil, fmt.Errorf("upcast contract %s from %s with %s: %w", r.policy.Contract, version, definition.MigrationID, err)
		}
		record := &MigrationRecord{Contract: r.policy.Contract, FromVersion: version, ToVersion: definition.MigrationTarget, UpcasterID: definition.MigrationID, PolicyDigest: r.policyDigest, InputDigest: versionPayloadDigest(payload), OutputDigest: versionPayloadDigest(canonical)}
		return canonical, record, nil
	default:
		return nil, nil, &VersionCompatibilityError{Contract: r.policy.Contract, Version: version}
	}
}

func versionPayloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
