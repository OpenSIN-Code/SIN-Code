// SPDX-License-Identifier: MIT
// Purpose: best-effort, cross-platform resource limits for the
// autonomous daemon (issue #71, multi-repo daemon support). Provides
// a soft memory cap (runtime/debug.SetMemoryLimit), a CPU cap
// (GOMAXPROCS) and a free-disk floor used to back-pressure leasing.
//
// These are deliberately "soft": the daemon stays portable and never
// hard-kills the process. The memory limit makes the Go GC more
// aggressive as the heap approaches the cap; the disk floor lets the
// worker pool refuse new goals before a repo checkout fills the disk.
package resource

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// Limits is the parsed, validated resource configuration. The zero
// value means "no limits" and Apply is a no-op.
type Limits struct {
	// MaxMemoryBytes is a soft heap limit applied via
	// debug.SetMemoryLimit. 0 = unlimited.
	MaxMemoryBytes int64
	// MaxProcs caps runtime.GOMAXPROCS. 0 = leave as-is.
	MaxProcs int
	// MinDiskBytes is the free-disk floor the worker pool checks
	// before leasing a new goal. 0 = no disk back-pressure.
	MinDiskBytes int64
}

// Apply installs the memory and CPU limits process-wide. The disk
// floor is enforced by callers via DiskFree, not here. Returns the
// previous memory limit so callers can restore it in tests.
func (l Limits) Apply() (prevMemLimit int64) {
	prevMemLimit = debug.SetMemoryLimit(-1) // read current without changing
	if l.MaxMemoryBytes > 0 {
		debug.SetMemoryLimit(l.MaxMemoryBytes)
	}
	if l.MaxProcs > 0 {
		runtime.GOMAXPROCS(l.MaxProcs)
	}
	return prevMemLimit
}

// Describe renders a one-line human summary for daemon startup logs.
func (l Limits) Describe() string {
	var b strings.Builder
	b.WriteString("limits[")
	if l.MaxMemoryBytes > 0 {
		fmt.Fprintf(&b, "mem=%s ", HumanBytes(l.MaxMemoryBytes))
	} else {
		b.WriteString("mem=unlimited ")
	}
	if l.MaxProcs > 0 {
		fmt.Fprintf(&b, "procs=%d ", l.MaxProcs)
	} else {
		fmt.Fprintf(&b, "procs=%d(default) ", runtime.GOMAXPROCS(0))
	}
	if l.MinDiskBytes > 0 {
		fmt.Fprintf(&b, "min-disk=%s", HumanBytes(l.MinDiskBytes))
	} else {
		b.WriteString("min-disk=off")
	}
	b.WriteString("]")
	return b.String()
}

// ParseLimits builds a Limits from the raw CLI flag strings. Empty
// size strings mean "no limit". maxProcs <= 0 means "leave default".
func ParseLimits(maxMemory string, maxProcs int, minDisk string) (Limits, error) {
	var l Limits
	if s := strings.TrimSpace(maxMemory); s != "" {
		b, err := ParseBytes(s)
		if err != nil {
			return Limits{}, fmt.Errorf("max-memory: %w", err)
		}
		l.MaxMemoryBytes = b
	}
	if maxProcs > 0 {
		l.MaxProcs = maxProcs
	}
	if s := strings.TrimSpace(minDisk); s != "" {
		b, err := ParseBytes(s)
		if err != nil {
			return Limits{}, fmt.Errorf("min-disk: %w", err)
		}
		l.MinDiskBytes = b
	}
	return l, nil
}

// ParseBytes parses a human byte size like "512", "64MB", "2GiB".
// Decimal suffixes (KB/MB/GB/TB) use 1000; binary suffixes
// (KiB/MiB/GiB/TiB) use 1024. A bare number is interpreted as bytes.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	upper := strings.ToUpper(s)
	type unit struct {
		suffix string
		mult   int64
	}
	// Order matters: check longer suffixes (KIB) before shorter (KB/B).
	units := []unit{
		{"TIB", 1 << 40}, {"GIB", 1 << 30}, {"MIB", 1 << 20}, {"KIB", 1 << 10},
		{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3},
		{"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(upper, u.suffix) {
			numPart := strings.TrimSpace(upper[:len(upper)-len(u.suffix)])
			if numPart == "" {
				return 0, fmt.Errorf("missing number in %q", s)
			}
			val, err := strconv.ParseFloat(numPart, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number %q: %w", numPart, err)
			}
			if val < 0 {
				return 0, fmt.Errorf("negative size %q", s)
			}
			return int64(val * float64(u.mult)), nil
		}
	}
	// No suffix: plain bytes.
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("negative size %q", s)
	}
	return val, nil
}

// HumanBytes renders a byte count using binary units for readable
// daemon logs (e.g. 2147483648 -> "2.0GiB").
func HumanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGT"[exp])
}
