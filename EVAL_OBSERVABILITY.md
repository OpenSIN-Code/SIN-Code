# 🎯 SIN-Code Evaluation & Observability System

## Übersicht

Dies ist eine vollständige Implementierung des **Evaluation & Observability Systems** für SIN-Code gemäß Issue #75. Das System besteht aus:

1. **OpenTelemetry Tracing** - Automatisches Capturing von Agent-Lifecycle-Events
2. **LLM-as-a-Judge** - Automatisierte Bewertung von Agent-Outputs
3. **Golden Datasets** - Deklarative Test-Suites mit kritischen Workflows
4. **Metrics & Reporting** - Quantitative Evaluierung und Regression-Schutz

## Dateistruktur

```
cmd/sin-code/
├── eval_cmd.go                    ← NEU: LLM-as-a-Judge CLI
├── trace_cmd.go                   ← NEU: Tracing-Konfiguration
└── internal/
    ├── trace/
    │   ├── provider.go            ← NEU: OTel Provider Setup
    │   └── hook_listener.go       ← NEU: Automatische Span-Erzeugung
    ├── dataset/
    │   ├── dataset.go             ← NEU: Golden Dataset Parser
    │   └── runner.go              ← NEU: Dataset-Execution-Engine
    └── eval/
        ├── judge.go               ← NEU: LLM-as-a-Judge Implementation
        └── metrics.go             ← NEU: Pass/Fail Metriken
evals/
└── critical.json                  ← NEU: Beispiel Golden Dataset (8 kritische Test-Cases)
```

## Installation & Setup

### 1. Dependencies hinzufügen

Die folgenden OpenTelemetry-Pakete müssen zu `go.mod` hinzugefügt werden:

```bash
cd /vercel/share/v0-project
go get go.opentelemetry.io/otel@latest
go get go.opentelemetry.io/otel/sdk@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest
go get go.opentelemetry.io/otel/exporters/stdout/stdouttrace@latest
```

Oder in der `go.mod` direkt eintragen:

```go
require (
    go.opentelemetry.io/otel v1.xx.x
    go.opentelemetry.io/otel/sdk v1.xx.x
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.xx.x
    go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.xx.x
)
```

### 2. Integration in main.go

In der `main.go` müssen die neuen Commands registriert werden (dies ist bereits in `eval_cmd.go` und `trace_cmd.go` vorbereitet):

```go
// Diese werden automatisch initialisiert wenn die *_cmd.go Dateien importiert werden
```

### 3. Hook-Listener Integration

Der Hook-Listener muss in der Agentloop-Initialisierung registriert werden:

```go
// In agentloop initialization:
trace.RegisterHookListener(hookManager)
```

## Verwendung

### Kommando 1: Evaluation Suite ausführen

```bash
# Mit Standard-Dataset (evals/critical.json)
sin eval

# Mit Custom-Dataset
sin eval --dataset evals/custom.json --output evals/custom_results.json

# Headless-Modus
sin eval --headless --timeout 600

# Alle Optionen
sin eval \
  --dataset evals/critical.json \
  --output evals/results.json \
  --headless \
  --timeout 300
```

**Output:**
- `evals/results.json` - Detaillierte Test-Ergebnisse
- `evals/metrics.json` - Aggregierte Metriken und Report
- Console: Human-readable Summary

### Kommando 2: Tracing aktivieren

```bash
# Stdout-Export (für local testing)
sin trace --exporter stdout

# OTLP-Export (für Langfuse/Jaeger/Phoenix)
sin trace --exporter otlp --endpoint localhost:4318

# Mit Langfuse (Production)
sin trace --exporter otlp --endpoint api.langfuse.com:443 --insecure=false

# Debug-Modus
sin trace --exporter stdout --debug
```

## Golden Dataset Format

Golden Datasets sind JSON-Dateien mit Test-Cases, die verschiedene Agent-Aspekte testen:

