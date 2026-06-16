// SPDX-License-Identifier: MIT
// Purpose: the .sin-code.yml schema. All fields are optional;
// the validator enforces "required" tags and the type system
// (yaml.v3) handles the rest.
//
// The schema mirrors the issue body. The Project.Type and
// Verify.Mode values are enums (string + Validate). The
// permissions entries are deliberately unstructured strings
// ("Bash:go test", "Read:**/*.go") to match the existing
// internal/permission pattern, which uses string matchers.
package compiler

// Config is the top-level .sin-code.yml document.
type Config struct {
	Version     int          `yaml:"version"`
	Project     Project      `yaml:"project"`
	Verify      Verify       `yaml:"verify"`
	Hooks       Hooks        `yaml:"hooks"`
	Permissions Permissions  `yaml:"permissions"`
	Loop        Loop         `yaml:"loop"` // v1.1 follow-up, ignored in v0
}

// Project is the project's metadata.
type Project struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // go|python|rust|node|polyglot
}

// Verify is the verify-gate configuration.
type Verify struct {
	Mode       string     `yaml:"mode"`       // minimal|standard|strict
	Predicates []Predicate `yaml:"predicates"`
}

// Predicate is a single verify check (e.g. "go test ./...").
type Predicate struct {
	Name     string `yaml:"name"`
	Command  string `yaml:"command"`
	Required bool   `yaml:"required"`
}

// Hooks is the pre-/post-tool hook configuration.
type Hooks struct {
	PreTool  []Hook `yaml:"pre-tool"`
	PostTool []Hook `yaml:"post-tool"`
}

// Hook is a single hook rule. The When / Run / Block / Message
// fields are deliberately unstructured strings — the hook
// engine in v1.1 will parse them.
type Hook struct {
	Name    string `yaml:"name"`
	When    string `yaml:"when"`
	Block   bool   `yaml:"block"`
	Run     string `yaml:"run"`
	Message string `yaml:"message"`
}

// Permissions is the allow/ask/deny policy. Each entry is a
// free-form string in the existing internal/permission format
// (e.g. "Bash:go test", "Read:**/*.go").
type Permissions struct {
	Allow []string `yaml:"allow"`
	Ask   []string `yaml:"ask"`
	Deny  []string `yaml:"deny"`
}

// Loop is the loop-engineering configuration (issue #155). v0
// parses it and re-emits it under a v1.1 top-level key, but the
// agents do not yet consume it. This is the migration path:
// v0 stores the loop config under .sin/loop.json; v1.1 wires
// the loop builder to read it.
type Loop struct {
	MaxTurns       int      `yaml:"max_turns"`
	MaxStopRejects int      `yaml:"max_stop_rejects"`
	StallThreshold int      `yaml:"stall_threshold"`
	MaxTokens      int      `yaml:"max_tokens"`
	VerifyMode     string   `yaml:"verify_mode"`
	DisableChecks  []string `yaml:"disable_checks"`
}

// SchemaVersion is the current schema version. Bumped when a
// breaking change to the YAML structure is made.
const SchemaVersion = 1
