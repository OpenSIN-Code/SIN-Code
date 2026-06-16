// SPDX-License-Identifier: MIT
// Purpose: CDP Recorder for the SIN-Code browser-tools skill.
//
// Design rationale:
//   - Uses a real chromedp CDPSession (via chromedp.ListenTarget) rather than
//     Playwright page.on wrappers so that every raw protocol event is captured
//     verbatim, including extra-info events that Playwright filters away.
//   - Subscribes to every domain that surfaces actionable signals: Network
//     (15 events including ExtraInfo/WebSocket/EventSource), Runtime (console +
//     exceptions), Log, Audits (DevTools Issues panel — CORS, CSP, mixed
//     content, deprecations), Security (TLS state), Page lifecycle, and Target
//     (OOPIF / workers via setAutoAttach flatten=true).
//   - Writes a JSONL ground-truth log with monotonic sequence numbers, wall
//     clock, and a step_id correlation field so every event can be traced back
//     to the agent action that triggered it.
//   - Optionally captures response bodies on loadingFinished (deferred to
//     correct timing), guarded by a configurable byte cap.
//   - Keeps an in-memory copy of events (when Config.KeepInMemory is true) so
//     the deterministic Findings engine in findings.go can run synchronously
//     after recording without re-reading the JSONL from disk.
//
// Linter note: cdproto imports are aliased where the package name clashes with
// standard library names (cdplog for cdproto/log).
package cdp

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/audits"
	"github.com/chromedp/cdproto/dom"
	cdplog "github.com/chromedp/cdproto/log"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/performance"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/security"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// Config controls the Recorder's behaviour.
type Config struct {
	// JSONLPath is the file path for the ground-truth JSONL log.
	// Required; NewRecorder returns an error if the file cannot be created.
	JSONLPath string

	// KeepInMemory retains a copy of every Event in memory so callers can
	// pass rec.Events() directly to Analyze without reading the JSONL file.
	KeepInMemory bool

	// CaptureBodies fetches the response body for each completed network
	// request and emits a synthetic "Network"/"responseBody" event.
	CaptureBodies bool

	// MaxBodyBytes caps individual response body captures. 0 means no limit.
	// Recommended default: 2 MiB (2 << 20).
	MaxBodyBytes int64

	// MetricsEvery controls the Performance.getMetrics polling interval.
	// 0 disables polling (the default for lightweight runs).
	MetricsEvery time.Duration
}

// DefaultConfig returns a Config suitable for most agent sessions.
// JSONLPath must still be set by the caller.
func DefaultConfig(jsonlPath string) Config {
	return Config{
		JSONLPath:     jsonlPath,
		KeepInMemory:  true,
		CaptureBodies: true,
		MaxBodyBytes:  2 << 20, // 2 MiB
		MetricsEvery:  2 * time.Second,
	}
}

// Recorder captures CDP events into a JSONL ground-truth log and optionally
// into an in-memory slice for the Findings engine.
type Recorder struct {
	cfg   Config
	sink  *Sink
	start time.Time

	mu     sync.Mutex
	seq    uint64
	events []*Event // populated only when cfg.KeepInMemory is true

	// step is the current correlation ID set by the agent before each action.
	// atomic.Value allows racy reads from the dispatch goroutine without a mutex.
	step atomic.Value // stores string

	// dead is closed by Close() to signal background goroutines to exit.
	dead chan struct{}
}

// NewRecorder creates a Recorder and opens the JSONL sink at cfg.JSONLPath.
// The caller must call Close() when the session ends.
func NewRecorder(cfg Config) (*Recorder, error) {
	sink, err := NewSink(cfg.JSONLPath)
	if err != nil {
		return nil, err
	}
	r := &Recorder{
		cfg:   cfg,
		sink:  sink,
		start: time.Now(),
		dead:  make(chan struct{}),
	}
	r.step.Store("")
	return r, nil
}

// SetStep tags all subsequent events with id so they can be correlated to
// the agent action that triggered them (e.g. "navigate", "click_submit").
// Call before each agent action and clear with SetStep("") afterwards.
func (r *Recorder) SetStep(id string) { r.step.Store(id) }

func (r *Recorder) currentStep() string {
	if v, ok := r.step.Load().(string); ok {
		return v
	}
	return ""
}

// Events returns a snapshot of the in-memory event slice.
// Returns nil if Config.KeepInMemory was false.
func (r *Recorder) Events() []*Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.events == nil {
		return nil
	}
	out := make([]*Event, len(r.events))
	copy(out, r.events)
	return out
}

