// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loopbuilder is refactored
// Purpose: small helper functions (firstNonEmpty, loadHooks, commandRunner)
// extracted from builder.go to keep each file ≤500 lines.
package loopbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func loadHooks(workspace string) []hooks.Hook {
	var all []hooks.Hook
	paths := []string{}
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.json"))
	}
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.json"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var hs []hooks.Hook
		if err := json.Unmarshal(data, &hs); err != nil {
			fmt.Fprintf(os.Stderr, "warn: skipping invalid hooks file %s: %v\n", p, err)
			continue
		}
		all = append(all, hs...)
	}
	return all
}

func commandRunner(command string, containerRunner autonomy.ContainerRunner, containerImage string) verify.Runner {
	if command == "" {
		return nil
	}
	return func(ctx context.Context, workspace string) (bool, string, error) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		var report string
		var err error
		if containerRunner != nil {
			report, err = containerRunner.RunInContainer(cctx, containerImage, workspace, command)
		} else {
			cmd := exec.CommandContext(cctx, "sh", "-c", command)
			cmd.Dir = workspace
			out, e := cmd.CombinedOutput()
			report = strings.TrimSpace(string(out))
			err = e
		}
		if err != nil {
			return false, report, nil
		}
		return true, report, nil
	}
}
