package goalspublication

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// N17 equivalent path: the abandonment owner check matched ANY root-shaped
// generation carrying the OS user, whether or not that generation was still in
// force (for example a former owner's superseded root). A retired root
// authenticates nobody. The publication's own root is separately checked by the
// predecessor lineage; this isolates the owner authentication.
func TestRetiredRootShapedGenerationCannotAuthenticateTheAbandoningOwner(t *testing.T) {
	r, f, at, q, _, _ := predecessorExecutionFixture(t)
	ctx := context.Background()
	e := Execution{Repository: r, Adapter: &fakeAdapter{}, Now: func() time.Time { return at }}
	root := rootOf(f)
	// A former owner's root: same principal, another OS user, later retired.
	former := root
	former.Ref = "installation-governance:former-owner"
	former.ProvenanceRef = strings.Split(root.ProvenanceRef, ":os-user:")[0] + ":os-user:former-owner"
	var err error
	if former.Digest, err = former.ComputeDigest(); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveAuthorityGeneration(ctx, former, former.EffectiveAt, nil); err != nil {
		t.Skipf("the store refuses a second root-shaped generation here (%v); the owner-loop gap needs a succession fixture", err)
	}
	if _, _, err := e.PrepareAbandonment(ctx, q.ID, "former-owner", "control"); err != nil {
		t.Fatalf("premise: while its generation is in force the OS user authenticates: %v", err)
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: former.Ref, Version: former.Version, GenerationDigest: former.Digest, InvalidationRef: "retire-former-owner", InvalidationVersion: "1", Kind: "superseded", SupersededBy: "2", InvalidatedBy: former.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
	if err := r.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.PrepareAbandonment(ctx, q.ID, "former-owner", "after retirement"); err == nil {
		t.Fatal("a retired root-shaped generation still authenticated the abandoning OS user")
	}
}

// The owner authentication both abandonment paths share (abandon.go and
// recovery_abandon.go call it), tested directly so it does not depend on the
// externally supplied signed artifacts the recovery fixture needs.
func TestOwnerOSUserAuthenticationUsesOnlyTheCurrentRoot(t *testing.T) {
	r, f, at, _, _, _ := predecessorExecutionFixture(t)
	ctx := context.Background()
	root := rootOf(f)
	user := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	if ok, err := requireCurrentOwnerOSUser(ctx, r, at, user); err != nil || !ok {
		t.Fatalf("control: the current owner's OS user must authenticate: %v %v", ok, err)
	}
	if ok, err := requireCurrentOwnerOSUser(ctx, r, at, "someone-else"); err != nil || ok {
		t.Fatalf("another OS user authenticated: %v %v", ok, err)
	}
	former := root
	former.Ref = "installation-governance:former-owner"
	former.ProvenanceRef = strings.Split(root.ProvenanceRef, ":os-user:")[0] + ":os-user:former-owner"
	var err error
	if former.Digest, err = former.ComputeDigest(); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveAuthorityGeneration(ctx, former, former.EffectiveAt, nil); err != nil {
		t.Fatalf("the store must accept the former owner's root: %v", err)
	}
	if ok, err := requireCurrentOwnerOSUser(ctx, r, at, "former-owner"); err != nil || !ok {
		t.Fatalf("premise: while its generation is in force the OS user authenticates: %v %v", ok, err)
	}
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: former.Ref, Version: former.Version, GenerationDigest: former.Digest, InvalidationRef: "retire-former-owner", InvalidationVersion: "1", Kind: "superseded", SupersededBy: "2", InvalidatedBy: former.Principal, EffectiveAt: time.Now().UTC(), Reason: "test retirement"}
	if err := r.SaveAuthorityGenerationInvalidation(ctx, invalidation, invalidation.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := requireCurrentOwnerOSUser(ctx, r, at.Add(time.Second), "former-owner"); err != nil || ok {
		t.Fatalf("a retired root-shaped generation still authenticated the OS user: %v %v", ok, err)
	}
	// A principal that is not the owner never authenticates, however current.
	stranger := root
	stranger.Ref = "installation-governance:stranger"
	stranger.Principal = contracts.PrincipalRef{ID: "installation-owner:stranger", Kind: "human"}
	stranger.ProvenanceRef = strings.Split(root.ProvenanceRef, ":os-user:")[0] + ":os-user:stranger"
	if stranger.Digest, err = stranger.ComputeDigest(); err != nil {
		t.Logf("a root of another principal cannot carry a valid digest here (%v): unreachable", err)
	} else if err := r.SaveAuthorityGeneration(ctx, stranger, stranger.EffectiveAt, nil); err != nil {
		t.Logf("the store refuses a root of another principal (%v): unreachable", err)
	} else if ok, _ := requireCurrentOwnerOSUser(ctx, r, at, "stranger"); ok {
		t.Fatal("a root of another principal authenticated as the installation owner")
	}
	// A generation that has a parent is not a root and never authenticates the owner.
	child := root
	child.Ref = "installation-governance:child"
	child.ParentRef, child.ParentVersion, child.ParentDigest = root.Ref, root.Version, root.Digest
	child.ProvenanceRef = strings.Split(root.ProvenanceRef, ":os-user:")[0] + ":os-user:child-user"
	if child.Digest, err = child.ComputeDigest(); err != nil {
		t.Logf("a rooted-principal child cannot carry a valid digest here (%v): unreachable", err)
	} else if err := r.SaveAuthorityGeneration(ctx, child, child.EffectiveAt, nil); err != nil {
		t.Logf("the store refuses a parented generation for the owner principal (%v): unreachable", err)
	} else if ok, _ := requireCurrentOwnerOSUser(ctx, r, at, "child-user"); ok {
		t.Fatal("a non-root generation authenticated as the installation owner")
	}
}
