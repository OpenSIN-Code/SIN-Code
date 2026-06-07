// SPDX-License-Identifier: MIT
// Purpose: efm — Ephemeral Full-Stack Mocking. Manages docker-compose stacks
// and ephemeral test environments. Pure Go implementation.
package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	efmStack   string
	efmAction  string
	efmTTL     int
	efmFormat  string
)

var EfmCmd = &cobra.Command{
	Use:   "efm",
	Short: "Ephemeral Full-Stack Mocking — spin up disposable test environments",
	Long: `Manage disposable full-stack environments (Docker Compose, ephemeral containers).
Pure Go implementation.

Examples:
  sin-code efm --action list
  sin-code efm --action up --stack docker-compose.yml --ttl 3600
  sin-code efm --action down --stack docker-compose.yml
  sin-code efm --action status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEFM(efmAction, efmStack, efmTTL, efmFormat)
	},
}

type efmResult struct {
	Action    string        `json:"action"`
	Stack     string        `json:"stack,omitempty"`
	Status    string        `json:"status"`
	Services  []efmService  `json:"services,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  string        `json:"duration,omitempty"`
}

type efmService struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Ports  []string `json:"ports,omitempty"`
	Image  string `json:"image,omitempty"`
}

func runEFM(action, stack string, ttl int, format string) error {
	start := time.Now()
	result := efmResult{Action: action, Stack: stack}

	switch action {
	case "list":
		services, err := listDockerContainers()
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
		err := dockerComposeUp(stack, ttl)
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
		} else {
			result.Status = "started"
			// List services after start
			services, _ := listDockerContainers()
			result.Services = filterServices(services, stack)
		}
	case "down":
		if stack == "" {
			return fmt.Errorf("--stack is required for action 'down'")
		}
		err := dockerComposeDown(stack)
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
		} else {
			result.Status = "stopped"
		}
	case "status":
		if stack == "" {
			// List all running containers
			services, err := listDockerContainers()
			if err != nil {
				result.Status = "error"
				result.Error = err.Error()
			} else {
				result.Status = "ok"
				result.Services = services
			}
		} else {
			status, err := dockerComposeStatus(stack)
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

func listDockerContainers() ([]efmService, error) {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Image}}")
	out, err := cmd.Output()
	if err != nil {
		// Docker might not be running or installed
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("docker not available: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("docker not available: %w", err)
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

func dockerComposeUp(stack string, ttl int) error {
	absPath, err := filepath.Abs(stack)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("stack file not found: %w", err)
	}

	cmd := exec.Command("docker", "compose", "-f", absPath, "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Try legacy docker-compose
		cmd = exec.Command("docker-compose", "-f", absPath, "up", "-d")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker compose up failed: %w", err)
		}
	}

	// Store TTL info in a metadata file
	if ttl > 0 {
		metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
		_ = os.MkdirAll(metadataDir, 0755)
		metadataFile := filepath.Join(metadataDir, filepath.Base(absPath)+".meta")
		meta := map[string]string{
			"stack":     absPath,
			"started":   time.Now().Format(time.RFC3339),
			"ttl":       fmt.Sprintf("%d", ttl),
			"expires":   time.Now().Add(time.Duration(ttl) * time.Second).Format(time.RFC3339),
		}
		data, _ := json.MarshalIndent(meta, "", "  ")
		_ = os.WriteFile(metadataFile, data, 0644)
	}

	return nil
}

func dockerComposeDown(stack string) error {
	absPath, err := filepath.Abs(stack)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("stack file not found: %w", err)
	}

	cmd := exec.Command("docker", "compose", "-f", absPath, "down")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("docker-compose", "-f", absPath, "down")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker compose down failed: %w", err)
		}
	}

	// Remove metadata file
	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	metadataFile := filepath.Join(metadataDir, filepath.Base(absPath)+".meta")
	_ = os.Remove(metadataFile)

	return nil
}

func dockerComposeStatus(stack string) (string, error) {
	absPath, err := filepath.Abs(stack)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("stack file not found: %w", err)
	}

	cmd := exec.Command("docker", "compose", "-f", absPath, "ps", "--format", "{{.State}}")
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("docker-compose", "-f", absPath, "ps", "--format", "{{.State}}")
		out, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("docker compose ps failed: %w", err)
		}
	}

	states := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(states) == 0 || (len(states) == 1 && states[0] == "") {
		return "no containers running", nil
	}

	allRunning := true
	for _, state := range states {
		if !strings.Contains(strings.ToLower(state), "running") {
			allRunning = false
			break
		}
	}
	if allRunning {
		return "all running", nil
	}
	return "partial", nil
}

func filterServices(services []efmService, stack string) []efmService {
	// Filter services by stack name (project name)
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
	EfmCmd.Flags().StringVarP(&efmAction, "action", "a", "list", "Action: up|down|list|status")
	EfmCmd.Flags().StringVarP(&efmStack, "stack", "s", "", "Stack definition (docker-compose.yml, k8s manifest, etc.)")
	EfmCmd.Flags().IntVarP(&efmTTL, "ttl", "t", 3600, "Time-to-live in seconds (0 = no auto-cleanup)")
	EfmCmd.Flags().StringVarP(&efmFormat, "format", "f", "text", "Output format: text|json")
}
