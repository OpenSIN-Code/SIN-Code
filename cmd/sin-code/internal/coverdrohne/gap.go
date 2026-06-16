// SPDX-License-Identifier: MIT
// Purpose: gap analyzer — turns a Go coverage profile into a list of
// uncovered functions/blocks per package.
package coverdrohne

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// readFileHook is swappable for tests that exercise the go.mod error path.
var readFileHook = os.ReadFile

// openFileHook is swappable for tests that exercise the coverprofile open error path.
var openFileHook = os.Open

// modulePath returns the module import path declared in go.mod at root.
func modulePath(root string) string {
	data, err := readFileHook(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strings.Trim(fields[1], "\"'")
			}
		}
	}
	return ""
}

// PackageImportPath returns the Go import path for the package containing
// file, relative to root. If root has no go.mod, file is returned unchanged.
// This is a lightweight helper used by hooklife to map a written/edited .go
// file to the package that should be covered.
func PackageImportPath(root, file string) string {
	mod := modulePath(root)
	rel, err := filepath.Rel(root, file)
	if err != nil || strings.HasPrefix(rel, "..") {
		return file
	}
	if mod == "" {
		return file
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return mod
	}
	return mod + "/" + filepath.ToSlash(dir)
}

// profileFileToLocal converts a coverprofile file path (module import path)
// to a local filesystem path relative to root.
func profileFileToLocal(root, mod, file string) string {
	if mod != "" && strings.HasPrefix(file, mod+"/") {
		return filepath.Join(root, strings.TrimPrefix(file, mod+"/"))
	}
	if !filepath.IsAbs(file) {
		return filepath.Join(root, file)
	}
	return file
}

// Block is an uncovered or partially covered code block.
type Block struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	FuncName  string `json:"func_name"`
	NumStmts  int    `json:"num_stmts"`
	Count     int    `json:"count"`
}

// Gap holds uncovered blocks for a single Go file.
type Gap struct {
	File   string  `json:"file"`
	Blocks []Block `json:"blocks"`
}

// Gaps parses a coverage profile and returns uncovered blocks grouped by file.
// Only blocks with count == 0 are returned. srcRoot is the filesystem root used
// to resolve the file paths in the profile.
func Gaps(profilePath, srcRoot string) ([]Gap, error) {
	f, err := openFileHook(profilePath)
	if err != nil {
		return nil, fmt.Errorf("open coverprofile: %w", err)
	}
	defer f.Close()

	mod := modulePath(srcRoot)

	var gaps []Gap
	var currentGap *Gap
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			if !strings.HasPrefix(line, "mode: ") {
				return nil, fmt.Errorf("invalid coverprofile: missing mode line")
			}
			first = false
			continue
		}
		block, err := parseProfileLine(line)
		if err != nil {
			return nil, err
		}
		if block == nil || block.Count != 0 {
			continue
		}
		localFile := profileFileToLocal(srcRoot, mod, block.File)
		block.FuncName, _ = funcNameForBlock(srcRoot, localFile, block.StartLine)

		rel, _ := filepath.Rel(srcRoot, localFile)
		if rel == "" {
			rel = block.File
		}
		block.File = rel

		if currentGap == nil || currentGap.File != block.File {
			currentGap = &Gap{File: block.File}
			gaps = append(gaps, *currentGap)
			currentGap = &gaps[len(gaps)-1]
		}
		currentGap.Blocks = append(currentGap.Blocks, *block)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return gaps, nil
}

func parseProfileLine(line string) (*Block, error) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid profile line: %q", line)
	}
	filePos := parts[0]
	numStmts, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid statement count in profile line: %q", line)
	}
	count, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid count in profile line: %q", line)
	}
	file, startLine, endLine, err := parseFilePos(filePos)
	if err != nil {
		return nil, err
	}
	return &Block{
		File:      file,
		StartLine: startLine,
		EndLine:   endLine,
		NumStmts:  numStmts,
		Count:     count,
	}, nil
}

func parseFilePos(filePos string) (file string, startLine, endLine int, err error) {
	colonIdx := strings.LastIndex(filePos, ":")
	if colonIdx < 0 {
		return "", 0, 0, fmt.Errorf("invalid file position: %q", filePos)
	}
	file = filePos[:colonIdx]
	pos := filePos[colonIdx+1:]
	posParts := strings.Split(pos, ",")
	if len(posParts) != 2 {
		return "", 0, 0, fmt.Errorf("invalid position range: %q", pos)
	}
	startLine, err = lineFromPos(posParts[0])
	if err != nil {
		return "", 0, 0, err
	}
	endLine, err = lineFromPos(posParts[1])
	if err != nil {
		return "", 0, 0, err
	}
	return file, startLine, endLine, nil
}

func lineFromPos(pos string) (int, error) {
	dotIdx := strings.Index(pos, ".")
	if dotIdx < 0 {
		return strconv.Atoi(pos)
	}
	return strconv.Atoi(pos[:dotIdx])
}

func funcNameForBlock(root, file string, line int) (string, error) {
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, file)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, abs, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}
	var name string
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			start := fset.Position(x.Pos()).Line
			end := fset.Position(x.End()).Line
			if start <= line && line <= end {
				name = funcDeclName(x)
				return false
			}
		case *ast.FuncLit:
			start := fset.Position(x.Pos()).Line
			end := fset.Position(x.End()).Line
			if start <= line && line <= end {
				name = "<anonymous func>"
				return false
			}
		}
		return true
	})
	return name, nil
}

func funcDeclName(f *ast.FuncDecl) string {
	if f.Recv == nil {
		return f.Name.Name
	}
	var recv string
	if len(f.Recv.List) > 0 {
		t := f.Recv.List[0].Type
		switch n := t.(type) {
		case *ast.Ident:
			recv = n.Name
		case *ast.StarExpr:
			switch m := n.X.(type) {
			case *ast.Ident:
				recv = m.Name
			case *ast.IndexExpr:
				if id, ok := m.X.(*ast.Ident); ok {
					recv = id.Name
				}
			}
		case *ast.IndexExpr:
			if id, ok := n.X.(*ast.Ident); ok {
				recv = id.Name
			}
		}
	}
	if recv == "" {
		return f.Name.Name
	}
	return "(" + recv + ")." + f.Name.Name
}
