package crypto

import (
	"context"
	"crypto/ed25519"
	"errors"
)

var ErrPublisherSigningUnavailable = errors.New("publisher signing backend is unavailable")

// PublisherSigner is deliberately narrower than KeyWrapper. It exposes public
// identity and signing only; private material remains inside the backend.
type PublisherSigner interface {
	KeyID() string
	Algorithm() string
	PublicKey(context.Context) ([]byte, error)
	Sign(context.Context, []byte) ([]byte, error)
}

// PublisherSigningBackend is the purpose-separated protected-key boundary.
type PublisherSigningBackend interface {
	Generate(context.Context, string) (PublisherSigner, error)
	Open(context.Context, string) (PublisherSigner, error)
	Delete(context.Context, string) error
}

// MemoryPublisherSigner is test-only signing material. It is intentionally
// not registered as a production backend.
type MemoryPublisherSigner struct {
	ID      string
	Private ed25519.PrivateKey
}

func (s MemoryPublisherSigner) KeyID() string   { return s.ID }
func (MemoryPublisherSigner) Algorithm() string { return "ed25519" }
func (s MemoryPublisherSigner) PublicKey(context.Context) ([]byte, error) {
	if len(s.Private) != ed25519.PrivateKeySize {
		return nil, ErrPublisherSigningUnavailable
	}
	return append([]byte(nil), s.Private.Public().(ed25519.PublicKey)...), nil
}
func (s MemoryPublisherSigner) Sign(_ context.Context, message []byte) ([]byte, error) {
	if len(s.Private) != ed25519.PrivateKeySize {
		return nil, ErrPublisherSigningUnavailable
	}
	return ed25519.Sign(s.Private, message), nil
}
