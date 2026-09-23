package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func runPackageCommand(command string, args []string) error {
	ctx := context.Background()
	adapter := distribution.GitHubReleases{Token: os.Getenv("GITHUB_TOKEN")}
	switch command {
	case "discover":
		query := strings.Join(args, " ")
		items, err := adapter.Discover(ctx, query)
		if err != nil {
			return err
		}
		return printJSON(items)
	case "info":
		if len(args) != 1 {
			return errors.New("usage: praxis info <owner/repo[@tag]>")
		}
		ref, version, err := parseGitHubPackageRef(args[0])
		if err != nil {
			return err
		}
		release, err := adapter.Info(ctx, ref, version)
		if err != nil {
			return err
		}
		return printJSON(release)
	case "list":
		db, err := openPackageDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()
		items, err := state.New(db).InstalledPackages(ctx)
		if err != nil {
			return err
		}
		if items == nil {
			items = []state.InstalledPackage{}
		}
		return printJSON(items)
	case "install":
		refArg, allowFallback, err := parseInstallArgs(args)
		if err != nil {
			return err
		}
		ref, version, source, err := parsePackageDeployRef(refArg, os.Getenv)
		if err != nil {
			return err
		}
		release, err := source.Resolve(ctx, ref, version)
		if err != nil {
			return err
		}
		artifact, err := source.FetchArtifact(ctx, release)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		db, err := openPackageDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()
		at, err := approvedVerificationTime(ctx, db, os.Getenv, now)
		if err != nil {
			return err
		}
		resolution, err := resolveReleasePackages(ctx, source, release, artifact, os.Getenv, allowFallback, at)
		if err != nil {
			return err
		}
		deployment, err := packageDeploymentRequest(resolution, os.Getenv)
		if err != nil {
			return err
		}
		evidenceDigest, err := persistDeploymentEvidence(ctx, db, deployment, at, os.Getenv)
		if err != nil {
			return err
		}
		deployment.VerificationEvidenceDigest = evidenceDigest
		deployment.Intent.Parameters["verification_evidence_digest"] = evidenceDigest
		if err := revalidateDeploymentApproval(ctx, db, deployment, now, os.Getenv); err != nil {
			return err
		}
		if err := state.New(db).DeployPackages(ctx, deployment, now); err != nil {
			return err
		}
		return printJSON(map[string]any{"installed": release.Manifest.PackageID, "version": release.Manifest.Version, "digest": release.Manifest.ContentDigest, "signature_keys": signatureKeyIDs(release.Signature), "signature_profile": release.Signature.Profile, "dependency_resolution": resolution.Order, "entry_points": release.Manifest.Invocations, "contents": release.Manifest.Contents})
	case "update":
		packageID, targetRef, acceptChanges, allowFallback, err := parseUpdateArgs(args)
		if err != nil {
			return err
		}
		db, err := openPackageDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()
		store := state.New(db)
		installed, err := store.ActivePackage(ctx, packageID)
		if err != nil {
			return fmt.Errorf("active package %q: %w", packageID, err)
		}
		var latest distribution.Release
		var changed bool
		var source distribution.RootAdapter
		switch installed.SourceKind {
		case distribution.SourceLocalFirstParty:
			if targetRef == "" {
				return errors.New("local package update requires --to local:<package-id>@<version>; local sources have no moving latest")
			}
			ref, version, selected, err := parsePackageDeployRef(targetRef, os.Getenv)
			if err != nil {
				return err
			}
			if ref.Source != distribution.SourceLocalFirstParty || ref.Repo != packageID || version == "" {
				return errors.New("local update target must pin the same package id and an explicit version")
			}
			if version == installed.Manifest.Version {
				return errors.New("local update target must name a different version; use rollback for an earlier generation")
			}
			historical, err := store.PackageVersionWasInstalled(ctx, packageID, version)
			if err != nil {
				return err
			}
			if historical {
				return errors.New("local update target is a previously installed version; use rollback for an earlier generation")
			}
			source = selected
			latest, err = source.Resolve(ctx, ref, version)
			if err != nil {
				return err
			}
			changed = true
		case distribution.SourceGitHubReleases:
			if targetRef != "" {
				return errors.New("--to is only supported for an installed local-first-party package")
			}
			ref, _, selected, err := parsePackageDeployRef(installed.SourceRef, os.Getenv)
			if err != nil {
				return fmt.Errorf("installed source: %w", err)
			}
			if ref.Source != distribution.SourceGitHubReleases {
				return errors.New("installed source is not a GitHub package reference")
			}
			source = selected
			latest, changed, err = source.CheckUpdate(ctx, ref, installed.Manifest.Version)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported installed package source %q", installed.SourceKind)
		}
		if !changed {
			return printJSON(map[string]any{"package_id": packageID, "up_to_date": true, "version": installed.Manifest.Version})
		}
		review := packagecatalog.ReviewUpdate(installed.Manifest.Capabilities, latest.Manifest.Capabilities, !sameStrings(installed.Manifest.RequiredEnforcement, latest.Manifest.RequiredEnforcement), installed.Manifest.CryptoProfile != latest.Manifest.CryptoProfile)
		if review.RequiresReauthorization && !acceptChanges {
			return fmt.Errorf("update requires explicit review/reauthorization: added_capabilities=%v enforcement_changed=%v crypto_changed=%v; rerun with --accept-permission-changes after review", review.AddedCapabilities, review.EnforcementChanged, review.CryptoProfileChanged)
		}
		artifact, err := source.FetchArtifact(ctx, latest)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		at, err := approvedVerificationTime(ctx, db, os.Getenv, now)
		if err != nil {
			return err
		}
		resolution, err := resolveReleasePackages(ctx, source, latest, artifact, os.Getenv, allowFallback, at)
		if err != nil {
			return err
		}
		deployment, err := packageDeploymentRequest(resolution, os.Getenv)
		if err != nil {
			return err
		}
		evidenceDigest, err := persistDeploymentEvidence(ctx, db, deployment, at, os.Getenv)
		if err != nil {
			return err
		}
		deployment.VerificationEvidenceDigest = evidenceDigest
		deployment.Intent.Parameters["verification_evidence_digest"] = evidenceDigest
		if err := revalidateDeploymentApproval(ctx, db, deployment, now, os.Getenv); err != nil {
			return err
		}
		if err := store.DeployPackages(ctx, deployment, now); err != nil {
			return err
		}
		return printJSON(map[string]any{"updated": latest.Manifest.PackageID, "from": installed.Manifest.Version, "to": latest.Manifest.Version, "signature_keys": signatureKeyIDs(latest.Signature), "signature_profile": latest.Signature.Profile, "dependency_resolution": resolution.Order, "review": review})
	case "rollback":
		if len(args) != 1 {
			return errors.New("usage: praxis rollback <package-id>")
		}
		db, err := openPackageDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()
		approvalID, actorID, actorKind := os.Getenv("PRAXIS_PACKAGE_APPROVAL_ID"), os.Getenv("PRAXIS_AUTHORITY_ID"), os.Getenv("PRAXIS_AUTHORITY_KIND")
		if approvalID == "" || actorID == "" || actorKind == "" {
			return errors.New("PRAXIS_PACKAGE_APPROVAL_ID, PRAXIS_AUTHORITY_ID, and PRAXIS_AUTHORITY_KIND are required for package rollback")
		}
		store := state.New(db)
		request, err := store.PreparePackageRollback(ctx, args[0], approvalID, contracts.PrincipalRef{ID: actorID, Kind: actorKind})
		if err != nil {
			return err
		}
		if err := store.RollbackPackage(ctx, request, time.Now().UTC()); err != nil {
			return err
		}
		return printJSON(map[string]any{"package_id": args[0], "state": "rolled_back", "target": request.Targets[len(request.Targets)-1], "closure": request.Targets})
	case "disable", "uninstall":
		if len(args) != 1 {
			return fmt.Errorf("usage: praxis %s <package-id>", command)
		}
		db, err := openPackageDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()
		store := state.New(db)
		installed, err := store.SelectedPackage(ctx, args[0])
		if err != nil {
			return fmt.Errorf("selected package %q: %w", args[0], err)
		}
		operation := packagecatalog.TransitionDisable
		if command == "uninstall" {
			operation = packagecatalog.TransitionRemove
		}
		transition, err := packageTransitionRequest(installed.Manifest, operation, os.Getenv)
		if err != nil {
			return err
		}
		if err := store.TransitionPackage(ctx, transition, time.Now().UTC()); err != nil {
			return err
		}
		return printJSON(map[string]any{"package_id": args[0], "state": strings.TrimPrefix(string(operation), "package."), "durable_history_preserved": true})
	default:
		return fmt.Errorf("unknown package command %q", command)
	}
}

