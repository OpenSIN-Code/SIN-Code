// SPDX-License-Identifier: MIT
// Purpose: scientific research sources — PubMed, arXiv, USPTO (issue
// #387). Each source implements ResearchSource so the autonomy daemon
// (and the ReportGenerator from issue #384) can collect results from
// heterogeneous scientific databases behind one interface. Every source
// performs real HTTP via an injectable HTTPDoer transport (defaults to
// http.DefaultClient) so tests can substitute an httptest.Server-backed
// client without touching the network.
//
// All sources are stateless once constructed; Query is safe for
// concurrent use (M7). The unexported baseURL field on each source lets
// same-package tests redirect requests to an httptest.Server; in
// production it stays empty and the default API endpoint constant is
// used.
package autonomy

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPDoer is the minimal transport surface every source depends on. It
// is satisfied by *http.Client and by test doubles; the indirection is
// what makes the sources testable without hitting the public internet.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// QueryOpts tunes a research query. MaxResults caps the number of
// results returned (0 => provider default). SortBy is a provider
// specific sort hint (e.g. "relevance", "date", "pub_date"). DateFrom
// and DateTo are optional RFC3339-ish bounds; providers that do not
// support a bound ignore it.
type QueryOpts struct {
	MaxResults int
	SortBy     string
	DateFrom   string
	DateTo     string
}

// ResearchResult is the normalized record returned by every source.
// PublishedAt is the best-known publication date; it is the zero value
// when the source does not expose one. Source is the lowercased
// provider name ("pubmed", "arxiv", "uspto").
type ResearchResult struct {
	Title       string
	Authors     []string
	Abstract    string
	URL         string
	DOI         string
	PublishedAt time.Time
	Source      string
}

// ResearchSource is the common interface every scientific database
// source implements. Name is the stable, lowercased provider
// identifier; Query performs a real search and returns normalized
// results.
type ResearchSource interface {
	Name() string
	Query(ctx context.Context, q string, opts QueryOpts) ([]ResearchResult, error)
}

// defaultMaxResults is used when QueryOpts.MaxResults is not positive.
const defaultMaxResults = 10

// researchUserAgent identifies the SIN-Code agent to upstream APIs.
const researchUserAgent = "SIN-Code/research (+https://github.com/OpenSIN-Code/SIN-Code)"

