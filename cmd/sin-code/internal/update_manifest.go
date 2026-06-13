// SPDX-License-Identifier: MIT
// Purpose: UpdateManifest — pre/post snapshot of all updateable components.
// Used by BackupManager and Rollback to enable safe revert.
// Docs: update_manifest.doc.md
package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type UpdateManifest struct {
	Timestamp  string         `json:"timestamp"`
	SinCodeVer string         `json:"sin_code_version"`
	GoVersion  string         `json:"go_version"`
	OSArch     string         `json:"os_arch"`
	Pre        UpdateSnapshot `json:"pre"`
	Post       UpdateSnapshot `json:"post,omitempty"`
	Success    bool           `json:"success"`
	Error      string         `json:"error,omitempty"`
}

type UpdateSnapshot struct {
	PipxPackages map[string]string `json:"pipx_packages"`
	GoBins       map[string]string `json:"go_bins"`
	SkillsDirs   map[string]string `json:"skills_dirs"`
}

func SnapshotDir(stateRoot, ts string) string {
	return filepath.Join(stateRoot, "updates", ts)
}

func (m *UpdateManifest) Write(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600)
}

func ReadManifest(dir string) (*UpdateManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m UpdateManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func NewManifest(sinCodeVer string) *UpdateManifest {
	return &UpdateManifest{
		Timestamp:  time.Now().UTC().Format("20060102T150405Z"),
		SinCodeVer: sinCodeVer,
	}
}
