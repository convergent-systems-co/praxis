package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

// CanonicalActionIntent is a stable representation used for approval/signature binding.
type CanonicalActionIntent struct {
	Version       string            `json:"version"`
	ID            string            `json:"id"`
	ActorID       string            `json:"actor_id"`
	ActorKind     string            `json:"actor_kind"`
	Operation     string            `json:"operation"`
	Target        string            `json:"target"`
	Parameters    [][2]string       `json:"parameters,omitempty"`
	Scope         string            `json:"scope"`
	Preconditions [][2]string       `json:"preconditions,omitempty"`
	CryptoProfile CryptoProfile     `json:"crypto_profile,omitempty"`
}

func sortedPairs(m map[string]string) [][2]string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([][2]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, [2]string{k, m[k]})
	}
	return pairs
}

func (a ActionIntent) CanonicalBytes() ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	c := CanonicalActionIntent{
		Version: a.Version, ID: a.ID, ActorID: a.Actor.ID, ActorKind: a.Actor.Kind,
		Operation: a.Operation, Target: a.Target, Parameters: sortedPairs(a.Parameters),
		Scope: a.Scope, Preconditions: sortedPairs(a.Preconditions), CryptoProfile: a.CryptoProfile,
	}
	return json.Marshal(c)
}

func (a ActionIntent) Digest() (string, error) {
	b, err := a.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (a ApprovalBinding) AuthorizesIntent(intent ActionIntent, now time.Time) error {
	if err := a.Validate(now); err != nil {
		return err
	}
	if a.IntentDigest == "" {
		return errors.New("policy-bound approval requires policy evaluator")
	}
	d, err := intent.Digest()
	if err != nil {
		return err
	}
	if d != a.IntentDigest {
		return errors.New("approval does not bind this action intent")
	}
	return nil
}
