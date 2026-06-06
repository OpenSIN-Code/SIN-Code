package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Container-Tool-Go/internal/scanner"
)

func main() {
	var (
		image          = flag.String("image", "", "Docker image to scan")
		path           = flag.String("path", "", "Filesystem path to scan (alternative to image)")
		failOn         = flag.String("fail-on", "high", "Severity threshold for failure (critical, high, medium, low)")
		scanType       = flag.String("scan-type", "all", "Scan type (vuln, config, secret, all)")
		dockerfile     = flag.String("dockerfile", "", "Path to Dockerfile for additional auditing")
		includeSBOM    = flag.Bool("sbom", false, "Generate SBOM (not yet implemented)")
		output         = flag.String("output", "", "Output file for JSON results (default: stdout)")
		trivyPath      = flag.String("trivy-path", "trivy", "Path to Trivy binary")
		timeout        = flag.Duration("timeout", 10*time.Minute, "Scan timeout")
	)
	flag.Parse()

	if *image == "" && *path == "" {
		fmt.Fprintln(os.Stderr, "❌ Error: either -image or -path must be specified")
		flag.Usage()
		os.Exit(1)
	}

	s := scanner.NewContainerScanner(*trivyPath, *timeout)

	var result interface{}
	var err error

	if *image != "" {
		result, err = s.ScanImage(*image, *failOn, *scanType, *dockerfile, *includeSBOM)
	} else {
		result, err = s.ScanFilesystem(*path, *failOn, *scanType, *dockerfile)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Scan failed: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ JSON marshal failed: %v\n", err)
		os.Exit(1)
	}

	if *output != "" {
		if err := os.WriteFile(*output, jsonData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Write output failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Results written to %s\n", *output)
	} else {
		fmt.Println(string(jsonData))
	}
}
