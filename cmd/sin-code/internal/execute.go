// SPDX-License-Identifier: MIT
// Purpose: execute — safe shell command execution with safety checks, secret
// redaction, timeout handling, and error analysis. Built-in Go implementation.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

var ExecuteCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute shell commands safely with secret redaction and timeout",
	Long: `Execute shell commands with safety checks, secret redaction, timeout
handling, and error analysis. Pure Go implementation — no external binary needed.

Example:
  sin-code execute --command "ls -la" --timeout 10 --format json`,
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
	if runtime.GOOS == "windows" {
		shell, shellArg = "cmd", "/c"
	} else {
		shell, shellArg = "/bin/sh", "-c"
	}

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

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

	err := c.Run()
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
	ExecuteCmd.Flags().StringVarP(&execCommand, "command", "c", "", "Command to execute")
	_ = ExecuteCmd.MarkFlagRequired("command")
	ExecuteCmd.Flags().IntVarP(&execTimeout, "timeout", "t", 60, "Timeout in seconds (0 = no timeout)")
	ExecuteCmd.Flags().StringVarP(&execFormat, "format", "f", "text", "Output format: text|json")
	ExecuteCmd.Flags().BoolVarP(&execStream, "stream", "S", false, "Stream output in real-time")
}
