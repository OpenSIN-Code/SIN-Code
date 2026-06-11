// SPDX-License-Identifier: MIT

package internal

import (
	"encoding/gob"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── Index Data Model ──────────────────────────────────────

type indexEntry struct {
	File      string
	ModTime   time.Time
	Size      int64
	Trigrams  []uint32
	Symbols   []symbolIndex
	IsBinary  bool
	Lines     int
}

type symbolIndex struct {
	Name       string
	Type       string
	StartLine  int
	EndLine    int
	Signature  string
}

type persistentIndex struct {
	Version   int
	CreatedAt time.Time
	RootPath  string
	Entries   map[string]indexEntry
}

// ── In-Memory Index (Query Hot Path) ─────────────────────

type fileIndex struct {
	path      string
	modTime   time.Time
	size      int64
	trigrams  map[uint32]struct{}
	symbols   []symbolIndex
	isBinary  bool
	lines     int
}

type inMemoryIndex struct {
	root      string
	version   int
	createdAt time.Time
	files     map[string]*fileIndex
	mu        sync.RWMutex
}

func (idx *inMemoryIndex) rootPath() string { return idx.root }

func (idx *inMemoryIndex) hasFile(path string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	_, ok := idx.files[path]
	return ok
}

func (idx *inMemoryIndex) fileModTime(path string) (time.Time, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	f, ok := idx.files[path]
	if !ok {
		return time.Time{}, false
	}
	return f.modTime, true
}

func (idx *inMemoryIndex) searchTrigram(query string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	queryTri := queryTrigrams(query)
	if len(queryTri) == 0 {
		return nil
	}
	var matches []string
	for p, fi := range idx.files {
		if fi.isBinary {
			continue
		}
		if len(fi.trigrams) == 0 {
			continue
		}
		all := true
		for t := range queryTri {
			if _, ok := fi.trigrams[t]; !ok {
				all = false
				break
			}
		}
		if all {
			matches = append(matches, p)
		}
	}
	return matches
}

func (idx *inMemoryIndex) searchSymbols(name string, stype string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var matches []string
	for p, fi := range idx.files {
		if fi.isBinary {
			continue
		}
		for _, sym := range fi.symbols {
			if stype != "" && sym.Type != stype {
				continue
			}
			if sym.Name == name || (strings.Contains(sym.Name, name) && len(name) >= 3) {
				matches = append(matches, p)
				break
			}
		}
	}
	return matches
}

func (idx *inMemoryIndex) allIndexedPaths() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]string, 0, len(idx.files))
	for p := range idx.files {
		out = append(out, p)
	}
	return out
}

func (idx *inMemoryIndex) clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.files = make(map[string]*fileIndex)
}

func (idx *inMemoryIndex) add(e indexEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	tris := make(map[uint32]struct{}, len(e.Trigrams))
	for _, t := range e.Trigrams {
		tris[t] = struct{}{}
	}
	idx.files[e.File] = &fileIndex{
		path:     e.File,
		modTime:  e.ModTime,
		size:     e.Size,
		trigrams: tris,
		symbols:  e.Symbols,
		isBinary: e.IsBinary,
		lines:    e.Lines,
	}
}

func (idx *inMemoryIndex) remove(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.files, path)
}

func (idx *inMemoryIndex) len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.files)
}

// ── Trigram Utilities ─────────────────────────────────────

func queryTrigrams(query string) map[uint32]struct{} {
	q := strings.ToLower(query)
	if len(q) < 3 {
		return nil
	}
	tri := make(map[uint32]struct{})
	for i := 0; i <= len(q)-3; i++ {
		h := fnv.New32a()
		_, _ = h.Write([]byte(q[i : i+3]))
		tri[h.Sum32()] = struct{}{}
	}
	return tri
}

func buildTrigrams(content string) []uint32 {
	lc := strings.ToLower(content)
	if len(lc) < 3 {
		return nil
	}
	seen := make(map[uint32]struct{})
	for i := 0; i <= len(lc)-3; i++ {
		h := fnv.New32a()
		_, _ = h.Write([]byte(lc[i : i+3]))
		seen[h.Sum32()] = struct{}{}
	}
	out := make([]uint32, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	return out
}

// ── Persistence (gob) ───────────────────────────────────────

func indexPath(root string) string {
	return filepath.Join(root, ".sin-code", "index.bin")
}

func loadIndex(root string) (*inMemoryIndex, error) {
	idx := &inMemoryIndex{
		root:  root,
		files: make(map[string]*fileIndex),
	}
	p := indexPath(root)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return idx, nil
		}
		return nil, err
	}
	defer f.Close()
	var pi persistentIndex
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&pi); err != nil {
		return idx, nil // corrupt — treat as empty
	}
	idx.version = pi.Version
	idx.createdAt = pi.CreatedAt
	for _, e := range pi.Entries {
		idx.add(e)
	}
	return idx, nil
}

