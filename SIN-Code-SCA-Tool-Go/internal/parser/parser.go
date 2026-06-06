package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/pkg/models"
)

// DetectEcosystem determines the package ecosystem of a project.
func DetectEcosystem(projectPath string) (string, error) {
	files := []struct {
		path      string
		ecosystem string
	}{
		{"package-lock.json", "npm"},
		{"yarn.lock", "npm"},
		{"pnpm-lock.yaml", "npm"},
		{"package.json", "npm"},
		{"requirements.txt", "PyPI"},
		{"Pipfile.lock", "PyPI"},
		{"poetry.lock", "PyPI"},
		{"go.mod", "Go"},
		{"pom.xml", "Maven"},
	}

	for _, f := range files {
		if _, err := os.Stat(filepath.Join(projectPath, f.path)); err == nil {
			return f.ecosystem, nil
		}
	}

	return "", fmt.Errorf("no known ecosystem detected")
}

// ParseDependencies parses all dependencies for the detected ecosystem.
func ParseDependencies(projectPath, ecosystem string) ([]models.Package, error) {
	switch ecosystem {
	case "npm":
		return parseNPM(projectPath)
	case "PyPI":
		return parsePyPI(projectPath)
	case "Go":
		return parseGo(projectPath)
	case "Maven":
		return parseMaven(projectPath)
	default:
		return nil, fmt.Errorf("unsupported ecosystem: %s", ecosystem)
	}
}

func parseNPM(projectPath string) ([]models.Package, error) {
	lockFile := filepath.Join(projectPath, "package-lock.json")
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return nil, fmt.Errorf("read package-lock.json: %w", err)
	}

	var lockData struct {
		Packages map[string]struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"packages"`
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}

	if err := json.Unmarshal(data, &lockData); err != nil {
		return nil, fmt.Errorf("parse package-lock.json: %w", err)
	}

	var packages []models.Package

	// npm v2+ format
	for pkgPath, pkg := range lockData.Packages {
		if pkgPath == "" {
			// Root project
			if pkg.Name != "" && pkg.Version != "" {
				packages = append(packages, models.Package{
					Name:      pkg.Name,
					Version:   pkg.Version,
					Ecosystem: "npm",
				})
			}
			continue
		}
		if !strings.HasPrefix(pkgPath, "node_modules/") {
			continue
		}
		name := pkg.Name
		if name == "" {
			name = strings.TrimPrefix(pkgPath, "node_modules/")
		}
		if name != "" && pkg.Version != "" {
			packages = append(packages, models.Package{
				Name:      name,
				Version:   pkg.Version,
				Ecosystem: "npm",
			})
		}
	}

	// npm v1 format (legacy)
	for name, dep := range lockData.Dependencies {
		if name != "" && dep.Version != "" {
			packages = append(packages, models.Package{
				Name:      name,
				Version:   dep.Version,
				Ecosystem: "npm",
			})
		}
	}

	return packages, nil
}

func parsePyPI(projectPath string) ([]models.Package, error) {
	reqFile := filepath.Join(projectPath, "requirements.txt")
	data, err := os.ReadFile(reqFile)
	if err != nil {
		return nil, fmt.Errorf("read requirements.txt: %w", err)
	}

	var packages []models.Package
	re := regexp.MustCompile(`^([a-zA-Z0-9_-]+)==([^\s]+)`)

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			packages = append(packages, models.Package{
				Name:      matches[1],
				Version:   matches[2],
				Ecosystem: "PyPI",
			})
		}
	}

	return packages, nil
}

func parseGo(projectPath string) ([]models.Package, error) {
	goMod := filepath.Join(projectPath, "go.mod")
	data, err := os.ReadFile(goMod)
	if err != nil {
		return nil, fmt.Errorf("read go.mod: %w", err)
	}

	var packages []models.Package
	re := regexp.MustCompile(`^([^\s]+)\s+(v[^\s]+)`)
	inRequire := false

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}

		if inRequire || strings.HasPrefix(line, "require ") {
			matches := re.FindStringSubmatch(line)
			if len(matches) == 3 {
				packages = append(packages, models.Package{
					Name:      matches[1],
					Version:   matches[2],
					Ecosystem: "Go",
				})
			}
		}
	}

	return packages, nil
}

func parseMaven(projectPath string) ([]models.Package, error) {
	pomFile := filepath.Join(projectPath, "pom.xml")
	data, err := os.ReadFile(pomFile)
	if err != nil {
		return nil, fmt.Errorf("read pom.xml: %w", err)
	}

	var packages []models.Package
	re := regexp.MustCompile(`<dependency>\s*<groupId>([^<]+)</groupId>\s*<artifactId>([^<]+)</artifactId>\s*<version>([^<]+)</version>`)
	matches := re.FindAllStringSubmatch(string(data), -1)

	for _, m := range matches {
		if len(m) == 4 {
			packages = append(packages, models.Package{
				Name:      m[1] + ":" + m[2],
				Version:   m[3],
				Ecosystem: "Maven",
			})
		}
	}

	return packages, nil
}
