package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sin",
	Short: "SIN-Code: Secure Infrastructure Orchestration",
	Long:  "Advanced infrastructure security and orchestration tool with agent skills support",
}

func init() {
	// Initialize skill commands
	initSkillCommands()
}

func initSkillCommands() {
	// This will be called from skill_cmds.go init()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
