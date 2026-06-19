// SPDX-License-Identifier: MIT
// Purpose: tests for the scientific research sources (issue #387). Each
// source is exercised against an httptest.Server mock so no real network
// traffic occurs. The unexported baseURL field on each source is set to
// the test server's URL so requests are redirected into the mock. These
// tests are race-free (M7): all goroutine use is inside the stdlib
// http.Client and httptest.Server.
package autonomy

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Compile-time assertions that every source implements ResearchSource.
var (
	_ ResearchSource = (*PubMedSource)(nil)
	_ ResearchSource = (*ArxivSource)(nil)
	_ ResearchSource = (*USPTOSource)(nil)
)

// --- PubMed ---------------------------------------------------------------

func TestPubMedESearchEFetch(t *testing.T) {
	var esearchHits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/esearch.fcgi"):
			esearchHits = r.URL.Query()["idlist"]
			_ = esearchHits
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"esearchresult":{"idlist":["111","222"],"count":"2","retmax":"2"}}`))
		case strings.HasSuffix(r.URL.Path, "/efetch.fcgi"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>111</PMID>
      <Article>
        <ArticleTitle>CRISPR base editing in mice</ArticleTitle>
        <Abstract><AbstractText>We edit bases.</AbstractText></Abstract>
        <AuthorList>
          <Author><LastName>Doe</LastName><ForeName>Jane</ForeName></Author>
          <Author><LastName>Roe</LastName><ForeName>John</ForeName></Author>
        </AuthorList>
        <Journal><JournalIssue><PubDate><Year>2023</Year><Month>Mar</Month><Day>7</Day></PubDate></JournalIssue></Journal>
        <ArticleIdList>
          <ArticleId IdType="doi">10.1000/abc</ArticleId>
          <ArticleId IdType="pubmed">111</ArticleId>
        </ArticleIdList>
      </Article>
    </MedlineCitation>
  </PubmedArticle>
</PubmedArticleSet>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := NewPubMedSource("test-key")
	s.baseURL = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, err := s.Query(ctx, "CRISPR", QueryOpts{MaxResults: 5, SortBy: "relevance"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	got := res[0]
	if got.Source != "pubmed" {
		t.Errorf("Source = %q", got.Source)
	}
	if got.Title != "CRISPR base editing in mice" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Abstract != "We edit bases." {
		t.Errorf("Abstract = %q", got.Abstract)
	}
	if len(got.Authors) != 2 || got.Authors[0] != "Jane Doe" || got.Authors[1] != "John Roe" {
		t.Errorf("Authors = %v", got.Authors)
	}
	if got.DOI != "10.1000/abc" {
		t.Errorf("DOI = %q", got.DOI)
	}
	if got.URL != "https://pubmed.ncbi.nlm.nih.gov/111/" {
		t.Errorf("URL = %q", got.URL)
	}
	want := time.Date(2023, time.March, 7, 0, 0, 0, 0, time.UTC)
	if !got.PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, want)
	}
}

func TestPubMedAPIKeyPassed(t *testing.T) {
	var seenKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.URL.Query().Get("api_key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[],"count":"0"}}`))
	}))
	defer srv.Close()

	s := NewPubMedSource("secret-123")
	s.baseURL = srv.URL
	_, _ = s.Query(context.Background(), "x", QueryOpts{MaxResults: 3})
	if seenKey != "secret-123" {
		t.Fatalf("api_key not propagated; got %q", seenKey)
	}
}

func TestPubMedNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[],"count":"0"}}`))
	}))
	defer srv.Close()

	s := NewPubMedSource("")
	s.baseURL = srv.URL
	res, err := s.Query(context.Background(), "asdfqwerty", QueryOpts{MaxResults: 5})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result for empty idlist, got %v", res)
	}
}

func TestPubMedEmptyQueryRejected(t *testing.T) {
	s := NewPubMedSource("")
	if _, err := s.Query(context.Background(), "   ", QueryOpts{}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestPubMedHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream broken", http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewPubMedSource("")
	s.baseURL = srv.URL
	_, err := s.Query(context.Background(), "cancer", QueryOpts{MaxResults: 5})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

// --- arXiv ----------------------------------------------------------------

const arxivAtomFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/2401.12345v1</id>
    <title>Attention Is All You Need (repro)</title>
    <summary>We propose a transformer architecture.</summary>
    <published>2024-01-15T00:00:00Z</published>
    <author><name>Ada Lovelace</name></author>
    <author><name>Alan Turing</name></author>
    <link href="http://arxiv.org/abs/2401.12345v1" rel="alternate" type="text/html"/>
    <arxiv:doi>10.48550/arXiv.2401.12345</arxiv:doi>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2402.67890v2</id>
    <title>  Whitespace   heavy   title  </title>
    <summary>Second paper.</summary>
    <published>2024-02-20T12:30:00Z</published>
    <author><name>Grace Hopper</name></author>
    <link href="http://arxiv.org/pdf/2402.67890v2" rel="related" type="application/pdf"/>
    <link href="http://arxiv.org/abs/2402.67890v2" rel="alternate" type="text/html"/>
  </entry>
</feed>`

func TestArxivQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/query") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("search_query") != "all:transformer" {
			t.Errorf("search_query = %q", r.URL.Query().Get("search_query"))
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(arxivAtomFixture))
	}))
	defer srv.Close()

	s := NewArxivSource()
	s.baseURL = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, err := s.Query(ctx, "transformer", QueryOpts{MaxResults: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(res))
	}
	if res[0].Source != "arxiv" {
		t.Errorf("Source = %q", res[0].Source)
	}
	if res[0].Title != "Attention Is All You Need (repro)" {
		t.Errorf("Title[0] = %q", res[0].Title)
	}
	if res[0].DOI != "10.48550/arXiv.2401.12345" {
		t.Errorf("DOI[0] = %q", res[0].DOI)
	}
	want := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !res[0].PublishedAt.Equal(want) {
		t.Errorf("PublishedAt[0] = %v, want %v", res[0].PublishedAt, want)
	}
	if len(res[0].Authors) != 2 || res[0].Authors[0] != "Ada Lovelace" {
		t.Errorf("Authors[0] = %v", res[0].Authors)
	}
	// Whitespace should be collapsed, not just trimmed.
	if strings.Contains(res[1].Title, "  ") {
		t.Errorf("Title[1] should have collapsed whitespace: %q", res[1].Title)
	}
	if res[1].Title != "Whitespace heavy title" {
		t.Errorf("Title[1] = %q", res[1].Title)
	}
}

func TestArxivAlternateLinkPreferred(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2402.67890v2</id>
    <title>T</title>
    <summary>S</summary>
    <published>2024-02-20T12:30:00Z</published>
    <link href="http://arxiv.org/pdf/2402.67890v2" rel="related" type="application/pdf"/>
    <link href="http://arxiv.org/abs/2402.67890v2" rel="alternate" type="text/html"/>
  </entry>
</feed>`))
	}))
	defer srv.Close()

	s := NewArxivSource()
	s.baseURL = srv.URL
	res, err := s.Query(context.Background(), "x", QueryOpts{MaxResults: 1})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(res))
	}
	if res[0].URL != "http://arxiv.org/abs/2402.67890v2" {
		t.Errorf("URL = %q; alternate link should win over pdf/related", res[0].URL)
	}
}

func TestArxivEmptyQueryRejected(t *testing.T) {
	s := NewArxivSource()
	if _, err := s.Query(context.Background(), "", QueryOpts{}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

// --- USPTO ----------------------------------------------------------------

func TestUSPTOQuery(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("X-OS-API-Key")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "patents": [
		    {
		      "patent_number": "11223344",
		      "patent_title": "Quantum widget",
		      "patent_abstract": "A widget using quantum effects.",
		      "patent_date": "2022-07-04",
		      "inventors": [
		        {"inventor_first_name":"Marie","inventor_last_name":"Curie"},
		        {"inventor_first_name":"Nikola","inventor_last_name":"Tesla"}
		      ]
		    }
		  ],
		  "total_patent_count": 1
		}`))
	}))
	defer srv.Close()

	s := NewUSPTOSource("patentsview-key")
	s.baseURL = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, err := s.Query(ctx, "quantum", QueryOpts{MaxResults: 3})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q; want POST", gotMethod)
	}
	if gotPath != "/patents/query" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAPIKey != "patentsview-key" {
		t.Errorf("X-OS-API-Key = %q", gotAPIKey)
	}
	if !strings.Contains(gotBody, "patent_title") || !strings.Contains(gotBody, "quantum") {
		t.Errorf("request body missing query: %q", gotBody)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 patent, got %d", len(res))
	}
	got := res[0]
	if got.Source != "uspto" {
		t.Errorf("Source = %q", got.Source)
	}
	if got.Title != "Quantum widget" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Abstract != "A widget using quantum effects." {
		t.Errorf("Abstract = %q", got.Abstract)
	}
	if len(got.Authors) != 2 || got.Authors[0] != "Marie Curie" || got.Authors[1] != "Nikola Tesla" {
		t.Errorf("Authors = %v", got.Authors)
	}
	if got.URL != "https://patents.google.com/patent/US11223344" {
		t.Errorf("URL = %q", got.URL)
	}
	want := time.Date(2022, time.July, 4, 0, 0, 0, 0, time.UTC)
	if !got.PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, want)
	}
}

func TestUSPTOHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	s := NewUSPTOSource("")
	s.baseURL = srv.URL
	_, err := s.Query(context.Background(), "lens", QueryOpts{MaxResults: 2})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 error, got %v", err)
	}
}

// TestResearchSourceNames confirms each Name() returns the documented
// stable identifier — a public API contract per AGENTS.md §10.
func TestResearchSourceNames(t *testing.T) {
	if got := NewPubMedSource("").Name(); got != "pubmed" {
		t.Errorf("PubMedSource.Name = %q, want \"pubmed\"", got)
	}
	if got := NewArxivSource().Name(); got != "arxiv" {
		t.Errorf("ArxivSource.Name = %q, want \"arxiv\"", got)
	}
	if got := NewUSPTOSource("").Name(); got != "uspto" {
		t.Errorf("USPTOSource.Name = %q, want \"uspto\"", got)
	}
}

// TestArxivXMLStructDecodes is a focused unit test for the Atom parsing
// without an HTTP round-trip, ensuring the namespace-aware struct tags
// match the real feed shape.
func TestArxivXMLStructDecodes(t *testing.T) {
	var feed arxivFeed
	if err := xml.Unmarshal([]byte(arxivAtomFixture), &feed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(feed.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(feed.Entries))
	}
	if feed.Entries[0].DOI != "10.48550/arXiv.2401.12345" {
		t.Errorf("DOI = %q", feed.Entries[0].DOI)
	}
	if len(feed.Entries[0].Authors) != 2 || feed.Entries[0].Authors[0].Name != "Ada Lovelace" {
		t.Errorf("authors = %+v", feed.Entries[0].Authors)
	}
}
