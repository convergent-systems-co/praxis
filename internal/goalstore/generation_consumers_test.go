package goalstore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The immutable generation readers say what was enrolled, never whether it is
// still in force (N17). Every non-test call site must therefore be classified,
// so a new consumer cannot silently treat an immutable record as authority.
//
//	E  evidence, audit, diagnostic, creation or read-back: the result never authorizes an effect
//	M  monotone: any generation, current or not, only makes the operation MORE restrictive
//	C  authority-exercising and it applies the currentness predicate in the SAME function
//
// A C entry is only accepted when the enclosing function textually calls one of
// currentnessPredicates; an E or M entry needs a reason.
var immutableGenerationReaders = map[string]bool{"LoadAuthorityGeneration": true, "ListAuthorityGenerations": true, "listAuthorityGenerationsSnapshot": true}

var currentnessPredicates = []string{"requireCurrentGeneration", "requireCurrentLineage", "ValidateAuthorityGeneration", "ValidateAuthorityGenerationLineage", "requireLive", "requireLiveIdentity", "LoadCurrentInstallationRoot", "CheckGoalsPublicationInvalidation", "currentAgainstFacts", "LoadCurrentAuthorityGeneration", "currentAuthorityGenerations", "ListCurrentAuthorityGenerations", "loadInstallationRoot", "CheckAuthorityInForceInTx", "requireLiveInTx", "governanceSnapshot", "Governance"}

type consumer struct{ class, reason string }

var generationConsumerRegistry = map[string]consumer{
	"cmd/praxis/authoritybootstrap.go:runAuthorityBootstrapWithTerminal":                         {"M", "any existing generation, current or not, forbids a second root enrollment; the idempotent 'already enrolled' path and the read-back only report"},
	"cmd/praxis/authoritybootstrap.go:runAuthorityDelegate":                                      {"C", "the delegating parent is loaded with LoadCurrentAuthorityGeneration before its enrolled OS user is trusted; the child read-back is only printed"},
	"cmd/praxis/core.go:inspectAuthorityTopologyWithRepository":                                  {"E", "doctor topology report; the 'qualified' status is derived from LoadCurrentInstallationRoot"},
	"cmd/praxis/package_manager_authority.go:runPackageManagerDeploymentApprove":                 {"C", "the root is validated with ValidateAuthorityGeneration on the same ref/version/digest before the effect"},
	"internal/goalstore/generation_currentness.go:LoadCurrentAuthorityGeneration":                {"C", "loads the immutable record and applies requireCurrentGeneration"},
	"internal/goalstore/generation_currentness.go:currentAuthorityGenerations":                   {"C", "filters one snapshot with invalidation, liveness digest, anchored retirement and re-anchor void"},
	"internal/goalstore/generation_currentness.go:requireCurrentLineage":                         {"C", "requireCurrentGeneration on the generation and every ancestor"},
	"internal/goalstore/goals_publication.go:LoadGoalsPublicationAuthorization":                  {"C", "root and child both pass CheckGoalsPublicationInvalidation on the only success path"},
	"internal/goalstore/goals_publication.go:SaveGoalsPublicationRequest":                        {"C", "CheckGoalsPublicationInvalidation on the root before the request is saved"},
	"internal/goalstore/goals_publication_recovery.go:LoadGoalsPublicationRecoveryAuthorization": {"C", "root and child both checked, and re-checked in the transaction"},
	"internal/goalstore/goals_publication_recovery.go:SaveGoalsPublicationRecoveryRequest":       {"C", "CheckGoalsPublicationInvalidation on the root before the request is saved"},
	"internal/goalstore/governance.go:InitializeGovernanceAnchor":                                {"M", "refuses to create an anchor when any generation exists; presence only restricts"},
	"internal/goalstore/package_deployment_approval.go:DerivePackageDeploymentApproval":          {"C", "issuing root via ValidateAuthorityGeneration and the operational manager generation via requireCurrentLineage before a bearer approval exists"},
	"internal/goalstore/package_deployment_approval.go:SavePackageDeploymentRequest":             {"E", "creates a pending request only; approval derivation and the approve command re-validate"},
	"internal/goalstore/reanchor.go:completeRootReadmission":                                     {"C", "refuses a root retired by an anchored fact and, since Repair 7, one carrying an invalidation record; liveness is deliberately re-established by the ceremony"},
	"internal/goalstore/reanchor.go:installationRootCandidate":                                   {"E", "derives the candidate the owner attests in the ceremony; excludes invalidated generations; admission itself is completeRootReadmission"},
	"internal/goalstore/repository.go:SaveAuthorityDecisionAndDelegatedAuthorityGeneration":      {"C", "the delegating parent must pass requireCurrentGeneration; the replay branch returns an existing child"},
	"internal/goalstore/repository.go:SaveAuthorityGenerationInvalidation":                       {"E", "builds the retirement record about that generation"},
	"internal/goalstore/repository.go:SaveDelegatedAuthorityGeneration":                          {"C", "the delegating parent must pass requireCurrentGeneration"},
	"internal/goalstore/repository.go:ValidateAuthorityGeneration":                               {"C", "the single-generation currentness predicate"},
	"internal/goalstore/repository.go:ValidateAuthorityGenerationLineage":                        {"C", "walks the chain applying invalidation and requireLive to every node and terminates at the current root"},
	"internal/goalstore/repository.go:validatePackageDeploymentDecision":                         {"C", "the operational generation must pass requireCurrentLineage"},
	"internal/goalstore/root_authority_succession.go:loadInstallationRoot":                       {"C", "the reference resolver: invalidation, liveness digest, anchored retirement and admission void"},
	"internal/goalstore/root_authority_succession.go:loadRootPredecessor":                        {"E", "reconstructs the superseded predecessor for succession lineage integrity"},
	"internal/goalstore/root_authority_succession.go:validateRootAuthorityLineage":               {"E", "lineage integrity of the already-resolved current root"},
	"internal/goalstore/historical_authority.go:loadHistoricalGeneration":                        {"E", "expired-authority evidence; the output is a documented non-executable projection, and re-requesting still needs a live decision"},
	"internal/goalstore/publisher_governance.go:SaveAuthorityModelAdoptionSupersession":          {"E", "passes the adoption root only as a row-lock target; the gate is LoadCurrentInstallationRoot in adopt and abandon"},
	"internal/goalstore/publisher_governance.go:saveAuthorityModelTransitionWithLock":            {"E", "row-lock target for the adoption root; the gate is LoadCurrentInstallationRoot"},
	"internal/goalstore/repository.go:AttachAcceptedWorkPlan":                                    {"E", "generation namespace passed as a row-lock target so an invalidation cannot race the write; the currentness gate (verifyPlanAuthorityLineage) precedes it"},
	"internal/goalstore/repository.go:SaveAcceptedWorkPlanFromAuthorityDecision":                 {"C", "ValidateAuthorityGeneration on the decision's generation"},
	"internal/goalstore/repository.go:SaveAuthorityDecision":                                     {"C", "ValidateAuthorityGeneration and requireLive before the decision is saved"},
	"internal/goalstore/repository.go:SaveRoutingIssuance":                                       {"C", "ValidateAuthorityGenerationLineage on the issuing generation"},
	"internal/lifecycle/authority.go:NewTransactionalAuthorityGuard":                             {"E", "binds the record digests at construction; it authorizes nothing"},
	"internal/lifecycle/authority.go:RevalidateInTx":                                             {"C", "in-transaction invalidation check, then the injected Governance guard (CheckAuthorityInForceInTx: liveness, anchored retirement, re-anchor void)"},
	"internal/lifecycle/authority.go:ValidateLifecycleAuthority":                                 {"C", "Source.ValidateAuthorityGeneration on the same ref/version before the load is used"},
}

