package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func packageManagerProposalCurrent(ctx context.Context, repo goalstore.Repository, p contracts.GovernedAuthorityProposal, now time.Time) error {
	if p.Profile != contracts.DelegationProfilePackageDeploy || p.Capability != contracts.GovernedPackageDeploy || p.PrincipalID != contracts.PackageManagerPrincipalID || p.PrincipalKind != contracts.PackageManagerPrincipalKind || p.TargetKind != contracts.PackageManagerPrincipalKind || p.TargetIdentity != contracts.PackageManagerPrincipalID || p.AuthorityModelVersion != contracts.AuthorityModelDeploymentVersion || p.AuthorityModelDigest != contracts.AuthorityModelDeploymentDigest() {
		return errors.New("proposal is not the closed v3 package-deploy profile")
	}
	model, err := repo.LoadAuthorityModelState(ctx, now)
	if err != nil || !contracts.AuthorityModelStateRetains(model, contracts.AuthorityModelDeploymentVersion) {
		return errors.New("package-deploy proposal requires an adopted authority model that retains v3 deployment semantics")
	}
	bootstrapDigest := p.BootstrapDigest
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil || p.OwnerID != owner.ID || p.OwnerKind != owner.Kind {
		return errors.New("proposal owner does not bind installation")
	}
	root, err := currentInstallationRoot(ctx, repo, owner, now)
	if err != nil || root.Ref != p.ParentRef || root.Version != p.ParentVersion || root.Digest != p.ParentDigest {
		return errors.New("proposal root is not current")
	}
	scope, err := contracts.PackageDeploymentScope(root.Digest)
	if err != nil || p.Scope != scope || p.TargetDigest != root.Digest {
		return errors.New("proposal scope does not bind exact installation root")
	}
	return contracts.ValidateBuiltinPackageDeployDelegation(root, contracts.DelegationRequest{Profile: p.Profile, ParentRef: p.ParentRef, ParentVersion: p.ParentVersion, ParentDigest: p.ParentDigest, DelegatedPrincipal: contracts.PackageManagerPrincipal(), TargetKind: p.TargetKind, TargetIdentity: p.TargetIdentity, TargetVersion: p.TargetVersion, TargetDigest: p.TargetDigest, TargetConstraints: []string{p.TargetDigest}, RequestedCapabilities: []string{}, RequestedOperations: []string{}, RequestedAuthority: p.Capability, RequestedOperation: "deploy", RequestedScope: p.Scope, ProposalVersion: p.Version, ProposalDigest: "sha256:" + strings.Repeat("0", 64), ReviewVersion: "1", ReviewDigest: "sha256:" + strings.Repeat("0", 64), ExpiresAt: p.ExpiresAt, Reason: p.Reason, PolicyRef: p.AuthorityModel, PolicyVersion: p.AuthorityModelVersion, PolicyDigest: p.AuthorityModelDigest, SubjectKind: p.PrincipalKind, SubjectID: p.PrincipalID, SubjectVersion: p.TargetVersion, SubjectDigest: p.TargetDigest}, now)
}

