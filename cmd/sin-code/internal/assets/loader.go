// SPDX-License-Identifier: MIT
// Purpose: walk a directory tree and load every *.md file as the given
// kind. `SKILL.md` files are picked up as KindSkill.
// Docs: loader.doc.md
package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// package-level hooks to make filesystem error branches testable.
var (
	walkDirHook  = filepath.WalkDir
	osReadFileHook = os.ReadFile
)

// LoadDir walks a directory tree and loads every *.md file as the
// given kind. Skills are expected as <dir>/<name>/SKILL.md; agents
// and commands as flat *.md.
func LoadDir(root string, kind Kind) ([]*Asset, error) {
	var out []*Asset
	err := walkDirHook(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".md") {
			return nil
		}
		if kind == KindSkill && name != "skill.md" {
			return nil // skills live in SKILL.md only
		}
		data, err := osReadFileHook(path)
		if err != nil {
			return err
		}
		a, err := ParseAsset(kind, path, data)
		if err != nil {
			return err
		}
		// Default the name to the file/dir stem when frontmatter omits it.
		if a.Name == "" {
			if kind == KindSkill {
				a.Name = filepath.Base(filepath.Dir(path))
			} else {
				a.Name = strings.TrimSuffix(d.Name(), ".md")
			}
		}
		out = append(out, a)
		return nil
	})
	return out, err
}

// LoadStandardLayout loads agents/, commands/ and skills/ relative to
// a base (e.g. a vendored copy of the ECC repo), tolerating missing
// directories.
func LoadStandardLayout(base string) ([]*Asset, error) {
	var all []*Asset
	specs := []struct {
		dir  string
		kind Kind
	}{
		{filepath.Join(base, "agents"), KindAgent},
		{filepath.Join(base, "commands"), KindCommand},
		{filepath.Join(base, ".agents", "skills"), KindSkill},
		{filepath.Join(base, "skills"), KindSkill},
	}
	for _, s := range specs {
		if _, err := os.Stat(s.dir); os.IsNotExist(err) {
			continue
		}
		loaded, err := LoadDir(s.dir, s.kind)
		if err != nil {
			return nil, err
		}
		all = append(all, loaded...)
	}
	return all, nil
}
