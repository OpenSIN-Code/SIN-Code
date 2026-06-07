// SPDX-License-Identifier: MIT
// Purpose: small helpers shared across orchestrator files.
package orchestrator

import "time"

func timeNow() time.Time { return time.Now().UTC() }
