// SPDX-License-Identifier: MIT
//go:build darwin

package tui
// sin-debt: shrink, upgrade: when a second darwin-clipboard function is needed, merge into a shared file

import "os/exec"

// CopyToClipboard copies text to the system clipboard using pbcopy.
func CopyToClipboard(text string) error {
	cmd := exec.Command("pbcopy")
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
