package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/OpenSIN-Code/SIN-Code/internal/rtk"
)

// NewRTKCmd creates the RTK command
func NewRTKCmd() *cobra.Command {
	var (
		binaryPath string
		configDir  string
		logLevel   string
	)

	rtkCmd := &cobra.Command{
		Use:   "rtk",
		Short: "Manage RTK (Rapid Toolkit) integration",
		Long: `Manage RTK integration for SIN-Code.

RTK provides linting, formatting, testing, and analysis capabilities.
This command allows you to detect, configure, and use RTK tools.

Examples:
  sin-code rtk detect          # Detect RTK binary
  sin-code rtk run lint        # Run RTK linter
  sin-code rtk config show     # Show configuration
  sin-code rtk metrics         # Show metrics
  sin-code rtk status          # Show RTK status`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if logLevel == "" {
				logLevel = "info"
			}
			return nil
		},
	}

	rtkCmd.PersistentFlags().StringVar(&binaryPath, "binary", "", "Path to rtk binary")
	rtkCmd.PersistentFlags().StringVar(&configDir, "config", "", "Config directory")
	rtkCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	// Subcommands
	rtkCmd.AddCommand(
		newRTKInstallCmd(),
		newRTKDetectCmd(&binaryPath, &configDir),
		newRTKRunCmd(&binaryPath, &configDir),
		newRTKConfigCmd(&binaryPath, &configDir),
		newRTKMetricsCmd(&binaryPath, &configDir),
		newRTKStatusCmd(&binaryPath, &configDir),
		newRTKInitCmd(&binaryPath, &configDir),
	)

	return rtkCmd
}

// newRTKDetectCmd creates the detect subcommand
func newRTKDetectCmd(binaryPath *string, configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect RTK binary",
		Long:  "Auto-detect RTK binary in system paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			config := &rtk.RTKConfig{
				Enabled:      true,
				DetectBinary: true,
				BinaryPath:   *binaryPath,
			}

			executor, err := rtk.NewSimpleExecutor(config)
			if err != nil {
				return err
			}

			cliCmd := rtk.NewCLICommand(executor)
			return cliCmd.DetectCommand(ctx)
		},
	}
}

// newRTKRunCmd creates the run subcommand
func newRTKRunCmd(binaryPath *string, configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "run [tool] [args...]",
		Short: "Run an RTK tool",
		Long:  "Execute an RTK tool with arguments",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			// Load configuration
			manager := rtk.NewConfigManager(*configDir)
			config, err := manager.LoadConfig()
			if err != nil {
				// Use default config if load fails
				config = &rtk.RTKConfig{
					Enabled:      true,
					DetectBinary: true,
					GlobalTimeout: rtk.DefaultGlobalTimeout,
					CacheEnabled: true,
					CacheTTL:     rtk.DefaultCacheTTL,
					StripANSI:    true,
				}
			}

			if *binaryPath != "" {
				config.BinaryPath = *binaryPath
			}

			executor, err := rtk.NewSimpleExecutor(config)
			if err != nil {
				return err
			}

			cliCmd := rtk.NewCLICommand(executor)

			toolName := args[0]
			toolArgs := args[1:]

			return cliCmd.RunCommand(ctx, toolName, toolArgs)
		},
	}
}

// newRTKConfigCmd creates the config subcommand
func newRTKConfigCmd(binaryPath *string, configDir *string) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage RTK configuration",
		Long:  "Show, get, or set RTK configuration",
	}

	// config show
	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := rtk.NewConfigManager(*configDir)
			config, err := manager.LoadConfig()
			if err != nil {
				return err
			}

			executor, err := rtk.NewSimpleExecutor(config)
			if err != nil {
				return err
			}

			cliCmd := rtk.NewCLICommand(executor)
			return cliCmd.ConfigCommand("show", "", "")
		},
	})

	// config set
	configCmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := rtk.NewConfigManager(*configDir)
			config, err := manager.LoadConfig()
			if err != nil {
				config = &rtk.RTKConfig{}
			}

			executor, err := rtk.NewSimpleExecutor(config)
			if err != nil {
				return err
			}

			cliCmd := rtk.NewCLICommand(executor)
			return cliCmd.ConfigCommand("set", args[0], args[1])
		},
	})

	// config get
	configCmd.AddCommand(&cobra.Command{
		Use:   "get [key]",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := rtk.NewConfigManager(*configDir)
			config, err := manager.LoadConfig()
			if err != nil {
				return err
			}

			executor, err := rtk.NewSimpleExecutor(config)
			if err != nil {
				return err
			}

			cliCmd := rtk.NewCLICommand(executor)
			return cliCmd.ConfigCommand("get", args[0], "")
		},
	})

	return configCmd
}

