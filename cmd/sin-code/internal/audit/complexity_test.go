package audit

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoFilesSkipsVendorAndTests(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "main.go"), "package main\n")
	mustWrite(t, filepath.Join(dir, "main_test.go"), "package main\n")
	mustWrite(t, filepath.Join(dir, "vendor", "x.go"), "package x\n")
	mustWrite(t, filepath.Join(dir, "nested", "y.go"), "package nested\n")
	_ = os.MkdirAll(filepath.Join(dir, "node_modules"), 0755)

	files, err := goFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 .go files, got %d: %v", len(files), files)
	}
}

func TestDetectSingleImplInterface(t *testing.T) {
	src := `package foo
type Reader interface { Read() }`
	f, fset := parseFile(t, "x.go", src)
	findings := staticPass(packageInfo{f: f, src: src, fset: fset, path: "x.go"}, allTagsMap(), DefaultSinDebtRE())
	found := false
	for _, f := range findings {
		if f.Tag == TagYagni && strings.Contains(f.Problem, "Reader") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected yagni interface finding, got %v", findings)
	}
}

func TestDetectSingleProductFactory(t *testing.T) {
	src := `package foo
func NewThing() *Thing { return &Thing{} }
type Thing struct{}`
	f, fset := parseFile(t, "x.go", src)
	findings := staticPass(packageInfo{f: f, src: src, fset: fset, path: "x.go"}, allTagsMap(), DefaultSinDebtRE())
	found := false
	for _, f := range findings {
		if f.Tag == TagYagni && strings.Contains(f.Problem, "NewThing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected yagni factory finding, got %v", findings)
	}
}

func TestDetectWrapper(t *testing.T) {
	src := `package foo
func Wrap(s string) string { return lower(s) }
func lower(s string) string { return strings.ToLower(s) }`
	f, fset := parseFile(t, "x.go", src)
	findings := staticPass(packageInfo{f: f, src: src, fset: fset, path: "x.go"}, allTagsMap(), DefaultSinDebtRE())
	found := false
	for _, f := range findings {
		if f.Tag == TagShrink && strings.Contains(f.Problem, "Wrap") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected shrink wrapper finding, got %v", findings)
	}
}

func TestDetectOneExportFile(t *testing.T) {
	src := `package foo
func Exported() {}`
	f, fset := parseFile(t, "x.go", src)
	findings := staticPass(packageInfo{f: f, src: src, fset: fset, path: "x.go"}, allTagsMap(), DefaultSinDebtRE())
	found := false
	for _, f := range findings {
		if f.Tag == TagShrink && strings.Contains(f.Problem, "exports only one") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected one-export finding, got %v", findings)
	}
}

func TestDetectDeadFlag(t *testing.T) {
	src := `package foo
var VerboseFlag bool`
	f, fset := parseFile(t, "x.go", src)
	findings := staticPass(packageInfo{f: f, src: src, fset: fset, path: "x.go"}, allTagsMap(), DefaultSinDebtRE())
	found := false
	for _, f := range findings {
		if f.Tag == TagDelete && strings.Contains(f.Problem, "VerboseFlag") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected dead flag finding, got %v", findings)
	}
}

func TestDetectHandRolledStdlib(t *testing.T) {
	src := `package foo
func use(s, sub string) bool { return contains(s, sub) }
func contains(s, sub string) bool { return strings.Contains(s, sub) }`
	f, fset := parseFile(t, "x.go", src)
	findings := staticPass(packageInfo{f: f, src: src, fset: fset, path: "x.go"}, allTagsMap(), DefaultSinDebtRE())
	found := false
	for _, f := range findings {
		if f.Tag == TagStdlib && strings.Contains(f.Problem, "contains") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected stdlib hand-roll finding, got %v", findings)
	}
}

func TestSinDebtApproval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	mustWrite(t, path, "package foo\n// sin-debt: legacy shim\ntype Reader interface { Read() }\n")
	re := DefaultSinDebtRE()

	f, fset := parseFile(t, path, string(mustRead(t, path)))
	src := string(mustRead(t, path))
	findings := staticPass(packageInfo{f: f, src: src, fset: fset, path: path}, allTagsMap(), re)
	var target Finding
	for _, f := range findings {
		if f.Tag == TagYagni && strings.Contains(f.Problem, "Reader") {
			target = f
			break
		}
	}
	if target.Path == "" {
		t.Fatalf("expected yagni Reader finding, got %v", findings)
	}
	approved, approver := approvedBySinDebt(target, re)
	if !approved || approver != "legacy shim" {
		t.Fatalf("expected approval 'legacy shim', got %v %q", approved, approver)
	}
}

