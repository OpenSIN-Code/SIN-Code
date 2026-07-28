// SPDX-License-Identifier: MIT
// Purpose: execute — safe shell command execution with safety checks, secret
// redaction, timeout handling, and error analysis. Built-in Go implementation.
package internal

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	execCommand string
	execTimeout int
	execFormat  string
	execStream  bool
)

var (
	execIsWindows  = func() bool { return runtime.GOOS == "windows" }
	execNewContext = func(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, timeout)
	}
	execTimeoutDuration = func(timeout int) time.Duration { return time.Duration(timeout) * time.Second }
	execRunCommand      = func(c *exec.Cmd) error { return c.Run() }
)

var ExecuteCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute shell commands safely with secret redaction and timeout",
	Long: `Execute shell commands with safety checks, secret redaction, timeout
handling, and error analysis. Pure Go implementation — no external binary needed.

Example:
  sin-code execute --command "ls -la" --timeout 10 --format json`,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		if execCommand == "" {
			return fmt.Errorf("--command is required")
		}
		if err := checkSafety(execCommand); err != nil {
			return err
		}
		return runCommand(execCommand, execTimeout, execFormat, execStream)
	},
}

type execResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration"`
	Error    string `json:"error,omitempty"`
	Redacted bool   `json:"redacted"`
}

func runCommand(command string, timeout int, format string, stream bool) error {
	start := time.Now()

	// Use shell to execute the command
	var shell, shellArg string
	if execIsWindows() {
		shell, shellArg = "cmd", "/c"
	} else {
		shell, shellArg = "/bin/sh", "-c"
	}

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = execNewContext(ctx, execTimeoutDuration(timeout))
		defer cancel()
	}

	// #nosec G204 -- `execute` is the product's explicit operator-requested
	// shell surface; checkSafety runs before this sink and the process is bounded.
	c := exec.CommandContext(ctx, shell, shellArg, command)
	c.Env = os.Environ()

	var stdout, stderr strings.Builder
	if stream {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	} else {
		c.Stdout = &stdout
		c.Stderr = &stderr
	}

	err := execRunCommand(c)
	duration := time.Since(start)

	// Collect output
	outStr := stdout.String()
	errStr := stderr.String()

	// Redact secrets from output
	redacted := false
	outStr = redactSecrets(outStr)
	errStr = redactSecrets(errStr)
	if outStr != stdout.String() || errStr != stderr.String() {
		redacted = true
	}

	exitCode := 0
	var errorMsg string
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if ctx.Err() == context.DeadlineExceeded {
				errorMsg = fmt.Sprintf("TIMEOUT after %ds: %s", timeout, err)
				exitCode = 124
			} else {
				errorMsg = fmt.Sprintf("EXIT CODE %d: %s", exitCode, analyzeError(exitCode, command))
			}
		} else if ctx.Err() == context.DeadlineExceeded {
			exitCode = 124
			errorMsg = fmt.Sprintf("TIMEOUT after %ds: %s", timeout, err)
		} else {
			exitCode = 1
			errorMsg = err.Error()
		}
	}

	if stream {
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s\n", errorMsg)
		}
		return nil
	}

	result := execResult{
		Command:  command,
		ExitCode: exitCode,
		Stdout:   outStr,
		Stderr:   errStr,
		Duration: duration.String(),
		Redacted: redacted,
	}
	if errorMsg != "" {
		result.Error = errorMsg
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("Command:  %s\n", result.Command)
	fmt.Printf("Duration: %s\n", result.Duration)
	fmt.Printf("Exit:     %d\n", result.ExitCode)
	if result.Stdout != "" {
		fmt.Printf("\n--- stdout ---\n%s\n", result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Printf("\n--- stderr ---\n%s\n", result.Stderr)
	}
	if result.Error != "" {
		fmt.Printf("\nERROR: %s\n", result.Error)
	}
	return nil
}

func checkSafety(command string) error {
	lower := strings.ToLower(command)

	// Dangerous patterns
	dangerous := []string{
		"rm -rf /", "rm -rf /*", "rm -rf ~", "rm -rf $HOME",
		"> /dev/sda", "mkfs.", "dd if=/dev/zero",
		":(){ :|:& };:", "chmod 000 /", "chown -R /",
		"rm -rf /usr", "rm -rf /etc", "rm -rf /var",
		"mv / /dev/null", " shred -", "> /etc/passwd",
		"curl .* | sh", "curl .* | bash", "wget .* | sh", "wget .* | bash",
		"eval $(curl", "eval $(wget", "bash <(curl", "bash <(wget",
	}
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return fmt.Errorf("SAFETY BLOCK: command contains dangerous pattern '%s'", d)
		}
	}

	// Block recursive rm on root or home without explicit confirmation
	if matched, _ := regexp.MatchString(`\brm\s+.*-r.*\s+(/|~|/\.\*|/\*|\$HOME|\$HOME/\.*)`, lower); matched {
		return fmt.Errorf("SAFETY BLOCK: recursive rm on root/home requires explicit confirmation")
	}

	return nil
}

