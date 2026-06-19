package filemode

import (
	"os"
	"testing"
)

func TestDefault_Returns0644(t *testing.T) {
	t.Setenv("SIN_CODE_FILE_MODE", "")
	if got := Default(); got != 0o644 {
		t.Fatalf("Default() = 0o%o, want 0o644", got)
	}
}

func TestResolve_ValidOctal(t *testing.T) {
	cases := []struct {
		in   string
		want os.FileMode
	}{
		{"0o600", 0o600},
		{"0600", 0o600},
		{"0o640", 0o640},
		{"0O755", 0o755},
		{" 0o600 ", 0o600},
	}
	for _, tc := range cases {
		got, err := Resolve(tc.in, 0o644)
		if err != nil {
			t.Errorf("Resolve(%q) returned error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Resolve(%q) = 0o%o, want 0o%o", tc.in, got, tc.want)
		}
	}
}

func TestResolve_InvalidOctal(t *testing.T) {
	bad := []string{"foo", "0o8", "0o999", "-1", "0xff"}
	for _, in := range bad {
		if _, err := Resolve(in, 0o644); err == nil {
			t.Errorf("Resolve(%q) expected error, got nil", in)
		}
	}
}

func TestResolve_EmptyFallback(t *testing.T) {
	got, err := Resolve("", 0o644)
	if err != nil {
		t.Fatalf("Resolve(\"\", 0o644) returned error: %v", err)
	}
	if got != 0o644 {
		t.Fatalf("Resolve(\"\", 0o644) = 0o%o, want 0o644", got)
	}
}

func TestResolve_RejectsWorldWritable(t *testing.T) {
	loose := []string{"0o666", "0o664", "0o660", "0o662", "0o646"}
	for _, in := range loose {
		if _, err := Resolve(in, 0o644); err == nil {
			t.Errorf("Resolve(%q) expected world-writable rejection, got nil", in)
		}
	}
}

func TestResolve_KeepsExecutableBit(t *testing.T) {
	// Executable bits are kept on purpose: some write paths (notably
	// atomic install / overlay staging) land in directories where the
	// binary needs to be runnable. The knob tightens write access,
	// never silently strips exec.
	got, err := Resolve("0o755", 0o644)
	if err != nil {
		t.Fatalf("Resolve(\"0o755\") returned error: %v", err)
	}
	if got != 0o755 {
		t.Fatalf("Resolve(\"0o755\") = 0o%o, want 0o755", got)
	}
}
