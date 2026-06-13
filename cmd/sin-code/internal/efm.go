// SPDX-License-Identifier: MIT
// Purpose: efm — Ephemeral Full-Stack Mocking. Manages docker-compose stacks
// and ephemeral test environments. Pure Go implementation.
package internal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	efmStack   string
	efmAction  string
	efmTTL     int
	efmFormat  string
	efmRuntime string
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
	Action    string        `json:"action"`
	Stack     string        `json:"stack,omitempty"`
	Status    string        `json:"status"`
	Services  []efmService  `json:"services,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  string        `json:"duration,omitempty"`
	Runtime   string        `json:"runtime,omitempty"`
}

type efmService struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Ports  []string `json:"ports,omitempty"`
	Image  string `json:"image,omitempty"`
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
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("orb"); err == nil {
			return "orb"
		}
		if _, err := exec.LookPath("docker"); err == nil {
			return "docker"
		}
		return "docker"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return "docker"
}

func containerCommand(rt string, args ...string) *exec.Cmd {
	bin := rt
	if bin == "" {
		bin = detectContainerRuntime()
	}
	if bin == "" {
		bin = "docker"
	}
	return exec.Command(bin, args...)
}

func legacyComposeCommand(rt string, args ...string) *exec.Cmd {
	if rt == "orb" || rt == "docker" {
		return exec.Command(rt+"-compose", args...)
	}
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
		cmd := exec.Command(c, "ps", "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Image}}")
		var err error
		out, err = cmd.Output()
		if err == nil {
			usedRt = c
			break
		}
		lastErr = err
	}
	if out == nil {
		if lastErr != nil {
			return nil, fmt.Errorf("no container runtime responded (tried %v): %w", cands, lastErr)
		}
		return nil, fmt.Errorf("no container runtime binary found (tried %v)", cands)
	}
	if usedRt != "" {
		_ = usedRt
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

func dockerComposeUp(stack string, ttl int, rt string) error {
	absPath, err := filepath.Abs(stack)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("stack file not found: %w", err)
	}

	rt = resolveComposeRuntime(rt)
	if err := runComposeCandidates(rt, []string{"-f", absPath, "up", "-d"}, true); err != nil {
		return fmt.Errorf("%s compose up failed: %w", rt, err)
	}

	if ttl > 0 {
		metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
		_ = os.MkdirAll(metadataDir, 0755)
		metadataFile := filepath.Join(metadataDir, metadataKey(absPath))
		meta := map[string]string{
			"stack":   absPath,
			"started": time.Now().Format(time.RFC3339),
			"ttl":     fmt.Sprintf("%d", ttl),
			"expires": time.Now().Add(time.Duration(ttl) * time.Second).Format(time.RFC3339),
			"runtime": rt,
		}
		data, _ := json.MarshalIndent(meta, "", "  ")
		_ = os.WriteFile(metadataFile, data, 0644)
	}

	return nil
}

func dockerComposeDown(stack string, rt string) error {
	absPath, err := filepath.Abs(stack)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("stack file not found: %w", err)
	}

	rt = resolveComposeRuntime(rt)
	if err := runComposeCandidates(rt, []string{"-f", absPath, "down"}, true); err != nil {
		return fmt.Errorf("%s compose down failed: %w", rt, err)
	}

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	metadataFile := filepath.Join(metadataDir, metadataKey(absPath))
	_ = os.Remove(metadataFile)

	return nil
}

func dockerComposeStatus(stack string, rt string) (string, error) {
	absPath, err := filepath.Abs(stack)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("stack file not found: %w", err)
	}

	rt = resolveComposeRuntime(rt)
	out, err := runComposeCapture(rt, []string{"-f", absPath, "ps", "--format", "{{.State}}"})
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
		cmd := exec.Command(c, full...)
		if attachStdio {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		if err := cmd.Run(); err != nil {
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
		cmd := exec.Command(c, full...)
		out, err := cmd.Output()
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
	projectName := strings.TrimSuffix(filepath.Base(stack), filepath.Ext(stack))
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
