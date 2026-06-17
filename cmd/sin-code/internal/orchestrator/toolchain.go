// SPDX-License-Identifier: MIT
// Purpose: mandatory tool-chain registry per classified intent. The registry is
// read-only after init and is referenced by the planner to attach a ToolChain
// to every Plan. It introduces no runtime dependencies and is deterministic.
package orchestrator

// Canonical sin-code tool names used in intent tool chains.
const (
	ToolSecurityScan = "sin_security_scan"
	ToolSBOMGenerate = "sin_sbom_generate"
	ToolOracle       = "sin_oracle"
	ToolADW          = "sin_adw"
	ToolPoC          = "sin_poc"
	ToolMap          = "sin_map"
	ToolSCKG         = "sin_sckg"
	ToolTest         = "sin_test"
	ToolScout        = "sin_scout"
	ToolHarvest      = "sin_harvest"
	ToolRead         = "sin_read"
)

// DefaultToolChains maps each primary Intent to the tools that must, may, or
// must not be invoked when executing a plan for that intent. The registry is
// frozen at package init time; callers receive a copy of the slice values so
// accidental mutation cannot affect the canonical mapping.
var defaultToolChains = map[Intent]ToolChain{
	IntentSecurity: {
		Required: []string{ToolSecurityScan, ToolSBOMGenerate, ToolOracle},
	},
	IntentReview: {
		Required: []string{ToolADW, ToolOracle, ToolPoC},
	},
	IntentArchitecture: {
		Required: []string{ToolMap, ToolSCKG, ToolOracle},
	},
	IntentTest: {
		Required: []string{ToolTest, ToolOracle},
	},
	IntentCodebase: {
		Required: []string{ToolScout, ToolMap},
	},
	IntentDocs: {
		Required: []string{ToolHarvest, ToolRead},
	},
}

// ToolChainForIntent returns the canonical ToolChain for an intent. Unknown
// intents and the general-query intent receive an empty chain (no required,
// optional, or forbidden tools). The returned slices are copied so the caller
// can safely modify them.
func ToolChainForIntent(intent Intent) *ToolChain {
	chain, ok := defaultToolChains[intent]
	if !ok {
		return &ToolChain{}
	}
	return &ToolChain{
		Required:  append([]string(nil), chain.Required...),
		Optional:  append([]string(nil), chain.Optional...),
		Forbidden: append([]string(nil), chain.Forbidden...),
	}
}

// RequiredToolsForIntent returns only the required tool names for an intent.
// It is a convenience wrapper for callers that do not need optional/forbidden.
func RequiredToolsForIntent(intent Intent) []string {
	chain := ToolChainForIntent(intent)
	return chain.Required
}
