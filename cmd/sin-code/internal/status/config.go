// SPDX-License-Identifier: MIT
// Purpose: configuration for the `sin-code status` readiness snapshot.
package status

import "time"

// Config controls what the status report collects and how it is rendered.
type Config struct {
	Workspace string    // directory used for debt scanning and workspace filtering
	Since     time.Time // optional ledger time filter (zero = all time)
	Markdown  bool      // render markdown (default output)
	JSON      bool      // render JSON
	OutPath   string    // when non-empty, write report to this file instead of stdout
}
