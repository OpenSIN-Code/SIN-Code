// SPDX-License-Identifier: MIT
// Purpose: autonomous research-report generation (issue #384).
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

type ResearchReport struct {
	Topic       string    `json:"topic"`
	Sources     []Source  `json:"sources"`
	Body        string    `json:"body"`
	GeneratedAt time.Time `json:"generated_at"`
	Slug        string    `json:"slug"`
}

type Source struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	FetchedAt time.Time `json:"fetched_at,omitempty"`
	BodyBytes int       `json:"body_bytes,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// sin-debt: yagni, upgrade: when a second implementation lands, remove this marker
type Searcher interface {
	Search(ctx context.Context, query string, max int) ([]Source, error)
}

// sin-debt: yagni, upgrade: when a second implementation lands, remove this marker
type Fetcher interface {
	Fetch(ctx context.Context, url string) (body string, fetchedAt time.Time, err error)
}

// sin-debt: yagni, upgrade: when a second implementation lands, remove this marker
type LLM interface {
	Ask(ctx context.Context, system, user string) (string, error)
}

type GeneratorConfig struct {
	MaxSources         int
	MaxBytesPerFetch   int
	FetchTimeout       time.Duration
	SynthesizeTimeout  time.Duration
	RequireFrontmatter bool
	Now                func() time.Time
}

type Generator struct {
	cfg    GeneratorConfig
	source Searcher
	body   Fetcher
	think  LLM
}

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

var ErrInvalid = errors.New("autonomy/research: report failed validation")
var ErrNotWired = errors.New("autonomy/research: not wired")

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
- Never use hedging language. State facts.
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
		fmt.Fprintf(&b, "[%d] %s - %s%s\n", i+1, title, s.URL, bodyHint)
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

type HTTPFetcher struct {
	Client    *http.Client
	UserAgent string
}

func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		Client:    &http.Client{Timeout: 20 * time.Second},
		UserAgent: "sin-code/1.0 (+research-report)",
	}
}

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

type StaticSearcher struct {
	Hits []Source
	Err  error
}

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

type StaticLLM struct {
	Reply string
	Err   error
}

func (s *StaticLLM) Ask(ctx context.Context, system, user string) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	return s.Reply, nil
}
