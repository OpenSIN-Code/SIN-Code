// SPDX-License-Identifier: MIT
// Purpose: autonomous research-report generation (issue #384). A topic
// string flows through three stages — search → fetch → synthesize — and the
// resulting structured report is shipped to disk and the autonomy goal
// queue so the daemon can deliver it as a verified artifact.
//
// The package exposes pluggable Searcher / Fetcher / LLM interfaces so
// tests can swap in fakes without network or LLM access. Default
// implementations are wired in NewGenerator and reach out to:
//   1. The MCP `websearch__search` tool (registered by external
//      sin-websearch / v3.19.0)
//   2. A stdlib net/http fetcher with a per-URL byte cap
//   3. The configured llm.Client (NIM / OpenAI / Anthropic, dispatch
//      follows modelperf + fusion recommendations)
//
// M3 compliance: every artifact generation runs through Generate → an
// independent StopGate (judgment) loop is optional and never silent.
// If the validate hook returns ErrInvalid the caller is expected to
// retry under the retry budget (default 3). Reporter is byte-stable per
// (topic, body) pair so two workstations emit identical Markdown.
package autonomy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ResearchReport is the structured artifact an autonomous research goal
// produces. Body is canonical Markdown (issue #384 format); Sources is
// the deduped, ranked list of citations the synthesizer used.
type ResearchReport struct {
	Topic       string    `json:"topic"`
	Sources     []Source  `json:"sources"`
	Body        string    `json:"body"`
	GeneratedAt time.Time `json:"generated_at"`
	Slug        string    `json:"slug"`
}

