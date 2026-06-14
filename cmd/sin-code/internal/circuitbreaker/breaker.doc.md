# `internal/circuitbreaker` — Stdlib-Only Circuit Breaker

## Was

Native Go-Circuit-Breaker-Paket ohne Drittanbieter-Abhängigkeiten
([sony/gobreaker](https://github.com/sony/gobreaker),
[failsafe-go](https://github.com/failsafe-go/failsafe-go),
[hystrix-go](https://github.com/afex/hystrix-go) sind explizit
*explizit verboten* — siehe Mandate M2 / Mandate M7). Implementiert
das klassische Drei-Zustands-FSM
**Closed → Open → HalfOpen → (Closed | Open)** mit konfigurierbaren
Schwellenwerten.

## Warum kein vendoring

**Mandate M2** erzwingt `CGO_ENABLED=0` + stdlib-only; **Mandate M7**
erfordert `-race`-Sauberkeit. Beide Mandate schließen
CGo-Bindings (`sony/gobreaker` ist zwar Go-pure, aber bringt eine
zusätzliche transitive `golang.org/x/sync` mit; `hystrix-go` hat
veraltete glog-Abhängigkeiten; alle drei erhöhen die SBOM-Footprint).
Außerdem: die Breakermechanik ist < 200 LOC — selbst zu schreiben
ist schneller als zu korrigieren, was eine externe API uns antut, wenn
wir eine Schwelle anders timen wollen.

## Files

| File | Lines | Verantwortung |
|------|-------|---------------|
| `breaker.go`        | ~290 | FSM (`Closed`/`Open`/`HalfOpen`), `Execute(fn)`, `IsAllowed()`, `RecordSuccess/Failure`, `Stats()`, panic-Recovery |
| `integration.go`    | ~95  | `RoundTripper(inner, breaker)` — wickelt `net/http.RoundTripper`; 5xx + Transport-Errors = Failure, 4xx = Success |
| `breaker_test.go`   | ~390 | 9 Tests, alle `-race`-clean (closed/opening/rejecting/half-open probe ok / half-open probe fail / concurrent stress / panic-recovery / transport-5xx / transport-4xx) |

## Public API

```go
type State int32  // StateClosed | StateOpen | StateHalfOpen
type Config struct {
    FailureThreshold int           // default 5
    OpenDuration     time.Duration // default 10s
    HalfOpenProbes   int           // default 1
    SuccessThreshold int           // default 1
    Now              func() time.Time // default time.Now (overridable für Tests)
    Name             string        // für Logs / Metrics-Labels
}
type Breaker struct { /* unexported */ }
func New(*Config) *Breaker
func (b *Breaker) Execute(fn func() error) error  // REQUIRED API
func (b *Breaker) IsAllowed() error               // admits + halfInFlight++; Callsite muss Record* rufen
func (b *Breaker) RecordSuccess()
func (b *Breaker) RecordFailure(err error)
func (b *Breaker) State() State
func (b *Breaker) Stats() Stats
var ErrBreakerOpen = errors.New("circuitbreaker: breaker is open")
func RoundTripper(inner http.RoundTripper, breaker *Breaker) http.RoundTripper
```

## State machine

```
                failures >= FailureThreshold         now - openedAt >= OpenDuration
       ┌────────────────────────────────────────┐  ┌──────────────────────────────┐
       │                                        ▼  │                              ▼
   ┌───────┐                              ┌──────────┐                       ┌──────────┐
   │Closed │ ────── failure (panic/error)──▶  Open   │ ◀──── probe fail ────  HalfOpen  │
   │       │                              │          │                       │          │
   └───────┘ ◀── SuccessThreshold probes ──└──────────┘                       └──────────┘
       ▲       (consecutive in HalfOpen)     ▲  ▲                                ▲  ▲
       │                                    │  └─────── any probe fail ─────────┘  │
       └──────────────────── success ────────┴──────────────────────────────────────┘
```

## Wiring — wo der Breaker bereits eingehängt ist

| Caller-Paket | Datei | Konstruktion |
|--------------|-------|--------------|
| `internal/vane` | `vane.go` (`NewClient`) | `http.Client.Transport = RoundTripper(http.DefaultTransport, breaker)`, 5 Failures / 30s Open / 1 Probe / 1 Success |
| `internal/llm`  | `provider.go` (`NewClient` + `ProviderFromConfig`) | dito, identische Schwellen |
| `internal/harvest` | `harvest.go` (`harvestURLFetch`) | dito, identische Schwellen |

`ghbridge` ist **kein** HTTP-Caller (Subprocess-`exec`); fällt nicht in den Scope dieser Änderung.

## Failure-Klassifikation (RoundTripper)

| Antwort / Fehler | Failure? |
|------------------|----------|
| Transport-Error (DNS / TCP / TLS / Timeout / EOF) | ✅ Ja |
| HTTP 5xx (`resp.StatusCode >= 500`) | ✅ Ja (Body wird gedraint + geschlossen) |
| HTTP 4xx (z.B. 404, 429) | ❌ Nein — Server ist gesund, Caller-Anfrage ist falsch |
| HTTP 3xx (Redirect) | ❌ Nein |
| HTTP 2xx | ❌ Nein |

`429 Too Many Requests` zählt bewusst nicht als Failure, weil der
Upstream weiter antwortet — der Agent sollte Retries mit exponential
Backoff machen, nicht den Upstream ganz abschalten.

## Panic-Semantik

`Execute(fn)` recovers Panics aus `fn`. Der Panic wird in einen
wrapped Error umgewandelt (`fmt.Errorf("circuitbreaker: panic in
protected fn: %v", r)`), als Failure gezählt, und an den Caller
propagiert. So kann ein Buggy-Upstream den Agent-Loop nie crashen.

## Configuration pragmatics

Für Upstream-Calls im `sin-code`-Stack:

| Upstream     | Threshold | OpenDuration | Probe | Why |
|--------------|-----------|--------------|-------|-----|
| Vane         | 5         | 30s          | 1     | Vane ist self-hosted, oft sporadisch; lange Pause schützt vor Hot-Loops |
| LLM Provider | 5         | 30s          | 1     | Echte Provider (OpenAI / Anthropic / NIM); aggressiveres Backoff |
| Generic URL  | 5         | 30s          | 1     | `harvest` ist user-facing; kurze Reopens |

Alle drei benutzen das gleiche Config-Template mit unterschiedlichem
`Name`-String für Log-Distinguishability.

## Override-Pfad für Tests

`Config.Now func() time.Time` ist öffentlich und überschreibbar —
`breaker_test.go.newClock` baut eine test-gesteuerte Uhr. So können
Tests Open → HalfOpen deterministisch in 50ns auslösen statt in 5s
zu warten.

## Caveats

- **Nicht empfohlen für lokale Loops** — wenn dein Caller selbst ein
  Retry-Loop ist, würde der Breaker das Retry selbst kappen. Lagere
  Retries in den Caller oder in eine höhere Schicht aus.
- **In-Process only** — der State ist nicht über Prozesse geteilt.
  Verteilt brauchst du Redis-/Consul-backed State. Das ist explizit
  nicht Teil dieser Implementierung.
- **Keine Metriken nach außen** — wir loggen Fehler via `slog`/`log`
  upstream, aber exportieren kein Prometheus-Format. Folge-PR wenn
  der Bedarf entsteht.

## Test-Coverage (`go test -race`)

```
TestBreaker_ClosedAllowsTraffic         100 calls, alle OK, bleibt closed
TestBreaker_OpensAfterThreshold          N consecutive fails → open
TestBreaker_RejectsWhileOpen             fn wird NIE aufgerufen, ErrBreakerOpen
TestBreaker_HalfOpen_ProbeSuccess_Closes clock-injected, probe ok → closed
TestBreaker_HalfOpen_ProbeFailure_Reopens clock-injected, probe fail → reopen
TestBreaker_Concurrent_NoRace            50 Goroutines × 20 calls, kein race
TestBreaker_PanicCountsAsFailure         panic wird recovered + als fail gezählt
TestRoundTripper_TripsOnTransportError   2 × 5xx via real HTTP → open + dritter Call wird nicht weitergeleitet
TestRoundTripper_4xxIsNotFailure         10 × 404 → bleibt closed
```

Stand: 9 Tests, alle grün unter `-race -count=1`.
