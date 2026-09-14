package state

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func fixturePackage(id, version, digest, alias string) packagecatalog.Manifest {
	return packagecatalog.Manifest{
		ContractVersion: packagecatalog.ManifestContractCurrentVersion(),
		PackageID:       id, Version: version, ContentDigest: digest,
		Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentGraph, ID: id + ".graph", Version: version, Digest: "sha256:graph-" + version, Artifact: "graphs/default.json"}},
		Invocations: []contracts.InvocationContract{{
			Version: "v1", PackageID: id, PackageVersion: version,
			GraphID: id + ".graph", GraphVersion: version,
			EntryPointID: id + ".default", Aliases: []string{alias},
			Options: []contracts.InvocationOption{{Name: "flag", Type: "bool"}},
		}},
	}
}

func fixturePackageArtifact(t *testing.T, manifest *packagecatalog.Manifest) []byte {
	t.Helper()
	if len(manifest.Contents) == 0 {
		artifact := []byte("package-artifact:" + manifest.PackageID + "@" + manifest.Version)
		sum := sha256.Sum256(artifact)
		manifest.ContentDigest = "sha256:" + hex.EncodeToString(sum[:])
		return artifact
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for i := range manifest.Contents {
		content := &manifest.Contents[i]
		var body []byte
		if content.Kind == packagecatalog.ContentGraph {
			graph := kernel.GraphDef{ID: content.ID, Version: content.Version, EntryNode: "done", Nodes: []kernel.NodeDef{{ID: "done", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded}}}
			var err error
			body, err = json.Marshal(graph)
			if err != nil {
				t.Fatal(err)
			}
		} else if content.Kind == packagecatalog.ContentAgentDefinition {
			binding := contracts.PackageGraphBinding{ID: strings.TrimSuffix(content.ID, "-agent") + ".graph", Version: "1"}
			for _, candidate := range manifest.Contents {
				if candidate.Kind == packagecatalog.ContentGraph {
					binding = contracts.PackageGraphBinding{ID: candidate.ID, Version: candidate.Version}
					break
				}
			}
			var err error
			body, err = json.Marshal(contracts.PackageAgentDefinition{Version: contracts.PackageAgentDefinitionCurrentVersion(), Graphs: []contracts.PackageGraphBinding{binding}})
			if err != nil {
				t.Fatal(err)
			}
		} else {
			body = []byte(string(content.Kind) + ":" + content.ID + "@" + content.Version)
		}
		sum := sha256.Sum256(body)
		content.Digest = "sha256:" + hex.EncodeToString(sum[:])
		if err := tw.WriteHeader(&tar.Header{Name: content.Artifact, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	artifact := archive.Bytes()
	sum := sha256.Sum256(artifact)
	manifest.ContentDigest = "sha256:" + hex.EncodeToString(sum[:])
	return artifact
}

func fixtureActivationRequest(t *testing.T, ctx context.Context, db *sql.DB, manifest packagecatalog.Manifest, now time.Time) (packagecatalog.Manifest, packagecatalog.ActivationRequest) {
	t.Helper()
	manifest, verified := fixtureVerifiedPackage(t, manifest, nil, now)
	actor := contracts.PrincipalRef{ID: "fixture-authority", Kind: "user"}
	intent, err := packagecatalog.NewActivationIntent(verified, actor)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	approvalID := "approval:" + manifest.PackageID + ":" + manifest.Version
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, approvalID, actor.ID, actor.Kind, intentDigest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return manifest, packagecatalog.ActivationRequest{Package: verified, Intent: intent, ApprovalID: approvalID}
}

func fixtureVerifiedPackage(t *testing.T, manifest packagecatalog.Manifest, dependencies map[string]packagecatalog.VerifiedPackage, now time.Time) (packagecatalog.Manifest, packagecatalog.VerifiedPackage) {
	t.Helper()
	artifact := fixturePackageArtifact(t, &manifest)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestSum := sha256.Sum256(manifestBytes)
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: "sha256:" + hex.EncodeToString(manifestSum[:]), ArtifactDigest: manifest.ContentDigest}
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "fixture-publisher"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	verified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope, ResolvedDependencies: dependencies, SourceKind: "fixture", SourceRef: manifest.PackageID + "@" + manifest.Version, VerifiedAt: now}, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"fixture-publisher": pub}}})
	if err != nil {
		t.Fatal(err)
	}
	return manifest, verified
}

