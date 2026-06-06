// SPDX-License-Identifier: MIT
// Purpose: Shared utilities for the sin-code unified binary (error printing,
// shared flags, output formatting). All subcommands import this package.
package internal

import (
	"fmt"
	"os"
)

// PrintError prints an error to stderr in a consistent format and exits with code 1.
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "sin-code: %v\n", err)
	os.Exit(1)
}
