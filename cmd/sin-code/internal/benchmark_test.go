// SPDX-License-Identifier: MIT

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to create a synthetic project tree with N files, M lines per file.
func makeTree(b *testing.B, root string, files, linesPerFile int) {
	b.Helper()
	os.MkdirAll(root, 0750)
	content := makeGoFile(linesPerFile)
	for i := 0; i < files; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%d", i%10))
		os.MkdirAll(dir, 0750)
		name := filepath.Join(dir, fmt.Sprintf("file%d.go", i))
		os.WriteFile(name, []byte(content), 0644)
	}
}

func makeGoFile(lines int) string {
	var b strings.Builder
	b.WriteString("package pkg\n\n")
	for i := 0; i < lines-2; i++ {
		if i%5 == 0 {
			b.WriteString(fmt.Sprintf("func Func%d() {}\n", i))
		} else {
			b.WriteString(fmt.Sprintf("// comment line %d\n", i))
		}
	}
	return b.String()
}

// ── Scout Benchmarks ────────────────────────────────────────

func BenchmarkScout_FullScan_100files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 100, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := searchFiles(root, "func", "regex", 50, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScout_Indexed_100files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 100, 50)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	setFileIndex(idx)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := searchWithIndex(idx, root, "func", "regex", 50, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScout_FullScan_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := searchFiles(root, "func", "regex", 50, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScout_Indexed_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	setFileIndex(idx)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := searchWithIndex(idx, root, "func", "regex", 50, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScout_Symbol_Indexed_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	setFileIndex(idx)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := searchWithIndex(idx, root, "Func5", "symbol", 50, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── Grasp Benchmarks ────────────────────────────────────────

func BenchmarkGrasp_SmallFile(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "test.go")
	content := makeGoFile(50)
	os.WriteFile(path, []byte(content), 0644)
	info, _ := os.Stat(path)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzeFile(path, info)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGrasp_LargeFile_1000lines(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "test.go")
	content := makeGoFile(1000)
	os.WriteFile(path, []byte(content), 0644)
	info, _ := os.Stat(path)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzeFile(path, info)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── Map Benchmarks ────────────────────────────────────────

func BenchmarkMap_100files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 100, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mapArchitecture(root, "map")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMap_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mapArchitecture(root, "map")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── SCKG Benchmarks ───────────────────────────────────────

func BenchmarkSCKG_Build_100files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 100, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := buildGraph(root)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSCKG_Build_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := buildGraph(root)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── Index Benchmarks ────────────────────────────────────────

func BenchmarkIndex_Build_100files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 100, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx, err := buildIndex(root)
		if err != nil {
			b.Fatal(err)
		}
		_ = idx
	}
}

func BenchmarkIndex_Build_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx, err := buildIndex(root)
		if err != nil {
			b.Fatal(err)
		}
		_ = idx
	}
}

func BenchmarkIndex_SaveLoad_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	idx, _ := buildIndex(root)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := saveIndex(idx); err != nil {
			b.Fatal(err)
		}
		_, err := loadIndex(root)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIndex_Refresh_1000files(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 1000, 50)
	idx, _ := buildIndex(root)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, err := refreshIndex(idx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── ParseOutline Benchmarks ─────────────────────────────────

func BenchmarkParseOutline_Go_1000lines(b *testing.B) {
	content := makeGoFile(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseOutline("test.go", []byte(content))
	}
}

func BenchmarkParseOutline_Python_1000lines(b *testing.B) {
	content := makePythonFile(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseOutline("test.py", []byte(content))
	}
}

func BenchmarkParseOutline_JS_1000lines(b *testing.B) {
	content := makeJSFile(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseOutline("test.js", []byte(content))
	}
}

func makePythonFile(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		if i%5 == 0 {
			b.WriteString(fmt.Sprintf("def func%d():\n    pass\n", i))
		} else {
			b.WriteString(fmt.Sprintf("# comment %d\n", i))
		}
	}
	return b.String()
}

func makeJSFile(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		if i%5 == 0 {
			b.WriteString(fmt.Sprintf("function func%d() {}\n", i))
		} else {
			b.WriteString(fmt.Sprintf("// comment %d\n", i))
		}
	}
	return b.String()
}

// ── Benchmark Utility: Comparison Table ─────────────────────

func BenchmarkComparisonTable(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 500, 50)

	b.Run("fullscan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			searchFiles(root, "func", "regex", 50, true)
		}
	})

	idx, _ := buildIndex(root)
	b.Run("indexed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			searchWithIndex(idx, root, "func", "regex", 50, true)
		}
	})
}


