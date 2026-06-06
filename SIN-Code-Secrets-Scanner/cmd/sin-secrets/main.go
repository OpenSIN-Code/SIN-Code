package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-Secrets-Scanner/internal/engine"
	"github.com/OpenSIN-Code/SIN-Code-Secrets-Scanner/pkg/models"
	"github.com/OpenSIN-Code/SIN-Code-Secrets-Scanner/pkg/rules"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"

	secretTypes    string
	minSeverity    string
	exclude        string
	scanGitHistory bool
	checkEntropy   bool
	output         string
	verbose        bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "sin-secrets",
		Short: "SIN-Code Secrets Scanner - Detect leaked secrets in code",
		Long: `🔐 SIN-Code Secrets Scanner - Detect leaked secrets in code

Detects 22+ secret types across all files:
  • API Keys: OpenAI, AWS, GitHub, Google, Stripe, Twilio, SendGrid, Mailgun, Heroku
  • Tokens: Slack, JWT, Bearer, Discord, GitHub PAT
  • Credentials: Database passwords, private keys, certificates
  • Config Files: .env, Docker config, Kubernetes secrets, Terraform state

Features:
  • Entropy-based filtering to reduce false positives
  • Git history scanning (planned)
  • Severity classification (Critical/High/Medium/Low)
  • JSON and text output formats

Perfect for CI/CD pipelines and pre-commit hooks!`,
		Version: version,
	}

	scanCmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan a directory or file for secrets",
		Args:  cobra.ExactArgs(1),
		RunE:  runScan,
	}
	scanCmd.Flags().StringVar(&secretTypes, "types", "", "Comma-separated secret types to scan (api-key,token,password,private-key,certificate,config-file)")
	scanCmd.Flags().StringVar(&minSeverity, "severity", "low", "Minimum severity (low, medium, high, critical)")
	scanCmd.Flags().StringVar(&exclude, "exclude", "", "Comma-separated patterns to exclude")
	scanCmd.Flags().BoolVar(&scanGitHistory, "scan-git-history", false, "Scan Git history for secrets (planned)")
	scanCmd.Flags().BoolVar(&checkEntropy, "check-entropy", true, "Use entropy filtering to reduce false positives")
	scanCmd.Flags().StringVarP(&output, "output", "o", "text", "Output format (text, json)")
	scanCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	listRulesCmd := &cobra.Command{
		Use:   "list-rules",
		Short: "List all available detection rules",
		Run:   runListRules,
	}

	rootCmd.AddCommand(scanCmd, listRulesCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runScan(cmd *cobra.Command, args []string) error {
	path := args[0]

	opts := models.ScanOptions{
		Path:           path,
		Severity:       minSeverity,
		ScanGitHistory: scanGitHistory,
		EntropyCheck:   checkEntropy,
		Verbose:        verbose,
	}

	if secretTypes != "" {
		opts.SecretTypes = splitAndTrim(secretTypes)
	}
	if exclude != "" {
		opts.Exclude = splitAndTrim(exclude)
	}

	allRules := rules.AllRules()
	if len(opts.SecretTypes) > 0 {
		allRules = rules.FilterRulesByType(allRules, opts.SecretTypes)
	}
	allRules = rules.FilterRulesBySeverity(allRules, opts.Severity)

	if opts.Verbose {
		fmt.Printf("🔐 Secrets Scan Configuration:\n")
		fmt.Printf("   Path: %s\n", path)
		fmt.Printf("   Secret Types: %v\n", opts.SecretTypes)
		fmt.Printf("   Min Severity: %s\n", opts.Severity)
		fmt.Printf("   Entropy Check: %v\n", opts.EntropyCheck)
		fmt.Printf("   Rules: %d\n", len(allRules))
		fmt.Println()
	}

	eng := engine.NewEngine(allRules, opts.Exclude, opts.EntropyCheck)
	result, err := eng.Scan(path)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	switch output {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(result)
	default:
		printTextResult(result)
	}

	if result.Status == "failed" {
		os.Exit(1)
	}
	return nil
}

func runListRules(cmd *cobra.Command, args []string) {
	allRules := rules.AllRules()
	fmt.Println("\n📋 Secrets Detection Rules")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-12s %-30s %-12s %-12s %s\n", "ID", "Name", "Severity", "Type", "Confidence")
	fmt.Println(strings.Repeat("-", 100))
	for _, r := range allRules {
		fmt.Printf("%-12s %-30s %-12s %-12s %s\n", r.ID, r.Name, r.Severity, r.SecretType, r.Confidence)
	}
	fmt.Printf("\nTotal: %d rules\n\n", len(allRules))
}

func printTextResult(result *models.SecretsResult) {
	bold := color.New(color.Bold)
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow)
	green := color.New(color.FgGreen)

	fmt.Println()
	bold.Println("🔐 Secrets Scan Results")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Path: %s\n", result.Path)
	fmt.Printf("Duration: %.2fs\n", result.ScanDurationSeconds)
	fmt.Printf("Timestamp: %s\n", result.Timestamp)
	fmt.Println()

	fmt.Println("📊 Summary")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Files Scanned:  %d\n", result.Summary.FilesScanned)
	fmt.Printf("Secrets Found:  %d\n", result.Summary.SecretsFound)
	fmt.Println()

	severityColor := green
	if result.Status == "warning" {
		severityColor = yellow
	} else if result.Status == "failed" {
		severityColor = red
	}
	severityColor.Printf("Status: %s\n", strings.ToUpper(result.Status))
	fmt.Println()

	fmt.Printf("  Critical: %d  High: %d  Medium: %d  Low: %d\n",
		result.Summary.Critical, result.Summary.High,
		result.Summary.Medium, result.Summary.Low)
	fmt.Println()

	if result.Summary.SecretsFound > 0 {
		bold.Println("🔴 Leaked Secrets")
		fmt.Println(strings.Repeat("-", 80))
		for i, f := range result.Findings {
			sevColor := color.New(color.FgWhite)
			switch f.Severity {
			case "critical":
				sevColor = red
			case "high":
				sevColor = color.New(color.FgRed)
			case "medium":
				sevColor = yellow
			case "low":
				sevColor = color.New(color.FgWhite)
			}
			sevColor.Printf("  [%s] %s\n", f.Severity, f.RuleID)
			fmt.Printf("  %s (%s)\n", f.RuleName, f.SecretType)
			fmt.Printf("  File: %s:%d\n", f.File, f.Line)
			// Mask the secret in output
			masked := maskSecret(f.Match)
			fmt.Printf("  Match: %s\n", masked)
			if f.Entropy > 0 {
				fmt.Printf("  Entropy: %.2f\n", f.Entropy)
			}
			fmt.Printf("  Remediation: %s\n", f.Remediation)
			if i < len(result.Findings)-1 {
				fmt.Println()
			}
		}
	} else {
		green.Println("✅ No secrets detected!")
	}
	fmt.Println()
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

func splitAndTrim(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
