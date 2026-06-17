// SPDX-License-Identifier: MIT
// Purpose: coverage tests for vane_cmd.go — exercises setup/doctor/search/config/serve
// using package-level hooks so tests never require a live Vane instance.
// Docs: vane.doc.md
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vane"
)

type vaneErrWriter struct{ err error }

func (e vaneErrWriter) Write(p []byte) (int, error) { return 0, e.err }

func saveVaneHooks(t *testing.T) {
	t.Helper()
	origLoadConfig := vaneLoadConfigHook
	origSaveConfig := vaneSaveConfigHook
	origRegisterMCP := vaneRegisterMCPHook
	origMCPConfigPath := vaneMCPConfigPathHook
	origConfigPath := vaneConfigPathHook
	origNewClient := vaneNewClientHook
	origServe := vaneServeHook
	origFormatAnswer := vaneFormatAnswerHook
	origLookPath := vaneExecLookPathHook
	t.Cleanup(func() {
		vaneLoadConfigHook = origLoadConfig
		vaneSaveConfigHook = origSaveConfig
		vaneRegisterMCPHook = origRegisterMCP
		vaneMCPConfigPathHook = origMCPConfigPath
		vaneConfigPathHook = origConfigPath
		vaneNewClientHook = origNewClient
		vaneServeHook = origServe
		vaneFormatAnswerHook = origFormatAnswer
		vaneExecLookPathHook = origLookPath
	})
}

type fakeVaneClient struct {
	healthyErr error
	searchErr  error
	answer     *vane.Answer
}

func (c *fakeVaneClient) Healthy(ctx context.Context) error { return c.healthyErr }
func (c *fakeVaneClient) Search(ctx context.Context, query, focus, opt string) (*vane.Answer, error) {
	return c.answer, c.searchErr
}

func runVaneCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewVaneCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestVaneSetupHealthy(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneSaveConfigHook = func(vane.Config) error { return nil }
	vaneMCPConfigPathHook = func() string { return "/tmp/mcp.json" }
	vaneRegisterMCPHook = func(string) (string, error) { return "/tmp/mcp.json", nil }
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{} }
	out, err := runVaneCmd(t, "setup", "--url", "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "vane MCP bridge registered") {
		t.Errorf("expected setup output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Vane instance reachable") {
		t.Errorf("expected reachable output, got %q", out.String())
	}
}

func TestVaneSetupUnreachable(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneSaveConfigHook = func(vane.Config) error { return nil }
	vaneMCPConfigPathHook = func() string { return "/tmp/mcp.json" }
	vaneRegisterMCPHook = func(string) (string, error) { return "/tmp/mcp.json", nil }
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	vaneExecLookPathHook = func(string) (string, error) { return "/usr/bin/docker", nil }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{healthyErr: errors.New("down")} }
	out, err := runVaneCmd(t, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no running Vane instance") {
		t.Errorf("expected unreachable output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "docker") {
		t.Errorf("expected docker command output, got %q", out.String())
	}
}

func TestVaneSetupDockerNotFound(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneSaveConfigHook = func(vane.Config) error { return nil }
	vaneMCPConfigPathHook = func() string { return "/tmp/mcp.json" }
	vaneRegisterMCPHook = func(string) (string, error) { return "/tmp/mcp.json", nil }
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	vaneExecLookPathHook = func(string) (string, error) { return "", errors.New("not found") }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{healthyErr: errors.New("down")} }
	out, err := runVaneCmd(t, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "docker not found") {
		t.Errorf("expected docker not found output, got %q", out.String())
	}
}

func TestVaneSetupInvalidURL(t *testing.T) {
	_, err := runVaneCmd(t, "setup", "--url", "not-a-url")
	if err == nil || !strings.Contains(err.Error(), "must be a valid http(s) URL") {
		t.Fatalf("expected invalid URL error, got %v", err)
	}
}

func TestVaneSetupSaveConfigError(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneSaveConfigHook = func(vane.Config) error { return errors.New("save boom") }
	_, err := runVaneCmd(t, "setup")
	if err == nil || !strings.Contains(err.Error(), "save boom") {
		t.Fatalf("expected save error, got %v", err)
	}
}

func TestVaneSetupRegisterMCPError(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneSaveConfigHook = func(vane.Config) error { return nil }
	vaneMCPConfigPathHook = func() string { return "/tmp/mcp.json" }
	vaneRegisterMCPHook = func(string) (string, error) { return "", errors.New("register boom") }
	_, err := runVaneCmd(t, "setup")
	if err == nil || !strings.Contains(err.Error(), "register MCP") {
		t.Fatalf("expected register error, got %v", err)
	}
}

