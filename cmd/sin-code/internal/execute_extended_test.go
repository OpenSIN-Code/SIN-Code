package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCheckSafety_DangerousRmRfRoot(t *testing.T) {
	err := checkSafety("rm -rf /")
	if err == nil {
		t.Error("expected safety block for 'rm -rf /'")
	}
	if !strings.Contains(err.Error(), "SAFETY BLOCK") {
		t.Errorf("expected SAFETY BLOCK error, got: %v", err)
	}
}

func TestCheckSafety_DangerousRmRfStar(t *testing.T) {
	err := checkSafety("rm -rf /*")
	if err == nil {
		t.Error("expected safety block for 'rm -rf /*'")
	}
}

func TestCheckSafety_DangerousRmRfHome(t *testing.T) {
	err := checkSafety("rm -rf ~")
	if err == nil {
		t.Error("expected safety block for 'rm -rf ~'")
	}
}

func TestCheckSafety_DangerousRmRfDollarHomeLower(t *testing.T) {
	err := checkSafety("rm -rf /home")
	if err == nil {
		t.Error("expected safety block for 'rm -rf $HOME'")
	}
}

func TestCheckSafety_DangerousMkfs(t *testing.T) {
	err := checkSafety("mkfs.ext4 /dev/sda1")
	if err == nil {
		t.Error("expected safety block for 'mkfs.ext4 /dev/sda1'")
	}
}

func TestCheckSafety_DangerousDd(t *testing.T) {
	err := checkSafety("dd if=/dev/zero of=/dev/sda")
	if err == nil {
		t.Error("expected safety block for 'dd if=/dev/zero'")
	}
}

func TestCheckSafety_DangerousForkBomb(t *testing.T) {
	err := checkSafety(":(){ :|:& };:")
	if err == nil {
		t.Error("expected safety block for fork bomb")
	}
}

func TestCheckSafety_DangerousChmod000(t *testing.T) {
	err := checkSafety("chmod 000 /")
	if err == nil {
		t.Error("expected safety block for 'chmod 000 /'")
	}
}

func TestCheckSafety_RecursiveRmSlashRegex(t *testing.T) {
	err := checkSafety("rm -r /")
	if err == nil {
		t.Error("expected safety block for recursive rm on root")
	}
}

func TestCheckSafety_DangerousRmRfUsr(t *testing.T) {
	err := checkSafety("rm -rf /usr")
	if err == nil {
		t.Error("expected safety block for 'rm -rf /usr'")
	}
}

func TestCheckSafety_DangerousRmRfEtc(t *testing.T) {
	err := checkSafety("rm -rf /etc")
	if err == nil {
		t.Error("expected safety block for 'rm -rf /etc'")
	}
}

func TestCheckSafety_DangerousRmRfVar(t *testing.T) {
	err := checkSafety("rm -rf /var")
	if err == nil {
		t.Error("expected safety block for 'rm -rf /var'")
	}
}

func TestCheckSafety_DangerousMvToNull(t *testing.T) {
	err := checkSafety("mv / /dev/null")
	if err == nil {
		t.Error("expected safety block for 'mv / /dev/null'")
	}
}

func TestCheckSafety_DangerousShred(t *testing.T) {
	err := checkSafety("echo data | shred -")
	if err == nil {
		t.Error("expected safety block for 'shred -'")
	}
}

func TestCheckSafety_DangerousOverwritePasswd(t *testing.T) {
	err := checkSafety("echo root > /etc/passwd")
	if err == nil {
		t.Error("expected safety block for '> /etc/passwd'")
	}
}

func TestCheckSafety_DangerousCurlPipeSh(t *testing.T) {
	err := checkSafety("curl .* | sh")
	if err == nil {
		t.Error("expected safety block for 'curl | sh'")
	}
}

func TestCheckSafety_DangerousWgetPipeBash(t *testing.T) {
	err := checkSafety("wget .* | bash")
	if err == nil {
		t.Error("expected safety block for 'wget | bash'")
	}
}

func TestCheckSafety_DangerousEvalCurl(t *testing.T) {
	err := checkSafety("eval $(curl")
	if err == nil {
		t.Error("expected safety block for 'eval $(curl'")
	}
}

func TestCheckSafety_DangerousBashCurlRedirect(t *testing.T) {
	err := checkSafety("bash <(curl")
	if err == nil {
		t.Error("expected safety block for 'bash <(curl'")
	}
}

func TestCheckSafety_DangerousEvalWget(t *testing.T) {
	err := checkSafety("eval $(wget")
	if err == nil {
		t.Error("expected safety block for 'eval $(wget'")
	}
}

func TestCheckSafety_DangerousBashWgetRedirect(t *testing.T) {
	err := checkSafety("bash <(wget")
	if err == nil {
		t.Error("expected safety block for 'bash <(wget'")
	}
}