func activateFixturePackage(t *testing.T, ctx context.Context, db *sql.DB, s *Store, manifest packagecatalog.Manifest, now time.Time) packagecatalog.Manifest {
	t.Helper()
	manifest, request := fixtureActivationRequest(t, ctx, db, manifest, now)
	if err := s.ActivatePackage(ctx, request, now); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func transitionFixturePackage(t *testing.T, ctx context.Context, db *sql.DB, s *Store, manifest packagecatalog.Manifest, operation packagecatalog.TransitionOperation, now time.Time) packagecatalog.TransitionRequest {
	t.Helper()
	actor := contracts.PrincipalRef{ID: "fixture-authority", Kind: "user"}
	approvalID := "approval:" + string(operation) + ":" + manifest.PackageID + ":" + manifest.Version
	request, err := packagecatalog.NewTransitionRequest(packagecatalog.PackageIdentity{PackageID: manifest.PackageID, Version: manifest.Version, ContentDigest: manifest.ContentDigest}, operation, actor, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, approvalID, actor.ID, actor.Kind, digest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionPackage(ctx, request, now); err != nil {
		t.Fatal(err)
	}
	return request
}

func approveAgentInstantiation(t *testing.T, ctx context.Context, db *sql.DB, request contracts.PackageAgentInstantiationRequest, now time.Time) {
	t.Helper()
	digest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, request.ApprovalID, request.Intent.Actor.ID, request.Intent.Actor.Kind, digest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}

func TestPackageActivationPublishesAndRemovesAliasesAndContents(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	m := fixturePackage("example/pkg", "1.0.0", "sha256:one", "example")
	m = activateFixturePackage(t, ctx, db, s, m, time.Now().UTC())
	got, err := s.ResolveInvocationAlias(ctx, "example")
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.PackageID != m.PackageID || got.ContentDigest != m.ContentDigest {
		t.Fatalf("wrong registered package: %#v", got)
	}
	content, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if content.PackageID != m.PackageID || len(content.ArtifactBytes) == 0 || content.Content.Digest != digestPackageBytes(content.ArtifactBytes) {
		t.Fatalf("wrong graph registration: %#v", content)
	}
	transitionFixturePackage(t, ctx, db, s, m, packagecatalog.TransitionDisable, time.Now().UTC())
	if _, err := s.ResolveInvocationAlias(ctx, "example"); err == nil {
		t.Fatal("disabled package alias must disappear")
	}
	if _, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1.0.0"); err == nil {
		t.Fatal("disabled package graph must disappear")
	}
}

func TestVerifiedGraphArtifactSurvivesActivationAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	manifest := activateFixturePackage(t, ctx, db, store, fixturePackage("research/graph", "1", "", "investigate"), time.Now().UTC())
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	graph, content, err := New(db).ResolveGraph(ctx, "research/graph.graph", "1")
	if err != nil {
		t.Fatal(err)
	}
	if graph.ID != "research/graph.graph" || graph.Version != "1" || content.PackageDigest != manifest.ContentDigest || digestPackageBytes(content.ArtifactBytes) != content.Content.Digest {
		t.Fatalf("restart did not reconstruct exact executable graph generation: graph=%+v content=%+v", graph, content)
	}
	if _, _, err := New(db).ResolveGraph(ctx, "research/graph.graph", "other"); err == nil {
		t.Fatal("graph resolution must remain pinned to the installed content version")
	}
	if _, err := db.ExecContext(ctx, `UPDATE package_contents SET artifact_bytes=? WHERE package_id=? AND kind=?`, []byte("tampered"), manifest.PackageID, packagecatalog.ContentGraph); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(db).ResolveGraph(ctx, "research/graph.graph", "1"); err == nil {
		t.Fatal("persisted content corruption must fail before graph decoding")
	}
}

