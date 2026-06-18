// SPDX-License-Identifier: MIT
package agentloop
import ("fmt"; "strings")
func toLowerTrim(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func errUnknownMode(s string) error { return fmt.Errorf("agentloop: unknown compaction mode %q", s) }
func errUnknownTrigger(s string) error { return fmt.Errorf("agentloop: unknown compaction trigger %q", s) }
