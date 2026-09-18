package workspace

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type TextMatch struct {
	Path       string
	Line       int
	Text       string
	FileDigest string
	Class      EvidenceClass
}

type SearchOptions struct {
	MaxFileBytes int64
	MaxMatches   int
	CaseSensitive bool
}

// SearchText performs deterministic local lexical search over explicitly
// supplied relative paths. Discovery/index layers decide which paths to pass.
func SearchText(root string, paths []string, query string, opts SearchOptions) ([]TextMatch, error) {
	if query == "" {
		return nil, errors.New("search query is required")
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = 2 << 20
	}
	if opts.MaxMatches <= 0 {
		opts.MaxMatches = 100
	}
	needle := query
	if !opts.CaseSensitive {
		needle = strings.ToLower(query)
	}

	matches := make([]TextMatch, 0)
	for _, rel := range paths {
		resolved, err := ResolveAuthorizedPath(root, rel)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > opts.MaxFileBytes {
			continue
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			haystack := text
			if !opts.CaseSensitive {
				haystack = strings.ToLower(text)
			}
			if strings.Contains(haystack, needle) {
				relPath, _ := filepath.Rel(root, resolved)
				matches = append(matches, TextMatch{Path: filepath.Clean(relPath), Line: line, Text: text, FileDigest: digest, Class: EvidenceExact})
				if len(matches) >= opts.MaxMatches {
					return matches, nil
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	return matches, nil
}
