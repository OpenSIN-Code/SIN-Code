// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when tools are MCP-externalized
// Purpose: HTTP fetch and web search tool implementations. The bounded
// HTTP GET (sin_http_get) caps response size at 256 KB with a 30 s timeout.
// The web search (sin_web_search) delegates to the internal/websearch
// engine which supports DuckDuckGo (free) plus Tavily, SerpAPI, and Brave
// when API keys are present. Specs and dispatch remain in
// chat_tools_extra.go.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/websearch"
)

// HTTP hook variables — injected by coverage tests to mock network calls.
var (
	toolHTTPNewRequestFn = http.NewRequestWithContext
	toolHTTPClientDoFn   = func(req *http.Request) (*http.Response, error) { return http.DefaultClient.Do(req) }
)

// toolHTTPGetFn is injected by coverage tests to mock the HTTP fetch.
var toolHTTPGetFn = toolHTTPGet

func toolHTTPGet(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("sin_http_get: only http(s) URLs allowed")
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := toolHTTPNewRequestFn(cctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sin-code-agent/3.5")
	resp, err := toolHTTPClientDoFn(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBytes))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d (%d bytes)\n%s", resp.StatusCode, len(body), body), nil
}

var webSearchEngineOnce sync.Once
var webSearchEngine *websearch.Engine

func getWebSearchEngine() *websearch.Engine {
	webSearchEngineOnce.Do(func() {
		cfg := websearch.LoadConfig()
		webSearchEngine = websearch.NewEngine(cfg, nil)
	})
	return webSearchEngine
}

func toolWebSearch(ctx context.Context, args map[string]any) (string, error) {
	query := argStr(args, "query")
	if query == "" {
		return "", fmt.Errorf("sin_web_search: 'query' is required")
	}
	maxStr := argStr(args, "max")
	maxResults := 10
	if maxStr != "" {
		if n, err := strconv.Atoi(maxStr); err == nil && n > 0 && n <= 50 {
			maxResults = n
		}
	}
	jsonOut := argBool(args, "json", false)

	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	engine := getWebSearchEngine()
	results, stats := engine.Search(cctx, query)
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	if jsonOut {
		type jsonResult struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Snippet string  `json:"snippet"`
			Source  string  `json:"source"`
			Score   float64 `json:"score"`
		}
		type jsonResp struct {
			Query    string          `json:"query"`
			Results  []jsonResult    `json:"results"`
			Stats    websearch.Stats `json:"stats"`
		}
		resp := jsonResp{Query: query, Stats: stats}
		for _, r := range results {
			resp.Results = append(resp.Results, jsonResult{
				Title: r.Title, URL: r.URL, Snippet: r.Snippet, Source: r.Source, Score: r.Score,
			})
		}
		b, _ := json.Marshal(resp)
		return string(b), nil
	}

	if len(results) == 0 {
		providers := ""
		if stats.Providers > 0 {
			providers = fmt.Sprintf(" (%d providers queried)", stats.Providers)
		}
		return fmt.Sprintf("No results for %q%s. Set WEBSEARCH_TAVILY_KEY or WEBSEARCH_SERPAPI_KEY for more providers.", query, providers), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Web search: %q\n%d results from %d providers (%dms)\n\n", query, len(results), stats.Providers, stats.DurationMS)
	for i, r := range results {
		fmt.Fprintf(&b, "%d. [%s] %s\n   %s\n   %s\n\n", i+1, r.Source, r.Title, r.URL, r.Snippet)
	}
	return b.String(), nil
}
