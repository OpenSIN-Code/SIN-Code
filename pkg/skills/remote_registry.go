package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const DefaultRegistryURL = "https://registry.sin-code.dev"

type RemoteRegistry struct {
	baseURL string
	client  *http.Client
}

type SkillMetadata struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Tags        []string `json:"tags"`
	Downloads   int      `json:"downloads"`
	SourceURL   string   `json:"source_url"`
}

func NewRemoteRegistry(baseURL string) *RemoteRegistry {
	if baseURL == "" {
		baseURL = DefaultRegistryURL
	}
	return &RemoteRegistry{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Search queries the central registry.
func (r *RemoteRegistry) Search(query string) ([]SkillMetadata, error) {
	reqURL := fmt.Sprintf("%s/api/skills/search?q=%s", r.baseURL, url.QueryEscape(query))
	resp, err := r.client.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var results []SkillMetadata
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// InstallFromRegistry downloads a skill from the registry and installs it.
func (r *RemoteRegistry) InstallFromRegistry(reg *Registry, skillName, version string) error {
	// First, fetch download URL
	reqURL := fmt.Sprintf("%s/api/skills/%s/versions/%s/download", r.baseURL, skillName, version)
	resp, err := r.client.Get(reqURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("skill %s version %s not found", skillName, version)
	}
	// The response could be a tarball or a git URL. For simplicity, assume a git URL.
	var download struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&download); err != nil {
		return err
	}
	// Install from git URL
	return reg.Install(download.URL)
}
