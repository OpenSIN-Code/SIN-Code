// SPDX-License-Identifier: MIT
// Purpose: supplementary unit tests that lift the instinct package's
// coverage from 22.9% to a healthier baseline. These tests cover the
// observation → classifier → extractor → manager → audit pipeline
// end-to-end (sans LLM) and the ECC frontmatter round-trip + the
// three documented Unmarshal failure modes. Stdlib only.
package instinct

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// 1. Classify domain taxonomy across all 12 representative inputs.
func TestClassify_DomainTaxonomy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		tool string
		meta map[string]string
		want string
	}{
		{"bash-git-commit", "Bash", map[string]string{"command": "git commit -m fix"}, "git"},
		{"bash-go-test", "Bash", map[string]string{"command": "go test ./..."}, "testing"},
		{"bash-docker", "Bash", map[string]string{"command": "docker run alpine"}, "infra"},
		{"edit-go-test-suffix", "Edit", map[string]string{"path": "/x/foo_test.go"}, "testing"},
		{"edit-env-secrets", "Edit", map[string]string{"path": "/x/.env"}, "security"},
		{"edit-secret-txt", "Edit", map[string]string{"path": "/x/secret.txt"}, "security"},
		{"edit-main-go", "Edit", map[string]string{"path": "/x/main.go"}, "go"},
		{"edit-app-tsx", "Edit", map[string]string{"path": "/x/app.tsx"}, "typescript"},
		{"edit-no-meta", "Edit", nil, "code-style"},
		{"read-tool", "Read", nil, "navigation"},
		{"unknown-tool-empty-meta", "MysteryTool", map[string]string{}, "general"},
		{"bash-pytest", "Bash", map[string]string{"command": "pytest -k foo"}, "testing"},
		{"bash-make", "Bash", map[string]string{"command": "make all"}, "build"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(c.tool, c.meta); got != c.want {
				t.Errorf("Classify(%q, %v) = %q, want %q", c.tool, c.meta, got, c.want)
			}
		})
	}
}

// 2. HeuristicExtractor: only one of six observations crosses the
// MinRepeats threshold; single buckets, failures, and empty-actions
// are filtered out. Re-runs are byte-identical.
func TestHeuristicExtractor_DeterministicOutput(t *testing.T) {
	t.Parallel()
	//   2x  (git, "commit code", Success=true)   → eligible bucket, count=2
	//   1x  (build, "make all", Success=true)    → single, filtered out
	//   1x  (testing, "run go test", Success=false) → skipped (Success=false)
	//   1x  (security, "", Success=true)          → skipped (empty Action)
	//   1x  (testing, "run pytest", Success=true) → single, filtered out
	obs := []Observation{
		{Tool: "Bash", Action: "commit code", Domain: "git", Success: true, Meta: map[string]string{"command": "git commit -m 'a'"}},
		{Tool: "Bash", Action: "commit code", Domain: "git", Success: true, Meta: map[string]string{"command": "git commit -m 'b'"}},
		{Tool: "Bash", Action: "make all", Domain: "build", Success: true, Meta: map[string]string{"command": "make all"}},
		{Tool: "Bash", Action: "run go test", Domain: "testing", Success: false, Meta: map[string]string{"command": "go test"}},
		{Tool: "Edit", Action: "", Domain: "security", Success: true, Meta: map[string]string{"path": "/x/secret.yaml"}},
		{Tool: "Bash", Action: "run pytest", Domain: "testing", Success: true, Meta: map[string]string{"command": "pytest -k foo"}},
	}
	h := HeuristicExtractor{MinRepeats: 2}
	cands, err := h.Extract(context.Background(), obs)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate (others filtered), got %d: %+v", len(cands), cands)
	}
	got := cands[0]
	if got.Domain != "git" || got.Action != "commit code" {
		t.Errorf("candidate = %+v, want git/commit code", got)
	}
	if got.Trigger == "" || len(got.Evidence) == 0 {
		t.Errorf("candidate lacks Trigger or Evidence: %+v", got)
	}

	first, err := h.Extract(context.Background(), obs)
	if err != nil {
		t.Fatalf("Extract(first): %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("Extract(first): count = %d, want 1", len(first))
	}
	for i := 0; i < 50; i++ {
		again, err := h.Extract(context.Background(), obs)
		if err != nil {
			t.Fatalf("Extract[%d]: %v", i, err)
		}
		if len(again) != len(first) {
			t.Fatalf("iter %d: count drift %d != %d", i, len(again), len(first))
		}
		if !candidateEqual(&again[0], &first[0]) {
			t.Fatalf("iter %d: candidate drift\nfirst=%+v\nagain=%+v", i, first[0], again[0])
		}
	}
}

