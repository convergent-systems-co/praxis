package goals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/plugin"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	PackageID                 = "praxis.package.goals"
	PackageVersion            = "0.1.5"
	GoalsPluginID             = "praxis.package.goals"
	GoalsPluginVersion        = "0.1.0"
	GoalsExecutableID         = "praxis.package.goals.executable"
	GoalsExecutableVersion    = "1"
	GoalsExecutablePath       = "plugins/praxis-goals-plugin"
	GoalsPluginRuntimeID      = "praxis.plugin.grpc"
	GoalsPluginRuntimeVersion = "1"
)

// PackageBuildInput creates the canonical first-party Goals package content.
// The executable bytes must come from the exact package build; this function
// never reads source checkout state or invents an executable artifact.
func PackageBuildInput(executable []byte) (packagecatalog.PackageBuildInput, error) {
	if len(executable) == 0 {
		return packagecatalog.PackageBuildInput{}, errors.New("Goals package executable bytes are required")
	}
	executableDigest := bytesDigest(executable)
	runtimeDigest := bytesDigest([]byte(GoalsPluginRuntimeID + "\x00" + GoalsPluginRuntimeVersion))
	definition := plugin.Manifest{ContractVersion: plugin.ManifestContractCurrentVersion(), ID: GoalsPluginID, Version: GoalsPluginVersion, ProtocolMin: "1", ProtocolMax: "1", Entrypoint: GoalsExecutablePath, ExecutableContentID: GoalsExecutableID, ExecutableContentVersion: GoalsExecutableVersion, Capabilities: []string{"goals.lifecycle.execute"}, ArtifactDigest: executableDigest}
	definitionBytes, err := json.Marshal(definition)
	if err != nil {
		return packagecatalog.PackageBuildInput{}, err
	}
	definitionDigest := bytesDigest(definitionBytes)
	binding := executableBinding(definitionDigest, executableDigest, runtimeDigest)
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: PackageID, Version: PackageVersion, Publisher: "publisher:praxis-first-party", Invocations: []contracts.InvocationContract{LifecycleInvocation(), GoalDriveInvocation()}, ExecutableBindings: []contracts.ExecutableBinding{binding}, Contents: []packagecatalog.ContentRef{
		{Kind: packagecatalog.ContentPlugin, ID: GoalsPluginID, Version: GoalsPluginVersion, Digest: definitionDigest, Artifact: "plugins/praxis-goals.json"},
		{Kind: packagecatalog.ContentPluginExecutable, ID: GoalsExecutableID, Version: GoalsExecutableVersion, Digest: executableDigest, Artifact: GoalsExecutablePath},
	}}
	return packagecatalog.PackageBuildInput{Manifest: manifest, Files: map[string][]byte{"plugins/praxis-goals.json": definitionBytes, GoalsExecutablePath: append([]byte(nil), executable...)}}, nil
}

func executableBinding(definitionDigest, executableDigest, runtimeDigest string) contracts.ExecutableBinding {
	return contracts.ExecutableBinding{EntryPointID: LifecycleInvocation().EntryPointID, PluginID: GoalsPluginID, PluginVersion: GoalsPluginVersion, PluginDefinitionDigest: definitionDigest, ExecutableContentID: GoalsExecutableID, ExecutableContentVersion: GoalsExecutableVersion, ExecutableDigest: executableDigest, RuntimeID: GoalsPluginRuntimeID, RuntimeVersion: GoalsPluginRuntimeVersion, RuntimeDigest: runtimeDigest, ProtocolMin: "1", ProtocolMax: "1"}
}

func bytesDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
