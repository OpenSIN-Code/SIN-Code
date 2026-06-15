// SPDX-License-Identifier: MIT
// Purpose: shared test helpers for the root package coverage suite.
package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// captureStdout redirects os.Stdout to a pipe and returns a function that
// restores the original stdout and returns everything written during the call.
// Use it to test cobra subcommands that print with fmt.Println/Printf.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		_ = r.Close()
		close(done)
	}()

	return func() string {
		os.Stdout = orig
		_ = w.Close()
		<-done
		return buf.String()
	}
}

// captureStderr redirects os.Stderr to a pipe and returns the captured text.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		_ = r.Close()
		close(done)
	}()

	return func() string {
		os.Stderr = orig
		_ = w.Close()
		<-done
		return buf.String()
	}
}
