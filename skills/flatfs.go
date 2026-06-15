// SPDX-License-Identifier: MIT
// Purpose: flatten the nested skills/ category directories into a single
// fs.FS that skillsmith can consume. Skillsmith expects all skill directories
// to live at the root of the filesystem; this wrapper maps each skill name to
// its real category-prefixed path without duplicating the embedded bytes.
// Docs: skills.doc.md
package skills

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// flatSkillFS presents a flattened view of nested skill directories.
// Each directory containing a SKILL.md becomes a root-level directory whose
// name is the last path component (e.g. "code-skills/add-endpoint" -> "add-endpoint").
type flatSkillFS struct {
	root fs.FS
	// mapping from flat skill name to its real directory inside root.
	dirs map[string]string
	// files maps flat paths (e.g. "add-endpoint/SKILL.md") to real paths.
	files map[string]string
}

func newFlatSkillFS(root fs.FS) (*flatSkillFS, error) {
	f := &flatSkillFS{
		root:  root,
		dirs:  make(map[string]string),
		files: make(map[string]string),
	}
	if err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(path.Base(p)) != "skill.md" {
			return nil
		}
		// p is something like "code-skills/add-endpoint/SKILL.md".
		dir := path.Dir(p)
		name := path.Base(dir)
		f.dirs[name] = dir
		return nil
	}); err != nil {
		return nil, err
	}

	for name, realDir := range f.dirs {
		if err := fs.WalkDir(root, realDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := strings.TrimPrefix(p, realDir)
			rel = strings.TrimPrefix(rel, "/")
			flatPath := path.Join(name, rel)
			f.files[flatPath] = p
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func (f *flatSkillFS) Open(name string) (fs.File, error) {
	name = path.Clean(name)
	if name == "." {
		return &flatRootDir{fs: f}, nil
	}
	real, ok := f.files[name]
	if !ok {
		// Directories are not stored in f.files, but skillsmith/fs.WalkDir
		// needs to be able to open them. Build the real dir path on demand.
		flatDir := name
		if idx := strings.Index(name, "/"); idx >= 0 {
			flatDir = name[:idx]
		}
		realDir, ok := f.dirs[flatDir]
		if !ok {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		real = path.Join(realDir, strings.TrimPrefix(name, flatDir))
		real = strings.TrimPrefix(real, "/")
	}
	return f.root.Open(real)
}

func (f *flatSkillFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = path.Clean(name)
	if name == "." {
		names := make([]string, 0, len(f.dirs))
		for n := range f.dirs {
			names = append(names, n)
		}
		sort.Strings(names)
		entries := make([]fs.DirEntry, len(names))
		for i, n := range names {
			entries[i] = flatDirEntry{name: n}
		}
		return entries, nil
	}
	real, ok := f.dirRealPath(name)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return fs.ReadDir(f.root, real)
}

func (f *flatSkillFS) Stat(name string) (fs.FileInfo, error) {
	name = path.Clean(name)
	if name == "." {
		return &flatRootInfo{}, nil
	}
	real, ok := f.files[name]
	if !ok {
		real, ok = f.dirRealPath(name)
		if !ok {
			return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
		}
	}
	return fs.Stat(f.root, real)
}

func (f *flatSkillFS) ReadFile(name string) ([]byte, error) {
	name = path.Clean(name)
	real, ok := f.files[name]
	if !ok {
		real, ok = f.dirRealPath(name)
		if !ok {
			return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
		}
	}
	return fs.ReadFile(f.root, real)
}

func (f *flatSkillFS) dirRealPath(flat string) (string, bool) {
	flatDir := flat
	if idx := strings.Index(flat, "/"); idx >= 0 {
		flatDir = flat[:idx]
	}
	realDir, ok := f.dirs[flatDir]
	if !ok {
		return "", false
	}
	rel := strings.TrimPrefix(flat, flatDir)
	rel = strings.TrimPrefix(rel, "/")
	return path.Join(realDir, rel), true
}

type flatDirEntry struct {
	name string
}

func (e flatDirEntry) Name() string               { return e.name }
func (e flatDirEntry) IsDir() bool                { return true }
func (e flatDirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (e flatDirEntry) Info() (fs.FileInfo, error) { return &flatDirInfo{name: e.name}, nil }

type flatDirInfo struct {
	name string
}

func (i flatDirInfo) Name() string       { return i.name }
func (i flatDirInfo) Size() int64        { return 0 }
func (i flatDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (i flatDirInfo) ModTime() time.Time { return time.Time{} }
func (i flatDirInfo) IsDir() bool        { return true }
func (i flatDirInfo) Sys() any           { return nil }

type flatRootInfo struct{}

func (i flatRootInfo) Name() string       { return "." }
func (i flatRootInfo) Size() int64        { return 0 }
func (i flatRootInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (i flatRootInfo) ModTime() time.Time { return time.Time{} }
func (i flatRootInfo) IsDir() bool        { return true }
func (i flatRootInfo) Sys() any           { return nil }

type flatRootDir struct {
	fs    *flatSkillFS
	idx   int
	names []string
}

func (d *flatRootDir) Stat() (fs.FileInfo, error) { return &flatRootInfo{}, nil }
func (d *flatRootDir) Read([]byte) (int, error)   { return 0, io.EOF }
func (d *flatRootDir) Close() error               { return nil }

func (d *flatRootDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.names == nil {
		d.names = make([]string, 0, len(d.fs.dirs))
		for name := range d.fs.dirs {
			d.names = append(d.names, name)
		}
		sort.Strings(d.names)
	}
	if d.idx >= len(d.names) {
		return nil, io.EOF
	}
	if n <= 0 || d.idx+n > len(d.names) {
		n = len(d.names) - d.idx
	}
	entries := make([]fs.DirEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = flatDirEntry{name: d.names[d.idx]}
		d.idx++
	}
	return entries, nil
}