func saveIndex(idx *inMemoryIndex) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	p := indexPath(idx.root)
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		return err
	}
	pi := persistentIndex{
		Version:   idx.version,
		CreatedAt: idx.createdAt,
		RootPath:  idx.root,
		Entries:   make(map[string]indexEntry, len(idx.files)),
	}
	for p, fi := range idx.files {
		tris := make([]uint32, 0, len(fi.trigrams))
		for t := range fi.trigrams {
			tris = append(tris, t)
		}
		pi.Entries[p] = indexEntry{
			File:     p,
			ModTime:  fi.modTime,
			Size:     fi.size,
			Trigrams: tris,
			Symbols:  fi.symbols,
			IsBinary: fi.isBinary,
			Lines:    fi.lines,
		}
	}
	tmp := p + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := gob.NewEncoder(f)
	if err := enc.Encode(pi); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, p)
}

// ── Parallel Build ──────────────────────────────────────────

func buildIndex(root string) (*inMemoryIndex, error) {
	idx := &inMemoryIndex{
		root:      root,
		version:   1,
		createdAt: time.Now(),
		files:     make(map[string]*fileIndex),
	}

	ignorePatterns := loadGitignore(root)

	entries := make([]string, 0, 256)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if ignorePatterns.matchDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if isBinaryFile(path) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if ignorePatterns.matchFile(rel) {
			return nil
		}
		entries = append(entries, path)
		return nil
	})

	const workerCount = 8
	type job struct {
		path string
	}
	jobs := make(chan job, len(entries))
	results := make(chan indexEntry, len(entries))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				ie := processFileForIndex(j.path, root)
				results <- ie
			}
		}()
	}

	go func() {
		for _, e := range entries {
			jobs <- job{path: e}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for ie := range results {
		idx.add(ie)
	}

	return idx, nil
}

func processFileForIndex(absPath string, root string) indexEntry {
	pathRel, _ := filepath.Rel(root, absPath)
	info, err := os.Stat(absPath)
	if err != nil {
		info = &mockFileInfo{}
	}
	ie := indexEntry{
		File:     pathRel,
		ModTime:  info.ModTime(),
		Size:     info.Size(),
		IsBinary: isBinaryFile(absPath),
	}
	if ie.IsBinary {
		return ie
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return ie
	}
	content := string(data)
	ie.Trigrams = buildTrigrams(content)
	ie.Lines = strings.Count(content, "\n") + 1

	outline := parseOutline(absPath, data)
	if outline == nil || outline.Engine == "none" {
		return ie
	}
	for _, sym := range outline.Symbols {
			ie.Symbols = append(ie.Symbols, symbolIndex{
				Name:      sym.Name,
				Type:      sym.Kind,
				StartLine: sym.StartLine,
				EndLine:   sym.EndLine,
			})
	}
	return ie
}

type mockFileInfo struct{}

func (m *mockFileInfo) Name() string       { return "" }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }

// ── Refresh / Incremental ─────────────────────────────────

func refreshIndex(idx *inMemoryIndex) (*inMemoryIndex, int, int, error) {
	root := idx.root
	ignorePatterns := loadGitignore(root)

	presentFiles := make(map[string]os.FileInfo)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if ignorePatterns.matchDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if isBinaryFile(path) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if ignorePatterns.matchFile(rel) {
			return nil
		}
		presentFiles[rel] = info
		return nil
	})

	added, removed := 0, 0

	// First pass: find files to add or update
	var toAdd []string
	for rel, info := range presentFiles {
		fi, ok := func() (*fileIndex, bool) {
			idx.mu.RLock()
			defer idx.mu.RUnlock()
			f, exists := idx.files[rel]
			return f, exists
		}()
		if !ok || info.ModTime() != fi.modTime || info.Size() != fi.size {
			toAdd = append(toAdd, filepath.Join(root, rel))
			added++
		}
	}

	// Remove stale entries
	idx.mu.Lock()
	for p := range idx.files {
		if _, ok := presentFiles[p]; !ok {
			delete(idx.files, p)
			removed++
		}
	}
	idx.mu.Unlock()

	// Add/update entries
	for _, absPath := range toAdd {
		ie := processFileForIndex(absPath, root)
		idx.add(ie)
	}

	return idx, added, removed, nil
}

// ── File Index Cache (singleton per root) ─────────────────

var (
	globalFileIndex   *inMemoryIndex
	globalFileIndexMu sync.Mutex
)

func getFileIndex(root string) (*inMemoryIndex, bool, error) {
	globalFileIndexMu.Lock()
	defer globalFileIndexMu.Unlock()

	if globalFileIndex != nil && globalFileIndex.root == root {
		return globalFileIndex, true, nil
	}
	idx, err := loadIndex(root)
	if err != nil {
		return nil, false, err
	}
	globalFileIndex = idx
	return idx, false, nil
}

func setFileIndex(idx *inMemoryIndex) {
	globalFileIndexMu.Lock()
	defer globalFileIndexMu.Unlock()
	globalFileIndex = idx
}
