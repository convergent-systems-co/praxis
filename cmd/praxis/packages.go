package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
)

func runPackageCommand(command string, args []string) error {
	ctx := context.Background()
	adapter := distribution.GitHubReleases{Token: os.Getenv("GITHUB_TOKEN")}
	switch command {
	case "discover":
		query := strings.Join(args, " ")
		items, err := adapter.Discover(ctx, query)
		if err != nil { return err }
		return printJSON(items)
	case "info":
		if len(args) != 1 { return errors.New("usage: praxis info <owner/repo[@tag]>") }
		ref, version, err := parseGitHubPackageRef(args[0]); if err != nil { return err }
		release, err := adapter.Info(ctx, ref, version); if err != nil { return err }
		return printJSON(release)
	case "list":
		db, err := openPackageDB(ctx); if err != nil { return err }; defer db.Close()
		items, err := state.New(db).InstalledPackages(ctx); if err != nil { return err }
		if items == nil { items = []state.InstalledPackage{} }
		return printJSON(items)
	case "install":
		refArg, allowFallback, err := parseInstallArgs(args)
		if err != nil { return err }
		ref, version, err := parseGitHubPackageRef(refArg); if err != nil { return err }
		release, err := adapter.Resolve(ctx, ref, version); if err != nil { return err }
		artifact, err := adapter.FetchArtifact(ctx, release); if err != nil { return err }
		if err := verifyReleasePackage(release, artifact, os.Getenv, allowFallback); err != nil { return err }
		db, err := openPackageDB(ctx); if err != nil { return err }; defer db.Close()
		if err := state.New(db).ActivatePackage(ctx, release.Manifest, "github-release", release.Ref.String()+"@"+release.Tag, time.Now().UTC()); err != nil { return err }
		return printJSON(map[string]any{"installed": release.Manifest.PackageID, "version": release.Manifest.Version, "digest": release.Manifest.ContentDigest, "signature_key": release.Signature.KeyID, "signature_profile": release.Signature.Profile, "entry_points": release.Manifest.Invocations, "contents": release.Manifest.Contents})
	case "update":
		packageID, acceptChanges, allowFallback, err := parseUpdateArgs(args)
		if err != nil { return err }
		db, err := openPackageDB(ctx); if err != nil { return err }; defer db.Close(); store := state.New(db)
		installed, err := store.ActivePackage(ctx, packageID); if err != nil { return fmt.Errorf("active package %q: %w", packageID, err) }
		ref, _, err := parseGitHubPackageRef(installed.SourceRef); if err != nil { return fmt.Errorf("installed source: %w", err) }
		latest, changed, err := adapter.CheckUpdate(ctx, ref, installed.Manifest.Version); if err != nil { return err }
		if !changed { return printJSON(map[string]any{"package_id": packageID, "up_to_date": true, "version": installed.Manifest.Version}) }
		review := packagecatalog.ReviewUpdate(installed.Manifest.Capabilities, latest.Manifest.Capabilities, !sameStrings(installed.Manifest.RequiredEnforcement, latest.Manifest.RequiredEnforcement), installed.Manifest.CryptoProfile != latest.Manifest.CryptoProfile)
		if review.RequiresReauthorization && !acceptChanges { return fmt.Errorf("update requires explicit review/reauthorization: added_capabilities=%v enforcement_changed=%v crypto_changed=%v; rerun with --accept-permission-changes after review", review.AddedCapabilities, review.EnforcementChanged, review.CryptoProfileChanged) }
		artifact, err := adapter.FetchArtifact(ctx, latest); if err != nil { return err }
		if err := verifyReleasePackage(latest, artifact, os.Getenv, allowFallback); err != nil { return err }
		if err := store.ActivatePackage(ctx, latest.Manifest, "github-release", latest.Ref.String()+"@"+latest.Tag, time.Now().UTC()); err != nil { return err }
		return printJSON(map[string]any{"updated": latest.Manifest.PackageID, "from": installed.Manifest.Version, "to": latest.Manifest.Version, "signature_key": latest.Signature.KeyID, "signature_profile": latest.Signature.Profile, "review": review})
	case "uninstall":
		if len(args) != 1 { return errors.New("usage: praxis uninstall <package-id>") }
		db, err := openPackageDB(ctx); if err != nil { return err }; defer db.Close()
		if err := state.New(db).RemovePackage(ctx, args[0]); err != nil { return err }
		return printJSON(map[string]any{"uninstalled": args[0], "durable_history_preserved": true})
	default:
		return fmt.Errorf("unknown package command %q", command)
	}
}

