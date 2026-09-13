package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var ErrWorkspaceEscape = errors.New("path escapes authorized workspace root")

// ResolveAuthorizedPath resolves symlinks for both the workspace root and the
// requested path, then verifies the result remains inside the granted root.
func ResolveAuthorizedPath(root, requested string) (string, error) {
	if root == "" || requested == "" {
		return "", errors.New("workspace root and requested path are required")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("absolute workspace root: %w", err)
	}

	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(resolvedRoot, candidate)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve requested path: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("absolute requested path: %w", err)
	}

	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("compare workspace path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrWorkspaceEscape
	}
	return resolved, nil
}