func parseInstallArgs(args []string) (string, bool, error) {
	if len(args) < 1 || len(args) > 2 {
		return "", false, errors.New("usage: praxis install <owner/repo[@tag]|local:<package-id>@<version>> [--allow-classical-signature-fallback]")
	}
	allow := false
	if len(args) == 2 {
		if args[1] != "--allow-classical-signature-fallback" {
			return "", false, fmt.Errorf("unknown install option %q", args[1])
		}
		allow = true
	}
	return args[0], allow, nil
}

func parseUpdateArgs(args []string) (string, string, bool, bool, error) {
	if len(args) < 1 || len(args) > 5 {
		return "", "", false, false, errors.New("usage: praxis update <package-id> [--to local:<package-id>@<version>] [--accept-permission-changes] [--allow-classical-signature-fallback]")
	}
	accept, fallback, target := false, false, ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--to":
			if target != "" || i+1 >= len(args) || args[i+1] == "" {
				return "", "", false, false, errors.New("update --to requires one exact package reference")
			}
			i++
			target = args[i]
		case "--accept-permission-changes":
			accept = true
		case "--allow-classical-signature-fallback":
			fallback = true
		default:
			return "", "", false, false, fmt.Errorf("unknown update option %q", arg)
		}
	}
	return args[0], target, accept, fallback, nil
}

