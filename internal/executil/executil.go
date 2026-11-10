// Package executil centralizes child-process launch details shared by the
// database manager, JDBC bridge, and Tailscale CLI integration.
package executil

import (
	"context"
	"os/exec"
)

// Command builds a command and applies platform-specific console behavior.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	HideConsole(cmd)
	return cmd
}

// CommandContext builds a context-bound command with hidden console behavior.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	HideConsole(cmd)
	return cmd
}
