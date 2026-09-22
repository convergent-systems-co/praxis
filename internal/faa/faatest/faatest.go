// Package faatest holds test doubles for the Forward Authority Anchor. Nothing
// outside tests may import it: a production build reaches the anchor only
// through the platform backend.
package faatest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/convergent-systems-co/praxis/internal/faa"
)

// Memory is an in-process anchor with fault injection, used to exercise the
// crash and failure boundaries between the store and the anchor.
type Memory struct {
	mu     sync.Mutex
	states map[string][]byte
	// Faults. FailLoad/FailSet return ErrUnavailable; CorruptOnLoad returns
	// ErrCorrupt; DropWrite makes Set report success without persisting (a
	// backend that lies), which the read-back must catch.
	FailLoad, FailSet, CorruptOnLoad, DropWrite bool
	// AfterSet, if set, runs after each successful Set (a crash hook).
	AfterSet func(faa.State)
	Sets     int
}

func NewMemory() *Memory { return &Memory{states: map[string][]byte{}} }

func (m *Memory) Load(ctx context.Context, installation string) (faa.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailLoad {
		return faa.State{}, faa.ErrUnavailable
	}
	if m.CorruptOnLoad {
		return faa.State{}, faa.ErrCorrupt
	}
	raw, ok := m.states[installation]
	if !ok {
		return faa.State{}, faa.ErrMissing
	}
	var s faa.State
	if err := json.Unmarshal(raw, &s); err != nil || s.Validate() != nil || s.Installation != installation {
		return faa.State{}, faa.ErrCorrupt
	}
	return s, nil
}

func (m *Memory) Set(ctx context.Context, installation string, expect *faa.State, next faa.State) error {
	m.mu.Lock()
	if m.FailSet {
		m.mu.Unlock()
		return faa.ErrUnavailable
	}
	if next.Installation != installation || next.Validate() != nil {
		m.mu.Unlock()
		return faa.ErrCorrupt
	}
	raw, ok := m.states[installation]
	switch {
	case expect == nil && ok:
		m.mu.Unlock()
		return faa.ErrConflict
	case expect != nil:
		var cur faa.State
		if !ok || json.Unmarshal(raw, &cur) != nil || cur != *expect || next.Seq <= cur.Seq {
			m.mu.Unlock()
			return faa.ErrConflict
		}
	}
	if !m.DropWrite {
		body, _ := json.Marshal(next)
		m.states[installation] = body
	}
	m.Sets++
	hook := m.AfterSet
	m.mu.Unlock()
	got, err := m.Load(ctx, installation)
	if err != nil || got != next {
		return faa.ErrUnavailable
	}
	if hook != nil {
		hook(next)
	}
	return nil
}

func (m *Memory) Revert(ctx context.Context, installation string, from, to faa.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, ok := m.states[installation]
	var cur faa.State
	if !ok || json.Unmarshal(raw, &cur) != nil || cur != from {
		return faa.ErrConflict
	}
	body, _ := json.Marshal(to)
	m.states[installation] = body
	return nil
}

// Snapshot and Restore let a test roll the ANCHOR itself back (the accepted
// storage-key/OS-user trust root, adversary A4), to show what it does and does
// not defend.
func (m *Memory) Snapshot(installation string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.states[installation]...)
}
func (m *Memory) Restore(installation string, raw []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if raw == nil {
		delete(m.states, installation)
		return
	}
	m.states[installation] = append([]byte(nil), raw...)
}

// File is a cross-process anchor for tests that spawn helper processes. It
// serialises with flock and is NOT an authorised production backend.
type File struct{ Dir string }

func (f File) path(installation string) string {
	return filepath.Join(f.Dir, "faa-"+filepath.Base(installation)+".json")
}

func (f File) lock() (*os.File, error) {
	l, err := os.OpenFile(filepath.Join(f.Dir, "faa.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(l.Fd()), syscall.LOCK_EX); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

func (f File) read(installation string) (faa.State, error) {
	raw, err := os.ReadFile(f.path(installation))
	if errors.Is(err, os.ErrNotExist) {
		return faa.State{}, faa.ErrMissing
	}
	if err != nil {
		return faa.State{}, faa.ErrUnavailable
	}
	var s faa.State
	if json.Unmarshal(raw, &s) != nil || s.Validate() != nil || s.Installation != installation {
		return faa.State{}, faa.ErrCorrupt
	}
	return s, nil
}

func (f File) Load(ctx context.Context, installation string) (faa.State, error) {
	l, err := f.lock()
	if err != nil {
		return faa.State{}, faa.ErrUnavailable
	}
	defer l.Close()
	return f.read(installation)
}

func (f File) Set(ctx context.Context, installation string, expect *faa.State, next faa.State) error {
	l, err := f.lock()
	if err != nil {
		return faa.ErrUnavailable
	}
	defer l.Close()
	cur, err := f.read(installation)
	switch {
	case expect == nil && err == nil:
		return faa.ErrConflict
	case expect == nil && !errors.Is(err, faa.ErrMissing):
		return err
	case expect != nil && (err != nil || cur != *expect || next.Seq <= cur.Seq):
		return faa.ErrConflict
	}
	body, _ := json.Marshal(next)
	if err := os.WriteFile(f.path(installation), body, 0o600); err != nil {
		return faa.ErrUnavailable
	}
	return nil
}

func (f File) Revert(ctx context.Context, installation string, from, to faa.State) error {
	l, err := f.lock()
	if err != nil {
		return faa.ErrUnavailable
	}
	defer l.Close()
	cur, err := f.read(installation)
	if err != nil || cur != from {
		return faa.ErrConflict
	}
	body, _ := json.Marshal(to)
	return os.WriteFile(f.path(installation), body, 0o600)
}

func (m *Memory) Reset(ctx context.Context, installation string, next faa.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailSet {
		return faa.ErrUnavailable
	}
	if next.Installation != installation || next.Validate() != nil {
		return faa.ErrCorrupt
	}
	body, _ := json.Marshal(next)
	m.states[installation] = body
	m.CorruptOnLoad = false
	return nil
}

func (f File) Reset(ctx context.Context, installation string, next faa.State) error {
	l, err := f.lock()
	if err != nil {
		return faa.ErrUnavailable
	}
	defer l.Close()
	if next.Installation != installation || next.Validate() != nil {
		return faa.ErrCorrupt
	}
	body, _ := json.Marshal(next)
	return os.WriteFile(f.path(installation), body, 0o600)
}