func TestAggregateEmpty(t *testing.T) {
	r := aggregate(nil, 0)
	if r.Status != "Lean already. Ship." || r.NetLines != 0 {
		t.Fatalf("expected lean status, got %+v", r)
	}
}

func TestAggregateNetLines(t *testing.T) {
	findings := []Finding{
		{Tag: TagDelete, LineCount: 50, Approved: false},
		{Tag: TagStdlib, LineCount: 30, Approved: false},
		{Tag: TagYagni, LineCount: 20, Approved: true, Approver: "kept"},
	}
	r := aggregate(findings, 0)
	if r.NetLines != 80 || r.DepsRemovable != 1 {
		t.Fatalf("expected net 80/1, got %+v", r)
	}
	if !strings.HasPrefix(r.Status, "net: -80") {
		t.Fatalf("unexpected status: %s", r.Status)
	}
}

func TestAuditIntegration(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo

type Reader interface { Read() }
func NewReader() Reader { return &reader{} }
type reader struct{}
func (r *reader) Read() {}
`)

	aud := NewAuditor(nil)
	opts := Options{NoLLM: true, Tags: []string{TagYagni}}
	res, err := aud.Audit(context.Background(), dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected findings")
	}
	if res.Status == "Lean already. Ship." {
		t.Fatal("expected non-lean status")
	}
}

func TestAuditStrictThreshold(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo
var VerboseFlag bool
var DebugConfig string
`)
	aud := NewAuditor(nil)
	opts := Options{NoLLM: true, Tags: []string{TagDelete}, Strict: true, MaxNet: 1}
	_, err := aud.Audit(context.Background(), dir, opts)
	if err == nil {
		t.Fatal("expected strict threshold error")
	}
}

func TestValidateTags(t *testing.T) {
	if err := ValidateTags([]string{"delete", "shrink"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTags([]string{"bad"}); err == nil {
		t.Fatal("expected error for unknown tag")
	}
}

func TestFormatFinding(t *testing.T) {
	f := Finding{Tag: TagShrink, Problem: "x", Replacement: "y", Path: "p.go", Line: 7}
	got := FormatFinding(f)
	want := "shrink: x. y. [p.go:7]"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestAuditRankDeps(t *testing.T) {
	findings := []Finding{
		{Tag: TagDelete, LineCount: 100},
		{Tag: TagStdlib, LineCount: 10},
		{Tag: TagNative, LineCount: 20},
	}
	r := aggregate(findings, 0)
	if r.NetLines != 130 || r.DepsRemovable != 2 {
		t.Fatalf("expected 130/2, got %+v", r)
	}
}

func allTagsMap() map[string]bool {
	m := make(map[string]bool)
	for _, t := range allTags {
		m[t] = true
	}
	return m
}

func parseFile(t *testing.T, path, src string) (*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return f, fset
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDefaultSinDebtRE(t *testing.T) {
	re := DefaultSinDebtRE()
	if !re.MatchString("// sin-debt: reason") {
		t.Fatal("expected match")
	}
}

func TestTagRank(t *testing.T) {
	if tagRank(TagStdlib) >= tagRank(TagShrink) {
		t.Fatal("stdlib should rank before shrink")
	}
}

func TestAuditLLMStub(t *testing.T) {
	stub := &llmStub{extra: []Finding{{Tag: TagDelete, Problem: "stub", Replacement: "remove", Path: "x.go", Line: 1, LineCount: 1}}}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "x.go"), "package demo\nfunc Unused() {}\n")
	aud := NewAuditor(stub)
	// Include shrink so static pass generates a candidate for the LLM pass.
	opts := Options{TopN: 1, Tags: []string{TagDelete, TagShrink}}
	res, err := aud.Audit(context.Background(), dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range res.Findings {
		if f.Problem == "stub" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected LLM stub finding")
	}
}

type llmStub struct {
	extra []Finding
}

func (l *llmStub) Judge(ctx context.Context, filePath, content string, candidates []Finding) ([]Finding, error) {
	return l.extra, nil
}

func TestApprovedFindingsExcludedFromNet(t *testing.T) {
	findings := []Finding{
		{Tag: TagDelete, LineCount: 100, Approved: true, Approver: "ok"},
	}
	r := aggregate(findings, 0)
	if r.NetLines != 0 {
		t.Fatalf("expected approved finding to not count, got %d", r.NetLines)
	}
}
