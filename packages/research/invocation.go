package research

import "github.com/convergent-systems-co/praxis/pkg/contracts"

func Invocation() contracts.InvocationContract {
	g:=Graph()
	return contracts.InvocationContract{
		Version:"1",PackageID:"praxis.package.research",PackageVersion:"0.1.0",GraphID:g.ID,GraphVersion:g.Version,EntryPointID:"research",Aliases:[]string{"research"},
		Options:[]contracts.InvocationOption{
			{Name:"goal-baseline",Type:"string",Required:false,Description:"optional exact Goal Baseline id/version/digest"},
			{Name:"persist",Type:"bool",Required:false,Default:"true",Description:"persist research evidence/baseline outputs subject to policy"},
			{Name:"rigor",Type:"enum",Required:false,Default:"auto",Description:"auto, direct, structured, or rigorous"},
		},
		RequiredCapabilities:[]string{"research.search"},
		OptionalCapabilities:[]string{"workspace.context.pack","document.write","presentation.dashboard"},
		RequiredEnforcement:[]string{"deterministic_authority_boundary"},
	}
}
