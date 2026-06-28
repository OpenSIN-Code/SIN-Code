// SPDX-License-Identifier: MIT
//go:build linux

package tui

import "os/exec"

// CopyToClipboard copies text to the system clipboard using xclip.
func CopyToClipboard(text string) error {
	cmd := exec.Command("xclip", "-selection", "clipboard")
	w, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := w.Write([]byte(text)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}
