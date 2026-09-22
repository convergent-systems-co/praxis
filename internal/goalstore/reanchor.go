package goalstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Governed re-anchoring (backup restore and anchor recovery).
//
// Restoring an older governance database, losing or resetting the anchor, or an
// interrupted anchored write all present the same way: the store and the anchor
// disagree, and every governance predicate refuses. That refusal is the safety
// property, so the only way out is an explicit, owner-authenticated operation
// whose effect is deliberately narrow:
//
//   - it appends ONE reanchor fact bridging exactly from the verified chain to
//     the anchor's sequence, so the incident is durable and inspectable;
//   - it VOIDS every decision and generation admitted before it (their liveness
//     stamps are below the fact's sequence), so nothing retired in the lost
//     interval can become current merely because a backup contains it;
//   - it RE-ADMITS exactly one thing: the installation root generation the owner
//     attested, which is the installation owner's own authority derived from the
//     enrollment identity, not a new grant;
//   - it may ADD classifications the owner names (restrictions only).
//
// It cannot grant a decision, approve a gate, broaden a Goal or WorkPlan, or
// recreate any retired authority. Everything else must be established again
// through the ordinary ceremonies.

// ReanchorPlan is what the owner is shown and confirms, digest-bound.
type ReanchorPlan struct {
	Installation string
	Cause        string
	AnchorSeq    uint64
	AnchorHead   string
	StoreSeq     uint64
	StoreHead    string
	LostFacts    uint64 // facts the anchor knows and the store lacks (0 unless behind)
	OrphanFacts  int
	RootRef      string
	RootVersion  string
	RootDigest   string
	RootUser     string
	Consistent   bool // nothing to re-anchor; only a pending root re-admission may remain
}

