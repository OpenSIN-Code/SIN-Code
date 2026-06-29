// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when testgen is refactored
package testgen

import (
	"fmt"
	"strings"
	"text/template"
)

var fallbackTemplate = template.Must(template.New("test").Funcs(template.FuncMap{
	"simpleType": simpleType,
	"zeroValue":  zeroValue,
	"joinArgs": func(args []Param) string {
		var parts []string
		for _, a := range args {
			parts = append(parts, a.Name)
		}
		return strings.Join(parts, ", ")
	},
	"joinArgsTyped": func(args []Param) string {
		var parts []string
		for _, a := range args {
			parts = append(parts, fmt.Sprintf("%s %s", a.Name, a.Type))
		}
		return strings.Join(parts, ", ")
	},
	"hasCasesFor": func(fn FuncInfo, cases map[string][]TestCase) bool {
		_, ok := cases[testKey(fn)]
		return ok
	},
	"casesFor": func(fn FuncInfo, cases map[string][]TestCase) []TestCase {
		return cases[testKey(fn)]
	},
	"renderCase": func(fn FuncInfo, tc TestCase) string {
		return renderCaseRow(fn, tc)
	},
	"joinReturns": func(rets []Param) string {
		var parts []string
		for _, r := range rets {
			parts = append(parts, r.Name)
		}
		return strings.Join(parts, ", ")
	},
	"returnTypes": func(rets []Param) string {
		var parts []string
		for _, r := range rets {
			parts = append(parts, r.Type)
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return "(" + strings.Join(parts, ", ") + ")"
	},
}).Parse(`{{ .Marker }}
package {{ .Package }}

import "testing"

{{ range .Funcs }}{{ $fn := . }}
func Test{{ if .IsMethod }}{{ .Receiver }}_{{ end }}{{ .Name }}(t *testing.T) {
	type args struct {
		{{ range .Args }}{{ .Name }} {{ .Type }}
		{{ end }}
	}
	tests := []struct {
		name string
		args args
		{{ range .Returns }}want{{ .Name }} {{ .Type }}
		{{ end }}wantErr bool
	}{
		{{ if hasCasesFor . $.Cases }}{{ range (casesFor . $.Cases) }}{{ renderCase $fn . }}
		{{ end }}{{ else }}
		{
			name: "basic case",
			{{ if .Args }}args: args{
				{{ range .Args }}{{ .Name }}: {{ zeroValue .Type }},
				{{ end }}
			},{{ end }}
			{{ range .Returns }}want{{ .Name }}: {{ zeroValue .Type }},
			{{ end }}wantErr: false,
		},
		{{ end }}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			{{ if .IsMethod }}{{ .Receiver }}{{ else }}{{ end }}{{ if .Returns }}got{{ range .Returns }}{{ .Name }}{{ end }} := {{ end }}{{ .Name }}({{ joinArgs .Args }})
			{{ if .HasError }}if (err != nil) != tt.wantErr {
				t.Errorf("{{ .Name }}() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			{{ end }}
			{{ range .Returns }}{{ if ne .Type "error" }}if got{{ .Name }} != tt.want{{ .Name }} {
				t.Errorf("{{ $fn.Name }}() = %v, want %v", got{{ .Name }}, tt.want{{ .Name }})
			}
			{{ end }}{{ end }}
		})
	}
}
{{ end }}
`))