func runPackageManagerAuthorityPreview(args []string, out io.Writer) error {
	f := flag.NewFlagSet("authority package-deploy-preview", flag.ContinueOnError)
	expiresAt := f.String("expires-at", "", "owner-selected RFC3339 expiry")
	output := f.String("output", "", "optional proposal preview JSON path")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *expiresAt == "" {
		return errors.New("usage: praxis authority package-deploy-preview --expires-at <RFC3339> [--output <file>]")
	}
	expires, err := time.Parse(time.RFC3339Nano, *expiresAt)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if !expires.After(now) {
		return errors.New("expiry must be in the future")
	}
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, now)
	if err != nil {
		return err
	}
	scope, err := contracts.PackageDeploymentScope(root.Digest)
	if err != nil {
		return err
	}
	model, err := repo.LoadAuthorityModelState(context.Background(), now)
	if err != nil || !contracts.AuthorityModelStateRetains(model, contracts.AuthorityModelDeploymentVersion) {
		return errors.New("package-deploy proposal requires an adopted authority model that retains v3 deployment semantics")
	}
	p := contracts.GovernedAuthorityProposal{ID: "package-manager-authority-proposal:" + root.Digest + ":" + strconv.FormatInt(now.UnixNano(), 10), Version: "1", Kind: contracts.GovernedAuthorityProposalKind, BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelDeploymentVersion, AuthorityModelDigest: contracts.AuthorityModelDeploymentDigest(), Profile: contracts.DelegationProfilePackageDeploy, Capability: contracts.GovernedPackageDeploy, PrincipalID: contracts.PackageManagerPrincipalID, PrincipalKind: contracts.PackageManagerPrincipalKind, TargetKind: contracts.PackageManagerPrincipalKind, TargetIdentity: contracts.PackageManagerPrincipalID, TargetVersion: "1", TargetDigest: root.Digest, Scope: scope, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, Reason: "governed installation-local package deployment", ExpiresAt: expires.UTC(), CreatedAt: now}
	digest, err := p.Digest()
	if err != nil {
		return err
	}
	result := map[string]any{"operation": "authority.package-deploy-proposal", "preview": true, "proposal": p, "proposal_digest": digest}
	if *output != "" {
		b, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		if err := writeCanonicalPreviewFile(*output, append(b, '\n')); err != nil {
			return err
		}
	}
	return printJSONTo(out, result)
}

