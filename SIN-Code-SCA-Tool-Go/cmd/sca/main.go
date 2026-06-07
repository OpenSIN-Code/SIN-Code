// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/internal/osv"
	"github.com/OpenSIN-Code/SIN-Code-SCA-Tool-Go/internal/scanner"
)

func main() {
	var (
		projectPath = flag.String("path", ".", "Path to project to scan")
		output      = flag.String("output", "", "Output file for JSON results (default: stdout)")
		timeout     = flag.Duration("timeout", 30*time.Second, "OSV API timeout")
	)
	flag.Parse()

	osvClient := osv.NewClient(*timeout)
	sca := scanner.New(osvClient)

	result, err := sca.ScanProject(*projectPath)
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
