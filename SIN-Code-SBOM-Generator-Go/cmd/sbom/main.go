package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/internal/generator"
	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/pkg/models"
)

func main() {
	var (
		scaResults   = flag.String("sca-results", "", "Path to SCA results JSON file")
		depsJSON     = flag.String("deps", "", "Path to dependencies JSON file")
		outputSPDX   = flag.String("output-spdx", "", "Output path for SPDX JSON")
		outputCycloneDX = flag.String("output-cyclonedx", "", "Output path for CycloneDX JSON")
		outputSummary   = flag.String("output-summary", "", "Output path for markdown summary")
		documentName = flag.String("name", "sbom", "Document name for SBOM")
	)
	flag.Parse()

	gen := generator.New("", "")

	var sbom *models.SBOM

	if *scaResults != "" {
		data, err := os.ReadFile(*scaResults)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error reading SCA results: %v\n", err)
			os.Exit(1)
		}
		var scaData map[string]interface{}
		if err := json.Unmarshal(data, &scaData); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error parsing SCA results: %v\n", err)
			os.Exit(1)
		}
		sbom = gen.GenerateFromSCAResults(scaData, *documentName)
	} else if *depsJSON != "" {
		data, err := os.ReadFile(*depsJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error reading dependencies: %v\n", err)
			os.Exit(1)
		}
		var deps []map[string]interface{}
		if err := json.Unmarshal(data, &deps); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error parsing dependencies: %v\n", err)
			os.Exit(1)
		}
		sbom = gen.GenerateFromRawDependencies(deps, *documentName)
	} else {
		fmt.Fprintln(os.Stderr, "❌ Error: either -sca-results or -deps must be specified")
		flag.Usage()
		os.Exit(1)
	}

	if *outputSPDX != "" {
		gen.ExportSPDX(sbom, *outputSPDX)
		fmt.Printf("✅ SPDX SBOM written to %s\n", *outputSPDX)
	}

	if *outputCycloneDX != "" {
		gen.ExportCycloneDX(sbom, *outputCycloneDX)
		fmt.Printf("✅ CycloneDX SBOM written to %s\n", *outputCycloneDX)
	}

	if *outputSummary != "" {
		summary := gen.ExportSummary(sbom)
		os.WriteFile(*outputSummary, []byte(summary), 0644)
		fmt.Printf("✅ Summary written to %s\n", *outputSummary)
	}

	if *outputSPDX == "" && *outputCycloneDX == "" && *outputSummary == "" {
		// Print SPDX to stdout by default
		fmt.Println(gen.ExportSPDX(sbom, ""))
	}
}