func parseInstallArgs(args []string) (string, bool, error) {
	if len(args) < 1 || len(args) > 2 { return "", false, errors.New("usage: praxis install <owner/repo[@tag]> [--allow-classical-signature-fallback]") }
	allow := false
	if len(args) == 2 {
		if args[1] != "--allow-classical-signature-fallback" { return "", false, fmt.Errorf("unknown install option %q", args[1]) }
		allow = true
	}
	return args[0], allow, nil
}

func parseUpdateArgs(args []string) (string, bool, bool, error) {
	if len(args) < 1 || len(args) > 3 { return "", false, false, errors.New("usage: praxis update <package-id> [--accept-permission-changes] [--allow-classical-signature-fallback]") }
	accept, fallback := false, false
	for _, arg := range args[1:] {
		switch arg {
		case "--accept-permission-changes": accept = true
		case "--allow-classical-signature-fallback": fallback = true
		default: return "", false, false, fmt.Errorf("unknown update option %q", arg)
		}
	}
	return args[0], accept, fallback, nil
}

func openPackageDB(ctx context.Context) (*sql.DB, error) {
	path := os.Getenv("PRAXIS_DB")
	if path == "" { return nil, errors.New("PRAXIS_DB is required for package lifecycle commands") }
	return state.OpenSQLite(ctx, path)
}

func parseGitHubPackageRef(raw string) (distribution.PackageRef, string, error) {
	if raw == "" { return distribution.PackageRef{}, "", errors.New("package reference is required") }
	base, version := raw, ""
	if at := strings.LastIndex(raw, "@"); at > 0 { base, version = raw[:at], raw[at+1:] }
	parts := strings.Split(base, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" { return distribution.PackageRef{}, "", fmt.Errorf("GitHub package reference must be owner/repo[@tag], got %q", raw) }
	return distribution.PackageRef{Source: "github-releases", Owner: parts[0], Repo: parts[1]}, version, nil
}

func verifyReleasePackage(release distribution.Release, artifact []byte, getenv func(string) string, allowPQPreferredFallback bool) error {
	if len(artifact) == 0 { return errors.New("empty release artifact") }
	if !strings.HasPrefix(release.Manifest.ContentDigest, "sha256:") { return fmt.Errorf("unsupported package content digest %q", release.Manifest.ContentDigest) }
	sum := sha256.Sum256(artifact)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != release.Manifest.ContentDigest { return fmt.Errorf("package artifact digest mismatch: manifest=%s actual=%s", release.Manifest.ContentDigest, actual) }
	if release.Signature.ManifestDigest != release.ManifestDigest || release.Signature.ArtifactDigest != actual { return errors.New("package signature envelope does not bind downloaded manifest and artifact") }
	keys, err := loadTrustedPublisherKeys(getenv)
	if err != nil { return err }
	if err := packagecatalog.VerifySignature(release.Signature, keys, allowPQPreferredFallback); err != nil { return fmt.Errorf("verify package signature: %w", err) }
	return nil
}

func loadTrustedPublisherKeys(getenv func(string) string) (map[string]ed25519.PublicKey, error) {
	raw := ""
	if getenv != nil { raw = getenv("PRAXIS_TRUSTED_KEYS") }
	if strings.TrimSpace(raw) == "" { return nil, errors.New("PRAXIS_TRUSTED_KEYS is required for package install/update; value is JSON mapping key_id to base64 Ed25519 public key") }
	var encoded map[string]string
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil { return nil, fmt.Errorf("parse PRAXIS_TRUSTED_KEYS: %w", err) }
	keys := make(map[string]ed25519.PublicKey, len(encoded))
	for id, value := range encoded {
		if id == "" || value == "" { return nil, errors.New("trusted publisher key id and value are required") }
		body, err := base64.StdEncoding.DecodeString(value)
		if err != nil { return nil, fmt.Errorf("decode trusted publisher key %q: %w", id, err) }
		if len(body) != ed25519.PublicKeySize { return nil, fmt.Errorf("trusted publisher key %q is not an Ed25519 public key", id) }
		keys[id] = ed25519.PublicKey(body)
	}
	return keys, nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) { return false }
	seen := map[string]int{}
	for _, v := range a { seen[v]++ }
	for _, v := range b { seen[v]-- }
	for _, n := range seen { if n != 0 { return false } }
	return true
}

func printJSON(v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil { return err }
	fmt.Println(string(body))
	return nil
}