```json
{
  "name": "SIN-Code Critical Path Tests",
  "version": "1.0.0",
  "description": "...",
  "test_cases": [
    {
      "id": "test_id",
      "prompt": "User prompt for agent",
      "constraints": {
        "must_use_tools": ["tool1", "tool2"],
        "forbidden_tools": ["tool3"],
        "max_turns": 5,
        "max_tokens": 2000,
        "require_verify": true,
        "timeout_seconds": 300
      },
      "expected": {
        "contains_keywords": ["keyword1", "keyword2"],
        "avoids_keywords": ["bad_keyword"],
        "min_quality": 0.8,
        "custom_criteria": "Custom evaluation criteria"
      },
      "verify_cmd": "Command to verify output",
      "metadata": {
        "category": "category_name",
        "priority": "critical|high|medium|low"
      }
    }
  ]
}
```

### Test-Case Kategorien in `evals/critical.json`:

1. **plan_basic** - Einfache Coding-Aufgaben
2. **tool_integration** - Tool-Usage-Validierung
3. **constraint_enforcement** - Constraint-Einhaltung
4. **error_recovery** - Fehlerbehandlung
5. **memory_persistence** - Lesson-Anwendung
6. **verification_gate** - Verify-Command-Integration
7. **multi_step_workflow** - Komplexe Multi-Step-Workflows
8. **reasoning_quality** - Tiefe des Reasoning

## Architektur-Details

### 1. OpenTelemetry Provider (`internal/trace/provider.go`)

Initialisiert und konfiguriert den OTel Tracer mit verschiedenen Exportern:

```go
config := trace.ProviderConfig{
    ServiceName:    "sin-code",
    ServiceVersion: "1.0.0",
    ExporterType:   "stdout",  // oder "otlp"
    OTLPEndpoint:   "localhost:4318",
    Insecure:       true,
}

tp, err := trace.InitProvider(ctx, config)
defer trace.Shutdown(ctx, tp)
```

**Unterstützte Exporter:**
- **stdout** - Spans to console (local debugging)
- **otlp** - OpenTelemetry Protocol (Langfuse, Jaeger, Phoenix)

### 2. Hook Listener (`internal/trace/hook_listener.go`)

Konvertiert die 24 Lifecycle-Events in OTel Spans:

```
Session.Start
  ├─ Turn.Start
  │   ├─ Plan
  │   ├─ ToolCall (pro Tool)
  │   │   └─ ToolResult
  │   ├─ Verify
  │   │   └─ VerifyResult
  │   └─ Turn.End
  ├─ MemoryWrite
  └─ Session.End
```

Jeder Span wird automatisch mit Attributen versehen (Session-ID, Tool-Namen, etc.)

### 3. Golden Datasets & Runner

**Parser** (`internal/dataset/dataset.go`):
- Lädt JSON-Datasets
- Validiert Test-Cases
- Speichert Datasets

**Runner** (`internal/dataset/runner.go`):
- Führt alle Test-Cases eines Datasets aus
- Respektiert Constraints (max_turns, timeout, etc.)
- Speichert Ergebnisse in JSON

### 4. LLM-as-a-Judge (`internal/eval/judge.go`)

Bewertet Agent-Outputs gegen Kriterien:

```go
judge := eval.NewJudge("gpt-4")
result, err := judge.Evaluate(ctx, agentOutput, []string{
    "completeness",
    "correctness",
    "clarity",
}, 0.8) // min quality threshold

// result.Score: 0.0-1.0
// result.Passed: bool
// result.Feedback: string
```

**Evaluierungs-Metriken:**
- **Score** (0.0-1.0) - Gesamtqualität
- **Criteria** - Einzelne Kriterien-Scores
- **Passed** - Boolean basierend auf min_quality Threshold
- **Reasoning** - LLM-Begründung
- **Feedback** - Konstruktives Feedback

### 5. Metrics & Reporting (`internal/eval/metrics.go`)

Aggregiert Evaluierungs-Ergebnisse:

