//go:build !windows

package python

import "os/exec"

// hideWindow is a no-op on non-Windows platforms
func hideWindow(cmd *exec.Cmd) {
	// No-op on non-Windows
}
