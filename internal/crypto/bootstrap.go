package crypto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const BootstrapRecordVersion = "1"

type SecurityLevel string

const (
	SecurityPlatformProtected      SecurityLevel = "platform-protected"
	SecurityHardwareProtected      SecurityLevel = "hardware-protected"
	SecurityPortableUserControlled SecurityLevel = "portable-user-controlled"
)

// BootstrapRequest requires an explicit backend selection. Registration and
// capability discovery never silently select a weaker provider.
type BootstrapRequest struct {
	ProviderID string
	KeyID      string
	Owner      string
	Purpose    string
	Profile    contracts.CryptoProfile
}

func (r BootstrapRequest) Validate() error {
	if r.ProviderID == "" || r.KeyID == "" || r.Owner == "" || r.Purpose == "" {
		return errors.New("bootstrap provider, key, owner, and purpose are required")
	}
	if err := r.Profile.Validate(); err != nil {
		return fmt.Errorf("bootstrap profile: %w", err)
	}
	return nil
}

// BootstrapRecord contains only non-secret binding metadata. Secret key bytes,
// recovery material, and private identity material stay inside the backend.
type BootstrapRecord struct {
	Version         string
	ProviderID      string
	KeyID           string
	KeyVersion      string
	Owner           string
	Purpose         string
	Profile         contracts.CryptoProfile
	SecurityLevel   SecurityLevel
	Platform        string
	Architecture    string
	CreatedAt       time.Time
	PredecessorHash string
}

func (r BootstrapRecord) Validate() error {
	if r.Version != BootstrapRecordVersion || r.ProviderID == "" || r.KeyID == "" || r.KeyVersion == "" || r.Owner == "" || r.Purpose == "" {
		return errors.New("bootstrap record identity and version are required")
	}
	if err := r.Profile.Validate(); err != nil {
		return fmt.Errorf("bootstrap record profile: %w", err)
	}
	switch r.SecurityLevel {
	case SecurityPlatformProtected, SecurityHardwareProtected, SecurityPortableUserControlled:
	default:
		return errors.New("bootstrap record security level is unknown")
	}
	if r.Platform == "" || r.Architecture == "" || r.CreatedAt.IsZero() {
		return errors.New("bootstrap record platform, architecture, and creation time are required")
	}
	return nil
}

// Digest identifies the non-secret binding and is safe to retain in evidence.
func (r BootstrapRecord) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	b := []byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", r.Version, r.ProviderID, r.KeyID, r.KeyVersion, r.Owner, r.Purpose, r.Profile, r.SecurityLevel, r.Platform, r.Architecture, r.CreatedAt.UTC().Format(time.RFC3339Nano), r.PredecessorHash))
	digest := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// BootstrapBackend owns platform-specific protection. It never returns secret
// key material to callers; KeyWrapper keeps it inside the backend.
type BootstrapBackend interface {
	ProviderID() string
	SecurityLevel(context.Context) (SecurityLevel, error)
	Available(context.Context) error
	Bootstrap(context.Context, BootstrapRequest) (BootstrapRecord, KeyWrapper, error)
	Open(context.Context, BootstrapRecord) (KeyWrapper, error)
}

// BootstrapRegistry is setup-time plumbing, not authority. A backend is used
// only when explicitly selected and available; no fallback occurs here.
type BootstrapRegistry struct{ backends map[string]BootstrapBackend }

func NewBootstrapRegistry() *BootstrapRegistry {
	return &BootstrapRegistry{backends: map[string]BootstrapBackend{}}
}

func (r *BootstrapRegistry) Register(backend BootstrapBackend) error {
	if r == nil || backend == nil {
		return errors.New("bootstrap registry and backend are required")
	}
	id := backend.ProviderID()
	if id == "" {
		return errors.New("bootstrap provider identity is required")
	}
	if _, exists := r.backends[id]; exists {
		return fmt.Errorf("bootstrap provider %q is already registered", id)
	}
	r.backends[id] = backend
	return nil
}

func (r *BootstrapRegistry) Bootstrap(ctx context.Context, request BootstrapRequest) (BootstrapRecord, KeyWrapper, error) {
	if err := request.Validate(); err != nil {
		return BootstrapRecord{}, nil, err
	}
	backend, err := r.resolve(request.ProviderID)
	if err != nil {
		return BootstrapRecord{}, nil, err
	}
	if err := backend.Available(ctx); err != nil {
		return BootstrapRecord{}, nil, fmt.Errorf("bootstrap provider %q unavailable: %w", request.ProviderID, err)
	}
	securityLevel, err := backend.SecurityLevel(ctx)
	if err != nil {
		return BootstrapRecord{}, nil, fmt.Errorf("discover bootstrap provider security level: %w", err)
	}
	record, wrapper, err := backend.Bootstrap(ctx, request)
	if err != nil {
		return BootstrapRecord{}, nil, err
	}
	if err := record.Validate(); err != nil {
		return BootstrapRecord{}, nil, fmt.Errorf("bootstrap provider returned invalid record: %w", err)
	}
	if record.ProviderID != request.ProviderID || record.KeyID != request.KeyID || record.Owner != request.Owner || record.Purpose != request.Purpose || wrapper == nil {
		return BootstrapRecord{}, nil, errors.New("bootstrap provider returned an unbound result")
	}
	if record.SecurityLevel != securityLevel {
		return BootstrapRecord{}, nil, errors.New("bootstrap provider security level changed during bootstrap")
	}
	return record, wrapper, nil
}

func (r *BootstrapRegistry) Open(ctx context.Context, record BootstrapRecord) (KeyWrapper, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	backend, err := r.resolve(record.ProviderID)
	if err != nil {
		return nil, err
	}
	if err := backend.Available(ctx); err != nil {
		return nil, fmt.Errorf("bootstrap provider %q unavailable: %w", record.ProviderID, err)
	}
	securityLevel, err := backend.SecurityLevel(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover bootstrap provider security level: %w", err)
	}
	if securityLevel != record.SecurityLevel {
		return nil, errors.New("bootstrap provider security level no longer matches binding")
	}
	return backend.Open(ctx, record)
}

func (r *BootstrapRegistry) resolve(providerID string) (BootstrapBackend, error) {
	if r == nil || providerID == "" {
		return nil, ErrUnknownKeyProvider
	}
	backend, ok := r.backends[providerID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownKeyProvider, providerID)
	}
	return backend, nil
}

// CurrentPlatformBinding is informational only; it never auto-selects a backend.
func CurrentPlatformBinding() (string, string) { return runtime.GOOS, runtime.GOARCH }
