// SPDX-License-Identifier: MIT
// Purpose: small helpers shared across orchestrator files.
package orchestrator

import "time"

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func timeNow() time.Time { return time.Now().UTC() }
