package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileRecord struct {
	Path      string
	Digest    string
	Size      int64
	ModTime   time.Time
	IndexedAt time.Time
}

type Index struct {
	mu    sync.RWMutex
	files map[string]FileRecord
}

func NewIndex() *Index {
	return &Index{files: map[string]FileRecord{}}
}

func (i *Index) UpdateFile(root, requested string, now time.Time) (FileRecord, bool, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return FileRecord{}, false, err
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return FileRecord{}, false, err
	}
	resolved, err := ResolveAuthorizedPath(root, requested)
	if err != nil {
		return FileRecord{}, false, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return FileRecord{}, false, err
	}
	if !info.Mode().IsRegular() {
		return FileRecord{}, false, errors.New("workspace index only accepts regular files")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return FileRecord{}, false, err
	}
	sum := sha256.Sum256(data)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return FileRecord{}, false, err
	}
	record := FileRecord{Path: filepath.Clean(rel), Digest: digest, Size: info.Size(), ModTime: info.ModTime().UTC(), IndexedAt: now.UTC()}

	i.mu.Lock()
	defer i.mu.Unlock()
	previous, ok := i.files[record.Path]
	changed := !ok || previous.Digest != record.Digest
	i.files[record.Path] = record
	return record, changed, nil
}

func (i *Index) Remove(path string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.files, filepath.Clean(path))
}

func (i *Index) Get(path string) (FileRecord, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	record, ok := i.files[filepath.Clean(path)]
	return record, ok
}

func (i *Index) Stale(path, digest string) bool {
	record, ok := i.Get(path)
	return !ok || record.Digest != digest
}
