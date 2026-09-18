package goalstore

import (
	"context"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// ResolvePackageManagerAuthority resolves the installation-local operational
// principal. The logical principal is not sufficient: the delegated
// generation and its exact governance-root parent are part of the identity.
func (r Repository) ResolvePackageManagerAuthority(ctx context.Context, installationRootDigest string, now time.Time) (contracts.AuthorityGeneration, error) {
	if installationRootDigest == "" {
		return contracts.AuthorityGeneration{}, errors.New("installation root digest is required")
	}
	model, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if !contracts.AuthorityModelStateRetains(model, contracts.AuthorityModelDeploymentVersion) {
		return contracts.AuthorityGeneration{}, errors.New("adopted authority model does not retain v3 deployment semantics")
	}
	generations, err := r.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	principal := contracts.PackageManagerPrincipal()
	for _, generation := range generations {
		if generation.State != contracts.AuthorityGenerationActive || generation.Principal != principal || generation.AuthorityModelVersion != contracts.AuthorityModelDeploymentVersion || generation.AuthorityModelDigest != contracts.AuthorityModelDeploymentDigest() || generation.DelegationProfile != contracts.DelegationProfilePackageDeploy || generation.ParentDigest != installationRootDigest || generation.SubjectKind != principal.Kind || generation.SubjectID != principal.ID || !containsAuthority(generation.Authorities, contracts.GovernedPackageDeploy) {
			continue
		}
		if err := generation.VerifyDigest(); err != nil {
			return contracts.AuthorityGeneration{}, err
		}
		if generation.ExpiresAt != nil && !now.Before(generation.ExpiresAt.UTC()) {
			return contracts.AuthorityGeneration{}, errors.New("package-manager authority is expired")
		}
		return generation, nil
	}
	return contracts.AuthorityGeneration{}, errors.New("installation-bound package-manager authority is unavailable")
}
