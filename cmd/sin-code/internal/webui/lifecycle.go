// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when webui is refactored
// Purpose: server lifecycle — ListenAndServe with graceful shutdown, browser
// auto-open, and the listenOn helper.
package webui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func (s *Server) ListenAndServe() error {
	ln, err := netListenHook("tcp", fmt.Sprintf("%s:%d", s.host, s.port))
	if err != nil {
		return err
	}
	s.ln = ln
	s.addr_ = ln.Addr().String()

	s.httpServer = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if s.openBrowser {
		go func() {
			time.Sleep(200 * time.Millisecond)
			_ = openBrowserHook("http://" + s.addr_)
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.Serve(ln)
	}()

	stop := make(chan os.Signal, 1)
	signalNotifyHook(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
}

func openInBrowser(target string) error {
	var cmd *exec.Cmd
	switch goosHook() {
	case "darwin":
		cmd = browserExecHook("open", target) // #nosec G204 — opens validated user URL in browser
	case "windows":
		cmd = browserExecHook("rundll32", "url.dll,FileProtocolHandler", target) // #nosec G204 — opens validated user URL in browser
	default:
		cmd = browserExecHook("xdg-open", target) // #nosec G204 — opens validated user URL in browser
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

func listenOn(host string, port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
}
