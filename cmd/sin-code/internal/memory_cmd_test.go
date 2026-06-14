// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the sin-code memory command helpers. (st-cov1)
package internal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"hello world", 8, "hello w…"},
		{"", 5, ""},
		{"a", 1, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

// TestOpenMemoryStore_WithTempDB verifies that openMemoryStore can open a
// temporary bbolt DB. (st-cov1)
func TestOpenMemoryStore_WithTempDB(t *testing.T) {
	old := memDBPath
	memDBPath = filepath.Join(t.TempDir(), "memory.db")
	defer func() { memDBPath = old }()

	store, err := openMemoryStore()
	if err != nil {
		t.Fatalf("openMemoryStore failed: %v", err)
	}
	if store == nil {
		t.Fatal("openMemoryStore returned nil store")
	}
	_ = store.Close()
}

func withMemoryDB(t *testing.T) {
	old := memDBPath
	memDBPath = filepath.Join(t.TempDir(), "memory.db")
	t.Cleanup(func() { memDBPath = old })
}

func captureMemoryCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.RunE(cmd, args)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func TestMemoryCommands(t *testing.T) {
	withMemoryDB(t)

	// add
	out, err := captureMemoryCmd(t, memAddCmd, []string{"hello world"})
	if err != nil {
		t.Fatalf("memAddCmd failed: %v", err)
	}
	if !strings.Contains(out, "Stored") {
		t.Errorf("expected 'Stored' in output, got %q", out)
	}

	// list
	out, err = captureMemoryCmd(t, memListCmd, []string{})
	if err != nil {
		t.Fatalf("memListCmd failed: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected list to contain memory, got %q", out)
	}

	// list empty project filter
	oldProject := memProject
	memProject = "nonexistent"
	out, err = captureMemoryCmd(t, memListCmd, []string{})
	memProject = oldProject
	if err != nil {
		t.Fatalf("memListCmd filter failed: %v", err)
	}
	if !strings.Contains(out, "(no memories)") {
		t.Errorf("expected '(no memories)', got %q", out)
	}

	// search (text, no embedding)
	out, err = captureMemoryCmd(t, memSearchCmd, []string{"hello"})
	if err != nil {
		t.Fatalf("memSearchCmd failed: %v", err)
	}
	if !strings.Contains(out, "Top") {
		t.Errorf("expected search output, got %q", out)
	}

	// stats
	out, err = captureMemoryCmd(t, memStatsCmd, []string{})
	if err != nil {
		t.Fatalf("memStatsCmd failed: %v", err)
	}
	if !strings.Contains(out, "Total:") {
		t.Errorf("expected stats output, got %q", out)
	}

	// forget (soft)
	out, err = captureMemoryCmd(t, memForgetCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatalf("memForgetCmd expected error for nonexistent id, got %q", out)
	}
}

func TestMemoryJSONFormat(t *testing.T) {
	withMemoryDB(t)
	oldFormat := memFormat
	memFormat = "json"
	defer func() { memFormat = oldFormat }()

	if _, err := captureMemoryCmd(t, memAddCmd, []string{"json memory"}); err != nil {
		t.Fatalf("memAddCmd failed: %v", err)
	}

	out, err := captureMemoryCmd(t, memListCmd, []string{})
	if err != nil {
		t.Fatalf("memListCmd json failed: %v", err)
	}
	if !strings.Contains(out, "[") {
		t.Errorf("expected JSON array, got %q", out)
	}
}

func TestMemoryGraphAndLink(t *testing.T) {
	withMemoryDB(t)

	captureMemoryCmd(t, memAddCmd, []string{"node a"})
	captureMemoryCmd(t, memAddCmd, []string{"node b"})

	// We need real IDs from the store to link; linking via placeholder IDs will fail.
	store, err := openMemoryStore()
	if err != nil {
		t.Fatalf("openMemoryStore failed: %v", err)
	}
	items, _ := store.List(memory.ListFilter{})
	_ = store.Close()
	if len(items) < 2 {
		t.Fatal("expected at least 2 memories")
	}
	idA, idB := items[0].ID, items[1].ID

	if _, err := captureMemoryCmd(t, memLinkCmd, []string{idA, idB}); err != nil {
		t.Fatalf("memLinkCmd failed: %v", err)
	}

	out, err := captureMemoryCmd(t, memGraphCmd, []string{idA})
	if err != nil {
		t.Fatalf("memGraphCmd failed: %v", err)
	}
	if !strings.Contains(out, "references") {
		t.Errorf("expected graph to show link, got %q", out)
	}

	if _, err := captureMemoryCmd(t, memUnlinkCmd, []string{idA, idB}); err != nil {
		t.Fatalf("memUnlinkCmd failed: %v", err)
	}
}

func TestMemoryShow(t *testing.T) {
	withMemoryDB(t)

	captureMemoryCmd(t, memAddCmd, []string{"show me"})

	store, err := openMemoryStore()
	if err != nil {
		t.Fatalf("openMemoryStore failed: %v", err)
	}
	items, _ := store.List(memory.ListFilter{})
	_ = store.Close()
	if len(items) == 0 {
		t.Fatal("expected one memory")
	}

	out, err := captureMemoryCmd(t, memShowCmd, []string{items[0].ID})
	if err != nil {
		t.Fatalf("memShowCmd failed: %v", err)
	}
	if !strings.Contains(out, "show me") {
		t.Errorf("expected show output, got %q", out)
	}
}

func TestMemoryPrime(t *testing.T) {
	withMemoryDB(t)

	captureMemoryCmd(t, memAddCmd, []string{"prime candidate"})

	out, err := captureMemoryCmd(t, memPrimeCmd, []string{"prime"})
	if err != nil {
		t.Fatalf("memPrimeCmd failed: %v", err)
	}
	if out == "" {
		t.Error("expected prime output, got empty")
	}
}

func TestMemoryForgetHard(t *testing.T) {
	withMemoryDB(t)

	captureMemoryCmd(t, memAddCmd, []string{"to be deleted"})
	store, _ := openMemoryStore()
	items, _ := store.List(memory.ListFilter{})
	_ = store.Close()
	if len(items) == 0 {
		t.Fatal("expected one memory")
	}

	oldHard := memForget
	memForget = true
	defer func() { memForget = oldHard }()

	out, err := captureMemoryCmd(t, memForgetCmd, []string{items[0].ID})
	if err != nil {
		t.Fatalf("memForgetCmd hard failed: %v", err)
	}
	if !strings.Contains(out, "Hard-deleted") {
		t.Errorf("expected hard delete output, got %q", out)
	}
}

func TestMemoryStats_EmbedderEnabled(t *testing.T) {
	withMemoryDB(t)
	oldKey := os.Getenv("SIN_NIM_API_KEY")
	os.Setenv("SIN_NIM_API_KEY", "fake-key")
	defer func() { os.Setenv("SIN_NIM_API_KEY", oldKey) }()

	out, err := captureMemoryCmd(t, memStatsCmd, []string{})
	if err != nil {
		t.Fatalf("memStatsCmd failed: %v", err)
	}
	if !strings.Contains(out, "Total:") {
		t.Errorf("expected stats output, got %q", out)
	}
}

func TestMemoryUnlink_Nonexistent(t *testing.T) {
	withMemoryDB(t)

	out, err := captureMemoryCmd(t, memUnlinkCmd, []string{"id1", "id2"})
	if err != nil {
		t.Fatalf("memUnlinkCmd failed: %v", err)
	}
	if !strings.Contains(out, "Unlinked") {
		t.Errorf("expected unlink output, got %q", out)
	}
}

func TestMemoryCommands_OpenStoreError(t *testing.T) {
	old := memDBPath
	memDBPath = t.TempDir() // directory, not a bbolt file
	defer func() { memDBPath = old }()

	if _, err := captureMemoryCmd(t, memAddCmd, []string{"x"}); err == nil {
		t.Fatal("expected memAddCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memListCmd, []string{}); err == nil {
		t.Fatal("expected memListCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memShowCmd, []string{"id"}); err == nil {
		t.Fatal("expected memShowCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memSearchCmd, []string{"q"}); err == nil {
		t.Fatal("expected memSearchCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memLinkCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected memLinkCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memUnlinkCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected memUnlinkCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memGraphCmd, []string{"id"}); err == nil {
		t.Fatal("expected memGraphCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memPrimeCmd, []string{"q"}); err == nil {
		t.Fatal("expected memPrimeCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memForgetCmd, []string{"id"}); err == nil {
		t.Fatal("expected memForgetCmd to fail when DB path is a directory")
	}
	if _, err := captureMemoryCmd(t, memStatsCmd, []string{}); err == nil {
		t.Fatal("expected memStatsCmd to fail when DB path is a directory")
	}
}

func TestMemoryShow_NotFound(t *testing.T) {
	withMemoryDB(t)
	if _, err := captureMemoryCmd(t, memShowCmd, []string{"nonexistent-id"}); err == nil {
		t.Fatal("expected memShowCmd to fail for nonexistent id")
	}
}

func TestMemorySearch_NoResults(t *testing.T) {
	withMemoryDB(t)
	out, err := captureMemoryCmd(t, memSearchCmd, []string{"definitely-not-there"})
	if err != nil {
		t.Fatalf("memSearchCmd: %v", err)
	}
	if !strings.Contains(out, "(no results)") {
		t.Errorf("expected no results, got %q", out)
	}
}

func TestMemoryList_JSON(t *testing.T) {
	withMemoryDB(t)
	captureMemoryCmd(t, memAddCmd, []string{"json list"})
	oldFormat := memFormat
	memFormat = "json"
	defer func() { memFormat = oldFormat }()
	out, err := captureMemoryCmd(t, memListCmd, []string{})
	if err != nil {
		t.Fatalf("memListCmd json: %v", err)
	}
	if !strings.Contains(out, "json list") {
		t.Errorf("expected JSON list output, got %q", out)
	}
}

func TestMemoryList_EmptyProjectBlank(t *testing.T) {
	withMemoryDB(t)
	captureMemoryCmd(t, memAddCmd, []string{"no project"})
	out, err := captureMemoryCmd(t, memListCmd, []string{})
	if err != nil {
		t.Fatalf("memListCmd: %v", err)
	}
	if !strings.Contains(out, "-") {
		t.Errorf("expected project placeholder '-', got %q", out)
	}
}

func TestMemoryGraphAndLink_JSON(t *testing.T) {
	withMemoryDB(t)
	captureMemoryCmd(t, memAddCmd, []string{"node a"})
	captureMemoryCmd(t, memAddCmd, []string{"node b"})
	store, _ := openMemoryStore()
	items, _ := store.List(memory.ListFilter{})
	_ = store.Close()
	if len(items) < 2 {
		t.Fatal("expected 2 memories")
	}
	idA, idB := items[0].ID, items[1].ID
	captureMemoryCmd(t, memLinkCmd, []string{idA, idB})
	oldFormat := memFormat
	memFormat = "json"
	defer func() { memFormat = oldFormat }()
	out, err := captureMemoryCmd(t, memGraphCmd, []string{idA})
	if err != nil {
		t.Fatalf("memGraphCmd json: %v", err)
	}
	if !strings.Contains(out, idB) {
		t.Errorf("expected JSON graph output, got %q", out)
	}
}

func TestMemoryStats_JSON(t *testing.T) {
	withMemoryDB(t)
	oldFormat := memFormat
	memFormat = "json"
	defer func() { memFormat = oldFormat }()
	out, err := captureMemoryCmd(t, memStatsCmd, []string{})
	if err != nil {
		t.Fatalf("memStatsCmd json: %v", err)
	}
	if !strings.Contains(out, "total") {
		t.Errorf("expected JSON stats, got %q", out)
	}
}

func TestMemoryPrime_NoResults(t *testing.T) {
	withMemoryDB(t)
	out, err := captureMemoryCmd(t, memPrimeCmd, []string{"definitely-not-there"})
	if err != nil {
		t.Fatalf("memPrimeCmd: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty prime output, got %q", out)
	}
}
