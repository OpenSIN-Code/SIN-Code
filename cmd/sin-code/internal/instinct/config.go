// SPDX-License-Identifier: MIT
// Purpose: package-level configuration values. Mirrors the
// continuous-learning-v2 env-override pattern (SIN_INSTINCT_*) for the
// activation/evolve/reinforce/contradict/promote/ttl thresholds.
// Docs: config.doc.md
package instinct

import (
	"os"
	"strconv"
)

// Config holds tunable thresholds. Defaults match the ECC-derived
// constants; override via env for experimentation without recompiling.
type Config struct {
	ActivationThreshold float64
	EvolveThreshold     float64
	ReinforceStep       float64
	ContradictStep      float64
	PromotionThreshold  int
	PruneTTLDays        int
}

func DefaultConfig() Config {
	return Config{
		ActivationThreshold: ActivationThreshold,
		EvolveThreshold:     EvolveThreshold,
		ReinforceStep:       0.25,
		ContradictStep:      0.40,
		PromotionThreshold:  PromotionThreshold,
		PruneTTLDays:        30,
	}
}

// LoadConfig reads overrides from environment variables:
//
//	SIN_INSTINCT_ACTIVATION, SIN_INSTINCT_EVOLVE,
//	SIN_INSTINCT_REINFORCE, SIN_INSTINCT_CONTRADICT,
//	SIN_INSTINCT_PROMOTE_N, SIN_INSTINCT_TTL_DAYS
func LoadConfig() Config {
	c := DefaultConfig()
	c.ActivationThreshold = envFloat("SIN_INSTINCT_ACTIVATION", c.ActivationThreshold)
	c.EvolveThreshold = envFloat("SIN_INSTINCT_EVOLVE", c.EvolveThreshold)
	c.ReinforceStep = envFloat("SIN_INSTINCT_REINFORCE", c.ReinforceStep)
	c.ContradictStep = envFloat("SIN_INSTINCT_CONTRADICT", c.ContradictStep)
	c.PromotionThreshold = envInt("SIN_INSTINCT_PROMOTE_N", c.PromotionThreshold)
	c.PruneTTLDays = envInt("SIN_INSTINCT_TTL_DAYS", c.PruneTTLDays)
	return c
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
