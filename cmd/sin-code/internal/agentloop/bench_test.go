// SPDX-License-Identifier: MIT
// Purpose: performance benchmarks for the agent loop and related subsystems.
// All benchmarks use stubs/fakes — no real API calls (mandate M3).
// Run with: go test -bench=. -benchmem -count=1 -timeout 60s ./cmd/sin-code/internal/agentloop/...
package agentloop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func benchSession(b *testing.B) *session.Session {
	b.Helper()
	store, err := session.Open(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { store.Close() })
	sess, err := store.StartOrResume("")
	if err != nil {
		b.Fatal(err)
	}
	return sess
}

func BenchmarkLoopRun_Stub(b *testing.B) {
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sess := benchSession(b)
		b.StartTimer()
		if _, err := loop.Run(context.Background(), sess, "benchmark prompt"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompaction_Hybrid(b *testing.B) {
	compactor := NewCompactor(nil)
	msgs := make([]session.Message, 100)
	for i := range msgs {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		msgs[i] = session.Message{
			Role:    role,
			Content: strings.Repeat("x", 200),
		}
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compactor.Compact(ctx, msgs, CompactionHybrid, 8000)
	}
}

type benchPromptCache struct {
	mu      sync.RWMutex
	entries map[string]string
}

func newBenchPromptCache() *benchPromptCache {
	c := &benchPromptCache{entries: make(map[string]string)}
	c.Set("cached-key", "prefix-id-123")
	return c
}

func (c *benchPromptCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.entries[key]
	return v, ok
}

func (c *benchPromptCache) Set(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = val
}

func BenchmarkPromptCache_HitPath(b *testing.B) {
	cache := newBenchPromptCache()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get("cached-key")
	}
}

func BenchmarkFrustrationDetection(b *testing.B) {
	detector := NewFrustrationDetector()
	messages := []string{
		"why doesn't this work?",
		"this is broken again",
		"stop doing that",
		"not helpful",
		"seriously, what the hell",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := messages[i%len(messages)]
		detector.Track(msg, time.Now())
	}
}

type benchDAGNode struct {
	ID          string
	Type        string
	DependsOn   []string
	Probability float64
}

type benchDAGPlan struct {
	Nodes []benchDAGNode
}

func buildBenchDAGPlan(prompt string) *benchDAGPlan {
	nodes := []benchDAGNode{
		{ID: "architect", Type: "architect", Probability: 1.0},
		{ID: "coder-1", Type: "coder", DependsOn: []string{"architect"}, Probability: 0.95},
		{ID: "coder-2", Type: "coder", DependsOn: []string{"architect"}, Probability: 0.90},
		{ID: "tester", Type: "tester", DependsOn: []string{"coder-1", "coder-2"}, Probability: 0.85},
		{ID: "reviewer", Type: "reviewer", DependsOn: []string{"coder-1", "coder-2"}, Probability: 0.70},
		{ID: "docs", Type: "docs", DependsOn: []string{"coder-2"}, Probability: 0.50},
	}
	hash := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(hash[:8])
	for i := range nodes {
		nodes[i].ID = nodes[i].ID + "-" + promptHash
	}
	return &benchDAGPlan{Nodes: nodes}
}

func BenchmarkDeepPlanner_BuildDAGPlan(b *testing.B) {
	prompt := "implement JWT authentication with refresh tokens and rate limiting"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildBenchDAGPlan(prompt)
	}
}

type benchPattern struct {
	TaskType    string
	Probability float64
	Position    int
}

type benchPatternDB struct {
	mu       sync.RWMutex
	patterns map[string][]benchPattern
}

func newBenchPatternDB() *benchPatternDB {
	db := &benchPatternDB{patterns: make(map[string][]benchPattern)}
	db.patterns["implement auth"] = []benchPattern{
		{TaskType: "architect", Probability: 1.0, Position: 0},
		{TaskType: "coder", Probability: 0.95, Position: 1},
		{TaskType: "tester", Probability: 0.85, Position: 2},
		{TaskType: "reviewer", Probability: 0.70, Position: 3},
	}
	db.patterns["fix bug"] = []benchPattern{
		{TaskType: "coder", Probability: 0.90, Position: 0},
		{TaskType: "tester", Probability: 0.85, Position: 1},
	}
	return db
}

func (db *benchPatternDB) MatchPrompt(prompt string) []benchPattern {
	db.mu.RLock()
	defer db.mu.RUnlock()
	lower := strings.ToLower(prompt)
	for key, pats := range db.patterns {
		if strings.Contains(lower, key) {
			return pats
		}
	}
	return nil
}

func BenchmarkPatternDB_MatchPrompt(b *testing.B) {
	db := newBenchPatternDB()
	prompts := []string{
		"implement auth for the API",
		"fix bug in the parser",
		"refactor the database layer",
		"add tests for the handler",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.MatchPrompt(prompts[i%len(prompts)])
	}
}

type benchToolSpec struct {
	Name        string
	Description string
}

