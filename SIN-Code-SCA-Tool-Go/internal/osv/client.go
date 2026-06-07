// SPDX-License-Identifier: MIT
package osv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/pkg/models"
)

const baseURL = "https://api.osv.dev/v1"

// Client provides access to the OSV.dev API.
type Client struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient creates a new OSV API client.
func NewClient(timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		timeout:    timeout,
	}
}

// QueryPackage queries vulnerabilities for a specific package version.
func (c *Client) QueryPackage(name, version, ecosystem string) ([]models.Vulnerability, error) {
	payload := map[string]interface{}{
		"version": version,
		"package": map[string]string{
			"name":      name,
			"ecosystem": ecosystem,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := c.httpClient.Post(baseURL+"/query", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("osv api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv api returned status %d", resp.StatusCode)
	}

	var result struct {
		Vulns []map[string]interface{} `json:"vulns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var vulns []models.Vulnerability
	for _, v := range result.Vulns {
		vulns = append(vulns, c.toVulnerability(v, name, version))
	}
	return vulns, nil
}

// BatchQuery queries vulnerabilities for multiple packages at once.
func (c *Client) BatchQuery(packages []models.Package) (map[string][]models.Vulnerability, error) {
	queries := make([]map[string]interface{}, len(packages))
	for i, pkg := range packages {
		queries[i] = map[string]interface{}{
			"version": pkg.Version,
			"package": map[string]string{
				"name":      pkg.Name,
				"ecosystem": pkg.Ecosystem,
			},
		}
	}

	payload := map[string]interface{}{"queries": queries}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := c.httpClient.Post(baseURL+"/querybatch", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("osv batch api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv batch api returned status %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			Vulns []map[string]interface{} `json:"vulns"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}

	results := make(map[string][]models.Vulnerability)
	for i, pkg := range packages {
		if i >= len(result.Results) {
			break
		}
		var vulns []models.Vulnerability
		for _, v := range result.Results[i].Vulns {
			vulns = append(vulns, c.toVulnerability(v, pkg.Name, pkg.Version))
		}
		results[pkg.Name] = vulns
	}
	return results, nil
}

func (c *Client) toVulnerability(v map[string]interface{}, pkgName, version string) models.Vulnerability {
	vuln := models.Vulnerability{
		ID:       getString(v, "id"),
		Package:  pkgName,
		Version:  version,
		Summary:  getString(v, "summary", "No summary available"),
		Severity: c.getSeverity(v),
		FixedIn:  c.getFixedVersion(v),
	}

	if refs, ok := v["references"].([]interface{}); ok {
		for _, r := range refs {
			if ref, ok := r.(map[string]interface{}); ok {
				if url, ok := ref["url"].(string); ok {
					vuln.References = append(vuln.References, url)
				}
			}
		}
	}

	if aliases, ok := v["aliases"].([]interface{}); ok {
		for _, a := range aliases {
			if s, ok := a.(string); ok {
				vuln.Aliases = append(vuln.Aliases, s)
			}
		}
	}

	return vuln
}

func (c *Client) getSeverity(v map[string]interface{}) string {
	if db, ok := v["database_specific"].(map[string]interface{}); ok {
		if sev, ok := db["severity"].(string); ok && sev != "" {
			return sev
		}
	}
	return "medium"
}

func (c *Client) getFixedVersion(v map[string]interface{}) string {
	if affected, ok := v["affected"].([]interface{}); ok {
		for _, a := range affected {
			if aff, ok := a.(map[string]interface{}); ok {
				if ranges, ok := aff["ranges"].([]interface{}); ok {
					for _, r := range ranges {
						if rng, ok := r.(map[string]interface{}); ok {
							if events, ok := rng["events"].([]interface{}); ok {
								for _, e := range events {
									if event, ok := e.(map[string]interface{}); ok {
										if fixed, ok := event["fixed"].(string); ok {
											return fixed
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func getString(m map[string]interface{}, key string, fallback ...string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}
