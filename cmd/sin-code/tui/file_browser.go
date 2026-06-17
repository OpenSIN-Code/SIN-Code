// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type FileNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children []*FileNode
	Expanded bool
}

type flatEntry struct {
	node  *FileNode
	depth int
}

type FileBrowser struct {
	mu     sync.Mutex
	root   string
	tree   *FileNode
	flat   []flatEntry
	cursor int
	scroll int
	loaded bool
}

func NewFileBrowser(root string) *FileBrowser {
	return &FileBrowser{
		root: root,
	}
}

func (b *FileBrowser) Load() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loadLocked()
}

func (b *FileBrowser) loadLocked() error {
	info, err := os.Stat(b.root)
	if err != nil {
		b.tree = nil
		b.flat = nil
		b.loaded = true
		return err
	}
	if !info.IsDir() {
		b.tree = nil
		b.flat = nil
		b.loaded = true
		return fmt.Errorf("not a directory: %s", b.root)
	}
	b.tree = b.buildNode(b.root, 0, 3)
	b.tree.Expanded = true
	b.rebuildFlatLocked()
	b.cursor = 0
	b.scroll = 0
	b.loaded = true
	return nil
}

func (b *FileBrowser) buildNode(path string, depth, maxDepth int) *FileNode {
	info, err := os.Stat(path)
	if err != nil {
		return &FileNode{
			Name:  filepath.Base(path),
			Path:  path,
			IsDir: false,
		}
	}
	node := &FileNode{
		Name:  info.Name(),
		Path:  path,
		IsDir: info.IsDir(),
	}
	if node.IsDir && depth < maxDepth {
		entries, err := os.ReadDir(path)
		if err == nil {
			var dirs, files []os.DirEntry
			for _, e := range entries {
				if e.IsDir() {
					dirs = append(dirs, e)
				} else {
					files = append(files, e)
				}
			}
			sort.Slice(dirs, func(i, j int) bool {
				return strings.ToLower(dirs[i].Name()) < strings.ToLower(dirs[j].Name())
			})
			sort.Slice(files, func(i, j int) bool {
				return strings.ToLower(files[i].Name()) < strings.ToLower(files[j].Name())
			})
			for _, d := range dirs {
				child := b.buildNode(filepath.Join(path, d.Name()), depth+1, maxDepth)
				node.Children = append(node.Children, child)
			}
			for _, f := range files {
				node.Children = append(node.Children, &FileNode{
					Name:  f.Name(),
					Path:  filepath.Join(path, f.Name()),
					IsDir: false,
				})
			}
		}
	}
	return node
}

func (b *FileBrowser) rebuildFlatLocked() {
	b.flat = nil
	if b.tree == nil {
		return
	}
	b.collectFlatLocked(b.tree, 0)
}

func (b *FileBrowser) collectFlatLocked(node *FileNode, depth int) {
	if node == nil {
		return
	}
	b.flat = append(b.flat, flatEntry{node: node, depth: depth})
	if node.IsDir && node.Expanded {
		for _, child := range node.Children {
			b.collectFlatLocked(child, depth+1)
		}
	}
}

func (b *FileBrowser) Render(styles Styles, width, height int) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	var bldr strings.Builder
	bldr.WriteString(styles.ContentHdr.Render("📁 Files"))
	bldr.WriteString("\n")
	bldr.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 8))))
	bldr.WriteString("\n")

	if b.tree == nil || len(b.flat) == 0 {
		bldr.WriteString(styles.Muted.Render("  (empty or not loaded)"))
		bldr.WriteString("\n")
		return bldr.String()
	}

	listHeight := height - 3
	if listHeight < 1 {
		listHeight = 1
	}

	if b.cursor < b.scroll {
		b.scroll = b.cursor
	}
	if b.cursor >= b.scroll+listHeight {
		b.scroll = b.cursor - listHeight + 1
	}
	if b.scroll < 0 {
		b.scroll = 0
	}

	contentWidth := width - 1
	if contentWidth < 5 {
		contentWidth = 5
	}

	end := b.scroll + listHeight
	if end > len(b.flat) {
		end = len(b.flat)
	}

	for i := b.scroll; i < end; i++ {
		entry := b.flat[i]
		indent := strings.Repeat("  ", entry.depth)
		icon := fileIcon(entry.node)
		name := entry.node.Name
		line := fmt.Sprintf("%s%s %s", indent, icon, name)

		isHidden := strings.HasPrefix(entry.node.Name, ".")
		if i == b.cursor {
			rendered := styles.SidebarSel.Render(padRight(line, contentWidth))
			bldr.WriteString(rendered)
		} else if isHidden {
			rendered := styles.Muted.Render(padRight(line, contentWidth))
			bldr.WriteString(rendered)
		} else if entry.node.IsDir {
			bldr.WriteString(padRight(styles.Bold.Render(line), contentWidth))
		} else {
			bldr.WriteString(padRight(styles.Content.Render(line), contentWidth))
		}
		bldr.WriteString("\n")
	}

	if len(b.flat) > listHeight {
		sb := renderScrollbar(b.scroll, listHeight, len(b.flat), 1)
		bldr.WriteString(styles.Muted.Render(sb))
	}

	return bldr.String()
}