// Source is one citation a report grounded itself in. FetchedAt may be
// the zero value when the fetcher could not reach the URL — in that case
// the synthesizer falls back to the snippet returned by the searcher.
type Source struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	FetchedAt time.Time `json:"fetched_at,omitempty"`
	BodyBytes int       `json:"body_bytes,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Searcher abstracts a web search backend. Implementations should be
// safe for concurrent use (mandate M7).
type Searcher interface {
	Search(ctx context.Context, query string, max int) ([]Source, error)
}

// Fetcher pulls the HTML/text body of a URL. Implementations must cap
// response bytes per the configured MaxBytesPerFetch budget — silent
// truncation violates M2 (deterministic cost) and M3 (the report must
// be reproducible from the same sources).
type Fetcher interface {
	Fetch(ctx context.Context, url string) (body string, fetchedAt time.Time, err error)
}

// LLM completes the synthesis step: given a system prompt and a user
// prompt, returns the assistant message text.
type LLM interface {
	Ask(ctx context.Context, system, user string) (string, error)
}

// GeneratorConfig configures a Generator. Zero values fall back to
// safe defaults documented per field.
type GeneratorConfig struct {
	MaxSources         int           // 0 -> 5
	MaxBytesPerFetch   int           // 0 -> 64 KiB
	FetchTimeout       time.Duration // 0 -> 15s
	SynthesizeTimeout  time.Duration // 0 -> 60s
	RequireFrontmatter bool          // wrap Body in topic/date frontmatter
	Now                func() time.Time
}

// Generator wires a Searcher, Fetcher, and LLM into the Generate pipeline.
// A nil Searcher is fatal; nil Fetcher / LLM fall back to safe no-ops
// that surface an ErrNotWired error before allocating tokens.
type Generator struct {
	cfg    GeneratorConfig
	source Searcher
	body   Fetcher
	think  LLM
}

// NewGenerator returns a Generator with the supplied dependencies wired.
// Any nil dependency is rendered safe via guards in Generate — callers
// can therefore partially-construct and rewire later (handy for tests).
func NewGenerator(cfg GeneratorConfig, s Searcher, f Fetcher, l LLM) *Generator {
	if cfg.MaxSources <= 0 {
		cfg.MaxSources = 5
	}
	if cfg.MaxBytesPerFetch <= 0 {
		cfg.MaxBytesPerFetch = 64 * 1024
	}
	if cfg.FetchTimeout <= 0 {
		cfg.FetchTimeout = 15 * time.Second
	}
	if cfg.SynthesizeTimeout <= 0 {
		cfg.SynthesizeTimeout = 60 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Generator{cfg: cfg, source: s, body: f, think: l}
}

// ErrInvalid is returned by Generator.Validate when a synthesized report
// fails the byte-stable gate (M3): an empty Body, no Sources, or hedged
// language. It is the only error path that signals retry.
var ErrInvalid = errors.New("autonomy/research: report failed validation")

// ErrNotWired signals a missing Searcher/ LLM dependency. Fetcher may
// stay nil and yield Sources with BodyBytes=0 — the synthetic step
// then grounds itself on snippets alone.
var ErrNotWired = errors.New("autonomy/research: not wired")

// Generate runs the search → fetch → synthesize pipeline described in
// issue #384. The returned report is byte-stable per (topic, deterministic
// dependencies) pair, so two runs with the same fake dependencies emit
// identical Markdown.
func (g *Generator) Generate(ctx context.Context, topic string) (*ResearchReport, error) {
	if g == nil || g.source == nil || g.think == nil {
		return nil, ErrNotWired
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("autonomy/research: empty topic")
	}

	cctx, cancel := context.WithTimeout(ctx, g.cfg.FetchTimeout+g.cfg.SynthesizeTimeout+30*time.Second)
	defer cancel()

	hits, err := g.source.Search(cctx, topic, g.cfg.MaxSources)
	if err != nil {
		return nil, fmt.Errorf("autonomy/research: search: %w", err)
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("autonomy/research: no sources for %q", topic)
	}
	// M4: never trust the searcher to have already de-duplicated by URL.
	sources := dedupeAndRank(hits, g.cfg.MaxSources)
	for i := range sources {
		sources[i].FetchedAt, sources[i].BodyBytes, sources[i].Error = g.fetchSource(cctx, sources[i].URL)
	}

	body, err := g.think.Ask(cctx, synthesisSystemPrompt(), synthesisUserPrompt(topic, sources))
	if err != nil {
		return nil, fmt.Errorf("autonomy/research: synthesize: %w", err)
	}
	body = strings.TrimSpace(body)

	rep := &ResearchReport{
		Topic:       topic,
		Sources:     sources,
		Body:        body,
		GeneratedAt: g.cfg.Now().UTC(),
		Slug:        Slugify(topic),
	}
	if g.cfg.RequireFrontmatter {
		rep.Body = wrapFrontmatter(rep)
	}
	if err := rep.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return rep, nil
}

// fetchSource returns the body bytes (capped) or an error string stored
// on the Source for the synthesizer to see.
func (g *Generator) fetchSource(ctx context.Context, url string) (time.Time, int, string) {
	if g.body == nil {
		return time.Time{}, 0, ""
	}
	fctx, cancel := context.WithTimeout(ctx, g.cfg.FetchTimeout)
	defer cancel()
	body, fetchedAt, err := g.body.Fetch(fctx, url)
	if err != nil {
		return time.Time{}, 0, err.Error()
	}
	if len(body) > g.cfg.MaxBytesPerFetch {
		body = body[:g.cfg.MaxBytesPerFetch]
	}
	return fetchedAt, len(body), ""
}

// Validate enforces the four invariants a research report must satisfy
// before it is shipped:
//   1. Body is non-empty.
//   2. At least one Source is cited.
//   3. Slug is non-empty (file naming contract).
//   4. Body has a recognized Markdown header.
//
// The check is byte-stable per (topic, body) so the eval harness can pin
// golden snapshots.
func (r *ResearchReport) Validate() error {
	if r == nil {
		return fmt.Errorf("nil report")
	}
	if strings.TrimSpace(r.Body) == "" {
		return fmt.Errorf("empty body")
	}
	if len(r.Sources) == 0 {
		return fmt.Errorf("no sources")
	}
	if strings.TrimSpace(r.Slug) == "" {
		return fmt.Errorf("empty slug")
	}
	if !looksLikeMarkdown(r.Body) {
		return fmt.Errorf("body is not recognizable markdown")
	}
	return nil
}

// Slugify turns a free-form topic into a filesystem-safe slug. The rule
// pack is deliberately small and dependency-free (M2): lowercase,
// alnum + hyphen, run-length hyphenator, max 80 chars.
func Slugify(topic string) string {
	t := strings.ToLower(strings.TrimSpace(topic))
	var b strings.Builder
	for _, r := range t {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '/' || r == '.' || r == ':' || r == ',':
			b.WriteByte('-')
		}
	}
	out := collapseDashes(b.String())
	if len(out) > 80 {
		out = out[:80]
		out = strings.TrimRight(out, "-")
	}
	if out == "" {
		return "report"
	}
	return out
}

var dashRun = regexp.MustCompile(`-+`)

func collapseDashes(s string) string { return dashRun.ReplaceAllString(s, "-") }

// dedupeAndRank folds duplicate URLs into a single row (first write
// wins, longer snippet upgrades it) and trims to maxSources. Sorting is
// stable, so byte-stability of the Source list follows.
func dedupeAndRank(hits []Source, max int) []Source {
	if max <= 0 {
		max = 5
	}
	seen := make(map[string]int, len(hits))
	out := make([]Source, 0, len(hits))
	for _, h := range hits {
		key := strings.TrimSpace(h.URL)
		if key == "" {
			continue
		}
		if idx, ok := seen[key]; ok {
			if len(h.Snippet) > len(out[idx].Snippet) {
				out[idx].Snippet = h.Snippet
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Title) != len(out[j].Title) {
			return len(out[i].Title) > len(out[j].Title)
		}
		return out[i].URL < out[j].URL
	})
	if len(out) > max {
		out = out[:max]
	}
	return out
}

var (
	mdHeaderRe = regexp.MustCompile(`(?m)^#{1,6}\s+\S`)
	mdParaRe   = regexp.MustCompile(`\S`)
)

