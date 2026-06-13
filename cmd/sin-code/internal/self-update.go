// SPDX-License-Identifier: MIT
// Purpose: self-update — check for and install new sin-code releases from GitHub.
// Auto-detects platform, downloads the correct binary, and replaces the current one.
// Docs: self-update.doc.md
package internal

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var SelfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Check for and install the latest sin-code release",
	Long: `self-update checks GitHub releases for a newer version of sin-code,
downloads the correct binary for your platform, and installs it.

The current binary is backed up before replacement. If the update fails,
the backup is restored automatically.

Usage:
  sin-code self-update              # Check and install latest stable
  sin-code self-update --version    # Show current version info
  sin-code self-update --dry-run    # Check only, don't install

Supported platforms: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		showVersion, _ := cmd.Flags().GetBool("version")

		if showVersion {
			return printVersionInfo()
		}

		return runSelfUpdate(dryRun)
	},
}

func init() {
	SelfUpdateCmd.Flags().BoolP("dry-run", "d", false, "Check for updates but don't install")
	SelfUpdateCmd.Flags().BoolP("version", "v", false, "Show current version and platform info")
}

// ─── Data structures ───────────────────────────────────────────────────────

type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Published  string `json:"published_at"`
	Body       string `json:"body"`
	Assets     []struct {
		Name string `json:"name"`
		Size int    `json:"size"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Version is set at build time via -ldflags "-X main.Version=..."
// We access it through the main package variable (unfortunately requires main.Version to be exported or we use a function).
// Since we can't import main package, we'll use a placeholder that gets overridden.
var currentVersion = "dev"

var githubAPIURL = "https://api.github.com/repos/OpenSIN-Code/SIN-Code/releases/latest"

func SetCurrentVersion(v string) {
	currentVersion = v
}

func printVersionInfo() error {
	fmt.Printf("sin-code version: %s\n", currentVersion)
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Binary path: %s\n", os.Args[0])

	// Check for newer version
	latest, err := fetchLatestRelease()
	if err != nil {
		fmt.Printf("\n⚠️  Could not check for updates: %v\n", err)
		return nil
	}

	fmt.Printf("Latest release: %s (published %s)\n", latest.TagName, formatDate(latest.Published))
	if latest.TagName != currentVersion {
		fmt.Printf("\n🔄 Update available: %s → %s\n", currentVersion, latest.TagName)
		fmt.Println("   Run 'sin-code self-update' to install.")
	} else {
		fmt.Printf("\n✅ You are running the latest version.\n")
	}
	return nil
}

func runSelfUpdate(dryRun bool) error {
	fmt.Printf("🔍 Checking for updates...\n")
	fmt.Printf("   Current version: %s\n", currentVersion)
	fmt.Printf("   Platform: %s/%s\n\n", runtime.GOOS, runtime.GOARCH)

	latest, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	fmt.Printf("📦 Latest release: %s (published %s)\n", latest.TagName, formatDate(latest.Published))

	if latest.TagName == currentVersion {
		fmt.Printf("\n✅ You are already running the latest version.\n")
		return nil
	}

	fmt.Printf("\n🔄 Update available: %s → %s\n", currentVersion, latest.TagName)

	if dryRun {
		fmt.Println("\n⏹️  Dry run mode — not installing. Run without --dry-run to update.")
		return nil
	}

	// Find the correct asset for this platform.
	assetName := fmt.Sprintf("sin-code-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName = fmt.Sprintf("sin-code-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	}

	var assetURL string
	for _, asset := range latest.Assets {
		if asset.Name == assetName {
			assetURL = asset.URL
			break
		}
	}

	if assetURL == "" {
		return fmt.Errorf("no binary found for %s/%s (looked for %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

	fmt.Printf("   Downloading: %s\n", assetName)

	// Download the archive.
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine current binary path: %w", err)
	}

	backupPath := binaryPath + ".backup"
	tmpDir := os.TempDir()
	archivePath := filepath.Join(tmpDir, assetName)

	if err := downloadFile(assetURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(archivePath)

	fmt.Printf("   Extracting binary...\n")

	// Extract binary from archive.
	extractedBinary, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}
	defer os.Remove(extractedBinary)

	// Backup current binary.
	if err := os.Rename(binaryPath, backupPath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Install new binary.
	if err := os.Rename(extractedBinary, binaryPath); err != nil {
		// Restore backup on failure.
		os.Rename(backupPath, binaryPath)
		return fmt.Errorf("install failed: %w", err)
	}

	// Make executable (on Unix).
	if runtime.GOOS != "windows" {
		os.Chmod(binaryPath, 0755)
	}

	// Remove backup on success.
	os.Remove(backupPath)

	fmt.Printf("\n✅ Updated to %s successfully!\n", latest.TagName)
	fmt.Printf("   Run 'sin-code --version' to verify.\n")
	return nil
}

// ─── GitHub API helpers ──────────────────────────────────────────────────

func fetchLatestRelease() (*GitHubRelease, error) {
	url := githubAPIURL
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadFile(url, path string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractBinary(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	}
	return extractTarGz(archivePath, destDir)
}

func extractTarGz(archivePath, destDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag == tar.TypeReg && strings.HasPrefix(header.Name, "sin-code") {
			path := filepath.Join(destDir, header.Name)
			out, err := os.Create(path)
			if err != nil {
				return "", err
			}
			defer out.Close()
			if _, err := io.Copy(out, tr); err != nil {
				return "", err
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("no sin-code binary found in archive")
}

func extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "sin-code") {
			path := filepath.Join(destDir, f.Name)
			out, err := os.Create(path)
			if err != nil {
				return "", err
			}
			defer out.Close()

			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			if _, err := io.Copy(out, rc); err != nil {
				return "", err
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("no sin-code binary found in archive")
}

// CheckUpdateAvailable queries GitHub for the latest release and reports whether
// the current binary is outdated.
func CheckUpdateAvailable() (string, bool, error) {
	latest, err := fetchLatestRelease()
	if err != nil {
		return "", false, err
	}
	if latest.TagName != currentVersion {
		return latest.TagName, true, nil
	}
	return latest.TagName, false, nil
}

func formatDate(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Format("2006-01-02")
}
