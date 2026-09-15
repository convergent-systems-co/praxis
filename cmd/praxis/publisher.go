package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	internalpublisher "github.com/convergent-systems-co/praxis/internal/publisher"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func runPublisherCommand(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		return writePublisherHelp(os.Stdout)
	}
	switch args[0] {
	case "key-create":
		return runPublisherKeyCreate(args[1:], os.Stdout)
	case "key-inspect":
		return runPublisherKeyInspect(args[1:], os.Stdout)
	case "enroll-preview":
		return runPublisherEnrollPreview(args[1:], os.Stdout)
	case "enroll-approve":
		return runPublisherEnrollmentApprove(args[1:], os.Stdout)
	case "enroll-approval-inspect":
		return runPublisherEnrollmentApprovalInspect(args[1:], os.Stdout)
	case "enroll":
		return runPublisherEnroll(args[1:], os.Getenv, os.Stdout)
	case "authority-preview":
		return runPublisherAuthorityRequest(args[1:], os.Getenv, os.Stdout, false)
	case "authority-request":
		return runPublisherAuthorityRequest(args[1:], os.Getenv, os.Stdout, true)
	case "package-build":
		return runPublisherPackageBuild(args[1:], os.Stdout)
	case "sign-preview":
		return runPublisherSignPreview(args[1:], os.Getenv, os.Stdout)
	case "sign":
		return runPublisherSign(args[1:], os.Getenv, os.Stdout)
	case "receipt":
		return runPublisherReceipt(args[1:], os.Getenv, os.Stdout)
	default:
		return errors.New("usage: praxis publisher {key-create|key-inspect|enroll-preview|enroll-approve|enroll-approval-inspect|enroll|package-build|sign-preview|sign|receipt}")
	}
}

func writePublisherHelp(w io.Writer) error {
	_, err := io.WriteString(w, "usage: praxis publisher <key-create|key-inspect|enroll-preview|enroll-approve|enroll-approval-inspect|enroll|authority-preview|authority-request|package-build|sign-preview|sign|receipt>\n\nPreview commands are read-only. Enrollment approval, authority issuance, and signing require the existing governed installation state and explicit owner authorization. Private key material is never printed or persisted by Praxis.\n")
	return err
}

func publisherBackend() praxiscrypto.PublisherSigningBackend {
	return praxiscrypto.NewMacOSKeychainPublisherBackend()
}

func runPublisherKeyCreate(args []string, out io.Writer) error {
	f := flag.NewFlagSet("publisher key-create", flag.ContinueOnError)
	f.SetOutput(out)
	keyID := f.String("key-id", "", "protected Keychain account identifier")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || strings.TrimSpace(*keyID) == "" {
		return errors.New("usage: praxis publisher key-create --key-id <protected-key-reference>")
	}
	signer, err := publisherBackend().Generate(context.Background(), *keyID)
	if err != nil {
		return err
	}
	return publisherKeyJSON(context.Background(), signer, out)
}

func runPublisherKeyInspect(args []string, out io.Writer) error {
	f := flag.NewFlagSet("publisher key-inspect", flag.ContinueOnError)
	f.SetOutput(out)
	keyID := f.String("key-id", "", "protected key reference")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || strings.TrimSpace(*keyID) == "" {
		return errors.New("usage: praxis publisher key-inspect --key-id <protected-key-reference>")
	}
	signer, err := publisherBackend().Open(context.Background(), *keyID)
	if err != nil {
		return err
	}
	return publisherKeyJSON(context.Background(), signer, out)
}

func publisherKeyJSON(ctx context.Context, signer praxiscrypto.PublisherSigner, out io.Writer) error {
	pub, err := signer.PublicKey(ctx)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(pub)
	return printJSONTo(out, map[string]any{"key_id": signer.KeyID(), "algorithm": signer.Algorithm(), "purpose": "publisher-signing", "public_key_digest": "sha256:" + hex.EncodeToString(sum[:]), "backend": "macos-keychain"})
}

type publisherFlags struct{ keyID, namespace, generation, predecessor, effectiveAt, enrollmentRef string }