func redactSecrets(text string) string {
	patterns := []struct {
		re      *regexp.Regexp
		replace string
	}{
		{regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*)["']?[a-zA-Z0-9_\-]{16,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(token\s*[:=]\s*)["']?[a-zA-Z0-9_\-]{16,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(password\s*[:=]\s*)["']?[^\s"']{4,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(secret\s*[:=]\s*)["']?[a-zA-Z0-9_\-]{8,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(auth\s*[:=]\s*)["']?[a-zA-Z0-9_\-]{16,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(bearer\s+)[a-zA-Z0-9_\-\.]{16,}`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(aws_access_key_id\s*[:=]\s*)["']?[A-Z0-9]{16,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(aws_secret_access_key\s*[:=]\s*)["']?[A-Za-z0-9/+=]{20,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`(?i)(private[_-]?key\s*[:=]\s*)["']?[^\s"']{20,}["']?`), `${1}[REDACTED]`},
		{regexp.MustCompile(`\b(sk|pk|ghp|gho|pypi|xox[bap])-[A-Za-z0-9_\-]{10,}\b`), `[REDACTED]`},
		{regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]+\b`), `[REDACTED]`},
	}

	for _, p := range patterns {
		text = p.re.ReplaceAllString(text, p.replace)
	}
	return text
}

func analyzeError(exitCode int, command string) string {
	// Common exit codes
	codes := map[int]string{
		1:   "general error",
		2:   "misuse of shell builtins",
		126: "command cannot execute (permission denied or not executable)",
		127: "command not found",
		128: "invalid exit argument",
		130: "command terminated by Ctrl-C",
		137: "command killed (SIGKILL, likely OOM)",
		139: "segmentation fault (SIGSEGV)",
		143: "command terminated (SIGTERM)",
	}
	if msg, ok := codes[exitCode]; ok {
		return msg
	}
	if exitCode > 128 && exitCode < 160 {
		return fmt.Sprintf("terminated by signal %d", exitCode-128)
	}
	return "unknown error"
}

func init() {
	RegisterVersionCmd(ExecuteCmd)
	ExecuteCmd.Flags().StringVarP(&execCommand, "command", "c", "", "Command to execute")
	_ = ExecuteCmd.MarkFlagRequired("command")
	ExecuteCmd.Flags().IntVarP(&execTimeout, "timeout", "t", 60, "Timeout in seconds (0 = no timeout)")
	ExecuteCmd.Flags().StringVarP(&execFormat, "format", "f", "text", "Output format: text|json")
	ExecuteCmd.Flags().BoolVarP(&execStream, "stream", "S", false, "Stream output in real-time")
}

var (
	efmStack   string
	efmAction  string
	efmTTL     int
	efmFormat  string
	efmRuntime string
	// efmGOOS allows tests to override runtime.GOOS for platform-specific branches.
	efmGOOS = runtime.GOOS
	// efmFilepathAbs is a test hook for filepath.Abs in EFM compose helpers.
	efmFilepathAbs = filepath.Abs
)

var EfmCmd = &cobra.Command{
	Use:   "efm",
	Short: "Ephemeral Full-Stack Mocking — spin up disposable test environments",
	Long: `Manage disposable full-stack environments (Docker Compose, ephemeral containers).
Pure Go implementation.

Container runtime:
  On macOS, OrbStack ('orb') is preferred and used automatically when available,
  with 'docker' as the fallback. On Linux, 'docker' is used directly.
  The runtime is fully Docker CLI-compatible, so the same compose commands work.

  Use --runtime to override the auto-detected value:
    --runtime auto    auto-detect (default)
    --runtime orb     force OrbStack
    --runtime docker  force Docker (incl. legacy docker-compose fallback)

Examples:
  sin-code efm --action list
  sin-code efm --action up --stack docker-compose.yml --ttl 3600
  sin-code efm --action down --stack docker-compose.yml
  sin-code efm --action status
  sin-code efm --action list --runtime orb`,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEFM(efmAction, efmStack, efmTTL, efmFormat, efmRuntime)
	},
}

