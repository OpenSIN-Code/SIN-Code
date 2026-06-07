// SPDX-License-Identifier: MIT
package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Container-Tool-Go/pkg/models"
)

// ContainerScanner handles container security scanning.
type ContainerScanner struct {
	TrivyPath         string
	timeout           time.Duration
	DockerAvailable   bool
	HadolintAvailable bool
}

// NewContainerScanner creates a new container scanner.
func NewContainerScanner(trivyPath string, timeout time.Duration) *ContainerScanner {
	if trivyPath == "" {
		trivyPath = "trivy"
	}
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	return &ContainerScanner{
		TrivyPath:         trivyPath,
		timeout:           timeout,
		DockerAvailable:   checkDocker(),
		HadolintAvailable: checkHadolint(),
	}
}

func checkDocker() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "version")
	return cmd.Run() == nil
}

func checkHadolint() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "hadolint", "--version")
	return cmd.Run() == nil
}

// ScanImage performs a complete security scan of a container image.
func (s *ContainerScanner) ScanImage(image, failOn, scanType, dockerfilePath string, includeSBOM bool) (*models.ContainerScanResult, error) {
	fmt.Printf("🔍 Starting scan of image: %s\n", image)

	// 1. Run Trivy scan
	fmt.Println("   📦 Running Trivy scan...")
	scanReport, err := s.runTrivyScan(image, scanType)
	if err != nil {
		return nil, fmt.Errorf("trivy scan: %w", err)
	}

	// 2. Get Docker image info if available
	var imageInfo *models.DockerImageInfo
	if s.DockerAvailable {
		fmt.Println("   🐳 Getting image metadata...")
		imageInfo = s.GetImageInfo(image)
	}

	// 3. Audit Dockerfile if provided
	var dockerfileIssues []models.DockerfileIssue
	if dockerfilePath != "" {
		fmt.Printf("   📋 Auditing Dockerfile: %s\n", dockerfilePath)
		issues, err := s.AuditDockerfile(dockerfilePath)
		if err != nil {
			fmt.Printf("   ⚠️ Dockerfile audit error: %v\n", err)
		} else {
			dockerfileIssues = issues
		}
	}

	// 4. Determine status
	status := DetermineStatus(scanReport, failOn)

	// 5. Generate recommendations
	recommendations := GenerateRecommendations(scanReport, dockerfileIssues, imageInfo)

	return &models.ContainerScanResult{
		Image:             image,
		ImageInfo:         imageInfo,
		ScanReport:        *scanReport,
		DockerfileIssues:  dockerfileIssues,
		Status:            status,
		Recommendations:   recommendations,
	}, nil
}

// ScanFilesystem performs a security scan of a filesystem path.
func (s *ContainerScanner) ScanFilesystem(path, failOn, scanType, dockerfilePath string) (*models.ContainerScanResult, error) {
	fmt.Printf("🔍 Starting filesystem scan of: %s\n", path)

	// 1. Run Trivy filesystem scan
	fmt.Println("   📦 Running Trivy filesystem scan...")
	scanReport, err := s.runTrivyFilesystemScan(path, scanType)
	if err != nil {
		return nil, fmt.Errorf("trivy fs scan: %w", err)
	}

	// 2. Audit Dockerfile if provided
	var dockerfileIssues []models.DockerfileIssue
	if dockerfilePath != "" {
		fmt.Printf("   📋 Auditing Dockerfile: %s\n", dockerfilePath)
		issues, err := s.AuditDockerfile(dockerfilePath)
		if err != nil {
			fmt.Printf("   ⚠️ Dockerfile audit error: %v\n", err)
		} else {
			dockerfileIssues = issues
		}
	}

	// 3. Determine status
	status := DetermineStatus(scanReport, failOn)

	// 4. Generate recommendations
	recommendations := GenerateRecommendations(scanReport, dockerfileIssues, nil)

	return &models.ContainerScanResult{
		Image:             path,
		ScanReport:        *scanReport,
		DockerfileIssues:  dockerfileIssues,
		Status:            status,
		Recommendations:   recommendations,
	}, nil
}

func (s *ContainerScanner) runTrivyScan(image, scanType string) (*models.TrivyScanReport, error) {
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
	cmdArgs = append(cmdArgs, image)

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.TrivyPath, cmdArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Continue parsing
		} else {
			return nil, fmt.Errorf("trivy failed: %w", err)
		}
	}

	out := strings.TrimSpace(string(output))
	if out == "" {
		return &models.TrivyScanReport{Image: image, Summary: models.NewTrivySummary()}, nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, fmt.Errorf("parse trivy output: %w", err)
	}

	return parseTrivyResults(image, data), nil
}

func (s *ContainerScanner) runTrivyFilesystemScan(path, scanType string) (*models.TrivyScanReport, error) {
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
	cmdArgs = append(cmdArgs, path)

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.TrivyPath, cmdArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Continue parsing
		} else {
			return nil, fmt.Errorf("trivy fs failed: %w", err)
		}
	}

	out := strings.TrimSpace(string(output))
	if out == "" {
		return &models.TrivyScanReport{Image: path, Summary: models.NewTrivySummary()}, nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, fmt.Errorf("parse trivy fs output: %w", err)
	}

	return parseTrivyResults(path, data), nil
}

