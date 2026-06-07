// SPDX-License-Identifier: MIT
package trivy

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Container-Tool-Go/pkg/models"
)

// Client wraps the Trivy CLI.
type Client struct {
	path    string
	timeout time.Duration
}

// NewClient creates a new Trivy client.
func NewClient(path string, timeout time.Duration) *Client {
	if path == "" {
		path = "trivy"
	}
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	return &Client{path: path, timeout: timeout}
}

// VerifyInstallation checks if Trivy is installed.
func (c *Client) VerifyInstallation() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.path, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trivy not found or not working: %w", err)
	}
	return nil
}

// ScanImage scans a Docker image for vulnerabilities and misconfigurations.
func (c *Client) ScanImage(image, scanType string, severity []string, ignoreUnfixed, offline bool) (*models.TrivyScanReport, error) {
	cmdArgs := []string{
		"image",
		"--format", "json",
		"--quiet",
	}

	if scanType == "all" {
		cmdArgs = append(cmdArgs, "--scanners", "vuln,config,secret")
	} else {
		cmdArgs = append(cmdArgs, "--scanners", scanType)
	}

	if len(severity) > 0 {
		cmdArgs = append(cmdArgs, "--severity", strings.Join(severity, ","))
	}
	if ignoreUnfixed {
		cmdArgs = append(cmdArgs, "--ignore-unfixed")
	}
	if offline {
		cmdArgs = append(cmdArgs, "--skip-db-update")
	}

	cmdArgs = append(cmdArgs, image)

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.path, cmdArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Continue parsing
		} else {
			return nil, fmt.Errorf("trivy scan failed: %w (output: %s)", err, string(output))
		}
	}

	out := strings.TrimSpace(string(output))
	if out == "" {
		return &models.TrivyScanReport{
			Image:   image,
			Summary: models.NewTrivySummary(),
		}, nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, fmt.Errorf("parse trivy output: %w", err)
	}

	return c.parseResults(image, data), nil
}

// ScanFilesystem scans a filesystem path (for CI/CD without Docker build).
func (c *Client) ScanFilesystem(path, scanType string, severity []string) (*models.TrivyScanReport, error) {
	cmdArgs := []string{
		"fs",
		"--format", "json",
		"--quiet",
	}

	if scanType == "all" {
		cmdArgs = append(cmdArgs, "--scanners", "vuln,config,secret")
	} else {
		cmdArgs = append(cmdArgs, "--scanners", scanType)
	}

	if len(severity) > 0 {
		cmdArgs = append(cmdArgs, "--severity", strings.Join(severity, ","))
	}

	cmdArgs = append(cmdArgs, path)

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.path, cmdArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Continue parsing
		} else {
			return nil, fmt.Errorf("trivy fs scan failed: %w", err)
		}
	}

	out := strings.TrimSpace(string(output))
	if out == "" {
		return &models.TrivyScanReport{
			Image:   path,
			Summary: models.NewTrivySummary(),
		}, nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, fmt.Errorf("parse trivy fs output: %w", err)
	}

	return c.parseResults(path, data), nil
}

