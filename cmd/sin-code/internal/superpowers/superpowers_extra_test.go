// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for the superpowers package.
// These tests target error branches and corner cases that the main test file
// does not exercise, using the existing package-level test hooks.
// Docs: superpowers.doc.md
package superpowers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setHook swaps a package-level hook for the duration of one test.
func setHook[T any](t *testing.T, hook *T, value T) {
	t.Helper()
	old := *hook
	*hook = value
	t.Cleanup(func() { *hook = old })
}

// makeGitRepo creates a local git repository with one skill for hermetic tests.
func makeGitRepo(t *testing.T) string {
	t.Helper()
	upstream := t.TempDir()
	skillDir := filepath.Join(upstream, "skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: fixture\ndescription: fixture skill\n---\n# Fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{
		{"-C", upstream, "init", "--initial-branch=main", "--quiet"},
		{"-C", upstream, "-c", "user.email=test@test", "-c", "user.name=test", "-c", "commit.gpgsign=false", "add", "."},
		{"-C", upstream, "-c", "user.email=test@test", "-c", "user.name=test", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "init"},
	}
	for _, args := range cmds {
		c := exec.Command("git", args...)
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return upstream
}

// errWriter always returns a configured error.
type errWriter struct {
	err error
}

func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }

// ── Home / Paths ───────────────────────────────────────────────────────

func TestHomeFallbackError(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	setHook(t, &osUserHomeDir, func() (string, error) { return "", errors.New("no home") })
	if got := Home(); got != filepath.Join(".", ".sin-code-home") {
		t.Errorf("Home fallback: got %q", got)
	}
}

// ── Install error paths ──────────────────────────────────────────────────

func TestInstallDefaults(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	setHook(t, &DefaultRepoURL, upstream)
	res, err := Install(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Install with defaults: %v", err)
	}
	if res.Repo != upstream || res.Branch != "main" || res.SHA == "" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestInstallExistingClone(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	ctx := context.Background()
	if _, err := Install(ctx, upstream, "main"); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	res, err := Install(ctx, upstream, "main")
	if err != nil {
		t.Fatalf("second Install on existing clone: %v", err)
	}
	if res.SHA == "" {
		t.Error("expected SHA on existing clone")
	}
}

func TestInstallMkdirParentError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	setHook(t, &osMkdirAll, func(path string, perm os.FileMode) error {
		if path == filepath.Dir(SkillsDir()) {
			return errors.New("mkdir parent")
		}
		return os.MkdirAll(path, perm)
	})
	if _, err := Install(context.Background(), upstream, "main"); err == nil {
		t.Error("expected error when parent mkdir fails")
	}
}

func TestInstallMkdirDstError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	orig := osMkdirAll
	setHook(t, &osMkdirAll, func(path string, perm os.FileMode) error {
		if path == SkillsDir() {
			return errors.New("mkdir dst")
		}
		return orig(path, perm)
	})
	if _, err := Install(context.Background(), upstream, "main"); err == nil {
		t.Error("expected error when dst mkdir fails")
	}
}

func TestInstallCloneError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	setHook(t, &runGitHook, func(ctx context.Context, dir string, args ...string) error {
		if len(args) > 0 && args[0] == "clone" {
			return errors.New("clone failed")
		}
		return runGit(ctx, dir, args...)
	})
	if _, err := Install(context.Background(), upstream, "main"); err == nil {
		t.Error("expected error when clone fails")
	}
}

func TestInstallFetchError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	ctx := context.Background()
	if _, err := Install(ctx, upstream, "main"); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	setHook(t, &runGitHook, func(ctx context.Context, dir string, args ...string) error {
		if len(args) > 0 && args[0] == "fetch" {
			return errors.New("fetch failed")
		}
		return runGit(ctx, dir, args...)
	})
	if _, err := Install(ctx, upstream, "main"); err == nil {
		t.Error("expected error when fetch fails")
	}
}

