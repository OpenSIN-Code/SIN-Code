// SPDX-License-Identifier: MIT
//go:build windows

package tui

import (
	"os"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
	procGetConsoleMode        = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode        = kernel32.NewProc("SetConsoleMode")
)

const (
	enableVirtualTerminalProcessing uint32 = 0x0004
	ctrlCEvent                      uint32 = 0
	ctrlBreakEvent                  uint32 = 1
	ctrlCloseEvent                  uint32 = 2
)

var (
	restoreOnce  sync.Once
	handlerCb    uintptr
	consoleHdl   syscall.Handle
	savedMode    uint32
	modeCaptured bool
)

func setupPlatformGuard() *PlatformGuard {
	consoleHdl = syscall.Handle(os.Stdout.Fd())

	r1, _, _ := procGetConsoleMode.Call(
		uintptr(consoleHdl),
		uintptr(unsafe.Pointer(&savedMode)),
	)
	modeCaptured = r1 != 0

	handlerCb = syscall.NewCallback(consoleCtrlHandler)
	procSetConsoleCtrlHandler.Call(handlerCb, 1)
	return &PlatformGuard{
		cleanup: func() {
			restoreConsoleMode()
			procSetConsoleCtrlHandler.Call(handlerCb, 0)
		},
	}
}

func consoleCtrlHandler(ctrlType uintptr) uintptr {
	switch uint32(ctrlType) {
	case ctrlCEvent, ctrlBreakEvent, ctrlCloseEvent:
		restoreConsoleMode()
		os.Exit(0)
	}
	return 0
}

func restoreConsoleMode() {
	restoreOnce.Do(func() {
		if !modeCaptured {
			return
		}
		procSetConsoleMode.Call(
			uintptr(consoleHdl),
			uintptr(savedMode&^enableVirtualTerminalProcessing),
		)
	})
}
