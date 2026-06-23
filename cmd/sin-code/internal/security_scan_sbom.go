// SPDX-License-Identifier: MIT
// Purpose: `sin-code security scan sbom` — vendored SBOM generator integration.
// Locates the `sin-sbom-go` binary (or builds it from the vendored
// SIN-Code-SBOM-Generator-Go module), collects project dependencies, runs the
// generator, and prints the resulting SPDX 2.3 or CycloneDX 1.5 JSON.
// Docs: security.doc.md
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/security/sca"
)

// sbomCacheDir can be overridden by tests to avoid relying on the user's real cache.
var sbomCacheDir = os.UserCacheDir

// sbomParseGoMod is swappable for tests; defaults to the vendored SCA parser.
var sbomParseGoMod = sca.ParseGoMod

// sbomProjectDetectors maps source filenames to their ecosystem type.
// It is used to pick a dependency parser and to report the SBOM source type.
var sbomProjectDetectors = []struct {
	file    string
	ecotype string
	parser  func(string) ([]sbomDep, error)
}{
	{"go.mod", "go", parseGoModDeps},
	{"package.json", "npm", parsePackageJSONDeps},
	{"requirements.txt", "pypi", parseRequirementsTxtDeps},
}

// sbomDep is a minimal dependency shape understood by the SBOM generator.
type sbomDep struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
	PURL    string `json:"purl,omitempty"`
}

