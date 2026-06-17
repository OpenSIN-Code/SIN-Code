package tui

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

const sampleDiff = `--- a/main.go
+++ b/main.go
@@ -10,4 +10,5 @@
 func main() {
 	fmt.Println("hello")
-	fmt.Println("world")
+	fmt.Println("universe")
+	fmt.Println("!")
 }
`

func TestParseDiffAddedLines(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(sampleDiff)
	var added []DiffLine
	for _, l := range lines {
		if l.Kind == DiffLineAdded {
			added = append(added, l)
		}
	}
	if len(added) != 2 {
		t.Fatalf("expected 2 added lines, got %d", len(added))
	}
	if added[0].Content != "\tfmt.Println(\"universe\")" {
		t.Errorf("expected universe content, got %q", added[0].Content)
	}
	if added[1].Content != "\tfmt.Println(\"!\")" {
		t.Errorf("expected ! content, got %q", added[1].Content)
	}
}

func TestParseDiffRemovedLines(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(sampleDiff)
	var removed []DiffLine
	for _, l := range lines {
		if l.Kind == DiffLineRemoved {
			removed = append(removed, l)
		}
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed line, got %d", len(removed))
	}
	if removed[0].Content != "\tfmt.Println(\"world\")" {
		t.Errorf("expected world content, got %q", removed[0].Content)
	}
}

func TestParseDiffContextLines(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(sampleDiff)
	var ctx []DiffLine
	for _, l := range lines {
		if l.Kind == DiffLineContext {
			ctx = append(ctx, l)
		}
	}
	found := false
	for _, l := range ctx {
		if strings.Contains(l.Content, "func main()") {
			found = true
		}
	}
	if !found {
		t.Error("expected context line with 'func main()'")
	}
}

func TestParseDiffHunkHeader(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(sampleDiff)
	var hunks []DiffLine
	for _, l := range lines {
		if l.Kind == DiffLineHunk {
			hunks = append(hunks, l)
		}
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk header, got %d", len(hunks))
	}
	if !strings.HasPrefix(hunks[0].Content, "@@") {
		t.Errorf("expected @@ prefix, got %q", hunks[0].Content)
	}
}

func TestParseDiffEmpty(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff("")
	if lines != nil {
		t.Errorf("expected nil for empty diff, got %v", lines)
	}
	lines = r.ParseDiff("no diff content here")
	if len(lines) == 0 {
		t.Error("expected non-empty result for non-diff text")
	}
	for _, l := range lines {
		if l.Kind != DiffLineContext {
			t.Errorf("expected all context lines for non-diff text, got kind %d", l.Kind)
		}
	}
}

func TestParseDiffMultiHunk(t *testing.T) {
	multiHunk := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 line1
-old1
+new1
 line2
@@ -10,3 +10,3 @@
 line10
-old10
+new10
 line11
`
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(multiHunk)
	var hunks []DiffLine
	for _, l := range lines {
		if l.Kind == DiffLineHunk {
			hunks = append(hunks, l)
		}
	}
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunk headers, got %d", len(hunks))
	}
}

func TestRenderColorsApplied(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(sampleDiff)
	output := r.Render(lines, testStyles(), 80)
	if !strings.Contains(output, "\x1b[") {
		t.Error("expected ANSI color codes in rendered output")
	}
	if !strings.Contains(output, "+2") {
		t.Error("expected added count in stats line")
	}
	if !strings.Contains(output, "-1") {
		t.Error("expected removed count in stats line")
	}
}

func TestRenderLineNumbers(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(sampleDiff)
	output := r.Render(lines, testStyles(), 80)
	stripped := stripANSI(output)
	foundOld := false
	foundNew := false
	for _, l := range lines {
		if l.Kind == DiffLineAdded && l.NewLine > 0 {
			if strings.Contains(stripped, fmt.Sprintf("%d", l.NewLine)) {
				foundNew = true
			}
		}
		if l.Kind == DiffLineRemoved && l.OldLine > 0 {
			if strings.Contains(stripped, fmt.Sprintf("%d", l.OldLine)) {
				foundOld = true
			}
		}
	}
	if !foundNew {
		t.Error("expected new line numbers in rendered output")
	}
	if !foundOld {
		t.Error("expected old line numbers in rendered output")
	}
}

func TestRenderWidthTruncation(t *testing.T) {
	longDiff := `--- a/file.go
+++ b/file.go
@@ -1,1 +1,1 @@
-this is a very long line that should definitely be truncated when rendered at a small width
+this is also a very long line that should be truncated when rendered at a small width value
`
	r := NewDiffRenderer(testStyles())
	lines := r.ParseDiff(longDiff)
	output := r.Render(lines, testStyles(), 30)
	if !strings.Contains(output, "...") {
		t.Error("expected truncation indicator '...' in output")
	}
}

func TestRenderCompactOnlyChanges(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	output := r.RenderCompact(sampleDiff, testStyles(), 80)
	stripped := stripANSI(output)
	if strings.Contains(stripped, "func main()") {
		t.Error("compact mode should not show context lines")
	}
	if !strings.Contains(stripped, "universe") {
		t.Error("expected added line content in compact output")
	}
	if !strings.Contains(stripped, "world") {
		t.Error("expected removed line content in compact output")
	}
}

func TestRenderCompactCollapseLarge(t *testing.T) {
	var b strings.Builder
	b.WriteString("--- a/big.go\n+++ b/big.go\n@@ -1,60 +1,60 @@\n")
	for i := 0; i < 60; i++ {
		b.WriteString("-old line ")
		b.WriteString(itoa(i))
		b.WriteString("\n")
		b.WriteString("+new line ")
		b.WriteString(itoa(i))
		b.WriteString("\n")
	}

	r := NewDiffRenderer(testStyles())
	output := r.RenderCompact(b.String(), testStyles(), 80)
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "...") {
		t.Error("expected collapse indicator '...' in output for large diff")
	}
	if !strings.Contains(stripped, "more") {
		t.Error("expected 'more' text in collapsed output")
	}
}

func TestDiffRendererConcurrentAccess(t *testing.T) {
	r := NewDiffRenderer(testStyles())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lines := r.ParseDiff(sampleDiff)
			_ = r.Render(lines, testStyles(), 80)
			_ = r.RenderCompact(sampleDiff, testStyles(), 80)
		}()
	}
	wg.Wait()
}
