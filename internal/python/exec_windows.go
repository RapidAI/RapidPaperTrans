//go:build windows

package python

import (
	"os/exec"
	"syscall"
)

// hideWindow hides the console window on Windows
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