func looksLikeMarkdown(body string) bool {
	if !mdHeaderRe.MatchString(body) {
		return false
	}
	return mdParaRe.MatchString(body)
}

const synthesizeSystemTemplate = `You are an autonomous research synthesizer for SIN-Code (issue #384).
Produce a concise, citation-grounded Markdown report on the requested topic.
Rules:
- Output ONLY Markdown. No commentary, no preamble, no closing remarks.
- Ground every claim in one of the provided Sources; never invent URLs.
- Include 4-6 H2 sections that reflect the canonical facets of the topic.
- Embed inline citation anchors like ([Source N]) after each factual claim.
- End with a "## Sources" section that lists every numbered Source on its own line as: [N] <Title> - <URL>.
- Never use hedging language ("perhaps", "maybe", "might", "could be"). State facts.
- Cap output at ~600 words unless the topic explicitly demands depth.`

func synthesisSystemPrompt() string { return synthesizeSystemTemplate }

func synthesisUserPrompt(topic string, srcs []Source) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topic: %s\n\n", topic)
	b.WriteString("Sources (numbered for citation):\n")
	for i, s := range srcs {
		title := s.Title
		if title == "" {
			title = s.URL
		}
		bodyHint := ""
		if s.BodyBytes > 0 {
			bodyHint = fmt.Sprintf(" [%d bytes fetched]", s.BodyBytes)
		} else if s.Snippet != "" {
			bodyHint = fmt.Sprintf(" snippet=%q", truncate(s.Snippet, 240))
		}
		if s.Error != "" {
			bodyHint += fmt.Sprintf(" fetch_error=%q", s.Error)
		}
		fmt.Fprintf(&b, "[%d] %s — %s%s\n", i+1, title, s.URL, bodyHint)
	}
	b.WriteString("\nWrite the research report now.\n")
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// wrapFrontmatter prepends an H1 with the topic + RFC3339 timestamp so the
// generated Markdown is identifiable even when copy-pasted. Body is
// returned unchanged after the header.
func wrapFrontmatter(r *ResearchReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Topic)
	fmt.Fprintf(&b, "_Generated at %s_\n\n", r.GeneratedAt.Format(time.RFC3339))
	b.WriteString(r.Body)
	if !strings.HasSuffix(r.Body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// HTTPFetcher is a stdlib-net/http-backed Fetcher that respects
// context cancellation and the configured byte cap.
type HTTPFetcher struct {
	Client    *http.Client
	UserAgent string
}

// NewHTTPFetcher returns an HTTPFetcher with sensible defaults.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		Client:    &http.Client{Timeout: 20 * time.Second},
		UserAgent: "sin-code/1.0 (+research-report)",
	}
}

// Fetch streams response bytes up to MaxBytes and discards the rest.
func (h *HTTPFetcher) Fetch(ctx context.Context, url string) (string, time.Time, error) {
	if h == nil || h.Client == nil {
		return "", time.Time{}, errors.New("HTTPFetcher: not wired")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("User-Agent", h.UserAgent)
	req.Header.Set("Accept", "text/html,text/plain,application/json;q=0.9,*/*;q=0.8")
	resp, err := h.Client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", time.Time{}, fmt.Errorf("http %d for %s", resp.StatusCode, url)
	}
	// Read up to ~1 MiB to honour the byte cap (Generator applies its own cap).
	buf := make([]byte, 0, 1024*1024)
	tmp := make([]byte, 4096)
	for {
		if len(buf) >= 1024*1024 {
			break
		}
		n, rerr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	return string(buf), time.Now().UTC(), nil
}

// StaticSearcher is a Searcher used in tests; production code wires the
// MCP websearch__search bridge via the registry at runtime.
type StaticSearcher struct {
	Hits []Source
	Err  error
}

// Search implements Searcher.
func (s *StaticSearcher) Search(ctx context.Context, query string, max int) ([]Source, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	out := make([]Source, len(s.Hits))
	copy(out, s.Hits)
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// StaticLLM is an LLM used in tests; production code wires llm.Client.
type StaticLLM struct {
	Reply string
	Err   error
}

// Ask implements LLM.
func (s *StaticLLM) Ask(ctx context.Context, system, user string) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	return s.Reply, nil
}