func parsePublisherFlags(name string, args []string) (publisherFlags, error) {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	p := publisherFlags{}
	f.StringVar(&p.keyID, "key-id", "", "protected key reference")
	f.StringVar(&p.namespace, "namespace", "praxis.package", "exact package namespace")
	f.StringVar(&p.generation, "generation", "", "publisher generation")
	f.StringVar(&p.predecessor, "predecessor", "", "predecessor generation digest")
	f.StringVar(&p.effectiveAt, "effective-at", "", "RFC3339 effective time")
	f.StringVar(&p.enrollmentRef, "enrollment-ref", "", "owner enrollment reference")
	if err := f.Parse(args); err != nil {
		return p, err
	}
	if f.NArg() != 0 || p.keyID == "" || p.generation == "" || p.effectiveAt == "" || p.enrollmentRef == "" {
		return p, errors.New("publisher generation requires --key-id --generation --effective-at --enrollment-ref")
	}
	return p, nil
}
func publisherGeneration(ctx context.Context, p publisherFlags) (contracts.PublisherGeneration, error) {
	signer, err := publisherBackend().Open(ctx, p.keyID)
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	pub, err := signer.PublicKey(ctx)
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	sum := sha256.Sum256(pub)
	effective, err := time.Parse(time.RFC3339Nano, p.effectiveAt)
	if err != nil {
		return contracts.PublisherGeneration{}, err
	}
	enrollmentSum := sha256.Sum256([]byte(p.enrollmentRef))
	g := contracts.PublisherGeneration{Version: contracts.PublisherGenerationVersion, Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, KeyID: signer.KeyID(), Algorithm: signer.Algorithm(), PublicKey: pub, PublicKeyDigest: "sha256:" + hex.EncodeToString(sum[:]), PackageNamespace: p.namespace, Generation: p.generation, EffectiveAt: effective.UTC(), EnrollmentRef: p.enrollmentRef, EnrollmentDigest: "sha256:" + hex.EncodeToString(enrollmentSum[:]), Predecessor: p.predecessor}
	return g, nil
}
func runPublisherEnrollPreview(args []string, out io.Writer) error {
	return publisherEnrollmentPreview(args, out)
}
func runPublisherEnroll(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("publisher enroll", flag.ContinueOnError)
	var p publisherFlags
	f.StringVar(&p.keyID, "key-id", "", "protected key reference")
	f.StringVar(&p.namespace, "namespace", "praxis.package", "exact package namespace")
	f.StringVar(&p.generation, "generation", "", "publisher generation")
	f.StringVar(&p.predecessor, "predecessor", "", "predecessor generation digest")
	f.StringVar(&p.effectiveAt, "effective-at", "", "RFC3339 effective time")
	f.StringVar(&p.enrollmentRef, "enrollment-ref", "", "owner enrollment reference")
	approval := f.String("approval-id", "", "exact governed enrollment approval")
	actorID := f.String("actor-id", "", "approving actor id")
	actorKind := f.String("actor-kind", "", "approving actor kind")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || p.keyID == "" || p.generation == "" || p.effectiveAt == "" || p.enrollmentRef == "" || *approval == "" || *actorID == "" || *actorKind == "" {
		return errors.New("usage: praxis publisher enroll --key-id <ref> --generation <id> --effective-at <RFC3339> --enrollment-ref <ref> --approval-id <id> --actor-id <id> --actor-kind <kind>")
	}
	g, err := publisherGeneration(context.Background(), p)
	if err != nil {
		return err
	}
	digest, err := g.Digest()
	if err != nil {
		return err
	}
	actor := contracts.PrincipalRef{ID: *actorID, Kind: *actorKind}
	intent := contracts.ActionIntent{Version: "v1", ID: "publisher-enroll:" + p.generation, Actor: actor, Operation: "publisher.enroll", Target: digest, Scope: "package:" + p.namespace}
	db, err := state.OpenSQLite(context.Background(), getenv("PRAXIS_DB"))
	if err != nil {
		return err
	}
	defer db.Close()
	commandID, err := state.New(db).EnrollPublisherGeneration(context.Background(), g, intent, *approval, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "publisher.enroll", "command_id": commandID, "generation_digest": digest, "principal": g.Principal, "scope": "package:" + p.namespace})
}

type publisherAuthorityOptions struct{ generation, parentRef, parentVersion, parentDigest, namespace, requestID, proposalVersion, proposalDigest, reviewVersion, reviewDigest, expiresAt, reason string }

