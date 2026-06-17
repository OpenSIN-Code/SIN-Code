// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestChatSearchFindsTextInUserMessage(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "Hello world from user"},
		{Kind: chatAssistant, Text: "Greetings! How can I help?"},
	}
	results := cs.Search(history, "hello")
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'hello'")
	}
	if results[0].MessageIdx != 0 {
		t.Errorf("expected MessageIdx 0, got %d", results[0].MessageIdx)
	}
	if results[0].MatchLen != 5 {
		t.Errorf("expected MatchLen 5, got %d", results[0].MatchLen)
	}
}

func TestChatSearchFindsTextInAssistantMessage(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "what is Go?"},
		{Kind: chatAssistant, Text: "Go is a statically typed programming language"},
	}
	results := cs.Search(history, "programming")
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'programming'")
	}
	if results[0].MessageIdx != 1 {
		t.Errorf("expected MessageIdx 1 (assistant), got %d", results[0].MessageIdx)
	}
}

func TestChatSearchCaseInsensitive(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "ERROR: something went wrong"},
	}
	results := cs.Search(history, "error")
	if len(results) == 0 {
		t.Fatal("expected case-insensitive match for 'error' in 'ERROR: ...'")
	}
	if results[0].MatchPos != 0 {
		t.Errorf("expected MatchPos 0, got %d", results[0].MatchPos)
	}
}

func TestChatSearchNoMatchReturnsEmpty(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "hello world"},
		{Kind: chatAssistant, Text: "hi there"},
	}
	results := cs.Search(history, "nonexistent")
	if results != nil && len(results) > 0 {
		t.Errorf("expected nil/empty results, got %d", len(results))
	}
}

func TestChatSearchNextPrevNavigation(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "test one test two test three"},
	}
	results := cs.Search(history, "test")
	if len(results) < 3 {
		t.Fatalf("expected at least 3 results, got %d", len(results))
	}

	cs.Next()
	cur := cs.CurrentResult()
	if cur == nil {
		t.Fatal("expected non-nil after Next")
	}
	if cur.MessageIdx != results[1].MessageIdx || cur.MatchPos != results[1].MatchPos {
		t.Errorf("expected 2nd result after Next, got pos %d", cur.MatchPos)
	}

	cs.Prev()
	cur = cs.CurrentResult()
	if cur == nil {
		t.Fatal("expected non-nil after Prev")
	}
	if cur.MatchPos != results[0].MatchPos {
		t.Errorf("expected 1st result after Prev, got pos %d", cur.MatchPos)
	}
}

func TestChatSearchNextWraps(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "find me here"},
	}
	cs.Search(history, "find")
	cur := cs.CurrentResult()
	if cur == nil {
		t.Fatal("expected non-nil initial result")
	}
	cs.Next()
	cur = cs.CurrentResult()
	if cur == nil {
		t.Fatal("expected non-nil after wrap Next")
	}
}

func TestChatSearchContextExtraction(t *testing.T) {
	cs := NewChatSearch()
	longText := strings.Repeat("x", 100) + "TARGET" + strings.Repeat("y", 100)
	history := []ChatMessage{
		{Kind: chatAssistant, Text: longText},
	}
	results := cs.Search(history, "TARGET")
	if len(results) == 0 {
		t.Fatal("expected result for TARGET")
	}
	ctx := results[0].Context
	if !strings.Contains(ctx, "TARGET") {
		t.Error("expected context to contain match")
	}
	if !strings.HasPrefix(ctx, "…") {
		t.Error("expected context to start with … when match is not at beginning")
	}
	if !strings.HasSuffix(ctx, "…") {
		t.Error("expected context to end with … when match is not at end")
	}
}

func TestChatSearchContextShortTextNoEllipsis(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "find this short text"},
	}
	results := cs.Search(history, "short")
	if len(results) == 0 {
		t.Fatal("expected result")
	}
	ctx := results[0].Context
	if strings.HasPrefix(ctx, "…") {
		t.Error("expected no leading … for short text")
	}
	if strings.HasSuffix(ctx, "…") {
		t.Error("expected no trailing … for short text")
	}
}

func TestChatSearchEmptyHistory(t *testing.T) {
	cs := NewChatSearch()
	results := cs.Search(nil, "anything")
	if results != nil {
		t.Error("expected nil results for empty history")
	}
	results = cs.Search([]ChatMessage{}, "anything")
	if results != nil {
		t.Error("expected nil results for empty history slice")
	}
}

func TestChatSearchEmptyQuery(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "hello"},
	}
	results := cs.Search(history, "")
	if results != nil {
		t.Error("expected nil results for empty query")
	}
}

func TestChatSearchMultipleOccurrencesInOneMessage(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "go go go go"},
	}
	results := cs.Search(history, "go")
	if len(results) != 4 {
		t.Errorf("expected 4 results for 'go' in 'go go go go', got %d", len(results))
	}
}

func TestChatSearchCurrentResultNilWhenEmpty(t *testing.T) {
	cs := NewChatSearch()
	if cs.CurrentResult() != nil {
		t.Error("expected nil CurrentResult before search")
	}
	cs.Search([]ChatMessage{{Kind: chatUser, Text: "hi"}}, "zzz")
	if cs.CurrentResult() != nil {
		t.Error("expected nil CurrentResult when no results")
	}
}

func TestChatSearchClear(t *testing.T) {
	cs := NewChatSearch()
	cs.Search([]ChatMessage{{Kind: chatUser, Text: "find me"}}, "find")
	if cs.CurrentResult() == nil {
		t.Fatal("expected non-nil before Clear")
	}
	cs.Clear()
	if cs.CurrentResult() != nil {
		t.Error("expected nil CurrentResult after Clear")
	}
	if cs.Query() != "" {
		t.Error("expected empty query after Clear")
	}
}

func TestChatSearchRenderContainsQuery(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "search for keywords here"},
	}
	cs.Search(history, "keywords")
	styles := NewStyles(Themes[0])
	out := cs.Render(styles, 70, 20)
	if !strings.Contains(out, "keywords") {
		t.Error("expected query in render output")
	}
	if !strings.Contains(out, "/search:") {
		t.Error("expected /search: prefix in render output")
	}
}

func TestChatSearchRenderNoMatches(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "hello world"},
	}
	cs.Search(history, "zzzzz")
	styles := NewStyles(Themes[0])
	out := cs.Render(styles, 70, 20)
	if !strings.Contains(out, "no matches") {
		t.Error("expected 'no matches' in render output")
	}
}

func TestChatSearchRenderBar(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "find this"},
	}
	cs.Search(history, "find")
	styles := NewStyles(Themes[0])
	bar := cs.RenderBar(styles)
	if !strings.Contains(bar, "/search:") {
		t.Error("expected /search: in bar")
	}
	if !strings.Contains(bar, "find") {
		t.Error("expected query in bar")
	}
}

func TestChatSearchRenderBarWithCount(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "test test test"},
	}
	cs.Search(history, "test")
	styles := NewStyles(Themes[0])
	bar := cs.RenderBar(styles)
	if !strings.Contains(bar, "1/") {
		t.Errorf("expected result count in bar, got %q", bar)
	}
}

func TestChatSearchConcurrentAccess(t *testing.T) {
	cs := NewChatSearch()
	history := []ChatMessage{
		{Kind: chatUser, Text: "concurrent test here"},
		{Kind: chatAssistant, Text: "another concurrent message"},
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs.Search(history, "concurrent")
			cs.Next()
			cs.Prev()
			cs.CurrentResult()
			cs.Results()
			cs.Query()
			cs.Clear()
		}()
	}
	wg.Wait()
}
