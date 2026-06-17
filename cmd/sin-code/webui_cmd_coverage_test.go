// SPDX-License-Identifier: MIT
// Purpose: coverage tests for webui_cmd.go — exercise the webui command RunE
// by stubbing webui.StartWith so no real HTTP server is started.
package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/webui"
)

func setWebuiHook[T any](t *testing.T, ptr *T, val T) {
	t.Helper()
	orig := *ptr
	*ptr = val
	t.Cleanup(func() { *ptr = orig })
}

func resetWebuiFlags() {
	webuiPort = 27402
	webuiHost = "127.0.0.1"
	webuiOpen = false
	webuiTodoDB = ""
	webuiNotifDB = ""
}

func TestWebuiCmd_RunE_Success(t *testing.T) {
	setWebuiHook(t, &webuiStartWithFn, func(cfg webui.Config) error {
		return nil
	})
	resetWebuiFlags()

	var gotCfg webui.Config
	webuiStartWithFn = func(cfg webui.Config) error {
		gotCfg = cfg
		return nil
	}

	if err := webuiCmd.RunE(webuiCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if gotCfg.Host != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", gotCfg.Host)
	}
	if gotCfg.Port != 27402 {
		t.Errorf("port = %d, want 27402", gotCfg.Port)
	}
}

func TestWebuiCmd_RunE_Error(t *testing.T) {
	resetWebuiFlags()
	setWebuiHook(t, &webuiStartWithFn, func(webui.Config) error {
		return errors.New("server failed")
	})

	err := webuiCmd.RunE(webuiCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "server failed") {
		t.Errorf("expected server error, got %v", err)
	}
}

func TestWebuiCmd_RunE_Flags(t *testing.T) {
	resetWebuiFlags()
	var gotCfg webui.Config
	setWebuiHook(t, &webuiStartWithFn, func(cfg webui.Config) error {
		gotCfg = cfg
		return nil
	})

	webuiPort = 8080
	webuiHost = "0.0.0.0"
	webuiOpen = true
	webuiTodoDB = "/tmp/t.db"
	webuiNotifDB = "/tmp/n.db"

	if err := webuiCmd.RunE(webuiCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if gotCfg.Port != 8080 {
		t.Errorf("port = %d, want 8080", gotCfg.Port)
	}
	if gotCfg.Host != "0.0.0.0" {
		t.Errorf("host = %q, want 0.0.0.0", gotCfg.Host)
	}
	if !gotCfg.OpenBrowser {
		t.Error("expected OpenBrowser true")
	}
	if gotCfg.TodoDB != "/tmp/t.db" {
		t.Errorf("todo db = %q, want /tmp/t.db", gotCfg.TodoDB)
	}
	if gotCfg.NotifDB != "/tmp/n.db" {
		t.Errorf("notif db = %q, want /tmp/n.db", gotCfg.NotifDB)
	}
}

// keep fmt import valid across edits.
var _ = fmt.Sprintf