// GetImageInfo retrieves Docker image metadata via Docker CLI.
func (s *ContainerScanner) GetImageInfo(image string) *models.DockerImageInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dockerCmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)

	output, err := dockerCmd.CombinedOutput()
	if err != nil {
		return nil
	}

	var inspectData []map[string]interface{}
	if err := json.Unmarshal(output, &inspectData); err != nil || len(inspectData) == 0 {
		return nil
	}

	img := inspectData[0]
	config, _ := img["Config"].(map[string]interface{})

	user := getString(config, "User")
	runsAsRoot := user == "" || user == "root" || user == "0"

	env := make(map[string]string)
	if envList, ok := config["Env"].([]interface{}); ok {
		for _, e := range envList {
			if s, ok := e.(string); ok {
				if idx := strings.Index(s, "="); idx > 0 {
					env[s[:idx]] = s[idx+1:]
				}
			}
		}
	}

	var exposedPorts []string
	if ports, ok := config["ExposedPorts"].(map[string]interface{}); ok {
		for port := range ports {
			exposedPorts = append(exposedPorts, port)
		}
	}

	rootFS, _ := img["RootFS"].(map[string]interface{})
	layers, _ := rootFS["Layers"].([]interface{})

	var entrypoint, cmd []string
	if ep, ok := config["Entrypoint"].([]interface{}); ok {
		for _, v := range ep {
			if s, ok := v.(string); ok {
				entrypoint = append(entrypoint, s)
			}
		}
	}
	if c, ok := config["Cmd"].([]interface{}); ok {
		for _, v := range c {
			if s, ok := v.(string); ok {
				cmd = append(cmd, s)
			}
		}
	}

	return &models.DockerImageInfo{
		ID:           getString(img, "Id"),
		Tags:         getStringSlice(img, "RepoTags"),
		Size:         int64(getFloat(img, "Size")),
		Created:      getString(img, "Created"),
		Architecture: getString(img, "Architecture"),
		OS:           getString(img, "Os"),
		Layers:       len(layers),
		Author:       getString(config, "Author"),
		ExposedPorts: exposedPorts,
		Environment:  env,
		User:         user,
		WorkingDir:   getString(config, "WorkingDir"),
		Entrypoint:   entrypoint,
		Cmd:          cmd,
		RunsAsRoot:   runsAsRoot,
	}
}

// AuditDockerfile audits a Dockerfile for best practices and security issues.
func (s *ContainerScanner) AuditDockerfile(path string) ([]models.DockerfileIssue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dockerfile: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var issues []models.DockerfileIssue

	// Run Hadolint if available
	if s.HadolintAvailable {
		issues = append(issues, s.runHadolint(path)...)
	}

	// Run custom best-practice checks
	issues = append(issues, checkBestPractices(lines)...)

	// Run special checks
	issues = append(issues, checkSpecial(lines)...)

	return issues, nil
}

func (s *ContainerScanner) runHadolint(path string) []models.DockerfileIssue {
	var issues []models.DockerfileIssue

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "hadolint", "--format", "json", path)

	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return issues
	}

	var hadolintData []map[string]interface{}
	if err := json.Unmarshal(output, &hadolintData); err != nil {
		return issues
	}

	for _, item := range hadolintData {
		line := 1
		if l, ok := item["line"].(float64); ok {
			line = int(l)
		}
		column := 0
		if c, ok := item["column"].(float64); ok {
			column = int(c)
		}
		issues = append(issues, models.DockerfileIssue{
			RuleID:   getString(item, "code"),
			Message:  getString(item, "message"),
			Severity: strings.ToUpper(getString(item, "level")),
			Line:     line,
			Column:   column,
		})
	}

	return issues
}

