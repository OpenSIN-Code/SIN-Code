package scanner

import (
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/internal/osv"
	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/internal/parser"
	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/pkg/models"
)

// SCA handles software composition analysis scanning.
type SCA struct {
	osvClient *osv.Client
}

// New creates a new SCA scanner.
func New(osvClient *osv.Client) *SCA {
	if osvClient == nil {
		osvClient = osv.NewClient(0)
	}
	return &SCA{osvClient: osvClient}
}

// ScanProject scans a complete project for vulnerabilities.
func (s *SCA) ScanProject(projectPath string) (*models.ScanResult, error) {
	ecosystem, err := parser.DetectEcosystem(projectPath)
	if err != nil {
		return nil, fmt.Errorf("detect ecosystem: %w", err)
	}
	fmt.Printf("🔍 Detected ecosystem: %s\n", ecosystem)

	packages, err := parser.ParseDependencies(projectPath, ecosystem)
	if err != nil {
		return nil, fmt.Errorf("parse dependencies: %w", err)
	}
	fmt.Printf("📦 Found %d packages\n", len(packages))

	var allVulns []models.Vulnerability
	batchSize := 1000

	for i := 0; i < len(packages); i += batchSize {
		end := i + batchSize
		if end > len(packages) {
			end = len(packages)
		}
		batch := packages[i:end]

		results, err := s.osvClient.BatchQuery(batch)
		if err != nil {
			fmt.Printf("⚠️ Batch query failed: %v\n", err)
			continue
		}

		for _, vulns := range results {
			allVulns = append(allVulns, vulns...)
		}
	}

	summary := generateSummary(allVulns)

	return &models.ScanResult{
		ProjectPath:     projectPath,
		Ecosystem:       ecosystem,
		Vulnerabilities: allVulns,
		Summary:         summary,
		PackagesScanned: len(packages),
	}, nil
}

func generateSummary(vulns []models.Vulnerability) map[string]int {
	summary := models.NewSummary()
	summary["total"] = len(vulns)

	for _, v := range vulns {
		sev := v.Severity
		if sev == "" {
			sev = "unknown"
		}
		if _, ok := summary[sev]; ok {
			summary[sev]++
		} else {
			summary["unknown"]++
		}
	}

	return summary
}
