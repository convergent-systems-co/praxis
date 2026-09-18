package crypto

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type KeyState string

const (
	KeyPending           KeyState = "pending"
	KeyActive            KeyState = "active"
	KeyRetiring          KeyState = "retiring"
	KeyRetiredVerifyOnly KeyState = "retired_verify_only"
	KeyRevoked           KeyState = "revoked"
	KeyDestroyed         KeyState = "destroyed"
)

// KeyReference is intentionally metadata-only. Secret key bytes never cross
// this boundary or enter generic durable state.
type KeyReference struct {
	ID                string
	Owner             string
	Purpose           string
	AlgorithmFamily   string
	Version           string
	ProviderRef       string
	State             KeyState
	CreatedAt         time.Time
	NotBefore         time.Time
	ExpiresAt         *time.Time
	RotationAncestor  string
	AllowedProfiles   []contracts.CryptoProfile
	AllowedOperations []string
	Exportable        bool
	HardwareBacked    bool
}

func (k KeyReference) Validate() error {
	if k.ID == "" || k.Owner == "" || k.Purpose == "" || k.AlgorithmFamily == "" || k.Version == "" || k.ProviderRef == "" || k.State == "" {
		return errors.New("key reference identity, purpose, provider, version, and state are required")
	}
	if k.CreatedAt.IsZero() || k.NotBefore.IsZero() {
		return errors.New("key reference creation and not-before times are required")
	}
	if len(k.AllowedProfiles) == 0 || len(k.AllowedOperations) == 0 {
		return errors.New("key reference crypto profiles and operations are required")
	}
	for _, profile := range k.AllowedProfiles {
		if err := profile.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type KeyRegistry struct {
	mu   sync.RWMutex
	keys map[string]KeyReference
}

func NewKeyRegistry() *KeyRegistry { return &KeyRegistry{keys: map[string]KeyReference{}} }

func (r *KeyRegistry) Register(key KeyReference) error {
	if r == nil {
		return errors.New("key registry is required")
	}
	if err := key.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	identity := key.ID + "@" + key.Version
	if _, exists := r.keys[identity]; exists {
		return fmt.Errorf("key version already exists: %s", identity)
	}
	r.keys[identity] = cloneKeyReference(key)
	return nil
}

func (r *KeyRegistry) Activate(id, version string, now time.Time) error {
	return r.transition(id, version, KeyPending, KeyActive, now)
}

func (r *KeyRegistry) Retire(id, version string, now time.Time) error {
	return r.transition(id, version, KeyActive, KeyRetiredVerifyOnly, now)
}

func (r *KeyRegistry) Revoke(id, version string) error {
	if r == nil {
		return errors.New("key registry is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key, ok := r.keys[id+"@"+version]
	if !ok {
		return errors.New("unknown key reference")
	}
	if key.State == KeyDestroyed {
		return errors.New("destroyed key cannot be revoked")
	}
	key.State = KeyRevoked
	r.keys[id+"@"+version] = key
	return nil
}

func (r *KeyRegistry) Rotate(id, oldVersion string, next KeyReference, now time.Time) error {
	if r == nil {
		return errors.New("key registry is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.keys[id+"@"+oldVersion]
	if !ok || old.State != KeyActive {
		return errors.New("only an active key can rotate")
	}
	if next.ID != id || next.RotationAncestor != oldVersion || next.State != KeyPending {
		return errors.New("rotation must preserve logical identity and ancestry")
	}
	if err := next.Validate(); err != nil {
		return err
	}
	old.State = KeyRetiredVerifyOnly
	r.keys[id+"@"+oldVersion] = old
	next.CreatedAt = now
	next.NotBefore = now
	r.keys[id+"@"+next.Version] = cloneKeyReference(next)
	return nil
}

func (r *KeyRegistry) CanUse(id, version, operation string, now time.Time) error {
	if r == nil {
		return errors.New("key registry is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, ok := r.keys[id+"@"+version]
	if !ok {
		return errors.New("unknown key reference")
	}
	if !now.IsZero() && now.Before(key.NotBefore) {
		return errors.New("key is not yet valid")
	}
	if key.ExpiresAt != nil && now.After(*key.ExpiresAt) {
		return errors.New("key is expired")
	}
	if key.State == KeyRevoked || key.State == KeyDestroyed || key.State == KeyPending {
		return errors.New("key is not usable")
	}
	if key.State == KeyRetiredVerifyOnly && operation != "verify" {
		return errors.New("retired key is verification-only")
	}
	for _, allowed := range key.AllowedOperations {
		if allowed == operation {
			return nil
		}
	}
	return errors.New("operation is not allowed for key")
}

func (r *KeyRegistry) Get(id, version string) (KeyReference, error) {
	if r == nil {
		return KeyReference{}, errors.New("key registry is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, ok := r.keys[id+"@"+version]
	if !ok {
		return KeyReference{}, errors.New("unknown key reference")
	}
	return cloneKeyReference(key), nil
}

func (r *KeyRegistry) transition(id, version string, from, to KeyState, now time.Time) error {
	if r == nil {
		return errors.New("key registry is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key, ok := r.keys[id+"@"+version]
	if !ok || key.State != from {
		return fmt.Errorf("key %s@%s is not %s", id, version, from)
	}
	if !now.IsZero() && now.Before(key.NotBefore) {
		return errors.New("key is not yet valid")
	}
	key.State = to
	r.keys[id+"@"+version] = key
	return nil
}

func cloneKeyReference(key KeyReference) KeyReference {
	key.AllowedProfiles = append([]contracts.CryptoProfile(nil), key.AllowedProfiles...)
	key.AllowedOperations = append([]string(nil), key.AllowedOperations...)
	return key
}
