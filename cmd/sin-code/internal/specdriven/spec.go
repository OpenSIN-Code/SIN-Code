// SPDX-License-Identifier: MIT
// Purpose: specdriven — Spec-Driven Development pipeline (issue #480).
// EARS (Easy Approach to Requirements Syntax) parser → Architecture
// generator → Code scaffolder. Three-stage deterministic pipeline:
// Spec → Architecture → Implementation. No LLM required.
package specdriven

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// EARS (Easy Approach to Requirements Syntax) format:
//   "The [system] shall [response]"                    — Ubiquitous
//   "When [trigger], the [system] shall [response]"    — Event-driven
//   "While [state], the [system] shall [response]"     — State-driven
//   "If [condition], then the [system] shall [response]" — Optional

// Requirement is a single parsed EARS requirement.
type Requirement struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "when" | "while" | "if" | "the"
	Subject   string `json:"subject"`
	Response  string `json:"response"`
	Condition string `json:"condition"`
	Raw       string `json:"raw"`
}

// Spec is a parsed specification document.
type Spec struct {
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Requirements []Requirement `json:"requirements"`
}

// ParseEARS parses EARS-format requirements from text. Lines starting
// with '#' are treated as comments and skipped. Blank lines are
// skipped. Non-EARS lines (those that don't start with a recognised
// keyword) are silently ignored — they may be prose between requirements.
func ParseEARS(text string) ([]Requirement, error) {
	var reqs []Requirement
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		req, err := parseEARSLine(line, i+1)
		if err != nil {
			return nil, err
		}
		if req != nil {
			reqs = append(reqs, *req)
		}
	}
	return reqs, nil
}

// parseEARSLine parses a single line as an EARS requirement. Returns
// nil (no error) if the line does not match any EARS keyword — the
// caller treats it as prose. Returns an error if the line starts with
// a keyword but is malformed.
func parseEARSLine(line string, lineNum int) (*Requirement, error) {
	lower := strings.ToLower(line)

	var reqType, subject, response, condition string

	switch {
	case strings.HasPrefix(lower, "when "):
		reqType = "when"
		rest := line[5:]
		idx := strings.Index(strings.ToLower(rest), ", the ")
		if idx < 0 {
			return nil, fmt.Errorf("line %d: EARS 'when' requires ', the <system> shall <response>'", lineNum)
		}
		condition = strings.TrimSpace(rest[:idx])
		after := rest[idx+6:]
		subject, response = parseShall(after)

	case strings.HasPrefix(lower, "while "):
		reqType = "while"
		rest := line[6:]
		idx := strings.Index(strings.ToLower(rest), ", the ")
		if idx < 0 {
			return nil, fmt.Errorf("line %d: EARS 'while' requires ', the <system> shall <response>'", lineNum)
		}
		condition = strings.TrimSpace(rest[:idx])
		after := rest[idx+6:]
		subject, response = parseShall(after)

	case strings.HasPrefix(lower, "if "):
		reqType = "if"
		rest := line[3:]
		idx := strings.Index(strings.ToLower(rest), ", then the ")
		if idx < 0 {
			return nil, fmt.Errorf("line %d: EARS 'if' requires ', then the <system> shall <response>'", lineNum)
		}
		condition = strings.TrimSpace(rest[:idx])
		after := rest[idx+11:]
		subject, response = parseShall(after)

	case strings.HasPrefix(lower, "the "):
		reqType = "the"
		rest := line[4:]
		subject, response = parseShall(rest)
		condition = "always"

	default:
		return nil, nil // not an EARS line, skip
	}

	if subject == "" || response == "" {
		return nil, fmt.Errorf("line %d: missing 'shall' clause", lineNum)
	}

	return &Requirement{
		ID:        fmt.Sprintf("REQ-%03d", lineNum),
		Type:      reqType,
		Subject:   subject,
		Response:  response,
		Condition: condition,
		Raw:       line,
	}, nil
}

// parseShall extracts the subject and response from text containing
// "shall". Returns empty strings if "shall" is not found.
func parseShall(s string) (subject, response string) {
	idx := strings.Index(strings.ToLower(s), " shall ")
	if idx < 0 {
		return "", ""
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+7:])
}

// LoadSpec reads a file and parses it as an EARS specification. The
// first comment line starting with "# Title:" sets Spec.Title; the
// first comment line starting with "# Description:" sets
// Spec.Description.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("specdriven: read %s: %w", path, err)
	}
	text := string(data)
	spec := &Spec{}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# Title:") {
			spec.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# Title:"))
		}
		if strings.HasPrefix(trimmed, "# Description:") {
			spec.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "# Description:"))
		}
	}

	reqs, err := ParseEARS(text)
	if err != nil {
		return nil, err
	}
	spec.Requirements = reqs
	return spec, nil
}

// ============================================================================
// Architecture generation (deterministic — no LLM)
// ============================================================================

