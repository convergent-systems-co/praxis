package contracts

import "strings"

// ScopeAllows is the canonical conservative scope relation used by lifecycle
// preview. Only exact scope or an explicit trailing :* hierarchy is allowed.
func ScopeAllows(granted, requested string) bool {
	if granted == requested {
		return true
	}
	if !strings.HasSuffix(granted, ":*") {
		return false
	}
	prefix := strings.TrimSuffix(granted, "*")
	return strings.HasPrefix(requested, prefix)
}
