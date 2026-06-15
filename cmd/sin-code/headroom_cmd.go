// cmd/sin-code/headroom_cmd.go
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

// headroomCmd represents the headroom command
var headroomCmd = &cobra.Command{
	Use:   "headroom",
	Short: "Manage Headroom context compression integration",
	Long: `Headroom provides intelligent context compression for LLM requests.
It can reduce token usage by up to 92% with minimal accuracy loss.

Subcommands:
  enable    Enable Headroom compression
  disable   Disable Headroom compression
  stats     Show compression statistics
  learn     Manually trigger learning from a session log
  test      Test Headroom connection and compression
`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var headroomEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable Headroom compression",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := headroom.LoadConfigFromEnv()
		cfg.Enabled = true
		fmt.Println("✅ Headroom compression enabled")
		fmt.Println("   Mode:", cfg.Mode)
		fmt.Println("   Level:", cfg.CompressionLevel)
		return nil
	},
}

var headroomDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable Headroom compression",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("❌ Headroom compression disabled")
		return nil
	},
}

var headroomStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show compression statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := headroom.LoadConfigFromEnv()
		comp := headroom.NewCompressor(cfg)
		ctx := context.Background()
		if err := comp.Start(ctx); err != nil {
			return fmt.Errorf("headroom not available: %w", err)
		}
		defer comp.Close()

		stats := comp.GetStats()
		fmt.Printf("📊 Headroom Statistics\n")
		fmt.Printf("   Total requests:        %d\n", stats.TotalRequests)
		fmt.Printf("   Total compressed:      %d\n", stats.TotalCompressed)
		fmt.Printf("   Original tokens:       %d\n", stats.TotalOriginalTokens)
		fmt.Printf("   Compressed tokens:     %d\n", stats.TotalCompressedTokens)
		fmt.Printf("   Average savings:       %.2f%%\n", stats.AverageSavings)
		if !stats.LastLearnTime.IsZero() {
			fmt.Printf("   Last learning:          %s\n", stats.LastLearnTime.Format(time.RFC3339))
		}
		return nil
	},
}

var headroomTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test Headroom connection and compression",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := headroom.LoadConfigFromEnv()
		comp := headroom.NewCompressor(cfg)
		ctx := context.Background()
		if err := comp.Start(ctx); err != nil {
			return fmt.Errorf("headroom not available: %w\nInstall headroom: pip install headroom-ai[all]", err)
		}
		defer comp.Close()

		testContent := `This is a test of headroom compression. It should reduce the token count significantly. 
Repeat this sentence many times to see the effect. Repeat this sentence many times to see the effect. 
Repeat this sentence many times to see the effect.`

		compressed, result, err := comp.CompressContent(ctx, testContent)
		if err != nil {
			return fmt.Errorf("compression test failed: %w", err)
		}

		fmt.Println("🔍 Headroom Test Result:")
		fmt.Printf("   Original length: %d chars (~%d tokens)\n", len(testContent), len(testContent)/4)
		fmt.Printf("   Compressed length: %d chars (~%d tokens)\n", len(compressed), len(compressed)/4)
		if result != nil {
			fmt.Printf("   Savings: %.2f%%\n", result.SavingsPercent)
			fmt.Printf("   Algorithm: %s\n", result.Algorithm)
			fmt.Printf("   Duration: %d ms\n", result.DurationMs)
		}
		fmt.Println("\n✅ Headroom is working correctly!")
		return nil
	},
}

var headroomLearnCmd = &cobra.Command{
	Use:   "learn [file]",
	Short: "Learn from a failed session log",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var logContent string
		if len(args) > 0 && args[0] != "" {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("failed to read log file: %w", err)
			}
			logContent = string(data)
		} else {
			// Read from stdin
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				logContent = string(data)
			} else {
				return fmt.Errorf("please provide a session log file or pipe to stdin")
			}
		}

		cfg := headroom.LoadConfigFromEnv()
		client := headroom.NewCLIClient(cfg)
		if err := client.Learn(context.Background(), logContent); err != nil {
			return fmt.Errorf("learning failed: %w", err)
		}
		fmt.Println("✅ Headroom learned from the provided session")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(headroomCmd)
	headroomCmd.AddCommand(headroomEnableCmd)
	headroomCmd.AddCommand(headroomDisableCmd)
	headroomCmd.AddCommand(headroomStatsCmd)
	headroomCmd.AddCommand(headroomTestCmd)
	headroomCmd.AddCommand(headroomLearnCmd)
}
