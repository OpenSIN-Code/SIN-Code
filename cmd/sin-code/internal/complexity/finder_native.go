// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import "fmt"

// nativeReplacements maps dependency imports to platform/stdlib equivalents.
var nativeReplacements = map[string]string{
	"github.com/pkg/errors": "errors.New / fmt.Errorf with %w",
}

// analyzeNativeImports flags imports that duplicate stdlib/platform features.
func analyzeNativeImports(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	for _, imp := range pkg.imports {
		replacement, ok := nativeReplacements[imp.path]
		if !ok {
			continue
		}
		findings = append(findings, Finding{
			Tag:         TagNative,
			What:        fmt.Sprintf("Import %s duplicates stdlib/platform behavior", imp.path),
			Replacement: replacement,
			Path:        imp.fileRel,
			Line:        imp.line,
			EndLine:     imp.line,
			LineCount:   1,
			DepsRemoved: []string{imp.path},
			ApprovedBy:  markerForLine(markers, imp.fileRel, imp.line),
		})
	}
	return findings
}