func (c *Client) parseResults(image string, data map[string]interface{}) *models.TrivyScanReport {
	var vulns []models.ContainerVulnerability
	var misconfigs []models.ContainerMisconfiguration
	var osFamily, osVersion string

	results, _ := data["Results"].([]interface{})
	for _, r := range results {
		result, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		target := getString(result, "Target")
		resultType := getString(result, "Type")
		resultClass := getString(result, "Class")

		if resultClass == "os-pkgs" {
			osFamily = resultType
			if idx := strings.LastIndex(target, ":"); idx != -1 {
				osVersion = target[idx+1:]
			}
		}

		// Parse vulnerabilities
		if vulnList, ok := result["Vulnerabilities"].([]interface{}); ok {
			for _, v := range vulnList {
				vulnMap, ok := v.(map[string]interface{})
				if !ok {
					continue
				}
				vulnType := "library"
				if resultClass == "os-pkgs" {
					vulnType = "os"
				}
				layer := ""
				if layerData, ok := vulnMap["Layer"].(map[string]interface{}); ok {
					layer = getString(layerData, "Digest")
				}
				vulns = append(vulns, models.ContainerVulnerability{
					ID:          getString(vulnMap, "VulnerabilityID"),
					Package:     getString(vulnMap, "PkgName"),
					Version:     getString(vulnMap, "InstalledVersion"),
					Severity:    getString(vulnMap, "Severity"),
					FixedIn:     getString(vulnMap, "FixedVersion"),
					Title:       getString(vulnMap, "Title"),
					Description: truncate(getString(vulnMap, "Description"), 500),
					Layer:       layer,
					VulnType:    vulnType,
					References:  getStringSlice(vulnMap, "References"),
					CVSSScore:   extractCVSS(vulnMap),
					PrimaryURL:  getString(vulnMap, "PrimaryURL"),
				})
			}
		}

		// Parse misconfigurations
		if misconfList, ok := result["Misconfigurations"].([]interface{}); ok {
			for _, m := range misconfList {
				misconfMap, ok := m.(map[string]interface{})
				if !ok {
					continue
				}
				category := "config"
				if strings.Contains(target, "Dockerfile") {
					category = "dockerfile"
				}
				lineNum := 0
				if causeMeta, ok := misconfMap["CauseMetadata"].(map[string]interface{}); ok {
					if ln, ok := causeMeta["StartLine"].(float64); ok {
						lineNum = int(ln)
					}
				}
				misconfigs = append(misconfigs, models.ContainerMisconfiguration{
					ID:         getString(misconfMap, "ID"),
					Title:      getString(misconfMap, "Title"),
					Message:    getString(misconfMap, "Message"),
					Severity:   getString(misconfMap, "Severity"),
					FilePath:   target,
					LineNumber: lineNum,
					Resolution: getString(misconfMap, "Resolution"),
					Category:   category,
				})
			}
		}
	}

	summary := generateSummary(vulns, misconfigs)

	return &models.TrivyScanReport{
		Image:             image,
		OSFamily:          osFamily,
		OSVersion:         osVersion,
		Vulnerabilities:   vulns,
		Misconfigurations: misconfigs,
		Summary:           summary,
	}
}

func generateSummary(vulns []models.ContainerVulnerability, misconfigs []models.ContainerMisconfiguration) map[string]int {
	summary := models.NewTrivySummary()
	summary["total"] = len(vulns)
	summary["misconfigurations"] = len(misconfigs)

	for _, v := range vulns {
		switch strings.ToUpper(v.Severity) {
		case "CRITICAL":
			summary["critical"]++
		case "HIGH":
			summary["high"]++
		case "MEDIUM":
			summary["medium"]++
		case "LOW":
			summary["low"]++
		default:
			summary["unknown"]++
		}
		if v.FixedIn != "" {
			summary["fixable"]++
		}
	}

	return summary
}

func extractCVSS(vuln map[string]interface{}) float64 {
	cvssData, ok := vuln["CVSS"].(map[string]interface{})
	if !ok {
		return 0
	}
	var maxScore float64
	for _, scores := range cvssData {
		scoresMap, ok := scores.(map[string]interface{})
		if !ok {
			continue
		}
		for _, data := range scoresMap {
			dataMap, ok := data.(map[string]interface{})
			if !ok {
				continue
			}
			if v3, ok := dataMap["V3Score"]; ok {
				if score, err := toFloat(v3); err == nil && score > maxScore {
					maxScore = score
				}
			}
			if v2, ok := dataMap["V2Score"]; ok {
				if score, err := toFloat(v2); err == nil && score > maxScore {
					maxScore = score
				}
			}
		}
	}
	return maxScore
}

func toFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, nil
		}
		return 0, fmt.Errorf("cannot parse string to float")
	case int:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float", v)
	}
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			var result []string
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
