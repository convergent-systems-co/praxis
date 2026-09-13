package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	portableRecordAddedEvent = "portable.record_added"
	portableEnvelopeImported = "portable.envelope_imported"
)

var ErrUnresolvedConflict = errors.New("portable state has unresolved competing heads")

type ImportAuthority interface {
	AuthorizeImport(ctx context.Context, destinationInstallation, envelopeID string, source contracts.PrincipalRef) (contracts.PrincipalRef, error)
}

type Ledger struct {
	store     eventstore.Store
	install   string
	authority ImportAuthority
}

type importPayload struct {
	Envelope  StateEnvelope          `json:"envelope"`
	Result    ImportResult           `json:"result"`
	Authority contracts.PrincipalRef `json:"authority"`
}

type ImportResult struct {
	EnvelopeID string     `json:"envelope_id"`
	AddedIDs   []string   `json:"added_ids"`
	StaleIDs   []string   `json:"stale_ids,omitempty"`
	Conflicts  []Conflict `json:"conflicts,omitempty"`
}

type State struct {
	Records         []PortableRecord
	Conflicts       []Conflict
	ImportedIDs     []string
	SourceSequences map[string]int64
}

func NewLedger(store eventstore.Store, installationID string, authority ImportAuthority) (*Ledger, error) {
	if store == nil || installationID == "" || authority == nil {
		return nil, errors.New("portable ledger requires event store, installation identity, and import authority")
	}
	return &Ledger{store: store, install: installationID, authority: authority}, nil
}

func portableAggregate(installationID string) string { return "portable-state:" + installationID }

func (l *Ledger) RecordLocal(ctx context.Context, record PortableRecord) error {
	if err := VerifyRecord(record); err != nil {
		return err
	}
	if record.SourceInstallation != l.install {
		return errors.New("local portable record source does not match ledger installation")
	}
	events, state, err := l.load(ctx)
	if err != nil {
		return err
	}
	if containsRecord(state.Records, record.ID) {
		return nil
	}
	if err := validateParentAvailability(record, state.Records, nil); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, portableAggregate(l.install), int64(len(events)), []eventstore.Event{{ID: "event:" + record.ID, AggregateType: "portable_state", Type: portableRecordAddedEvent, Version: portableEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: l.install, Kind: "installation"}, CommandID: "portable-record:" + record.ID, CorrelationID: record.EntityID, CausationID: firstParent(record), Trust: record.Trust, Payload: payload, CreatedAt: record.OccurredAt}})
	return err
}

func (l *Ledger) Export(ctx context.Context, sourceSequence int64, recordIDs []string, createdAt time.Time, cryptoProfile contracts.CryptoProfile, protectionRef string) (StateEnvelope, error) {
	if sourceSequence <= 0 || createdAt.IsZero() {
		return StateEnvelope{}, errors.New("portable export requires positive source sequence and time")
	}
	_, state, err := l.load(ctx)
	if err != nil {
		return StateEnvelope{}, err
	}
	selected := map[string]bool{}
	for _, id := range recordIDs {
		selected[id] = true
	}
	records := []PortableRecord{}
	for _, record := range state.Records {
		if len(selected) == 0 || selected[record.ID] {
			records = append(records, record)
			delete(selected, record.ID)
		}
	}
	if len(selected) != 0 {
		return StateEnvelope{}, errors.New("portable export requested unknown record identity")
	}
	return FreezeEnvelope(StateEnvelope{Source: contracts.PrincipalRef{ID: l.install, Kind: "installation"}, SourceSequence: sourceSequence, CreatedAt: createdAt.UTC(), Records: records, CryptoProfile: cryptoProfile, ProtectionEvidenceRef: protectionRef})
}