// Architecture represents generated architecture from a spec.
type Architecture struct {
	Spec       *Spec        `json:"spec"`
	Components []Component  `json:"components"`
	DataModels []DataModel  `json:"data_models"`
	Interfaces []Interface  `json:"interfaces"`
}

// Component is an architectural component derived from grouping
// requirements by subject.
type Component struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Responsibilities []string `json:"responsibilities"`
}

// DataModel is a data structure extracted from requirements.
type DataModel struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

// Field is a single field in a DataModel.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Interface is a Go interface generated from a component.
type Interface struct {
	Name    string   `json:"name"`
	Methods []Method `json:"methods"`
}

// Method is a single method in an Interface.
type Method struct {
	Name    string `json:"name"`
	Params  string `json:"params"`
	Returns string `json:"returns"`
}

// GenerateArchitecture derives an Architecture from a Spec by grouping
// requirements by subject. Each unique subject becomes a Component;
// each requirement's response becomes a responsibility. The result is
// deterministic — sorting by subject name ensures byte-stability.
func GenerateArchitecture(spec *Spec) *Architecture {
	subjectMap := make(map[string][]Requirement)
	for _, req := range spec.Requirements {
		subjectMap[req.Subject] = append(subjectMap[req.Subject], req)
	}

	subjects := make([]string, 0, len(subjectMap))
	for s := range subjectMap {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)

	var components []Component
	var interfaces []Interface
	for _, subj := range subjects {
		reqs := subjectMap[subj]
		var responsibilities []string
		for _, r := range reqs {
			responsibilities = append(responsibilities, r.Response)
		}
		compName := componentName(subj)
		components = append(components, Component{
			Name:             compName,
			Description:      fmt.Sprintf("Handles %s responsibilities (%d requirements)", subj, len(reqs)),
			Responsibilities: responsibilities,
		})

		var methods []Method
		for _, r := range reqs {
			methods = append(methods, Method{
				Name:    methodName(r.Response),
				Params:  "",
				Returns: "error",
			})
		}
		interfaces = append(interfaces, Interface{
			Name:    compName + "er",
			Methods: methods,
		})
	}

	return &Architecture{
		Spec:       spec,
		Components: components,
		Interfaces: interfaces,
	}
}

// componentName converts a subject string (e.g. "the system", "the
// auth service") into a PascalCase component name (e.g. "System",
// "AuthService").
func componentName(subject string) string {
	s := strings.TrimSpace(subject)
	s = strings.TrimPrefix(strings.ToLower(s), "the ")
	s = strings.TrimSpace(s)
	words := strings.Fields(s)
	var b strings.Builder
	for _, w := range words {
		if len(w) == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		if len(w) > 1 {
			b.WriteString(w[1:])
		}
	}
	if b.Len() == 0 {
		return "Component"
	}
	return b.String()
}

// methodName converts a response string (e.g. "persist the form") into
// a PascalCase method name (e.g. "PersistTheForm").
func methodName(response string) string {
	words := strings.Fields(strings.TrimSpace(response))
	var b strings.Builder
	for _, w := range words {
		if len(w) == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		if len(w) > 1 {
			b.WriteString(w[1:])
		}
	}
	if b.Len() == 0 {
		return "Do"
	}
	return b.String()
}

// ============================================================================
// Code scaffolding
// ============================================================================

const interfaceTemplate = `// Code generated by sin-code spec code (issue #480). DO NOT EDIT.

package {{.Package}}

{{range .Interfaces}}
// {{.Name}} is the interface generated from the spec for {{.Name}}.
type {{.Name}} interface {
{{- range .Methods}}
	{{.Name}}({{.Params}}) {{.Returns}}
{{- end}}
}
{{end}}
`

// GenerateCode produces Go interface stubs from an Architecture. The
// output is a single Go file containing one interface per component.
func GenerateCode(arch *Architecture, pkg string) (string, error) {
	tmpl, err := template.New("interface").Parse(interfaceTemplate)
	if err != nil {
		return "", fmt.Errorf("specdriven: parse template: %w", err)
	}
	if pkg == "" {
		pkg = "generated"
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, struct {
		Package    string
		Interfaces []Interface
	}{
		Package:    pkg,
		Interfaces: arch.Interfaces,
	}); err != nil {
		return "", fmt.Errorf("specdriven: execute template: %w", err)
	}
	return b.String(), nil
}

// WriteCode writes generated Go source to a directory. The file is
// named `spec_generated.go` and placed inside `dir`.
func WriteCode(arch *Architecture, pkg, dir string) (string, error) {
	src, err := GenerateCode(arch, pkg)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return src, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("specdriven: mkdir %s: %w", dir, err)
	}
	outPath := filepath.Join(dir, "spec_generated.go")
	if err := os.WriteFile(outPath, []byte(src), 0o644); err != nil {
		return "", fmt.Errorf("specdriven: write %s: %w", outPath, err)
	}
	return outPath, nil
}