func TestInstallResetError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	ctx := context.Background()
	if _, err := Install(ctx, upstream, "main"); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	setHook(t, &runGitHook, func(ctx context.Context, dir string, args ...string) error {
		if len(args) > 0 && args[0] == "reset" {
			return errors.New("reset failed")
		}
		return runGit(ctx, dir, args...)
	})
	if _, err := Install(ctx, upstream, "main"); err == nil {
		t.Error("expected error when reset fails")
	}
}

func TestInstallCurrentSHAError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	setHook(t, &currentShaHook, func(context.Context, string) (string, error) { return "", errors.New("sha") })
	if _, err := Install(context.Background(), upstream, "main"); err == nil {
		t.Error("expected error when currentSHA fails")
	}
}

func TestInstallWritePromptError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	setHook(t, &writePromptHook, func([]SkillInfo) (string, error) { return "", errors.New("prompt") })
	if _, err := Install(context.Background(), upstream, "main"); err == nil {
		t.Error("expected error when WritePrompt fails")
	}
}

func TestInstallWritePinError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	setHook(t, &osRenameHook, func(string, string) error { return errors.New("rename") })
	if _, err := Install(context.Background(), upstream, "main"); err == nil {
		t.Error("expected error when WriteJSON rename fails")
	}
}

// ── Pin error paths ──────────────────────────────────────────────────────

func TestPinResetError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	ctx := context.Background()
	if _, err := Install(ctx, upstream, "main"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	sha, _ := currentShaHook(ctx, SkillsDir())
	setHook(t, &runGitHook, func(ctx context.Context, dir string, args ...string) error {
		if len(args) > 0 && args[0] == "reset" {
			return errors.New("reset failed")
		}
		return runGit(ctx, dir, args...)
	})
	if _, err := Pin(ctx, sha); err == nil {
		t.Error("expected error when Pin reset fails")
	}
}

func TestPinWritePinError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	ctx := context.Background()
	if _, err := Install(ctx, upstream, "main"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	sha, _ := currentShaHook(ctx, SkillsDir())
	setHook(t, &osRenameHook, func(string, string) error { return errors.New("rename") })
	if _, err := Pin(ctx, sha); err == nil {
		t.Error("expected error when Pin WriteJSON fails")
	}
}

func TestPinWritePromptError(t *testing.T) {
	upstream := makeGitRepo(t)
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	ctx := context.Background()
	if _, err := Install(ctx, upstream, "main"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	sha, _ := currentShaHook(ctx, SkillsDir())
	setHook(t, &writePromptHook, func([]SkillInfo) (string, error) { return "", errors.New("prompt") })
	if _, err := Pin(ctx, sha); err == nil {
		t.Error("expected error when Pin WritePrompt fails")
	}
}

// ── CurrentPin error paths ───────────────────────────────────────────────

func TestCurrentPinReadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	pinPath := PinFile()
	setHook(t, &osReadFile, func(path string) ([]byte, error) {
		if path == pinPath {
			return nil, os.ErrPermission
		}
		return os.ReadFile(path)
	})
	if _, err := CurrentPin(); err == nil {
		t.Error("expected error when pin file read fails")
	}
}

func TestCurrentPinUnmarshalError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	if err := os.MkdirAll(filepath.Dir(PinFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PinFile(), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CurrentPin(); err == nil {
		t.Error("expected error when pin JSON is invalid")
	}
}

// ── List / Get / Find error paths ────────────────────────────────────────

func TestListWalkError(t *testing.T) {
	setHook(t, &walkDirHook, func(string, fs.WalkDirFunc) error { return errors.New("walk fail") })
	if _, err := List(t.TempDir()); err == nil {
		t.Error("expected List to return walk error")
	}
}

func TestListReadFileError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	skillDir := filepath.Join(SkillsDir(), "x")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(p, []byte("---\nname: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setHook(t, &osReadFile, func(path string) ([]byte, error) {
		if path == p {
			return nil, errors.New("read fail")
		}
		return os.ReadFile(path)
	})
	all, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 skills after read error, got %d", len(all))
	}
}

