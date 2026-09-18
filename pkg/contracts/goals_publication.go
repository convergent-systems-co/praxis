package contracts

// This is the single installation/package/destination operation accepted in
// ADR-078/SPEC-041. It is deliberately not a general publication contract.
import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

const (
	AuthorityModelGoalsPublicationVersion = "v4"
	AuthorityModelGoalsRecoveryVersion    = "v5"
	GoalsPublicationProfile               = "GOALS_INITIAL_PUBLICATION"
	GoalsPublicationRecoveryProfile       = "GOALS_PUBLICATION_FROM_ESTABLISHED_STATE"
	GoalsPublicationOperation             = "publish-initial-goals"
	GoalsPublicationBootstrap             = "sha256:3a4152a726102de408cc4e6ee329113ff8455e4924538bf28e0bab23e4995b00"
	GoalsPublicationRoot                  = "sha256:7e247747e70c88ad0feb59f485d31d3b2803e9049c22983587ddf61b500c1e47"
	GoalsPublicationPublisher             = "sha256:c199600cb21987e9010917c60d8ea526c16aa5f4a58f98257f7a71a0108dd17d"
	GoalsPublicationPackage               = "praxis.package.goals@0.1.0"
	GoalsPublicationManifest              = "sha256:023a191f4844e05c32e85a464e23bf229bc00c5c29be07ea14ace21356bb5e9d"
	GoalsPublicationArchive               = "sha256:857e3ca52b8708376f4ed3b5db3578292b69605ea4af243df293c4b57702e156"
	GoalsPublicationSignature             = "sha256:2296ed167c642648abca44d2f0e561b55f991e845575d074d1816d09b371fe59"
	GoalsPublicationExecutable            = "sha256:455d1a6af72bd206071e52daf2b3bfbb906177a5eea36f1e6840fab0053c5581"
	GoalsPublicationSigningReceipt        = "sha256:fc1d94e6bdc69e9d8e31b27b30db5ebd90945ff587e3f638fb2871aa19562d01"
	GoalsPublicationRepository            = "convergent-systems-co/praxis-packages"
	GoalsPublicationTag                   = "goals/v0.1.0"
)

// The successor binds the accepted exact rule specification, not merely a new
// display name. Historical model digest functions are deliberately unchanged.
func AuthorityModelGoalsPublicationDigest() string {
	b, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelGoalsPublicationVersion, AuthorityModelDeploymentDigest(), GoalsPublicationProfile, GoalsPublicationOperation, "sha256:f8e107bec312b1754a71280550df7729f67bc142eaebe46c081fb98ef081da7e"})
	return publicationSHA(b)
}

// AuthorityModelGoalsRecoveryDigest is the sole additive v5 rule. It retains
// the immutable v4 identity and adds the one closed established-state profile.
func AuthorityModelGoalsRecoveryDigest() string {
	b, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelGoalsRecoveryVersion, AuthorityModelGoalsPublicationDigest(), GoalsPublicationRecoveryProfile, "publish-goals-from-established-state", "sha256:deda0d3ab0c8ab6f62ead7754856d6d8037a8612bfc51021203d5483d5ce0f4a"})
	return publicationSHA(b)
}
func publicationSHA(b []byte) string { s := sha256.Sum256(b); return fmt.Sprintf("sha256:%x", s) }

// GoalsPublicationInput contains only observations required to freeze this
// operation. The existing ActionIntent is the persisted authoritative payload.
type GoalsPublicationInput struct {
	RepositoryID  int64
	OwnerID       int64
	AccountID     int64
	CreatedAt     time.Time
	ExpiresAt     time.Time
	Nonce         string
	ManifestSize  int64
	ArchiveSize   int64
	SignatureSize int64
}

