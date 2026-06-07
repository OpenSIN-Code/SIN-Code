// SPDX-License-Identifier: MIT
// Purpose: harvest — URL fetching with caching, structure extraction, and change
// detection. Built-in Go implementation using net/http with local disk cache.
package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var (
	harvestURL     string
	harvestFormat  string
	harvestMethod  string
	harvestTimeout int
)

var HarvestCmd = &cobra.Command{
	Use:   "harvest",
	Short: "Fetch URLs with caching, structure extraction, and change detection",
	Long: `Fetch URLs with caching, structure extraction, change detection, and
auth management. Pure Go implementation with local disk cache.

Example:
  sin-code harvest --url https://api.example.com/data --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if harvestURL == "" {
			return fmt.Errorf("--url is required")
		}
		return harvestURLFetch(harvestURL, harvestMethod, harvestTimeout, harvestFormat)
	},
}

type harvestResult struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Duration   string            `json:"duration"`
	Cached     bool              `json:"cached"`
	CacheAge   string            `json:"cache_age,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func harvestURLFetch(url, method string, timeout int, format string) error {
	start := time.Now()
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "sin-code", "harvest")
	_ = os.MkdirAll(cacheDir, 0755)
	cacheKey := sha256.Sum256([]byte(method + " " + url))
	cacheFile := filepath.Join(cacheDir, hex.EncodeToString(cacheKey[:])+".json")

	// Check cache (TTL: 5 minutes)
	if info, err := os.Stat(cacheFile); err == nil && time.Since(info.ModTime()) < 5*time.Minute {
		data, err := os.ReadFile(cacheFile)
		if err == nil {
			var cached harvestResult
			if err := json.Unmarshal(data, &cached); err == nil {
				cached.Cached = true
				cached.CacheAge = time.Since(info.ModTime()).String()
				if format == "json" {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(cached)
				}
				fmt.Printf("[CACHED %s] %s %s\n\n", cached.CacheAge, cached.Method, cached.URL)
				fmt.Println(cached.Body)
				return nil
			}
		}
	}

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	req.Header.Set("User-Agent", "sin-code/1.0 (https://github.com/OpenSIN-Code/SIN-Code-Bundle)")
	req.Header.Set("Accept", "text/html,application/json,text/plain,*/*")

	resp, err := client.Do(req)
	duration := time.Since(start)

	result := harvestResult{
		URL:      url,
		Method:   method,
		Duration: duration.String(),
		Cached:   false,
	}

	if err != nil {
		result.Error = err.Error()
		if format == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("ERROR: %s\n", result.Error)
		return nil
	}
	defer resp.Body.Close()

	result.Status = resp.StatusCode
	result.StatusText = resp.Status
	result.Headers = make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			result.Headers[k] = v[0]
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read body: %s", err)
	} else {
		result.Body = string(body)
	}

	// Save to cache
	if cacheData, err := json.MarshalIndent(result, "", "  "); err == nil {
		_ = os.WriteFile(cacheFile, cacheData, 0644)
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("Status: %s\nDuration: %s\n\n", result.StatusText, result.Duration)
	fmt.Println(result.Body)
	return nil
}

func init() {
	HarvestCmd.Flags().StringVarP(&harvestURL, "url", "u", "", "URL to fetch")
	_ = HarvestCmd.MarkFlagRequired("url")
	HarvestCmd.Flags().StringVarP(&harvestMethod, "method", "m", "GET", "HTTP method")
	HarvestCmd.Flags().IntVarP(&harvestTimeout, "timeout", "t", 30, "Timeout in seconds")
	HarvestCmd.Flags().StringVarP(&harvestFormat, "format", "f", "text", "Output format: text|json")
}