func TestVerifiedDependencyClosureDeploysAtomicallyAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC)
	dependencyManifest, dependency := fixtureVerifiedPackage(t, fixturePackage("research/evidence", "1", "pending", "evidence"), nil, now)
	rootManifest := fixturePackage("delivery/root", "2", "pending", "deliver")
	rootManifest.Dependencies = []packagecatalog.Dependency{{PackageID: dependencyManifest.PackageID, Version: dependencyManifest.Version, Digest: dependencyManifest.ContentDigest}}
	rootManifest, root := fixtureVerifiedPackage(t, rootManifest, map[string]packagecatalog.VerifiedPackage{dependencyManifest.PackageID: dependency}, now)
	actor := contracts.PrincipalRef{ID: "deployment-owner", Kind: "user"}
	request, err := packagecatalog.NewDeploymentRequest(root, map[string]packagecatalog.VerifiedPackage{dependencyManifest.PackageID: dependency}, actor, "approval:deployment")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, request.ApprovalID, actor.ID, actor.Kind, digest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := New(db).DeployPackages(ctx, request, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	for _, expected := range []packagecatalog.Manifest{dependencyManifest, rootManifest} {
		installed, err := store.ActivePackage(ctx, expected.PackageID)
		if err != nil || installed.Manifest.Version != expected.Version || installed.Manifest.ContentDigest != expected.ContentDigest {
			t.Fatalf("verified closure generation did not reconstruct after restart: expected=%+v got=%+v err=%v", expected, installed, err)
		}
		graph, _, err := store.ResolveGraph(ctx, expected.PackageID+".graph", expected.Version)
		if err != nil || graph.ID != expected.PackageID+".graph" {
			t.Fatalf("deployed graph is not executable content after restart: graph=%+v err=%v", graph, err)
		}
	}
}

func TestDependencyDeploymentFailureRollsBackClosureAndAuthority(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	dependencyManifest, dependency := fixtureVerifiedPackage(t, fixturePackage("research/evidence", "1", "pending", "evidence"), nil, now)
	rootManifest := fixturePackage("delivery/root", "1", "pending", "status")
	rootManifest.Dependencies = []packagecatalog.Dependency{{PackageID: dependencyManifest.PackageID, Version: dependencyManifest.Version, Digest: dependencyManifest.ContentDigest}}
	_, root := fixtureVerifiedPackage(t, rootManifest, map[string]packagecatalog.VerifiedPackage{dependencyManifest.PackageID: dependency}, now)
	actor := contracts.PrincipalRef{ID: "deployment-owner", Kind: "user"}
	request, err := packagecatalog.NewDeploymentRequest(root, map[string]packagecatalog.VerifiedPackage{dependencyManifest.PackageID: dependency}, actor, "approval:deployment-failure")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, request.ApprovalID, actor.ID, actor.Kind, digest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := New(db).DeployPackages(ctx, request, now); err == nil {
		t.Fatal("root policy failure must abort the complete dependency deployment")
	}
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval:deployment-failure'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM installed_packages`, 0)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM package_activation_receipts`, 0)
}

func TestSinglePackageActivationCannotBypassDependencyDeployment(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	dependencyManifest, dependency := fixtureVerifiedPackage(t, fixturePackage("research/evidence", "1", "pending", "evidence"), nil, now)
	rootManifest := fixturePackage("delivery/root", "1", "pending", "deliver")
	rootManifest.Dependencies = []packagecatalog.Dependency{{PackageID: dependencyManifest.PackageID, Version: dependencyManifest.Version, Digest: dependencyManifest.ContentDigest}}
	_, root := fixtureVerifiedPackage(t, rootManifest, map[string]packagecatalog.VerifiedPackage{dependencyManifest.PackageID: dependency}, now)
	actor := contracts.PrincipalRef{ID: "owner", Kind: "user"}
	intent, err := packagecatalog.NewActivationIntent(root, actor)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, "approval:root-only", actor.ID, actor.Kind, digest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	err = New(db).ActivatePackage(ctx, packagecatalog.ActivationRequest{Package: root, Intent: intent, ApprovalID: "approval:root-only"}, now)
	if err == nil || !strings.Contains(err.Error(), "closure-bound deployment") {
		t.Fatal("root-only activation bypassed the required verified dependency deployment")
	}
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval:root-only'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM installed_packages`, 0)
}

