// Purpose: Copy text to the system clipboard. Calls macOS `pbcopy` directly
// so the TUI has no extra runtime dependency on a Go clipboard library.
// Docs: copy.doc.md

package tui

import (
	"io"
	"os/exec"
)

// clipboardBin is the executable invoked to push text into the clipboard.
// macOS only today — the bundle has no CI on non-darwin and `pbcopy` is
// the canonical name. Linux/Windows callers will get ENOENT, surfaced
// to the user as a status-bar error rather than a panic.
const clipboardBin = "pbcopy"

// CopyToClipboard writes text to the system clipboard via pbcopy. Returns
// any I/O or exec error verbatim so the caller can decide whether to log,
// toast, or silently ignore.
func CopyToClipboard(text string) error {
	cmd := exec.Command(clipboardBin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Write then close before Wait — pbcopy reads stdin until EOF.
	if _, werr := io.WriteString(stdin, text); werr != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return werr
	}
	if cerr := stdin.Close(); cerr != nil {
		_ = cmd.Wait()
		return cerr
	}
	return cmd.Wait()
}
