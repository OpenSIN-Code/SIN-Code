// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the update subcommand (update_cmd.go).
package internal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func resetUpdateCmdFlags(t *testing.T) {
	t.Helper()
	UpdateCmd.SetArgs([]string{})
	defaults := map[string]string{
		"python-only":    "false",
		"go-only":        "false",
		"skills-only":    "false",
		"check":          "false",
		"dry-run":        "false",
		"force":          "false",
		"rollback":       "false",
		"skip-doctor":    "false",
		"state-root":     "",
		"keep-snapshots": "10",
	}
	for name, val := range defaults {
		if err := UpdateCmd.Flags().Set(name, val); err != nil {
			t.Fatalf("reset flag %s: %v", name, err)
		}
	}
}

func TestUpdateCmd_DryRun(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	if err := UpdateCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Execute(); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
}

func TestUpdateCmd_MutuallyExclusiveFlags(t *testing.T) {
	resetUpdateCmdFlags(t)
	cases := []struct {
		name  string
		flags map[string]string
	}{
		{"python+go", map[string]string{"python-only": "true", "go-only": "true"}},
		{"python+skills", map[string]string{"python-only": "true", "skills-only": "true"}},
		{"go+skills", map[string]string{"go-only": "true", "skills-only": "true"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetUpdateCmdFlags(t)
			for k, v := range tc.flags {
				if err := UpdateCmd.Flags().Set(k, v); err != nil {
					t.Fatalf("set flag %s: %v", k, err)
				}
			}
			if err := UpdateCmd.Execute(); err == nil {
				t.Error("expected error for mutually exclusive flags")
			}
		})
	}
}

func TestUpdateCmd_Rollback_NoSnapshot(t *testing.T) {
	resetUpdateCmdFlags(t)
	if err := UpdateCmd.Flags().Set("rollback", "true"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("state-root", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Execute(); err != nil {
		t.Fatalf("rollback with no snapshot failed: %v", err)
	}
}

func TestUpdateCmd_Rollback_WithSnapshot(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	bm.Now = func() string { return "snap1" }
	dir, err := bm.Create()
	if err != nil {
		t.Fatal(err)
	}
	m := NewManifest("v1.0.0")
	m.Pre = UpdateSnapshot{GoBins: map[string]string{"discover": "v1.0.0-pre"}}
	if err := m.Write(dir); err != nil {
		t.Fatal(err)
	}
	backupFile := filepath.Join(dir, "discover")
	if err := os.WriteFile(backupFile, []byte("fake-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	t.Setenv("SIN_CODE_BIN_DIR", binDir)

	resetUpdateCmdFlags(t)
	if err := UpdateCmd.Flags().Set("rollback", "true"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Execute(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(binDir, "discover"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fake-binary" {
		t.Errorf("restored content = %q", string(data))
	}
}

func TestUpdateCmd_NewBackupManagerError(t *testing.T) {
	resetUpdateCmdFlags(t)
	old := osUserHomeDir
	osUserHomeDir = func() (string, error) { return "", errors.New("no home") }
	defer func() { osUserHomeDir = old }()

	if err := UpdateCmd.Execute(); err == nil {
		t.Fatal("expected error when NewBackupManager fails")
	}
}

func TestUpdateCmd_CreateSnapshotError(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	// Create a file named "updates" so Create() cannot make the directory.
	if err := os.WriteFile(filepath.Join(td, "updates"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Execute(); err == nil {
		t.Fatal("expected error when Create snapshot fails")
	}
}

func TestUpdateCmd_ManifestWriteError(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}

	oldPy := runPythonPhaseFn
	oldGo := runGoPhaseFn
	oldMarshal := jsonMarshalIndent
	defer func() {
		runPythonPhaseFn = oldPy
		runGoPhaseFn = oldGo
		jsonMarshalIndent = oldMarshal
	}()
	runPythonPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "python"}, nil
	}
	runGoPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "go"}, nil
	}
	jsonMarshalIndent = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}

	if err := UpdateCmd.Execute(); err == nil {
		t.Fatal("expected error when manifest write fails")
	}
}

func TestUpdateCmd_PythonPhaseError(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}

	oldPy := runPythonPhaseFn
	defer func() { runPythonPhaseFn = oldPy }()
	runPythonPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return nil, errors.New("python phase failed")
	}

	if err := UpdateCmd.Execute(); err == nil {
		t.Fatal("expected error when python phase fails")
	}
}

func TestUpdateCmd_GoPhaseError(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}

	oldPy := runPythonPhaseFn
	oldGo := runGoPhaseFn
	defer func() {
		runPythonPhaseFn = oldPy
		runGoPhaseFn = oldGo
	}()
	runPythonPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "python"}, nil
	}
	runGoPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return nil, errors.New("go phase failed")
	}

	if err := UpdateCmd.Execute(); err == nil {
		t.Fatal("expected error when go phase fails")
	}
}

func TestUpdateCmd_DoctorError(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}

	oldPy := runPythonPhaseFn
	oldGo := runGoPhaseFn
	oldDr := runDoctorNonFatalFn
	defer func() {
		runPythonPhaseFn = oldPy
		runGoPhaseFn = oldGo
		runDoctorNonFatalFn = oldDr
	}()
	runPythonPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "python"}, nil
	}
	runGoPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "go"}, nil
	}
	runDoctorNonFatalFn = func(ctx context.Context) error { return errors.New("doctor failed") }

	if err := UpdateCmd.Execute(); err != nil {
		t.Fatalf("doctor error should be non-fatal: %v", err)
	}
}

func TestUpdateCmd_PruneError(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}

	oldPy := runPythonPhaseFn
	oldGo := runGoPhaseFn
	oldPrune := pruneSnapshotsFn
	defer func() {
		runPythonPhaseFn = oldPy
		runGoPhaseFn = oldGo
		pruneSnapshotsFn = oldPrune
	}()
	runPythonPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "python"}, nil
	}
	runGoPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "go"}, nil
	}
	pruneSnapshotsFn = func(bm *BackupManager, keep int) error { return errors.New("prune failed") }

	if err := UpdateCmd.Execute(); err != nil {
		t.Fatalf("prune error should be non-fatal: %v", err)
	}
}

func TestUpdateCmd_FullFlow(t *testing.T) {
	resetUpdateCmdFlags(t)
	td := t.TempDir()
	if err := UpdateCmd.Flags().Set("state-root", td); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCmd.Flags().Set("skip-doctor", "true"); err != nil {
		t.Fatal(err)
	}

	oldPy := runPythonPhaseFn
	oldGo := runGoPhaseFn
	oldDr := runDoctorNonFatalFn
	oldPrune := pruneSnapshotsFn
	defer func() {
		runPythonPhaseFn = oldPy
		runGoPhaseFn = oldGo
		runDoctorNonFatalFn = oldDr
		pruneSnapshotsFn = oldPrune
	}()
	runPythonPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "python", Updated: 1}, nil
	}
	runGoPhaseFn = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "go", Updated: 2}, nil
	}
	runDoctorNonFatalFn = func(ctx context.Context) error { return nil }
	pruneSnapshotsFn = func(bm *BackupManager, keep int) error { return nil }

	if err := UpdateCmd.Execute(); err != nil {
		t.Fatalf("full flow failed: %v", err)
	}
}
