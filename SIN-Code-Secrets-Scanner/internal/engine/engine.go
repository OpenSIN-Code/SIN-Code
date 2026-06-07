// SPDX-License-Identifier: MIT
package engine

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Secrets-Scanner/pkg/models"
)

// Engine represents the secrets scanning engine
type Engine struct {
	Rules           []models.DetectionRule
	ExcludePatterns []string
	CheckEntropy    bool
}

// NewEngine creates a new secrets engine with the given rules
func NewEngine(rules []models.DetectionRule, exclude []string, checkEntropy bool) *Engine {
	return &Engine{
		Rules:           rules,
		ExcludePatterns: exclude,
		CheckEntropy:    checkEntropy,
	}
}

// Scan performs a secrets scan on the given path
func (e *Engine) Scan(path string) (*models.SecretsResult, error) {
	start := time.Now()

	files, err := e.findFiles(path)
	if err != nil {
		return nil, fmt.Errorf("finding files: %w", err)
	}

	var findings []models.SecretFinding
	filesScanned := 0

	for _, file := range files {
		if e.isExcluded(file) {
			continue
		}
		fileFindings, err := e.scanFile(file)
		if err != nil {
			continue
		}
		findings = append(findings, fileFindings...)
		filesScanned++
	}

	summary := buildSummary(findings, filesScanned)
	status := "passed"
	if summary.Critical > 0 {
		status = "failed"
	} else if summary.High > 0 {
		status = "warning"
	}

	return &models.SecretsResult{
		Path:                path,
		Status:              status,
		Findings:            findings,
		Summary:             summary,
		ScanDurationSeconds: time.Since(start).Seconds(),
		Timestamp:           time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (e *Engine) findFiles(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "__pycache__" || name == ".venv" || name == "venv" {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func (e *Engine) isExcluded(file string) bool {
	for _, pattern := range e.ExcludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(file))
		if matched {
			return true
		}
		if strings.Contains(file, pattern) {
			return true
		}
	}
	return false
}

func (e *Engine) scanFile(file string) ([]models.SecretFinding, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []models.SecretFinding
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines and comments
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		for _, rule := range e.Rules {
			for _, pattern := range rule.Patterns {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				matches := re.FindAllStringIndex(line, -1)
				for _, match := range matches {
					matchStr := line[match[0]:match[1]]
					
					// Entropy check if enabled
					entropy := 0.0
					if e.CheckEntropy && rule.MinEntropy > 0 {
						entropy = calculateEntropy(matchStr)
						if entropy < rule.MinEntropy {
							continue
						}
					}

					finding := models.SecretFinding{
						RuleID:      rule.ID,
						RuleName:    rule.Name,
						Severity:    rule.Severity,
						SecretType:  rule.SecretType,
						File:        file,
						Line:        lineNum,
						Column:      match[0] + 1,
						Match:       matchStr,
						Context:     strings.TrimSpace(line),
						Remediation: rule.Remediation,
						Confidence:  rule.Confidence,
						Entropy:     entropy,
					}
					findings = append(findings, finding)
				}
			}
		}
	}

	return findings, scanner.Err()
}

func buildSummary(findings []models.SecretFinding, files int) models.SecretsSummary {
	summary := models.SecretsSummary{
		Critical:     0,
		High:         0,
		Medium:       0,
		Low:          0,
		FilesScanned: files,
		SecretsFound: len(findings),
		ByType:       make(map[string]int),
		ByFile:       make(map[string]int),
	}

	for _, f := range findings {
		summary.ByType[f.SecretType]++
		summary.ByFile[f.File]++
		switch f.Severity {
		case "critical":
			summary.Critical++
		case "high":
			summary.High++
		case "medium":
			summary.Medium++
		case "low":
			summary.Low++
		}
	}
	return summary
}

func calculateEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int)
	for _, r := range s {
		freq[r]++
	}
	entropy := 0.0
	length := float64(len(s))
	for _, count := range freq {
		p := float64(count) / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}
