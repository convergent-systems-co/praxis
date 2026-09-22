package goalstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Review #7 N17: a delegated package.publish generation that has been retired
// in a CURRENT store (no rollback, no replay, no restored Keychain state) must
// stop authorizing signing. These predicates exercise the resolver that both
// BuildSigningPreview and the final pre-sign revalidation use.

const publishNamespace = "praxis.package"

// delegatedNamespace is the namespace the fixture delegates; a test may widen it
// (and must restore the OBSERVED value) to separate the authority namespace from
// the publisher generation's own namespace.
var delegatedNamespace = publishNamespace

// publishDelegationFixture builds a real, anchored installation with an approved
// delegated package.publish generation for the first-party publisher.
func publishDelegationFixture(t *testing.T) (a anchored, root, child contracts.AuthorityGeneration, publisherDigest string, now time.Time) {
	t.Helper()
	a = anchoredFixture(t)
	now = time.Now().UTC().Truncate(time.Microsecond)
	root = rootSuccessionFixture(t, a.repo, now)
	publisherDigest = "sha256:" + strings.Repeat("e", 64)
	child = publishDelegationOn(t, a, root, now, publisherDigest, "sha256:"+strings.Repeat("d", 64), "2")
	return a, root, child, publisherDigest, now
}

// publishDelegationOn approves the package.publish delegation from root for the
// given publisher generation identity.
func publishDelegationOn(t *testing.T, a anchored, root contracts.AuthorityGeneration, now time.Time, publisherDigest, keyDigest, generationVersion string) contracts.AuthorityGeneration {
	t.Helper()
	child, err := tryPublishDelegation(t, a, root, now, publisherDigest, keyDigest, generationVersion)
	if err != nil {
		t.Fatalf("package.publish delegation failed: %v", err)
	}
	return child
}

// tryPublishDelegation is publishDelegationOn returning the refusal instead of failing.
func tryPublishDelegation(t *testing.T, a anchored, root contracts.AuthorityGeneration, now time.Time, publisherDigest, keyDigest, generationVersion string) (contracts.AuthorityGeneration, error) {
	t.Helper()
	ctx := context.Background()
	principal := contracts.PrincipalRef{ID: "publisher:praxis-first-party", Kind: "publisher"}
	scope, err := contracts.PackagePublishScope(delegatedNamespace)
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour)
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfilePackagePublish, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: principal, TargetKind: "publisher-generation", TargetIdentity: principal.ID, TargetVersion: delegatedNamespace, TargetDigest: publisherDigest, TargetConstraints: []string{delegatedNamespace}, SubjectKind: "publisher", SubjectID: principal.ID, SubjectVersion: generationVersion, SubjectDigest: publisherDigest, SubjectKeyDigest: keyDigest, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: "sign", RequestedScope: scope, ProposalVersion: "1", ProposalDigest: "sha256:" + strings.Repeat("b", 64), ReviewVersion: "1", ReviewDigest: "sha256:" + strings.Repeat("c", 64), ExpiresAt: expires, Reason: "governed package publishing", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelSuccessorVersion, PolicyDigest: contracts.AuthorityModelSuccessorDigest()}
	request := contracts.AuthorityRequest{ID: "publisher-authority-request:" + strings.Repeat("b", 64) + ":" + strings.Repeat("c", 64), Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: scope, Reason: delegation.Reason, Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	requestDigest, err := a.repo.SaveAuthorityRequest(ctx, request, now, &expires)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: "1", RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelSuccessorDigest(), IssuedAt: now, ExpiresAt: &expires, Delegation: &delegation}
	return a.repo.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(ctx, request.ID, "1", decision, builtinTestPolicy{}, now)
}

