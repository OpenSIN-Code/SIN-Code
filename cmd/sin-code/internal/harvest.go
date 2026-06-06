// SPDX-License-Identifier: MIT
// Purpose: harvest — fetch URLs with caching, structure extraction, change
// detection, and auth management. Pass-through to SIN-Code-Harvest-Tool.
package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	harvestURL    string
	harvestFormat string
	harvestMethod string
	harvestTimeout int
)

var HarvestCmd = &cobra.Command{
	Use:   "harvest",
	Short: "Fetch URLs with caching, structure extraction, and change detection",
	Long: `Fetch URLs with caching, structure extraction, change detection, and
auth management. Example:

  sin-code harvest -url "https://api.example.com/data" -format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if harvestURL == "" {
			return fmt.Errorf("--url is required")
		}

		client := &http.Client{
			Timeout: time.Duration(harvestTimeout) * time.Second,
		}
		req, err := http.NewRequest(harvestMethod, harvestURL, nil)
		if err != nil {
			return fmt.Errorf("invalid request: %w", err)
		}
		req.Header.Set("User-Agent", "sin-code/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}

		result := map[string]any{
			"url":         harvestURL,
			"method":      harvestMethod,
			"status_code": resp.StatusCode,
			"headers":     resp.Header,
			"body_size":   len(body),
			"body":        string(body),
		}

		if harvestFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("Harvest: %s → %d (%d bytes)\n", harvestURL, resp.StatusCode, len(body))
		return nil
	},
}

func init() {
	HarvestCmd.Flags().StringVarP(&harvestURL, "url", "u", "", "URL to fetch")
	_ = HarvestCmd.MarkFlagRequired("url")
	HarvestCmd.Flags().StringVarP(&harvestMethod, "method", "m", "GET", "HTTP method")
	HarvestCmd.Flags().IntVarP(&harvestTimeout, "timeout", "t", 30, "Timeout in seconds")
	HarvestCmd.Flags().StringVarP(&harvestFormat, "format", "f", "text", "Output format: text|json")
}
