//go:build !windows

package pdf

import "os/exec"

// hideWindowOnWindows 在非 Windows 平台上不做任何操作
func hideWindowOnWindows(cmd *exec.Cmd) {
	// 非 Windows 平台不需要隐藏窗口
}