func openPackageDB(ctx context.Context) (*sql.DB, error) {
	path := os.Getenv("PRAXIS_DB")
	if path == "" {
		return nil, errors.New("PRAXIS_DB is required for package lifecycle commands")
	}
	return state.OpenSQLite(ctx, path)
}

func parseGitHubPackageRef(raw string) (distribution.PackageRef, string, error) {
	if raw == "" {
		return distribution.PackageRef{}, "", errors.New("package reference is required")
	}
	base, version := raw, ""
	if at := strings.LastIndex(raw, "@"); at > 0 {
		base, version = raw[:at], raw[at+1:]
	}
	parts := strings.Split(base, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return distribution.PackageRef{}, "", fmt.Errorf("GitHub package reference must be owner/repo[@tag], got %q", raw)
	}
	return distribution.PackageRef{Source: distribution.SourceGitHubReleases, Owner: parts[0], Repo: parts[1]}, version, nil
}

func verifyReleasePackage(release distribution.Release, artifact []byte, getenv func(string) string, allowPQPreferredFallback bool, at time.Time) (packagecatalog.VerifiedPackage, error) {
	if err := checkGoalsPublicationAcquisition(context.Background(), release, artifact, getenv); err != nil {
		return packagecatalog.VerifiedPackage{}, err
	}
	if len(artifact) == 0 {
		return packagecatalog.VerifiedPackage{}, errors.New("empty release artifact")
	}
	if len(release.ManifestBytes) == 0 {
		return packagecatalog.VerifiedPackage{}, errors.New("immutable downloaded manifest bytes are required")
	}
	keys, err := loadTrustedPublisherKeys(getenv)
	if err != nil {
		return packagecatalog.VerifiedPackage{}, err
	}
	verifiers := []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: keys}}
	sourceRef := release.Ref.String() + "@" + release.Tag
	if sourceRef == "@" {
		sourceRef = "resolved-release"
	}
	verified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: release.ManifestBytes, ArtifactBytes: artifact, Signature: release.Signature, SourceKind: "github-release", SourceRef: sourceRef, VerifiedAt: at, AllowPQFallback: allowPQPreferredFallback}, verifiers)
	if err != nil {
		return packagecatalog.VerifiedPackage{}, fmt.Errorf("verify package: %w", err)
	}
	return verified, nil
}

// resolveReleasePackages verifies one root release and its locked
// dependency closure. adapter is whichever transport actually resolved
// release; it is registered under release.Ref.Source, the same source
// string that release's dependency locks (if any) must themselves carry to
// be resolvable. This keeps transport selection purely a routing decision:
// packagecatalog.VerifyPackage runs identically regardless of which
// distribution.RootAdapter supplied the bytes.
func resolveReleasePackages(ctx context.Context, adapter distribution.RootAdapter, release distribution.Release, artifact []byte, getenv func(string) string, allowPQPreferredFallback bool, at time.Time) (distribution.Resolution, error) {
	if err := checkGoalsPublicationAcquisition(ctx, release, artifact, getenv); err != nil {
		return distribution.Resolution{}, err
	}
	if len(artifact) == 0 || len(release.ManifestBytes) == 0 {
		return distribution.Resolution{}, errors.New("immutable downloaded manifest and artifact bytes are required")
	}
	if release.Ref.Source == "" {
		return distribution.Resolution{}, errors.New("release has no distribution source")
	}
	keys, err := loadTrustedPublisherKeys(getenv)
	if err != nil {
		return distribution.Resolution{}, err
	}
	resolver := distribution.Resolver{
		Sources:         map[string]distribution.LockedAdapter{release.Ref.Source: adapter},
		Verifiers:       []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: keys}},
		AllowPQFallback: allowPQPreferredFallback,
		VerifiedAt:      at,
	}
	resolution, err := resolver.Resolve(ctx, release, artifact)
	if err != nil {
		return distribution.Resolution{}, fmt.Errorf("resolve package dependency graph: %w", err)
	}
	return resolution, nil
}

