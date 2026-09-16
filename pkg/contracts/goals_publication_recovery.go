package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

const (
	GoalsRecoveryOperation     = "publish-goals-from-established-state"
	GoalsRecoveryContract      = "goals-established-state-publication/1"
	GoalsRecoveryRepositoryID  = "1372388187"
	GoalsRecoveryOwnerID       = "263966243"
	GoalsRecoveryCommit        = "fbdc98828d49cf5ddd515edf91d457b606e89a97"
	GoalsRecoveryTree          = "06476050446988f592cd2064823ff73c5a7a09f0"
	GoalsRecoveryReleaseID     = "389997269"
	GoalsRecoveryNonce         = "7faf8b95afeb3bdd58534b0e1139f01169b4d417cd8d30e3b77a1533ad84aad1"
	GoalsRecoveryAuthorityRoot = "sha256:7e247747e70c88ad0feb59f485d31d3b2803e9049c22983587ddf61b500c1e47"
	GoalsRecoveryPublisherKey  = "sha256:602527c02d1a5dfa84661699560289895f70fdcd99c4364fbf1703647eea8dd5"
)

type GoalsRecoveryInput struct {
	CreatedAt, ExpiresAt                           time.Time
	Identity                                       string
	AccountID                                      int64
	Sizes                                          [3]int64
	PredecessorRequestID, PredecessorRequestDigest string
	PredecessorIntentID, PredecessorIntentDigest   string
	AbandonmentEventID, AbandonmentDigest          string
}

// NewGoalsPublicationRecoveryIntent is closed around the one established
// Goals release and the one predecessor abandonment. It accepts no destination
// or package selector.
func NewGoalsPublicationRecoveryIntent(in GoalsRecoveryInput) (ActionIntent, error) {
	if in.Identity == "" || in.AccountID <= 0 || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.Before(in.CreatedAt.Add(24*time.Hour)) {
		return ActionIntent{}, errors.New("successor identity or bounded expiry invalid")
	}
	for _, d := range []string{in.PredecessorRequestDigest, in.PredecessorIntentDigest, in.AbandonmentDigest} {
		if !isSHA256Digest(d) {
			return ActionIntent{}, errors.New("successor predecessor lineage digest invalid")
		}
	}
	if in.PredecessorRequestID == "" || in.PredecessorIntentID == "" || in.AbandonmentEventID == "" {
		return ActionIntent{}, errors.New("successor predecessor lineage missing")
	}
	for _, s := range in.Sizes {
		if s <= 0 {
			return ActionIntent{}, errors.New("successor asset size invalid")
		}
	}
	actor := PrincipalRef{ID: FirstPartyPublisherPrincipal, Kind: "publisher"}
	expires := in.ExpiresAt.UTC().Format(time.RFC3339Nano)
	created := in.CreatedAt.UTC().Format(time.RFC3339Nano)
	params := map[string]string{
		"contract": GoalsRecoveryContract, "installation_digest": GoalsPublicationRoot, "publisher_generation": GoalsPublicationPublisher,
		"package": GoalsPublicationPackage, "manifest_digest": GoalsPublicationManifest, "archive_digest": GoalsPublicationArchive, "signature_digest": GoalsPublicationSignature, "executable_digest": GoalsPublicationExecutable, "signing_provenance": GoalsPublicationSigningReceipt,
		"repository": GoalsPublicationRepository, "repository_id": GoalsRecoveryRepositoryID, "owner_id": GoalsRecoveryOwnerID, "account_id": strconv.FormatInt(in.AccountID, 10), "commit": GoalsRecoveryCommit, "tree": GoalsRecoveryTree, "branch": "refs/heads/main", "branch_target": GoalsRecoveryCommit, "tag": GoalsPublicationTag, "tag_target": GoalsRecoveryCommit,
		"release_id": GoalsRecoveryReleaseID, "release_name": GoalsPublicationPackage, "release_body": "Exact signed Goals initial publication; intent " + GoalsRecoveryNonce, "draft": "true", "prerelease": "false", "generate_release_notes": "false", "make_latest": "false", "asset_inventory": "empty",
		"manifest_name": "praxis-package.json", "manifest_size": strconv.FormatInt(in.Sizes[0], 10), "archive_name": "praxis-package.tar.gz", "archive_size": strconv.FormatInt(in.Sizes[1], 10), "signature_name": "praxis-package.sig.json", "signature_size": strconv.FormatInt(in.Sizes[2], 10),
		"predecessor_request_id": in.PredecessorRequestID, "predecessor_request_digest": in.PredecessorRequestDigest, "predecessor_intent_id": in.PredecessorIntentID, "predecessor_intent_digest": in.PredecessorIntentDigest, "abandonment_event_id": in.AbandonmentEventID, "abandonment_digest": in.AbandonmentDigest, "predecessor_manifest_outcome": "unknown-unresolved",
		"permitted_effects": "upload-manifest,upload-archive,upload-signature,verify-draft-assets,publish-existing-release,verify-published-release", "created_at": created, "expires_at": expires,
	}
	return ActionIntent{Version: "1", ID: "goals-established-state-publication:" + in.Identity, Actor: actor, Operation: GoalsRecoveryOperation, Target: "github.com/" + GoalsPublicationRepository, Scope: "goals-established-state-publication:" + in.Identity, CryptoProfile: CryptoClassicalCompatible, Parameters: params, Preconditions: map[string]string{"repository_id": GoalsRecoveryRepositoryID, "owner_id": GoalsRecoveryOwnerID, "commit": GoalsRecoveryCommit, "tree": GoalsRecoveryTree, "main": GoalsRecoveryCommit, "tag": GoalsRecoveryCommit, "release_id": GoalsRecoveryReleaseID, "release_draft": "true", "assets": "empty", "predecessor_abandonment": in.AbandonmentDigest}}, nil
}