// NewSecurityScanSbomCmd returns the `sin-code security scan sbom` subcommand.
func NewSecurityScanSbomCmd() *cobra.Command {
	var (
		format  string
		noBuild bool
		timeout int
		name    string
	)

	cmd := &cobra.Command{
		Use:   "sbom [path]",
		Short: "Generate an SBOM for the project using the vendored generator",
		Long: `sbom generates a Software Bill of Materials (SBOM) for the project at the given path.

It auto-detects the project manifest (go.mod, package.json, requirements.txt),
runs the vendored SIN-Code-SBOM-Generator-Go, and emits SPDX 2.3 or CycloneDX 1.5 JSON.

The generator binary is located in the following order:
  1. $SIN_SBOM_BIN
  2. A binary named "sin-sbom-go" on PATH
  3. The vendored SIN-Code-SBOM-Generator-Go module (built into the user cache if needed)

Use --no-build to fail fast instead of compiling the vendored generator.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolve scan path: %w", err)
			}

			bin, err := sbomBinLocator(abs, noBuild)
			if err != nil {
				return err
			}

			out, err := runSBOMGenerator(abs, bin, format, name, timeout)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "spdx-json", "Output format: spdx-json, cyclonedx-json")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Do not build the vendored generator if missing")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout in seconds")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Document name for the SBOM (defaults to the project directory name)")

	return cmd
}

// sbomBinLocator is overridable for tests so they can supply a fake generator
// without touching PATH or the filesystem.
var sbomBinLocator = func(scanPath string, noBuild bool) (string, error) {
	return findSBOMGeneratorBinary(scanPath, noBuild)
}

// findSBOMGeneratorBinary resolves the sin-sbom-go binary, building it from the
// vendored module if necessary and allowed.
func findSBOMGeneratorBinary(scanPath string, noBuild bool) (string, error) {
	if env := os.Getenv("SIN_SBOM_BIN"); env != "" {
		if fileExists(env) {
			return env, nil
		}
		return "", fmt.Errorf("SIN_SBOM_BIN points to missing file: %s", env)
	}

	if p, err := exec.LookPath("sin-sbom-go"); err == nil {
		return p, nil
	}

	candidates := []string{scanPath}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if ex, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(ex))
	}

	for _, start := range candidates {
		moduleDir := findSBOMModuleRoot(start)
		if moduleDir == "" {
			continue
		}
		bin := cachedSBOMBinaryPath()
		if bin == "" {
			return "", fmt.Errorf("cannot determine cache directory for SBOM generator")
		}
		if fileExists(bin) {
			return bin, nil
		}
		if noBuild {
			return "", fmt.Errorf("SBOM generator not found and --no-build prevents build")
		}
		return buildSBOMGenerator(moduleDir)
	}

	return "", fmt.Errorf("SBOM generator not found; install sin-sbom-go or set SIN_SBOM_BIN")
}

// findSBOMModuleRoot walks upward from start looking for a directory that
// contains SIN-Code-SBOM-Generator-Go/go.mod.
func findSBOMModuleRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(abs, "SIN-Code-SBOM-Generator-Go")
		if fileExists(filepath.Join(candidate, "go.mod")) {
			return candidate
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return ""
}

// cachedSBOMBinaryPath returns the path where the built generator binary is cached.
func cachedSBOMBinaryPath() string {
	dir, err := sbomCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "sin-code", "sin-sbom-go"+binaryExt())
}

// buildSBOMGenerator compiles the vendored generator into the user cache.
func buildSBOMGenerator(moduleDir string) (string, error) {
	out := cachedSBOMBinaryPath()
	if out == "" {
		return "", fmt.Errorf("cannot determine cache directory")
	}
	cacheDir := filepath.Dir(out)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create SBOM generator cache dir: %w", err)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("go not found in PATH; cannot build SBOM generator")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Remove any stale binary so a failed build is not masked by an old file.
	_ = os.Remove(out)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/sbom")
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build SBOM generator in %s: %w\n%s", moduleDir, err, string(buildOut))
	}
	return out, nil
}

// runSBOMGenerator collects dependencies from path, invokes the generator, and
// returns the produced SBOM bytes for the requested format.
func runSBOMGenerator(path, binary, format, documentName string, timeout int) ([]byte, error) {
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	deps, sourceType, err := collectSBOMDependencies(path)
	if err != nil {
		return nil, fmt.Errorf("collect dependencies: %w", err)
	}

	depsFile, err := writeSBOMDepsFile(path, deps)
	if err != nil {
		return nil, fmt.Errorf("write dependencies file: %w", err)
	}
	defer os.Remove(depsFile)

	if documentName == "" {
		documentName = filepath.Base(path)
	}

	outFile, err := os.CreateTemp("", "sin-sbom-output-*.json")
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	_ = outFile.Close()
	defer os.Remove(outFile.Name())

	var outputFlag string
	switch format {
	case "spdx-json":
		outputFlag = "-output-spdx"
	case "cyclonedx-json":
		outputFlag = "-output-cyclonedx"
	default:
		return nil, fmt.Errorf("unsupported format %q; use spdx-json or cyclonedx-json", format)
	}

	args := []string{
		"-deps", depsFile,
		"-name", documentName,
		outputFlag, outFile.Name(),
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = os.Environ()
	combined, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("SBOM generator failed: %w\n%s", err, string(combined))
	}

	out, err := os.ReadFile(outFile.Name())
	if err != nil {
		return nil, fmt.Errorf("read generated SBOM: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("SBOM generator produced empty output (stderr: %s)", string(combined))
	}
	_ = sourceType // kept for future human-readable output; not used in JSON mode
	return out, nil
}

// collectSBOMDependencies inspects the project manifest and returns a list of
// dependencies plus a source-type hint.
func collectSBOMDependencies(path string) ([]sbomDep, string, error) {
	for _, d := range sbomProjectDetectors {
		if fileExists(filepath.Join(path, d.file)) {
			deps, err := d.parser(path)
			if err != nil {
				return nil, "", fmt.Errorf("parse %s: %w", d.file, err)
			}
			return deps, d.ecotype, nil
		}
	}
	// No recognised manifest: return an empty dependency list. The generator
	// will still produce an SBOM document with zero packages.
	return []sbomDep{}, "generic", nil
}

// parseGoModDeps uses the vendored SCA parser to read go.mod.
func parseGoModDeps(path string) ([]sbomDep, error) {
	pkgs, err := sbomParseGoMod(path)
	if err != nil {
		return nil, err
	}
	deps := make([]sbomDep, 0, len(pkgs))
	for _, p := range pkgs {
		version := p.Version
		if version == "" {
			version = "unknown"
		}
		deps = append(deps, sbomDep{
			Name:    p.Name,
			Version: version,
			Type:    "library",
			PURL:    fmt.Sprintf("pkg:golang/%s@%s", p.Name, version),
		})
	}
	return deps, nil
}

// sbomPackageJSON mirrors the subset of package.json we need to parse.
type sbomPackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// parsePackageJSONDeps reads direct and dev dependencies from package.json.
func parsePackageJSONDeps(path string) ([]sbomDep, error) {
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return nil, err
	}
	var pj sbomPackageJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	deps := make([]sbomDep, 0)
	add := func(name, version string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		cleanVersion := strings.TrimPrefix(version, "^")
		cleanVersion = strings.TrimPrefix(cleanVersion, "~")
		deps = append(deps, sbomDep{
			Name:    name,
			Version: cleanVersion,
			Type:    "library",
			PURL:    fmt.Sprintf("pkg:npm/%s@%s", name, cleanVersion),
		})
	}
	for n, v := range pj.Dependencies {
		add(n, v)
	}
	for n, v := range pj.DevDependencies {
		add(n, v)
	}
	return deps, nil
}

// requirementsLine matches lines like "requests==2.31.0" or "flask>=2.0.0".
var requirementsLine = regexp.MustCompile(`^\s*([A-Za-z0-9_.\-]+)\s*([=<>!~]+)\s*([^\s;#]+)`)

// parseRequirementsTxtDeps parses version-pinned requirements.txt lines.
func parseRequirementsTxtDeps(path string) ([]sbomDep, error) {
	data, err := os.ReadFile(filepath.Join(path, "requirements.txt"))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	deps := make([]sbomDep, 0, len(lines))
	seen := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := requirementsLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		version := m[3]
		if seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, sbomDep{
			Name:    name,
			Version: version,
			Type:    "library",
			PURL:    fmt.Sprintf("pkg:pypi/%s@%s", name, version),
		})
	}
	return deps, nil
}

// writeSBOMDepsFile writes a temporary deps.json file for the SBOM generator.
func writeSBOMDepsFile(path string, deps []sbomDep) (string, error) {
	f, err := os.CreateTemp("", "sin-sbom-deps-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(deps); err != nil {
		return "", err
	}
	return f.Name(), nil
}
