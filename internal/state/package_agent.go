package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func (s *Store) PreparePackageAgentInstantiation(ctx context.Context, packageID, definitionID, definitionVersion, agentID, generationID, ownerScope, governanceRef, approvalID string, actor contracts.PrincipalRef) (contracts.PackageAgentInstantiationRequest, error) {
	if s == nil || s.db == nil {
		return contracts.PackageAgentInstantiationRequest{}, errors.New("state store is required")
	}
	content, err := s.ResolveContent(ctx, packagecatalog.ContentAgentDefinition, definitionID, definitionVersion)
	if err != nil {
		return contracts.PackageAgentInstantiationRequest{}, err
	}
	if content.PackageID != packageID {
		return contracts.PackageAgentInstantiationRequest{}, errors.New("agent definition is not owned by requested package")
	}
	definition, err := decodePackageAgentDefinition(content.ArtifactBytes)
	if err != nil {
		return contracts.PackageAgentInstantiationRequest{}, err
	}
	for _, graph := range definition.Graphs {
		if _, _, err := s.ResolveGraph(ctx, graph.ID, graph.Version); err != nil {
			return contracts.PackageAgentInstantiationRequest{}, fmt.Errorf("resolve agent definition graph %s: %w", graph.Ref(), err)
		}
	}
	request := contracts.PackageAgentInstantiationRequest{
		PackageID: packageID, PackageVersion: content.PackageVersion, PackageDigest: content.PackageDigest,
		DefinitionID: definitionID, DefinitionVersion: definitionVersion,
		AgentID: agentID, GenerationID: generationID, OwnerScope: ownerScope, GovernanceRef: governanceRef,
		ApprovalID: approvalID,
	}
	request.Intent, err = packageAgentInstantiationIntent(request, content.Content.Digest, definition, actor)
	if err != nil {
		return contracts.PackageAgentInstantiationRequest{}, err
	}
	return request, request.ValidateShape()
}

