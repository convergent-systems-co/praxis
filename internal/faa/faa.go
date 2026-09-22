// Package faa is the Forward Authority Anchor: minimal, forward-only
// governance freshness state that lives OUTSIDE the mutable governance store
// (the SQLite file, the rollback domain).
//
// Why it exists (I13 temporal authority, I14 subject-governance continuity).
// Authenticated rows prove origin and integrity, not currency. A writer of the
// database file can restore any earlier state of it, and every predicate that
// reads only that file evaluates the restored state exactly as it did then. The
// one thing such a writer cannot roll back is state that is not in the file.
// The FAA holds the pair (Seq, Head): the length and the hash-chain head of the
// GOVERNANCE FACT CHAIN. Facts are the transitions whose loss or replay would
// broaden permission (retirements, classification, re-anchoring); each is an
// authenticated row chained to its predecessor. A store is consumable only when
// its verified chain ends exactly at the anchor.
//
// What the FAA is not. It grants no authority, records no owner decision,
// authorises nothing and cannot recreate anything: it can only make a store
// that is BEHIND, AHEAD OF or UNRELATED TO the anchor refuse to be consumed. Its
// contents are a counter and a digest.
package faa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	// ErrMissing: the anchor holds no state for this installation.
	ErrMissing = errors.New("forward authority anchor has no state for this installation")
	// ErrUnavailable: the anchor backend cannot be read or written.
	ErrUnavailable = errors.New("forward authority anchor is unavailable")
	// ErrCorrupt: the anchor state exists but is unreadable or malformed.
	ErrCorrupt = errors.New("forward authority anchor state is corrupt")
	// ErrConflict: a compare-and-set found the anchor different from expected,
	// or the requested state is not strictly forward.
	ErrConflict = errors.New("forward authority anchor compare-and-set conflict")
)

// State is everything the anchor stores.
type State struct {
	Installation string `json:"installation"`
	Seq          uint64 `json:"seq"`
	Head         string `json:"head"`
}

func (s State) Validate() error {
	if s.Installation == "" || !strings.HasPrefix(s.Head, "sha256:") || len(s.Head) != len("sha256:")+64 {
		return fmt.Errorf("%w: incomplete state", ErrCorrupt)
	}
	return nil
}

// Anchor is a forward-only cell. Set is compare-and-set: expect==nil means the
// cell must not exist. The next state must be strictly forward (higher Seq) and
// bound to the same installation. Implementations must read back what they wrote
// and report ErrUnavailable/ErrCorrupt rather than success if it differs.
// Callers serialise writers with the governance store's write lock.
type Anchor interface {
	Load(ctx context.Context, installation string) (State, error)
	Set(ctx context.Context, installation string, expect *State, next State) error
	// Revert undoes the single most recent Set of THIS caller after its own
	// database commit failed. It succeeds only if the cell still equals from.
	Revert(ctx context.Context, installation string, from, to State) error
	// Reset overwrites the cell unconditionally (creating it if absent or
	// replacing unreadable content). Only a governed re-anchor calls it, and it
	// still refuses a state that is not bound to the installation.
	Reset(ctx context.Context, installation string, next State) error
}

// NewGenesis creates the first fact of an installation's chain and the anchor
// state that pins it. The nonce is random and known only to the sealed fact and
// the anchor, so an anchor state at genesis cannot be computed from public
// information (the installation digest): a writer of the anchor who cannot read
// it cannot set it to any earlier valid value, including the empty chain.
func NewGenesis(installation, nonce string, at time.Time) (Fact, State, error) {
	if installation == "" || nonce == "" {
		return Fact{}, State{}, errors.New("a genesis needs an installation and a nonce")
	}
	f, err := Seal(Fact{Seq: 0, Kind: KindGenesis, Installation: installation, Prev: "", At: at.UTC(), Nonce: nonce})
	if err != nil {
		return Fact{}, State{}, err
	}
	return f, State{Installation: installation, Seq: 0, Head: f.Head}, nil
}