func candidateEqual(a, b *Candidate) bool {
	if a.Domain != b.Domain || a.Action != b.Action || a.Trigger != b.Trigger {
		return false
	}
	if len(a.Evidence) != len(b.Evidence) {
		return false
	}
	for i := range a.Evidence {
		if a.Evidence[i] != b.Evidence[i] {
			return false
		}
	}
	return true
}

// 3. Manager.Observe: first call creates; same-signature second call
// reinforces; Contradict emits "contradicted"; no-match is a no-op.
func TestManager_Observe_ReinforceVsCreate(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	project := Project{ID: "test-proj", Name: "test-proj"}
	mgr := NewManagerWithStore(store, project, nil)

	c := Candidate{
		Trigger:  "when making a commit",
		Domain:   "git",
		Action:   "commit code",
		Evidence: []string{"Bash: git commit -m 'a'"},
	}
	created, err := mgr.Observe(c)
	if err != nil {
		t.Fatalf("first Observe: %v", err)
	}
	if !created {
		t.Fatalf("first Observe should return created=true")
	}
	created2, err := mgr.Observe(c)
	if err != nil {
		t.Fatalf("second Observe: %v", err)
	}
	if created2 {
		t.Fatalf("second Observe should return created=false")
	}
	if err := mgr.Contradict(c); err != nil {
		t.Fatalf("Contradict: %v", err)
	}

	events, err := store.ReadAudit(0)
	if err != nil {
		t.Fatalf("ReadAudit: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 audit events (created, reinforced, contradicted), got %d: %+v", len(events), events)
	}
	want := []string{"created", "reinforced", "contradicted"}
	for i, w := range want {
		if events[i].Kind != w {
			t.Errorf("events[%d].Kind = %q, want %q", i, events[i].Kind, w)
		}
		if events[i].InstinctID == "" {
			t.Errorf("events[%d]: InstinctID must be non-empty", i)
		}
	}

	instincts, err := store.LoadProject("test-proj")
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if len(instincts) != 1 {
		t.Fatalf("expected 1 persisted instinct, got %d", len(instincts))
	}
	got := instincts[0]
	if got.Domain != "git" {
		t.Errorf("Domain = %q, want %q", got.Domain, "git")
	}
	if got.Contradictions != 1 {
		t.Errorf("Contradictions = %d, want 1", got.Contradictions)
	}
	if got.Observations != 2 {
		t.Errorf("Observations = %d, want 2 (initial 1 + 1 reinforce)", got.Observations)
	}

	preAudit, _ := store.ReadAudit(0)
	bogus := Candidate{
		Trigger:  "when doing something nobody did",
		Domain:   "go",
		Action:   "write a serializer",
		Evidence: []string{"Edit on /x/main.go"},
	}
	if err := mgr.Contradict(bogus); err != nil {
		t.Fatalf("Contradict(no-match): %v", err)
	}
	postAudit, _ := store.ReadAudit(0)
	if len(postAudit) != len(preAudit) {
		t.Errorf("no-match Contradict must not append audit: before=%d after=%d", len(preAudit), len(postAudit))
	}
}