```go
report := eval.CalculateMetrics(datasetName, results)

// report.PassRate: 0.0-1.0
// report.AverageScore: 0.0-1.0
// report.CriteriaScores: map[criterion]score
// report.MinScore, MaxScore: range
// report.FailedTestCases: []FailedTestInfo
```

## Integration in den bestehenden Agent Loop

### Schritt 1: Hook-Manager Integration

```go
// In agentloop initialization:
hm := hooks.NewManager()
trace.RegisterHookListener(hm)
```

### Schritt 2: OpenTelemetry Provider Startup

```go
// In main.go init:
tp, err := trace.InitProvider(ctx, trace.ProviderConfig{
    ServiceName:    "sin-code",
    ExporterType:   "stdout",
})
defer trace.Shutdown(ctx, tp)
```

### Schritt 3: Mit bestehenden Hooks kombinieren

Die neuen Spans erweitern die bestehenden Hooks, interferieren aber nicht:

```go
// Bestehende Hooks funktionieren wie vorher
hookMgr.On(hooks.SessionStart, myExistingHandler)

// Neue Span-Generierung läuft parallel
trace.RegisterHookListener(hookMgr)
```

## Workflows

### Workflow 1: Lokales Debugging mit Traces

```bash
# Terminal 1: Tracer starten (stdout)
sin trace --exporter stdout

# Terminal 2: Agent ausführen
sin chat "Create a hello world program"

# Terminal 1: Sieht alle Spans in Echtzeit
```

### Workflow 2: Automatisierte Evaluierung

```bash
# Evaluation Suite ausführen
sin eval --dataset evals/critical.json

# Ergebnisse inspizieren
cat evals/results.json
cat evals/metrics.json

# JSON-Parsing für CI/CD
jq '.[] | select(.success == false)' evals/results.json
```

### Workflow 3: Regression-Schutz in CI/CD

```bash
# In .github/workflows/eval.yml oder ähnlich
- name: Run Evaluation Suite
  run: sin eval --dataset evals/critical.json --output evals/results.json
  
- name: Check Pass Rate
  run: |
    PASS_RATE=$(jq '.pass_rate * 100' evals/metrics.json)
    if (( $(echo "$PASS_RATE < 90" | bc -l) )); then
      echo "FAILED: Pass rate $PASS_RATE% below threshold"
      exit 1
    fi
```

### Workflow 4: Custom Dataset für neue Features

```bash
# Neue Test-Cases hinzufügen zu evals/custom.json
sin eval --dataset evals/custom.json

# Ergebnisse vergleichen
diff <(jq '.[] | .test_case_id' evals/critical.json) \
     <(jq '.[] | .test_case_id' evals/custom.json)
```

## Erweiterungen & Roadmap

### Geplant (M1):
- [ ] n8n CI Integration - Automatische Evaluierung bei jedem Commit
- [ ] Eval-Ergebnisse → Lessons - Automatische Fehler-Dokumentation

### Geplant (M2):
- [ ] Native Static Binary Integration
- [ ] WebUI für Trace-Visualisierung
- [ ] Langfuse/Jaeger Dashboard Integration

### Geplant (M3):
- [ ] Multi-Agent Orchestration Tracing
- [ ] A/B Testing Framework
- [ ] Automated Golden Dataset Generation

## Troubleshooting

### Problem: "failed to create exporter"

```
Solution: OpenTelemetry-Pakete sind nicht installiert
Run: go mod tidy
```

### Problem: "OTLP endpoint unreachable"

```
Solution: Endpoint ist nicht erreichbar
Check: Langfuse/Jaeger läuft auf dem richtigen Port
Die --insecure Flag bei localhost verwenden
```

### Problem: "dataset contains no test cases"

```
Solution: Golden Dataset JSON ist invalid
Validate: jq . evals/critical.json
Check: Alle Test-Cases haben ID und Prompt
```

## Referenzen

- OpenTelemetry Docs: https://opentelemetry.io/docs/
- Langfuse Integration: https://langfuse.com/docs/tracing
- Jaeger: https://www.jaegertracing.io/
- Arize Phoenix: https://phoenix.arize.com/
