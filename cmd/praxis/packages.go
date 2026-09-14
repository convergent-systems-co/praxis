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
		ref, version, err := parseGitHubPackageRef(refArg)
		if err != nil {
			return err
		}
		release, err := adapter.Resolve(ctx, ref, version)
		if err != nil {
			return err
		}
		artifact, err := adapter.FetchArtifact(ctx, release)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		verified, err := verifyReleasePackage(release, artifact, os.Getenv, allowFallback, now)
		if err != nil {
			return err
		}
		activation, err := packageActivationRequest(verified, os.Getenv)
		if err != nil {
			return err
		}
		db, err := openPackageDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := state.New(db).ActivatePackage(ctx, activation, now); err != nil {
			return err
		}
		return printJSON(map[string]any{"installed": release.Manifest.PackageID, "version": release.Manifest.Version, "digest": release.Manifest.ContentDigest, "signature_keys": signatureKeyIDs(release.Signature), "signature_profile": release.Signature.Profile, "entry_points": release.Manifest.Invocations, "contents": release.Manifest.Contents})
	case "update":
		packageID, acceptChanges, allowFallback, err := parseUpdateArgs(args)
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
		ref, _, err := parseGitHubPackageRef(installed.SourceRef)
		if err != nil {
			return fmt.Errorf("installed source: %w", err)
		}
		latest, changed, err := adapter.CheckUpdate(ctx, ref, installed.Manifest.Version)
		if err != nil {
			return err
		}
		if !changed {
			return printJSON(map[string]any{"package_id": packageID, "up_to_date": true, "version": installed.Manifest.Version})
		}
		review := packagecatalog.ReviewUpdate(installed.Manifest.Capabilities, latest.Manifest.Capabilities, !sameStrings(installed.Manifest.RequiredEnforcement, latest.Manifest.RequiredEnforcement), installed.Manifest.CryptoProfile != latest.Manifest.CryptoProfile)
		if review.RequiresReauthorization && !acceptChanges {
			return fmt.Errorf("update requires explicit review/reauthorization: added_capabilities=%v enforcement_changed=%v crypto_changed=%v; rerun with --accept-permission-changes after review", review.AddedCapabilities, review.EnforcementChanged, review.CryptoProfileChanged)
		}
		artifact, err := adapter.FetchArtifact(ctx, latest)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		verified, err := verifyReleasePackage(latest, artifact, os.Getenv, allowFallback, now)
		if err != nil {
			return err
		}
		activation, err := packageActivationRequest(verified, os.Getenv)
		if err != nil {
			return err
		}
		if err := store.ActivatePackage(ctx, activation, now); err != nil {
			return err
		}
		return printJSON(map[string]any{"updated": latest.Manifest.PackageID, "from": installed.Manifest.Version, "to": latest.Manifest.Version, "signature_keys": signatureKeyIDs(latest.Signature), "signature_profile": latest.Signature.Profile, "review": review})
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
		installed, err := store.ActivePackage(ctx, args[0])
		if err != nil {
			return fmt.Errorf("active package %q: %w", args[0], err)
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
		return "", false, errors.New("usage: praxis install <owner/repo[@tag]> [--allow-classical-signature-fallback]")
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

func parseUpdateArgs(args []string) (string, bool, bool, error) {
	if len(args) < 1 || len(args) > 3 {
		return "", false, false, errors.New("usage: praxis update <package-id> [--accept-permission-changes] [--allow-classical-signature-fallback]")
	}
	accept, fallback := false, false
	for _, arg := range args[1:] {
		switch arg {
		case "--accept-permission-changes":
			accept = true
		case "--allow-classical-signature-fallback":
			fallback = true
		default:
			return "", false, false, fmt.Errorf("unknown update option %q", arg)
		}
	}
	return args[0], accept, fallback, nil
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
	return distribution.PackageRef{Source: "github-releases", Owner: parts[0], Repo: parts[1]}, version, nil
}

func verifyReleasePackage(release distribution.Release, artifact []byte, getenv func(string) string, allowPQPreferredFallback bool, at time.Time) (packagecatalog.VerifiedPackage, error) {
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