// 4. Observer: 10 goroutines × 100 Record calls are race-safe. After
// all goroutines join, Pending() == 1000, and Flush drains to
// (~created, 0, nil): the single git bucket emits 1 candidate, the
// first Observe creates (created>=1, reinforced=0).
func TestObserver_RecordAndFlush_Concurrent(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	mgr := NewManagerWithStore(store, Project{ID: "race-proj", Name: "race-proj"}, nil)
	obs := NewObserver(mgr, HeuristicExtractor{MinRepeats: 2})

	const goroutines = 10
	const perGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			sample := Observation{
				Tool:    "Bash",
				Action:  "commit code",
				Domain:  "git",
				Success: true,
				Meta:    map[string]string{"command": "git commit -m x"},
			}
			for i := 0; i < perGoroutine; i++ {
				obs.Record(sample)
			}
		}()
	}
	wg.Wait()

	if got := obs.Pending(); got != goroutines*perGoroutine {
		t.Fatalf("Pending() = %d, want %d", got, goroutines*perGoroutine)
	}
	created, reinforced, err := obs.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if created < 1 {
		t.Errorf("created = %d, want >= 1", created)
	}
	if reinforced != 0 {
		t.Errorf("reinforced = %d, want 0", reinforced)
	}
	if got := obs.Pending(); got != 0 {
		t.Errorf("after Flush Pending() = %d, want 0", got)
	}
}

// 5. Frontmatter round-trip plus the three documented Unmarshal error
// modes: missing leading delimiter, unterminated frontmatter, and a
// malformed YAML body (wrapped with "parse frontmatter:").
func TestFrontmatter_RoundTrip(t *testing.T) {
	t.Parallel()
	src := NewInstinct("when committing", "git", "run linters then tests", "session-observation", ScopeProject)
	src.ID = src.computeID()
	src.Evidence = []string{
		"Bash: git commit -m 'a'",
		"Bash: git commit -m 'b'",
		"Bash: git commit -m 'c'",
	}
	src.Confidence = 0.50
	src.Observations = 2

	data, err := Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.ID != src.ID {
		t.Errorf("ID: %q != %q", out.ID, src.ID)
	}
	if out.Trigger != src.Trigger {
		t.Errorf("Trigger: %q != %q", out.Trigger, src.Trigger)
	}
	if out.Domain != src.Domain {
		t.Errorf("Domain: %q != %q", out.Domain, src.Domain)
	}
	if out.Action != src.Action {
		t.Errorf("Action: %q != %q", out.Action, src.Action)
	}
	if out.Source != src.Source {
		t.Errorf("Source: %q != %q", out.Source, src.Source)
	}
	if out.Scope != src.Scope {
		t.Errorf("Scope: %q != %q", out.Scope, src.Scope)
	}
	if out.Confidence != src.Confidence {
		t.Errorf("Confidence: %v != %v", out.Confidence, src.Confidence)
	}
	if out.Status != src.Status {
		t.Errorf("Status: %q != %q", out.Status, src.Status)
	}
	if out.Observations != src.Observations {
		t.Errorf("Observations: %d != %d", out.Observations, src.Observations)
	}
	if len(out.Evidence) != 3 {
		t.Fatalf("Evidence count: %d != 3", len(out.Evidence))
	}
	for i, e := range out.Evidence {
		if e != src.Evidence[i] {
			t.Errorf("Evidence[%d]: %q != %q", i, e, src.Evidence[i])
		}
	}

	if _, err := Unmarshal([]byte("id: x\ntrigger: y\n")); err == nil ||
		!strings.Contains(err.Error(), "missing frontmatter delimiter") {
		t.Errorf("missing-delim: want 'missing frontmatter delimiter', got: %v", err)
	}
	if _, err := Unmarshal([]byte("---\nid: x\ntrigger: y\n")); err == nil ||
		!strings.Contains(err.Error(), "unterminated frontmatter") {
		t.Errorf("unterminated: want 'unterminated frontmatter', got: %v", err)
	}

	bad := [][]byte{
		[]byte("---\nid: \"unclosed\ntrigger: y\n---\n\n# T\n"),
		[]byte("---\nid: [1, 2, 3\ntrigger: y\n---\n\n# T\n"),
		[]byte("---\nid: { broken\ntrigger: y\n---\n\n# T\n"),
		[]byte("---\n\n\tkey: tabbed\n---\n"),
	}
	var last error
	matched := false
	for _, b := range bad {
		_, err := Unmarshal(b)
		if err != nil && strings.Contains(err.Error(), "parse frontmatter:") {
			matched = true
			last = err
			break
		}
		if err != nil {
			last = err
		}
	}
	if !matched {
		t.Errorf("malformed YAML: want 'parse frontmatter:' wrapped error, last err: %v", last)
	}
}
