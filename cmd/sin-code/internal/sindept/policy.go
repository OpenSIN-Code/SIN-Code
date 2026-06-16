// SPDX-License-Identifier: MIT
// Purpose: configurable policy for the sin-debt marker convention. The
// policy owns three things:
//
//  1. the canonical default reasons (issue/PR triage templates)
//  2. the optional upgrade trigger catalogue ("properties" detected from
//     the marker text)
//  3. the rot-risk thresholds used by `sin-code debt check` so callers
//     can plug a tiny `.sin-code/debt-policy.toml` and override defaults
//
// The shape is intentionally TOML-friendly: every field is a primitive
// (string slice / int / bool). No struct cycles, no nested maps.
// Docs: sindept.doc.md
package sindept

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Policy is the configurable surface for sin-debt. The zero value is
// the conservative default — every field is meaningful on its own, so
// callers can construct a Policy inline (e.g. in tests or CI scripts).
type Policy struct {
	// DefaultReasons is the catalogue of ceiling phrases the lint
	// recommends authors pick from. Authors can deviate from the list;
	// this is only guidance for `debt check --advice`.
	DefaultReasons []string

	// UpgradeTriggers catalogues suggested upgrade trigger phrases. The
	// value is the canonical phrase; the map key is the slug used in
	// `debt stats --by trigger`.
	UpgradeTriggers map[string]string

	// MaxNoUpgrade is the soft rot ceiling: above this count, `debt
	// check` exits 1 (and CI fails). 0 == disabled.
	MaxNoUpgrade int

	// RequireUpgrade forces `debt check` to fail when ANY marker lacks
	// the `upgrade:` clause. The default policy disables this — humans
	// often write provisional markers — but a CI can opt in by setting
	// it true in `.sin-code/debt-policy.toml`.
	RequireUpgrade bool

	// Source is the path the Policy was loaded from. Empty if defaults
	// only; populated when LoadPolicyFile succeeded.
	Source string
}

// DefaultPolicy is the conservative out-of-the-box policy. It is what the
// `debt` subcommand returns when the user has no `.sin-code/debt-policy.toml`.
//
// The DefaultReasons / UpgradeTriggers lists mirror ponytail's tagging
// conventions (delete / stdlib / native / yagni / shrink) so existing
// adopters have a familiar starting point.
func DefaultPolicy() Policy {
	return Policy{
		DefaultReasons: []string{
			"global mutex",
			"O(n²) scan",
			"hand-rolled retry",
			"hand-rolled backoff",
			"in-memory cache",
			"in-process queue",
			"tied loop",
			"manual json encode",
			"manual json decode",
			"disabled keepalive",
			"single-threaded worker",
			"polling",
			"polling-only watcher",
			"synchronous I/O",
			"synchronous fan-out",
			"blocking sleep",
			"rebuild-on-every-call",
			"reload-on-import",
			"polling timer ticks",
			"best-effort retry",
			"best-effort dedup",
		},
		UpgradeTriggers: map[string]string{
			"throughput": "when throughput exceeds threshold",
			"scale":      "when N exceeds threshold",
			"latency":    "when latency exceeds threshold",
			"errors":     "when error rate is non-trivial",
			"main":       "when the upstream API stabilises",
			"stable":     "when the upstream API stabilises",
			"context":    "when context cancellation is required",
			"rswitch":    "switch to <alternative> when threshold breached",
		},
		MaxNoUpgrade:   50,
		RequireUpgrade: false,
	}
}

// LoadPolicyFile overlays the on-disk `path` onto DefaultPolicy(). The
// file is a tiny subset of TOML:
//
//	[sin-debt]
//	max_no_upgrade   = 50
//	require_upgrade  = false
//	default_reasons  = ["global mutex", "O(n²) scan"]
//	[sin-debt.upgrade_triggers]
//	throughput = "when throughput exceeds threshold"
//	main       = "when the upstream API stabilises"
//
// Unknown keys are tolerated (forward-compat). A missing file returns the
// default policy with Source="". A malformed file returns an error,
// because the user explicitly asked for it.
func LoadPolicyFile(path string) (Policy, error) {
	policy := DefaultPolicy()
	if path == "" {
		return policy, nil
	}
	f, err := os.Open(path) // #nosec G304 — loaded by user
	if err != nil {
		if os.IsNotExist(err) {
			return policy, nil
		}
		return policy, fmt.Errorf("sindept: open policy %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	section := ""
	triggers := map[string]string{}
	var sc = bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)

		switch section {
		case "sin-debt":
			switch key {
			case "max_no_upgrade":
				if n, err := parseInt(val); err == nil {
					policy.MaxNoUpgrade = n
				}
			case "require_upgrade":
				policy.RequireUpgrade = parseBool(val)
			case "default_reasons":
				policy.DefaultReasons = parseStringList(val)
			}
		case "sin-debt.upgrade_triggers":
			if key != "" && val != "" {
				triggers[key] = val
			}
		}
	}
	if err := sc.Err(); err != nil {
		return policy, fmt.Errorf("sindept: scan policy %s: %w", path, err)
	}
	if len(triggers) > 0 {
		policy.UpgradeTriggers = triggers
	}
	policy.Source = path
	return policy, nil
}

