// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func toolBrowserScreenshot(ctx context.Context, selector, qualityStr string) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_screenshot: no active browser session")
	}
	quality := 80
	if qualityStr != "" {
		fmt.Sscanf(qualityStr, "%d", &quality)
	}
	var buf []byte
	var err error
	if selector != "" {
		// chromedp.Screenshot has no quality option; quality applies only to full-page.
		err = chromedp.Run(browserSession.cdpCtx, chromedp.WaitVisible(selector, chromedp.ByQuery), chromedp.Screenshot(selector, &buf, chromedp.ByQuery))
	} else {
		err = chromedp.Run(browserSession.cdpCtx, chromedp.FullScreenshot(&buf, quality))
	}
	if err != nil {
		return "", fmt.Errorf("sin_browser_screenshot: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf), nil
}

func toolBrowserClick(ctx context.Context, selector string) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_click: no active browser session")
	}
	if selector == "" {
		return "", fmt.Errorf("selector required")
	}
	err := chromedp.Run(browserSession.cdpCtx, chromedp.WaitVisible(selector, chromedp.ByQuery), chromedp.Click(selector, chromedp.ByQuery))
	if err != nil {
		return "", fmt.Errorf("sin_browser_click: %w", err)
	}
	return "clicked: " + selector, nil
}

func toolBrowserType(ctx context.Context, selector, text, submitStr string) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("no active browser session")
	}
	if selector == "" {
		return "", fmt.Errorf("selector required")
	}
	submit := strings.ToLower(submitStr) == "true"
	actions := []chromedp.Action{chromedp.WaitVisible(selector, chromedp.ByQuery), chromedp.Focus(selector, chromedp.ByQuery), chromedp.Clear(selector, chromedp.ByQuery), chromedp.SendKeys(selector, text, chromedp.ByQuery)}
	if submit {
		actions = append(actions, chromedp.SendKeys(selector, "\n", chromedp.ByQuery))
	}
	err := chromedp.Run(browserSession.cdpCtx, actions...)
	if err != nil {
		return "", fmt.Errorf("sin_browser_type: %w", err)
	}
	r := "typed " + text + " into " + selector
	if submit {
		r += " and submitted"
	}
	return r, nil
}

func toolBrowserEval(ctx context.Context, expr string) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("no active browser session")
	}
	if expr == "" {
		return "", fmt.Errorf("expression required")
	}
	var result any
	err := chromedp.Run(browserSession.cdpCtx, chromedp.Evaluate(expr, &result))
	if err != nil {
		return "", fmt.Errorf("sin_browser_eval: %w", err)
	}
	return fmt.Sprintf("%v", result), nil
}

func toolBrowserWait(ctx context.Context, selector, timeoutStr string) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("no active browser session")
	}
	if selector == "" {
		return "", fmt.Errorf("selector required")
	}
	timeout := 10 * time.Second
	if timeoutStr != "" {
		var sec int
		if n, _ := fmt.Sscanf(timeoutStr, "%d", &sec); n == 1 && sec > 0 && sec <= 120 {
			timeout = time.Duration(sec) * time.Second
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := chromedp.Run(waitCtx, chromedp.WaitVisible(selector, chromedp.ByQuery))
	if err != nil {
		return fmt.Sprintf("wait failed: %v", err), nil
	}
	return "element visible: " + selector, nil
}

func registerBrowserInteractionSpecs() []agentloopToolSpecAlias {
	s := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	o := func(p map[string]any, r ...string) map[string]any {
		return map[string]any{"type": "object", "properties": p, "required": r}
	}
	return []agentloopToolSpecAlias{
		{Name: "sin_browser_screenshot", Description: "Capture PNG screenshot as base64 data URI for visual understanding (issue #386)", InputSchema: o(map[string]any{"selector": s("CSS selector (optional)"), "quality": s("1-100 default 80")})},
		{Name: "sin_browser_click", Description: "Click element by CSS selector (issue #382)", InputSchema: o(map[string]any{"selector": s("CSS selector")}, "selector")},
		{Name: "sin_browser_type", Description: "Type text into input element (issue #382)", InputSchema: o(map[string]any{"selector": s("CSS selector"), "text": s("text to type"), "submit": s("true to submit")}, "selector", "text")},
		{Name: "sin_browser_eval", Description: "Evaluate JavaScript in page (issue #382)", InputSchema: o(map[string]any{"expr": s("JS expression")}, "expr")},
		{Name: "sin_browser_wait", Description: "Wait for element to appear (issue #382)", InputSchema: o(map[string]any{"selector": s("CSS selector"), "timeout": s("seconds default 10")}, "selector")},
	}
}

func dispatchBrowserInteraction(ctx context.Context, name string, args map[string]any) (string, bool, error) {
	switch name {
	case "sin_browser_screenshot":
		s, _ := args["selector"].(string)
		q, _ := args["quality"].(string)
		out, err := toolBrowserScreenshot(ctx, s, q)
		return out, true, err
	case "sin_browser_click":
		s, _ := args["selector"].(string)
		out, err := toolBrowserClick(ctx, s)
		return out, true, err
	case "sin_browser_type":
		s, _ := args["selector"].(string)
		t, _ := args["text"].(string)
		sub, _ := args["submit"].(string)
		out, err := toolBrowserType(ctx, s, t, sub)
		return out, true, err
	case "sin_browser_eval":
		e, _ := args["expr"].(string)
		out, err := toolBrowserEval(ctx, e)
		return out, true, err
	case "sin_browser_wait":
		s, _ := args["selector"].(string)
		t, _ := args["timeout"].(string)
		out, err := toolBrowserWait(ctx, s, t)
		return out, true, err
	default:
		return "", false, nil
	}
}
