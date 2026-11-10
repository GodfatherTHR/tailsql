//go:build !windows

package executil

import "os/exec"

// HideConsole is a no-op on platforms where spawned processes do not create a
// console window.
func HideConsole(cmd *exec.Cmd) {}
