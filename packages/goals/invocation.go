package goals

import "github.com/convergent-systems-co/praxis/pkg/contracts"

func Invocation() contracts.InvocationContract {
	return contracts.InvocationContract{
		Version: "1", PackageID: "praxis.package.goals", PackageVersion: PackageVersion,
		GraphID: "praxis.package.goals.default", GraphVersion: "0.3.0", EntryPointID: "goals",
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
		Version: "1", PackageID: "praxis.package.goals", PackageVersion: PackageVersion,
		GraphID: "praxis.package.goals.default", GraphVersion: "0.3.0", EntryPointID: "goal-drive",
		Aliases: []string{"goal-drive"},
		Options: []contracts.InvocationOption{
			{Name: "goal-id", Type: "string", Description: "exact durable Goal identity; goal-drive runs an existing Goal only and never creates one from prose (introduce a Goal with goals-lifecycle --operation=intake)"},
			{Name: "goal-version", Type: "string", Description: "exact immutable Goal Baseline generation version"},
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
			{Name: "recover-turn", Type: "string", Description: "exact durable BLOCKED turn identity whose uncommitted consequence this invocation validates, corrects, or discards under the same objective"},
		},
		RequiredEnforcement: []string{"deterministic_authority_boundary", "goal_drive_controller"},
	}
}

// LifecycleInvocation is the package-owned control surface for durable Goals
// governance. The core CLI resolves and dispatches this contract dynamically;
// it does not know these operations or their payload semantics.
func LifecycleInvocation() contracts.InvocationContract {
	return contracts.InvocationContract{
		Version: "1", PackageID: "praxis.package.goals", PackageVersion: PackageVersion,
		GraphID: "praxis.package.goals.default", GraphVersion: "0.3.0", EntryPointID: "goals-lifecycle",
		Aliases: []string{"goals-lifecycle"},
		Options: []contracts.InvocationOption{
			{Name: "operation", Type: "enum", Required: true, Description: "intake, import, propose, review, request, decide, accept, attach, inspect, evaluate, complete, or succeed (bind is an alias of accept)"},
			{Name: "input", Type: "path", Required: false, Description: "document path: the prose Goal document for intake, the canonical baseline import for import, the planner's decomposition for propose, or a legacy full lifecycle document"},
			{Name: "goal-id", Type: "string", Required: false, Description: "exact Goal identity (intake, inspect, request, attach)"},
			{Name: "goal-version", Type: "string", Required: false, Description: "exact Goal Baseline generation (inspect, request, attach); intake defaults to 1"},
			{Name: "proposal-digest", Type: "string", Required: false, Description: "exact durable WorkPlan proposal digest (review, request)"},
			{Name: "review-digest", Type: "string", Required: false, Description: "exact durable review digest (request)"},
			{Name: "request-digest", Type: "string", Required: false, Description: "exact durable authority request digest (accept)"},
			{Name: "acceptance-ref", Type: "string", Required: false, Description: "exact durable acceptance reference (attach)"},
			{Name: "status", Type: "enum", Required: false, Description: "reviewer judgment: acceptable_for_authority_decision, revision_required, insufficient_evidence, or authority_conflict (review)"},
			{Name: "reviewer-id", Type: "string", Required: false, Description: "reviewer principal identity; defaults to the installation owner (review)"},
			{Name: "reviewer-kind", Type: "string", Required: false, Description: "reviewer principal kind; defaults to human (review)"},
			{Name: "reviewer-generation", Type: "string", Required: false, Description: "reviewer generation; defaults to the current installation root generation (review)"},
			{Name: "finding", Type: "string", Required: false, Description: "optional review finding (review)"},
			{Name: "reason", Type: "string", Required: false, Description: "optional human reason recorded with the request (request)"},
		},
		RequiredCapabilities: []string{"goals.lifecycle.execute"},
		RequiredEnforcement:  []string{"deterministic_authority_boundary", "goals_lifecycle_controller"},
	}
}