func scanGenerationConsumers(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..")
	found := map[string]string{} // "path:Func" -> comma-joined predicate names present in that function
	for _, dir := range []string{"cmd", "internal", "packages", "pkg", "plugins"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			rel, _ := filepath.Rel(root, path)
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				calls := map[string]bool{}
				reads := false
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					name := ""
					switch f := call.Fun.(type) {
					case *ast.SelectorExpr:
						name = f.Sel.Name
					case *ast.Ident:
						name = f.Name
					}
					calls[name] = true
					if immutableGenerationReaders[name] {
						reads = true
					}
					// A direct read of the generation namespace is the same immutable read.
					for _, arg := range call.Args {
						if id, ok := arg.(*ast.Ident); ok && id.Name == "authorityGenerationNamespace" {
							reads = true
						}
					}
					return true
				})
				if !reads || immutableGenerationReaders[fn.Name.Name] {
					continue // the readers' own definitions are not consumers
				}
				var present []string
				for _, p := range currentnessPredicates {
					if calls[p] {
						present = append(present, p)
					}
				}
				found[filepath.ToSlash(rel)+":"+fn.Name.Name] = strings.Join(present, ",")
			}
			return nil
		})
	}
	return found
}

func TestEveryImmutableGenerationConsumerIsClassified(t *testing.T) {
	found := scanGenerationConsumers(t)
	var unclassified, stale, bad []string
	for site, predicates := range found {
		entry, ok := generationConsumerRegistry[site]
		switch {
		case !ok:
			unclassified = append(unclassified, site+"  [currentness calls in function: "+predicates+"]")
		case entry.class != "E" && entry.class != "M" && entry.class != "C":
			bad = append(bad, site+": class must be E, M or C")
		case entry.reason == "":
			bad = append(bad, site+": a classification needs a reason")
		case entry.class == "C" && predicates == "":
			bad = append(bad, site+": classified C but the function calls no currentness predicate")
		}
	}
	for site := range generationConsumerRegistry {
		if _, ok := found[site]; !ok {
			stale = append(stale, site)
		}
	}
	sort.Strings(unclassified)
	sort.Strings(stale)
	sort.Strings(bad)
	if len(unclassified)+len(stale)+len(bad) > 0 {
		t.Fatalf("immutable authority-generation readers must each be classified (E/M/C):\nunclassified:\n  %s\nstale registry entries:\n  %s\ninvalid:\n  %s", strings.Join(unclassified, "\n  "), strings.Join(stale, "\n  "), strings.Join(bad, "\n  "))
	}
}