func (l *Ledger) Import(ctx context.Context, envelope StateEnvelope) (ImportResult, error) {
	if err := VerifyEnvelope(envelope); err != nil {
		return ImportResult{}, err
	}
	if envelope.Source.ID == l.install {
		return ImportResult{}, errors.New("portable envelope cannot import itself")
	}
	events, state, err := l.load(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	if containsString(state.ImportedIDs, envelope.ID) {
		return ImportResult{EnvelopeID: envelope.ID}, nil
	}
	if envelope.SourceSequence <= state.SourceSequences[envelope.Source.ID] {
		return ImportResult{}, errors.New("portable envelope source sequence is stale or replayed")
	}
	for _, record := range envelope.Records {
		if err := validateParentAvailability(record, state.Records, envelope.Records); err != nil {
			return ImportResult{}, err
		}
	}
	principal, err := l.authority.AuthorizeImport(ctx, l.install, envelope.ID, envelope.Source)
	if err != nil {
		return ImportResult{}, fmt.Errorf("authorize portable import: %w", err)
	}
	if principal.Validate() != nil || principal.ID == envelope.Source.ID {
		return ImportResult{}, errors.New("portable import requires distinct valid destination authority")
	}
	result, next := reconcile(state, envelope)
	payload, err := json.Marshal(importPayload{Envelope: envelope, Result: result, Authority: principal})
	if err != nil {
		return ImportResult{}, err
	}
	_, err = l.store.Append(ctx, portableAggregate(l.install), int64(len(events)), []eventstore.Event{{ID: "event:import:" + envelope.ID, AggregateType: "portable_state", Type: portableEnvelopeImported, Version: portableEventVersions.CurrentVersion(), Actor: principal, CommandID: "portable-import:" + envelope.ID, CorrelationID: envelope.ID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: envelope.CreatedAt}})
	if err != nil {
		return ImportResult{}, err
	}
	_ = next
	return result, nil
}

func (l *Ledger) State(ctx context.Context) (State, error) {
	_, state, err := l.load(ctx)
	return state, err
}

// Heads returns immutable lineage heads without selecting through a conflict.
// Domain consumers decide what a non-tombstone head means; core only preserves
// lineage and refuses ambiguous authority.
func (l *Ledger) Heads(ctx context.Context, kind RecordKind, entityID, scope string) ([]PortableRecord, error) {
	state, err := l.State(ctx)
	if err != nil {
		return nil, err
	}
	candidates := []PortableRecord{}
	notHead := map[string]bool{}
	for _, record := range state.Records {
		if record.Kind == kind && record.EntityID == entityID && record.Scope == scope {
			candidates = append(candidates, record)
			for _, id := range append(append([]string(nil), record.ParentIDs...), record.SupersedesIDs...) {
				notHead[id] = true
			}
		}
	}
	heads := []PortableRecord{}
	for _, record := range candidates {
		if !notHead[record.ID] {
			heads = append(heads, record)
		}
	}
	for _, conflict := range state.Conflicts {
		if conflict.Kind != kind || conflict.EntityID != entityID || conflict.Scope != scope {
			continue
		}
		competingHeads := 0
		for _, id := range conflict.CompetingRecordIDs {
			if !notHead[id] {
				competingHeads++
			}
		}
		if competingHeads > 1 {
			return heads, ErrUnresolvedConflict
		}
	}
	sort.Slice(heads, func(i, j int) bool { return heads[i].ID < heads[j].ID })
	return heads, nil
}

func (l *Ledger) load(ctx context.Context) ([]eventstore.Event, State, error) {
	if l == nil || l.store == nil || l.install == "" {
		return nil, State{}, errors.New("portable ledger is unavailable")
	}
	events, err := l.store.LoadAggregate(ctx, portableAggregate(l.install), 0)
	if err != nil {
		return nil, State{}, err
	}
	state := State{SourceSequences: map[string]int64{}}
	for _, event := range events {
		payload, _, err := portableEventVersions.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, State{}, err
		}
		switch event.Type {
		case portableRecordAddedEvent:
			var record PortableRecord
			if json.Unmarshal(payload, &record) != nil || VerifyRecord(record) != nil || record.SourceInstallation != l.install || event.ID != "event:"+record.ID || event.Actor != (contracts.PrincipalRef{ID: l.install, Kind: "installation"}) || event.CorrelationID != record.EntityID || event.CausationID != firstParent(record) || event.Trust != record.Trust {
				return nil, State{}, errors.New("portable local record event metadata is invalid")
			}
			if containsRecord(state.Records, record.ID) {
				return nil, State{}, errors.New("duplicate local portable record event")
			}
			if err := validateParentAvailability(record, state.Records, nil); err != nil {
				return nil, State{}, err
			}
			state.Records = append(state.Records, record)
		case portableEnvelopeImported:
			var imported importPayload
			if json.Unmarshal(payload, &imported) != nil || VerifyEnvelope(imported.Envelope) != nil || event.ID != "event:import:"+imported.Envelope.ID || event.CorrelationID != imported.Envelope.ID || imported.Authority.Validate() != nil || event.Actor != imported.Authority || event.Actor.ID == imported.Envelope.Source.ID || event.Trust != contracts.TrustPolicy {
				return nil, State{}, errors.New("portable import event metadata is invalid")
			}
			if containsString(state.ImportedIDs, imported.Envelope.ID) || imported.Envelope.SourceSequence <= state.SourceSequences[imported.Envelope.Source.ID] {
				return nil, State{}, errors.New("portable replay found duplicate or stale import")
			}
			for _, record := range imported.Envelope.Records {
				if err := validateParentAvailability(record, state.Records, imported.Envelope.Records); err != nil {
					return nil, State{}, err
				}
			}
			expected, next := reconcile(state, imported.Envelope)
			if !equalImportResult(expected, imported.Result) {
				return nil, State{}, errors.New("portable import reconciliation result changed during replay")
			}
			state = next
		default:
			return nil, State{}, errors.New("unknown portable state event type")
		}
	}
	canonicalizeState(&state)
	return events, state, nil
}

