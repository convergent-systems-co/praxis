package goalstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// ValidatePackagePublishAuthority resolves the effective v2 authority
// generation from durable governance state. It deliberately does not use
// CapabilityLease: package publishing is governed authority, not runtime
// capability. The publisher generation digest is the exact delegated subject.
func (r Repository) ValidatePackagePublishAuthority(ctx context.Context, publisherGenerationDigest, packageID string, now time.Time) error {
	_, err := r.ResolvePackagePublishAuthority(ctx, publisherGenerationDigest, packageID, now)
	return err
}

// ResolvePackagePublishAuthority returns the exact effective authority and
// its request/decision lineage. Callers must use this result in provenance;
// nil validation alone is not sufficient for an owner-reviewed operation.
func (r Repository) ResolvePackagePublishAuthority(ctx context.Context, publisherGenerationDigest, packageID string, now time.Time) (contracts.PackagePublishAuthorization, error) {
	if publisherGenerationDigest == "" || packageID == "" {
		return contracts.PackagePublishAuthorization{}, errors.New("publisher generation and package identity are required")
	}
	// N17: only CURRENT generations are candidates. The immutable record's
	// State says what was enrolled, not whether it is still in force: a child
	// retired by invalidation, missing its liveness record, retired by an
	// anchored fact, or voided by a re-anchor must never authorize signing,
	// however current its parent decision still is.
	generations, err := r.currentAuthorityGenerations(ctx, now)
	if err != nil {
		return contracts.PackagePublishAuthorization{}, err
	}
	for _, generation := range generations {
		if generation.State != contracts.AuthorityGenerationActive || generation.AuthorityModelVersion != contracts.AuthorityModelSuccessorVersion || generation.AuthorityModelDigest != contracts.AuthorityModelSuccessorDigest() || generation.DelegationProfile != contracts.DelegationProfilePackagePublish || generation.SubjectDigest != publisherGenerationDigest || !containsAuthority(generation.Authorities, contracts.GovernedPackagePublish) {
			continue
		}
		if !strings.HasPrefix(generation.Scope, "package-namespace:") {
			continue
		}
		namespace := strings.TrimPrefix(generation.Scope, "package-namespace:")
		// The namespace is encoded in the canonical authority scope, not by
		// lexical prefix matching on the package identifier.
		want, scopeErr := contracts.PackagePublishScope(namespace)
		if scopeErr == nil && generation.Scope == want && contracts.PackageIDInNamespace(packageID, namespace) {
			requestID, requestVersion, ok := strings.Cut(generation.DelegationRef, "/")
			if !ok {
				continue
			}
			request, requestErr := r.LoadAuthorityRequest(ctx, requestID, requestVersion, now)
			if requestErr != nil {
				continue
			}
			decision, decisionErr := r.LoadAuthorityDecision(ctx, requestID, requestVersion, now)
			if decisionErr != nil || decision.RequestDigest == "" {
				continue
			}
			requestDigest, digestErr := request.DigestAt(now)
			if digestErr != nil || requestDigest != decision.RequestDigest || generation.DelegationDigest == "" {
				continue
			}
			// The whole lineage must be current, not only the child: the parent
			// decision above is effective on its own liveness, but it says nothing
			// about the root that issued it. A child of a superseded root, or with a
			// retired ancestor, must never authorize signing.
			if lineageErr := r.requireCurrentLineage(ctx, generation, now); lineageErr != nil {
				continue
			}
			return contracts.PackagePublishAuthorization{Generation: generation, Request: request, Decision: decision}, nil
		}
	}
	return contracts.PackagePublishAuthorization{}, errors.New("no current bounded package.publish authority for publisher generation and package")
}

func containsAuthority(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