type efmResult struct {
	Action   string       `json:"action"`
	Stack    string       `json:"stack,omitempty"`
	Status   string       `json:"status"`
	Services []efmService `json:"services,omitempty"`
	Error    string       `json:"error,omitempty"`
	Duration string       `json:"duration,omitempty"`
	Runtime  string       `json:"runtime,omitempty"`
}

type efmService struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Ports  []string `json:"ports,omitempty"`
	Image  string   `json:"image,omitempty"`
}

func runEFM(action, stack string, ttl int, format string, runtimeOverride string) error {
	start := time.Now()
	rt := resolveContainerRuntime(runtimeOverride)
	result := efmResult{Action: action, Stack: stack, Runtime: rt}

	switch action {
	case "list":
		services, err := listDockerContainers(rt)
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
		} else {
			result.Status = "ok"
			result.Services = services
		}
	case "up":
		if stack == "" {
			return fmt.Errorf("--stack is required for action 'up'")
		}
		err := dockerComposeUp(stack, ttl, rt)
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
		} else {
			result.Status = "started"
			services, _ := listDockerContainers(rt)
			result.Services = filterServices(services, stack)
		}
	case "down":
		if stack == "" {
			return fmt.Errorf("--stack is required for action 'down'")
		}
		err := dockerComposeDown(stack, rt)
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
		} else {
			result.Status = "stopped"
		}
	case "status":
		if stack == "" {
			services, err := listDockerContainers(rt)
			if err != nil {
				result.Status = "error"
				result.Error = err.Error()
			} else {
				result.Status = "ok"
				result.Services = services
			}
		} else {
			status, err := dockerComposeStatus(stack, rt)
			if err != nil {
				result.Status = "error"
				result.Error = err.Error()
			} else {
				result.Status = status
			}
		}
	default:
		return fmt.Errorf("unknown action: %s (use up|down|list|status)", action)
	}

	result.Duration = time.Since(start).String()

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return outputTextEFM(result)
}

func resolveContainerRuntime(override string) string {
	switch override {
	case "orb":
		return "orb"
	case "docker":
		return "docker"
	case "", "auto":
		return detectContainerRuntime()
	default:
		return detectContainerRuntime()
	}
}

func detectContainerRuntime() string {
	if efmGOOS == "darwin" {
		// Docker is the stable Docker-compatible surface exposed by both
		// Docker Desktop and OrbStack. The `orb` binary is a management CLI
		// and remains available only through an explicit --runtime=orb override.
		if _, err := exec.LookPath("docker"); err == nil {
			return "docker"
		}
		if _, err := exec.LookPath("orb"); err == nil {
			return "orb"
		}
		return "docker"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return "docker"
}

const efmCommandTimeout = 30 * time.Second

// efmDetectRuntime is a test hook for detectContainerRuntime in containerCommand.
var efmDetectRuntime = detectContainerRuntime

func containerCommand(rt string, args ...string) *exec.Cmd {
	bin := rt
	if bin == "" {
		bin = efmDetectRuntime()
	}
	if bin == "" {
		bin = "docker"
	}
	// #nosec G204 -- bin is constrained to the internally detected Docker/Orb
	// runtime and this helper only constructs argv without invoking a shell.
	return exec.Command(bin, args...)
}

func legacyComposeCommand(rt string, args ...string) *exec.Cmd {
	if rt == "orb" || rt == "docker" {
		// #nosec G204 -- rt is an allowlisted literal and args are passed as argv.
		return exec.Command(rt+"-compose", args...)
	}
	// #nosec G204 -- fixed compatibility binary; no shell interpretation.
	return exec.Command("docker-compose", args...)
}

func listDockerContainers(rt string) ([]efmService, error) {
	rt = resolveComposeRuntime(rt)
	cands := composeCandidates(rt)
	var out []byte
	var lastErr error
	var usedRt string
	for _, c := range cands {
		if _, err := exec.LookPath(c); err != nil {
			continue
		}
		var err error
		out, err = runEFMCommandOutput(c, "ps", "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Image}}")
		if err == nil {
			usedRt = c
			break
		}
		lastErr = err
	}
	if usedRt == "" {
		if lastErr != nil {
			return nil, fmt.Errorf("no container runtime responded (tried %v): %w", cands, lastErr)
		}
		return nil, fmt.Errorf("no container runtime binary found (tried %v)", cands)
	}

	var services []efmService
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) >= 2 {
			svc := efmService{
				Name:   parts[0],
				Status: parts[1],
			}
			if len(parts) >= 3 && parts[2] != "" {
				svc.Ports = strings.Split(parts[2], ", ")
			}
			if len(parts) >= 4 {
				svc.Image = parts[3]
			}
			services = append(services, svc)
		}
	}
	return services, nil
}