func reconcile(state State, envelope StateEnvelope) (ImportResult, State) {
	next := cloneState(state)
	result := ImportResult{EnvelopeID: envelope.ID}
	for _, incoming := range envelope.Records {
		if containsRecord(next.Records, incoming.ID) {
			continue
		}
		stale := false
		for _, local := range next.Records {
			if !sameSubject(local, incoming) {
				continue
			}
			switch incoming.Merge {
			case MergeCommutative:
				// Immutable union is the declared deterministic merge.
			case MergeVersioned:
				if isAncestor(incoming.ID, local.ID, append(next.Records, envelope.Records...)) {
					stale = true
				} else if !isAncestor(local.ID, incoming.ID, append(next.Records, envelope.Records...)) {
					result.Conflicts = appendUniqueConflict(result.Conflicts, newConflict(local, incoming, false))
				}
			case MergeExclusive:
				if !containsString(incoming.SupersedesIDs, local.ID) && !containsString(local.SupersedesIDs, incoming.ID) {
					result.Conflicts = appendUniqueConflict(result.Conflicts, newConflict(local, incoming, incoming.Kind == RecordPolicy))
				}
			}
		}
		next.Records = append(next.Records, incoming)
		result.AddedIDs = append(result.AddedIDs, incoming.ID)
		if stale {
			result.StaleIDs = append(result.StaleIDs, incoming.ID)
		}
	}
	next.Conflicts = append(next.Conflicts, result.Conflicts...)
	next.ImportedIDs = append(next.ImportedIDs, envelope.ID)
	next.SourceSequences[envelope.Source.ID] = envelope.SourceSequence
	canonicalizeResult(&result)
	canonicalizeState(&next)
	return result, next
}

func newConflict(a, b PortableRecord, security bool) Conflict {
	conflict, _ := freezeConflict(Conflict{Kind: a.Kind, EntityID: a.EntityID, Scope: a.Scope, CompetingRecordIDs: []string{a.ID, b.ID}, SourceInstallations: []string{a.SourceInstallation, b.SourceInstallation}, SecuritySignificant: security, AllowedResolutions: []string{"governed_select", "preserve_both"}})
	return conflict
}

func validateParentAvailability(record PortableRecord, existing, batch []PortableRecord) error {
	for _, parent := range append(append([]PortableRecord(nil), existing...), batch...) {
		if containsString(record.ParentIDs, parent.ID) && (parent.Kind != record.Kind || parent.EntityID != record.EntityID) {
			return errors.New("portable record parent crosses record identity")
		}
	}
	for _, parentID := range append(record.ParentIDs, record.SupersedesIDs...) {
		if !containsRecord(existing, parentID) && !containsRecord(batch, parentID) {
			return errors.New("portable record lineage references unavailable parent")
		}
	}
	return nil
}

func isAncestor(ancestor, descendant string, records []PortableRecord) bool {
	byID := map[string]PortableRecord{}
	for _, record := range records {
		byID[record.ID] = record
	}
	seen := map[string]bool{}
	queue := []string{descendant}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, parent := range byID[id].ParentIDs {
			if parent == ancestor {
				return true
			}
			queue = append(queue, parent)
		}
	}
	return false
}

func sameSubject(a, b PortableRecord) bool {
	return a.Kind == b.Kind && a.EntityID == b.EntityID && a.Scope == b.Scope
}

func firstParent(record PortableRecord) string {
	if len(record.ParentIDs) > 0 {
		return record.ParentIDs[0]
	}
	return ""
}

func containsRecord(records []PortableRecord, id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func appendUniqueConflict(conflicts []Conflict, conflict Conflict) []Conflict {
	for _, existing := range conflicts {
		if existing.ID == conflict.ID {
			return conflicts
		}
	}
	return append(conflicts, conflict)
}

func cloneState(state State) State {
	out := State{Records: append([]PortableRecord(nil), state.Records...), Conflicts: append([]Conflict(nil), state.Conflicts...), ImportedIDs: append([]string(nil), state.ImportedIDs...), SourceSequences: map[string]int64{}}
	for source, sequence := range state.SourceSequences {
		out.SourceSequences[source] = sequence
	}
	return out
}

func canonicalizeResult(result *ImportResult) {
	result.AddedIDs = canonicalStrings(result.AddedIDs)
	result.StaleIDs = canonicalStrings(result.StaleIDs)
	sort.Slice(result.Conflicts, func(i, j int) bool { return result.Conflicts[i].ID < result.Conflicts[j].ID })
}

func canonicalizeState(state *State) {
	sort.Slice(state.Records, func(i, j int) bool { return state.Records[i].ID < state.Records[j].ID })
	sort.Slice(state.Conflicts, func(i, j int) bool { return state.Conflicts[i].ID < state.Conflicts[j].ID })
	state.ImportedIDs = canonicalStrings(state.ImportedIDs)
}

func equalImportResult(a, b ImportResult) bool {
	canonicalizeResult(&a)
	canonicalizeResult(&b)
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