func TestCheckSafety_DangerousDevSda(t *testing.T) {
	err := checkSafety("echo data > /dev/sda")
	if err == nil {
		t.Error("expected safety block for '> /dev/sda'")
	}
}

func TestCheckSafety_DangerousCurlPipeBash(t *testing.T) {
	err := checkSafety("curl .* | bash")
	if err == nil {
		t.Error("expected safety block for 'curl | bash'")
	}
}

func TestCheckSafety_DangerousWgetPipeSh(t *testing.T) {
	err := checkSafety("wget .* | sh")
	if err == nil {
		t.Error("expected safety block for 'wget | sh'")
	}
}

func TestCheckSafety_SafeCommand(t *testing.T) {
	err := checkSafety("ls -la /tmp")
	if err != nil {
		t.Errorf("safe command should pass: %v", err)
	}
}

func TestCheckSafety_SafeRmDir(t *testing.T) {
	err := checkSafety("rm -rf ./build")
	if err != nil {
		t.Errorf("safe rm -rf should pass: %v", err)
	}
}

func TestCheckSafety_RecursiveRmRootRegex(t *testing.T) {
	err := checkSafety("rm -r /")
	if err == nil {
		t.Error("expected safety block for 'rm -r /'")
	}
}

func TestAnalyzeError_GeneralError(t *testing.T) {
	msg := analyzeError(1, "cmd")
	if msg != "general error" {
		t.Errorf("exit 1: expected 'general error', got %q", msg)
	}
}

func TestAnalyzeError_MisuseOfShellBuiltins(t *testing.T) {
	msg := analyzeError(2, "cmd")
	if msg != "misuse of shell builtins" {
		t.Errorf("exit 2: expected 'misuse of shell builtins', got %q", msg)
	}
}

func TestAnalyzeError_PermissionDenied(t *testing.T) {
	msg := analyzeError(126, "cmd")
	if !strings.Contains(msg, "permission denied") {
		t.Errorf("exit 126: expected permission denied, got %q", msg)
	}
}

func TestAnalyzeError_CommandNotFound(t *testing.T) {
	msg := analyzeError(127, "cmd")
	if !strings.Contains(msg, "not found") {
		t.Errorf("exit 127: expected command not found, got %q", msg)
	}
}

func TestAnalyzeError_InvalidExitArg(t *testing.T) {
	msg := analyzeError(128, "cmd")
	if !strings.Contains(msg, "invalid exit argument") {
		t.Errorf("exit 128: expected invalid exit argument, got %q", msg)
	}
}

func TestAnalyzeError_CtrlC(t *testing.T) {
	msg := analyzeError(130, "cmd")
	if !strings.Contains(msg, "Ctrl-C") {
		t.Errorf("exit 130: expected Ctrl-C, got %q", msg)
	}
}

func TestAnalyzeError_Sigkill(t *testing.T) {
	msg := analyzeError(137, "cmd")
	if !strings.Contains(msg, "SIGKILL") {
		t.Errorf("exit 137: expected SIGKILL, got %q", msg)
	}
}

func TestAnalyzeError_Sigsegv(t *testing.T) {
	msg := analyzeError(139, "cmd")
	if !strings.Contains(msg, "segmentation fault") {
		t.Errorf("exit 139: expected segfault, got %q", msg)
	}
}

func TestAnalyzeError_Sigterm(t *testing.T) {
	msg := analyzeError(143, "cmd")
	if !strings.Contains(msg, "SIGTERM") {
		t.Errorf("exit 143: expected SIGTERM, got %q", msg)
	}
}

func TestAnalyzeError_SignalRange(t *testing.T) {
	msg := analyzeError(129, "cmd")
	if !strings.Contains(msg, "terminated by signal") {
		t.Errorf("exit 129: expected signal message, got %q", msg)
	}
}

func TestAnalyzeError_SignalHighRange(t *testing.T) {
	msg := analyzeError(158, "cmd")
	if !strings.Contains(msg, "terminated by signal") {
		t.Errorf("exit 158: expected signal message, got %q", msg)
	}
}

func TestAnalyzeError_Unknown(t *testing.T) {
	msg := analyzeError(50, "cmd")
	if msg != "unknown error" {
		t.Errorf("exit 50: expected 'unknown error', got %q", msg)
	}
}

func TestAnalyzeError_OutOfSignalRange(t *testing.T) {
	msg := analyzeError(160, "cmd")
	if msg != "unknown error" {
		t.Errorf("exit 160: expected 'unknown error', got %q", msg)
	}
}

func TestRedactSecrets_APIKey(t *testing.T) {
	input := "api_key=abcdef1234567890"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890") {
		t.Error("api_key should be redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("should contain [REDACTED]")
	}
}