func TestDynamicInvocationRejectsValidButDigestMismatchedPersistedContract(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manifest := activateFixturePackage(t, ctx, db, New(db), fixturePackage("client/exact", "1", "", "exact"), time.Now().UTC())
	mutated := manifest.Invocations[0]
	mutated.GraphID = "attacker.graph"
	body, err := json.Marshal(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE invocation_registry SET contract_json=? WHERE package_id=? AND active=1`, body, manifest.PackageID); err != nil {
		t.Fatal(err)
	}
	if _, err := New(db).ResolveInvocationAlias(ctx, "exact"); err == nil {
		t.Fatal("valid caller-selected contract bytes must not replace the digest-bound active contract")
	}
}

func TestAgentOnlyAndMixedPackagesAreFirstClassContents(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	agentOnly := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "agent/pkg", Version: "1", ContentDigest: "sha256:a", Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentAgentDefinition, ID: "researcher", Version: "1", Digest: "sha256:agent", Artifact: "agents/researcher.json"}}}
	activateFixturePackage(t, ctx, db, s, agentOnly, time.Now().UTC())
	agents, err := s.ActiveContents(ctx, packagecatalog.ContentAgentDefinition)
	if err != nil || len(agents) != 1 || agents[0].Content.ID != "researcher" {
		t.Fatalf("agent contents: %#v err=%v", agents, err)
	}
	mixed := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "mixed/pkg", Version: "1", ContentDigest: "sha256:m", Contents: []packagecatalog.ContentRef{
		{Kind: packagecatalog.ContentGraph, ID: "mixed.graph", Version: "1", Digest: "sha256:g", Artifact: "graphs/mixed.json"},
		{Kind: packagecatalog.ContentPlugin, ID: "mixed.provider", Version: "1", Digest: "sha256:p", Artifact: "plugins/provider"},
	}}
	activateFixturePackage(t, ctx, db, s, mixed, time.Now().UTC())
	plugins, err := s.ActiveContents(ctx, packagecatalog.ContentPlugin)
	if err != nil || len(plugins) != 1 || plugins[0].Content.ID != "mixed.provider" {
		t.Fatalf("plugin contents: %#v err=%v", plugins, err)
	}
}

func TestInstalledDefinitionsInstantiateIndependentDomainAgentsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	now := time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC)
	domains := []struct {
		name, packageID, graphID, definitionID, agentID, generationID, owner string
	}{
		{name: "software-delivery", packageID: "delivery/agent", graphID: "delivery.graph", definitionID: "delivery-agent", agentID: "agent-delivery", generationID: "agent-delivery-g1", owner: "workspace:delivery"},
		{name: "research", packageID: "research/agent", graphID: "research.graph", definitionID: "research-agent", agentID: "agent-research", generationID: "agent-research-g1", owner: "research:private"},
	}
	for index, domain := range domains {
		graphPackage := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: domain.packageID + "/graphs", Version: "1", Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentGraph, ID: domain.graphID, Version: "1", Artifact: "graphs/default.json", Digest: "pending"}}}
		activateFixturePackage(t, ctx, db, store, graphPackage, now.Add(time.Duration(index)*time.Minute))
		agentPackage := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: domain.packageID, Version: "1", Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentAgentDefinition, ID: domain.definitionID, Version: "1", Artifact: "agents/default.json", Digest: "pending"}}}
		activateFixturePackage(t, ctx, db, store, agentPackage, now.Add(time.Duration(index)*time.Minute+time.Second))
		actor := contracts.PrincipalRef{ID: "owner-" + domain.name, Kind: "user"}
		request, err := store.PreparePackageAgentInstantiation(ctx, domain.packageID, domain.definitionID, "1", domain.agentID, domain.generationID, domain.owner, "policy:"+domain.name, "approval:instantiate:"+domain.name, actor)
		if err != nil {
			t.Fatal(err)
		}
		approveAgentInstantiation(t, ctx, db, request, now)
		instance, err := store.InstantiatePackageAgent(ctx, request, now.Add(5*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if instance.AgentID != domain.agentID || instance.GenerationID != domain.generationID || instance.GraphRefs[0] != domain.graphID+"@1" || instance.GovernanceRef != "policy:"+domain.name {
			t.Fatalf("%s instantiation lost package/local identity: instance=%+v", domain.name, instance)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runtime := agent.Runtime{Events: NewSQLiteEventStore(db)}
	delivery, deliveryGeneration, _, err := runtime.Load(ctx, "agent-delivery")
	if err != nil {
		t.Fatal(err)
	}
	research, researchGeneration, _, err := runtime.Load(ctx, "agent-research")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.ID == research.ID || deliveryGeneration.ID == researchGeneration.ID || delivery.OwnerScope == research.OwnerScope || deliveryGeneration.GraphRefs[0] == researchGeneration.GraphRefs[0] {
		t.Fatalf("materially different package definitions did not retain independent local agents: delivery=%+v/%+v research=%+v/%+v", delivery, deliveryGeneration, research, researchGeneration)
	}
	events, err := NewSQLiteEventStore(db).LoadAggregate(ctx, delivery.ID, 0)
	if err != nil || len(events) != 1 || events[0].Trust != contracts.TrustUserConfirmed || events[0].Actor.ID != "owner-software-delivery" {
		t.Fatalf("governed instantiation did not retain authority-bound creation evidence: events=%+v err=%v", events, err)
	}
}

func TestAgentInstantiationCannotMutateApprovedIdentity(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Now().UTC()
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "research/agent", Version: "1", Contents: []packagecatalog.ContentRef{
		{Kind: packagecatalog.ContentGraph, ID: "research.graph", Version: "1", Artifact: "graphs/default.json", Digest: "pending"},
		{Kind: packagecatalog.ContentAgentDefinition, ID: "research-agent", Version: "1", Artifact: "agents/default.json", Digest: "pending"},
	}}
	activateFixturePackage(t, ctx, db, store, manifest, now)
	request, err := store.PreparePackageAgentInstantiation(ctx, manifest.PackageID, "research-agent", "1", "agent-original", "generation-original", "research:private", "policy:research", "approval:agent", contracts.PrincipalRef{ID: "owner", Kind: "user"})
	if err != nil {
		t.Fatal(err)
	}
	approveAgentInstantiation(t, ctx, db, request, now)
	request.AgentID = "agent-attacker-selected"
	if _, err := store.InstantiatePackageAgent(ctx, request, now.Add(time.Minute)); err == nil {
		t.Fatal("caller-mutated local agent identity crossed the intent-bound authority boundary")
	}
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval:agent'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM events WHERE aggregate_type='agent'`, 0)
}