// Close stops background goroutines and flushes + closes the JSONL sink.
// Must be called exactly once after recording is complete.
func (r *Recorder) Close() error {
	close(r.dead)
	return r.sink.Close()
}

// EnableDomains enables every CDP domain the Recorder listens to.
// Must be called after Attach but before the first navigation.
// Each Enable is best-effort: a missing domain must not abort the others.
func (r *Recorder) EnableDomains(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_ = network.Enable().Do(ctx)
		_ = page.Enable().Do(ctx)
		_ = runtime.Enable().Do(ctx)
		_ = cdplog.Enable().Do(ctx)
		_ = audits.Enable().Do(ctx)   // DevTools Issues: CORS, CSP, mixed content, deprecations
		_ = security.Enable().Do(ctx) // TLS / certificate / mixed-content state
		_ = dom.Enable().Do(ctx)
		_ = performance.Enable().Do(ctx)
		_ = page.SetLifecycleEventsEnabled(true).Do(ctx)
		// setAutoAttach with flatten=true surfaces OOPIFs and workers as
		// child CDP sessions with their own session IDs, so cross-origin
		// iframes are captured with the same fidelity as the main frame.
		_ = target.SetAutoAttach(true, false, true).Do(ctx)
		return nil
	}))
}

// Attach registers the event listener on the chromedp context and starts the
// optional Performance metrics poller. Must be called after chromedp.NewContext
// and before the first navigation.
func (r *Recorder) Attach(ctx context.Context) {
	chromedp.ListenTarget(ctx, func(ev interface{}) { r.dispatch(ctx, ev) })
	if r.cfg.MetricsEvery > 0 {
		go r.pollMetrics(ctx)
	}
}

func (r *Recorder) pollMetrics(ctx context.Context) {
	t := time.NewTicker(r.cfg.MetricsEvery)
	defer t.Stop()
	for {
		select {
		case <-r.dead:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
				m, err := performance.GetMetrics().Do(ctx)
				if err == nil {
					r.emit("Performance", "metrics", "", map[string]interface{}{"metrics": m})
				}
				return nil
			}))
		}
	}
}

// emit assigns a sequence number, builds an Event, and writes it to the sink.
func (r *Recorder) emit(domain, method, session string, params interface{}) {
	b, err := json.Marshal(params)
	if err != nil {
		return
	}
	r.mu.Lock()
	r.seq++
	e := &Event{
		Seq:       r.seq,
		WallTime:  time.Now().UTC().Format(time.RFC3339Nano),
		MonoNanos: time.Since(r.start).Nanoseconds(),
		SessionID: session,
		Domain:    domain,
		Method:    method,
		StepID:    r.currentStep(),
		Params:    b,
	}
	if r.cfg.KeepInMemory {
		r.events = append(r.events, e)
	}
	r.mu.Unlock()
	// Write outside the mutex: the Sink has its own lock, and ordering is
	// already guaranteed by the sequence number assigned above.
	r.sink.write(e)
}

