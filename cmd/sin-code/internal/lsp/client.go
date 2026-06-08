// SPDX-License-Identifier: MIT
// Purpose: LSP (Language Server Protocol) client. JSON-RPC 2.0 over stdio
// with Content-Length framing per the LSP spec. Supports initialize,
// textDocument/didOpen, didChange, didClose, and the standard
// textDocument/* requests.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  io.ReadCloser
	mu      sync.Mutex
	nextID  int
	rootURI string
	lang    string
}

type InitializeParams struct {
	ProcessID    int                `json:"processId"`
	RootURI      string             `json:"rootUri"`
	Capabilities ClientCapabilities `json:"capabilities"`
	ClientInfo   map[string]string  `json:"clientInfo,omitempty"`
}

type ClientCapabilities struct {
	TextDocument map[string]any `json:"textDocument,omitempty"`
	Workspace    map[string]any `json:"workspace,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   map[string]string  `json:"serverInfo,omitempty"`
}

type ServerCapabilities struct {
	TextDocumentSync         any  `json:"textDocumentSync,omitempty"`
	HoverProvider            any  `json:"hoverProvider,omitempty"`
	DefinitionProvider       any  `json:"definitionProvider,omitempty"`
	ReferencesProvider       any  `json:"referencesProvider,omitempty"`
	RenameProvider           any  `json:"renameProvider,omitempty"`
	DocumentFormattingProvider any `json:"documentFormattingProvider,omitempty"`
	DocumentSymbolProvider   any  `json:"documentSymbolProvider,omitempty"`
	CodeActionProvider       any  `json:"codeActionProvider,omitempty"`
	CompletionProvider       any  `json:"completionProvider,omitempty"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type LocationLink struct {
	OriginSelectionRange *Range  `json:"originSelectionRange,omitempty"`
	TargetURI            string `json:"targetUri"`
	TargetRange          Range  `json:"targetRange"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position              `json:"position"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`
	RangeLength *int   `json:"rangeLength,omitempty"`
	Text        string `json:"text"`
}

type Hover struct {
	Contents any    `json:"contents"`
	Range    *Range `json:"range,omitempty"`
}

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Tags           []any            `json:"tags,omitempty"`
	Deprecated     *bool            `json:"deprecated,omitempty"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

type SymbolInformation struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	Deprecated    *bool  `json:"deprecated,omitempty"`
	Location      Location `json:"location"`
	ContainerName string `json:"containerName,omitempty"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity *int   `json:"severity,omitempty"`
	Code     any    `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position              `json:"position"`
	NewName      string                `json:"newName"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func Start(binary string, args []string, lang, rootURI string) (*Client, error) {
	if binary == "" {
		return nil, fmt.Errorf("binary required")
	}
	cmd := exec.Command(binary, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		stderr:  stderr,
		rootURI: rootURI,
		lang:    lang,
	}
	if err := c.initialize(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) initialize() error {
	params := InitializeParams{
		ProcessID:    0,
		RootURI:      c.rootURI,
		Capabilities: ClientCapabilities{TextDocument: map[string]any{"synchronization": map[string]any{"dynamicRegistration": false}}},
		ClientInfo:   map[string]string{"name": "sin-code", "version": "2.0.0"},
	}
	var result InitializeResult
	if err := c.Call("initialize", params, &result, 30*time.Second); err != nil {
		return fmt.Errorf("initialize %s: %w", c.lang, err)
	}
	return c.Notify("initialized", map[string]any{})
}

func (c *Client) Lang() string { return c.lang }
func (c *Client) RootURI() string { return c.rootURI }

func (c *Client) Close() error {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	_ = c.Notify("shutdown", nil)
	_ = c.Notify("exit", nil)
	_ = c.stdin.Close()
	return c.cmd.Wait()
}

func (c *Client) DidOpen(doc TextDocumentItem) error {
	return c.Notify("textDocument/didOpen", map[string]any{"textDocument": doc})
}

func (c *Client) DidChange(uri string, version int, changes []TextDocumentContentChangeEvent) error {
	return c.Notify("textDocument/didChange", map[string]any{
		"textDocument": VersionedTextDocumentIdentifier{URI: uri, Version: version},
		"contentChanges": changes,
	})
}

func (c *Client) DidClose(uri string) error {
	return c.Notify("textDocument/didClose", map[string]any{
		"textDocument": TextDocumentIdentifier{URI: uri},
	})
}

func (c *Client) Definition(uri string, pos Position) ([]Location, error) {
	var raw []json.RawMessage
	if err := c.Call("textDocument/definition", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     pos,
	}, &raw, 30*time.Second); err != nil {
		return nil, err
	}
	out := make([]Location, 0, len(raw))
	for _, r := range raw {
		var l Location
		if err := json.Unmarshal(r, &l); err == nil && l.URI != "" {
			out = append(out, l)
		} else {
			var link LocationLink
			if err := json.Unmarshal(r, &link); err == nil && link.TargetURI != "" {
				out = append(out, Location{URI: link.TargetURI, Range: link.TargetRange})
			}
		}
	}
	return out, nil
}

func (c *Client) References(uri string, pos Position, includeDecl bool) ([]Location, error) {
	var raw []json.RawMessage
	params := struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position              `json:"position"`
		Context      struct {
			IncludeDeclaration bool `json:"includeDeclaration"`
		} `json:"context"`
	}{}
	params.TextDocument = TextDocumentIdentifier{URI: uri}
	params.Position = pos
	params.Context.IncludeDeclaration = includeDecl
	if err := c.Call("textDocument/references", params, &raw, 30*time.Second); err != nil {
		return nil, err
	}
	out := make([]Location, 0, len(raw))
	for _, r := range raw {
		var l Location
		if err := json.Unmarshal(r, &l); err == nil && l.URI != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

func (c *Client) Hover(uri string, pos Position) (*Hover, error) {
	var h Hover
	if err := c.Call("textDocument/hover", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     pos,
	}, &h, 30*time.Second); err != nil {
		return nil, err
	}
	if h.Contents == nil {
		return nil, nil
	}
	return &h, nil
}

func (c *Client) Symbols(uri string) ([]DocumentSymbol, error) {
	var raw []json.RawMessage
	if err := c.Call("textDocument/documentSymbol", map[string]any{
		"textDocument": TextDocumentIdentifier{URI: uri},
	}, &raw, 30*time.Second); err != nil {
		return nil, err
	}
	out := make([]DocumentSymbol, 0, len(raw))
	for _, r := range raw {
		var s DocumentSymbol
		if err := json.Unmarshal(r, &s); err == nil && s.Name != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func (c *Client) DefinitionRaw(uri string, pos Position) (string, error) {
	var raw json.RawMessage
	if err := c.Call("textDocument/definition", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     pos,
	}, &raw, 30*time.Second); err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *Client) Rename(uri string, pos Position, newName string) (*WorkspaceEdit, error) {
	var w WorkspaceEdit
	params := RenameParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     pos,
		NewName:      newName,
	}
	if err := c.Call("textDocument/rename", params, &w, 30*time.Second); err != nil {
		return nil, err
	}
	return &w, nil
}

func (c *Client) Format(uri string) ([]TextEdit, error) {
	var raw []TextEdit
	params := map[string]any{
		"textDocument": TextDocumentIdentifier{URI: uri},
		"options":      map[string]any{"tabSize": 4, "insertSpaces": true},
	}
	if err := c.Call("textDocument/formatting", params, &raw, 30*time.Second); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Client) Call(method string, params any, result any, timeout time.Duration) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(raw))
	if _, err := c.stdin.Write([]byte(header + string(raw))); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		stdout := c.stdout
		c.mu.Unlock()
		line, err := stdout.ReadString('\n')
		if err == io.EOF {
			return fmt.Errorf("server closed before response")
		}
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		var headerKey, headerVal string
		if idx := strings.Index(line, ":"); idx > 0 {
			headerKey = strings.TrimSpace(line[:idx])
			headerVal = strings.TrimSpace(line[idx+1:])
		}
		if headerKey == "Content-Length" {
			length, _ := strconv.Atoi(headerVal)
			buf := make([]byte, length)
			if _, err := io.ReadFull(stdout, buf); err != nil {
				return err
			}
			var resp Response
			if err := json.Unmarshal(buf, &resp); err != nil {
				return err
			}
			if resp.Error != nil {
				return fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			if result != nil && len(resp.Result) > 0 {
				return json.Unmarshal(resp.Result, result)
			}
			return nil
		}
	}
	return fmt.Errorf("LSP timeout after %s", timeout)
}

func (c *Client) Notify(method string, params any) error {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(raw))
	_, err = c.stdin.Write([]byte(header + string(raw)))
	return err
}
