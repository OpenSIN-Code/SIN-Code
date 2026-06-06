package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-SAST-Tool/pkg/models"
)

// Engine represents the SAST scanning engine
type Engine struct {
	Rules         []models.Rule
	ExcludePatterns []string
}

// NewEngine creates a new SAST engine with the given rules
func NewEngine(rules []models.Rule, exclude []string) *Engine {
	return &Engine{
		Rules:           rules,
		ExcludePatterns: exclude,
	}
}

// Scan performs a SAST scan on the given path
func (e *Engine) Scan(path string) (*models.SASTResult, error) {
	start := time.Now()

	files, err := e.findFiles(path)
	if err != nil {
		return nil, fmt.Errorf("finding files: %w", err)
	}

	var findings []models.SASTFinding
	filesScanned := 0
	linesScanned := 0

	for _, file := range files {
		if e.isExcluded(file) {
			continue
		}
		lang := detectLanguage(file)
		fileFindings, fileLines, err := e.scanFile(file, lang)
		if err != nil {
			continue
		}
		findings = append(findings, fileFindings...)
		filesScanned++
		linesScanned += fileLines
	}

	summary := buildSummary(findings, filesScanned, linesScanned)
	status := "passed"
	if summary.Critical > 0 {
		status = "failed"
	} else if summary.High > 0 {
		status = "warning"
	}

	return &models.SASTResult{
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

	extensions := map[string]bool{
		".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".go": true, ".java": true, ".php": true, ".rb": true, ".cs": true,
		".c": true, ".cpp": true, ".h": true, ".hpp": true,
		".yaml": true, ".yml": true, ".json": true, ".xml": true,
		".sh": true, ".bash": true, ".ps1": true,
	}

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
		ext := strings.ToLower(filepath.Ext(path))
		if extensions[ext] {
			files = append(files, path)
		}
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

func (e *Engine) scanFile(file, lang string) ([]models.SASTFinding, int, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var findings []models.SASTFinding
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		for _, rule := range e.Rules {
			if !contains(rule.Languages, lang) && !contains(rule.Languages, "*") {
				continue
			}
			for _, pattern := range rule.Patterns {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				loc := re.FindStringIndex(line)
				if loc != nil {
					finding := models.SASTFinding{
						RuleID:      rule.ID,
						RuleName:    rule.Name,
						Severity:    rule.Severity,
						CWE:         rule.CWE,
						OWASP:       rule.OWASP,
						Language:    lang,
						File:        file,
						Line:        lineNum,
						Column:      loc[0] + 1,
						Match:       strings.TrimSpace(line[loc[0]:loc[1]]),
						Context:     strings.TrimSpace(line),
						Remediation: rule.Remediation,
						Confidence:  rule.Confidence,
						Description: rule.Description,
					}
					findings = append(findings, finding)
				}
			}
		}
	}

	return findings, lineNum, scanner.Err()
}

func detectLanguage(file string) string {
	ext := strings.ToLower(filepath.Ext(file))
	langMap := map[string]string{
		".py": "python", ".js": "javascript", ".ts": "typescript",
		".jsx": "javascript", ".tsx": "typescript",
		".go": "go", ".java": "java", ".php": "php", ".rb": "ruby",
		".cs": "csharp", ".c": "c", ".cpp": "cpp", ".h": "c", ".hpp": "cpp",
		".yaml": "yaml", ".yml": "yaml", ".json": "json", ".xml": "xml",
		".sh": "bash", ".bash": "bash", ".ps1": "powershell",
	}
	return langMap[ext]
}

func buildSummary(findings []models.SASTFinding, files, lines int) models.SASTSummary {
	summary := models.SASTSummary{
		Critical:       0,
		High:           0,
		Medium:         0,
		Low:            0,
		FilesScanned:   files,
		LinesScanned:   lines,
		RulesTriggered: len(findings),
		ByLanguage:     make(map[string]int),
		ByOWASP:        make(map[string]int),
	}

	seenRules := make(map[string]bool)
	for _, f := range findings {
		seenRules[f.RuleID] = true
		summary.ByLanguage[f.Language]++
		summary.ByOWASP[f.OWASP]++
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
	summary.RulesTriggered = len(seenRules)
	return summary
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