func (b *FileBrowser) MoveUp() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cursor > 0 {
		b.cursor--
	}
}

func (b *FileBrowser) MoveDown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cursor < len(b.flat)-1 {
		b.cursor++
	}
}

func (b *FileBrowser) ToggleExpand() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cursor < 0 || b.cursor >= len(b.flat) {
		return
	}
	node := b.flat[b.cursor].node
	if !node.IsDir {
		return
	}
	node.Expanded = !node.Expanded
	if node.Expanded && len(node.Children) == 0 {
		entries, err := os.ReadDir(node.Path)
		if err == nil {
			var dirs, files []os.DirEntry
			for _, e := range entries {
				if e.IsDir() {
					dirs = append(dirs, e)
				} else {
					files = append(files, e)
				}
			}
			sort.Slice(dirs, func(i, j int) bool {
				return strings.ToLower(dirs[i].Name()) < strings.ToLower(dirs[j].Name())
			})
			sort.Slice(files, func(i, j int) bool {
				return strings.ToLower(files[i].Name()) < strings.ToLower(files[j].Name())
			})
			for _, d := range dirs {
				node.Children = append(node.Children, &FileNode{
					Name:  d.Name(),
					Path:  filepath.Join(node.Path, d.Name()),
					IsDir: true,
				})
			}
			for _, f := range files {
				node.Children = append(node.Children, &FileNode{
					Name:  f.Name(),
					Path:  filepath.Join(node.Path, f.Name()),
					IsDir: false,
				})
			}
		}
	}
	b.rebuildFlatLocked()
}

func (b *FileBrowser) SelectedPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cursor < 0 || b.cursor >= len(b.flat) {
		return ""
	}
	return b.flat[b.cursor].node.Path
}

func (b *FileBrowser) SelectedIsDir() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cursor < 0 || b.cursor >= len(b.flat) {
		return false
	}
	return b.flat[b.cursor].node.IsDir
}

func (b *FileBrowser) SetRoot(path string) {
	b.mu.Lock()
	b.root = path
	b.tree = nil
	b.flat = nil
	b.cursor = 0
	b.scroll = 0
	b.loaded = false
	b.mu.Unlock()
	_ = b.Load()
}

func (b *FileBrowser) Root() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.root
}

func (b *FileBrowser) Cursor() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cursor
}

func (b *FileBrowser) FlatCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.flat)
}

func (b *FileBrowser) Loaded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loaded
}

func fileIcon(node *FileNode) string {
	if node.IsDir {
		if node.Expanded {
			return "📂"
		}
		return "📁"
	}
	ext := strings.ToLower(filepath.Ext(node.Name))
	switch ext {
	case ".go":
		return "🐹"
	case ".py":
		return "🐍"
	default:
		return "📄"
	}
}

func renderScrollbar(scroll, visible, total, width int) string {
	if total <= visible {
		return strings.Repeat(" ", width)
	}
	ratio := float64(scroll) / float64(total-visible)
	thumbPos := int(ratio * float64(visible))
	if thumbPos >= visible {
		thumbPos = visible - 1
	}
	var b strings.Builder
	for i := 0; i < visible; i++ {
		if i == thumbPos {
			b.WriteString("█")
		} else {
			b.WriteString("░")
		}
		b.WriteString("\n")
	}
	return b.String()
}