func TestVaneDoctorLoadConfigError(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.Config{}, false, errors.New("load boom") }
	_, err := runVaneCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestVaneDoctorNotConfigured(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.Config{}, false, nil }
	_, err := runVaneCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "vane not configured") {
		t.Fatalf("expected not configured error, got %v", err)
	}
}

func TestVaneDoctorHealthy(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.Config{BaseURL: "http://x"}, true, nil }
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{} }
	out, err := runVaneCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "base_url:") {
		t.Errorf("expected base_url output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "reachable and ready") {
		t.Errorf("expected ready output, got %q", out.String())
	}
}

func TestVaneDoctorUnhealthy(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.Config{BaseURL: "http://x"}, true, nil }
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{healthyErr: errors.New("down")} }
	_, err := runVaneCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "vane doctor failed") {
		t.Fatalf("expected doctor failed error, got %v", err)
	}
}

func TestVaneSearchLoadConfigError(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.Config{}, false, errors.New("load boom") }
	_, err := runVaneCmd(t, "search", "query")
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestVaneSearchSearchError(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{searchErr: errors.New("search boom")} }
	_, err := runVaneCmd(t, "search", "query")
	if err == nil || !strings.Contains(err.Error(), "search boom") {
		t.Fatalf("expected search error, got %v", err)
	}
}

func TestVaneSearchSuccess(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneFormatAnswerHook = func(*vane.Answer) string { return "formatted answer" }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{answer: &vane.Answer{}} }
	out, err := runVaneCmd(t, "search", "query", "--focus", "webSearch", "--optimization", "speed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "formatted answer") {
		t.Errorf("expected answer output, got %q", out.String())
	}
}

func TestVaneConfigLoadError(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.Config{}, false, errors.New("load boom") }
	_, err := runVaneCmd(t, "config")
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestVaneConfigSuccess(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) {
		return vane.Config{BaseURL: "http://x", ChatProvider: "p", ChatModel: "m", EmbedProvider: "ep", EmbedModel: "em", TimeoutSeconds: 30}, true, nil
	}
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	out, err := runVaneCmd(t, "config")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"base_url:", "chat_provider:", "chat_model:", "embedding_provider:", "embedding_model:", "timeout_seconds:", "config_path:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in output, got %q", want, out.String())
		}
	}
}

func TestVaneServe(t *testing.T) {
	saveVaneHooks(t)
	vaneServeHook = func(ctx context.Context) error { return nil }
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := NewVaneCmd()
	cmd.SetArgs([]string{"serve"})
	setOutAll(cmd, io.Discard)
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestVaneSearchFocusOptimizationDefault(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneFormatAnswerHook = func(*vane.Answer) string { return "answer" }
	vaneNewClientHook = func(vane.Config) vaneClient { return &fakeVaneClient{answer: &vane.Answer{}} }
	out, err := runVaneCmd(t, "search", "query")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "answer") {
		t.Errorf("expected answer output, got %q", out.String())
	}
}
func TestVaneNewClientHookDefault(t *testing.T) {
	client := vaneNewClientHook(vane.Config{})
	if client == nil {
		t.Fatal("expected default vaneNewClientHook to return a non-nil client")
	}
}

func TestVaneDoctorNilClient(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.Config{BaseURL: "http://x"}, true, nil }
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	vaneNewClientHook = func(vane.Config) vaneClient { return nil }
	out, err := runVaneCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "vane doctor failed") {
		t.Fatalf("expected doctor failed error, got %v", err)
	}
	if !strings.Contains(out.String(), "unreachable: no client") {
		t.Errorf("expected no client output, got %q", out.String())
	}
}

func TestVaneSearchNilClient(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) { return vane.DefaultConfig(), true, nil }
	vaneNewClientHook = func(vane.Config) vaneClient { return nil }
	_, err := runVaneCmd(t, "search", "query")
	if err == nil || !strings.Contains(err.Error(), "vane search: no client") {
		t.Fatalf("expected no client error, got %v", err)
	}
}

func TestVaneConfigDefaults(t *testing.T) {
	saveVaneHooks(t)
	vaneLoadConfigHook = func() (vane.Config, bool, error) {
		return vane.Config{BaseURL: "http://x"}, true, nil
	}
	vaneConfigPathHook = func() string { return "/tmp/vane.json" }
	out, err := runVaneCmd(t, "config")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"chat_provider:      (instance default)", "chat_model:         (instance default)", "embedding_provider: (instance default)", "embedding_model:    (instance default)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in output, got %q", want, out.String())
		}
	}
}
