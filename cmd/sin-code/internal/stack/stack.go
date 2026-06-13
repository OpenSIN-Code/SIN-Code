// SPDX-License-Identifier: MIT
// Purpose: stack orchestrator — unified 3-layer (and growing) SIN-Code
// stack installer, doctor, and pretty-printer. Wires together superpowers
// (methodology), dox (context hierarchy), vane (research / runtime), and
// the sin-code tool surface into a single idempotent Install + a
// read-only Doctor. CGo-free, stdlib-only.
// Docs: stack.doc.md
package stack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dox"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/superpowers"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vane"
)

// Layer identifies a single component in the SIN-Code stack. Layers are
// reported (and can be skipped) independently in Install/Doctor.
type Layer string

const (
	LayerSuperpowers Layer = "superpowers"
	LayerDox         Layer = "dox"
	LayerVane        Layer = "vane"
)

// ── Report types ───────────────────────────────────────────────────────

// Component is one row in a Report. OK=true means the layer is healthy.
// OK=false + Skipped=true means the caller asked to skip it (informational).
// OK=false + Skipped=false means a real failure during Install/Doctor.
// Detail carries a short, human-readable status string (SHA, version,
// count, error, "DOWN", etc.) suitable for Format output.
type Component struct {
	Name    string `json:"name"`
	Layer   string `json:"layer"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
	Skipped bool   `json:"skipped"`
}

// Report is the aggregated result of an Install or Doctor call. AllOK
// is true iff every non-skipped component has OK=true.
type Report struct {
	Components []Component `json:"components"`
	AllOK      bool        `json:"all_ok"`
}

// ── Install options ────────────────────────────────────────────────────

// InstallOptions configures one Install call. The three Skip* flags
// short-circuit the matching layer; empty strings fall back to package
// defaults (superpowers.Home(), dox.AgentsFileName, etc.).
type InstallOptions struct {
	// SkipSuperpowers / SkipDox / SkipVane: when true, the matching layer
	// is recorded as OK=true + Skipped=true and no work is done.
	SkipSuperpowers bool
	SkipDox         bool
	SkipVane        bool
	// AgentsMDPath: destination for dox.InjectRoot and the
	// superpowers overlay block. Defaults to "AGENTS.md" in cwd.
	AgentsMDPath string
	// VaneURL: optional override for the vane BaseURL.
	VaneURL string
	// RepoURL / Branch: optional overrides for superpowers.Install.
	// Empty → superpowers.DefaultRepoURL / DefaultBranch.
	RepoURL string
	Branch  string
	// Timeout caps the entire Install call. 0 → 10 minutes.
	Timeout time.Duration
}

// ── Install ────────────────────────────────────────────────────────────

// Install runs every non-skipped layer in sequence, collecting a Report.
// A failure in one layer is recorded as Component.OK=false and Install
// continues with the next layer — partial installs are preferred to
// hard aborts, and a single down-vane instance should not block the
// superpowers + dox layers.
func Install(opts InstallOptions) Report {
	if opts.AgentsMDPath == "" {
		opts.AgentsMDPath = dox.AgentsFileName
	}
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	rep := Report{Components: make([]Component, 0, 3)}

	// 1) superpowers — Install + RegisterMCP + InjectAGENTS overlay.
	if opts.SkipSuperpowers {
		rep.Components = append(rep.Components, Component{
			Name:    "superpowers.install",
			Layer:   string(LayerSuperpowers),
			OK:      true,
			Skipped: true,
			Detail:  "skipped by request",
		})
	} else {
		rep.Components = append(rep.Components, installSuperpowers(ctx, opts))
	}

	// 2) dox — InjectRoot into AGENTS.md (idempotent).
	if opts.SkipDox {
		rep.Components = append(rep.Components, Component{
			Name:    "dox.inject",
			Layer:   string(LayerDox),
			OK:      true,
			Skipped: true,
			Detail:  "skipped by request",
		})
	} else {
		rep.Components = append(rep.Components, installDox(opts))
	}

	// 3) vane — SaveConfig + RegisterMCP. A down instance is OK;
	//    a missing config is a real failure.
	if opts.SkipVane {
		rep.Components = append(rep.Components, Component{
			Name:    "vane.register",
			Layer:   string(LayerVane),
			OK:      true,
			Skipped: true,
			Detail:  "skipped by request",
		})
	} else {
		rep.Components = append(rep.Components, installVane(opts))
	}

	rep.AllOK = true
	for _, c := range rep.Components {
		if !c.Skipped && !c.OK {
			rep.AllOK = false
			break
		}
	}
	return rep
}

func installSuperpowers(ctx context.Context, opts InstallOptions) Component {
	url := opts.RepoURL
	if url == "" {
		url = superpowers.DefaultRepoURL
	}
	branch := opts.Branch
	if branch == "" {
		branch = superpowers.DefaultBranch
	}
	res, err := superpowers.Install(ctx, url, branch)
	if err != nil {
		return Component{
			Name:   "superpowers.install",
			Layer:  string(LayerSuperpowers),
			OK:     false,
			Detail: trimError(err),
		}
	}
	// RegisterMCP — separate failure domain (file IO vs git). Record
	// both in the same Component: success path uses the resolved SHA.
	if _, rerr := superpowers.RegisterMCP(""); rerr != nil {
		return Component{
			Name:   "superpowers.install",
			Layer:  string(LayerSuperpowers),
			OK:     false,
			Detail: "installed " + shortSHA(res.SHA) + " but RegisterMCP failed: " + trimError(rerr),
		}
	}
	// InjectAGENTS — best-effort. If the agents file does not exist,
	// we do NOT create it implicitly: callers must point AgentsMDPath
	// at a real file. A failure here is recorded but not fatal.
	if opts.AgentsMDPath != "" {
		if _, statErr := os.Stat(opts.AgentsMDPath); statErr == nil {
			prompt := buildSuperpowersPrompt(res)
			if err := superpowers.InjectAGENTS(opts.AgentsMDPath, prompt); err != nil {
				return Component{
					Name:   "superpowers.install",
					Layer:  string(LayerSuperpowers),
					OK:     false,
					Detail: "installed " + shortSHA(res.SHA) + " but InjectAGENTS failed: " + trimError(err),
				}
			}
		}
	}
	return Component{
		Name:   "superpowers.install",
		Layer:  string(LayerSuperpowers),
		OK:     true,
		Detail: fmt.Sprintf("sha=%s branch=%s skills=%d", shortSHA(res.SHA), res.Branch, res.Skills),
	}
}

func installDox(opts InstallOptions) Component {
	// Ensure the parent directory exists so a fresh checkout can
	// receive its first dox block without manual mkdir.
	if dir := filepath.Dir(opts.AgentsMDPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, dox.DefaultDirMode); err != nil {
			return Component{
				Name:   "dox.inject",
				Layer:  string(LayerDox),
				OK:     false,
				Detail: "mkdir agents parent: " + trimError(err),
			}
		}
	}
	body := buildDoxBody()
	if err := dox.InjectRoot(opts.AgentsMDPath, body); err != nil {
		return Component{
			Name:   "dox.inject",
			Layer:  string(LayerDox),
			OK:     false,
			Detail: trimError(err),
		}
	}
	return Component{
		Name:   "dox.inject",
		Layer:  string(LayerDox),
		OK:     true,
		Detail: opts.AgentsMDPath,
	}
}

func installVane(opts InstallOptions) Component {
	// LoadConfig returns (cfg, existsOnDisk, err). On error we fall
	// back to DefaultConfig so a fresh install still produces a
	// usable Config that gets saved on disk.
	cfg, _, err := vane.LoadConfig()
	if err != nil {
		cfg = vane.DefaultConfig()
	}
	if opts.VaneURL != "" {
		cfg.BaseURL = opts.VaneURL
	}
	if err := vane.SaveConfig(cfg); err != nil {
		return Component{
			Name:   "vane.register",
			Layer:  string(LayerVane),
			OK:     false,
			Detail: "save config: " + trimError(err),
		}
	}
	if _, err := vane.RegisterMCP(""); err != nil {
		return Component{
			Name:   "vane.register",
			Layer:  string(LayerVane),
			OK:     false,
			Detail: "register mcp: " + trimError(err),
		}
	}
	return Component{
		Name:   "vane.register",
		Layer:  string(LayerVane),
		OK:     true,
		Detail: fmt.Sprintf("url=%s", cfg.BaseURL),
	}
}

// ── Doctor ─────────────────────────────────────────────────────────────

// Doctor performs a read-only health check of every layer. It NEVER
// modifies on-disk state (no Install, no file writes). A down vane
// instance is NOT a failure: the Component is marked OK=true with a
// "DOWN" detail so the user can tell at a glance that the layer is
// configured but unreachable. Truly broken layers (missing config,
// empty skill tree, etc.) are reported as OK=false.
func Doctor(root string) Report {
	rep := Report{Components: make([]Component, 0, 4)}

	// superpowers: must have a non-empty List and a PinState.
	rep.Components = append(rep.Components, doctorSuperpowers(root))

	// dox: structural Check on the given root.
	rep.Components = append(rep.Components, doctorDox(root))

	// vane config: must exist and parse.
	rep.Components = append(rep.Components, doctorVaneConfig())

	// vane health: instance reachable? Informational.
	rep.Components = append(rep.Components, doctorVaneHealth())

	rep.AllOK = true
	for _, c := range rep.Components {
		if !c.OK {
			rep.AllOK = false
			break
		}
	}
	return rep
}

func doctorSuperpowers(root string) Component {
	infos, err := superpowers.List(root)
	if err != nil {
		return Component{
			Name:   "superpowers",
			Layer:  string(LayerSuperpowers),
			OK:     false,
			Detail: "list: " + trimError(err),
		}
	}
	if len(infos) == 0 {
		return Component{
			Name:   "superpowers",
			Layer:  string(LayerSuperpowers),
			OK:     false,
			Detail: "no skills found under " + superpowers.SkillsDir(),
		}
	}
	pin, perr := superpowers.CurrentPin()
	if perr != nil {
		return Component{
			Name:   "superpowers",
			Layer:  string(LayerSuperpowers),
			OK:     false,
			Detail: "pin read: " + trimError(perr),
		}
	}
	if pin == nil {
		return Component{
			Name:   "superpowers",
			Layer:  string(LayerSuperpowers),
			OK:     false,
			Detail: fmt.Sprintf("skills=%d pin=missing", len(infos)),
		}
	}
	return Component{
		Name:   "superpowers",
		Layer:  string(LayerSuperpowers),
		OK:     true,
		Detail: fmt.Sprintf("skills=%d pin=%s", len(infos), shortSHA(pin.SHA)),
	}
}

func doctorDox(root string) Component {
	if root == "" {
		root = "."
	}
	findings, err := dox.Check(root)
	if err != nil {
		return Component{
			Name:   "dox",
			Layer:  string(LayerDox),
			OK:     false,
			Detail: "check: " + trimError(err),
		}
	}
	errs := 0
	for _, f := range findings {
		if f.Severity == "error" {
			errs++
		}
	}
	if errs > 0 {
		return Component{
			Name:   "dox",
			Layer:  string(LayerDox),
			OK:     false,
			Detail: fmt.Sprintf("%d structural error(s) under %s", errs, root),
		}
	}
	warn := len(findings) - errs
	return Component{
		Name:   "dox",
		Layer:  string(LayerDox),
		OK:     true,
		Detail: fmt.Sprintf("root=%s warnings=%d", root, warn),
	}
}

func doctorVaneConfig() Component {
	cfg, exists, err := vane.LoadConfig()
	if err != nil {
		return Component{
			Name:   "vane.config",
			Layer:  string(LayerVane),
			OK:     false,
			Detail: "load: " + trimError(err),
		}
	}
	if !exists {
		return Component{
			Name:   "vane.config",
			Layer:  string(LayerVane),
			OK:     false,
			Detail: "no config on disk (run Install)",
		}
	}
	if cfg.BaseURL == "" {
		return Component{
			Name:   "vane.config",
			Layer:  string(LayerVane),
			OK:     false,
			Detail: "empty BaseURL in " + vane.Home(),
		}
	}
	return Component{
		Name:   "vane.config",
		Layer:  string(LayerVane),
		OK:     true,
		Detail: "url=" + cfg.BaseURL,
	}
}

func doctorVaneHealth() Component {
	cfg, _, _ := vane.LoadConfig()
	cli := vane.NewClient(cfg)
	if cli == nil {
		// Graceful degradation: a configured-but-down instance is
		// NOT a failure for Doctor. Install/Doctor only fail when
		// the local config is broken, not when the remote is up.
		return Component{
			Name:   "vane.health",
			Layer:  string(LayerVane),
			OK:     true,
			Detail: "DOWN (no client) — informational only",
		}
	}
	// Use a short timeout so Doctor never blocks on a slow vane.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := cli.Healthy(ctx); err != nil {
		return Component{
			Name:   "vane.health",
			Layer:  string(LayerVane),
			OK:     true,
			Detail: "DOWN (" + trimError(err) + ") — informational only",
		}
	}
	return Component{
		Name:   "vane.health",
		Layer:  string(LayerVane),
		OK:     true,
		Detail: "UP",
	}
}

// ── Format ─────────────────────────────────────────────────────────────

// Format renders a Report as a multi-line, human-readable string with
// the same ✓/✗/- markers used by the rest of the SIN-Code CLI. The
// format is stable and machine-greppable so log scrapers can extract
// per-layer status.
func Format(r Report) string {
	var b strings.Builder
	b.WriteString("SIN-Code stack report\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteByte('\n')
	for _, c := range r.Components {
		marker := "✗"
		switch {
		case c.OK && c.Skipped:
			marker = "-"
		case c.OK:
			marker = "✓"
		}
		fmt.Fprintf(&b, "%s %-22s %-12s %s\n", marker, c.Name, "("+c.Layer+")", c.Detail)
	}
	b.WriteString(strings.Repeat("─", 60))
	b.WriteByte('\n')
	if r.AllOK {
		b.WriteString("overall: OK\n")
	} else {
		b.WriteString("overall: DEGRADED\n")
	}
	return b.String()
}

// ── Helpers ────────────────────────────────────────────────────────────

// shortSHA returns the first 12 chars of s, or s itself if shorter.
// Used to keep the Format output on a single terminal line.
func shortSHA(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

// trimError normalizes an error for the Detail field: drops the
// trailing newline and caps at 200 chars to keep Format readable.
func trimError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// buildSuperpowersPrompt is the body that gets injected into the
// superpowers-managed region of AGENTS.md. Kept short: the full skill
// catalog is loaded at runtime via superpowers_use_skill, not at boot.
func buildSuperpowersPrompt(res *superpowers.InstallResult) string {
	if res == nil {
		return "superpowers installed (no result metadata)"
	}
	return fmt.Sprintf(
		"superpowers installed\nrepo: %s\nsha:   %s\nbranch: %s\nskills: %d",
		res.Repo, res.SHA, res.Branch, res.Skills,
	)
}

// buildDoxBody is the dox-managed block for the project root. Mirrors
// the stack's own description: a 3-layer orchestrator over
// superpowers / dox / vane.
func buildDoxBody() string {
	return "stack: sin-code v3.8.0\n" +
		"layers: superpowers (methodology) + dox (context) + vane (research) + sin-code (tools)\n" +
		"managed by: internal/stack\n"
}

// ── Public assertion ──────────────────────────────────────────────────

// ErrNotInstalled is returned by helpers that need to signal "stack
// components exist on disk but no Doctor/Install has been run yet".
// Currently exported for the test suite; the CLI layer can use it
// to print actionable hints.
var ErrNotInstalled = errors.New("stack: not installed")
