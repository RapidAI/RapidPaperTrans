//go:build windows

package pdf

import (
	"os/exec"
	"syscall"
)

// hideWindowOnWindows 在 Windows 上隐藏命令行窗口
func hideWindowOnWindows(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