// doGet issues a GET via the transport (falling back to
// http.DefaultClient when nil), attaches the User-Agent, and returns
// the fully-read response body. Non-2xx responses are surfaced as
// errors. The caller's context is honoured via the request.
func doGet(ctx context.Context, transport HTTPDoer, target string) ([]byte, error) {
	if transport == nil {
		transport = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("research: build request: %w", err)
	}
	req.Header.Set("User-Agent", researchUserAgent)
	req.Header.Set("Accept", "application/json, application/xml;q=0.9, */*;q=0.5")
	resp, err := transport.Do(req)
	if err != nil {
		return nil, fmt.Errorf("research: HTTP: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("research: upstream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

// ===========================================================================
// PubMed — NCBI E-utilities (esearch + efetch)
// ===========================================================================

const pubmedDefaultBase = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"

// PubMedSource queries the NCBI E-utilities. An API key raises the
// rate limit from 3 to 10 requests/second and is recommended but not
// required.
type PubMedSource struct {
	APIKey    string
	transport HTTPDoer
	baseURL   string
}

// NewPubMedSource returns a PubMedSource using http.DefaultClient. The
// apiKey is optional; pass "" to use the unauthenticated tier.
func NewPubMedSource(apiKey string) *PubMedSource {
	return &PubMedSource{APIKey: apiKey, transport: http.DefaultClient}
}

// Name returns "pubmed".
func (s *PubMedSource) Name() string { return "pubmed" }

// pubmedESearchResult is the JSON envelope returned by esearch.fcgi.
type pubmedESearchResult struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
		Count  string   `json:"count"`
	} `json:"esearchresult"`
}

// pubmedArticleSet is the XML envelope returned by efetch.fcgi. Only the
// fields needed to populate a ResearchResult are decoded.
type pubmedArticleSet struct {
	XMLName  xml.Name         `xml:"PubmedArticleSet"`
	Articles []pubmedArticle  `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	XMLName        xml.Name `xml:"PubmedArticle"`
	MedlineCitation struct {
		PMID    string `xml:"PMID"`
		Article struct {
			Title      string `xml:"ArticleTitle"`
			Abstract   struct {
				Texts []struct {
					Text string `xml:",chardata"`
				} `xml:"AbstractText"`
			} `xml:"Abstract"`
			Authors struct {
				Authors []struct {
					LastName string `xml:"LastName"`
					ForeName string `xml:"ForeName"`
				} `xml:"Author"`
			} `xml:"AuthorList"`
			Journal struct {
				Issue struct {
					PubDate struct {
						Year  string `xml:"Year"`
						Month string `xml:"Month"`
						Day   string `xml:"Day"`
					} `xml:"PubDate"`
				} `xml:"JournalIssue"`
			} `xml:"Journal"`
			ArticleIDs struct {
				IDs []struct {
					ID   string `xml:",chardata"`
					Type string `xml:"IdType,attr"`
				} `xml:"ArticleId"`
			} `xml:"ArticleIdList"`
		} `xml:"Article"`
	} `xml:"MedlineCitation"`
}

// Query searches PubMed: first an esearch to resolve PMIDs, then an
// efetch to retrieve article metadata. Results are normalized to
// ResearchResult with Source="pubmed".
func (s *PubMedSource) Query(ctx context.Context, q string, opts QueryOpts) ([]ResearchResult, error) {
	if strings.TrimSpace(q) == "" {
		return nil, fmt.Errorf("pubmed: empty query")
	}
	max := opts.MaxResults
	if max <= 0 {
		max = defaultMaxResults
	}
	base := s.baseURL
	if base == "" {
		base = pubmedDefaultBase
	}

	esearch := base + "/esearch.fcgi"
	params := url.Values{}
	params.Set("db", "pubmed")
	params.Set("term", q)
	params.Set("retmax", strconv.Itoa(max))
	params.Set("retmode", "json")
	if opts.SortBy != "" {
		params.Set("sort", opts.SortBy)
	}
	if opts.DateFrom != "" {
		params.Set("mindate", opts.DateFrom)
	}
	if opts.DateTo != "" {
		params.Set("maxdate", opts.DateTo)
	}
	if s.APIKey != "" {
		params.Set("api_key", s.APIKey)
	}
	esearchURL := esearch + "?" + params.Encode()

	body, err := doGet(ctx, s.transport, esearchURL)
	if err != nil {
		return nil, fmt.Errorf("pubmed: esearch: %w", err)
	}
	var sr pubmedESearchResult
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("pubmed: esearch decode: %w", err)
	}
	ids := sr.ESearchResult.IDList
	if len(ids) == 0 {
		return nil, nil
	}

	efetch := base + "/efetch.fcgi"
	fp := url.Values{}
	fp.Set("db", "pubmed")
	fp.Set("id", strings.Join(ids, ","))
	fp.Set("retmode", "xml")
	if s.APIKey != "" {
		fp.Set("api_key", s.APIKey)
	}
	efetchURL := efetch + "?" + fp.Encode()
	xmlBody, err := doGet(ctx, s.transport, efetchURL)
	if err != nil {
		return nil, fmt.Errorf("pubmed: efetch: %w", err)
	}
	var set pubmedArticleSet
	if err := xml.Unmarshal(xmlBody, &set); err != nil {
		return nil, fmt.Errorf("pubmed: efetch decode: %w", err)
	}

	results := make([]ResearchResult, 0, len(set.Articles))
	for _, a := range set.Articles {
		results = append(results, pubmedArticleToResult(a))
	}
	return results, nil
}

func pubmedArticleToResult(a pubmedArticle) ResearchResult {
	r := ResearchResult{Source: "pubmed"}
	mc := a.MedlineCitation
	r.Title = strings.TrimSpace(mc.Article.Title)
	abstractParts := make([]string, 0, len(mc.Article.Abstract.Texts))
	for _, t := range mc.Article.Abstract.Texts {
		if v := strings.TrimSpace(t.Text); v != "" {
			abstractParts = append(abstractParts, v)
		}
	}
	r.Abstract = strings.Join(abstractParts, " ")
	for _, au := range mc.Article.Authors.Authors {
		name := strings.TrimSpace(strings.Join([]string{au.ForeName, au.LastName}, " "))
		if name != "" {
			r.Authors = append(r.Authors, name)
		}
	}
	for _, id := range mc.Article.ArticleIDs.IDs {
		switch strings.ToLower(id.Type) {
		case "doi":
			r.DOI = strings.TrimSpace(id.ID)
		case "pubmed":
			r.URL = "https://pubmed.ncbi.nlm.nih.gov/" + strings.TrimSpace(id.ID) + "/"
		}
	}
	if r.URL == "" && mc.PMID != "" {
		r.URL = "https://pubmed.ncbi.nlm.nih.gov/" + strings.TrimSpace(mc.PMID) + "/"
	}
	r.PublishedAt = parsePubDate(mc.Article.Journal.Issue.PubDate)
	return r
}

// parsePubDate builds a time.Time from NCBI's PubDate Year/Month/Day
// triple. Month may be an English month abbreviation or a number;
// missing fields default to 1. A missing Year yields the zero time.
func parsePubDate(d struct {
	Year  string `xml:"Year"`
	Month string `xml:"Month"`
	Day   string `xml:"Day"`
}) time.Time {
	year, _ := strconv.Atoi(d.Year)
	if year == 0 {
		return time.Time{}
	}
	month := 1
	if m, ok := monthNumber(d.Month); ok {
		month = m
	}
	day, _ := strconv.Atoi(d.Day)
	if day == 0 {
		day = 1
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func monthNumber(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if m, err := strconv.Atoi(s); err == nil && m >= 1 && m <= 12 {
		return m, true
	}
	abbr := map[string]int{
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	}
	if m, ok := abbr[strings.ToLower(s[:3])]; ok {
		return m, true
	}
	return 0, false
}

// ===========================================================================
// arXiv — Atom 1.0 API
// ===========================================================================

const arxivDefaultBase = "http://export.arxiv.org/api"

// ArxivSource queries the arXiv API. No API key is required.
type ArxivSource struct {
	transport HTTPDoer
	baseURL   string
}

// NewArxivSource returns an ArxivSource using http.DefaultClient.
func NewArxivSource() *ArxivSource {
	return &ArxivSource{transport: http.DefaultClient}
}

// Name returns "arxiv".
func (s *ArxivSource) Name() string { return "arxiv" }

// arxivFeed is the Atom 1.0 feed root. The Atom namespace is declared
// explicitly in the struct tags so encoding/xml matches correctly.
type arxivFeed struct {
	XMLName xml.Name     `xml:"http://www.w3.org/2005/Atom feed"`
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID        string       `xml:"id"`
	Title     string       `xml:"title"`
	Summary   string       `xml:"summary"`
	Published string       `xml:"published"`
	Authors   []arxivAuthor `xml:"author"`
	Links     []arxivLink  `xml:"link"`
	DOI       string       `xml:"http://arxiv.org/schemas/atom doi"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// Query searches arXiv and normalizes Atom entries to ResearchResult
// with Source="arxiv". The abs-page link is preferred for the URL so
// the result points to the human-readable landing page.
func (s *ArxivSource) Query(ctx context.Context, q string, opts QueryOpts) ([]ResearchResult, error) {
	if strings.TrimSpace(q) == "" {
		return nil, fmt.Errorf("arxiv: empty query")
	}
	max := opts.MaxResults
	if max <= 0 {
		max = defaultMaxResults
	}
	base := s.baseURL
	if base == "" {
		base = arxivDefaultBase
	}
	params := url.Values{}
	params.Set("search_query", "all:"+q)
	params.Set("start", "0")
	params.Set("max_results", strconv.Itoa(max))
	if opts.SortBy != "" {
		params.Set("sortBy", opts.SortBy)
	} else {
		params.Set("sortBy", "relevance")
	}
	target := base + "/query?" + params.Encode()

	body, err := doGet(ctx, s.transport, target)
	if err != nil {
		return nil, fmt.Errorf("arxiv: query: %w", err)
	}
	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("arxiv: decode: %w", err)
	}
	results := make([]ResearchResult, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		results = append(results, arxivEntryToResult(e))
	}
	return results, nil
}

func arxivEntryToResult(e arxivEntry) ResearchResult {
	r := ResearchResult{Source: "arxiv"}
	r.Title = strings.Join(strings.Fields(e.Title), " ")
	r.Abstract = strings.Join(strings.Fields(e.Summary), " ")
	for _, a := range e.Authors {
		if name := strings.TrimSpace(a.Name); name != "" {
			r.Authors = append(r.Authors, name)
		}
	}
	r.DOI = strings.TrimSpace(e.DOI)
	r.URL = arxivAbsURL(e)
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(e.Published)); err == nil {
		r.PublishedAt = t
	}
	return r
}

