package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const snapshotEnvelopeVersion = "lifecycle-snapshot/v1"

// ArtifactResolver verifies and retrieves an external artifact without
// exposing its backing store or granting lifecycle authority.
type ArtifactResolver interface {
	ResolveArtifact(context.Context, contracts.LifecycleArtifactRef) ([]byte, error)
}

type SnapshotPayload struct {
	ArtifactID string `json:"artifact_id"`
	Bytes      []byte `json:"bytes,omitempty"`
}

type SnapshotEnvelope struct {
	Version        string                              `json:"version"`
	EnvelopeDigest string                              `json:"envelope_digest"`
	Manifest       contracts.LifecycleSnapshotManifest `json:"manifest"`
	Payloads       []SnapshotPayload                   `json:"payloads,omitempty"`
}

type SnapshotExportRequest struct {
	Manifest contracts.LifecycleSnapshotManifest
	Embedded []SnapshotPayload
	External ArtifactResolver
}

type VerifiedArtifact struct {
	Reference contracts.LifecycleArtifactRef
	Digest    string
	Size      int64
}

// FencedSnapshot is an inspected snapshot, never an activated installation.
// Restored authority and runtime state remain historical until a separate
// governed reconciliation transition makes them current.
type FencedSnapshot struct {
	EnvelopeDigest string
	Manifest       contracts.LifecycleSnapshotManifest
	Artifacts      []VerifiedArtifact
	AuthorityState string
	Executable     bool
}

func ExportSnapshot(ctx context.Context, req SnapshotExportRequest) ([]byte, string, error) {
	if err := req.Manifest.Validate(); err != nil {
		return nil, "", fmt.Errorf("snapshot manifest: %w", err)
	}
	if err := req.Manifest.VerifyDigest(); err != nil {
		return nil, "", fmt.Errorf("snapshot manifest digest: %w", err)
	}
	if err := verifyArtifacts(ctx, req.Manifest, req.Embedded, req.External); err != nil {
		return nil, "", err
	}
	payloads := append([]SnapshotPayload(nil), req.Embedded...)
	sort.Slice(payloads, func(i, j int) bool { return payloads[i].ArtifactID < payloads[j].ArtifactID })
	return freezeEnvelope(SnapshotEnvelope{Version: snapshotEnvelopeVersion, Manifest: req.Manifest, Payloads: payloads})
}

func ImportSnapshot(ctx context.Context, data []byte, external ArtifactResolver) (FencedSnapshot, error) {
	var envelope SnapshotEnvelope
	if len(data) == 0 || json.Unmarshal(data, &envelope) != nil {
		return FencedSnapshot{}, errors.New("snapshot envelope is not valid JSON")
	}
	if envelope.Version != snapshotEnvelopeVersion {
		return FencedSnapshot{}, fmt.Errorf("unsupported snapshot envelope version %q", envelope.Version)
	}
	_, digest, err := freezeEnvelope(envelope)
	if err != nil {
		return FencedSnapshot{}, err
	}
	if digest != envelope.EnvelopeDigest {
		return FencedSnapshot{}, errors.New("snapshot envelope digest mismatch")
	}
	if err := envelope.Manifest.Validate(); err != nil {
		return FencedSnapshot{}, fmt.Errorf("snapshot manifest: %w", err)
	}
	if err := envelope.Manifest.VerifyDigest(); err != nil {
		return FencedSnapshot{}, fmt.Errorf("snapshot manifest digest: %w", err)
	}
	if err := verifyArtifacts(ctx, envelope.Manifest, envelope.Payloads, external); err != nil {
		return FencedSnapshot{}, err
	}
	artifacts := make([]VerifiedArtifact, 0, len(envelope.Manifest.Entries))
	for _, entry := range envelope.Manifest.Entries {
		artifacts = append(artifacts, VerifiedArtifact{Reference: entry.Artifact, Digest: entry.Artifact.Digest, Size: entry.Artifact.Size})
	}
	return FencedSnapshot{EnvelopeDigest: envelope.EnvelopeDigest, Manifest: envelope.Manifest, Artifacts: artifacts, AuthorityState: "historical_only", Executable: false}, nil
}

func freezeEnvelope(envelope SnapshotEnvelope) ([]byte, string, error) {
	if envelope.Version == "" || envelope.Manifest.SnapshotID == "" {
		return nil, "", errors.New("snapshot envelope identity is required")
	}
	copy := envelope
	copy.EnvelopeDigest = ""
	bytes, err := json.Marshal(copy)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(bytes)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	envelope.EnvelopeDigest = digest
	final, err := json.Marshal(envelope)
	if err != nil {
		return nil, "", err
	}
	return final, digest, nil
}

func verifyArtifacts(ctx context.Context, manifest contracts.LifecycleSnapshotManifest, payloads []SnapshotPayload, external ArtifactResolver) error {
	entries := make(map[string]contracts.LifecycleArtifactRef, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		entries[entry.Artifact.ID] = entry.Artifact
	}
	seen := make(map[string]bool, len(payloads))
	for _, payload := range payloads {
		artifact, ok := entries[payload.ArtifactID]
		if !ok || seen[payload.ArtifactID] {
			return fmt.Errorf("snapshot payload does not bind one manifest artifact: %q", payload.ArtifactID)
		}
		seen[payload.ArtifactID] = true
		if artifact.Reference != "" {
			return fmt.Errorf("external artifact %q must not be embedded", artifact.ID)
		}
		if int64(len(payload.Bytes)) != artifact.Size || digestBytes(payload.Bytes) != artifact.Digest {
			return fmt.Errorf("embedded artifact %q digest or size mismatch", artifact.ID)
		}
	}
	for _, entry := range manifest.Entries {
		artifact := entry.Artifact
		if artifact.Reference == "" {
			if !seen[artifact.ID] {
				return fmt.Errorf("snapshot is incomplete: embedded artifact %q is missing", artifact.ID)
			}
			continue
		}
		if external == nil {
			return fmt.Errorf("external artifact %q requires a verifier", artifact.ID)
		}
		bytes, err := external.ResolveArtifact(ctx, artifact)
		if err != nil {
			return fmt.Errorf("resolve external artifact %q: %w", artifact.ID, err)
		}
		if int64(len(bytes)) != artifact.Size || digestBytes(bytes) != artifact.Digest {
			return fmt.Errorf("external artifact %q digest or size mismatch", artifact.ID)
		}
	}
	return nil
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
