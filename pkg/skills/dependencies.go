package skills

import (
	"fmt"
	"regexp"
	"strings"
)

// SkillWithDeps extends Skill with dependency tracking
type SkillWithDeps struct {
	*Skill
	Requires []string `json:"requires"` // skill names
}

// ParseDependencies looks for a "## Dependencies" section in SKILL.md.
func (s *Skill) ParseDependencies() []string {
	depSection, ok := s.Sections["Dependencies"]
	if !ok {
		return nil
	}
	re := regexp.MustCompile(`-?\s+([a-z0-9_-]+)`)
	matches := re.FindAllStringSubmatch(depSection, -1)
	deps := []string{}
	for _, m := range matches {
		if len(m) > 1 {
			deps = append(deps, strings.TrimSpace(m[1]))
		}
	}
	return deps
}

// DependencyResolver installs missing dependencies before running a skill.
type DependencyResolver struct {
	registry *Registry
	remote   *RemoteRegistry
}

func NewDependencyResolver(reg *Registry, remote *RemoteRegistry) *DependencyResolver {
	return &DependencyResolver{registry: reg, remote: remote}
}

// EnsureDependencies checks if all required skills are installed, installs them if possible.
func (dr *DependencyResolver) EnsureDependencies(skill *Skill) error {
	deps := skill.ParseDependencies()
	for _, dep := range deps {
		if _, err := dr.registry.Get(dep); err == nil {
			continue // already installed
		}
		// Try to install from remote registry
		fmt.Printf("Missing dependency '%s' for skill '%s'. Attempting auto-install...\n", dep, skill.Name)
		if dr.remote == nil {
			return fmt.Errorf("dependency %s missing and no remote registry configured", dep)
		}
		// Search for latest version
		results, err := dr.remote.Search(dep)
		if err != nil || len(results) == 0 {
			return fmt.Errorf("dependency %s not found in remote registry", dep)
		}
		best := results[0]
		if err := dr.remote.InstallFromRegistry(dr.registry, best.Name, best.Version); err != nil {
			return fmt.Errorf("failed to install dependency %s: %w", dep, err)
		}
		fmt.Printf("✅ Installed dependency: %s v%s\n", best.Name, best.Version)
	}
	return nil
}