type benchLazyToolLoader struct {
	specs []benchToolSpec
}

func newBenchLazyToolLoader() *benchLazyToolLoader {
	specs := []benchToolSpec{
		{"sin_read", "Read a file from the local filesystem"},
		{"sin_write", "Write a file to the local filesystem atomically"},
		{"sin_edit", "Surgical file edit using AST-anchored edits"},
		{"sin_scout", "Search code with regex semantic symbol and usage search"},
		{"sin_bash", "Execute shell commands safely with secret redaction"},
		{"sin_grasp", "Deep code understanding for a single file"},
		{"sin_map", "Map code architecture with dependency graphs"},
		{"sin_discover", "Discover files with relevance scoring"},
		{"sin_poc", "Proof of correctness verify code satisfies specification"},
		{"sin_oracle", "Verification Oracle independent verification of claims"},
		{"sin_efm", "Ephemeral full stack mocking spin up disposable test envs"},
		{"sin_sckg", "Semantic codebase knowledge graphs build and query code graph"},
		{"sin_adw", "Architectural debt watchdogs detect god modules circular deps"},
		{"sin_ibd", "Intent based diffing compare code changes against stated intent"},
		{"sin_harvest", "Fetch URLs with caching and structure extraction"},
		{"sin_execute", "Execute shell commands safely with secret redaction"},
	}
	return &benchLazyToolLoader{specs: specs}
}

func (l *benchLazyToolLoader) Search(query string, k int) []benchToolSpec {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || k <= 0 {
		return nil
	}
	tokens := strings.Fields(query)
	type scored struct {
		spec  benchToolSpec
		score int
	}
	var results []scored
	for _, spec := range l.specs {
		name := strings.ToLower(spec.Name)
		desc := strings.ToLower(spec.Description)
		score := 0
		for _, tok := range tokens {
			if name == tok {
				score += 10
			}
			if strings.Contains(name, tok) {
				score += 5
			}
			if strings.Contains(desc, tok) {
				score += 1
			}
		}
		if score > 0 {
			results = append(results, scored{spec: spec, score: score})
		}
	}
	if len(results) == 0 {
		return nil
	}
	if len(results) > k {
		results = results[:k]
	}
	out := make([]benchToolSpec, len(results))
	for i, r := range results {
		out[i] = r.spec
	}
	return out
}

func BenchmarkLazyToolLoader_Search(b *testing.B) {
	loader := newBenchLazyToolLoader()
	queries := []string{
		"read file",
		"execute command",
		"search code",
		"edit surgical",
		"verify proof",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loader.Search(queries[i%len(queries)], 5)
	}
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

func benchHighlightGo(code string) string {
	var b strings.Builder
	b.Grow(len(code) * 2)
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			b.WriteString("[comment]")
			b.WriteString(line)
			b.WriteString("[/comment]")
			b.WriteByte('\n')
			continue
		}
		words := strings.Fields(line)
		for _, word := range words {
			clean := strings.Trim(word, "();{},")
			if goKeywords[clean] {
				b.WriteString("[kw]")
				b.WriteString(word)
				b.WriteString("[/kw] ")
			} else if strings.HasPrefix(word, "\"") {
				b.WriteString("[str]")
				b.WriteString(word)
				b.WriteString("[/str] ")
			} else {
				b.WriteString(word)
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func BenchmarkSyntaxHighlight_Go(b *testing.B) {
	lines := make([]string, 100)
	keywords := []string{"func", "var", "const", "if", "for", "return", "type", "struct", "package", "import"}
	for i := 0; i < 100; i++ {
		if i%10 == 0 {
			lines[i] = "// comment line " + string(rune('0'+i%10))
		} else {
			kw := keywords[i%len(keywords)]
			lines[i] = kw + " name" + string(rune('0'+i%10)) + " = \"value\""
		}
	}
	code := strings.Join(lines, "\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchHighlightGo(code)
	}
}

type benchStreamingBuffer struct {
	mu   sync.Mutex
	text strings.Builder
}

func newBenchStreamingBuffer() *benchStreamingBuffer {
	return &benchStreamingBuffer{}
}

func (b *benchStreamingBuffer) Append(text string) {
	if text == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text.WriteString(text)
}

func BenchmarkStreamingBuffer_Append(b *testing.B) {
	buf := newBenchStreamingBuffer()
	chunk := "streaming text chunk with some content "
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Append(chunk)
	}
}

func BenchmarkPermissionCheck_RiskClassifier(b *testing.B) {
	classifier := permission.NewRiskClassifier()
	tools := []struct {
		name string
		args map[string]any
	}{
		{"sin_read", nil},
		{"sin_write", nil},
		{"sin_bash", map[string]any{"command": "ls -la"}},
		{"sin_git_push", nil},
		{"sin_edit", nil},
		{"sin_scout", nil},
		{"sin_bash", map[string]any{"command": "rm -rf /tmp/old"}},
		{"unknown_tool", nil},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc := tools[i%len(tools)]
		classifier.Classify(tc.name, tc.args)
	}
}