func TestRedactSecrets_APIKeyQuoted(t *testing.T) {
	input := `api_key="abcdef1234567890"`
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890") {
		t.Error("quoted api_key should be redacted")
	}
}

func TestRedactSecrets_APIKeyDash(t *testing.T) {
	input := "api-key=abcdef1234567890"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890") {
		t.Error("api-key dash form should be redacted")
	}
}

func TestRedactSecrets_Token(t *testing.T) {
	input := "token=ghp_abcdef1234567890"
	output := redactSecrets(input)
	if strings.Contains(output, "ghp_abcdef1234567890") {
		t.Error("token should be redacted")
	}
}

func TestRedactSecrets_Password(t *testing.T) {
	input := "password=s3cret"
	output := redactSecrets(input)
	if strings.Contains(output, "s3cret") {
		t.Error("password should be redacted")
	}
}

func TestRedactSecrets_Secret(t *testing.T) {
	input := "secret=abcdefghij"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdefghij") {
		t.Error("secret should be redacted")
	}
}

func TestRedactSecrets_Bearer(t *testing.T) {
	input := "Bearer abcdef1234567890token"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890token") {
		t.Error("Bearer token should be redacted")
	}
	if !strings.Contains(output, "Bearer [REDACTED]") {
		t.Error("should contain 'Bearer [REDACTED]'")
	}
}

func TestRedactSecrets_AWSAccessKeyID(t *testing.T) {
	input := "aws_access_key_id=ABCDEFGHIJKLMNOP"
	output := redactSecrets(input)
	if strings.Contains(output, "ABCDEFGHIJKLMNOP") {
		t.Error("AWS access key should be redacted")
	}
}

func TestRedactSecrets_AWSSecretAccessKey(t *testing.T) {
	input := "aws_secret_access_key=AbCdEf1234567890/+/xyz="
	output := redactSecrets(input)
	if strings.Contains(output, "AbCdEf1234567890") {
		t.Error("AWS secret key should be redacted")
	}
}

func TestRedactSecrets_PrivateKey(t *testing.T) {
	input := "private_key=abcdef1234567890123456789012"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890123456789012") {
		t.Error("private_key should be redacted")
	}
}

func TestRedactSecrets_Auth(t *testing.T) {
	input := "auth=abcdef1234567890"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890") {
		t.Error("auth should be redacted")
	}
}

func TestRedactSecrets_MultipleSecrets(t *testing.T) {
	input := "api_key=abcdef1234567890 token=ghijklmnopqrstuv password=pass1"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890") || strings.Contains(output, "ghijklmnopqrstuv") || strings.Contains(output, "pass1") {
		t.Error("all secrets should be redacted")
	}
}

func TestRedactSecrets_NoSecrets(t *testing.T) {
	input := "just some normal text without secrets"
	output := redactSecrets(input)
	if output != input {
		t.Error("text without secrets should not be modified")
	}
}

func TestRedactSecrets_CaseInsensitive(t *testing.T) {
	input := "API_KEY=abcdef1234567890"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890") {
		t.Error("API_KEY should be redacted (case insensitive)")
	}
}

func TestRedactSecrets_ColonSyntax(t *testing.T) {
	input := "api_key: abcdef1234567890"
	output := redactSecrets(input)
	if strings.Contains(output, "abcdef1234567890") {
		t.Error("api_key with colon should be redacted")
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("sleep 10", 1, "text", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Logf("runCommand returned error (expected for timeout): %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "TIMEOUT") {
		t.Errorf("expected TIMEOUT in output, got: %s", output)
	}
}

func TestRunCommand_ZeroTimeout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo no_timeout", 0, "text", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error with zero timeout: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "no_timeout") {
		t.Errorf("expected output from echo, got: %s", output)
	}
}

func TestRunCommand_JSONFormat(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo json_test", 5, "json", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		if unmarshalErr := json.Unmarshal([]byte(output), &result); unmarshalErr != nil {
			t.Fatalf("expected valid JSON output, got: %s", output)
		}
	}

	if result.Command != "echo json_test" {
		t.Errorf("expected command in JSON, got: %s", result.Command)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got: %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "json_test") {
		t.Errorf("expected stdout to contain 'json_test', got: %s", result.Stdout)
	}
}

func TestRunCommand_JSONFormatWithExitCode(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = runCommand("exit 42", 5, "json", false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s, err: %v", output, err)
	}
	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got: %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "EXIT CODE 42") {
		t.Errorf("expected EXIT CODE 42 in error, got: %s", result.Error)
	}
}

func TestRunCommand_TextFormat(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo text_format_test", 5, "text", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "text_format_test") {
		t.Errorf("expected output, got: %s", output)
	}
	if !strings.Contains(output, "Command:") {
		t.Errorf("expected 'Command:' label in text format, got: %s", output)
	}
}

