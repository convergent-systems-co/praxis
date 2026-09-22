package goalstore

import (
	"context"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Generation currentness (Review #7, N17).
//
// LoadAuthorityGeneration and ListAuthorityGenerations return the IMMUTABLE
// record: they say what was enrolled, never whether it is still in force.
// A generation is CURRENT only while all of these hold, and nothing that
// exercises authority may treat an immutable `State == active` as a substitute:
//
//  1. it has no invalidation record;
//  2. its authenticated liveness record is present and names its exact digest
//     (absence of a negative record is never evidence of force, I12);
//  3. no anchored generation_retired fact retires it, so deleting the
//     invalidation or replaying an earlier liveness row cannot restore it (I13);
//  4. it was not admitted before the latest governed re-anchor.
//
// requireCurrentGeneration is the single-generation form and
// currentAuthorityGenerations the enumerating form; both apply the same
// predicate as `ValidateAuthorityGeneration` and the root resolver, and every
// non-test consumer of the immutable readers is classified in
// generationConsumerRegistry (generation_consumers_test.go) so a new one cannot
// silently skip it.

// requireCurrentGeneration reports whether generation is current now.
func (r Repository) requireCurrentGeneration(ctx context.Context, generation contracts.AuthorityGeneration, now time.Time) error {
	if _, _, err := r.loadWorkPlanBlob(ctx, authorityGenerationInvalidationNamespace, generation.Ref, generation.Version, now); err == nil {
		return errors.New("authority generation is revoked or superseded")
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
		return err
	}
	return r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, generation.Ref, generation.Version, generation.Digest, now)
}

// requireCurrentLineage requires generation AND every ancestor up to its root to
// be current. It deliberately does not require the chain to terminate at THE
// installation root: the Goals-publication chain legitimately terminates at its
// own bootstrap root and has its own invalidation mechanism. It only refuses a
// chain in which any generation was retired, lost its liveness record, was
// retired by an anchored fact, or was voided by a re-anchor, and a cycle or an
// ancestor whose digest differs from the one the child bound.
func (r Repository) requireCurrentLineage(ctx context.Context, generation contracts.AuthorityGeneration, now time.Time) error {
	seen := map[string]bool{}
	for {
		key := generation.Ref + "@" + generation.Version
		if seen[key] {
			return errors.New("authority generation lineage contains a cycle")
		}
		seen[key] = true
		if err := r.requireCurrentGeneration(ctx, generation, now); err != nil {
			return err
		}
		if generation.ParentRef == "" {
			return nil
		}
		parent, err := r.LoadAuthorityGeneration(ctx, generation.ParentRef, generation.ParentVersion, now)
		if err != nil {
			return err
		}
		if parent.Digest != generation.ParentDigest {
			return errors.New("authority generation parent digest mismatch")
		}
		generation = parent
	}
}

// LoadCurrentAuthorityGeneration loads a generation and requires it to be current.
func (r Repository) LoadCurrentAuthorityGeneration(ctx context.Context, ref, version string, now time.Time) (contracts.AuthorityGeneration, error) {
	generation, err := r.LoadAuthorityGeneration(ctx, ref, version, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if err := r.requireCurrentGeneration(ctx, generation, now); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	return generation, nil
}

// currentAuthorityGenerations enumerates only the generations that are current.
// The generations, their invalidations and their liveness records are read from
// one statement and the anchored facts are read first, exactly as the root
// resolver does, so a concurrent succession or retirement is never observed
// half-applied and a store that is not current against the anchor yields nothing.
func (r Repository) currentAuthorityGenerations(ctx context.Context, now time.Time) ([]contracts.AuthorityGeneration, error) {
	snap, err := r.governanceSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	generations, invalidated, live, err := r.listAuthorityGenerationsSnapshot(ctx, now)
	if err != nil {
		return nil, err
	}
	current := make([]contracts.AuthorityGeneration, 0, len(generations))
	for _, generation := range generations {
		key := generation.Ref + "@" + generation.Version
		if invalidated[key] {
			continue
		}
		stored, ok := live[key]
		if !ok || stored.Digest != generation.Digest {
			continue
		}
		if err := r.currentAgainstFacts(snap, stored); err != nil {
			continue
		}
		current = append(current, generation)
	}
	return current, nil
}

// ListCurrentAuthorityGenerations is the exported enumerating form for consumers
// outside this package (owner authentication in goalspublication).
func (r Repository) ListCurrentAuthorityGenerations(ctx context.Context, now time.Time) ([]contracts.AuthorityGeneration, error) {
	return r.currentAuthorityGenerations(ctx, now)
}
