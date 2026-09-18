package contracts

// authorityModelPredecessor is the ADR-094 succession graph: each model
// version binds the exact digest of the version it succeeds by value
// (see the digest constructors), so a valid active model retains every
// model on its predecessor chain. v4 and v5 are the installation-scoped Goals
// branch off v3; v6 is the global routing branch off v3.
var authorityModelPredecessor = map[string]string{
	AuthorityModelSuccessorVersion:        AuthorityModelVersion,
	AuthorityModelDeploymentVersion:       AuthorityModelSuccessorVersion,
	AuthorityModelGoalsPublicationVersion: AuthorityModelDeploymentVersion,
	AuthorityModelGoalsRecoveryVersion:    AuthorityModelGoalsPublicationVersion,
	AuthorityModelRoutingVersion:          AuthorityModelDeploymentVersion,
}

// AuthorityModelRetains reports whether the exact active model identity
// preserves the semantics of requiredVersion: the active identity must be a
// supported immutable model, and requiredVersion must be the active version
// itself or lie on its predecessor chain. Retention never flows backward
// (v3 does not retain v6) and never crosses branches (v5 does not retain v6).
func AuthorityModelRetains(activeVersion, activeDigest, requiredVersion string) bool {
	if requiredVersion == "" || ValidateAuthorityModel(AuthorityModelID, activeVersion, activeDigest) != nil {
		return false
	}
	for version := activeVersion; version != ""; version = authorityModelPredecessor[version] {
		if version == requiredVersion {
			return true
		}
	}
	return false
}

// AuthorityModelStateRetains applies AuthorityModelRetains to a durable
// active model state.
func AuthorityModelStateRetains(state AuthorityModelState, requiredVersion string) bool {
	return state.ActiveModel == AuthorityModelID && AuthorityModelRetains(state.ActiveVersion, state.ActiveDigest, requiredVersion)
}