func TestPackageCannotShadowCoreCommand(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	activateFixturePackageExpectError(t, ctx, db, New(db), fixturePackage("bad/pkg", "1", "sha256:x", "status"), time.Now().UTC(), "reserved core command")
}

func TestPackageAliasCollisionFailsWithoutChangingExistingOwner(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	now := time.Now().UTC()
	activateFixturePackage(t, ctx, db, s, fixturePackage("a/pkg", "1", "sha256:a", "shared"), now)
	activateFixturePackageExpectError(t, ctx, db, s, fixturePackage("b/pkg", "1", "sha256:b", "shared"), now, "alias collision")
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval:b/pkg:1'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM package_activation_receipts WHERE package_id='b/pkg'`, 0)
	got, err := s.ResolveInvocationAlias(ctx, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.PackageID != "a/pkg" {
		t.Fatalf("existing alias owner changed: %s", got.Contract.PackageID)
	}
}

func TestPackageUpdateAtomicallyReplacesAliasAndContentSurface(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	now := time.Now().UTC()
	activateFixturePackage(t, ctx, db, s, fixturePackage("example/pkg", "1", "sha256:1", "old"), now)
	v2 := activateFixturePackage(t, ctx, db, s, fixturePackage("example/pkg", "2", "sha256:2", "new"), now.Add(time.Second))
	if _, err := s.ResolveInvocationAlias(ctx, "old"); err == nil {
		t.Fatal("old alias must not remain active")
	}
	if _, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1"); err == nil {
		t.Fatal("old graph generation must not remain active")
	}
	got, err := s.ResolveInvocationAlias(ctx, "new")
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.PackageVersion != "2" || got.ContentDigest != v2.ContentDigest {
		t.Fatalf("new generation not active: %#v", got)
	}
}

func TestPackageActivationRequiresExactPersistedAuthorityAndIsAtomic(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC)
	manifest, request := fixtureActivationRequest(t, ctx, db, fixturePackage("research/pkg", "1", "", "research"), now)
	request.Intent.Actor = contracts.PrincipalRef{ID: "caller-selected", Kind: "user"}
	if err := store.ActivatePackage(ctx, request, now); err == nil {
		t.Fatal("caller-selected authority must not activate a package")
	}
	if _, err := store.ActivePackage(ctx, manifest.PackageID); err == nil {
		t.Fatal("denied activation must not publish an active package")
	}
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval:research/pkg:1'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM package_activation_receipts WHERE package_id='research/pkg'`, 0)
}

