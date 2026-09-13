package workspace

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type DiscoveryOptions struct {
	MaxFiles     int
	MaxFileBytes int64
	ExcludeDirs  []string
	ExcludeGlobs []string
}

func DiscoverFiles(root string, opts DiscoveryOptions) ([]string, error) {
	resolvedRoot, err := ResolveAuthorizedPath(root, ".")
	if err != nil {
		return nil, err
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 10000
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = 2 << 20
	}
	excludedDirs := map[string]struct{}{}
	for _, d := range opts.ExcludeDirs {
		excludedDirs[filepath.Clean(d)] = struct{}{}
	}

	files := make([]string, 0)
	err = filepath.WalkDir(resolvedRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(resolvedRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.Clean(rel)
		if entry.Type()&fs.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if rel != "." && excludedDirectory(rel, excludedDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= opts.MaxFiles {
			return errors.New("workspace discovery file limit exceeded")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > opts.MaxFileBytes || matchesAny(rel, opts.ExcludeGlobs) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func excludedDirectory(rel string, excluded map[string]struct{}) bool {
	for d := range excluded {
		if rel == d || strings.HasPrefix(rel, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func matchesAny(path string, globs []string) bool {
	for _, pattern := range globs {
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}
	return false
}
