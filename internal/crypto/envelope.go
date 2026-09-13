package crypto

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const EnvelopeVersion = "1"
const ContentCipherAES256GCM = "AES-256-GCM"

type WrappedKey struct {
	Ciphertext      []byte
	SuiteID         string
	KeyRef          string
	KeyVersion      string
	SelectedProfile contracts.CryptoProfile
}

type KeyWrapper interface {
	Capabilities(ctx context.Context, keyRef string) (Capabilities, error)
	Wrap(ctx context.Context, keyRef string, profile contracts.CryptoProfile, plaintextKey []byte) (WrappedKey, error)
	Unwrap(ctx context.Context, wrapped WrappedKey) ([]byte, error)
}

type Envelope struct {
	Version          string
	ContentCipher    string
	Nonce            []byte
	Ciphertext       []byte
	WrappedDEK       WrappedKey
	RequestedProfile contracts.CryptoProfile
	AAD              []byte
}

type EnvelopePolicy struct {
	AllowPQPreferredFallback bool
}

type EnvelopeService struct {
	Wrapper KeyWrapper
	Random  io.Reader
	Policy  EnvelopePolicy
}

type authenticatedHeader struct {
	EnvelopeVersion  string                  `json:"envelope_version"`
	ContentCipher    string                  `json:"content_cipher"`
	RequestedProfile contracts.CryptoProfile `json:"requested_profile"`
	SelectedProfile  contracts.CryptoProfile `json:"selected_profile"`
	WrapSuite         string                  `json:"wrap_suite"`
	KeyRef            string                  `json:"key_ref"`
	KeyVersion        string                  `json:"key_version"`
	WrappedKeyDigest  string                  `json:"wrapped_key_digest"`
}

func (s EnvelopeService) Seal(ctx context.Context, keyRef string, requested contracts.CryptoProfile, plaintext, aad []byte) (Envelope, error) {
	if s.Wrapper == nil || keyRef == "" {
		return Envelope{}, errors.New("key wrapper and key reference are required")
	}
	caps, err := s.Wrapper.Capabilities(ctx, keyRef)
	if err != nil {
		return Envelope{}, fmt.Errorf("discover key wrapper capabilities: %w", err)
	}
	resolution, err := Resolve(requested, caps, s.Policy.AllowPQPreferredFallback)
	if err != nil {
		return Envelope{}, err
	}

	rng := s.Random
	if rng == nil {
		rng = rand.Reader
	}
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rng, dek); err != nil {
		return Envelope{}, fmt.Errorf("generate DEK: %w", err)
	}
	defer zero(dek)

	wrapped, err := s.Wrapper.Wrap(ctx, keyRef, resolution.Selected, dek)
	if err != nil {
		return Envelope{}, fmt.Errorf("wrap DEK: %w", err)
	}
	if wrapped.KeyRef != keyRef || wrapped.SuiteID == "" || wrapped.KeyVersion == "" {
		return Envelope{}, errors.New("key wrapper returned incomplete metadata")
	}
	if wrapped.SelectedProfile != resolution.Selected {
		return Envelope{}, errors.New("key wrapper profile does not match resolved profile")
	}

	block, err := aes.NewCipher(dek)
	if err != nil {
		return Envelope{}, fmt.Errorf("create content cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, fmt.Errorf("create content AEAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rng, nonce); err != nil {
		return Envelope{}, fmt.Errorf("generate nonce: %w", err)
	}
	headerAAD, err := envelopeAssociatedData(EnvelopeVersion, ContentCipherAES256GCM, requested, wrapped, aad)
	if err != nil {
		return Envelope{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, headerAAD)

	return Envelope{Version: EnvelopeVersion, ContentCipher: ContentCipherAES256GCM, Nonce: nonce, Ciphertext: ciphertext, WrappedDEK: wrapped, RequestedProfile: requested, AAD: append([]byte(nil), aad...)}, nil
}

func (s EnvelopeService) Open(ctx context.Context, envelope Envelope, expectedAAD []byte) ([]byte, error) {
	if s.Wrapper == nil {
		return nil, errors.New("key wrapper is required")
	}
	if envelope.Version != EnvelopeVersion || envelope.ContentCipher != ContentCipherAES256GCM {
		return nil, errors.New("unsupported encrypted envelope format")
	}
	if envelope.WrappedDEK.SuiteID == "" || envelope.WrappedDEK.KeyRef == "" || envelope.WrappedDEK.KeyVersion == "" {
		return nil, errors.New("encrypted envelope key metadata is incomplete")
	}
	if err := envelope.RequestedProfile.Validate(); err != nil {
		return nil, err
	}
	if err := envelope.WrappedDEK.SelectedProfile.Validate(); err != nil {
		return nil, fmt.Errorf("selected crypto profile: %w", err)
	}
	if envelope.RequestedProfile == contracts.CryptoPQRequired && envelope.WrappedDEK.SelectedProfile != contracts.CryptoPQRequired {
		return nil, errors.New("pq-required envelope was downgraded")
	}
	if envelope.RequestedProfile == contracts.CryptoHybridHighAssurance && envelope.WrappedDEK.SelectedProfile != contracts.CryptoHybridHighAssurance {
		return nil, errors.New("hybrid envelope was downgraded")
	}
	if !bytes.Equal(envelope.AAD, expectedAAD) {
		return nil, errors.New("encrypted envelope associated data mismatch")
	}

	dek, err := s.Wrapper.Unwrap(ctx, envelope.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("unwrap DEK: %w", err)
	}
	defer zero(dek)
	if len(dek) != 32 {
		return nil, errors.New("unwrapped DEK must be 256 bits")
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(envelope.Nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid envelope nonce size")
	}
	headerAAD, err := envelopeAssociatedData(envelope.Version, envelope.ContentCipher, envelope.RequestedProfile, envelope.WrappedDEK, expectedAAD)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, envelope.Nonce, envelope.Ciphertext, headerAAD)
	if err != nil {
		return nil, errors.New("encrypted envelope authentication failed")
	}
	return plaintext, nil
}

func envelopeAssociatedData(version, contentCipher string, requested contracts.CryptoProfile, wrapped WrappedKey, applicationAAD []byte) ([]byte, error) {
	digest := sha256.Sum256(wrapped.Ciphertext)
	header := authenticatedHeader{
		EnvelopeVersion:  version,
		ContentCipher:    contentCipher,
		RequestedProfile: requested,
		SelectedProfile:  wrapped.SelectedProfile,
		WrapSuite:         wrapped.SuiteID,
		KeyRef:            wrapped.KeyRef,
		KeyVersion:        wrapped.KeyVersion,
		WrappedKeyDigest:  "sha256:" + hex.EncodeToString(digest[:]),
	}
	encoded, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("encode authenticated envelope header: %w", err)
	}
	out := make([]byte, 0, len(encoded)+1+len(applicationAAD))
	out = append(out, encoded...)
	out = append(out, 0)
	out = append(out, applicationAAD...)
	return out, nil
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