// arxivAbsURL resolves the human-readable landing-page URL. The
// alternate text/html link wins; otherwise the entry id (which is
// already an abs URL) is used.
func arxivAbsURL(e arxivEntry) string {
	for _, l := range e.Links {
		if l.Rel == "alternate" {
			return l.Href
		}
	}
	return strings.TrimSpace(e.ID)
}

// ===========================================================================
// USPTO — PatentsView API
// ===========================================================================

const usptoDefaultBase = "https://api.patentsview.org"

// usptoResponse is the JSON envelope returned by the PatentsView
// /patents/query endpoint. Only the fields needed to populate a
// ResearchResult are decoded.
type usptoResponse struct {
	Patents []usptoPatent `json:"patents"`
	Count   int           `json:"total_patent_count"`
}

type usptoPatent struct {
	Number   string `json:"patent_number"`
	Title    string `json:"patent_title"`
	Abstract string `json:"patent_abstract"`
	Date     string `json:"patent_date"`
	Inventors []struct {
		First string `json:"inventor_first_name"`
		Last  string `json:"inventor_last_name"`
	} `json:"inventors"`
}

// USPTOSource queries the PatentsView API. An API key is recommended for
// higher rate limits; pass "" to attempt the unauthenticated tier.
type USPTOSource struct {
	APIKey    string
	transport HTTPDoer
	baseURL   string
}

