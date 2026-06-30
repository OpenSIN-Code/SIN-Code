// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` headless/one-shot mode execution.
// sin-debt: shrink, upgrade: when a second chat-run-related function is needed, merge
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// runHeadlessMode executes the headless one-shot path: dispatch the
// user prompt, set up optional progress output, run the loop, and
// print the result. Returns the result of the loop run or an error.
func runHeadlessMode(
	ctx context.Context,
	opts *chatOptions,
	loop *agentloop.Loop,
	sess *session.Session,
	act *chatActivator,
	dispatchUserPrompt func(string),
	sinCfg internal.SinCodeConfig,
	origStderr io.Writer,
) error {
	dispatchUserPrompt(opts.prompt)

	progress := opts.progress
	if progress == "" {
		progress = sinCfg.OutputProgress
	}
	var progressFile *os.File
	if progress != "off" && progress != "" {
		var w io.Writer = origStderr
		switch opts.progressDest {
		case "stdout":
			w = chatStdout
		case "file":
			if opts.progressFile == "" {
				fmt.Fprintln(chatStderr, "warn: --progress-dest=file requires --progress-file")
			} else {
				f, ferr := os.OpenFile(opts.progressFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filemode.Default())
				if ferr != nil {
					fmt.Fprintf(chatStderr, "warn: cannot open progress file: %v\n", ferr)
				} else {
					w = f
					progressFile = f
				}
			}
		}
		pw := agentloop.NewProgressWriter(w)
		loop.ProgressWriter = pw
		loop.SessionID = sess.ID
		defer func() {
			pw.Close()
			if progressFile != nil {
				_ = progressFile.Close()
			}
		}()
	}

	var res *agentloop.Result
	var err error
	if chatRunOverrideFn != nil {
		res, err = chatRunOverrideFn(ctx, sess, opts.prompt)
	} else {
		res, err = loop.Run(ctx, sess, opts.prompt)
	}
	if err != nil {
		act.Act.EndSession(sess.ID)
		return friendlyError(err)
	}
	act.Act.EndSession(sess.ID)

	// Feature 1: when streaming was active (headless && !jsonOut),
	// the model's text was already printed token-by-token to stdout
	// during loop.Run(). Skip the duplicate summary and only emit
	// the trailing newline + session metadata line.
	if !opts.jsonOut {
		fmt.Fprintln(chatStdout)
		fmt.Fprintf(chatStdout, "[session=%s verified=%v turns=%d]\n", res.SessionID, res.Verified, res.Turns)
		return nil
	}
	return chatPrintResultFn(res, opts.jsonOut)
}