func runPackageManagerAuthorityProposal(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority package-deploy-proposal", flag.ContinueOnError)
	file := f.String("preview-file", "", "system-produced proposal preview JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *file == "" {
		return errors.New("usage: praxis authority package-deploy-proposal --preview-file <file>")
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var e struct {
		Proposal contracts.GovernedAuthorityProposal `json:"proposal"`
		Digest   string                              `json:"proposal_digest"`
	}
	if err := json.Unmarshal(b, &e); err != nil {
		return err
	}
	d, err := e.Proposal.Digest()
	if err != nil || d != e.Digest {
		return errors.New("proposal preview digest mismatch")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	if err := packageManagerProposalCurrent(context.Background(), repo, e.Proposal, now); err != nil {
		return err
	}
	stored, err := repo.SaveGovernedAuthorityProposal(context.Background(), e.Proposal, now)
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.package-deploy-proposal", "proposal_digest": stored})
}

func runPackageManagerAuthorityReview(args []string, out io.Writer) error {
	f := flag.NewFlagSet("authority package-deploy-review", flag.ContinueOnError)
	digest := f.String("proposal-digest", "", "exact durable proposal digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *digest == "" || !isInteractiveTerminal() {
		return errAuthorityBootstrapConfirmation
	}
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	p, err := repo.LoadGovernedAuthorityProposalByDigest(context.Background(), *digest, now)
	if err != nil {
		return err
	}
	if err := packageManagerProposalCurrent(context.Background(), repo, p, now); err != nil {
		return err
	}
	bootstrapDigest, err := record.Digest()
	if err != nil || p.BootstrapDigest != bootstrapDigest {
		return errors.New("proposal belongs to another installation")
	}
	owner, _ := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return errors.New("authenticated reviewer unavailable")
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, now)
	if err != nil || root.Ref != p.ParentRef || root.Digest != p.ParentDigest || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+current.Username) {
		return errors.New("authenticated reviewer is not installation owner")
	}
	fmt.Fprintf(out, "Review exact package-deploy proposal %s. Type %q to approve: ", *digest, "REVIEW-PACKAGE-DEPLOY "+*digest)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "REVIEW-PACKAGE-DEPLOY "+*digest {
		return errAuthorityBootstrapConfirmation
	}
	repo2, db2, err := openGovernedRepository(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db2.Close()
	review := contracts.GovernedAuthorityReview{ID: "package-manager-authority-review:" + *digest, Version: "1", Kind: contracts.GovernedAuthorityReviewKind, ProposalID: p.ID, ProposalVersion: p.Version, ProposalDigest: *digest, ReviewedBy: owner.ID, ReviewedKind: owner.Kind, Decision: "approve", ReviewedAt: now}
	rd, err := repo2.SaveGovernedAuthorityReview(context.Background(), review, now)
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.package-deploy-review", "review_digest": rd, "proposal_digest": *digest})
}

func runPackageManagerAuthorityRequest(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority package-deploy-request", flag.ContinueOnError)
	pd := f.String("proposal-digest", "", "exact durable proposal digest")
	rd := f.String("review-digest", "", "exact durable review digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *pd == "" || *rd == "" {
		return errors.New("usage: praxis authority package-deploy-request --proposal-digest <digest> --review-digest <digest>")
	}
	repo, db, _, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	p, err := repo.LoadGovernedAuthorityProposalByDigest(context.Background(), *pd, now)
	if err != nil {
		return err
	}
	review, err := repo.LoadGovernedAuthorityReviewByDigest(context.Background(), *rd, now)
	if err != nil {
		return err
	}
	if review.ProposalID != p.ID || review.ProposalVersion != p.Version || review.ProposalDigest != *pd {
		return errors.New("review does not bind exact proposal")
	}
	if err := packageManagerProposalCurrent(context.Background(), repo, p, now); err != nil {
		return err
	}
	delegation := contracts.DelegationRequest{Profile: p.Profile, ParentRef: p.ParentRef, ParentVersion: p.ParentVersion, ParentDigest: p.ParentDigest, DelegatedPrincipal: contracts.PackageManagerPrincipal(), TargetKind: p.TargetKind, TargetIdentity: p.TargetIdentity, TargetVersion: p.TargetVersion, TargetDigest: p.TargetDigest, TargetConstraints: []string{p.TargetDigest}, RequestedCapabilities: []string{}, RequestedOperations: []string{}, RequestedAuthority: p.Capability, RequestedOperation: "deploy", RequestedScope: p.Scope, ProposalVersion: p.Version, ProposalDigest: *pd, ReviewVersion: review.Version, ReviewDigest: *rd, ExpiresAt: p.ExpiresAt, Reason: p.Reason, PolicyRef: p.AuthorityModel, PolicyVersion: p.AuthorityModelVersion, PolicyDigest: p.AuthorityModelDigest, SubjectKind: p.PrincipalKind, SubjectID: p.PrincipalID, SubjectVersion: p.TargetVersion, SubjectDigest: p.TargetDigest}
	request := contracts.AuthorityRequest{ID: "package-manager-authority-request:" + *pd + ":" + *rd, Version: "1", RequestedAuthority: contracts.GovernedPackageDeploy, RequestedScope: p.Scope, Reason: p.Reason, Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	digest, err := request.Digest()
	if err != nil {
		return err
	}
	writable, db2, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db2.Close()
	stored, err := writable.SaveAuthorityRequest(context.Background(), request, now, &p.ExpiresAt)
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.package-deploy-request", "request": request, "request_digest": stored, "request_digest_check": digest})
}

func runPackageManagerDeploymentApprove(args []string, getenv func(string) string, input io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("authority package-deploy-approve", flag.ContinueOnError)
	digest := f.String("request", "", "exact durable package-deploy request digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *digest == "" || !isInteractiveTerminal() {
		return errors.New("usage: praxis authority package-deploy-approve --request <digest> (interactive confirmation required)")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	record, err := praxiscrypto.LoadBootstrapRecord(getenv("PRAXIS_BOOTSTRAP_RECORD"))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	request, err := repo.LoadAuthorityRequestByDigest(context.Background(), *digest, now)
	if err != nil {
		return err
	}
	if request.RequestedAuthority != contracts.GovernedPackageDeploy || request.Delegation != nil || request.Intent == nil {
		return errors.New("request is not an exact package-deploy request")
	}
	manager, err := repo.ResolvePackageManagerAuthority(context.Background(), request.InstallationDigest, now)
	if err != nil {
		return err
	}
	root, err := repo.LoadAuthorityGeneration(context.Background(), manager.ParentRef, manager.ParentVersion, now)
	if err != nil || root.Digest != request.InstallationDigest {
		return errors.New("request installation root is not current")
	}
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil || root.Principal != owner {
		return errors.New("request decision authority is not the installation owner")
	}
	if err := repo.ValidateAuthorityGeneration(context.Background(), contracts.AuthorityDecision{AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, DecidedBy: owner, GrantedScope: root.Scope}, now); err != nil {
		return err
	}
	requestDigest, err := request.Digest()
	if err != nil {
		return err
	}
	intentDigest, err := request.Intent.Digest()
	if err != nil {
		return err
	}
	evidence, err := state.New(db).LoadVerificationEvidence(context.Background(), request.VerificationEvidenceDigest)
	if err != nil || evidence.InstallationDigest != request.InstallationDigest || evidence.ClosureDigest != request.ClosureDigest {
		return errors.New("request verification evidence is unavailable or changed")
	}
	payload, _ := json.Marshal(map[string]any{"request": request, "request_digest": requestDigest, "intent": request.Intent, "intent_digest": intentDigest, "verification_evidence": evidence, "operational_authority": manager, "decision_authority": root})
	if _, err := fmt.Fprintf(out, "Authorize this exact package deployment:\n%s\nType %q to continue: ", payload, "APPROVE-PACKAGE-DEPLOY "+requestDigest); err != nil {
		return err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "APPROVE-PACKAGE-DEPLOY "+requestDigest {
		return errAuthorityBootstrapConfirmation
	}
	expires := now.Add(24 * time.Hour)
	if manager.ExpiresAt != nil && manager.ExpiresAt.Before(expires) {
		expires = manager.ExpiresAt.UTC()
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: owner, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: root.AuthorityModelDigest, IssuedAt: now, ExpiresAt: &expires, OperationalAuthorityRef: manager.Ref, OperationalAuthorityVersion: manager.Version, OperationalAuthorityGenerationDigest: manager.Digest}
	if err := repo.SaveAuthorityDecision(context.Background(), request.ID, request.Version, decision, now, &expires); err != nil {
		return err
	}
	approvalID, err := repo.DerivePackageDeploymentApproval(context.Background(), request.ID, request.Version, *request.Intent, now)
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.package-deploy-approve", "request_digest": requestDigest, "decision": decision, "approval_id": approvalID})
}

// parsePackageDeployRef routes one package-deploy-intent-preview argument to
// its exact transport. "local:<package-id>@<version>" selects the
// local-first-party adapter, rooted at the required PRAXIS_LOCAL_PACKAGES_DIR;
// anything else is parsed exactly as before, unchanged, as a GitHub
// owner/repo[@tag] reference. An argument that matches neither recognized
// form fails closed through parseGitHubPackageRef's own format error; there
// is no silent default transport.
// containsPathTraversalSegment reports whether any "/"-delimited segment of
// s is exactly "..". It does not reject "." or an empty segment, and it does
// not reject "/" itself -- only the traversal shape.
func containsPathTraversalSegment(s string) bool {
	for _, segment := range strings.Split(s, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func parsePackageDeployRef(raw string, getenv func(string) string) (distribution.PackageRef, string, distribution.RootAdapter, error) {
	if strings.HasPrefix(raw, "local:") {
		spec := strings.TrimPrefix(raw, "local:")
		at := strings.LastIndex(spec, "@")
		if at <= 0 || at == len(spec)-1 {
			return distribution.PackageRef{}, "", nil, fmt.Errorf("local package reference must be local:<package-id>@<version>, got %q", raw)
		}
		packageID, version := spec[:at], spec[at+1:]
		// Defense in depth alongside distribution.LocalFirstParty's own path
		// containment check: neither a package id nor a version may contain a
		// ".." path segment. A legitimate package id may still contain "/"
		// (this codebase's own package ids do, e.g. "shared/graph"), so this
		// only rejects the traversal shape, not slashes generally.
		if containsPathTraversalSegment(packageID) || containsPathTraversalSegment(version) {
			return distribution.PackageRef{}, "", nil, fmt.Errorf("local package reference must not contain a path-traversal segment, got %q", raw)
		}
		root := getenv("PRAXIS_LOCAL_PACKAGES_DIR")
		if root == "" {
			return distribution.PackageRef{}, "", nil, errors.New("PRAXIS_LOCAL_PACKAGES_DIR is required for a local-first-party package reference")
		}
		ref := distribution.PackageRef{Source: distribution.SourceLocalFirstParty, Owner: "local", Repo: packageID}
		return ref, version, distribution.LocalFirstParty{Root: root}, nil
	}
	ref, version, err := parseGitHubPackageRef(raw)
	if err != nil {
		return distribution.PackageRef{}, "", nil, err
	}
	return ref, version, distribution.GitHubReleases{Token: getenv("GITHUB_TOKEN")}, nil
}

func runPackageDeploymentIntentPreview(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority package-deploy-intent-preview", flag.ContinueOnError)
	output := f.String("output", "", "optional system-produced intent preview JSON path")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return errors.New("usage: praxis authority package-deploy-intent-preview <owner/repo[@tag]|local:<package-id>@<version>> [--output <file>]")
	}
	ref, version, adapter, err := parsePackageDeployRef(f.Arg(0), getenv)
	if err != nil {
		return err
	}
	release, err := adapter.Resolve(context.Background(), ref, version)
	if err != nil {
		return err
	}
	artifact, err := adapter.FetchArtifact(context.Background(), release)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	resolution, err := resolveReleasePackages(context.Background(), adapter, release, artifact, getenv, false, now)
	if err != nil {
		return err
	}
	deployment, err := packagecatalog.NewDeploymentRequest(resolution.Root, resolution.Dependencies, contracts.PackageManagerPrincipal(), "preview")
	if err != nil {
		return err
	}
	db, err := openPackageDB(context.Background())
	if err != nil {
		return err
	}
	defer db.Close()
	repo, authDB, record, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer authDB.Close()
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, now)
	if err != nil {
		return err
	}
	evidence, err := packagecatalog.NewVerificationEvidenceRecord(deployment.Root, deployment.Packages, root.Digest, now)
	if err != nil {
		return err
	}
	if err := state.New(db).SaveVerificationEvidence(context.Background(), evidence); err != nil {
		return err
	}
	deployment.VerificationEvidenceDigest = evidence.ID
	deployment.Intent.Parameters["verification_evidence_digest"] = evidence.ID
	if err := deployment.Validate(); err != nil {
		return err
	}
	intentDigest, err := deployment.Intent.Digest()
	if err != nil {
		return err
	}
	result := map[string]any{"operation": "authority.package-deploy-intent", "preview": true, "intent": deployment.Intent, "intent_digest": intentDigest, "installation_digest": root.Digest, "closure_digest": deployment.Intent.Parameters["closure_digest"], "verification_evidence_digest": evidence.ID, "resolution": resolution.Order}
	if *output != "" {
		b, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		if err := writeCanonicalPreviewFile(*output, append(b, '\n')); err != nil {
			return err
		}
	}
	return printJSONTo(out, result)
}

func runPackageDeploymentIntentRequest(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority package-deploy-intent-request", flag.ContinueOnError)
	file := f.String("preview-file", "", "system-produced exact intent preview JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *file == "" {
		return errors.New("usage: praxis authority package-deploy-intent-request --preview-file <file>")
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var e struct {
		Intent                     contracts.ActionIntent `json:"intent"`
		IntentDigest               string                 `json:"intent_digest"`
		InstallationDigest         string                 `json:"installation_digest"`
		ClosureDigest              string                 `json:"closure_digest"`
		VerificationEvidenceDigest string                 `json:"verification_evidence_digest"`
	}
	if err := json.Unmarshal(b, &e); err != nil {
		return err
	}
	d, err := e.Intent.Digest()
	if err != nil || d != e.IntentDigest || e.Intent.Parameters["verification_evidence_digest"] != e.VerificationEvidenceDigest || e.Intent.Parameters["closure_digest"] != e.ClosureDigest {
		return errors.New("intent preview digest or evidence binding mismatch")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	request, digest, err := repo.SavePackageDeploymentRequest(context.Background(), e.Intent, e.InstallationDigest, e.ClosureDigest, e.VerificationEvidenceDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.package-deploy-intent-request", "request": request, "request_digest": digest})
}
