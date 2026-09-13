package goals

import "github.com/convergent-systems-co/praxis/pkg/contracts"

func Invocation() contracts.InvocationContract {
	return contracts.InvocationContract{
		Version:"1", PackageID:"praxis.package.goals", PackageVersion:"0.1.0",
		GraphID:"praxis.package.goals.default", GraphVersion:"0.1.0", EntryPointID:"goals",
		Aliases:[]string{"goals","design"},
		Options:[]contracts.InvocationOption{
			{Name:"rigor",Type:"enum",Default:"auto",Description:"auto, direct, structured, or rigorous"},
			{Name:"recommendations",Type:"enum",Default:"calibrate",Description:"calibrate, review-all, or delegate-clear"},
			{Name:"persist",Type:"bool",Default:"true",Description:"persist the resulting Goal Baseline"},
		},
		OptionalCapabilities:[]string{"workspace.context.pack","research.search","presentation.dashboard"},
		RequiredEnforcement:[]string{"deterministic_authority_boundary"},
	}
}
