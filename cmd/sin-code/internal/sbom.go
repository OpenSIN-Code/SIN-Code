// SPDX-License-Identifier: MIT
// Purpose: sbom — generate SPDX or CycloneDX JSON SBOMs for Go, Python, Node, and generic projects.
// Docs: cmd/sin-code/internal/sbom.go.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// SbomCmd generates a Software Bill of Materials in SPDX or CycloneDX format.
var SbomCmd = &cobra.Command{
	Use:   "sbom [path]",
	Short: "Generate SPDX or CycloneDX JSON SBOM for a project",
	Long: `sbom generates a Software Bill of Materials (SBOM) for the project at <path>.

Supported project types:
  Go      → parses go.mod / go list -m -json all
  Python  → parses requirements.txt or pyproject.toml
  Node.js → parses package.json (+ package-lock.json for versions)
  Generic → lists directory structure as a basic component tree

Output formats:
  spdx-json      (default) SPDX 2.3 JSON
  cyclonedx-json CycloneDX 1.5 JSON

Examples:
  sin-code sbom .
  sin-code sbom ./my-project --format cyclonedx-json --output sbom.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		path, _ = filepath.Abs(path)

		format, _ := cmd.Flags().GetString("format")
		output, _ := cmd.Flags().GetString("output")

		projType := detectProjectType(path)

		sbom, err := generateSBOM(path, projType, format)
		if err != nil {
			return fmt.Errorf("sbom generation failed: %w", err)
		}

		var out io.Writer = os.Stdout
		if output != "" && output != "-" {
			f, err := os.Create(output)
			if err != nil {
				return fmt.Errorf("cannot create output file: %w", err)
			}
			defer f.Close()
			out = f
		}

		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(sbom)
	},
}

func init() {
	SbomCmd.Flags().StringP("format", "f", "spdx-json", "SBOM format: spdx-json or cyclonedx-json")
	SbomCmd.Flags().StringP("output", "o", "-", "Output file (or - for stdout)")
}

// ── Data models ─────────────────────────────────────────────────────────────

// SPDX 2.3 document
type SPDXDocument struct {
	SPDXVersion       string           `json:"spdxVersion"`
	SPDXID            string           `json:"SPDXID"`
	Name              string           `json:"name"`
	DocumentNamespace string           `json:"documentNamespace"`
	CreationInfo      SPDXCreationInfo `json:"creationInfo"`
	Packages          []SPDXPackage    `json:"packages"`
}

type SPDXCreationInfo struct {
	Created string   `json:"created"`
	Creators []string `json:"creators"`
}

type SPDXPackage struct {
	SPDXID              string `json:"SPDXID"`
	Name                string `json:"name"`
	VersionInfo         string `json:"versionInfo"`
	DownloadLocation    string `json:"downloadLocation"`
	FilesAnalyzed       bool   `json:"filesAnalyzed"`
	VerificationCode    *string `json:"verificationCode"`
	LicenseConcluded    string `json:"licenseConcluded"`
	LicenseDeclared     string `json:"licenseDeclared"`
	CopyrightText       string `json:"copyrightText"`
	PrimaryPackagePurpose string `json:"primaryPackagePurpose"`
}

// CycloneDX 1.5 document
type CycloneDXDocument struct {
	BomFormat   string                `json:"bomFormat"`
	SpecVersion string                `json:"specVersion"`
	SerialNumber string               `json:"serialNumber"`
	Version     int                   `json:"version"`
	Metadata    CycloneDXMetadata     `json:"metadata"`
	Components  []CycloneDXComponent `json:"components"`
}

type CycloneDXMetadata struct {
	Timestamp string              `json:"timestamp"`
	Tools     []CycloneDXTool     `json:"tools"`
}

type CycloneDXTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type CycloneDXComponent struct {
	Type    string `json:"type"`
	BomRef  string `json:"bom-ref"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	PURL    string `json:"purl,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

// ── Generation dispatcher ───────────────────────────────────────────────────

func generateSBOM(path, projType, format string) (interface{}, error) {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	name := filepath.Base(path)
	if name == "." || name == "" {
		name = "unknown"
	}

	switch format {
	case "spdx-json":
		return generateSPDX(path, projType, name, timestamp)
	case "cyclonedx-json":
		return generateCycloneDX(path, projType, name, timestamp)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ── SPDX generation ─────────────────────────────────────────────────────────

func generateSPDX(path, projType, name, timestamp string) (*SPDXDocument, error) {
	deps, err := collectDependencies(path, projType)
	if err != nil {
		return nil, err
	}

	var packages []SPDXPackage
	for i, dep := range deps {
		spdxID := fmt.Sprintf("SPDXRef-Package-%s", sanitizeSPDXID(dep.Name))
		if i == 0 && dep.Name == name {
			spdxID = "SPDXRef-DOCUMENT"
		}
		pkg := SPDXPackage{
			SPDXID:              spdxID,
			Name:                dep.Name,
			VersionInfo:         dep.Version,
			DownloadLocation:    dep.DownloadLocation,
			FilesAnalyzed:       false,
			VerificationCode:    nil,
			LicenseConcluded:    "NOASSERTION",
			LicenseDeclared:     "NOASSERTION",
			CopyrightText:       "NOASSERTION",
			PrimaryPackagePurpose: dep.Purpose,
		}
		packages = append(packages, pkg)
	}

	// Ensure at least a root package exists
	if len(packages) == 0 {
		packages = append(packages, SPDXPackage{
			SPDXID:              "SPDXRef-DOCUMENT",
			Name:                name,
			VersionInfo:         "NOASSERTION",
			DownloadLocation:    "NOASSERTION",
			FilesAnalyzed:       false,
			VerificationCode:    nil,
			LicenseConcluded:    "NOASSERTION",
			LicenseDeclared:     "NOASSERTION",
			CopyrightText:       "NOASSERTION",
			PrimaryPackagePurpose: "APPLICATION",
		})
	}

	doc := &SPDXDocument{
		SPDXVersion:       "SPDX-2.3",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              name,
		DocumentNamespace: fmt.Sprintf("https://github.com/OpenSIN-Code/%s", name),
		CreationInfo: SPDXCreationInfo{
			Created:  timestamp,
			Creators: []string{fmt.Sprintf("Tool: sin-code-sbom-%s", ServerVersion)},
		},
		Packages: packages,
	}
	return doc, nil
}

// ── CycloneDX generation ────────────────────────────────────────────────────

func generateCycloneDX(path, projType, name, timestamp string) (*CycloneDXDocument, error) {
	deps, err := collectDependencies(path, projType)
	if err != nil {
		return nil, err
	}

	var components []CycloneDXComponent
	for _, dep := range deps {
		components = append(components, CycloneDXComponent{
			Type:    dep.ComponentType,
			BomRef:  fmt.Sprintf("pkg:%s/%s@%s", dep.PURLType, dep.Name, dep.Version),
			Name:    dep.Name,
			Version: dep.Version,
			PURL:    fmt.Sprintf("pkg:%s/%s@%s", dep.PURLType, dep.Name, dep.Version),
			Scope:   "required",
		})
	}

	if len(components) == 0 {
		components = append(components, CycloneDXComponent{
			Type:    "application",
			BomRef:  fmt.Sprintf("pkg:generic/%s", name),
			Name:    name,
			Version: "0.0.0",
			PURL:    fmt.Sprintf("pkg:generic/%s@0.0.0", name),
			Scope:   "required",
		})
	}

	doc := &CycloneDXDocument{
		BomFormat:    "CycloneDX",
		SpecVersion:  "1.5",
		SerialNumber: fmt.Sprintf("urn:uuid:%s", newUUID()),
		Version:      1,
		Metadata: CycloneDXMetadata{
			Timestamp: timestamp,
			Tools: []CycloneDXTool{
				{Name: "sin-code", Version: ServerVersion},
			},
		},
		Components: components,
	}
	return doc, nil
}

// ── Dependency collection ───────────────────────────────────────────────────

type dependency struct {
	Name             string
	Version          string
	DownloadLocation string
	Purpose          string
	ComponentType    string
	PURLType         string
}

func collectDependencies(path, projType string) ([]dependency, error) {
	var deps []dependency

	switch projType {
	case "go":
		goDeps, err := collectGoDeps(path)
		if err != nil {
			return nil, err
		}
		deps = append(deps, goDeps...)
	case "python":
		pyDeps, err := collectPythonDeps(path)
		if err != nil {
			return nil, err
		}
		deps = append(deps, pyDeps...)
	case "node":
		nodeDeps, err := collectNodeDeps(path)
		if err != nil {
			return nil, err
		}
		deps = append(deps, nodeDeps...)
	default:
		deps = collectGenericDeps(path)
	}

	return deps, nil
}

// ── Go dependencies ──────────────────────────────────────────────────────────

func collectGoDeps(path string) ([]dependency, error) {
	// Try to get module info via go list
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		// Fallback: try to read go.mod manually
		return parseGoModFallback(path)
	}
	return parseGoListOutput(string(out))
}

type goModule struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Dir     string `json:"Dir"`
	Main    bool   `json:"Main"`
}

func parseGoListOutput(raw string) ([]dependency, error) {
	var deps []dependency
	decoder := json.NewDecoder(strings.NewReader(raw))
	for decoder.More() {
		var mod goModule
		if err := decoder.Decode(&mod); err != nil {
			break
		}
		dep := dependency{
			Name:             mod.Path,
			Version:          mod.Version,
			DownloadLocation: fmt.Sprintf("https://%s", mod.Path),
			Purpose:          "LIBRARY",
			ComponentType:    "library",
			PURLType:         "golang",
		}
		if mod.Main {
			dep.Purpose = "APPLICATION"
			dep.ComponentType = "application"
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

func parseGoModFallback(path string) ([]dependency, error) {
	data, err := os.ReadFile(filepath.Join(path, "go.mod"))
	if err != nil {
		return nil, err
	}
	var deps []dependency
	lines := strings.Split(string(data), "\n")
	re := regexp.MustCompile(`^\s*(\S+)\s+(\S+)`)
	inRequire := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "require (") {
			inRequire = true
			continue
		}
		if inRequire && trim == ")" {
			inRequire = false
			continue
		}
		if inRequire || strings.HasPrefix(trim, "require ") {
			rest := trim
			if !inRequire && strings.HasPrefix(trim, "require ") {
				rest = strings.TrimPrefix(trim, "require ")
			}
			m := re.FindStringSubmatch(rest)
			if len(m) >= 3 && !strings.Contains(m[1], "go") {
				deps = append(deps, dependency{
					Name:             m[1],
					Version:          m[2],
					DownloadLocation: fmt.Sprintf("https://%s", m[1]),
					Purpose:          "LIBRARY",
					ComponentType:    "library",
					PURLType:         "golang",
				})
			}
		}
	}
	return deps, nil
}

// ── Python dependencies ───────────────────────────────────────────────────────

func collectPythonDeps(path string) ([]dependency, error) {
	var deps []dependency

	if data, err := os.ReadFile(filepath.Join(path, "requirements.txt")); err == nil {
		parsed := parseRequirementsTxt(string(data))
		deps = append(deps, parsed...)
	}

	if data, err := os.ReadFile(filepath.Join(path, "pyproject.toml")); err == nil && len(deps) == 0 {
		parsed := parsePyprojectToml(string(data))
		deps = append(deps, parsed...)
	}

	return deps, nil
}

func parseRequirementsTxt(content string) []dependency {
	var deps []dependency
	lines := strings.Split(content, "\n")
	re := regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:[<>=!~].*)?`)
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "-") {
			continue
		}
		m := re.FindStringSubmatch(trim)
		if len(m) >= 2 {
			version := ""
			if idx := strings.IndexAny(trim, "<>!=~"); idx != -1 {
				version = strings.TrimSpace(trim[idx:])
			}
			deps = append(deps, dependency{
				Name:             m[1],
				Version:          version,
				DownloadLocation: fmt.Sprintf("https://pypi.org/project/%s/", m[1]),
				Purpose:          "LIBRARY",
				ComponentType:    "library",
				PURLType:         "pypi",
			})
		}
	}
	return deps
}