// LoadPolicyForRoot walks up from `root` looking for `.sin-code/debt-policy.toml`.
// Stops at the first hit or the filesystem root. The returned policy.Source
// is the absolute path that matched.
func LoadPolicyForRoot(root string) (Policy, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return DefaultPolicy(), nil
	}
	dir := abs
	for {
		candidate := filepath.Join(dir, ".sin-code", "debt-policy.toml")
		if _, err := os.Stat(candidate); err == nil {
			return LoadPolicyFile(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return DefaultPolicy(), nil
		}
		dir = parent
	}
}

// CheckResult is what `debt check` prints and exits on.
type CheckResult struct {
	Ok         bool
	Total      int
	Missing    int
	Threshold  int
	Failed     []Marker
	RequireUpg bool
	MissingUpg []Marker
}

// RunCheck implements the `debt check` policy gate. It exits non-zero
// (CheckResult.Ok == false) when either:
//
//  1. The count of markers without an `upgrade:` clause exceeds
//     policy.MaxNoUpgrade (when MaxNoUpgrade > 0), OR
//  2. Policy.RequireUpgrade is true and any marker lacks `upgrade:`.
//
// Failed is populated with the markers that tripped the gate so the
// human can fix them. When MaxNoUpgrade trips, Failed is capped at the
// offending population (so the printed list is bounded).
func (p Policy) RunCheck(markers []Marker) CheckResult {
	res := CheckResult{Threshold: p.MaxNoUpgrade, RequireUpg: p.RequireUpgrade, Total: len(markers)}
	for _, m := range markers {
		if !(m.HasUpg && m.Upgrade != "") {
			res.Missing++
			res.MissingUpg = append(res.MissingUpg, m)
		}
	}
	if p.MaxNoUpgrade > 0 && res.Missing > p.MaxNoUpgrade {
		res.Failed = append(res.Failed, res.MissingUpg...)
		res.Ok = false
		return res
	}
	if p.RequireUpgrade && res.Missing > 0 {
		res.Failed = append(res.Failed, res.MissingUpg...)
		res.Ok = false
		return res
	}
	res.Ok = true
	return res
}

// FormatCheckResult renders the gate verdict as a markdown bullet list.
// Used by `debt check` and by tests as golden output.
func FormatCheckResult(r CheckResult) string {
	var b strings.Builder
	if r.Ok {
		fmt.Fprintf(&b, "- ok: %d markers, %d missing upgrade, threshold=%d\n",
			r.Total, r.Missing, r.Threshold)
		return b.String()
	}
	fmt.Fprintf(&b, "- FAIL: %d markers, %d missing upgrade, threshold=%d\n",
		r.Total, r.Missing, r.Threshold)
	if r.RequireUpg {
		fmt.Fprintln(&b, "- reason: require_upgrade is true and at least one marker has no upgrade clause")
	}
	if r.Threshold > 0 && r.Missing > r.Threshold {
		fmt.Fprintf(&b, "- reason: missing-upgrade count %d exceeds threshold %d\n", r.Missing, r.Threshold)
	}
	for _, m := range r.Failed {
		fmt.Fprintf(&b, "  - %s:%d — %s\n", m.File, m.Line, m.Reason)
	}
	return b.String()
}

// parseInt is a tiny strconv substitute that ignores trailing chars.
func parseInt(s string) (int, error) {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 && s != "0" {
		return 0, fmt.Errorf("not a number: %q", s)
	}
	return n, nil
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// parseStringList accepts `["a", "b"]`, `["a","b"]` and the simpler
// `a, b, c` form. The result is trimmed and dropped if empty.
func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	var out []string
	for _, raw := range strings.Split(s, ",") {
		v := strings.TrimSpace(raw)
		v = strings.Trim(v, `"'`)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