func packageActivationRequest(verified packagecatalog.VerifiedPackage, getenv func(string) string) (packagecatalog.ActivationRequest, error) {
	if getenv == nil {
		return packagecatalog.ActivationRequest{}, errors.New("package activation authority environment is required")
	}
	approvalID, actorID, actorKind := getenv("PRAXIS_PACKAGE_APPROVAL_ID"), getenv("PRAXIS_AUTHORITY_ID"), getenv("PRAXIS_AUTHORITY_KIND")
	if approvalID == "" || actorID == "" || actorKind == "" {
		return packagecatalog.ActivationRequest{}, errors.New("PRAXIS_PACKAGE_APPROVAL_ID, PRAXIS_AUTHORITY_ID, and PRAXIS_AUTHORITY_KIND are required; verification does not grant installation authority")
	}
	intent, err := packagecatalog.NewActivationIntent(verified, contracts.PrincipalRef{ID: actorID, Kind: actorKind})
	if err != nil {
		return packagecatalog.ActivationRequest{}, err
	}
	request := packagecatalog.ActivationRequest{Package: verified, Intent: intent, ApprovalID: approvalID}
	return request, request.Validate()
}

func packageDeploymentRequest(resolution distribution.Resolution, getenv func(string) string) (packagecatalog.DeploymentRequest, error) {
	if getenv == nil {
		return packagecatalog.DeploymentRequest{}, errors.New("package deployment authority environment is required")
	}
	approvalID := getenv("PRAXIS_PACKAGE_APPROVAL_ID")
	if approvalID == "" {
		return packagecatalog.DeploymentRequest{}, errors.New("PRAXIS_PACKAGE_APPROVAL_ID is required; package deployment authority is installation-bound and cannot be selected from the environment")
	}
	// The package manager is selected by installation governance state, never by
	// PRAXIS_AUTHORITY_ID/KIND. Those variables remain selectors for legacy
	// non-deployment transitions and cannot manufacture an installation actor.
	return packagecatalog.NewDeploymentRequest(resolution.Root, resolution.Dependencies, contracts.PackageManagerPrincipal(), approvalID)
}

// approvedVerificationTime resolves the verification instant bound by the
// governed deployment approval. Verification evidence identities include the
// verification time, so a deployment re-verifies the exact bytes as of the
// approved instant: identical bytes, keys, source, and installation reproduce
// the approved evidence identity, and any difference still fails closed when
// the approval is revalidated against the fresh closure.
func approvedVerificationTime(ctx context.Context, db *sql.DB, getenv func(string) string, now time.Time) (time.Time, error) {
	if getenv == nil {
		return time.Time{}, errors.New("package deployment authority environment is required")
	}
	approvalID := getenv("PRAXIS_PACKAGE_APPROVAL_ID")
	if approvalID == "" {
		return time.Time{}, errors.New("PRAXIS_PACKAGE_APPROVAL_ID is required; package deployment authority is installation-bound and cannot be selected from the environment")
	}
	store := state.New(db)
	requestID, requestVersion, err := store.PackageApprovalRequest(ctx, approvalID)
	if err != nil {
		return time.Time{}, err
	}
	repo, authDB, _, err := openGovernedRepositoryReadOnly(ctx, getenv)
	if err != nil {
		return time.Time{}, err
	}
	defer authDB.Close()
	request, err := repo.LoadAuthorityRequest(ctx, requestID, requestVersion, now)
	if err != nil {
		return time.Time{}, err
	}
	if request.RequestedAuthority != contracts.GovernedPackageDeploy || request.VerificationEvidenceDigest == "" {
		return time.Time{}, errors.New("package-deploy approval does not bind durable verification evidence")
	}
	evidence, err := store.LoadVerificationEvidence(ctx, request.VerificationEvidenceDigest)
	if err != nil {
		return time.Time{}, err
	}
	if evidence.VerifiedAt.IsZero() || evidence.VerifiedAt.After(now) {
		return time.Time{}, errors.New("approved verification evidence time is invalid")
	}
	return evidence.VerifiedAt.UTC(), nil
}