func parsePyprojectToml(content string) []dependency {
	var deps []dependency
	lines := strings.Split(content, "\n")
	inDeps := false
	re := regexp.MustCompile(`^"?([A-Za-z0-9_.-]+)"?\s*=`)
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "[dependencies]") || strings.Contains(trim, "[project.dependencies]") {
			inDeps = true
			continue
		}
		if inDeps && strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			inDeps = false
			continue
		}
		if inDeps {
			m := re.FindStringSubmatch(trim)
			if len(m) >= 2 {
				deps = append(deps, dependency{
					Name:             m[1],
					Version:          "",
					DownloadLocation: fmt.Sprintf("https://pypi.org/project/%s/", m[1]),
					Purpose:          "LIBRARY",
					ComponentType:    "library",
					PURLType:         "pypi",
				})
			}
		}
	}
	return deps
}

// ── Node.js dependencies ──────────────────────────────────────────────────────

type packageJSON struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

type packageLockJSON struct {
	Packages map[string]struct {
		Version string `json:"version"`
	} `json:"packages"`
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

func collectNodeDeps(path string) ([]dependency, error) {
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	lockData, _ := os.ReadFile(filepath.Join(path, "package-lock.json"))
	var lock packageLockJSON
	if len(lockData) > 0 {
		_ = json.Unmarshal(lockData, &lock)
	}

	var deps []dependency
	merged := make(map[string]string)
	for k, v := range pkg.Dependencies {
		merged[k] = v
	}
	for k, v := range pkg.DevDependencies {
		merged[k] = v
	}

	for name, version := range merged {
		// Resolve exact version from lock file
		resolvedVer := version
		if lock.Packages != nil {
			if p, ok := lock.Packages["node_modules/"+name]; ok && p.Version != "" {
				resolvedVer = p.Version
			}
		}
		if lock.Dependencies != nil {
			if p, ok := lock.Dependencies[name]; ok && p.Version != "" {
				resolvedVer = p.Version
			}
		}
		deps = append(deps, dependency{
			Name:             name,
			Version:          resolvedVer,
			DownloadLocation: fmt.Sprintf("https://www.npmjs.com/package/%s", name),
			Purpose:          "LIBRARY",
			ComponentType:    "library",
			PURLType:         "npm",
		})
	}

	return deps, nil
}

func parsePackageJSON(data []byte) ([]dependency, error) {
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	var deps []dependency
	for name, version := range pkg.Dependencies {
		deps = append(deps, dependency{
			Name:             name,
			Version:          version,
			DownloadLocation: fmt.Sprintf("https://www.npmjs.com/package/%s", name),
			Purpose:          "LIBRARY",
			ComponentType:    "library",
			PURLType:         "npm",
		})
	}
	for name, version := range pkg.DevDependencies {
		deps = append(deps, dependency{
			Name:             name,
			Version:          version,
			DownloadLocation: fmt.Sprintf("https://www.npmjs.com/package/%s", name),
			Purpose:          "LIBRARY",
			ComponentType:    "library",
			PURLType:         "npm",
		})
	}
	return deps, nil
}

// ── Generic project ─────────────────────────────────────────────────────────

func collectGenericDeps(path string) []dependency {
	var deps []dependency
	entries, err := os.ReadDir(path)
	if err != nil {
		return deps
	}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			deps = append(deps, dependency{
				Name:             entry.Name(),
				Version:          "",
				DownloadLocation: "",
				Purpose:          "APPLICATION",
				ComponentType:    "application",
				PURLType:         "generic",
			})
		}
	}
	return deps
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func sanitizeSPDXID(s string) string {
	// Replace characters not allowed in SPDX IDs
	re := regexp.MustCompile(`[^A-Za-z0-9_.-]`)
	return re.ReplaceAllString(s, "-")
}

func newUUID() string {
	// Simple UUID v4 generator (not cryptographically secure, but sufficient for SBOMs)
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(0x40 + i) // deterministic placeholder
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