func runPublisherAuthorityRequest(args []string, getenv func(string) string, out io.Writer, persist bool) error {
	f := flag.NewFlagSet("publisher authority", flag.ContinueOnError)
	o := publisherAuthorityOptions{}
	f.StringVar(&o.generation, "publisher-generation-digest", "", "exact enrolled PublisherGeneration digest")
	f.StringVar(&o.parentRef, "parent-ref", "", "exact installation-root authority ref")
	f.StringVar(&o.parentVersion, "parent-version", "1", "parent authority generation version")
	f.StringVar(&o.parentDigest, "parent-digest", "", "exact installation-root authority digest")
	f.StringVar(&o.namespace, "namespace", "praxis.package", "exact package namespace")
	f.StringVar(&o.requestID, "request-id", "", "stable authority request identity")
	f.StringVar(&o.proposalVersion, "proposal-version", "1", "publisher authority proposal version")
	f.StringVar(&o.proposalDigest, "proposal-digest", "", "exact publisher authority proposal digest")
	f.StringVar(&o.reviewVersion, "review-version", "1", "publisher authority review version")
	f.StringVar(&o.reviewDigest, "review-digest", "", "exact publisher authority review digest")
	f.StringVar(&o.expiresAt, "expires-at", "", "RFC3339 authority expiry")
	f.StringVar(&o.reason, "reason", "first-party package publishing", "bounded authority reason")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || o.generation == "" || o.parentRef == "" || o.parentDigest == "" || o.requestID == "" || o.proposalDigest == "" || o.reviewDigest == "" || o.expiresAt == "" {
		return errors.New("usage: praxis publisher authority-{preview|request} --publisher-generation-digest <digest> --parent-ref <ref> --parent-digest <digest> --request-id <id> --proposal-digest <digest> --review-digest <digest> --expires-at <RFC3339>")
	}
	expires, err := time.Parse(time.RFC3339Nano, o.expiresAt)
	if err != nil {
		return err
	}
	scope, err := contracts.PackagePublishScope(o.namespace)
	if err != nil {
		return err
	}
	db, err := state.OpenSQLite(context.Background(), getenv("PRAXIS_DB"))
	if err != nil {
		return err
	}
	defer db.Close()
	publisherRecord, err := state.New(db).PublisherGeneration(context.Background(), o.generation)
	if err != nil {
		return err
	}
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfilePackagePublish, ParentRef: o.parentRef, ParentVersion: o.parentVersion, ParentDigest: o.parentDigest, DelegatedPrincipal: publisherRecord.Generation.Principal, TargetKind: "publisher-generation", TargetIdentity: publisherRecord.Generation.Principal.ID, TargetVersion: o.namespace, TargetDigest: o.generation, TargetConstraints: []string{o.namespace}, SubjectKind: publisherRecord.Generation.Principal.Kind, SubjectID: publisherRecord.Generation.Principal.ID, SubjectVersion: publisherRecord.Generation.Generation, SubjectDigest: o.generation, SubjectKeyDigest: publisherRecord.Generation.PublicKeyDigest, RequestedCapabilities: nil, RequestedOperations: nil, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: "sign", RequestedScope: scope, ProposalVersion: o.proposalVersion, ProposalDigest: o.proposalDigest, ReviewVersion: o.reviewVersion, ReviewDigest: o.reviewDigest, ExpiresAt: expires.UTC(), Reason: o.reason, PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelSuccessorVersion, PolicyDigest: contracts.AuthorityModelSuccessorDigest()}
	request := contracts.AuthorityRequest{ID: o.requestID, Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: scope, Reason: o.reason, Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	digest, err := request.Digest()
	if err != nil {
		return err
	}
	result := map[string]any{"operation": "authority.delegate", "profile": contracts.DelegationProfilePackagePublish, "request": request, "request_digest": digest, "publisher_generation_digest": o.generation, "scope": scope, "preview": !persist}
	if persist {
		repo, db2, e := openGovernedRepository(context.Background(), getenv)
		if e != nil {
			return e
		}
		defer db2.Close()
		saved, e := repo.SaveAuthorityRequest(context.Background(), request, time.Now().UTC(), &expires)
		if e != nil {
			return e
		}
		result["request_digest"] = saved
	}
	return printJSONTo(out, result)
}