func composeCandidates(rt string) []string {
	if rt == "" {
		rt = detectContainerRuntime()
	}
	cands := []string{}
	seen := map[string]bool{}
	add := func(b string) {
		if b != "" && !seen[b] {
			seen[b] = true
			cands = append(cands, b)
		}
	}
	if rt == "orb" {
		add("orb")
		add("orb-compose")
		add("docker")
		add("docker-compose")
	} else {
		add("docker")
		add("docker-compose")
		if rt == "docker" {
			add("orb")
			add("orb-compose")
		}
	}
	return cands
}

func isModern(bin string) bool {
	return bin == "docker" || bin == "orb"
}

func resolveComposeRuntime(rt string) string {
	if rt == "" || rt == "auto" {
		return detectContainerRuntime()
	}
	return rt
}

// metadataKey returns a deterministic, path-safe filename for a stack's
// metadata file. Using a hash of the absolute path prevents collisions when
// two stacks share the same basename in different directories.
func metadataKey(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(h[:]) + ".meta"
}

func efmMetadataPath(absPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for EFM metadata: %w", err)
	}
	metadataDir := filepath.Join(home, ".local", "state", "sin-code", "efm")
	// #nosec G703 -- metadataDir contains only fixed path components below the
	// operating system's current-user home directory.
	if err := os.MkdirAll(metadataDir, 0o700); err != nil {
		return "", fmt.Errorf("create EFM metadata directory: %w", err)
	}
	return filepath.Join(metadataDir, metadataKey(absPath)), nil
}

func composeProjectName(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	// Docker Compose project names must be lowercase, start with a letter or
	// number, and contain only [a-z0-9_-]. Prefix with "efm" to satisfy the
	// letter rule and keep names short enough for human-readable container names.
	return "efm" + hex.EncodeToString(h[:])[:12]
}

func dockerComposeUp(stack string, ttl int, rt string) error {
	absPath, err := efmFilepathAbs(stack)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("stack file not found: %w", err)
	}

	rt = resolveComposeRuntime(rt)
	projectName := composeProjectName(absPath)
	if err := runComposeCandidates(rt, []string{"-p", projectName, "-f", absPath, "up", "-d"}, true); err != nil {
		return fmt.Errorf("%s compose up failed: %w", rt, err)
	}

	if ttl > 0 {
		metadataFile, err := efmMetadataPath(absPath)
		if err != nil {
			return err
		}
		meta := map[string]string{
			"stack":   absPath,
			"started": time.Now().Format(time.RFC3339),
			"ttl":     fmt.Sprintf("%d", ttl),
			"expires": time.Now().Add(time.Duration(ttl) * time.Second).Format(time.RFC3339),
			"runtime": rt,
		}
		data, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return fmt.Errorf("encode EFM metadata: %w", err)
		}
		// #nosec G703 -- metadataFile is fixed below the current user's state
		// directory and its basename is a SHA-256 digest, never raw input.
		if err := os.WriteFile(metadataFile, data, 0o600); err != nil {
			return fmt.Errorf("write EFM metadata: %w", err)
		}
	}

	return nil
}

func dockerComposeDown(stack string, rt string) error {
	absPath, err := efmFilepathAbs(stack)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("stack file not found: %w", err)
	}

	rt = resolveComposeRuntime(rt)
	projectName := composeProjectName(absPath)
	if err := runComposeCandidates(rt, []string{"-p", projectName, "-f", absPath, "down"}, true); err != nil {
		return fmt.Errorf("%s compose down failed: %w", rt, err)
	}

	metadataFile, err := efmMetadataPath(absPath)
	if err != nil {
		return err
	}
	// #nosec G703 -- metadataFile is fixed below the current user's state
	// directory and its basename is a SHA-256 digest, never raw input.
	if err := os.Remove(metadataFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove EFM metadata: %w", err)
	}

	return nil
}

func dockerComposeStatus(stack string, rt string) (string, error) {
	absPath, err := efmFilepathAbs(stack)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("stack file not found: %w", err)
	}

	rt = resolveComposeRuntime(rt)
	projectName := composeProjectName(absPath)
	out, err := runComposeCapture(rt, []string{"-p", projectName, "-f", absPath, "ps", "--format", "{{.State}}"})
	if err != nil {
		return "", fmt.Errorf("%s compose ps failed: %w", rt, err)
	}
	return parseComposeStates(string(out)), nil
}

