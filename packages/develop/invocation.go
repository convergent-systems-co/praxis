package develop

import "github.com/convergent-systems-co/praxis/pkg/contracts"

func InvocationContract() contracts.InvocationContract {
	g := Graph()
	return contracts.InvocationContract{
		Version:        "v1",
		PackageID:      "praxis/develop",
		PackageVersion: "0.2.0",
		GraphID:        g.ID,
		GraphVersion:   g.Version,
		EntryPointID:   "praxis.package.develop.default",
		Aliases:        []string{"develop"},
		Options: []contracts.InvocationOption{
			{Name: "dashboard", Type: "bool", Required: false, Default: "false", Description: "attach the dashboard presentation capability"},
			{Name: "mode", Type: "enum:auto|fast|deep", Required: false, Default: "auto", Description: "reasoning/latency posture"},
			{Name: "workspace", Type: "string", Required: false, Description: "workspace/repository path or configured workspace identity"},
			{Name: "goal-baseline", Type: "string", Required: false, Description: "optional exact Goal Baseline id/version/digest reference"},
			{Name: "approval", Type: "enum:policy|interactive", Required: false, Default: "policy", Description: "approval posture within local policy"},
		},
		RequiredCapabilities: []string{"workspace.search.path", "workspace.search.text", "workspace.context.pack"},
		OptionalCapabilities: []string{"presentation.dashboard", "vcs.write", "ci.read", "pr.write"},
	}
}