func runPublisherPackageBuild(args []string, out io.Writer) error {
	f := flag.NewFlagSet("publisher package-build", flag.ContinueOnError)
	f.SetOutput(out)
	exe := f.String("executable", "", "Goals plugin executable")
	dir := f.String("output-dir", "", "artifact output directory")
	source := f.String("source-identity", "", "exact committed source identity")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *exe == "" || *dir == "" || *source == "" {
		return errors.New("usage: praxis publisher package-build --executable <path> --output-dir <dir> --source-identity <commit/tree>")
	}
	body, err := os.ReadFile(*exe)
	if err != nil {
		return err
	}
	input, err := goals.PackageBuildInput(body)
	if err != nil {
		return err
	}
	built, err := packagecatalog.BuildPackage(input)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*dir, "praxis-package.json"), built.ManifestBytes, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*dir, "praxis-package.tar.gz"), built.ArtifactBytes, 0600); err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"package_id": built.Manifest.PackageID, "version": built.Manifest.Version, "manifest_digest": built.ManifestDigest, "artifact_digest": built.ArtifactDigest, "source_identity": *source, "executable_digest": goalsExecutableDigest(body), "files": []string{"praxis-package.json", "praxis-package.tar.gz"}})
}
func goalsExecutableDigest(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

type packageFiles struct {
	built                     packagecatalog.BuiltPackage
	manifestPath, archivePath string
}

func loadUnsignedPackage(dir string) (packageFiles, error) {
	m, err := os.ReadFile(filepath.Join(dir, "praxis-package.json"))
	if err != nil {
		return packageFiles{}, err
	}
	a, err := os.ReadFile(filepath.Join(dir, "praxis-package.tar.gz"))
	if err != nil {
		return packageFiles{}, err
	}
	b, err := packagecatalog.ParseUnsignedPackage(m, a)
	if err != nil {
		return packageFiles{}, err
	}
	return packageFiles{built: b, manifestPath: filepath.Join(dir, "praxis-package.json"), archivePath: filepath.Join(dir, "praxis-package.tar.gz")}, nil
}
func runPublisherSignPreview(args []string, getenv func(string) string, out io.Writer) error {
	dir, gen, keyID, source, builder, qual, err := parseSignFlags("publisher sign-preview", args)
	if err != nil {
		return err
	}
	pk, err := loadUnsignedPackage(dir)
	if err != nil {
		return err
	}
	db, err := state.OpenSQLite(context.Background(), getenv("PRAXIS_DB"))
	if err != nil {
		return err
	}
	defer db.Close()
	record, err := state.New(db).PublisherGeneration(context.Background(), gen)
	if err != nil {
		return err
	}
	if !record.Generation.PackageNamespaceAllowed(pk.built.Manifest.PackageID) {
		return errors.New("publisher namespace does not allow package")
	}
	if keyID != record.Generation.KeyID {
		return errors.New("signing preview key does not match enrolled publisher generation")
	}
	return printJSONTo(out, map[string]any{"operation": "package.sign", "preview": true, "publisher_principal": record.Generation.Principal.ID, "publisher_generation_digest": gen, "key_id": keyID, "package_id": pk.built.Manifest.PackageID, "package_version": pk.built.Manifest.Version, "manifest_digest": pk.built.ManifestDigest, "artifact_digest": pk.built.ArtifactDigest, "source_identity": source, "builder_identity": builder, "qualification_ref": qual, "namespace": record.Generation.PackageNamespace})
}
func parseSignFlags(name string, args []string) (string, string, string, string, string, string, error) {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	dir := f.String("package-dir", "", "unsigned package directory")
	gen := f.String("generation-digest", "", "enrolled publisher generation digest")
	keyID := f.String("key-id", "", "protected publisher signing key reference")
	source := f.String("source-identity", "", "committed source identity")
	builder := f.String("builder-identity", "praxis-package-builder/v1", "builder identity")
	qual := f.String("qualification-ref", "", "qualification reference")
	if err := f.Parse(args); err != nil {
		return "", "", "", "", "", "", err
	}
	if f.NArg() != 0 || *dir == "" || *gen == "" || *source == "" {
		return "", "", "", "", "", "", errors.New("usage: praxis publisher sign[-preview] --package-dir <dir> --generation-digest <digest> --key-id <protected-key-reference> --source-identity <commit/tree>")
	}
	if *keyID == "" {
		return "", "", "", "", "", "", errors.New("--key-id is required")
	}
	return *dir, *gen, *keyID, *source, *builder, *qual, nil
}
func runPublisherSign(args []string, getenv func(string) string, out io.Writer) error {
	dir, gen, keyID, source, builder, qual, err := parseSignFlags("publisher sign", args)
	if err != nil {
		return err
	}
	pk, err := loadUnsignedPackage(dir)
	if err != nil {
		return err
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	signer, err := publisherBackend().Open(context.Background(), keyID)
	if err != nil {
		return err
	}
	signed, err := internalpublisher.Sign(context.Background(), state.New(db), repo, signer, gen, pk.built, source, builder, qual, time.Now().UTC())
	if err != nil {
		return err
	}
	body, err := json.Marshal(signed.Envelope)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "praxis-package.sig.json"), body, 0600); err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"signed": true, "package_id": pk.built.Manifest.PackageID, "version": pk.built.Manifest.Version, "manifest_digest": pk.built.ManifestDigest, "artifact_digest": pk.built.ArtifactDigest, "signature_digest": signed.Provenance.SignatureEnvelopeDigest, "provenance_digest": signed.ProvenanceDigest})
}
func runPublisherReceipt(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("publisher receipt", flag.ContinueOnError)
	d := f.String("digest", "", "provenance digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *d == "" {
		return errors.New("usage: praxis publisher receipt --digest <provenance-digest>")
	}
	db, err := state.OpenSQLite(context.Background(), getenv("PRAXIS_DB"))
	if err != nil {
		return err
	}
	defer db.Close()
	r, err := state.New(db).PublisherSigningReceipt(context.Background(), *d)
	if err != nil {
		return err
	}
	return printJSON(r)
}
func printJSONTo(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