// newRTKMetricsCmd creates the metrics subcommand
func newRTKMetricsCmd(binaryPath *string, configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "metrics",
		Short: "Show RTK metrics",
		Long:  "Display collected RTK performance metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := rtk.NewConfigManager(*configDir)
			config, err := manager.LoadConfig()
			if err != nil {
				config = &rtk.RTKConfig{}
			}

			executor, err := rtk.NewSimpleExecutor(config)
			if err != nil {
				return err
			}

			cliCmd := rtk.NewCLICommand(executor)
			return cliCmd.MetricsCommand()
		},
	}
}

// newRTKStatusCmd creates the status subcommand
func newRTKStatusCmd(binaryPath *string, configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show RTK status",
		Long:  "Display current RTK status and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			manager := rtk.NewConfigManager(*configDir)
			config, err := manager.LoadConfig()
			if err != nil {
				// Use default if load fails
				config = &rtk.RTKConfig{
					Enabled:      true,
					DetectBinary: true,
				}
			}

			if *binaryPath != "" {
				config.BinaryPath = *binaryPath
			}

			executor, err := rtk.NewSimpleExecutor(config)
			if err != nil {
				return err
			}

			cliCmd := rtk.NewCLICommand(executor)
			return cliCmd.StatusCommand(ctx)
		},
	}
}

// newRTKInitCmd creates the init subcommand
func newRTKInitCmd(binaryPath *string, configDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize RTK configuration",
		Long:  "Create default RTK configuration files",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := rtk.NewConfigManager(*configDir)

			if err := manager.ResetConfig(); err != nil {
				return err
			}

			config := manager.GetConfig()
			fmt.Printf("RTK configuration initialized at: %s\n", manager.GetConfigFile())
			fmt.Printf("Configuration:\n")
			fmt.Printf("  Enabled: %v\n", config.Enabled)
			fmt.Printf("  Cache: %v (TTL: %v)\n", config.CacheEnabled, config.CacheTTL)
			fmt.Printf("  ANSI Stripping: %v\n", config.StripANSI)
			fmt.Printf("  Global Timeout: %v\n", config.GlobalTimeout)

			return nil
		},
	}
}

// newRTKInstallCmd installs the official rtk binary from https://github.com/rtk-ai/rtk
func newRTKInstallCmd() *cobra.Command {
	var method string

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the official RTK binary",
		Long: `Install the official RTK binary from https://github.com/rtk-ai/rtk

RTK is a CLI proxy that reduces LLM token consumption by 60-90% on common
dev commands. SIN-Code wraps this binary; this command fetches it for you.

Methods:
  script  (default) curl -fsSL .../install.sh | sh   -> installs to ~/.local/bin
  brew              brew install rtk
  cargo             cargo install --git https://github.com/rtk-ai/rtk

Examples:
  sin-code rtk install
  sin-code rtk install --method brew
  sin-code rtk install --method cargo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			// Skip if already present
			if path, err := exec.LookPath("rtk"); err == nil {
				fmt.Printf("RTK already installed at: %s\n", path)
				out, _ := exec.CommandContext(ctx, path, "--version").Output()
				if v := strings.TrimSpace(string(out)); v != "" {
					fmt.Printf("Version: %s\n", v)
				}
				return nil
			}

			var install *exec.Cmd
			switch method {
			case "brew":
				fmt.Println("Installing RTK via Homebrew...")
				install = exec.CommandContext(ctx, "brew", "install", "rtk")
			case "cargo":
				fmt.Println("Installing RTK via Cargo...")
				install = exec.CommandContext(ctx, "cargo", "install", "--git", "https://github.com/rtk-ai/rtk")
			case "script", "":
				fmt.Println("Installing RTK via official install script...")
				const script = "curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh"
				install = exec.CommandContext(ctx, "sh", "-c", script)
			default:
				return fmt.Errorf("unknown install method %q (use: script, brew, cargo)", method)
			}

			install.Stdout = os.Stdout
			install.Stderr = os.Stderr
			install.Stdin = os.Stdin
			if err := install.Run(); err != nil {
				return fmt.Errorf("RTK installation failed: %w", err)
			}

			// Verify
			path, err := exec.LookPath("rtk")
			if err != nil {
				fmt.Println("\nRTK installed, but not found on PATH yet.")
				fmt.Println("Add it to your PATH, e.g.:")
				fmt.Println(`  export PATH="$HOME/.local/bin:$PATH"`)
				fmt.Println("Then run: sin-code rtk status")
				return nil
			}

			fmt.Printf("\nRTK installed successfully at: %s\n", path)
			out, _ := exec.CommandContext(ctx, path, "--version").Output()
			if v := strings.TrimSpace(string(out)); v != "" {
				fmt.Printf("Version: %s\n", v)
			}
			fmt.Println("Run 'sin-code rtk status' to verify the integration.")
			return nil
		},
	}

	installCmd.Flags().StringVar(&method, "method", "script", "Install method: script, brew, or cargo")
	return installCmd
}
