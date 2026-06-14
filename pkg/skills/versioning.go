package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type SkillVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URL     string `json:"url"`  // git repo or tarball
	Hash    string `json:"hash"` // git commit hash
}

type SkillManifest struct {
	Skills []SkillVersion `json:"skills"`
}

// VersionedRegistry extends Registry with version tracking.
type VersionedRegistry struct {
	*Registry
	manifestPath string
	manifest     SkillManifest
}

func NewVersionedRegistry(skillsDir string) (*VersionedRegistry, error) {
	r, err := NewRegistry(skillsDir)
	if err != nil {
		return nil, err
	}
	vr := &VersionedRegistry{
		Registry:     r,
		manifestPath: filepath.Join(skillsDir, ".sin-skill-manifest.json"),
	}
	vr.loadManifest()
	return vr, nil
}

func (vr *VersionedRegistry) loadManifest() {
	data, err := os.ReadFile(vr.manifestPath)
	if err == nil {
		json.Unmarshal(data, &vr.manifest)
	}
	if vr.manifest.Skills == nil {
		vr.manifest.Skills = []SkillVersion{}
	}
}

func (vr *VersionedRegistry) saveManifest() error {
	data, err := json.MarshalIndent(vr.manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(vr.manifestPath, data, 0644)
}

// InstallVersion installs a specific version of a skill from a Git tag.
func (vr *VersionedRegistry) InstallVersion(skillName, version, repoURL string) error {
	// Simulate: clone repo at specific tag into temp dir, then copy SKILL.md
	// For production, use go-git or exec.Command("git", "clone", "--branch", version, repoURL, tempDir)
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("skill_%s_%s", skillName, version))
	defer os.RemoveAll(tempDir)
	// Placeholder: assume we have a function CloneRepo(url, ref, dest)
	// CloneRepo(repoURL, version, tempDir)
	// Then install from tempDir
	return vr.Install(tempDir)
}

// Upgrade checks for newer versions of installed skills.
func (vr *VersionedRegistry) Upgrade(skillName string) error {
	// For each skill, look up its git tags or a central registry
	// This is a placeholder – you'd query GitHub releases or a custom API.
	fmt.Printf("Upgrading skill %s...\n", skillName)
	// Simulate upgrade: remove and reinstall from source
	if err := vr.Remove(skillName); err != nil {
		return err
	}
	// Reinstall from original URL (stored in manifest)
	var originalURL string
	for _, sv := range vr.manifest.Skills {
		if sv.Name == skillName {
			originalURL = sv.URL
			break
		}
	}
	if originalURL == "" {
		return fmt.Errorf("no source URL for %s", skillName)
	}
	return vr.Install(originalURL)
}

// UpgradeAll upgrades all installed skills.
func (vr *VersionedRegistry) UpgradeAll() error {
	skills := vr.List()
	for _, name := range skills {
		if err := vr.Upgrade(name); err != nil {
			fmt.Printf("Warning: upgrade of %s failed: %v\n", name, err)
		}
	}
	return nil
}
