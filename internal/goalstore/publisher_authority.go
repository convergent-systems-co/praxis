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
	if publisherGenerationDigest == "" || packageID == "" {
		return errors.New("publisher generation and package identity are required")
	}
	generations, err := r.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return err
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
			return nil
		}
	}
	return errors.New("no current bounded package.publish authority for publisher generation and package")
}

func containsAuthority(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