func retirePublishChild(t *testing.T, a anchored, child, root contracts.AuthorityGeneration) {
	t.Helper()
	at := time.Now().UTC().Truncate(time.Microsecond)
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: child.Ref, Version: child.Version, GenerationDigest: child.Digest, InvalidationRef: "retire-" + child.Ref, InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: at, Reason: "test retirement"}
	if err := a.repo.SaveAuthorityGenerationInvalidation(context.Background(), invalidation, at, nil); err != nil {
		t.Fatal(err)
	}
}

func resolveAt(a anchored, publisher string) (contracts.PackagePublishAuthorization, error) {
	return a.repo.ResolvePackagePublishAuthority(context.Background(), publisher, "praxis.package.goals", time.Now().UTC().Add(time.Second))
}

func TestPackagePublishAuthorityResolvesForACurrentDelegatedGeneration(t *testing.T) {
	a, _, child, publisher, _ := publishDelegationFixture(t)
	got, err := resolveAt(a, publisher)
	if err != nil || got.Generation.Digest != child.Digest {
		t.Fatalf("control: a current delegated generation must resolve: %v %+v", err, got.Generation)
	}
}

// N17: the child generation is retired in a current store.
func TestRetiredDelegatedPublisherGenerationDoesNotResolvePackagePublishAuthority(t *testing.T) {
	a, root, child, publisher, _ := publishDelegationFixture(t)
	retirePublishChild(t, a, child, root)
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("N17: a retired delegated package.publish generation still resolves and would authorize signing")
	}
	if err := a.repo.ValidatePackagePublishAuthority(context.Background(), publisher, "praxis.package.goals", time.Now().UTC().Add(time.Second)); err == nil {
		t.Fatal("N17: the final pre-sign validation accepts a retired delegated generation")
	}
}

// Deleting the invalidation row must not restore signing authority: the
// anchored generation_retired fact and the retired liveness record remain.
func TestDeletingTheInvalidationRowDoesNotRestorePackagePublishAuthority(t *testing.T) {
	a, root, child, publisher, _ := publishDelegationFixture(t)
	retirePublishChild(t, a, child, root)
	keylessDelete(t, a.store, authorityGenerationInvalidationNamespace, child.Ref, child.Version)
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("deleting the invalidation row restored package.publish authority")
	}
}

// Deleting the invalidation row AND replaying the earlier authentic liveness row
// (N14's attack) must not restore authority: the anchored fact still retires it.
func TestReplayingTheLivenessRowDoesNotRestorePackagePublishAuthority(t *testing.T) {
	a, root, child, publisher, _ := publishDelegationFixture(t)
	earlier := snapshotRow(t, a.store, state.AuthorityGenerationLiveNamespace, child.Ref, child.Version)
	retirePublishChild(t, a, child, root)
	keylessDelete(t, a.store, authorityGenerationInvalidationNamespace, child.Ref, child.Version)
	replayRow(t, a.store, earlier)
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("replaying the earlier liveness row restored package.publish authority")
	}
}

// Deleting the liveness row (a positive record) never grants: absence is not authority.
func TestMissingLivenessRowYieldsNoPackagePublishAuthority(t *testing.T) {
	a, _, child, publisher, _ := publishDelegationFixture(t)
	keylessDelete(t, a.store, state.AuthorityGenerationLiveNamespace, child.Ref, child.Version)
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("a delegated generation without its authenticated liveness record must not authorize signing")
	}
}

// Parent current, child retired: the parent decision is still effective, which is
// exactly what made the resolver accept the child.
func TestParentCurrentChildRetiredIsRefused(t *testing.T) {
	a, root, child, publisher, _ := publishDelegationFixture(t)
	retirePublishChild(t, a, child, root)
	requestID, requestVersion, _ := strings.Cut(child.DelegationRef, "/")
	if _, err := a.repo.LoadAuthorityDecision(context.Background(), requestID, requestVersion, time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatalf("premise: the parent decision must still be current: %v", err)
	}
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("parent-current / child-retired must be refused")
	}
}