// Fact kinds.
const (
	KindGenesis         = "genesis"
	KindDecisionRetired   = "decision_retired"
	KindGenerationRetired = "generation_retired"
	KindGoalClassified    = "goal_classified"
	KindReanchor          = "reanchor"
)

// Fact is one governance transition whose loss or replay would broaden
// permission. It carries no authority of its own.
type Fact struct {
	Seq          uint64    `json:"seq"`
	Kind         string    `json:"kind"`
	Installation string    `json:"installation"`
	Prev         string    `json:"prev"`
	At           time.Time `json:"at"`

	// decision_retired
	RequestID      string `json:"request_id,omitempty"`
	RequestVersion string `json:"request_version,omitempty"`
	DecisionDigest string `json:"decision_digest,omitempty"`
	// generation_retired
	GenerationRef     string `json:"generation_ref,omitempty"`
	GenerationVersion string `json:"generation_version,omitempty"`
	GenerationDigest  string `json:"generation_digest,omitempty"`
	Reason            string `json:"reason,omitempty"`
	// goal_classified
	GoalID        string `json:"goal_id,omitempty"`
	KernelVersion string `json:"kernel_version,omitempty"`
	// genesis
	Nonce string `json:"nonce,omitempty"`
	// reanchor
	Reanchor *Reanchor `json:"reanchor,omitempty"`

	Head string `json:"head"`
}

// Reanchor is the durable, inspectable provenance of a governed re-anchor. It
// never creates authority: it declares that every admission stamped before it is
// void, re-admits only the installation root the owner attested, and may add
// classifications (restrictions) the owner names.
type Reanchor struct {
	Cause           string   `json:"cause"` // behind | ahead | missing | unreadable | unrelated
	DBSeq           uint64   `json:"db_seq"`
	DBHead          string   `json:"db_head"`
	AnchorSeq       uint64   `json:"anchor_seq"`
	AnchorHead      string   `json:"anchor_head,omitempty"`
	RootRef         string   `json:"root_ref"`
	RootVersion     string   `json:"root_version"`
	RootDigest      string   `json:"root_digest"`
	OSUser          string   `json:"os_user"`
	CeremonyDigest  string   `json:"ceremony_digest"`
	ClassifiedGoals []string `json:"classified_goals,omitempty"`
}

// HeadOf computes the chain head of a fact whose Head field is ignored.
func HeadOf(f Fact) (string, error) {
	f.Head = ""
	body, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte("praxis-faa-fact/v1\n" + f.Prev + "\n" + string(body)))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// Seal fills Head.
func Seal(f Fact) (Fact, error) {
	head, err := HeadOf(f)
	if err != nil {
		return Fact{}, err
	}
	f.Head = head
	return f, nil
}

// RetiredKey identifies a decision or generation in the retirement sets.
func DecisionKey(requestID, requestVersion, digest string) string {
	return requestID + "\x00" + requestVersion + "\x00" + digest
}
func GenerationKey(ref, version, digest string) string {
	return ref + "\x00" + version + "\x00" + digest
}

// View is the verified result of a fact chain.
type View struct {
	Seq                uint64
	Head               string
	RetiredDecisions   map[string]uint64 // key -> seq of the retiring fact
	RetiredGenerations map[string]uint64
	Classified         map[string]string // goal -> kernel version
	ReanchorSeq        uint64            // seq of the latest reanchor fact, 0 if none
	Reanchors          []Fact
}

// ErrChain is returned for every structural chain failure. It is always a
// refusal: a chain that cannot be verified never reads as "no facts".
var ErrChain = errors.New("governance fact chain is not a valid continuous chain from genesis")