func ValidateGoalsPublicationRecoveryIntent(a ActionIntent) error {
	p := a.Parameters
	parse := func(k string) int64 { n, _ := strconv.ParseInt(p[k], 10, 64); return n }
	created, e1 := time.Parse(time.RFC3339Nano, p["created_at"])
	expiry, e2 := time.Parse(time.RFC3339Nano, p["expires_at"])
	if e1 != nil || e2 != nil {
		return errors.New("successor intent timestamps malformed")
	}
	want, err := NewGoalsPublicationRecoveryIntent(GoalsRecoveryInput{CreatedAt: created, ExpiresAt: expiry, Identity: stringsTrimPrefix(a.ID, "goals-established-state-publication:"), AccountID: parse("account_id"), Sizes: [3]int64{parse("manifest_size"), parse("archive_size"), parse("signature_size")}, PredecessorRequestID: p["predecessor_request_id"], PredecessorRequestDigest: p["predecessor_request_digest"], PredecessorIntentID: p["predecessor_intent_id"], PredecessorIntentDigest: p["predecessor_intent_digest"], AbandonmentEventID: p["abandonment_event_id"], AbandonmentDigest: p["abandonment_digest"]})
	if err != nil {
		return err
	}
	x, _ := json.Marshal(want)
	y, _ := json.Marshal(a)
	if !reflect.DeepEqual(x, y) {
		return fmt.Errorf("successor intent differs from its closed established-state contract")
	}
	return nil
}

func ValidateGoalsPublicationRecoveryDelegation(parent AuthorityGeneration, d DelegationRequest, a ActionIntent, now time.Time) error {
	if err := ValidateGoalsPublicationRecoveryIntent(a); err != nil {
		return err
	}
	digest, err := a.Digest()
	if err != nil {
		return err
	}
	expiry, _ := time.Parse(time.RFC3339Nano, a.Parameters["expires_at"])
	owner, _ := InstallationOwnerPrincipal(GoalsPublicationBootstrap)
	if parent.Digest != GoalsPublicationRoot || parent.ParentRef != "" || parent.Version != "1" || parent.Principal != owner || parent.ProvenanceDigest != GoalsPublicationBootstrap || parent.Scope != InstallationGovernanceScopePrefix+GoalsPublicationBootstrap || parent.AuthorityModel != AuthorityModelID || parent.AuthorityModelVersion != AuthorityModelVersion || parent.AuthorityModelDigest != AuthorityModelDigest() || !containsString(parent.Capabilities, AuthorityDelegateCapability) {
		return errors.New("successor parent is not the exact installation governance root")
	}
	if d.Profile != GoalsPublicationRecoveryProfile || d.ParentRef != parent.Ref || d.ParentVersion != parent.Version || d.ParentDigest != parent.Digest || d.DelegatedPrincipal != a.Actor || d.RequestedAuthority != GovernedPackagePublish || d.RequestedOperation != GoalsRecoveryOperation || d.RequestedScope != a.Scope || len(d.RequestedCapabilities) != 0 || len(d.RequestedOperations) != 0 || d.TargetKind != "action-intent" || d.TargetIdentity != a.ID || d.TargetVersion != a.Version || d.TargetDigest != digest || !reflect.DeepEqual(d.TargetConstraints, []string{a.Target, a.Parameters["repository_id"]}) || d.ProposalVersion != a.Version || d.ProposalDigest != digest || d.ReviewVersion != a.Version || d.ReviewDigest != digest || !d.ExpiresAt.Equal(expiry) || d.PolicyRef != AuthorityModelID || d.PolicyVersion != AuthorityModelGoalsRecoveryVersion || d.PolicyDigest != AuthorityModelGoalsRecoveryDigest() || d.SubjectKind != "publisher" || d.SubjectID != FirstPartyPublisherPrincipal || d.SubjectVersion != "2" || d.SubjectDigest != GoalsPublicationPublisher || d.SubjectKeyDigest != GoalsRecoveryPublisherKey || d.Reason != "publish the exact signed Goals assets to the established draft release" || !now.Before(expiry) {
		return errors.New("delegation is outside the exact v5 Goals successor profile")
	}
	return nil
}

func stringsTrimPrefix(s, p string) string {
	if len(s) >= len(p) && s[:len(p)] == p {
		return s[len(p):]
	}
	return ""
}
