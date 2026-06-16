// SPDX-License-Identifier: MIT
// Purpose: Web Vitals injection for the CDP Recorder.
//
// InstallVitals adds a PerformanceObserver script to every new document so
// that LCP, CLS, INP, and LongTask metrics are forwarded via a tagged
// console.debug call into the CDP event stream. The findings engine in
// findings.go intercepts these "__SINCDP_VITAL__"-tagged events before the
// general console handler so they are classified as "vital" findings rather
// than plain console entries.
package cdp

import (
	"context"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// vitalsScript installs PerformanceObservers for LCP, CLS, INP, and LongTask.
// Each metric is forwarded to the host via console.debug with the tag
// "__SINCDP_VITAL__" so the findings layer can recognise and classify it.
const vitalsScript = `
(() => {
  const send = (name, value, extra) => {
    try {
      console.debug("__SINCDP_VITAL__", JSON.stringify({ name, value, ...extra }));
    } catch (e) {}
  };

  // Largest Contentful Paint
  try {
    new PerformanceObserver((l) => {
      const entries = l.getEntries();
      const last = entries[entries.length - 1];
      if (last) send("LCP", last.startTime, { size: last.size });
    }).observe({ type: "largest-contentful-paint", buffered: true });
  } catch (e) {}

  // Cumulative Layout Shift
  try {
    let cls = 0;
    new PerformanceObserver((l) => {
      for (const entry of l.getEntries()) {
        if (!entry.hadRecentInput) cls += entry.value;
      }
      send("CLS", cls, {});
    }).observe({ type: "layout-shift", buffered: true });
  } catch (e) {}

  // Long Tasks (main-thread blocking)
  try {
    new PerformanceObserver((l) => {
      for (const entry of l.getEntries()) {
        send("LongTask", entry.duration, { start: entry.startTime });
      }
    }).observe({ type: "longtask", buffered: true });
  } catch (e) {}

  // Interaction to Next Paint (INP)
  try {
    new PerformanceObserver((l) => {
      for (const entry of l.getEntries()) {
        const delay = entry.processingStart - entry.startTime;
        send("INP", delay, { name: entry.name });
      }
    }).observe({ type: "event", buffered: true, durationThreshold: 40 });
  } catch (e) {}
})();
`

// InstallVitals registers the PerformanceObserver script so it runs on every
// new document loaded in the tab. Call once after EnableDomains and before
// navigating. No-op if the Page domain is not enabled.
func (r *Recorder) InstallVitals(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(vitalsScript).Do(ctx)
		return err
	}))
}

// EvalVitalsNow forces an immediate metric flush by re-running the observer
// script in the current document context. Useful when called right before
// BuildReport on a page that is already loaded, so that final CLS/LCP values
// are captured even if the observers never fired a final callback.
func (r *Recorder) EvalVitalsNow(ctx context.Context) {
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, exp, err := runtime.Evaluate(vitalsScript).Do(ctx)
		_ = exp
		return err
	}))
}
