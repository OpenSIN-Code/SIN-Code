package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-SAST-Tool/internal/engine"
	"github.com/OpenSIN-Code/SIN-Code-SAST-Tool/pkg/models"
	"github.com/OpenSIN-Code/SIN-Code-SAST-Tool/pkg/rules"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"

	languages  string
	minSeverity string
	rulesFilter string
	exclude    string
	output     string
	verbose    bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "sin-sast",
		Short: "SIN-Code SAST - Static Application Security Testing",
		Long: `🔍 SIN-Code SAST - Static Application Security Testing

Detects 20+ vulnerability categories across 10+ languages:
  • SQL Injection, Command Injection, XSS
  • Hardcoded Secrets & API Keys
  • Insecure Cryptography (MD5, SHA1, weak random)
  • Path Traversal, SSRF, Open Redirect
  • Insecure Deserialization, Debug Mode
  • Race Conditions, Insecure TLS, and more

Perfect for scanning AnythingLLM-based projects like OpenAfD-Chat!`,
		Version: version,
	}

	scanCmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Run SAST scan on a codebase",
		Args:  cobra.ExactArgs(1),
		RunE:  runScan,
	}
	scanCmd.Flags().StringVar(&languages, "languages", "", "Comma-separated languages to scan (python,go,js,ts,java,php,ruby)")
	scanCmd.Flags().StringVar(&minSeverity, "severity", "low", "Minimum severity (low, medium, high, critical)")
	scanCmd.Flags().StringVar(&rulesFilter, "rules", "", "Comma-separated rule IDs to run (e.g., SAST-001,SAST-002)")
	scanCmd.Flags().StringVar(&exclude, "exclude", "", "Comma-separated patterns to exclude")
	scanCmd.Flags().StringVarP(&output, "output", "o", "text", "Output format (text, json, sarif)")
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
		Path:     path,
		Severity: minSeverity,
		Verbose:  verbose,
	}

	if languages != "" {
		opts.Languages = splitAndTrim(languages)
	}
	if rulesFilter != "" {
		opts.Rules = splitAndTrim(rulesFilter)
	}
	if exclude != "" {
		opts.Exclude = splitAndTrim(exclude)
	}

	allRules := rules.AllRules()
	if len(opts.Languages) > 0 {
		allRules = rules.FilterRulesByLanguage(allRules, opts.Languages)
	}
	allRules = rules.FilterRulesBySeverity(allRules, opts.Severity)

	if opts.Verbose {
		fmt.Printf("🔍 SAST Scan Configuration:\n")
		fmt.Printf("   Path: %s\n", path)
		fmt.Printf("   Languages: %v\n", opts.Languages)
		fmt.Printf("   Min Severity: %s\n", opts.Severity)
		fmt.Printf("   Rules: %d\n", len(allRules))
		fmt.Println()
	}

	eng := engine.NewEngine(allRules, opts.Exclude)
	result, err := eng.Scan(path)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	switch output {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(result)
	case "sarif":
		fmt.Println("SARIF output not yet implemented. Use JSON.")
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
	fmt.Println("\n📋 SAST Detection Rules")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("%-10s %-30s %-12s %-10s %s\n", "ID", "Name", "Severity", "CWE", "Languages")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range allRules {
		fmt.Printf("%-10s %-30s %-12s %-10s %s\n", r.ID, r.Name, r.Severity, r.CWE, strings.Join(r.Languages, ", "))
	}
	fmt.Printf("\nTotal: %d rules\n\n", len(allRules))
}

func printTextResult(result *models.SASTResult) {
	bold := color.New(color.Bold)
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow)
	green := color.New(color.FgGreen)

	fmt.Println()
	bold.Println("🔍 SAST Scan Results")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Path: %s\n", result.Path)
	fmt.Printf("Duration: %.2fs\n", result.ScanDurationSeconds)
	fmt.Printf("Timestamp: %s\n", result.Timestamp)
	fmt.Println()

	fmt.Println("📊 Summary")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Files Scanned:  %d\n", result.Summary.FilesScanned)
	fmt.Printf("Lines Scanned:  %d\n", result.Summary.LinesScanned)
	fmt.Printf("Rules Triggered: %d\n", result.Summary.RulesTriggered)
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

	if len(result.Findings) > 0 {
		bold.Println("🚨 Findings")
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
			fmt.Printf("  %s\n", f.RuleName)
			fmt.Printf("  File: %s:%d\n", f.File, f.Line)
			fmt.Printf("  Match: %s\n", f.Match)
			fmt.Printf("  CWE: %s | OWASP: %s\n", f.CWE, f.OWASP)
			fmt.Printf("  Remediation: %s\n", f.Remediation)
			if i < len(result.Findings)-1 {
				fmt.Println()
			}
		}
	} else {
		green.Println("✅ No security findings detected!")
	}
	fmt.Println()
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
