package client

import (
	"context"
	"errors"
	"sync"
)

// ResolvedInvocation is the package-neutral result of contract resolution.
// Package adapters may attach execution behavior to this value, but the
// client layer never interprets package or domain semantics.
type ResolvedInvocation struct {
	EntryPointID   string
	PackageID      string
	PackageVersion string
	PackageDigest  string
	GraphID        string
	GraphVersion   string
	Arguments      []string
	Options        map[string]string
}

type InvocationHandler func(context.Context, ResolvedInvocation, func(string) string) error

var handlers = struct {
	sync.RWMutex
	items map[string]InvocationHandler
}{items: make(map[string]InvocationHandler)}

// RegisterInvocationHandler lets an installed-package adapter provide the
// execution side of an InvocationContract. Registration is keyed by the
// contract's immutable package and entry-point identity, never by a core
// command name.
func RegisterInvocationHandler(packageID, entryPointID string, handler InvocationHandler) error {
	if packageID == "" || entryPointID == "" || handler == nil {
		return errors.New("invocation handler identity and handler are required")
	}
	key := packageID + "\x00" + entryPointID
	handlers.Lock()
	defer handlers.Unlock()
	if _, exists := handlers.items[key]; exists {
		return errors.New("invocation handler already registered")
	}
	handlers.items[key] = handler
	return nil
}

func DispatchInvocation(ctx context.Context, invocation ResolvedInvocation, getenv func(string) string) (bool, error) {
	key := invocation.PackageID + "\x00" + invocation.EntryPointID
	handlers.RLock()
	handler := handlers.items[key]
	handlers.RUnlock()
	if handler == nil {
		return false, nil
	}
	return true, handler(ctx, invocation, getenv)
}