func runComposeCandidates(rt string, args []string, attachStdio bool) error {
	cands := composeCandidates(rt)
	var lastErr error
	for _, c := range cands {
		if _, err := exec.LookPath(c); err != nil {
			continue
		}
		full := append([]string{}, args...)
		if isModern(c) {
			full = append([]string{"compose"}, full...)
		}
		if err := runEFMCommand(c, full, attachStdio); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no container runtime binary found (tried %v)", cands)
}

func runComposeCapture(rt string, args []string) ([]byte, error) {
	cands := composeCandidates(rt)
	var lastErr error
	for _, c := range cands {
		if _, err := exec.LookPath(c); err != nil {
			continue
		}
		full := append([]string{}, args...)
		if isModern(c) {
			full = append([]string{"compose"}, full...)
		}
		out, err := runEFMCommandOutput(c, full...)
		if err != nil {
			lastErr = err
			continue
		}
		return out, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no container runtime binary found (tried %v)", cands)
}

func runEFMCommand(bin string, args []string, attachStdio bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), efmCommandTimeout)
	defer cancel()
	// #nosec G204 -- bin comes only from composeCandidates' fixed allowlist;
	// args are internally generated Docker/Compose argv and never shell text.
	cmd := exec.CommandContext(ctx, bin, args...)
	if attachStdio {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%s timed out after %s: %w", bin, efmCommandTimeout, ctx.Err())
	}
	return err
}

func runEFMCommandOutput(bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), efmCommandTimeout)
	defer cancel()
	// #nosec G204 -- bin comes only from composeCandidates' fixed allowlist;
	// args are internally generated Docker/Compose argv and never shell text.
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("%s timed out after %s: %w", bin, efmCommandTimeout, ctx.Err())
	}
	return out, err
}

func parseComposeStates(raw string) string {
	states := strings.Split(strings.TrimSpace(raw), "\n")
	if len(states) == 0 || (len(states) == 1 && states[0] == "") {
		return "no containers running"
	}
	allRunning := true
	for _, state := range states {
		if !strings.Contains(strings.ToLower(state), "running") {
			allRunning = false
			break
		}
	}
	if allRunning {
		return "all running"
	}
	return "partial"
}

func filterServices(services []efmService, stack string) []efmService {
	absPath, err := efmFilepathAbs(stack)
	if err != nil {
		absPath = stack
	}
	projectName := composeProjectName(absPath)
	var filtered []efmService
	for _, svc := range services {
		if strings.HasPrefix(svc.Name, projectName) {
			filtered = append(filtered, svc)
		}
	}
	return filtered
}

func outputTextEFM(r efmResult) error {
	fmt.Printf("EFM: %s\n", r.Action)
	if r.Runtime != "" {
		fmt.Printf("Runtime: %s\n", r.Runtime)
	}
	if r.Stack != "" {
		fmt.Printf("Stack: %s\n", r.Stack)
	}
	fmt.Printf("Status: %s\n", r.Status)
	if r.Duration != "" {
		fmt.Printf("Duration: %s\n", r.Duration)
	}

	if r.Error != "" {
		fmt.Printf("Error: %s\n", r.Error)
	}

	if len(r.Services) > 0 {
		fmt.Printf("\nServices:\n")
		for _, svc := range r.Services {
			fmt.Printf("  %-20s %s", svc.Name, svc.Status)
			if svc.Image != "" {
				fmt.Printf("  (%s)", svc.Image)
			}
			if len(svc.Ports) > 0 && svc.Ports[0] != "" {
				fmt.Printf("  [%s]", strings.Join(svc.Ports, ", "))
			}
			fmt.Println()
		}
	}
	return nil
}

func init() {
	RegisterVersionCmd(EfmCmd)
	EfmCmd.Flags().StringVarP(&efmAction, "action", "a", "list", "Action: up|down|list|status")
	EfmCmd.Flags().StringVarP(&efmStack, "stack", "s", "", "Stack definition (docker-compose.yml, k8s manifest, etc.)")
	EfmCmd.Flags().IntVarP(&efmTTL, "ttl", "t", 3600, "Time-to-live in seconds (0 = no auto-cleanup)")
	EfmCmd.Flags().StringVarP(&efmFormat, "format", "f", "text", "Output format: text|json")
	EfmCmd.Flags().StringVar(&efmRuntime, "runtime", "auto", "Container runtime: auto|orb|docker (default: auto — OrbStack on macOS, Docker on Linux)")
}
