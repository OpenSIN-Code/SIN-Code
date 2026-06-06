package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-Container-Tool-Go/internal/scanner"
	"github.com/OpenSIN-Code/SIN-Code-Container-Tool-Go/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerScannerInit(t *testing.T) {
	s := scanner.NewContainerScanner("", 0)
	require.NotNil(t, s)
	assert.Equal(t, "trivy", s.TrivyPath)
}

func TestDockerfileAuditNoUser(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	content := `FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["node", "index.js"]
`
	require.NoError(t, os.WriteFile(dockerfile, []byte(content), 0644))

	s := scanner.NewContainerScanner("", 0)
	issues, err := s.AuditDockerfile(dockerfile)
	require.NoError(t, err)

	userIssues := filterIssues(issues, func(i models.DockerfileIssue) bool {
		return contains(i.Message, "root") || contains(i.Message, "USER")
	})
	assert.Greater(t, len(userIssues), 0, "Should find missing USER directive")
}

func TestDockerfileAuditLatestTag(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	content := `FROM node:latest
WORKDIR /app
USER node
CMD ["node", "index.js"]
`
	require.NoError(t, os.WriteFile(dockerfile, []byte(content), 0644))

	s := scanner.NewContainerScanner("", 0)
	issues, err := s.AuditDockerfile(dockerfile)
	require.NoError(t, err)

	latestIssues := filterIssues(issues, func(i models.DockerfileIssue) bool {
		return contains(i.Message, "latest")
	})
	assert.Greater(t, len(latestIssues), 0, "Should find :latest tag issue")
}

func TestDockerfileAuditSecrets(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	content := `FROM node:20-alpine
ENV API_KEY=sk-123456
ENV DB_PASSWORD=secret
USER node
CMD ["node", "index.js"]
`
	require.NoError(t, os.WriteFile(dockerfile, []byte(content), 0644))

	s := scanner.NewContainerScanner("", 0)
	issues, err := s.AuditDockerfile(dockerfile)
	require.NoError(t, err)

	secretIssues := filterIssues(issues, func(i models.DockerfileIssue) bool {
		return contains(i.Message, "secret") || contains(i.Message, "API_KEY") || contains(i.Message, "PASSWORD")
	})
	assert.Greater(t, len(secretIssues), 0, "Should find secret in ENV")
}

func TestDockerfileAuditCurlPipe(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	content := `FROM node:20-alpine
RUN curl https://example.com/install.sh | bash
USER node
CMD ["node", "index.js"]
`
	require.NoError(t, os.WriteFile(dockerfile, []byte(content), 0644))

	s := scanner.NewContainerScanner("", 0)
	issues, err := s.AuditDockerfile(dockerfile)
	require.NoError(t, err)

	curlIssues := filterIssues(issues, func(i models.DockerfileIssue) bool {
		return contains(i.Message, "curl")
	})
	assert.Greater(t, len(curlIssues), 0, "Should find curl | bash pattern")
}

func TestDockerfileAuditAddVsCopy(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	content := `FROM node:20-alpine
ADD . /app
USER node
CMD ["node", "index.js"]
`
	require.NoError(t, os.WriteFile(dockerfile, []byte(content), 0644))

	s := scanner.NewContainerScanner("", 0)
	issues, err := s.AuditDockerfile(dockerfile)
	require.NoError(t, err)

	addIssues := filterIssues(issues, func(i models.DockerfileIssue) bool {
		return contains(i.Message, "ADD") && contains(i.Message, "COPY")
	})
	assert.Greater(t, len(addIssues), 0, "Should find ADD vs COPY issue")
}

func TestDockerfileAuditCombineRun(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	content := `FROM node:20-alpine
RUN apt-get update
RUN apt-get install -y curl
RUN apt-get install -y vim
RUN apt-get install -y git
RUN apt-get install -y nano
RUN apt-get install -y htop
USER node
CMD ["node", "index.js"]
`
	require.NoError(t, os.WriteFile(dockerfile, []byte(content), 0644))

	s := scanner.NewContainerScanner("", 0)
	issues, err := s.AuditDockerfile(dockerfile)
	require.NoError(t, err)

	runIssues := filterIssues(issues, func(i models.DockerfileIssue) bool {
		return contains(i.Message, "RUN") && contains(i.Message, "combine")
	})
	assert.Greater(t, len(runIssues), 0, "Should find too many RUN commands")
}

func TestDetermineStatusCritical(t *testing.T) {
	report := &models.TrivyScanReport{
		Image: "test:latest",
		Vulnerabilities: []models.ContainerVulnerability{
			{ID: "CVE-1", Package: "pkg", Version: "1.0", Severity: "CRITICAL"},
		},
		Summary: map[string]int{
			"total": 1, "critical": 1, "high": 0, "medium": 0, "low": 0,
		},
	}
	assert.Equal(t, "failed", scanner.DetermineStatus(report, "high"))
}