// VerifyChain checks facts (already authenticated by the caller) form one
// continuous chain from the installation's genesis, and returns the resulting
// view. Gaps, reordering, forks, foreign installations and altered facts are
// errors. Only a reanchor fact may skip sequence numbers, and only by declaring
// exactly the state of the chain it bridges from.
func VerifyChain(installation string, facts []Fact) (View, error) {
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].Seq < facts[j].Seq })
	view := View{Head: "", RetiredDecisions: map[string]uint64{}, RetiredGenerations: map[string]uint64{}, Classified: map[string]string{}}
	for _, f := range facts {
		if f.Installation != installation {
			return View{}, fmt.Errorf("%w: fact %d belongs to another installation", ErrChain, f.Seq)
		}
		if f.Prev != view.Head {
			return View{}, fmt.Errorf("%w: fact %d does not chain to its predecessor", ErrChain, f.Seq)
		}
		want, err := HeadOf(f)
		if err != nil || want != f.Head {
			return View{}, fmt.Errorf("%w: fact %d head does not match its content", ErrChain, f.Seq)
		}
		first := len(facts) > 0 && f.Seq == facts[0].Seq && view.Head == ""
		if first && f.Kind != KindGenesis && f.Kind != KindReanchor {
			return View{}, fmt.Errorf("%w: the chain must begin with a genesis or a re-anchor, not %q", ErrChain, f.Kind)
		}
		switch f.Kind {
		case KindGenesis:
			if !first || f.Seq != 0 || f.Nonce == "" {
				return View{}, fmt.Errorf("%w: a genesis is only valid as the first fact at sequence 0", ErrChain)
			}
		case KindReanchor:
			if f.Seq <= view.Seq || f.Reanchor == nil || f.Reanchor.DBSeq != view.Seq || f.Reanchor.DBHead != view.Head {
				return View{}, fmt.Errorf("%w: reanchor fact %d does not bridge exactly from the chain it follows", ErrChain, f.Seq)
			}
			view.ReanchorSeq = f.Seq
			view.Reanchors = append(view.Reanchors, f)
			for _, goal := range f.Reanchor.ClassifiedGoals {
				if _, ok := view.Classified[goal]; !ok {
					view.Classified[goal] = ""
				}
			}
		default:
			if f.Seq != view.Seq+1 {
				return View{}, fmt.Errorf("%w: fact %d follows %d without a re-anchor", ErrChain, f.Seq, view.Seq)
			}
			switch f.Kind {
			case KindDecisionRetired:
				if f.RequestID == "" || f.RequestVersion == "" || f.DecisionDigest == "" {
					return View{}, fmt.Errorf("%w: fact %d is incomplete", ErrChain, f.Seq)
				}
				view.RetiredDecisions[DecisionKey(f.RequestID, f.RequestVersion, f.DecisionDigest)] = f.Seq
			case KindGenerationRetired:
				if f.GenerationRef == "" || f.GenerationVersion == "" || f.GenerationDigest == "" {
					return View{}, fmt.Errorf("%w: fact %d is incomplete", ErrChain, f.Seq)
				}
				view.RetiredGenerations[GenerationKey(f.GenerationRef, f.GenerationVersion, f.GenerationDigest)] = f.Seq
			case KindGoalClassified:
				if f.GoalID == "" || f.KernelVersion == "" {
					return View{}, fmt.Errorf("%w: fact %d is incomplete", ErrChain, f.Seq)
				}
				view.Classified[f.GoalID] = f.KernelVersion
			default:
				return View{}, fmt.Errorf("%w: fact %d has unknown kind %q", ErrChain, f.Seq, f.Kind)
			}
		}
		view.Seq, view.Head = f.Seq, f.Head
	}
	return view, nil
}

// Relation classifies a verified chain against the anchor.
type Relation string

const (
	Consistent Relation = "consistent"
	Behind     Relation = "behind"    // the store lost transitions the anchor knows about (rollback / truncation / erasure)
	Ahead      Relation = "ahead"     // the store has transitions the anchor never saw (anchor reset or an interrupted write)
	Unrelated  Relation = "unrelated" // same length, different head (fork or substitution)
)

// Compare relates a verified chain to the anchor state.
func Compare(view View, anchor State) Relation {
	switch {
	case view.Seq == anchor.Seq && view.Head == anchor.Head:
		return Consistent
	case view.Seq < anchor.Seq:
		return Behind
	case view.Seq > anchor.Seq:
		return Ahead
	default:
		return Unrelated
	}
}
