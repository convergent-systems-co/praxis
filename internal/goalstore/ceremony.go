package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const ownerCeremonyNamespace = "owner_ceremony"

// SaveOwnerCeremony persists the durable record of one completed interactive
// owner ceremony and returns the digest a protected decision must carry. It is
// the only source of ceremony evidence: SaveAuthorityDecision resolves this
// record rather than trusting a caller-chosen digest string.
//
// Trust model, stated precisely: the record is written only by the CLI
// ceremony (cmd/praxis authority decide) after its TTY confirmation, and this
// method independently refuses a record whose owner, root lineage, or
// authenticated OS user is not the installation's current enrolled root. It
// does not, and in-process cannot, prove that a terminal prompt was answered:
// a caller that already holds this installation's storage key can write any
// record. The guarantee is that copied public identifiers and arbitrary
// digests are no longer accepted as ceremony proof, and that every consumer
// re-resolves and re-matches the record.
func (r Repository) SaveOwnerCeremony(ctx context.Context, evidence contracts.OwnerCeremonyEvidence, now time.Time) (string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", err
	}
	digest, err := evidence.Digest()
	if err != nil {
		return "", err
	}
	owner, err := contracts.InstallationOwnerPrincipal(r.InstallationDigest)
	if err != nil {
		return "", fmt.Errorf("ceremony requires the protected installation identity: %w", err)
	}
	root, err := r.LoadCurrentInstallationRoot(ctx, r.InstallationDigest, now)
	if err != nil {
		return "", fmt.Errorf("load current installation root: %w", err)
	}
	if evidence.Owner != owner || evidence.RootRef != root.Ref || evidence.RootVersion != root.Version || evidence.RootDigest != root.Digest {
		return "", errors.New("ceremony evidence is not for this installation's owner under the current root")
	}
	if !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+evidence.AuthenticatedOSUser) {
		return "", errors.New("ceremony evidence names an OS user that does not own the enrolled installation root")
	}
	if _, loadErr := r.loadOwnerCeremony(ctx, digest, now); loadErr == nil {
		return digest, nil
	} else if !errors.Is(loadErr, state.ErrSecureBlobNotFound) {
		return "", loadErr
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		return "", fmt.Errorf("encode ceremony evidence: %w", err)
	}
	if err := r.putWorkPlanBlob(ctx, ownerCeremonyNamespace, digest, "1", payload, now, nil); err != nil {
		return "", fmt.Errorf("persist ceremony evidence: %w", err)
	}
	return digest, nil
}

func (r Repository) loadOwnerCeremony(ctx context.Context, digest string, now time.Time) (contracts.OwnerCeremonyEvidence, error) {
	payload, _, err := r.loadWorkPlanBlob(ctx, ownerCeremonyNamespace, digest, "1", now)
	if err != nil {
		return contracts.OwnerCeremonyEvidence{}, err
	}
	var evidence contracts.OwnerCeremonyEvidence
	if err := contracts.UnmarshalExactJSON(payload, &evidence, true); err != nil {
		return contracts.OwnerCeremonyEvidence{}, fmt.Errorf("decode ceremony evidence: %w", err)
	}
	if got, err := evidence.Digest(); err != nil || got != digest {
		return contracts.OwnerCeremonyEvidence{}, errors.New("ceremony evidence identity does not match its digest")
	}
	return evidence, nil
}

// verifyDecisionCeremony proves that a protected decision is backed by a
// durable ceremony record for exactly that decision. Requests without a
// ceremony profile are not protected and are unaffected.
func (r Repository) verifyDecisionCeremony(ctx context.Context, request contracts.AuthorityRequest, decision contracts.AuthorityDecision, now time.Time) error {
	if request.CeremonyProfile == "" {
		return nil
	}
	evidence, err := r.loadOwnerCeremony(ctx, decision.CeremonyEvidenceDigest, now)
	if err != nil {
		return fmt.Errorf("protected decision ceremony evidence does not resolve to a durable ceremony record: %w", err)
	}
	if evidence.Profile != request.CeremonyProfile || !evidence.BindsDecision(decision) {
		return errors.New("protected decision does not match its ceremony record")
	}
	return nil
}