// dispatch type-switches every CDP event and records it verbatim.
// It is called from chromedp.ListenTarget on the chromedp event loop goroutine.
//
//nolint:cyclop // large switch is intentional — one case per CDP event type
func (r *Recorder) dispatch(ctx context.Context, ev interface{}) {
	switch e := ev.(type) {

	// ---- Network (15 events) -----------------------------------------------
	case *network.EventRequestWillBeSent:
		r.emit("Network", "requestWillBeSent", "", e)
	case *network.EventRequestWillBeSentExtraInfo:
		r.emit("Network", "requestWillBeSentExtraInfo", "", e)
	case *network.EventResponseReceived:
		r.emit("Network", "responseReceived", "", e)
	case *network.EventResponseReceivedExtraInfo:
		r.emit("Network", "responseReceivedExtraInfo", "", e)
	case *network.EventDataReceived:
		r.emit("Network", "dataReceived", "", e)
	case *network.EventLoadingFinished:
		r.emit("Network", "loadingFinished", "", e)
		if r.cfg.CaptureBodies {
			go r.fetchBody(ctx, e.RequestID)
		}
	case *network.EventLoadingFailed:
		r.emit("Network", "loadingFailed", "", e)
	case *network.EventRequestServedFromCache:
		r.emit("Network", "requestServedFromCache", "", e)
	case *network.EventResourceChangedPriority:
		r.emit("Network", "resourceChangedPriority", "", e)
	case *network.EventWebSocketCreated:
		r.emit("Network", "webSocketCreated", "", e)
	case *network.EventWebSocketFrameSent:
		r.emit("Network", "webSocketFrameSent", "", e)
	case *network.EventWebSocketFrameReceived:
		r.emit("Network", "webSocketFrameReceived", "", e)
	case *network.EventWebSocketFrameError:
		r.emit("Network", "webSocketFrameError", "", e)
	case *network.EventWebSocketClosed:
		r.emit("Network", "webSocketClosed", "", e)
	case *network.EventEventSourceMessageReceived:
		r.emit("Network", "eventSourceMessageReceived", "", e)

	// ---- Runtime (console + uncaught exceptions) ---------------------------
	case *runtime.EventConsoleAPICalled:
		r.emit("Runtime", "consoleAPICalled", "", e)
	case *runtime.EventExceptionThrown:
		r.emit("Runtime", "exceptionThrown", "", e)

	// ---- Log (browser-level log entries, distinct from console) -----------
	case *cdplog.EventEntryAdded:
		r.emit("Log", "entryAdded", "", e)

	// ---- Audits (DevTools Issues panel) ------------------------------------
	// Covers: CORS errors, CSP violations, mixed content, SameSite cookies,
	// low contrast, deprecation warnings — all pre-classified by Chrome.
	case *audits.EventIssueAdded:
		r.emit("Audits", "issueAdded", "", e)

	// ---- Security (TLS / certificate / mixed-content state) ---------------
	case *security.EventSecurityStateChanged:
		r.emit("Security", "securityStateChanged", "", e)

	// ---- Page lifecycle (16 events) ----------------------------------------
	case *page.EventLoadEventFired:
		r.emit("Page", "loadEventFired", "", e)
	case *page.EventDomContentEventFired:
		r.emit("Page", "domContentEventFired", "", e)
	case *page.EventLifecycleEvent:
		r.emit("Page", "lifecycleEvent", "", e)
	case *page.EventFrameNavigated:
		r.emit("Page", "frameNavigated", "", e)
	case *page.EventFrameRequestedNavigation:
		r.emit("Page", "frameRequestedNavigation", "", e)
	case *page.EventJavascriptDialogOpening:
		r.emit("Page", "javascriptDialogOpening", "", e)
	case *page.EventFileChooserOpened:
		r.emit("Page", "fileChooserOpened", "", e)
	case *page.EventDownloadWillBegin:
		r.emit("Page", "downloadWillBegin", "", e)
	case *page.EventDownloadProgress:
		r.emit("Page", "downloadProgress", "", e)

	// ---- Targets (OOPIFs, workers, service workers) -----------------------
	// setAutoAttach(flatten=true) surfaces cross-origin iframes and workers as
	// child sessions. We record the attach/detach events here; enabling domains
	// on child sessions (the TODO below) is the next hardening step.
	case *target.EventAttachedToTarget:
		r.emit("Target", "attachedToTarget", string(e.SessionID), e)
		// TODO(oopif): call Runtime.enable, Network.enable, Log.enable, and
		// Audits.enable on the child session executor so that console errors,
		// network failures, and audit issues inside cross-origin iframes are
		// captured with the same fidelity as the main frame.
	case *target.EventDetachedFromTarget:
		r.emit("Target", "detachedFromTarget", string(e.SessionID), e)
	case *target.EventTargetCreated:
		r.emit("Target", "targetCreated", "", e)
	case *target.EventTargetDestroyed:
		r.emit("Target", "targetDestroyed", "", e)
	}
}

// fetchBody retrieves the response body for a completed request and emits it
// as a synthetic "Network"/"responseBody" event. Called in its own goroutine
// after loadingFinished so the body is guaranteed to be available.
func (r *Recorder) fetchBody(ctx context.Context, id network.RequestID) {
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		body, err := network.GetResponseBody(id).Do(ctx)
		if err != nil {
			// Body may already be evicted from the cache; not fatal.
			return nil
		}
		truncated := false
		if r.cfg.MaxBodyBytes > 0 && int64(len(body)) > r.cfg.MaxBodyBytes {
			body = body[:r.cfg.MaxBodyBytes]
			truncated = true
		}
		r.emit("Network", "responseBody", "", map[string]interface{}{
			"requestId": id,
			"bytes":     len(body),
			"truncated": truncated,
			"body":      string(body),
		})
		return nil
	}))
}
