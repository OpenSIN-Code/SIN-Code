package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/pkg/skills"
)

var (
	skillsDir   string
	verbose     bool
	autoConfirm bool
	maxSteps    int
	budget      int
)

func init() {
	// Standard-Skills-Verzeichnis: ~/.sin/skills oder ./skills
	home, err := os.UserHomeDir()
	if err != nil {
		skillsDir = "./skills"
	} else {
		skillsDir = filepath.Join(home, ".sin", "skills")
	}

	// Root command already exists; we add a new "skill" command
	skillCmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage and run agent skills",
		Long:  "Install, list, remove, and execute SKILL.md workflows.",
	}

	// Persistent flag so every subcommand can point at a custom skills dir.
	skillCmd.PersistentFlags().StringVar(&skillsDir, "skills-dir", skillsDir, "Directory containing installed skills")

	// List skills
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE:  runSkillList,
	}
	skillCmd.AddCommand(listCmd)

	// Install a skill
	installCmd := &cobra.Command{
		Use:   "install <path-or-git-url>",
		Short: "Install a skill from local path or git repository",
		Args:  cobra.ExactArgs(1),
		RunE:  runSkillInstall,
	}
	skillCmd.AddCommand(installCmd)

	// Remove a skill
	removeCmd := &cobra.Command{
		Use:   "remove <skill-name>",
		Short: "Remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE:  runSkillRemove,
	}
	skillCmd.AddCommand(removeCmd)

	// Run a skill
	runCmd := &cobra.Command{
		Use:   "run <skill-name>",
		Short: "Execute a skill",
		Args:  cobra.ExactArgs(1),
		RunE:  runSkillRun,
	}
	runCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	runCmd.Flags().BoolVarP(&autoConfirm, "yes", "y", false, "Auto-confirm steps")
	runCmd.Flags().IntVarP(&maxSteps, "max-steps", "m", 0, "Maximum steps to execute")
	runCmd.Flags().IntVarP(&budget, "budget", "b", 100000, "Token budget for agents")
	skillCmd.AddCommand(runCmd)

	// Validate a skill
	validateCmd := &cobra.Command{
		Use:   "validate <skill-path>",
		Short: "Validate a SKILL.md file",
		Args:  cobra.ExactArgs(1),
		RunE:  runSkillValidate,
	}
	skillCmd.AddCommand(validateCmd)

	// Add to root command
	if rootCmd != nil {
		rootCmd.AddCommand(skillCmd)
	}
}

func runSkillList(cmd *cobra.Command, args []string) error {
	reg, err := skills.NewRegistry(skillsDir)
	if err != nil {
		return err
	}
	names := reg.List()
	if len(names) == 0 {
		fmt.Println("No skills installed.")
		return nil
	}
	fmt.Println("Installed skills:")
	for _, n := range names {
		s, _ := reg.Get(n)
		fmt.Printf("  %s - %s\n", n, s.Description)
	}
	return nil
}

func runSkillInstall(cmd *cobra.Command, args []string) error {
	source := args[0]
	reg, err := skills.NewRegistry(skillsDir)
	if err != nil {
		return err
	}
	if err := reg.Install(source); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}
	fmt.Printf("Skill installed from %s\n", source)
	return reg.SaveIndex()
}

func runSkillRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	reg, err := skills.NewRegistry(skillsDir)
	if err != nil {
		return err
	}
	if err := reg.Remove(name); err != nil {
		return err
	}
	fmt.Printf("Removed skill %s\n", name)
	return reg.SaveIndex()
}

func runSkillRun(cmd *cobra.Command, args []string) error {
	name := args[0]
	reg, err := skills.NewRegistry(skillsDir)
	if err != nil {
		return err
	}
	runner := skills.NewRunner(reg, nil, nil)
	opts := skills.RunOptions{
		Verbose:      verbose,
		AutoConfirm:  autoConfirm,
		MaxSteps:     maxSteps,
		BudgetTokens: budget,
	}
	ctx := context.Background()
	result, err := runner.Run(ctx, name, opts)
	if err != nil {
		log.Fatal(err)
	}
	if result.Success {
		fmt.Printf("✅ Skill '%s' executed successfully (%d steps).\n", name, result.StepsExecuted)
	} else {
		fmt.Printf("❌ Skill failed: %v\n", result.Error)
	}
	return nil
}

func runSkillValidate(cmd *cobra.Command, args []string) error {
	path := args[0]
	skill, err := skills.ParseSkillFile(path)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	var errors []string
	if skill.Name == "" {
		errors = append(errors, "missing skill name (first heading)")
	}
	if len(skill.Steps) == 0 {
		errors = append(errors, "no steps found (look for numbered list under ## Steps)")
	}
	if _, ok := skill.Sections["Verification"]; !ok {
		errors = append(errors, "missing ## Verification section")
	}
	if _, ok := skill.Sections["Anti-Rationalization"]; !ok {
		errors = append(errors, "missing ## Anti-Rationalization section (recommended)")
	}

	if len(errors) > 0 {
		fmt.Printf("❌ Validation failed for %s:\n", path)
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		os.Exit(1)
	}
	fmt.Printf("✅ Skill '%s' is valid.\n", skill.Name)
	return nil
}
