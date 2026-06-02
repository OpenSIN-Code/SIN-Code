# PLAN: Unit Tests für alle 7 SIN-Code Tools

**Ziel:** Jedes Tool bekommt umfassende Unit Tests, damit Code-Änderungen verifiziert werden können.

**Status:** ✅ DONE (2026-06-02)
**Aufwand:** ~20 Minuten pro Tool = ~2.5 Stunden total

---

## Test-Framework

Standard Go testing package (`testing`).

```go
func TestDiscoverJSON(t *testing.T) {
    // Test code
}
```

Run mit: `go test ./...`

---

## Test-Kategorien pro Tool

### 1. Discover-Tool
- [x] TestDiscoverJSON — Valide Pfad → JSON Array
- [x] TestDiscoverSortByRelevance — Sortierung nach Score
- [x] TestDiscoverSortByName — Alphabetische Sortierung
- [x] TestDiscoverMaxResults — Truncation
- [x] TestDiscoverNonExistentPath — JSON Error (nicht plain text)
- [x] TestDiscoverTotalMatches — Korrekte Anzahl vor Truncation

### 2. Execute-Tool
- [x] TestExecuteSimple — `echo hello` → output
- [x] TestExecuteTimeout — Blockierender Command → Timeout Error
- [x] TestExecuteSecretRedaction — `curl -H "Authorization: Bearer abc123" ...` → REDACTED
- [x] TestExecuteEnvVarRedaction — `$MY_SECRET` → REDACTED
- [x] TestExecuteErrorField — Failing command → `error: "..."`
- [x] TestExecuteDurationMs — Float64 mit Millisekunden

### 3. Map-Tool
- [x] TestMapBasic — Valide Pfad → Module Map
- [x] TestMapNonExistentPath — JSON Error
- [x] TestMapModuleEdges — Module-level Dependencies
- [x] TestMapDependencyGraph — Vollständiger Graph

### 4. Grasp-Tool
- [x] TestGraspFile — Valide Datei → Struktur
- [x] TestGraspFileAlias — `file` statt `file_path`
- [x] TestGraspNonExistentFile — JSON Error
- [x] TestGraspRelatedFiles — Related Files Output

### 5. Scout-Tool
- [x] TestScoutRegex — `func.*main` → Matches
- [x] TestScoutVenvExclusion — `.venv` und `venv` ignoriert
- [x] TestScoutSummary — `summary` Field vorhanden
- [x] TestScoutDurationMs — Float64 Millisekunden

### 6. Harvest-Tool
- [x] TestHarvestSuccess — 200 → status: 200, body: "..."
- [x] TestHarvest404 — 404 → status: 404, kein error
- [x] TestHarvestInvalidURL — JSON Error
- [x] TestHarvestStatusField — `status` Field vorhanden

### 7. Orchestrate-Tool
- [x] TestOrchestrateAdd — Task hinzufügen
- [x] TestOrchestrateList — Alle Tasks
- [x] TestOrchestrateIdShorthand — `-id` statt `-task_id`
- [x] TestOrchestratePlanField — `plan` Field vorhanden
- [x] TestOrchestrateRollbackField — `rollback` Field vorhanden
- [x] TestOrchestrateDependencies — `dependencies` Array parsing
- [x] TestOrchestrateTags — `tags` Array parsing
- [x] TestOrchestrateInvalidAction — JSON Error

---

## Test-Datei Struktur

```
pkg/tools/
├── discover_test.go
├── execute_test.go
├── map_test.go
├── grasp_test.go
├── scout_test.go
├── harvest_test.go
└── orchestrate_test.go
```

---

## Beispiel Test

```go
// pkg/tools/discover_test.go
package tools

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestDiscoverJSON(t *testing.T) {
    // Create temp directory with test files
    tmpDir := t.TempDir()
    testFile := filepath.Join(tmpDir, "test.py")
    if err := os.WriteFile(testFile, []byte("print('hello')"), 0644); err != nil {
        t.Fatal(err)
    }
    
    // Run discover
    input := DiscoverInput{
        Path:       tmpDir,
        Pattern:    "**/*.py",
        SortBy:     "relevance",
        MaxResults: 10,
        Format:     "json",
    }
    
    result, err := RunDiscover(input)
    if err != nil {
        t.Fatal(err)
    }
    
    // Parse result
    var files []FileMetadata
    if err := json.Unmarshal([]byte(result.JSON), &files); err != nil {
        t.Fatalf("Invalid JSON: %v", err)
    }
    
    if len(files) != 1 {
        t.Errorf("Expected 1 file, got %d", len(files))
    }
    
    if files[0].Path != testFile {
        t.Errorf("Expected %s, got %s", testFile, files[0].Path)
    }
}
```

---

## Coverage Goal

- **Minimum:** 70% coverage
- **Target:** 85% coverage
- **Stretch:** 95% coverage

---

## Umsetzung

1. Erstelle `pkg/tools/discover_test.go`
2. Erstelle `pkg/tools/execute_test.go`
3. Erstelle `pkg/tools/map_test.go`
4. Erstelle `pkg/tools/grasp_test.go`
5. Erstelle `pkg/tools/scout_test.go`
6. Erstelle `pkg/tools/harvest_test.go`
7. Erstelle `pkg/tools/orchestrate_test.go`
8. Run `go test ./...` für jedes Tool
9. Run `go test -cover ./...` für Coverage-Report
10. Füge Coverage-Threshold zu CI hinzu

---

## Geschätzte Zeit

- Pro Tool: 20 Minuten (2-3 Tests + Setup)
- Total: ~2.5 Stunden
- Plus Debugging: +30 Minuten
- **Total: ~3 Stunden**