func TestRunCommand_StreamMode(t *testing.T) {
	err := runCommand("echo stream_test", 5, "text", true)
	if err != nil {
		t.Errorf("stream mode should not return error for success: %v", err)
	}
}

func TestRunCommand_StreamModeWithError(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_ = runCommand("exit 1", 5, "text", true)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	_ = buf.String()
}

func TestRunCommand_RedactedOutput(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo 'api_key=abcdef1234567890xyz'", 5, "json", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if !result.Redacted {
		t.Error("expected Redacted=true when output contains secrets")
	}
	if strings.Contains(result.Stdout, "abcdef1234567890xyz") {
		t.Error("secret should be redacted in output")
	}
}

func TestRunCommand_StderrCapture(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo stderr_test >&2", 5, "json", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if !strings.Contains(result.Stderr, "stderr_test") {
		t.Errorf("expected stderr content, got: %s", result.Stderr)
	}
}

func TestRunCommand_NonZeroExit(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = runCommand("exit 7", 5, "json", false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if result.ExitCode != 7 {
		t.Errorf("expected exit code 7, got: %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "EXIT CODE 7") {
		t.Errorf("expected error message with EXIT CODE 7, got: %s", result.Error)
	}
}

func TestRunCommand_TimeoutJSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = runCommand("sleep 10", 1, "json", false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if result.ExitCode != 124 {
		t.Errorf("expected exit code 124 for timeout, got: %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "TIMEOUT") {
		t.Errorf("expected TIMEOUT in error, got: %s", result.Error)
	}
}

func TestRunCommand_DurationRecorded(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo duration_test", 5, "json", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if result.Duration == "" {
		t.Error("expected non-empty duration")
	}
	dur, parseErr := time.ParseDuration(result.Duration)
	if parseErr != nil {
		t.Errorf("duration should be parseable: %v", parseErr)
	}
	if dur < 0 {
		t.Error("duration should be non-negative")
	}
}

func TestRunCommand_TextFormatNoStdout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("true", 5, "text", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if strings.Contains(output, "--- stdout ---") {
		t.Error("should not show stdout section when stdout is empty")
	}
}

func TestRunCommand_TextFormatNoStderr(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo hello", 5, "text", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if strings.Contains(output, "--- stderr ---") {
		t.Error("should not show stderr section when stderr is empty")
	}
}

func TestExecuteCmd_SafetyBlock(t *testing.T) {
	execCommand = "rm -rf /"
	execFormat = "text"
	execTimeout = 5
	err := ExecuteCmd.RunE(ExecuteCmd, []string{})
	if err == nil {
		t.Error("expected safety block error")
	}
	if !strings.Contains(err.Error(), "SAFETY BLOCK") {
		t.Errorf("expected SAFETY BLOCK, got: %v", err)
	}
}

func TestExecuteCmd_JSONOutput(t *testing.T) {
	execCommand = "echo json_cmd_test"
	execFormat = "json"
	execTimeout = 5
	execStream = false

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := ExecuteCmd.RunE(ExecuteCmd, []string{})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "json_cmd_test") {
		t.Errorf("expected json_cmd_test in output, got: %s", output)
	}
}

func TestRunCommand_ShortTimeout(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = runCommand("sleep 5", 1, "json", false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if result.ExitCode != 124 {
		t.Errorf("expected exit code 124, got: %d", result.ExitCode)
	}
}

func TestRunCommand_ExecTimeoutNoExitError(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = runCommand("sleep 999", 1, "json", false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if result.ExitCode != 124 {
		t.Errorf("expected exit code 124 for timeout, got: %d", result.ExitCode)
	}
	if result.Duration == "" {
		t.Error("expected duration to be recorded")
	}
}

func TestRunCommand_StreamWithTimeout(t *testing.T) {
	err := runCommand("sleep 999", 1, "text", true)
	if err != nil {
		t.Errorf("stream with timeout should not return error: %v", err)
	}
}

func TestRunCommand_CommandWithLargeOutput(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("seq 1 1000", 5, "json", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", output[:200])
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got: %d", result.ExitCode)
	}
}

func TestRunCommand_TextFormatWithError(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("false", 5, "text", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("runCommand should not return error for non-zero exit: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "ERROR:") {
		t.Errorf("expected ERROR in text output for non-zero exit, got: %s", output)
	}
}

func TestRunCommand_RedactedFalseWhenNoSecrets(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo clean_output", 5, "json", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result execResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", output)
	}
	if result.Redacted {
		t.Error("expected Redacted=false when no secrets in output")
	}
}

func TestRunCommand_TextFormatWithStderr(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCommand("echo err >&2 && echo out", 5, "text", false)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("expected no error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "--- stderr ---") {
		t.Errorf("expected stderr section in text output, got: %s", output)
	}
	if !strings.Contains(output, "--- stdout ---") {
		t.Errorf("expected stdout section in text output, got: %s", output)
	}
}