// A store that is not current against the anchor yields no publish authority
// (must stay true after the repair).
func TestRolledBackStoreYieldsNoPackagePublishAuthority(t *testing.T) {
	a, _, _, publisher, _ := publishDelegationFixture(t)
	if _, err := resolveAt(a, publisher); err != nil {
		t.Fatalf("premise: %v", err)
	}
	cur, err := a.anchor.Load(context.Background(), successionTestBootstrap)
	if err != nil {
		t.Fatal(err)
	}
	next := cur
	next.Seq++
	next.Head = "sha256:" + strings.Repeat("9", 64)
	if err := a.anchor.Set(context.Background(), successionTestBootstrap, &cur, next); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("a store behind the forward authority anchor must yield no package.publish authority")
	}
}

// A delegated child whose ROOT was superseded is not current either: the parent
// decision remains effective (LoadAuthorityDecision does not check its issuer),
// but the lineage no longer terminates at the current installation root.
func TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority(t *testing.T) {
	a, root, _, publisher, now := publishDelegationFixture(t)
	if _, err := resolveAt(a, publisher); err != nil {
		t.Fatalf("control: %v", err)
	}
	successor, _, _, _ := acceptedRootSuccession(t, a.repo, root, now.Add(time.Millisecond))
	if successor.Digest == root.Digest {
		t.Fatal("premise: the root must have been superseded")
	}
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("N17-equivalent: a delegated publisher generation whose root was superseded still authorizes signing")
	}
}

