// SPDX-License-Identifier: MIT
// Purpose: harvest — URL fetching. Thin wrapper around standalone SIN-Code-Harvest-Tool.
package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
auth management. Delegates to standalone SIN-Code-Harvest-Tool.

If the standalone binary is not installed, falls back to a built-in HTTP client.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if harvestURL == "" {
			return fmt.Errorf("--url is required")
		}

		binary, binErr := lookupStandalone("harvest")
		if binErr == nil {
			cArgs := []string{
				"-url", harvestURL,
				"-method", harvestMethod,
				"-format", harvestFormat,
			}
			c := exec.Command(binary, cArgs...)
			c.Stderr = os.Stderr
			c.Stdout = os.Stdout
			return c.Run()
		}

		client := &http.Client{Timeout: time.Duration(harvestTimeout) * time.Second}
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
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	},
}

func init() {
	HarvestCmd.Flags().StringVarP(&harvestURL, "url", "u", "", "URL to fetch")
	_ = HarvestCmd.MarkFlagRequired("url")
	HarvestCmd.Flags().StringVarP(&harvestMethod, "method", "m", "GET", "HTTP method")
	HarvestCmd.Flags().IntVarP(&harvestTimeout, "timeout", "t", 30, "Timeout in seconds")
	HarvestCmd.Flags().StringVarP(&harvestFormat, "format", "f", "text", "Output format: text|json")
}