func persistDeploymentEvidence(ctx context.Context, db *sql.DB, deployment packagecatalog.DeploymentRequest, now time.Time, getenv func(string) string) (string, error) {
	repo, authDB, record, err := openGovernedRepositoryReadOnly(ctx, getenv)
	if err != nil {
		return "", err
	}
	defer authDB.Close()
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return "", err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return "", err
	}
	root, err := currentInstallationRoot(ctx, repo, owner, now)
	if err != nil {
		return "", err
	}
	evidence, err := packagecatalog.NewVerificationEvidenceRecord(deployment.Root, deployment.Packages, root.Digest, now)
	if err != nil {
		return "", err
	}
	if err := state.New(db).SaveVerificationEvidence(ctx, evidence); err != nil {
		return "", err
	}
	return evidence.ID, nil
}

func revalidateDeploymentApproval(ctx context.Context, db *sql.DB, deployment packagecatalog.DeploymentRequest, now time.Time, getenv func(string) string) error {
	requestID, requestVersion, err := state.New(db).PackageApprovalRequest(ctx, deployment.ApprovalID)
	if err != nil {
		return err
	}
	// Deriving the approval re-persists the canonical governed lineage
	// idempotently before comparing it, so revalidation needs the writable
	// governed repository even though it changes nothing on a match.
	repo, authDB, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer authDB.Close()
	derived, err := repo.DerivePackageDeploymentApproval(ctx, requestID, requestVersion, deployment.Intent, now)
	if err != nil {
		return err
	}
	if derived != deployment.ApprovalID {
		return errors.New("approval selector does not resolve to the exact governed deployment lineage")
	}
	return nil
}

func packageTransitionRequest(manifest packagecatalog.Manifest, operation packagecatalog.TransitionOperation, getenv func(string) string) (packagecatalog.TransitionRequest, error) {
	if getenv == nil {
		return packagecatalog.TransitionRequest{}, errors.New("package transition authority environment is required")
	}
	approvalID, actorID, actorKind := getenv("PRAXIS_PACKAGE_APPROVAL_ID"), getenv("PRAXIS_AUTHORITY_ID"), getenv("PRAXIS_AUTHORITY_KIND")
	if approvalID == "" || actorID == "" || actorKind == "" {
		return packagecatalog.TransitionRequest{}, errors.New("PRAXIS_PACKAGE_APPROVAL_ID, PRAXIS_AUTHORITY_ID, and PRAXIS_AUTHORITY_KIND are required for package transitions")
	}
	return packagecatalog.NewTransitionRequest(packagecatalog.PackageIdentity{PackageID: manifest.PackageID, Version: manifest.Version, ContentDigest: manifest.ContentDigest}, operation, contracts.PrincipalRef{ID: actorID, Kind: actorKind}, approvalID)
}

func signatureKeyIDs(envelope packagecatalog.SignatureEnvelope) []string {
	ids := make([]string, 0, len(envelope.Proofs))
	for _, proof := range envelope.Proofs {
		ids = append(ids, proof.KeyID)
	}
	return ids
}

func loadTrustedPublisherKeys(getenv func(string) string) (map[string]ed25519.PublicKey, error) {
	raw := ""
	if getenv != nil {
		raw = getenv("PRAXIS_TRUSTED_KEYS")
	}
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("PRAXIS_TRUSTED_KEYS is required for package install/update; value is JSON mapping key_id to base64 Ed25519 public key")
	}
	var encoded map[string]string
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil {
		return nil, fmt.Errorf("parse PRAXIS_TRUSTED_KEYS: %w", err)
	}
	keys := make(map[string]ed25519.PublicKey, len(encoded))
	for id, value := range encoded {
		if id == "" || value == "" {
			return nil, errors.New("trusted publisher key id and value are required")
		}
		body, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("decode trusted publisher key %q: %w", id, err)
		}
		if len(body) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("trusted publisher key %q is not an Ed25519 public key", id)
		}
		keys[id] = ed25519.PublicKey(body)
	}
	return keys, nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func printJSON(v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}