func (s *Store) InstantiatePackageAgent(ctx context.Context, request contracts.PackageAgentInstantiationRequest, now time.Time) (contracts.PackageAgentInstance, error) {
	if s == nil || s.db == nil || now.IsZero() {
		return contracts.PackageAgentInstance{}, errors.New("state store and instantiation time are required")
	}
	if err := request.ValidateShape(); err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	defer tx.Rollback()

	var content packagecatalog.ContentRef
	var packageID, packageVersion, packageDigest string
	var definitionBytes []byte
	err = tx.QueryRowContext(ctx, `SELECT package_id,package_version,content_digest,kind,content_id,content_version,artifact_digest,artifact_ref,COALESCE(compatibility,''),artifact_bytes FROM package_contents WHERE active=1 AND package_id=? AND package_version=? AND content_digest=? AND kind=? AND content_id=? AND content_version=?`, request.PackageID, request.PackageVersion, request.PackageDigest, packagecatalog.ContentAgentDefinition, request.DefinitionID, request.DefinitionVersion).Scan(&packageID, &packageVersion, &packageDigest, &content.Kind, &content.ID, &content.Version, &content.Digest, &content.Artifact, &content.Compatibility, &definitionBytes)
	if err != nil {
		return contracts.PackageAgentInstance{}, fmt.Errorf("resolve exact active agent definition: %w", err)
	}
	if digestPackageBytes(definitionBytes) != content.Digest {
		return contracts.PackageAgentInstance{}, errors.New("persisted agent definition digest mismatch")
	}
	definition, err := decodePackageAgentDefinition(definitionBytes)
	if err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	for _, graph := range definition.Graphs {
		if err := validateDefinitionGraphTx(ctx, tx, graph); err != nil {
			return contracts.PackageAgentInstance{}, err
		}
	}
	expected, err := packageAgentInstantiationIntent(request, content.Digest, definition, request.Intent.Actor)
	if err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	wantDigest, err := expected.Digest()
	if err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	gotDigest, err := request.Intent.Digest()
	if err != nil || gotDigest != wantDigest {
		return contracts.PackageAgentInstance{}, errors.New("agent instantiation intent does not bind exact package definition and local identity")
	}
	if err := consumePackageApproval(ctx, tx, request.ApprovalID, request.Intent.Actor, gotDigest, now); err != nil {
		return contracts.PackageAgentInstance{}, err
	}

	graphRefs := make([]string, 0, len(definition.Graphs))
	for _, graph := range definition.Graphs {
		graphRefs = append(graphRefs, graph.Ref())
	}
	identity := persistedPackageAgent{ID: request.AgentID, OwnerScope: request.OwnerScope, CurrentGeneration: request.GenerationID, Lifecycle: "active", CreatedAt: now.UTC()}
	generation := persistedPackageGeneration{
		ID: request.GenerationID, AgentID: request.AgentID, Number: 1, GraphRefs: graphRefs,
		PreferenceRef: definition.PreferenceRef, CreationReason: "instantiated from " + packageID + "@" + packageVersion + ":" + content.ID,
		GovernanceRef: request.GovernanceRef, CreatedAt: now.UTC(),
	}
	payload, err := json.Marshal(struct {
		Agent      persistedPackageAgent      `json:"agent"`
		Generation persistedPackageGeneration `json:"generation"`
	}{identity, generation})
	if err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	commandSum := sha256.Sum256([]byte(gotDigest + "\x00" + request.ApprovalID))
	commandID := "agent-instantiation:sha256:" + hex.EncodeToString(commandSum[:])
	command := CommandRecord{ID: commandID, Type: "agent.instantiate", Version: request.Intent.Version, Actor: request.Intent.Actor, Scope: request.OwnerScope, CorrelationID: commandID, Payload: payload, CreatedAt: now.UTC()}
	if err := insertCommand(ctx, tx, command); err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	if err := compareAndAdvanceAggregate(ctx, tx, identity.ID, "agent", 0, 1); err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	provenance, _ := json.Marshal(map[string]string{"package_id": packageID, "package_version": packageVersion, "package_digest": packageDigest, "definition_id": content.ID, "definition_version": content.Version, "definition_digest": content.Digest, "approval_id": request.ApprovalID})
	event := EventRecord{ID: identity.ID + ":generation:" + generation.ID, AggregateID: identity.ID, AggregateType: "agent", AggregateVersion: 1, Type: "agent.created", Version: "1", Actor: request.Intent.Actor, CommandID: commandID, CorrelationID: commandID, ProvenanceJSON: provenance, TrustClass: contracts.TrustUserConfirmed, Payload: payload, CreatedAt: now.UTC()}
	if err := insertEvent(ctx, tx, event); err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE commands SET status='committed',completed_at=? WHERE command_id=?`, now.UTC().Format(time.RFC3339Nano), commandID); err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	if err := tx.Commit(); err != nil {
		return contracts.PackageAgentInstance{}, err
	}
	return contracts.PackageAgentInstance{AgentID: identity.ID, GenerationID: generation.ID, OwnerScope: identity.OwnerScope, GraphRefs: append([]string(nil), graphRefs...), PreferenceRef: generation.PreferenceRef, GovernanceRef: generation.GovernanceRef, CreatedAt: identity.CreatedAt}, nil
}

type persistedPackageAgent struct {
	ID                string
	OwnerScope        string
	CurrentGeneration string
	Lifecycle         string
	CreatedAt         time.Time
}

type persistedPackageGeneration struct {
	ID               string
	AgentID          string
	Number           uint64
	ParentGeneration string
	GraphRefs        []string
	LearningRefs     []string
	PreferenceRef    string
	CreationReason   string
	GovernanceRef    string
	CreatedAt        time.Time
}

func decodePackageAgentDefinition(body []byte) (contracts.PackageAgentDefinition, error) {
	var definition contracts.PackageAgentDefinition
	if err := json.Unmarshal(body, &definition); err != nil {
		return contracts.PackageAgentDefinition{}, fmt.Errorf("decode package agent definition: %w", err)
	}
	if err := definition.Validate(); err != nil {
		return contracts.PackageAgentDefinition{}, fmt.Errorf("validate package agent definition: %w", err)
	}
	return definition, nil
}

func validateDefinitionGraphTx(ctx context.Context, tx *sql.Tx, binding contracts.PackageGraphBinding) error {
	var digest string
	var body []byte
	if err := tx.QueryRowContext(ctx, `SELECT artifact_digest,artifact_bytes FROM package_contents WHERE active=1 AND kind=? AND content_id=? AND content_version=?`, packagecatalog.ContentGraph, binding.ID, binding.Version).Scan(&digest, &body); err != nil {
		return fmt.Errorf("resolve exact agent graph %s: %w", binding.Ref(), err)
	}
	if digestPackageBytes(body) != digest {
		return fmt.Errorf("agent graph %s artifact digest mismatch", binding.Ref())
	}
	var graph kernel.GraphDef
	if err := json.Unmarshal(body, &graph); err != nil {
		return fmt.Errorf("decode agent graph %s: %w", binding.Ref(), err)
	}
	if graph.ID != binding.ID || graph.Version != binding.Version {
		return fmt.Errorf("agent graph %s identity mismatch", binding.Ref())
	}
	return graph.Validate()
}

func packageAgentInstantiationIntent(request contracts.PackageAgentInstantiationRequest, definitionDigest string, definition contracts.PackageAgentDefinition, actor contracts.PrincipalRef) (contracts.ActionIntent, error) {
	if err := actor.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	graphRefs := make([]string, 0, len(definition.Graphs))
	for _, graph := range definition.Graphs {
		graphRefs = append(graphRefs, graph.Ref())
	}
	sort.Strings(graphRefs)
	graphBody, err := json.Marshal(graphRefs)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	intent := contracts.ActionIntent{
		Version: contracts.AgentInstantiationIntentCurrentVersion(), ID: "agent-instantiation:" + request.AgentID + "@" + request.GenerationID,
		Actor: actor, Operation: "agent.instantiate", Target: request.AgentID + "@" + request.GenerationID, Scope: request.OwnerScope,
		Parameters: map[string]string{
			"package_id": request.PackageID, "package_version": request.PackageVersion, "package_digest": request.PackageDigest,
			"definition_id": request.DefinitionID, "definition_version": request.DefinitionVersion, "definition_digest": definitionDigest,
			"graphs_digest": digestPackageBytes(graphBody), "owner_scope": request.OwnerScope, "governance_ref": request.GovernanceRef,
		},
	}
	return intent, intent.Validate()
}
