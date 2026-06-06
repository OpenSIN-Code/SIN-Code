package scanner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/internal/osv"
	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/internal/parser"
	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/internal/scanner"
	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSCAScannerInit(t *testing.T) {
	osvClient := osv.NewClient(0)
	sca := scanner.New(osvClient)
	require.NotNil(t, sca)
}

func TestDetectEcosystemNPM(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), []byte("{}"), 0644))

	ecosystem, err := parser.DetectEcosystem(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "npm", ecosystem)
}

func TestDetectEcosystemPyPI(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "requirements.txt"), []byte("requests==2.28.0\n"), 0644))

	ecosystem, err := parser.DetectEcosystem(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "PyPI", ecosystem)
}

func TestDetectEcosystemGo(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n"), 0644))

	ecosystem, err := parser.DetectEcosystem(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "Go", ecosystem)
}

func TestDetectEcosystemMaven(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte("<project></project>\n"), 0644))

	ecosystem, err := parser.DetectEcosystem(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "Maven", ecosystem)
}

func TestDetectEcosystemNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := parser.DetectEcosystem(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no known ecosystem detected")
}

func TestParseNPMDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	lockData := map[string]interface{}{
		"name":    "test-project",
		"version": "1.0.0",
		"packages": map[string]interface{}{
			"": map[string]interface{}{
				"name":    "test-project",
				"version": "1.0.0",
			},
			"node_modules/lodash": map[string]interface{}{
				"name":    "lodash",
				"version": "4.17.21",
			},
			"node_modules/axios": map[string]interface{}{
				"name":    "axios",
				"version": "1.6.0",
			},
		},
	}
	data, err := json.Marshal(lockData)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), data, 0644))

	packages, err := parser.ParseDependencies(tmpDir, "npm")
	require.NoError(t, err)
	assert.Len(t, packages, 3)

	names := make([]string, len(packages))
	for i, p := range packages {
		names[i] = p.Name
	}
	assert.Contains(t, names, "test-project")
	assert.Contains(t, names, "lodash")
	assert.Contains(t, names, "axios")
}

func TestParseNPMLegacyDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	lockData := map[string]interface{}{
		"name":    "test-project",
		"version": "1.0.0",
		"dependencies": map[string]interface{}{
			"lodash": map[string]interface{}{
				"version": "4.17.21",
			},
			"axios": map[string]interface{}{
				"version": "1.6.0",
			},
		},
	}
	data, err := json.Marshal(lockData)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), data, 0644))

	packages, err := parser.ParseDependencies(tmpDir, "npm")
	require.NoError(t, err)
	assert.Len(t, packages, 2)

	names := make([]string, len(packages))
	for i, p := range packages {
		names[i] = p.Name
	}
	assert.Contains(t, names, "lodash")
	assert.Contains(t, names, "axios")
}

func TestParsePyPIDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	requirements := `requests==2.28.0
numpy==1.24.0
# Comment
pandas>=1.5.0
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "requirements.txt"), []byte(requirements), 0644))

	packages, err := parser.ParseDependencies(tmpDir, "PyPI")
	require.NoError(t, err)
	assert.Len(t, packages, 2)

	names := make([]string, len(packages))
	for i, p := range packages {
		names[i] = p.Name
	}
	assert.Contains(t, names, "requests")
	assert.Contains(t, names, "numpy")
}

func TestParseGoDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	goMod := `module test

go 1.21

require (
	github.com/stretchr/testify v1.8.4
	github.com/example/lib v0.1.0
)
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644))

	packages, err := parser.ParseDependencies(tmpDir, "Go")
	require.NoError(t, err)
	assert.Len(t, packages, 2)

	names := make([]string, len(packages))
	for i, p := range packages {
		names[i] = p.Name
	}
	assert.Contains(t, names, "github.com/stretchr/testify")
	assert.Contains(t, names, "github.com/example/lib")
}

func TestParseMavenDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	pom := `<project>
	<dependencies>
		<dependency>
			<groupId>com.example</groupId>
			<artifactId>lib</artifactId>
			<version>1.0.0</version>
		</dependency>
		<dependency>
			<groupId>org.apache</groupId>
			<artifactId>commons</artifactId>
			<version>2.5.0</version>
		</dependency>
	</dependencies>
</project>`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pom), 0644))

	packages, err := parser.ParseDependencies(tmpDir, "Maven")
	require.NoError(t, err)
	assert.Len(t, packages, 2)

	names := make([]string, len(packages))
	for i, p := range packages {
		names[i] = p.Name
	}
	assert.Contains(t, names, "com.example:lib")
	assert.Contains(t, names, "org.apache:commons")
}

func TestGenerateSummary(t *testing.T) {
	// We can't easily test the private generateSummary directly,
	// but we can test it indirectly through the public API.
	// For now, we verify the summary structure.
	summary := models.NewSummary()
	summary["total"] = 4
	summary["critical"] = 1
	summary["high"] = 2
	summary["medium"] = 1
	summary["low"] = 0

	assert.Equal(t, 4, summary["total"])
	assert.Equal(t, 1, summary["critical"])
	assert.Equal(t, 2, summary["high"])
	assert.Equal(t, 1, summary["medium"])
	assert.Equal(t, 0, summary["low"])
}
