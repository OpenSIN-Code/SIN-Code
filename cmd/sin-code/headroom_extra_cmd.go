// cmd/sin-code/headroom_extra_cmd.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

var (
	proxyAddr     string
	proxyUpstream string
	lessonsPath   string
)

// headroomProxyCmd starts the Headroom HTTP compression proxy.
var headroomProxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Run the Headroom HTTP compression proxy",
	Long: `Starts a reverse proxy that intercepts OpenAI-compatible chat requests,
compresses message contents through Headroom, and forwards them upstream.

Point your LLM client's base URL at this proxy for zero-code-change compression:

  sin-code headroom proxy --addr :8787 --upstream https://api.openai.com
  export OPENAI_BASE_URL=http://localhost:8787/v1
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := headroom.LoadConfigFromEnv()
		cfg.Enabled = true
		// The proxy needs a backend that actually compresses; prefer CLI mode
		// so the proxy itself does the work rather than delegating to a proxy.
		if cfg.Mode == headroom.ModeProxy {
			cfg.Mode = headroom.ModeCLI
		}

		comp := headroom.NewCompressor(cfg)
		ctx := context.Background()
		if err := comp.Start(ctx); err != nil {
			return fmt.Errorf("headroom backend not available: %w\nInstall headroom: pip install headroom-ai[all]", err)
		}
		defer comp.Close()

		proxy, err := headroom.NewProxy(cfg, comp, proxyUpstream)
		if err != nil {
			return err
		}

		// Graceful shutdown on SIGINT/SIGTERM.
		errCh := make(chan error, 1)
		go func() { errCh <- proxy.Start(proxyAddr) }()

		fmt.Printf("Headroom proxy listening on %s -> %s\n", proxyAddr, proxyUpstream)
		fmt.Printf("Set your client base URL to http://localhost%s/v1\n", proxyAddr)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		select {
		case err := <-errCh:
			return err
		case <-sigCh:
			fmt.Println("\nShutting down proxy...")
			return proxy.Shutdown(context.Background())
		}
	},
}

// headroomLessonsCmd manages the local lessons store.
var headroomLessonsCmd = &cobra.Command{
	Use:   "lessons",
	Short: "Inspect lessons learned from failed sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := headroom.NewLessonStore(lessonsPath)
		if err != nil {
			return fmt.Errorf("opening lessons store: %w", err)
		}

		lessons := store.Top(0)
		if len(lessons) == 0 {
			fmt.Println("No lessons recorded yet.")
			return nil
		}

		fmt.Printf("Lessons (%d):\n", store.Count())
		for i, l := range lessons {
			fmt.Printf("%d. [%s] weight=%.2f hits=%d\n", i+1, l.Category, l.Weight, l.Hits)
			fmt.Printf("   insight: %s\n", l.Insight)
		}
		return nil
	},
}

// headroomLessonsClearCmd removes all recorded lessons.
var headroomLessonsClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all recorded lessons",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := lessonsPath
		if path == "" {
			path = headroom.DefaultLessonsPath()
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clearing lessons: %w", err)
		}
		fmt.Println("Lessons cleared.")
		return nil
	},
}

func init() {
	headroomProxyCmd.Flags().StringVar(&proxyAddr, "addr", ":8787", "Address for the proxy to listen on")
	headroomProxyCmd.Flags().StringVar(&proxyUpstream, "upstream", "https://api.openai.com", "Upstream OpenAI-compatible base URL")

	headroomLessonsCmd.PersistentFlags().StringVar(&lessonsPath, "path", "", "Path to the lessons store file (default ~/.sin-code/headroom/lessons.json)")
	headroomLessonsCmd.AddCommand(headroomLessonsClearCmd)

	headroomCmd.AddCommand(headroomProxyCmd)
	headroomCmd.AddCommand(headroomLessonsCmd)
}