// NewUSPTOSource returns a USPTOSource using http.DefaultClient.
func NewUSPTOSource(apiKey string) *USPTOSource {
	return &USPTOSource{APIKey: apiKey, transport: http.DefaultClient}
}

// Name returns "uspto".
func (s *USPTOSource) Name() string { return "uspto" }

// Query searches USPTO patents via PatentsView and normalizes to
// ResearchResult with Source="uspto". DOI is not populated (patents do
// not carry DOIs); URL points to the PatentsView detail page.
func (s *USPTOSource) Query(ctx context.Context, q string, opts QueryOpts) ([]ResearchResult, error) {
	if strings.TrimSpace(q) == "" {
		return nil, fmt.Errorf("uspto: empty query")
	}
	max := opts.MaxResults
	if max <= 0 {
		max = defaultMaxResults
	}
	base := s.baseURL
	if base == "" {
		base = usptoDefaultBase
	}

	// PatentsView accepts a JSON query body describing the search and
	// the fields to return.
	qOpts := map[string]any{
		"patent_title":         q,
		"_limit":               max,
		"_sort":                "patent_date desc",
	}
	if opts.SortBy != "" {
		qOpts["_sort"] = opts.SortBy
	}
	payload, err := json.Marshal(qOpts)
	if err != nil {
		return nil, fmt.Errorf("uspto: marshal query: %w", err)
	}
	target := base + "/patents/query"

	if s.transport == nil {
		s.transport = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("uspto: build request: %w", err)
	}
	req.Header.Set("User-Agent", researchUserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.APIKey != "" {
		req.Header.Set("X-OS-API-Key", s.APIKey)
	}
	resp, err := s.transport.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uspto: HTTP: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("uspto: upstream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("uspto: read body: %w", err)
	}
	var ur usptoResponse
	if err := json.Unmarshal(body, &ur); err != nil {
		return nil, fmt.Errorf("uspto: decode: %w", err)
	}
	results := make([]ResearchResult, 0, len(ur.Patents))
	for _, p := range ur.Patents {
		results = append(results, usptoPatentToResult(p))
	}
	return results, nil
}

func usptoPatentToResult(p usptoPatent) ResearchResult {
	r := ResearchResult{Source: "uspto"}
	r.Title = strings.TrimSpace(p.Title)
	r.Abstract = strings.TrimSpace(p.Abstract)
	for _, inv := range p.Inventors {
		name := strings.TrimSpace(strings.Join([]string{inv.First, inv.Last}, " "))
		if name != "" {
			r.Authors = append(r.Authors, name)
		}
	}
	num := strings.TrimSpace(p.Number)
	if num != "" {
		r.URL = "https://patents.google.com/patent/US" + num
	}
	if d := strings.TrimSpace(p.Date); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			r.PublishedAt = t
		}
	}
	return r
}