func GoalsPublicationDescriptor() []byte {
	b, _ := json.Marshal(struct{ Package, Manifest, Archive, Signature, Executable, SigningProvenance string }{GoalsPublicationPackage, GoalsPublicationManifest, GoalsPublicationArchive, GoalsPublicationSignature, GoalsPublicationExecutable, GoalsPublicationSigningReceipt})
	return append(b, '\n')
}
func goalsGitObject(kind string, b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s %d%c", kind, len(b), byte(0))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// GoalsPublicationGitObjects freezes a root commit without running Git, reading
// Git config, creating a worktree, or mutating any external repository.
func GoalsPublicationGitObjects(at time.Time, nonce string) (descriptor, tree, commit []byte) {
	descriptor = GoalsPublicationDescriptor()
	blob, _ := hex.DecodeString(goalsGitObject("blob", descriptor))
	tree = append([]byte("100644 goals-publication.json\x00"), blob...)
	identity := fmt.Sprintf("Praxis publisher <publisher@praxis.invalid> %d +0000", at.Unix())
	commit = []byte(fmt.Sprintf("tree %s\nauthor %s\ncommitter %s\n\nGoals initial publication %s\n", goalsGitObject("tree", tree), identity, identity, nonce))
	return
}
func NewGoalsPublicationIntent(p GoalsPublicationInput) (ActionIntent, error) {
	if p.RepositoryID <= 0 || p.OwnerID <= 0 || p.AccountID <= 0 || p.CreatedAt.IsZero() || !p.ExpiresAt.After(p.CreatedAt) || p.ManifestSize <= 0 || p.ArchiveSize <= 0 || p.SignatureSize <= 0 {
		return ActionIntent{}, errors.New("incomplete Goals publication observations")
	}
	nonce, e := hex.DecodeString(p.Nonce)
	if e != nil || len(nonce) != 32 || hex.EncodeToString(nonce) != p.Nonce {
		return ActionIntent{}, errors.New("publication nonce must be 32 random bytes in canonical hex")
	}
	at := p.CreatedAt.UTC().Truncate(time.Second)
	descriptor, tree, commit := GoalsPublicationGitObjects(at, p.Nonce)
	a := ActionIntent{Version: "1", ID: "goals-initial-publication:" + p.Nonce, Actor: PrincipalRef{ID: FirstPartyPublisherPrincipal, Kind: "publisher"}, Operation: GoalsPublicationOperation, Target: "github.com/" + GoalsPublicationRepository, Scope: fmt.Sprintf("goals-initial-publication:github.com:%d:%s", p.RepositoryID, p.Nonce), CryptoProfile: CryptoClassicalCompatible}
	a.Parameters = map[string]string{
		"contract": "goals-initial-publication/1", "bootstrap": GoalsPublicationBootstrap, "root": GoalsPublicationRoot, "publisher_generation": GoalsPublicationPublisher, "package": GoalsPublicationPackage,
		"manifest_digest": GoalsPublicationManifest, "archive_digest": GoalsPublicationArchive, "signature_digest": GoalsPublicationSignature, "executable_digest": GoalsPublicationExecutable, "signing_provenance": GoalsPublicationSigningReceipt,
		"repository_id": strconv.FormatInt(p.RepositoryID, 10), "owner_id": strconv.FormatInt(p.OwnerID, 10), "account_id": strconv.FormatInt(p.AccountID, 10), "nonce": p.Nonce, "created_at": at.Format(time.RFC3339), "expires_at": p.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"manifest_size": strconv.FormatInt(p.ManifestSize, 10), "archive_size": strconv.FormatInt(p.ArchiveSize, 10), "signature_size": strconv.FormatInt(p.SignatureSize, 10),
		"descriptor_digest": publicationSHA(descriptor), "tree": goalsGitObject("tree", tree), "commit": goalsGitObject("commit", commit), "branch": "refs/heads/main", "tag": GoalsPublicationTag,
		"release_name": GoalsPublicationPackage, "release_body": "Exact signed Goals initial publication; intent " + p.Nonce, "prerelease": "false", "generate_release_notes": "false", "make_latest": "false",
		"steps": "refs,draft,manifest,archive,signature,verify,publish,verify-published",
	}
	a.Preconditions = map[string]string{"repository": "existing-empty", "branch": "absent", "tag": "absent", "release": "absent", "force": "forbidden", "replace_assets": "forbidden"}
	return a, nil
}

func ValidateGoalsPublicationIntent(a ActionIntent) error {
	p := a.Parameters
	parse := func(k string) int64 {
		v, e := strconv.ParseInt(p[k], 10, 64)
		if e != nil {
			return 0
		}
		return v
	}
	at, e := time.Parse(time.RFC3339, p["created_at"])
	if e != nil {
		return e
	}
	expiry, e := time.Parse(time.RFC3339Nano, p["expires_at"])
	if e != nil {
		return e
	}
	want, e := NewGoalsPublicationIntent(GoalsPublicationInput{RepositoryID: parse("repository_id"), OwnerID: parse("owner_id"), AccountID: parse("account_id"), CreatedAt: at, ExpiresAt: expiry, Nonce: p["nonce"], ManifestSize: parse("manifest_size"), ArchiveSize: parse("archive_size"), SignatureSize: parse("signature_size")})
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(a, want) {
		return errors.New("Goals initial-publication intent differs from the closed operation")
	}
	return nil
}

// ValidateGoalsPublicationDelegation validates containment AND the protected
// intent. Callers must load that intent from durable state, never from a grant
// file. All historical validators retain their original semantics.
func ValidateGoalsPublicationDelegation(parent AuthorityGeneration, d DelegationRequest, a ActionIntent, now time.Time) error {
	if e := ValidateGoalsPublicationIntent(a); e != nil {
		return e
	}
	if e := d.Validate(now); e != nil {
		return e
	}
	if e := parent.VerifyDigest(); e != nil {
		return e
	}
	digest, _ := a.Digest()
	expiry, _ := time.Parse(time.RFC3339Nano, a.Parameters["expires_at"])
	owner, _ := InstallationOwnerPrincipal(GoalsPublicationBootstrap)
	if parent.Digest != GoalsPublicationRoot || parent.ParentRef != "" || parent.Version != "1" || parent.Principal != owner || parent.ProvenanceDigest != GoalsPublicationBootstrap || parent.Scope != InstallationGovernanceScopePrefix+GoalsPublicationBootstrap || !containsString(parent.Capabilities, AuthorityDelegateCapability) {
		return errors.New("publication parent is not the exact installation root")
	}
	if d.Profile != GoalsPublicationProfile || d.ParentRef != parent.Ref || d.ParentVersion != parent.Version || d.ParentDigest != parent.Digest || d.DelegatedPrincipal != a.Actor || d.RequestedAuthority != GovernedPackagePublish || d.RequestedOperation != GoalsPublicationOperation || d.RequestedScope != a.Scope || len(d.RequestedCapabilities) != 0 || len(d.RequestedOperations) != 0 || d.TargetKind != "action-intent" || d.TargetIdentity != a.ID || d.TargetVersion != a.Version || d.TargetDigest != digest || !reflect.DeepEqual(d.TargetConstraints, []string{a.Target, a.Parameters["repository_id"]}) || d.ProposalVersion != a.Version || d.ProposalDigest != digest || d.ReviewVersion != a.Version || d.ReviewDigest != digest || !d.ExpiresAt.Equal(expiry) || d.PolicyRef != AuthorityModelID || d.PolicyVersion != AuthorityModelGoalsPublicationVersion || d.PolicyDigest != AuthorityModelGoalsPublicationDigest() || d.SubjectKind != "publisher" || d.SubjectID != FirstPartyPublisherPrincipal || d.SubjectVersion != "2" || d.SubjectDigest != GoalsPublicationPublisher || d.SubjectKeyDigest != "sha256:602527c02d1a5dfa84661699560289895f70fdcd99c4364fbf1703647eea8dd5" {
		return errors.New("delegation is not the exact Goals initial-publication edge")
	}
	return nil
}