func TestGetListError(t *testing.T) {
	setHook(t, &walkDirHook, func(string, fs.WalkDirFunc) error { return errors.New("walk fail") })
	if _, err := Get("anything"); err == nil {
		t.Error("expected Get to propagate List error")
	}
}

func TestFindListError(t *testing.T) {
	setHook(t, &walkDirHook, func(string, fs.WalkDirFunc) error { return errors.New("walk fail") })
	if _, err := Find("query", 0); err == nil {
		t.Error("expected Find to propagate List error")
	}
}

// ── WriteJSON error paths ────────────────────────────────────────────────

func TestWriteJSONMarshalError(t *testing.T) {
	setHook(t, &jsonMarshalIndent, func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") })
	if err := WriteJSON(filepath.Join(t.TempDir(), "x.json"), map[string]any{}); err == nil {
		t.Error("expected marshal error")
	}
}

func TestWriteJSONMkdirError(t *testing.T) {
	setHook(t, &osMkdirAll, func(string, os.FileMode) error { return errors.New("mkdir") })
	if err := WriteJSON(filepath.Join(t.TempDir(), "sub", "x.json"), map[string]any{"a": 1}); err == nil {
		t.Error("expected mkdir error")
	}
}

func TestWriteJSONCreateTempError(t *testing.T) {
	setHook(t, &osCreateTemp, func(string, string) (*os.File, error) { return nil, errors.New("createtemp") })
	if err := WriteJSON(filepath.Join(t.TempDir(), "x.json"), map[string]any{"a": 1}); err == nil {
		t.Error("expected createtemp error")
	}
}

func TestWriteJSONCopyError(t *testing.T) {
	setHook(t, &ioCopyHook, func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") })
	if err := WriteJSON(filepath.Join(t.TempDir(), "x.json"), map[string]any{"a": 1}); err == nil {
		t.Error("expected copy error")
	}
}

func TestWriteJSONWriteError(t *testing.T) {
	setHook(t, &fileWriteHook, func(*os.File, []byte) (int, error) { return 0, errors.New("write") })
	if err := WriteJSON(filepath.Join(t.TempDir(), "x.json"), map[string]any{"a": 1}); err == nil {
		t.Error("expected write error")
	}
}

func TestWriteJSONCloseError(t *testing.T) {
	setHook(t, &fileCloseHook, func(*os.File) error { return errors.New("close") })
	if err := WriteJSON(filepath.Join(t.TempDir(), "x.json"), map[string]any{"a": 1}); err == nil {
		t.Error("expected close error")
	}
}

func TestWriteJSONRenameError(t *testing.T) {
	setHook(t, &osRenameHook, func(string, string) error { return errors.New("rename") })
	if err := WriteJSON(filepath.Join(t.TempDir(), "x.json"), map[string]any{"a": 1}); err == nil {
		t.Error("expected rename error")
	}
}

// ── Git helpers ──────────────────────────────────────────────────────────

func TestRunGitError(t *testing.T) {
	if err := runGit(context.Background(), t.TempDir(), "status"); err == nil {
		t.Error("expected runGit to fail outside a repo")
	}
}

func TestCurrentSHAError(t *testing.T) {
	if _, err := currentSHA(context.Background(), t.TempDir()); err == nil {
		t.Error("expected currentSHA to fail outside a repo")
	}
}

func TestCurrentBranchError(t *testing.T) {
	if _, err := currentBranch(context.Background(), t.TempDir()); err == nil {
		t.Error("expected currentBranch to fail outside a repo")
	}
}

// ── InjectAGENTS error paths ─────────────────────────────────────────────

func TestInjectAGENTSReadError(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setHook(t, &osReadFile, func(path string) ([]byte, error) {
		if path == agentsPath {
			return nil, os.ErrPermission
		}
		return os.ReadFile(path)
	})
	if err := InjectAGENTS(agentsPath, "prompt"); err == nil {
		t.Error("expected read error to propagate")
	}
}

func TestInjectAGENTSWriteError(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	setHook(t, &osWriteFile, func(string, []byte, os.FileMode) error { return errors.New("write") })
	if err := InjectAGENTS(agentsPath, "prompt"); err == nil {
		t.Error("expected write error to propagate")
	}
}

// ── Overlay error paths and edge cases ───────────────────────────────────

func TestAppendOverlayReadError(t *testing.T) {
	setHook(t, &overlayReadFile, func(string) ([]byte, error) { return nil, errors.New("read") })
	if AppendOverlay("/any/path/SKILL.md") {
		t.Error("expected false on read error")
	}
}

func TestAppendOverlayWriteError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(p, []byte("# body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setHook(t, &overlayWriteFile, func(string, []byte, os.FileMode) error { return errors.New("write") })
	if AppendOverlay(p) {
		t.Error("expected false on write error")
	}
}

func TestAppendOverlayNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(p, []byte("# body"), 0o644); err != nil {
		t.Fatal(err)
	}
	setHook(t, &commitHintHook, func(string) string { return "abc12345" })
	if !AppendOverlay(p) {
		t.Fatal("expected AppendOverlay to modify file")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, OverlayMarker) {
		t.Error("missing overlay marker")
	}
	if !strings.Contains(body, "abc12345") {
		t.Error("missing injected commit hint")
	}
	// Ensure the newline was inserted between the body and the overlay.
	if !strings.Contains(body, "# body\n\n") {
		t.Error("missing newline separator")
	}
}

func TestCommitHintBranches(t *testing.T) {
	home := t.TempDir()
	// The current implementation walks up three directories from the skill path.
	skillDir := filepath.Join(home, "skills", "superpowers", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(p, []byte("# body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pinDir := filepath.Join(home, "skills")
	pinPath := filepath.Join(pinDir, ".sin-code-pin")

	if got := commitHint(p); got != "" {
		t.Errorf("missing pin: expected empty, got %q", got)
	}

	writePin := func(content string) {
		if err := os.WriteFile(pinPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writePin("{\n  \"sha\": \"0123456789abcdef\"\n}\n")
	if got := commitHint(p); got != "01234567" {
		t.Errorf("sha field: got %q", got)
	}

	writePin("{\n  \"sha\": \"ab\"\n}\n")
	if got := commitHint(p); got != "ab" {
		t.Errorf("short sha: got %q", got)
	}

	writePin("1234567890")
	if got := commitHint(p); got != "12345678" {
		t.Errorf("content fallback: got %q", got)
	}

	writePin("x")
	if got := commitHint(p); got != "" {
		t.Errorf("too short content: expected empty, got %q", got)
	}
}

// ── Frontmatter corner cases ───────────────────────────────────────────

func TestParseFrontmatterNoClosingFence(t *testing.T) {
	fm, ok := ParseFrontmatter("---\nname: x\n")
	if ok {
		t.Errorf("expected no closing fence to fail, got %+v", fm)
	}
}

func TestParseFrontmatterClosingFenceAtEOF(t *testing.T) {
	fm, ok := ParseFrontmatter("---\nname: x\n---")
	if !ok || fm["name"] != "x" {
		t.Errorf("expected closing fence at EOF to parse, got %+v ok=%v", fm, ok)
	}
}

func TestParseFrontmatterBodyEndsWithDash(t *testing.T) {
	fm, ok := ParseFrontmatter("---\nname: x\n-")
	if ok {
		t.Errorf("expected trailing dash without fence to fail, got %+v", fm)
	}
}

func TestParseFrontmatterKeyOnly(t *testing.T) {
	fm, ok := ParseFrontmatter("---\nname:\n---\n")
	if !ok || fm["name"] != "" {
		t.Errorf("expected empty value, got %+v ok=%v", fm, ok)
	}
}

func TestParseFrontmatterNoColonLine(t *testing.T) {
	fm, ok := ParseFrontmatter("---\nname: x\nplain text\n---\n")
	if !ok || fm["name"] != "x" || fm["plain text"] != "" {
		t.Errorf("expected no-colon line to be skipped, got %+v", fm)
	}
}

func TestParseFrontmatterLeadingSpacesKey(t *testing.T) {
	fm, ok := ParseFrontmatter("---\n  name: x\n---\n")
	if !ok || fm["name"] != "x" {
		t.Errorf("expected leading spaces to be stripped, got %+v", fm)
	}
}

func TestParseFrontmatterQuoteToggles(t *testing.T) {
	body := "---\n'a:b': v1\n\"c:d\": v2\n---\n"
	fm, ok := ParseFrontmatter(body)
	if !ok {
		t.Fatalf("expected ok, got false")
	}
	if fm["'a:b'"] != "v1" || fm["\"c:d\""] != "v2" {
		t.Errorf("unexpected map: %+v", fm)
	}
}

func TestParseFrontmatterCollectBlockBreak(t *testing.T) {
	body := "---\ndescription: >-\n  line1\n  line2\nplain: value\n---\n"
	fm, ok := ParseFrontmatter(body)
	if !ok {
		t.Fatalf("expected ok")
	}
	if fm["plain"] != "value" {
		t.Errorf("expected plain value after block break, got %+v", fm)
	}
}

func TestParseFrontmatterFoldedScalarCRLF(t *testing.T) {
	body := "---\ndescription: >-\n  line1\r\n  line2\r\n---\n"
	fm, ok := ParseFrontmatter(body)
	if !ok {
		t.Fatalf("expected ok")
	}
	if strings.Contains(fm["description"], "\n") || strings.Contains(fm["description"], "\r") {
		t.Errorf("folded scalar should not contain line breaks, got %q", fm["description"])
	}
}

func TestLeadingSpaces(t *testing.T) {
	if got := leadingSpaces("   abc"); got != 3 {
		t.Errorf("leadingSpaces: got %d", got)
	}
	if got := leadingSpaces("   \t"); got != 4 {
		t.Errorf("leadingSpaces all-space: got %d", got)
	}
}

func TestTrimUnicode(t *testing.T) {
	if got := trimUnicode("a\tb\nc"); got != "a b c" {
		t.Errorf("trimUnicode: got %q", got)
	}
}

// ── Prompt rendering ─────────────────────────────────────────────────────

func TestRenderPromptNoPin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	body := RenderPrompt(nil)
	if !strings.Contains(body, "not installed") {
		t.Errorf("expected no-pin message, got:\n%s", body)
	}
}

func TestRenderPromptEmptySkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	body := RenderPrompt([]SkillInfo{})
	if !strings.Contains(body, "No skills discovered") {
		t.Errorf("expected empty-skills message, got:\n%s", body)
	}
}

func TestRenderPromptFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	body := RenderPrompt([]SkillInfo{{Name: "", Description: "", Path: "/tmp/my-skill/SKILL.md", Hash: "h"}})
	if !strings.Contains(body, "my-skill") {
		t.Error("expected name fallback from path")
	}
	if !strings.Contains(body, "no description in frontmatter") {
		t.Error("expected description fallback")
	}
}

func TestAGENTSSnippetNoPin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	body := AGENTSSnippet(nil)
	if !strings.Contains(body, "Not installed") {
		t.Errorf("expected no-pin message, got:\n%s", body)
	}
}

func TestAGENTSSnippetFallbacks(t *testing.T) {
	body := AGENTSSnippet([]SkillInfo{{Name: "", Description: "", Path: "/tmp/foo/SKILL.md", Hash: "h"}})
	if !strings.Contains(body, "foo") {
		t.Error("expected name fallback from path")
	}
	if !strings.Contains(body, "(no description)") {
		t.Error("expected description fallback")
	}
}

// ── MCP server ───────────────────────────────────────────────────────────

func TestMCPServerNewServer(t *testing.T) {
	s := NewServer(t.TempDir())
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestMCPServerServeContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewServerWithIO(strings.NewReader(""), &strings.Builder{}, &strings.Builder{}, t.TempDir())
	if err := s.Serve(ctx); err != context.Canceled {
		t.Errorf("expected canceled, got %v", err)
	}
}

func TestMCPServerServeMalformedJSON(t *testing.T) {
	var errOut strings.Builder
	s := NewServerWithIO(strings.NewReader("not json\n"), &strings.Builder{}, &errOut, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := s.Serve(ctx); err == nil {
		t.Fatalf("expected timeout or decode error")
	}
	if !strings.Contains(errOut.String(), "decode error") {
		t.Errorf("expected decode error on stderr, got %q", errOut.String())
	}
}

func TestMCPServerServeEncodeError(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	out := &errWriter{err: errors.New("write fail")}
	s := NewServerWithIO(in, out, &strings.Builder{}, t.TempDir())
	if err := s.Serve(context.Background()); err == nil {
		t.Error("expected encode error")
	}
}

func TestMCPServerHandleNotification(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list"}` + "\n")
	var out strings.Builder
	s := NewServerWithIO(in, &out, &strings.Builder{}, t.TempDir())
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("expected no response for notification, got %q", out.String())
	}
}

func TestMCPServerResultMarshalError(t *testing.T) {
	s := NewServerWithIO(nil, nil, nil, t.TempDir())
	id := json.RawMessage("1")
	resp := s.result(&jsonRPCRequest{ID: &id}, map[string]any{"bad": make(chan int)})
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response")
	}
}

func TestMCPServerCallToolListError(t *testing.T) {
	s := NewServerWithIO(nil, nil, nil, t.TempDir())
	setHook(t, &mcpListFunc, func(string) ([]SkillInfo, error) { return nil, errors.New("list") })
	id := json.RawMessage("1")
	resp := s.callTool(context.Background(), &jsonRPCRequest{ID: &id}, &toolCallParams{Name: "superpowers_list_skills"})
	if resp == nil || resp.Error == nil {
		t.Error("expected error response")
	}
}

func TestMCPServerCallToolFindError(t *testing.T) {
	s := NewServerWithIO(nil, nil, nil, t.TempDir())
	setHook(t, &mcpFindFunc, func(string, int) ([]SkillInfo, error) { return nil, errors.New("find") })
	id := json.RawMessage("1")
	resp := s.callTool(context.Background(), &jsonRPCRequest{ID: &id}, &toolCallParams{Name: "superpowers_find_skill", Arguments: []byte("{}")})
	if resp == nil || resp.Error == nil {
		t.Error("expected error response")
	}
}

func TestMCPServerCallToolGetError(t *testing.T) {
	s := NewServerWithIO(nil, nil, nil, t.TempDir())
	setHook(t, &mcpGetFunc, func(string) (*SkillInfo, error) { return nil, errors.New("get") })
	id := json.RawMessage("1")
	resp := s.callTool(context.Background(), &jsonRPCRequest{ID: &id}, &toolCallParams{Name: "superpowers_use_skill", Arguments: []byte("{}")})
	if resp == nil || resp.Error == nil {
		t.Error("expected error response")
	}
}

func TestMCPServerCallToolReadFileError(t *testing.T) {
	s := NewServerWithIO(nil, nil, nil, t.TempDir())
	setHook(t, &mcpGetFunc, func(string) (*SkillInfo, error) { return &SkillInfo{Name: "x", Path: "/x/SKILL.md"}, nil })
	setHook(t, &mcpReadFile, func(string) ([]byte, error) { return nil, errors.New("read") })
	id := json.RawMessage("1")
	resp := s.callTool(context.Background(), &jsonRPCRequest{ID: &id}, &toolCallParams{Name: "superpowers_use_skill", Arguments: []byte("{}")})
	if resp == nil || resp.Error == nil {
		t.Error("expected error response")
	}
}

func TestMCPServerCallToolUnknownTool(t *testing.T) {
	s := NewServerWithIO(nil, nil, nil, t.TempDir())
	id := json.RawMessage("1")
	resp := s.callTool(context.Background(), &jsonRPCRequest{ID: &id}, &toolCallParams{Name: "superpowers_no_such_tool"})
	if resp == nil || resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected unknown tool error, got %+v", resp)
	}
}

func TestRegisterMCPUpdatesDifferentCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	mcpPath := MCPConfigPath()
	cfg := map[string]any{"mcpServers": map[string]any{"superpowers": map[string]any{"command": "other"}}}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(mcpPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterMCP(mcpPath); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(updated, &got); err != nil {
		t.Fatal(err)
	}
	servers := got["mcpServers"].(map[string]any)
	entry := servers["superpowers"].(map[string]any)
	if entry["command"] != "sin-code" {
		t.Errorf("command not updated: %v", entry["command"])
	}
}

// TestNewServerEmptyCfgDir covers the cfgDir fallback inside NewServer.
func TestNewServerEmptyCfgDir(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	s := NewServer("")
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

// TestMCPServerPing covers the "ping" method dispatch.
func TestMCPServerPing(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"ping"}` + "\n")
	var out strings.Builder
	s := NewServerWithIO(in, &out, &strings.Builder{}, t.TempDir())
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(out.String(), "pong") {
		t.Errorf("expected pong response, got %s", out.String())
	}
}

// TestRegisterMCPEmptyPath covers the empty mcpPath fallback in RegisterMCP.
func TestRegisterMCPEmptyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	path, err := RegisterMCP("")
	if err != nil {
		t.Fatal(err)
	}
	if path != MCPConfigPath() {
		t.Errorf("path: got %q want %q", path, MCPConfigPath())
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("mcp.json not written: %v", err)
	}
}

// TestListNameFallback covers the parent-directory fallback when frontmatter
// has no name.
func TestListNameFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	skillDir := filepath.Join(SkillsDir(), "fallback-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: no name\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Name != "fallback-skill" {
		t.Errorf("unexpected skills: %+v", all)
	}
}

// TestListWalkEntryError covers the walkDir callback error-skip path.
func TestListWalkEntryError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	skillDir := filepath.Join(SkillsDir(), "x")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	setHook(t, &walkDirHook, func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "x", "SKILL.md"), nil, errors.New("entry error"))
	})
	all, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 skills after entry error, got %d", len(all))
	}
}

// TestInjectAGENTSMissingEndMarker covers the branch where the begin marker is
// present but the end marker is not.
func TestInjectAGENTSMissingEndMarker(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("<!-- SIN-Code superpowers:begin -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InjectAGENTS(agentsPath, "prompt"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if strings.Count(body, "SIN-Code superpowers:begin") != 1 {
		t.Errorf("expected exactly one begin marker, got %d", strings.Count(body, "SIN-Code superpowers:begin"))
	}
	if !strings.Contains(body, "SIN-Code superpowers:end") {
		t.Error("expected end marker to be added")
	}
}

// TestInjectAGENTSNoTrailingNewline covers the newline-padding branch when the
// existing file body does not end with a newline.
func TestInjectAGENTSNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InjectAGENTS(agentsPath, "prompt"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "existing\n\n<!-- SIN-Code superpowers:begin -->") {
		t.Errorf("expected newline padding, got:\n%s", string(b))
	}
}
