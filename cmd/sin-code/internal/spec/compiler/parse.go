// SPDX-License-Identifier: MIT
// Purpose: parse .sin-code.yml into a Config. Uses gopkg.in/yaml.v3,
// which is already in go.mod (transitive of other packages, no
// new dependency). Returns a ParseError with the field path on
// failure so the CLI can print a clear error message.
package compiler

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultFile is the canonical config filename. Operators
// commit this to the repo root.
const DefaultFile = ".sin-code.yml"

// ParseError carries a field path so the CLI can print
// "verify.mode: invalid value 'foo' (expected minimal|standard|strict)"
// instead of a generic yaml error.
type ParseError struct {
	Path    string // e.g. "verify.mode"
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Parse reads bytes as a Config. yaml.v3 is permissive (unknown
// fields are silently dropped), so forward-compat is preserved:
// a v2 spec with a new top-level key parses cleanly under v1
// (the unknown key is dropped, the rest is preserved).
func Parse(data []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, &ParseError{Path: "<root>", Message: err.Error()}
	}
	return &c, nil
}

// ParseFile reads .sin-code.yml from disk. A missing file is
// an error (the CLI distinguishes "no config" from "invalid
// config" — the former is a hint to run `sin-code compile-spec
// --init`, the latter is a hard fail).
func ParseFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{Path: path, Message: err.Error()}
	}
	return Parse(b)
}

// InitTemplate returns a minimal valid .sin-code.yml suitable for
// `sin-code compile-spec --init`. The template is conservative:
// it uses minimal verify mode and one example predicate, so the
// operator can see the format without being overwhelmed.
func InitTemplate(projectName, projectType string) []byte {
	if projectName == "" {
		projectName = "my-project"
	}
	if projectType == "" {
		projectType = "go"
	}
	c := Config{
		Version: SchemaVersion,
		Project: Project{Name: projectName, Type: projectType},
		Verify: Verify{
			Mode: "standard",
			Predicates: []Predicate{
				{Name: "builds", Command: "go build ./...", Required: true},
			},
		},
	}
	b, _ := yaml.Marshal(c)
	return b
}

// errEmpty is the sentinel for "config file was empty". An empty
// file is a Parse failure (not a no-op) because the operator
// almost certainly meant to write something.
var errEmpty = errors.New("empty config")