// G4: a retired root must not be able to mint a delegated generation. The
// delegation path loaded its parent by immutable record and wrote the decision
// inline, so nothing asked whether the parent was still in force.
func TestRetiredRootCannotMintADelegatedPublisherGeneration(t *testing.T) {
	a := anchoredFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	root := rootSuccessionFixture(t, a.repo, now)
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: root.Ref, Version: root.Version, GenerationDigest: root.Digest, InvalidationRef: "retire-root", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: root.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
	if err := a.repo.SaveAuthorityGenerationInvalidation(context.Background(), invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if child, err := tryPublishDelegation(t, a, root, now, "sha256:"+strings.Repeat("e", 64), "sha256:"+strings.Repeat("d", 64), "2"); err == nil {
		t.Fatalf("a retired root minted a delegated generation: %+v", child)
	}
}

// Every selection term of the resolver is exercised: a candidate that violates
// exactly ONE term must not resolve, even though it is otherwise a complete,
// current, correctly delegated generation (its lineage, request and decision are
// the legitimate ones). The legitimate child is retired first so the variant is
// the only candidate. A variant the store itself refuses to save is asserted as
// refused: the resolver term is then unreachable through the store.
func TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm(t *testing.T) {
	variants := []struct {
		name   string
		mutate func(g *contracts.AuthorityGeneration)
	}{
		{"authority model version", func(g *contracts.AuthorityGeneration) {
			g.AuthorityModelVersion, g.AuthorityModelDigest = contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelDeploymentDigest()
		}},
		{"authority model digest", func(g *contracts.AuthorityGeneration) {
			g.AuthorityModelDigest = contracts.AuthorityModelDeploymentDigest()
		}},
		{"delegation profile", func(g *contracts.AuthorityGeneration) {
			g.DelegationProfile = contracts.DelegationProfilePackageDeploy
		}},
		{"subject digest", func(g *contracts.AuthorityGeneration) { g.SubjectDigest = "sha256:" + strings.Repeat("f", 64) }},
		{"granted authority", func(g *contracts.AuthorityGeneration) { g.Authorities = []string{contracts.GovernedPackageDeploy} }},
		{"scope namespace form", func(g *contracts.AuthorityGeneration) { g.Scope = "not-a-package-namespace:" + publishNamespace }},
		{"scope namespace differs from canonical", func(g *contracts.AuthorityGeneration) { g.Scope = "package-namespace:other.namespace" }},
		{"generation state", func(g *contracts.AuthorityGeneration) { g.State = contracts.AuthorityGenerationState("superseded") }},
		{"delegation reference form", func(g *contracts.AuthorityGeneration) { g.DelegationRef = "no-request-version-delimiter" }},
		{"delegation request exists", func(g *contracts.AuthorityGeneration) { g.DelegationRef = "publisher-authority-request:absent/1" }},
		{"delegation digest present", func(g *contracts.AuthorityGeneration) { g.DelegationDigest = "" }},
		{"parent digest binds the parent record", func(g *contracts.AuthorityGeneration) { g.ParentDigest = "sha256:" + strings.Repeat("f", 64) }},
	}
	for _, tc := range variants {
		t.Run(tc.name, func(t *testing.T) {
			a, root, child, publisher, _ := publishDelegationFixture(t)
			variant := child
			variant.Ref = child.Ref + "-variant"
			tc.mutate(&variant)
			var err error
			if variant.Digest, err = variant.ComputeDigest(); err != nil {
				// An inconsistent record cannot carry a valid digest, so it can never be
				// stored or decoded: the resolver term is unreachable through any record
				// the store will hold. Asserted, not assumed.
				t.Logf("no valid digest exists for this variant (%v): unreachable through the store", err)
				return
			}
			if err := a.repo.SaveAuthorityGeneration(context.Background(), variant, variant.EffectiveAt, nil); err != nil {
				t.Logf("the store refuses this variant (%v): the resolver term is unreachable through the store", err)
				return
			}
			retirePublishChild(t, a, child, root) // the legitimate child no longer shadows the variant
			if got, err := resolveAt(a, publisher); err == nil {
				t.Fatalf("a candidate violating the %q term resolved: %+v", tc.name, got.Generation)
			}
		})
	}
}

// The resolver is bound to the exact publisher generation it was asked about.
func TestResolverDoesNotResolveForADifferentPublisherGeneration(t *testing.T) {
	a, _, _, _, _ := publishDelegationFixture(t)
	if _, err := resolveAt(a, "sha256:"+strings.Repeat("9", 64)); err == nil {
		t.Fatal("authority for one publisher generation resolved for another")
	}
	if _, err := resolveAt(a, ""); err == nil {
		t.Fatal("an empty publisher generation resolved")
	}
	if _, err := a.repo.ResolvePackagePublishAuthority(context.Background(), "sha256:"+strings.Repeat("e", 64), "", time.Now().UTC()); err == nil {
		t.Fatal("an empty package identity resolved")
	}
	if _, err := a.repo.ResolvePackagePublishAuthority(context.Background(), "sha256:"+strings.Repeat("e", 64), "other.namespace.pkg", time.Now().UTC().Add(time.Second)); err == nil {
		t.Fatal("a package outside the delegated namespace resolved")
	}
}

// LoadCurrentAuthorityGeneration is the loader the CLI uses to trust a delegating
// parent: it returns a current generation and refuses one that is retired, and a
// generation whose liveness record is gone.
func TestLoadCurrentAuthorityGenerationRefusesARetiredGeneration(t *testing.T) {
	a, root, child, _, _ := publishDelegationFixture(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(time.Second)
	if got, err := a.repo.LoadCurrentAuthorityGeneration(ctx, child.Ref, child.Version, at); err != nil || got.Digest != child.Digest {
		t.Fatalf("control: a current generation must load: %v", err)
	}
	retirePublishChild(t, a, child, root)
	if _, err := a.repo.LoadCurrentAuthorityGeneration(ctx, child.Ref, child.Version, at); err == nil {
		t.Fatal("a retired generation loaded as current")
	}
	if got, err := a.repo.LoadAuthorityGeneration(ctx, child.Ref, child.Version, at); err != nil || got.Digest != child.Digest {
		t.Fatalf("the immutable record must remain readable as evidence: %v", err)
	}
	other, err := a.repo.LoadCurrentAuthorityGeneration(ctx, root.Ref, root.Version, at)
	if err != nil || other.Digest != root.Digest {
		t.Fatalf("the still-current root must load: %v", err)
	}
	keylessDelete(t, a.store, state.AuthorityGenerationLiveNamespace, root.Ref, root.Version)
	if _, err := a.repo.LoadCurrentAuthorityGeneration(ctx, root.Ref, root.Version, at); err == nil {
		t.Fatal("a generation without its liveness record loaded as current")
	}
}

// The enumerating form itself refuses a store that is not current against the
// forward authority anchor, independent of any later layer of the resolver.
func TestCurrentGenerationListingRefusesAStoreBehindTheAnchor(t *testing.T) {
	a, _, child, _, _ := publishDelegationFixture(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(time.Second)
	if listed, err := a.repo.ListCurrentAuthorityGenerations(ctx, at); err != nil || len(listed) < 2 {
		t.Fatalf("control: a current store lists its current generations: %d %v", len(listed), err)
	} else {
		found := false
		for _, g := range listed {
			found = found || g.Digest == child.Digest
		}
		if !found {
			t.Fatal("control: the current child must be listed")
		}
	}
	cur, err := a.anchor.Load(ctx, successionTestBootstrap)
	if err != nil {
		t.Fatal(err)
	}
	next := cur
	next.Seq++
	next.Head = "sha256:" + strings.Repeat("9", 64)
	if err := a.anchor.Set(ctx, successionTestBootstrap, &cur, next); err != nil {
		t.Fatal(err)
	}
	if listed, err := a.repo.ListCurrentAuthorityGenerations(ctx, at); err == nil {
		t.Fatalf("a store behind the anchor listed %d current generations", len(listed))
	}
}

// Revoking the delegating DECISION (not the generation) stops resolution too.
func TestRevokedParentDecisionDoesNotResolvePackagePublishAuthority(t *testing.T) {
	a, _, child, publisher, _ := publishDelegationFixture(t)
	ctx := context.Background()
	requestID, requestVersion, _ := strings.Cut(child.DelegationRef, "/")
	decision, err := a.repo.LoadAuthorityDecision(ctx, requestID, requestVersion, time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	revocation := contracts.AuthorityRevocation{RequestID: requestID, RequestVersion: requestVersion, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: digest, RevocationRef: "revoke-" + requestID, RevocationVersion: "1", RevokedBy: decision.DecidedBy, AuthorityDigest: "sha256:" + strings.Repeat("a", 64), EffectiveAt: time.Now().UTC(), Reason: "withdrawn"}
	if err := a.repo.SaveAuthorityRevocation(ctx, requestID, requestVersion, revocation, time.Now().UTC(), nil); err != nil {
		t.Fatalf("revoke the delegating decision: %v", err)
	}
	if _, err := resolveAt(a, publisher); err == nil {
		t.Fatal("a generation whose delegating decision was revoked still resolves package.publish authority")
	}
}

// R7.09j: a generation whose Scope is not the "package-namespace:" form at all
// (for example the bare package identity) must be refused, not accepted because
// its "namespace" (the whole malformed scope) happens to equal the package ID.
func TestResolverRefusesAGenerationWhoseScopeIsNotThePackageNamespaceForm(t *testing.T) {
	a, root, child, publisher, _ := publishDelegationFixture(t)
	variant := child
	variant.Ref = child.Ref + "-bare-scope"
	variant.Scope = "praxis.package.goals" // exactly the package identity, no prefix
	var err error
	if variant.Digest, err = variant.ComputeDigest(); err != nil {
		t.Fatal(err)
	}
	if err := a.repo.SaveAuthorityGeneration(context.Background(), variant, variant.EffectiveAt, nil); err != nil {
		t.Skipf("the store refuses this scope form (%v): unreachable through the store", err)
	}
	retirePublishChild(t, a, child, root)
	if got, err := resolveAt(a, publisher); err == nil {
		t.Fatalf("a generation whose scope was the bare package identity resolved: %+v", got.Generation)
	}
}