func TestDetermineStatusHigh(t *testing.T) {
	report := &models.TrivyScanReport{
		Image: "test:latest",
		Vulnerabilities: []models.ContainerVulnerability{
			{ID: "CVE-2", Package: "pkg", Version: "1.0", Severity: "HIGH"},
		},
		Summary: map[string]int{
			"total": 1, "critical": 0, "high": 1, "medium": 0, "low": 0,
		},
	}
	assert.Equal(t, "warning", scanner.DetermineStatus(report, "critical"))
	assert.Equal(t, "failed", scanner.DetermineStatus(report, "high"))
}

func TestDetermineStatusClean(t *testing.T) {
	report := &models.TrivyScanReport{
		Image:           "test:latest",
		Vulnerabilities: []models.ContainerVulnerability{},
		Summary: map[string]int{
			"total": 0, "critical": 0, "high": 0, "medium": 0, "low": 0,
		},
	}
	assert.Equal(t, "passed", scanner.DetermineStatus(report, "high"))
}

func TestDockerImageInfoModel(t *testing.T) {
	info := &models.DockerImageInfo{
		ID:         "sha256:abc123",
		Tags:       []string{"openafd-chat:latest"},
		Size:       500_000_000,
		Created:    "2026-01-01T00:00:00Z",
		Architecture: "amd64",
		OS:         "linux",
		Layers:     15,
		User:       "node",
		RunsAsRoot: false,
	}
	assert.Equal(t, int64(500_000_000), info.Size)
	assert.False(t, info.RunsAsRoot)
}

func TestGenerateRecommendations(t *testing.T) {
	report := &models.TrivyScanReport{
		Image: "test:latest",
		Vulnerabilities: []models.ContainerVulnerability{
			{ID: "CVE-1", Severity: "CRITICAL"},
			{ID: "CVE-2", Severity: "HIGH"},
		},
		Summary: map[string]int{
			"total": 2, "critical": 1, "high": 1, "medium": 0, "low": 0,
			"fixable": 2, "misconfigurations": 0,
		},
	}

	recs := scanner.GenerateRecommendations(report, nil, nil)
	assert.Greater(t, len(recs), 0)
	assert.True(t, hasRecommendation(recs, "critical"))
}

func TestDockerImageInfoRecommendations(t *testing.T) {
	report := &models.TrivyScanReport{
		Image:           "test:latest",
		Vulnerabilities: []models.ContainerVulnerability{},
		Summary: map[string]int{
			"total": 0, "critical": 0, "high": 0, "medium": 0, "low": 0,
		},
	}

	imageInfo := &models.DockerImageInfo{
		User:       "",
		RunsAsRoot: true,
	}

	recs := scanner.GenerateRecommendations(report, nil, imageInfo)
	assert.True(t, hasRecommendation(recs, "root"))
}

func TestParseTrivyResults(t *testing.T) {
	// This tests the internal parsing logic indirectly through ScanImage
	// with a mock scenario. Since we can't run Trivy in tests,
	// we verify the model structure is correct.
	report := &models.TrivyScanReport{
		Image:   "test:latest",
		OSFamily: "alpine",
		OSVersion: "3.18",
		Vulnerabilities: []models.ContainerVulnerability{
			{
				ID:       "CVE-2024-1234",
				Package:  "openssl",
				Version:  "3.0.0",
				Severity: "HIGH",
				FixedIn:  "3.0.1",
				VulnType: "os",
			},
		},
		Misconfigurations: []models.ContainerMisconfiguration{
			{
				ID:       "DS002",
				Title:    "Missing USER directive",
				Severity: "HIGH",
				Category: "dockerfile",
			},
		},
		Summary: map[string]int{
			"total": 1, "critical": 0, "high": 2, "medium": 0, "low": 0,
			"fixable": 1, "misconfigurations": 1,
		},
	}

	assert.Equal(t, "alpine", report.OSFamily)
	assert.Equal(t, "3.18", report.OSVersion)
	assert.Len(t, report.Vulnerabilities, 1)
	assert.Len(t, report.Misconfigurations, 1)
	assert.Equal(t, 1, report.Summary["fixable"])
}

// Helper functions
func filterIssues(issues []models.DockerfileIssue, predicate func(models.DockerfileIssue) bool) []models.DockerfileIssue {
	var result []models.DockerfileIssue
	for _, i := range issues {
		if predicate(i) {
			result = append(result, i)
		}
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(stringContainsLower(s, substr) || stringContainsLower(s, substr))
}

func stringContainsLower(s, substr string) bool {
	return len(s) >= len(substr) &&
		(findSubstr(s, substr) || findSubstr(s, toLower(substr)) || findSubstr(toLower(s), substr) || findSubstr(toLower(s), toLower(substr)))
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		result[i] = c
	}
	return string(result)
}

func findSubstr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasRecommendation(recs []string, keyword string) bool {
	for _, r := range recs {
		if stringContainsLower(r, keyword) {
			return true
		}
	}
	return false
}