// Digest binds the confirmation to exactly this plan.
func (p ReanchorPlan) Digest() string {
	body, _ := json.Marshal(p)
	sum := sha256.Sum256(append([]byte("praxis-reanchor-plan/v1\n"), body...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// chainPrefix authenticates the fact rows one by one and returns the longest
// valid prefix, the rows that follow it (orphans) and the prefix's view.
func (r Repository) chainPrefix(ctx context.Context, records []state.SecureBlobRecord) (faa.View, []state.SecureBlobRecord, error) {
	sort.SliceStable(records, func(i, j int) bool { return records[i].ObjectID < records[j].ObjectID })
	var valid []faa.Fact
	cut := len(records)
	for i, record := range records {
		facts, err := r.openFacts(ctx, []state.SecureBlobRecord{record})
		if err != nil {
			cut = i
			break
		}
		candidate := append(append([]faa.Fact(nil), valid...), facts[0])
		if _, err := faa.VerifyChain(r.InstallationDigest, candidate); err != nil {
			cut = i
			break
		}
		valid = candidate
	}
	view, err := faa.VerifyChain(r.InstallationDigest, valid)
	if err != nil {
		return faa.View{}, nil, err
	}
	return view, records[cut:], nil
}

// installationRootCandidate finds the one installation root generation that
// the store's rows and the valid fact prefix leave un-retired. Liveness is NOT
// consulted: after a restore it is exactly what cannot be trusted.
func (r Repository) installationRootCandidate(ctx context.Context, tx *sql.Tx, view faa.View) (contracts.AuthorityGeneration, error) {
	owner, err := contracts.InstallationOwnerPrincipal(r.InstallationDigest)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	scope, err := contracts.InstallationGovernanceScope(r.InstallationDigest)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	var generations []contracts.AuthorityGeneration
	var invalidated map[string]bool
	if tx != nil {
		all, err := state.ListSecureBlobsInNamespacesTx(ctx, tx, []string{authorityGenerationNamespace, authorityGenerationInvalidationNamespace, state.AuthorityGenerationLiveNamespace}, time.Now().UTC())
		if err != nil {
			return contracts.AuthorityGeneration{}, err
		}
		generations, invalidated, _, err = r.decodeGenerationSnapshot(ctx, all)
		if err != nil {
			return contracts.AuthorityGeneration{}, err
		}
	} else {
		generations, invalidated, _, err = r.listAuthorityGenerationsSnapshot(ctx, time.Now().UTC())
		if err != nil {
			return contracts.AuthorityGeneration{}, err
		}
	}
	var candidates []contracts.AuthorityGeneration
	for _, g := range generations {
		if g.Ref != scope || g.Principal != owner || g.ParentRef != "" || g.DelegatedBy != (contracts.PrincipalRef{}) || g.ProvenanceDigest != r.InstallationDigest || g.Scope != scope {
			continue
		}
		if invalidated[g.Ref+"@"+g.Version] {
			continue
		}
		if _, retired := view.RetiredGenerations[faa.GenerationKey(g.Ref, g.Version, g.Digest)]; retired {
			continue
		}
		candidates = append(candidates, g)
	}
	if len(candidates) != 1 {
		return contracts.AuthorityGeneration{}, fmt.Errorf("re-anchoring needs exactly one un-retired installation root in the store, found %d; resolve root succession first", len(candidates))
	}
	return candidates[0], nil
}

// PlanReanchor computes what a re-anchor would do without changing anything.
func (r Repository) PlanReanchor(ctx context.Context) (ReanchorPlan, error) {
	if !r.anchorEnabled() {
		return ReanchorPlan{}, errors.New("this repository has no forward authority anchor")
	}
	records, err := r.Store.ListGovernanceFactRecords(ctx)
	if err != nil {
		return ReanchorPlan{}, err
	}
	return r.planFrom(ctx, nil, records)
}

func (r Repository) planFrom(ctx context.Context, tx *sql.Tx, records []state.SecureBlobRecord) (ReanchorPlan, error) {
	view, orphans, err := r.chainPrefix(ctx, records)
	if err != nil {
		return ReanchorPlan{}, err
	}
	plan := ReanchorPlan{Installation: r.InstallationDigest, StoreSeq: view.Seq, StoreHead: view.Head, OrphanFacts: len(orphans)}
	anchor, anchorErr := r.FAA.Load(ctx, r.InstallationDigest)
	switch {
	case anchorErr == nil:
		plan.AnchorSeq, plan.AnchorHead = anchor.Seq, anchor.Head
		switch faa.Compare(view, anchor) {
		case faa.Consistent:
			plan.Cause, plan.Consistent = "consistent", len(orphans) == 0
		case faa.Behind:
			plan.Cause, plan.LostFacts = "behind", anchor.Seq-view.Seq
		case faa.Ahead:
			plan.Cause = "ahead"
		default:
			plan.Cause = "unrelated"
		}
		if len(orphans) > 0 && plan.Cause == "consistent" {
			plan.Cause = "orphaned"
		}
	case errors.Is(anchorErr, faa.ErrMissing):
		plan.Cause = "missing"
	case errors.Is(anchorErr, faa.ErrCorrupt):
		plan.Cause = "unreadable"
	default:
		return ReanchorPlan{}, fmt.Errorf("%w: %v", ErrGovernanceAnchorUnavailable, anchorErr)
	}
	root, err := r.installationRootCandidate(ctx, tx, view)
	if err != nil {
		return ReanchorPlan{}, err
	}
	plan.RootRef, plan.RootVersion, plan.RootDigest = root.Ref, root.Version, root.Digest
	if i := lastIndex(root.ProvenanceRef, ":os-user:"); i >= 0 {
		plan.RootUser = root.ProvenanceRef[i+len(":os-user:"):]
	}
	return plan, nil
}

func lastIndex(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ReanchorRequest carries the owner's authentication. The caller (the CLI)
// has verified the interactive terminal, the authenticated OS user against the
// root's enrollment user, and the digest-bound typed confirmation.
type ReanchorRequest struct {
	PlanDigest      string
	OSUser          string
	CeremonyDigest  string
	ClassifiedGoals []string
	Now             time.Time
}

// Reanchor performs the governed re-anchor. The plan is recomputed inside the
// store's write lock and must still match the digest the owner confirmed.
func (r Repository) Reanchor(ctx context.Context, req ReanchorRequest) (faa.Fact, error) {
	if !r.anchorEnabled() {
		return faa.Fact{}, errors.New("this repository has no forward authority anchor")
	}
	if err := mustNotBeBlank(req.PlanDigest, req.OSUser, req.CeremonyDigest); err != nil {
		return faa.Fact{}, err
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	goals := append([]string(nil), req.ClassifiedGoals...)
	sort.Strings(goals)
	var written faa.Fact
	err := r.Store.AppendGovernanceFact(ctx, func(ctx context.Context, tx *sql.Tx, existing []state.SecureBlobRecord) (state.SecureBlobRecord, func(context.Context), error) {
		plan, err := r.planFrom(ctx, tx, existing)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		if plan.Digest() != req.PlanDigest {
			return state.SecureBlobRecord{}, nil, errors.New("the store or the anchor changed since the owner confirmed this re-anchor; nothing was written")
		}
		if plan.Consistent {
			return state.SecureBlobRecord{}, nil, nil
		}
		view, orphans, err := r.chainPrefix(ctx, existing)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		fact := faa.Fact{
			Seq: maxUint(view.Seq, plan.AnchorSeq) + 1, Kind: faa.KindReanchor, Installation: r.InstallationDigest, Prev: view.Head, At: req.Now.UTC(),
			Reanchor: &faa.Reanchor{Cause: plan.Cause, DBSeq: view.Seq, DBHead: view.Head, AnchorSeq: plan.AnchorSeq, AnchorHead: plan.AnchorHead, RootRef: plan.RootRef, RootVersion: plan.RootVersion, RootDigest: plan.RootDigest, OSUser: req.OSUser, CeremonyDigest: req.CeremonyDigest, ClassifiedGoals: goals},
		}
		fact, err = faa.Seal(fact)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		payload, err := json.Marshal(fact)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		record, err := r.workPlanSecureRecord(ctx, state.GovernanceFactNamespace, state.GovernanceFactID(fact.Seq), "1", payload, req.Now, nil)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		// Fact rows beyond the last valid link are preserved, out of the chain: an
		// authentic one is re-sealed under the orphan namespace, an unauthentic one
		// is replaced by a sealed marker naming its identity and digest.
		for _, orphan := range orphans {
			var body []byte
			if payload, openErr := r.Crypto.Open(ctx, orphan.Envelope, state.SecureBlobAAD(orphan.Namespace, orphan.ObjectID, orphan.ObjectVersion, orphan.ObjectDigest)); openErr == nil && payloadDigest(payload) == orphan.ObjectDigest {
				body = payload
			} else {
				body, _ = json.Marshal(map[string]string{"object_id": orphan.ObjectID, "object_digest": orphan.ObjectDigest, "state": "unauthentic when preserved"})
			}
			preserved, err := r.workPlanSecureRecord(ctx, governanceOrphanNamespace, orphan.ObjectID, orphan.ObjectVersion, body, req.Now, nil)
			if err != nil {
				return state.SecureBlobRecord{}, nil, err
			}
			if err := state.DeleteSecureBlobInTx(ctx, tx, orphan.Namespace, orphan.ObjectID, orphan.ObjectVersion); err != nil {
				return state.SecureBlobRecord{}, nil, fmt.Errorf("move orphaned governance fact: %w", err)
			}
			if err := state.InsertSecureBlobInTx(ctx, tx, preserved); err != nil {
				return state.SecureBlobRecord{}, nil, fmt.Errorf("preserve orphaned governance fact: %w", err)
			}
		}
		next := faa.State{Installation: r.InstallationDigest, Seq: fact.Seq, Head: fact.Head}
		var undo func(context.Context)
		switch plan.Cause {
		case "missing":
			if err := r.FAA.Set(ctx, r.InstallationDigest, nil, next); err != nil {
				return state.SecureBlobRecord{}, nil, r.classifyAnchorError(err)
			}
			undo = nil
		case "unreadable":
			if err := r.FAA.Reset(ctx, r.InstallationDigest, next); err != nil {
				return state.SecureBlobRecord{}, nil, r.classifyAnchorError(err)
			}
		default:
			cur := faa.State{Installation: r.InstallationDigest, Seq: plan.AnchorSeq, Head: plan.AnchorHead}
			if err := r.FAA.Set(ctx, r.InstallationDigest, &cur, next); err != nil {
				return state.SecureBlobRecord{}, nil, r.classifyAnchorError(err)
			}
			undo = func(ctx context.Context) { _ = r.FAA.Revert(ctx, r.InstallationDigest, next, cur) }
		}
		written = fact
		return record, undo, nil
	})
	if err != nil {
		return faa.Fact{}, err
	}
	if written.Seq == 0 {
		// Already consistent: only a pending root re-admission may remain.
		if err := r.completeRootReadmission(ctx, req.Now); err != nil {
			return faa.Fact{}, err
		}
		return faa.Fact{}, nil
	}
	if err := r.completeRootReadmission(ctx, req.Now); err != nil {
		return written, fmt.Errorf("the re-anchor is durable but the root re-admission is incomplete; run the re-anchor again to complete it: %w", err)
	}
	return written, nil
}

func maxUint(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// completeRootReadmission re-stamps the liveness record of the root the latest
// reanchor fact attested, so the installation owner's own authority is current
// again. It is idempotent and writes only what the owner-authenticated fact
// already names.
func (r Repository) completeRootReadmission(ctx context.Context, now time.Time) error {
	snap, err := r.governanceSnapshot(ctx)
	if err != nil {
		return err
	}
	if len(snap.View.Reanchors) == 0 {
		return nil
	}
	last := snap.View.Reanchors[len(snap.View.Reanchors)-1]
	ra := last.Reanchor
	root, err := r.LoadAuthorityGeneration(ctx, ra.RootRef, ra.RootVersion, now)
	if err != nil {
		return fmt.Errorf("load attested root: %w", err)
	}
	if root.Digest != ra.RootDigest {
		return errors.New("the stored root differs from the root the re-anchor attested")
	}
	if _, retired := snap.View.RetiredGenerations[faa.GenerationKey(root.Ref, root.Version, root.Digest)]; retired {
		return errors.New("the attested root is retired by a later fact and is not re-admitted")
	}
	// An invalidation record is a negative fact too: an attested root that carries
	// one is never re-stamped live (the candidate derivation already excludes it).
	if _, _, invErr := r.loadWorkPlanBlob(ctx, authorityGenerationInvalidationNamespace, root.Ref, root.Version, now); invErr == nil {
		return errors.New("the attested root carries an invalidation record and is not re-admitted")
	} else if !errors.Is(invErr, state.ErrSecureBlobNotFound) && !errors.Is(invErr, state.ErrSecureBlobExpired) {
		return invErr
	}
	if stored, err := r.loadLiveness(ctx, state.AuthorityGenerationLiveNamespace, root.Ref, root.Version, now); err == nil && stored.Digest == root.Digest && stored.AdmittedAtSeq >= last.Seq {
		return nil
	}
	record, err := state.SealedLivenessRecord(ctx, r.Crypto, r.KeyRef, r.Profile, r.Sensitivity, state.AuthorityGenerationLiveNamespace, root.Ref, root.Version, root.Digest, now, last.Seq)
	if err != nil {
		return err
	}
	return r.Store.ReplaceLivenessRecord(ctx, record)
}
