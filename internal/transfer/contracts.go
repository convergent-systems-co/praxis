package transfer

import "github.com/convergent-systems-co/praxis/pkg/contracts"

var (
	transferPolicyVersions      = currentContract("knowledge_transfer.policy")
	transferRequestVersions     = currentContract("knowledge_transfer.request")
	transferArtifactVersions    = currentContract("knowledge_transfer.artifact")
	transferEvaluationVersions  = currentContract("knowledge_transfer.evaluation")
	transferPublicationVersions = currentContract("knowledge_transfer.publication")
	transferAdoptionVersions    = currentContract("knowledge_transfer.adoption")
)

func currentContract(name string) *contracts.VersionRegistry {
	return contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: name, CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
}