func TestPackageVerificationAuthorityAndRegistrationsSurviveRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	now := time.Date(2026, 9, 13, 18, 30, 0, 0, time.UTC)
	manifest, request := fixtureActivationRequest(t, ctx, db, fixturePackage("delivery/pkg", "1", "", "deliver"), now)
	wantVerificationID := request.Package.Evidence().ID
	wantIntentDigest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ActivatePackage(ctx, request, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	replayed := New(restarted)
	receipts, err := replayed.PackageActivationReceipts(ctx, manifest.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("one consumed approval must produce one durable activation receipt, got %d", len(receipts))
	}
	receipt := receipts[0]
	if receipt.Verification.ID != wantVerificationID || receipt.IntentDigest != wantIntentDigest || receipt.ApprovalID != request.ApprovalID || receipt.Authority != request.Intent.Actor {
		t.Fatalf("restart lost verification/authority lineage: %#v", receipt)
	}
	if digestPackageBytes(receipt.ManifestBytes) != receipt.Verification.ManifestDigest || digestPackageBytes(receipt.ArtifactBytes) != receipt.Verification.ArtifactDigest {
		t.Fatal("restart lost exact manifest/artifact bytes behind verification evidence")
	}
	resolved, err := replayed.ResolveInvocationAlias(ctx, "deliver")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Contract.PackageID != manifest.PackageID || resolved.ContentDigest != manifest.ContentDigest {
		t.Fatalf("restart changed active client-visible generation: %#v", resolved)
	}
}

func TestGovernedDisableAndRemovePreserveHistoryAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	now := time.Date(2026, 9, 13, 19, 0, 0, 0, time.UTC)
	manifest := activateFixturePackage(t, ctx, db, store, fixturePackage("research/lifecycle", "1", "", "study"), now)
	disable := transitionFixturePackage(t, ctx, db, store, manifest, packagecatalog.TransitionDisable, now.Add(time.Minute))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store = New(restarted)
	if _, err := store.ResolveInvocationAlias(ctx, "study"); err == nil {
		t.Fatal("disabled package command returned after restart")
	}
	selected, err := store.SelectedPackage(ctx, manifest.PackageID)
	if err != nil || selected.State != "disabled" || selected.Manifest.ContentDigest != manifest.ContentDigest {
		t.Fatalf("disabled exact generation is not selectable for governed removal: selected=%+v err=%v", selected, err)
	}
	receipts, err := store.PackageTransitionReceipts(ctx, manifest.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].Request.Operation != packagecatalog.TransitionDisable || receipts[0].Request.Identity.ContentDigest != manifest.ContentDigest || receipts[0].Request.ApprovalID != disable.ApprovalID {
		t.Fatalf("restart lost exact disable evidence: %#v", receipts)
	}
	remove := transitionFixturePackage(t, ctx, restarted, store, manifest, packagecatalog.TransitionRemove, now.Add(2*time.Minute))
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}

	finalDB, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer finalDB.Close()
	finalStore := New(finalDB)
	installed, err := finalStore.InstalledPackages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range installed {
		if item.Manifest.PackageID == manifest.PackageID {
			t.Fatal("removed package remained discoverable as installed")
		}
	}
	receipts, err = finalStore.PackageTransitionReceipts(ctx, manifest.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	operations := map[packagecatalog.TransitionOperation]string{}
	for _, receipt := range receipts {
		operations[receipt.Request.Operation] = receipt.Request.ApprovalID
	}
	if operations[packagecatalog.TransitionDisable] != disable.ApprovalID || operations[packagecatalog.TransitionRemove] != remove.ApprovalID {
		t.Fatalf("remove restart lost governed transition history: %#v", operations)
	}
}

func TestPackageTransitionCannotChangeOperationAfterApproval(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Date(2026, 9, 13, 19, 30, 0, 0, time.UTC)
	manifest := activateFixturePackage(t, ctx, db, store, fixturePackage("delivery/transition", "1", "", "ship"), now)
	actor := contracts.PrincipalRef{ID: "fixture-authority", Kind: "user"}
	request, err := packagecatalog.NewTransitionRequest(packagecatalog.PackageIdentity{PackageID: manifest.PackageID, Version: manifest.Version, ContentDigest: manifest.ContentDigest}, packagecatalog.TransitionDisable, actor, "approval-transition-tamper")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, request.ApprovalID, actor.ID, actor.Kind, digest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	request.Operation = packagecatalog.TransitionRemove
	if err := store.TransitionPackage(ctx, request, now.Add(time.Second)); err == nil {
		t.Fatal("caller changed approved disable into removal")
	}
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval-transition-tamper'`, 1)
	if _, err := store.ResolveInvocationAlias(ctx, "ship"); err != nil {
		t.Fatalf("denied transition changed active surface: %v", err)
	}
}

func activateFixturePackageExpectError(t *testing.T, ctx context.Context, db *sql.DB, s *Store, manifest packagecatalog.Manifest, now time.Time, label string) {
	t.Helper()
	_, request := fixtureActivationRequest(t, ctx, db, manifest, now)
	if err := s.ActivatePackage(ctx, request, now); err == nil {
		t.Fatalf("%s must fail", label)
	}
}
