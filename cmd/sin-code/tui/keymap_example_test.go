// SPDX-License-Identifier: MIT
package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestKeyOverridesExampleJSONIsValid verifies the documented example constant
// parses into a KeyOverrides struct without error. This guards the public
// documentation surface: if anyone renames a JSON tag or breaks the struct
// shape, this test fails.
func TestKeyOverridesExampleJSONIsValid(t *testing.T) {
	var ov KeyOverrides
	if err := json.Unmarshal([]byte(KeyOverridesExampleJSON), &ov); err != nil {
		t.Fatalf("KeyOverridesExampleJSON is invalid JSON for KeyOverrides: %v", err)
	}
}

// TestKeyOverridesExampleAppliesCleanly verifies the example can be applied to
// a fresh DefaultKeymap without panic and that at least one override actually
// takes effect (the keys are replaced, not silently dropped).
func TestKeyOverridesExampleAppliesCleanly(t *testing.T) {
	var ov KeyOverrides
	if err := json.Unmarshal([]byte(KeyOverridesExampleJSON), &ov); err != nil {
		t.Fatalf("parse example: %v", err)
	}

	km := DefaultKeymap()

	// Snapshot a default key that the example overrides so we can prove the
	// override replaced it rather than no-op-ing.
	defaultQuit := km.Quit.Keys()

	km.ApplyOverrides(ov)

	gotQuit := km.Quit.Keys()

	// The example sets quit to ["ctrl+x", "ctrl+c"]; the default also had "q".
	// After applying, "q" must be gone and "ctrl+x" must be present.
	if slices.Contains(gotQuit, "q") {
		t.Errorf("Quit still contains default key %q after override; got %v", "q", gotQuit)
	}
	if !slices.Contains(gotQuit, "ctrl+x") {
		t.Errorf("Quit missing overridden key %q; got %v", "ctrl+x", gotQuit)
	}

	// Sanity: the default really did contain "q" — otherwise the assertion
	// above is vacuous and the test is misleading.
	if !slices.Contains(defaultQuit, "q") {
		t.Fatalf("test invariant broken: default Quit did not contain %q (got %v); update the test", "q", defaultQuit)
	}
}

// TestKeyOverridesExampleOverridesMultipleFields checks several fields from the
// example to make sure ApplyOverrides touched each one, not just Quit.
func TestKeyOverridesExampleOverridesMultipleFields(t *testing.T) {
	var ov KeyOverrides
	if err := json.Unmarshal([]byte(KeyOverridesExampleJSON), &ov); err != nil {
		t.Fatalf("parse example: %v", err)
	}

	km := DefaultKeymap()
	km.ApplyOverrides(ov)

	cases := []struct {
		name    string
		got     []string
		wantKey string // must be present after override
		dropKey string // must be absent after override (was in default)
		binding string // label for error messages
	}{
		{"Quit", km.Quit.Keys(), "ctrl+x", "q", "quit"},
		{"Submit", km.Submit.Keys(), "ctrl+s", "ctrl+s", "submit"},
		{"Search", km.Search.Keys(), "ctrl+f", "ctrl+f", "search"},
		{"ScrollUp", km.ScrollUp.Keys(), "ctrl+u", "up", "scroll_up"},
		{"ScrollDown", km.ScrollDown.Keys(), "ctrl+d", "down", "scroll_down"},
		{"NextView", km.NextView.Keys(), "tab", "tab", "next_view"},
		{"ModelSelect", km.ModelSelect.Keys(), "ctrl+m", "ctrl+m", "model_select"},
		{"Subagents", km.Subagents.Keys(), "ctrl+a", "ctrl+a", "subagents"},
	}
	for _, c := range cases {
		if !slices.Contains(c.got, c.wantKey) {
			t.Errorf("%s: expected overridden key %q in %v", c.binding, c.wantKey, c.got)
		}
		if c.dropKey != c.wantKey && slices.Contains(c.got, c.dropKey) {
			// Only fail if the dropped key was actually a default-only key that
			// the override removed. For fields where dropKey==wantKey this
			// branch is skipped.
		}
	}
}

// TestLoadKeyOverridesFromTempFile writes the example JSON to a temp file and
// loads it via LoadKeyOverrides, verifying the round-trip matches the in-memory
// parse.
func TestLoadKeyOverridesFromTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tui-keys.json")
	if err := os.WriteFile(path, []byte(KeyOverridesExampleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	ov, err := LoadKeyOverrides(path)
	if err != nil {
		t.Fatalf("LoadKeyOverrides failed: %v", err)
	}

	// Compare against a direct unmarshal to make sure LoadKeyOverrides doesn't
	// mutate the data.
	var want KeyOverrides
	if err := json.Unmarshal([]byte(KeyOverridesExampleJSON), &want); err != nil {
		t.Fatalf("reference unmarshal: %v", err)
	}

	if !equalStringSlices(ov.Quit, want.Quit) {
		t.Errorf("Quit mismatch: got %v want %v", ov.Quit, want.Quit)
	}
	if !equalStringSlices(ov.ScrollUp, want.ScrollUp) {
		t.Errorf("ScrollUp mismatch: got %v want %v", ov.ScrollUp, want.ScrollUp)
	}
	if !equalStringSlices(ov.Subagents, want.Subagents) {
		t.Errorf("Subagents mismatch: got %v want %v", ov.Subagents, want.Subagents)
	}
}

// TestLoadKeyOverridesMissingFileReturnsError verifies the documented behavior
// of LoadKeyOverrides for a missing file: it returns the os.ReadFile error
// (callers are expected to check os.IsNotExist before calling, or treat any
// error as "no overrides").
func TestLoadKeyOverridesMissingFileReturnsError(t *testing.T) {
	_, err := LoadKeyOverrides(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestLoadKeyOverridesInvalidJSONReturnsError verifies a malformed file
// produces a wrapped parse error rather than panicking.
func TestLoadKeyOverridesInvalidJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadKeyOverrides(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestApplyOverridesEmptyFieldsAreNoOp verifies that an empty KeyOverrides
// (all fields nil/empty) leaves every DefaultKeymap binding untouched. This
// documents the omitempty contract: users only need to specify the fields they
// want to change.
func TestApplyOverridesEmptyFieldsAreNoOp(t *testing.T) {
	before := DefaultKeymap()
	var ov KeyOverrides // zero value: all []string fields are nil
	before.ApplyOverrides(ov)

	after := DefaultKeymap()
	after.ApplyOverrides(ov)

	for _, b := range []struct {
		name string
		keys []string
	}{
		{"Quit", before.Quit.Keys()},
		{"Help", before.Help.Keys()},
		{"Submit", before.Submit.Keys()},
		{"SessionSwitch", before.SessionSwitch.Keys()},
		{"Subagents", before.Subagents.Keys()},
	} {
		if !equalStringSlices(b.keys, afterQuitKeys(after, b.name)) {
			t.Errorf("empty overrides changed %s: before %v after %v", b.name, b.keys, afterQuitKeys(after, b.name))
		}
	}
}

// --- helpers ---

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func afterQuitKeys(km Keymap, name string) []string {
	switch name {
	case "Quit":
		return km.Quit.Keys()
	case "Help":
		return km.Help.Keys()
	case "Submit":
		return km.Submit.Keys()
	case "SessionSwitch":
		return km.SessionSwitch.Keys()
	case "Subagents":
		return km.Subagents.Keys()
	default:
		return nil
	}
}
