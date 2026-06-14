// SPDX-License-Identifier: MIT
package resource

import (
	"os"
	"runtime/debug"
	"testing"
)

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"0", 0, false},
		{"512", 512, false},
		{"512B", 512, false},
		{"1KB", 1000, false},
		{"1KiB", 1024, false},
		{"2MB", 2_000_000, false},
		{"2MiB", 2 * 1024 * 1024, false},
		{"1GB", 1_000_000_000, false},
		{"2GiB", 2 * 1024 * 1024 * 1024, false},
		{"1TiB", 1 << 40, false},
		{"1.5GiB", int64(1.5 * float64(1<<30)), false},
		{"64m", 64 * 1024 * 1024, false}, // bare M = MiB
		{"", 0, true},
		{"abc", 0, true},
		{"-5MB", 0, true},
		{"MB", 0, true},
	}
	for _, c := range cases {
		got, err := ParseBytes(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseBytes(%q): expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseBytes(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseLimits(t *testing.T) {
	l, err := ParseLimits("256MiB", 2, "1GiB")
	if err != nil {
		t.Fatalf("ParseLimits: %v", err)
	}
	if l.MaxMemoryBytes != 256*1024*1024 {
		t.Errorf("MaxMemoryBytes = %d", l.MaxMemoryBytes)
	}
	if l.MaxProcs != 2 {
		t.Errorf("MaxProcs = %d", l.MaxProcs)
	}
	if l.MinDiskBytes != 1<<30 {
		t.Errorf("MinDiskBytes = %d", l.MinDiskBytes)
	}

	// Empty strings -> no limits.
	z, err := ParseLimits("", 0, "")
	if err != nil {
		t.Fatalf("ParseLimits empty: %v", err)
	}
	if z != (Limits{}) {
		t.Errorf("expected zero Limits, got %+v", z)
	}

	// Bad size surfaces an error.
	if _, err := ParseLimits("notasize", 0, ""); err == nil {
		t.Error("expected error for bad max-memory")
	}
}

func TestLimitsApplyMemory(t *testing.T) {
	orig := debug.SetMemoryLimit(-1)
	defer debug.SetMemoryLimit(orig)

	l := Limits{MaxMemoryBytes: 512 * 1024 * 1024}
	l.Apply()
	if got := debug.SetMemoryLimit(-1); got != l.MaxMemoryBytes {
		t.Errorf("memory limit = %d, want %d", got, l.MaxMemoryBytes)
	}
}

func TestDescribe(t *testing.T) {
	l := Limits{}
	if got := l.Describe(); got == "" {
		t.Error("Describe returned empty for zero Limits")
	}
	l2 := Limits{MaxMemoryBytes: 1 << 30, MaxProcs: 4, MinDiskBytes: 1 << 30}
	got := l2.Describe()
	if got == "" {
		t.Error("Describe returned empty")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:                    "512B",
		1024:                   "1.0KiB",
		2 * 1024 * 1024:        "2.0MiB",
		3 * 1024 * 1024 * 1024: "3.0GiB",
	}
	for in, want := range cases {
		if got := HumanBytes(in); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDiskFreeOnTempDir(t *testing.T) {
	// DiskFree may be unavailable (non-unix); only assert when ok.
	free, ok := DiskFree(os.TempDir())
	if ok && free <= 0 {
		t.Errorf("DiskFree returned ok with non-positive free=%d", free)
	}
}
