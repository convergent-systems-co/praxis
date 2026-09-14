package goals

import "github.com/convergent-systems-co/praxis/pkg/contracts"

func Invocation() contracts.InvocationContract {
	return contracts.InvocationContract{
		Version: "1", PackageID: "praxis.package.goals", PackageVersion: "0.1.0",
		GraphID: "praxis.package.goals.default", GraphVersion: "0.1.0", EntryPointID: "goals",
		Aliases: []string{"goals", "design"},
		Options: []contracts.InvocationOption{
			{Name: "rigor", Type: "enum", Default: "auto", Description: "auto, direct, structured, or rigorous"},
			{Name: "recommendations", Type: "enum", Default: "calibrate", Description: "calibrate, review-all, or delegate-clear"},
			{Name: "persist", Type: "bool", Default: "true", Description: "persist the resulting Goal Baseline"},
		},
		OptionalCapabilities: []string{"workspace.context.pack", "research.search", "presentation.dashboard"},
		RequiredEnforcement:  []string{"deterministic_authority_boundary"},
	}
}

// GoalDriveInvocation describes the post-release controller surface. It is a
// package-owned contract and must be activated through the normal verified
// invocation registry; discovery grants no execution authority.
func GoalDriveInvocation() contracts.InvocationContract {
	return contracts.InvocationContract{
		Version: "1", PackageID: "praxis.package.goals", PackageVersion: "0.1.0",
		GraphID: "praxis.package.goals.default", GraphVersion: "0.1.0", EntryPointID: "goal-drive",
		Aliases: []string{"goal-drive"},
		Options: []contracts.InvocationOption{
			{Name: "goal", Type: "string", Description: "literal Goal text"},
			{Name: "goal-file", Type: "path", Description: "Goal input file captured with provenance and digest"},
			{Name: "goal-id", Type: "string", Description: "exact durable Goal generation"},
			{Name: "mode", Type: "enum", Default: "supervised", Description: "supervised stops after one persisted progressed unit; continuous permits bounded repetition"},
			{Name: "invocation-id", Type: "string", Description: "durable identity of this supervised or continuous invocation"},
			{Name: "provider", Type: "string", Description: "registered provider identity"},
			{Name: "model", Type: "string", Description: "provider model/profile hint"},
			{Name: "repo", Type: "path", Description: "repository workspace"},
			{Name: "branch", Type: "string", Description: "repository branch authority"},
			{Name: "max-turns", Type: "integer", Description: "bounded maximum worker turns"},
			{Name: "turn-timeout", Type: "duration", Description: "per-turn deadline"},
			{Name: "no-progress-limit", Type: "integer", Description: "bounded repeated no-progress turns"},
			{Name: "ledger", Type: "path", Description: "authoritative Praxis state location"},
			{Name: "no-push", Type: "bool", Description: "retain validated local checkpoint without publication"},
			{Name: "require-clean", Type: "bool", Default: "true", Description: "require clean synchronized repository"},
		},
		RequiredEnforcement: []string{"deterministic_authority_boundary", "goal_drive_controller"},
	}
}