func checkBestPractices(lines []string) []models.DockerfileIssue {
	var issues []models.DockerfileIssue
	var hasUserDirective bool
	runCount := 0

	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}

		lineNum := i + 1

		// USER directive
		if strings.HasPrefix(stripped, "USER ") {
			hasUserDirective = true
			parts := strings.Fields(stripped)
			if len(parts) > 1 && (parts[1] == "root" || parts[1] == "0") {
				issues = append(issues, models.DockerfileIssue{
					RuleID:     "SIN001",
					Message:    "Container runs as root user",
					Severity:   "HIGH",
					Line:       lineNum,
					Suggestion: "Use a non-root user (e.g., USER appuser)",
				})
			}
		}

		// FROM with :latest
		if strings.HasPrefix(stripped, "FROM ") && strings.Contains(stripped, ":latest") {
			issues = append(issues, models.DockerfileIssue{
				RuleID:     "SIN002",
				Message:    "FROM uses :latest tag (not reproducible)",
				Severity:   "MEDIUM",
				Line:       lineNum,
				Suggestion: "Use a specific tag with SHA digest",
			})
		}

		// ADD when COPY would work
		if strings.HasPrefix(stripped, "ADD ") {
			if !strings.Contains(stripped, "http") && !strings.Contains(stripped, ".tar") && !strings.Contains(stripped, ".tgz") {
				issues = append(issues, models.DockerfileIssue{
					RuleID:     "SIN003",
					Message:    "ADD used, but COPY would suffice",
					Severity:   "LOW",
					Line:       lineNum,
					Suggestion: "Use COPY for local files",
				})
			}
		}

		// Count RUN commands
		if strings.HasPrefix(stripped, "RUN ") {
			runCount++
		}

		// Secrets in ENV
		if strings.HasPrefix(stripped, "ENV ") {
			upper := strings.ToUpper(stripped)
			sensitive := []string{"PASSWORD", "SECRET", "KEY", "TOKEN", "API_KEY", "CREDENTIAL"}
			for _, key := range sensitive {
				if strings.Contains(upper, key) {
					issues = append(issues, models.DockerfileIssue{
						RuleID:     "SIN004",
						Message:    fmt.Sprintf("Possible secret in ENV found (%s)", key),
						Severity:   "CRITICAL",
						Line:       lineNum,
						Suggestion: "Use Docker Secrets or runtime environment variables",
					})
				}
			}
		}
	}

	// Global checks
	if !hasUserDirective {
		issues = append(issues, models.DockerfileIssue{
			RuleID:     "SIN001",
			Message:    "Container should not run as root",
			Severity:   "HIGH",
			Line:       0,
			Suggestion: "Add 'USER nonroot' at the end of the Dockerfile",
		})
	}

	if runCount > 5 {
		issues = append(issues, models.DockerfileIssue{
			RuleID:     "SIN005",
			Message:    fmt.Sprintf("Too many RUN commands (%d), combine with && to reduce layers", runCount),
			Severity:   "LOW",
			Line:       0,
			Suggestion: "Use 'RUN apt-get update && apt-get install -y pkg'",
		})
	}

	return issues
}

func checkSpecial(lines []string) []models.DockerfileIssue {
	var issues []models.DockerfileIssue
	curlPipeRe := regexp.MustCompile(`(?i)curl\s+.*\|\s*(bash|sh)`)

	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		lineNum := i + 1

		if strings.HasPrefix(stripped, "RUN ") && curlPipeRe.MatchString(stripped) {
			issues = append(issues, models.DockerfileIssue{
				RuleID:     "SIN006",
				Message:    "curl | bash pattern detected (dangerous remote execution)",
				Severity:   "CRITICAL",
				Line:       lineNum,
				Suggestion: "Download script first, verify checksum, then execute",
			})
		}
	}

	return issues
}

// DetermineStatus determines the overall scan status based on findings and fail threshold.
func DetermineStatus(report *models.TrivyScanReport, failOn string) string {
	failOn = strings.ToLower(failOn)
	order := []string{"critical", "high", "medium", "low"}
	failIdx := -1
	for i, o := range order {
		if o == failOn {
			failIdx = i
			break
		}
	}
	if failIdx == -1 {
		failIdx = 1 // default to high
	}

	for i := 0; i <= failIdx; i++ {
		if report.Summary[order[i]] > 0 {
			return "failed"
		}
	}

	if report.Summary["total"] > 0 {
		return "warning"
	}
	return "passed"
}

// GenerateRecommendations generates actionable recommendations based on scan results.
func GenerateRecommendations(report *models.TrivyScanReport, issues []models.DockerfileIssue, imageInfo *models.DockerImageInfo) []string {
	var recs []string

	if report.Summary["critical"] > 0 {
		recs = append(recs, fmt.Sprintf("🚨 %d critical vulnerabilities found - prioritize immediate patching", report.Summary["critical"]))
	}
	if report.Summary["high"] > 0 {
		recs = append(recs, fmt.Sprintf("⚠️  %d high vulnerabilities found - patch within 7 days", report.Summary["high"]))
	}
	if report.Summary["fixable"] > 0 {
		recs = append(recs, fmt.Sprintf("🔧 %d vulnerabilities have available fixes - run 'trivy image --ignore-unfixed' to see only fixable", report.Summary["fixable"]))
	}
	if report.Summary["misconfigurations"] > 0 {
		recs = append(recs, fmt.Sprintf("🏗️  %d misconfigurations found - review Dockerfile and container configs", report.Summary["misconfigurations"]))
	}

	if imageInfo != nil && imageInfo.RunsAsRoot {
		recs = append(recs, "👤 Container runs as root - add a non-root USER directive")
	}

	for _, issue := range issues {
		if issue.RuleID == "SIN002" {
			recs = append(recs, "🏷️  Avoid :latest tag - use pinned versions for reproducible builds")
		}
		if issue.RuleID == "SIN004" {
			recs = append(recs, "🔐 Secrets detected in Dockerfile - use Docker Secrets or mount at runtime")
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "✅ No issues found - container security looks good!")
	}

	return recs
}

func parseTrivyResults(image string, data map[string]interface{}) *models.TrivyScanReport {
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

	return &models.TrivyScanReport{
		Image:             image,
		OSFamily:          osFamily,
		OSVersion:         osVersion,
		Vulnerabilities:   vulns,
		Misconfigurations: misconfigs,
		Summary:           generateSummary(vulns, misconfigs),
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

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
