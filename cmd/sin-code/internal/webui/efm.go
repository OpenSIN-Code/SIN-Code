// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when webui is refactored
// Purpose: EFM stack discovery and default database path helpers for the
// web UI server. Detects running Docker/OrbStack compose projects from
// EFM metadata files and resolves default SQLite database locations.
package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type efmStack struct {
	Name        string
	Path        string
	Status      string
	StatusBadge string
	Started     *time.Time
	Expires     *time.Time
	Runtime     string
}

func defaultTodoDB() string {
	if env := os.Getenv("SIN_CODE_TODO_DB"); env != "" {
		return env
	}
	cfg, err := userConfigDirHook()
	if err != nil {
		return "todo.db"
	}
	return filepath.Join(cfg, "sin-code", "todo.db")
}

func defaultNotifDB() string {
	if env := os.Getenv("SIN_CODE_NOTIF_DB"); env != "" {
		return env
	}
	cfg, err := userConfigDirHook()
	if err != nil {
		return "notifications.db"
	}
	return filepath.Join(cfg, "sin-code", "notifications.db")
}

func efmMetaDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state", "sin-code", "efm")
	}
	cfg, err := userConfigDirHook()
	if err != nil {
		return filepath.Join(osTempDirHook(), "sin-code-efm")
	}
	return filepath.Join(cfg, "sin-code", "efm")
}

func efmMetaKey(stackPath string) string {
	h := sha256.Sum256([]byte(stackPath))
	return hex.EncodeToString(h[:]) + ".meta"
}

func detectContainerRuntime() string {
	if goosHook() == "darwin" {
		if _, err := lookPathHook("orb"); err == nil {
			return "orb"
		}
	}
	if _, err := lookPathHook("docker"); err == nil {
		return "docker"
	}
	return ""
}

func discoverEfmStacks() ([]efmStack, string, error) {
	rt := detectContainerRuntime()
	dir := efmMetaDir()
	entries, err := readDirHook(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []efmStack{}, rt, nil
		}
		return nil, rt, err
	}
	var out []efmStack
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta") {
			continue
		}
		raw, err := readFileHook(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var meta map[string]string
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		stackPath := meta["stack"]
		name := strings.TrimSuffix(filepath.Base(stackPath), filepath.Ext(stackPath))
		status := "unknown"
		if rt != "" {
			outBytes, _ := execCommandRunner(rt, "ps", "-a", "--filter", "label=com.docker.compose.project="+name, "--format", "{{.Status}}")
			running := strings.Contains(string(outBytes), "Up")
			if running {
				status = "running"
			} else {
				status = "stopped"
			}
		}
		var started, expires *time.Time
		if t, err := time.Parse(time.RFC3339, meta["started"]); err == nil {
			started = &t
		}
		if t, err := time.Parse(time.RFC3339, meta["expires"]); err == nil {
			expires = &t
		}
		st := efmStack{
			Name:        name,
			Path:        stackPath,
			Status:      status,
			StatusBadge: status,
			Started:     started,
			Expires:     expires,
			Runtime:     meta["runtime"],
		}
		out = append(out, st)
	}
	if out == nil {
		out = []efmStack{}
	}
	return out, rt, nil
}
