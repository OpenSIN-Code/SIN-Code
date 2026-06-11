// SPDX-License-Identifier: MIT

package internal

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
)

// captureStdout returns a function that, when called, returns the captured
// stdout output as a string and restores os.Stdout. The capture runs
// synchronously via a WaitGroup, ensuring the goroutine reading the pipe
// has finished before the returned function is called.
func captureStdout(t *testing.T) (final func() string) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("captureStdout: os.Pipe failed: %v", err)
	}
	os.Stdout = w

	var (
		mu  sync.Mutex
		buf bytes.Buffer
		wg  sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&bufWrapper{buf: &buf, mu: &mu}, r)
	}()

	restore := func() string {
		_ = w.Close()
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		os.Stdout = orig
		return buf.String()
	}
	t.Cleanup(func() { _ = restore() })
	return restore
}

type bufWrapper struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func (b *bufWrapper) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// fakeSinCodeScript is a shell script that echoes its arguments as JSON.
// Used to test the MCP handlers in serve_extra_handlers.go.
const fakeSinCodeScript = `#!/bin/sh
echo "{\"args\":$([ $# -gt 0 ] && echo -n \"[\"; for a in "$@"; do printf '"%s",' "$a"; done; [ $# -gt 0 ] && echo -n ']')}"
`
